# vire-server: OAuth Provider Requirements

**Date:** 2026-02-22
**Status:** Phase 2 & 3 implemented
**Audience:** vire-server developer

## Summary

The portal side of Google and GitHub OAuth is complete. The portal redirects to vire-server and handles the JWT callback. What remains is vire-server implementing the provider-facing OAuth endpoints that sit between the portal redirect and the portal callback.

Portal redirect stubs already call:

```
GET {apiURL}/api/auth/login/google?callback={portalCallbackURL}
GET {apiURL}/api/auth/login/github?callback={portalCallbackURL}
```

vire-server needs to handle these requests, redirect to the provider, receive the authorization code, exchange it for user info, mint a signed JWT, and redirect back to the portal callback with the token.

## Current State

### What exists (portal)

| Component | Status | Detail |
|-----------|--------|--------|
| `GET /api/auth/login/google` | Done | 302 to `{apiURL}/api/auth/login/google?callback={callbackURL}` |
| `GET /api/auth/login/github` | Done | 302 to `{apiURL}/api/auth/login/github?callback={callbackURL}` |
| `GET /auth/callback?token=<jwt>` | Done | Sets `vire_session` cookie, redirects to `/dashboard` |
| JWT validation (HMAC-SHA256) | Done | `ValidateJWT(token, secret)` checks signature + expiry |
| MCP session completion on callback | Done | If `mcp_session_id` cookie present, completes MCP OAuth flow |
| Email/password login | Done | `POST /api/auth/login` forwards to vire-server |
| Integration tests | Done | `TestOAuthRedirect_GoogleCallbackChain`, `TestOAuthRedirect_GitHubCallbackChain` |
| UI browser test | Done | `TestAuthGoogleLoginRedirect` verifies redirect leaves portal |

### What exists (vire-server)

| Component | Status | Detail |
|-----------|--------|--------|
| `POST /api/auth/login` | Done | Email/password login with bcrypt, returns signed JWT |
| `POST /api/auth/oauth` | Done | Provider-based exchange (handles `dev`, `google`, `github`) |
| `GET /api/auth/login/google` | Done | Accept `?callback=`, HMAC-signed state, redirect to Google consent screen |
| `GET /api/auth/callback/google` | Done | Receive code from Google, exchange for token, fetch profile, mint JWT, redirect to portal |
| `GET /api/auth/login/github` | Done | Accept `?callback=`, HMAC-signed state, redirect to GitHub consent screen |
| `GET /api/auth/callback/github` | Done | Receive code from GitHub, exchange for token, fetch profile (with email fallback), mint JWT, redirect to portal |
| JWT signing (HMAC-SHA256) | Done | Shared secret with portal, includes `name` claim from provider profile |
| User creation/lookup | Done | Users stored in SurrealDB, upsert support |
| Email-based account linking | Done | `GetUserByEmail` (case-insensitive) links accounts across providers by email |
| Error redirects in callbacks | Done | All post-state-decode errors redirect to portal with `?error={code}` instead of JSON errors |
| Provider user name tracking | Done | `InternalUser.Name` field populated from provider profile, updated on re-login |

## Full OAuth Flow

```
Browser              Portal (:8500)           vire-server (:8501)       Provider
  |                      |                          |                       |
  |  click "Sign in      |                          |                       |
  |  with Google"        |                          |                       |
  |--------------------->|                          |                       |
  |                      |                          |                       |
  |  302 to vire-server  |                          |                       |
  |  /api/auth/login/    |                          |                       |
  |  google?callback=... |                          |                       |
  |<---------------------|                          |                       |
  |                                                 |                       |
  |  GET /api/auth/login/google?callback=...        |                       |
  |------------------------------------------------>|                       |
  |                                                 |                       |
  |                                 generate state  |                       |
  |                                 store {state -> callback}               |
  |                                                 |                       |
  |  302 to accounts.google.com/o/oauth2/v2/auth    |                       |
  |  ?client_id=...&redirect_uri=.../callback/google|                       |
  |  &scope=openid+email+profile&state=...          |                       |
  |<------------------------------------------------|                       |
  |                                                                         |
  |  user authenticates with Google                                         |
  |------------------------------------------------------------------------>|
  |                                                                         |
  |  302 to vire-server /api/auth/callback/google?code=...&state=...        |
  |<------------------------------------------------------------------------|
  |                                                                         |
  |  GET /api/auth/callback/google?code=...&state=...                       |
  |------------------------------------------------>|                       |
  |                                                 |                       |
  |                                 validate state  |                       |
  |                                 lookup callback |                       |
  |                                                 |  POST token endpoint  |
  |                                                 |  exchange code        |
  |                                                 |---------------------->|
  |                                                 |  { access_token }     |
  |                                                 |<----------------------|
  |                                                 |                       |
  |                                                 |  GET /userinfo        |
  |                                                 |---------------------->|
  |                                                 |  { email, name, ... } |
  |                                                 |<----------------------|
  |                                                 |                       |
  |                                 create/update user                      |
  |                                 mint JWT (HS256)                        |
  |                                                 |                       |
  |  302 to portal /auth/callback?token={jwt}       |                       |
  |<------------------------------------------------|                       |
  |                                                                         |
  |  GET /auth/callback?token={jwt}                 |                       |
  |--------------------->|                          |                       |
  |                      |                          |                       |
  |  Set-Cookie:         |                          |                       |
  |  vire_session={jwt}  |                          |                       |
  |  302 to /dashboard   |                          |                       |
  |<---------------------|                          |                       |
```

## vire-server Endpoints Required

### 1. `GET /api/auth/login/google`

Initiates the Google OAuth flow.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `callback` | Yes | Portal callback URL to redirect to after exchange (e.g. `http://localhost:8500/auth/callback`) |

**Behavior:**

1. Generate a random `state` value (min 32 bytes, base64url encoded)
2. Store `{ state -> callback }` with a TTL (10 minutes)
3. Build Google authorization URL:
   - Endpoint: `https://accounts.google.com/o/oauth2/v2/auth`
   - `client_id`: from config `[auth.google].client_id`
   - `redirect_uri`: `{server_base_url}/api/auth/callback/google`
   - `response_type`: `code`
   - `scope`: `openid email profile`
   - `state`: generated state value
   - `access_type`: `offline` (optional, for refresh tokens)
4. 302 redirect to Google authorization URL

**Error cases:**
- Missing `callback` param: 400 Bad Request
- Google OAuth not configured: 501 Not Implemented (or redirect to portal with `?error=provider_not_configured`)

### 2. `GET /api/auth/callback/google`

Receives the authorization code from Google after user consent.

**Query parameters:**

| Param | Required | Description |
|-------|----------|-------------|
| `code` | Yes | Authorization code from Google |
| `state` | Yes | State parameter for CSRF validation |
| `error` | No | Error from Google (e.g. `access_denied`) |

**Behavior:**

1. If `error` param present: redirect to portal with `?error={error}`
2. Validate `state` against stored values; reject if missing or expired
3. Look up the portal `callback` URL from the state store
4. Exchange `code` for tokens with Google:
   - POST `https://oauth2.googleapis.com/token`
   - Body: `{ code, client_id, client_secret, redirect_uri, grant_type: "authorization_code" }`
   - Response: `{ access_token, id_token, ... }`
5. Fetch user profile from Google:
   - GET `https://www.googleapis.com/oauth2/v2/userinfo` with `Authorization: Bearer {access_token}`
   - Response: `{ id, email, name, picture, verified_email }`
6. Create or update user in database:
   - Match by email
   - Set `provider: "google"`, store Google user ID
   - Set `name`, `picture` if provided
7. Mint HMAC-SHA256 signed JWT:
   ```json
   {
     "sub": "<vire-user-id>",
     "email": "<email>",
     "name": "<display-name>",
     "provider": "google",
     "iss": "vire-server",
     "iat": <now>,
     "exp": <now + 24h>
   }
   ```
8. Delete the state entry (single-use)
9. 302 redirect to `{callback}?token={jwt}`

**Error cases:**
- Missing `code` or `state`: 400 Bad Request
- Invalid/expired `state`: 403 Forbidden (or redirect to portal with error)
- Google token exchange fails: redirect to portal with `?error=exchange_failed`
- Google userinfo fetch fails: redirect to portal with `?error=profile_failed`

### 3. `GET /api/auth/login/github`

Same pattern as Google. Initiates the GitHub OAuth flow.

**Differences from Google:**

| | Google | GitHub |
|---|--------|--------|
| Authorization URL | `https://accounts.google.com/o/oauth2/v2/auth` | `https://github.com/login/oauth/authorize` |
| Scopes | `openid email profile` | `read:user user:email` |
| Config | `[auth.google]` | `[auth.github]` |
| Callback route | `/api/auth/callback/google` | `/api/auth/callback/github` |

**Query parameters:** Same as Google (`callback` required).

### 4. `GET /api/auth/callback/github`

Receives the authorization code from GitHub.

**Differences from Google callback:**

| Step | Google | GitHub |
|------|--------|--------|
| Token exchange | POST `https://oauth2.googleapis.com/token` (JSON) | POST `https://github.com/login/oauth/access_token` (Accept: application/json) |
| User profile | GET `https://www.googleapis.com/oauth2/v2/userinfo` | GET `https://api.github.com/user` |
| Email (if private) | Included in userinfo | GET `https://api.github.com/user/emails` (pick primary verified) |
| User ID field | `id` (numeric string) | `id` (integer), `login` (username) |
| Provider claim | `"google"` | `"github"` |

**GitHub token exchange:**
```
POST https://github.com/login/oauth/access_token
Accept: application/json
Content-Type: application/json

{
  "client_id": "...",
  "client_secret": "...",
  "code": "...",
  "redirect_uri": "..."
}
```

Response: `{ "access_token": "...", "token_type": "bearer", "scope": "..." }`

**GitHub user profile:**
```
GET https://api.github.com/user
Authorization: Bearer {access_token}
```

Response: `{ "id": 12345, "login": "octocat", "email": "...", "name": "...", "avatar_url": "..." }`

If `email` is null (private), fetch emails:
```
GET https://api.github.com/user/emails
Authorization: Bearer {access_token}
```

Response: `[ { "email": "...", "primary": true, "verified": true }, ... ]`

Pick the first entry where `primary == true && verified == true`.

## Configuration Required

### vire-server TOML

```toml
[auth]
jwt_secret = "shared-secret-with-portal"
token_expiry = "24h"

[auth.google]
client_id = ""
client_secret = ""

[auth.github]
client_id = ""
client_secret = ""
```

### Environment variable overrides

| Variable | Config key |
|----------|-----------|
| `VIRE_AUTH_JWT_SECRET` | `auth.jwt_secret` |
| `VIRE_AUTH_TOKEN_EXPIRY` | `auth.token_expiry` |
| `VIRE_AUTH_GOOGLE_CLIENT_ID` | `auth.google.client_id` |
| `VIRE_AUTH_GOOGLE_CLIENT_SECRET` | `auth.google.client_secret` |
| `VIRE_AUTH_GITHUB_CLIENT_ID` | `auth.github.client_id` |
| `VIRE_AUTH_GITHUB_CLIENT_SECRET` | `auth.github.client_secret` |

### Provider setup

**Google:**
1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create OAuth 2.0 Client ID (Web application)
3. Add authorized redirect URI: `{server_base_url}/api/auth/callback/google`
4. Copy Client ID and Client Secret to config

**GitHub:**
1. Go to [GitHub Developer Settings](https://github.com/settings/developers)
2. Create a new OAuth App
3. Set Authorization callback URL: `{server_base_url}/api/auth/callback/github`
4. Copy Client ID and Client Secret to config

## State Store

The state parameter links a login request to its callback. It must be:

- Random: minimum 32 bytes, base64url encoded
- Single-use: deleted after successful exchange
- Time-limited: 10-minute TTL
- Maps to: the portal `callback` URL that initiated the flow

Recommended implementation: in-memory map with mutex (matches existing `CodeStore` and `SessionStore` patterns in the portal's MCP OAuth implementation).

```go
type OAuthState struct {
    CallbackURL string
    CreatedAt   time.Time
}

type StateStore struct {
    mu     sync.RWMutex
    states map[string]OAuthState // state -> OAuthState
    ttl    time.Duration         // 10 minutes
}
```

## User Mapping

When a user authenticates via Google or GitHub, vire-server must create or update the user record.

### Matching strategy

1. Look up user by email (case-insensitive)
2. If found: update `name`, `picture`/`avatar_url`, record provider
3. If not found: create new user with:
   - `username`: email local part (or GitHub `login`)
   - `email`: from provider
   - `role`: `"user"` (default)
   - `provider`: `"google"` or `"github"`
   - No password (OAuth-only users cannot use email/password login)

### Account linking

A user who first logs in with Google and later with GitHub (same email) should get the same account. The email is the primary key for matching. Store the provider used for each login in the JWT `provider` claim so the portal can display it.

## JWT Contract

The JWT returned to the portal must match this format exactly. The portal's `ValidateJWT` function expects these fields:

```go
type JWTClaims struct {
    Sub      string `json:"sub"`      // vire user ID (required)
    Email    string `json:"email"`    // user email (required)
    Name     string `json:"name"`     // display name
    Provider string `json:"provider"` // "google", "github", "email"
    Iss      string `json:"iss"`      // must be "vire-server"
    Iat      int64  `json:"iat"`      // issued at (unix seconds)
    Exp      int64  `json:"exp"`      // expiry (unix seconds, required)
}
```

- **Signing:** HMAC-SHA256 with shared `jwt_secret`
- **Header:** `{ "alg": "HS256", "typ": "JWT" }`
- **Expiry:** 24 hours from issue time

## Error Handling

On any failure during the OAuth flow, vire-server should redirect back to the portal rather than showing a raw error page:

```
302 to {callback}?error={error_code}
```

Error codes the portal should handle:

| Code | Meaning |
|------|---------|
| `provider_not_configured` | OAuth client ID/secret not set for this provider |
| `invalid_state` | State parameter missing, expired, or already used |
| `access_denied` | User denied consent at the provider |
| `exchange_failed` | Code-to-token exchange with provider failed |
| `profile_failed` | Could not fetch user profile from provider |
| `user_creation_failed` | Could not create/update user in database |

The portal's `HandleOAuthCallback` currently checks for an empty `token` param and redirects to `/error?reason=auth_failed`. It should also check for an `error` param.

## Testing

### Unit tests (vire-server)

1. **State generation and validation** -- generate state, store it, retrieve it, verify TTL expiry, verify single-use deletion
2. **Google token exchange** -- mock Google token endpoint, verify request format, handle success and error responses
3. **GitHub token exchange** -- mock GitHub token endpoint, verify Accept header, handle success and error
4. **GitHub email fallback** -- mock `/user/emails` endpoint for private email case
5. **User creation** -- verify new user created with correct fields from provider profile
6. **User matching** -- verify existing user found by email, fields updated
7. **JWT minting** -- verify JWT contains correct claims, signed with correct secret
8. **Redirect URL construction** -- verify `{callback}?token={jwt}` format
9. **Error redirects** -- verify error codes redirected correctly

### Integration tests (vire-server)

1. Full flow with mock provider: `GET /api/auth/login/google?callback=...` -> verify redirect to mock Google -> `GET /api/auth/callback/google?code=...&state=...` -> verify redirect to callback with token
2. Same for GitHub

### Portal integration tests (already exist)

- `TestOAuthRedirect_GoogleCallbackChain` -- verifies portal redirect URL format and callback cookie handling
- `TestOAuthRedirect_GitHubCallbackChain` -- same for GitHub
- `TestAuthGoogleLoginRedirect` -- UI browser test verifying redirect leaves portal

### Manual end-to-end test

1. Configure real Google/GitHub credentials in vire-server
2. Start vire-server and portal
3. Click "Sign in with Google" on landing page
4. Complete Google login
5. Verify redirect back to portal dashboard with valid session

## Implementation Order

1. **State store** -- reusable for both providers
2. **Google login + callback** -- `GET /api/auth/login/google`, `GET /api/auth/callback/google`
3. **Google user mapping** -- create/update user from Google profile
4. **Google tests** -- unit + integration with mock provider
5. **GitHub login + callback** -- same pattern, different URLs and scopes
6. **GitHub email fallback** -- handle private email via `/user/emails`
7. **GitHub tests** -- unit + integration
8. **Portal error handling** -- update `HandleOAuthCallback` to check `error` param
9. **End-to-end test** -- manual test with real credentials

## Files Expected to Change (vire-server)

| File | Change |
|------|--------|
| `auth_handler.go` (or equivalent) | Add `HandleGoogleLogin`, `HandleGoogleCallback`, `HandleGitHubLogin`, `HandleGitHubCallback` |
| `state_store.go` (new) | OAuth state parameter storage with TTL |
| `google_oauth.go` (new) | Google token exchange and userinfo fetch |
| `github_oauth.go` (new) | GitHub token exchange, user fetch, email fallback |
| `user_store.go` / `user_handler.go` | Add find-by-email, create-from-oauth methods |
| `config.go` | Add `[auth.google]` and `[auth.github]` config sections |
| `routes.go` | Register new OAuth endpoints |
| `*_test.go` | Unit and integration tests for all of the above |

## No Portal Changes Required

The portal side is complete. No code changes are needed in vire-portal for Phases 2 and 3, with one minor exception: `HandleOAuthCallback` should check for an `error` query parameter in addition to checking for a missing `token`. This is a small defensive improvement:

```go
// In HandleOAuthCallback:
if errCode := r.URL.Query().Get("error"); errCode != "" {
    http.Redirect(w, r, "/error?reason="+errCode, http.StatusFound)
    return
}
```
