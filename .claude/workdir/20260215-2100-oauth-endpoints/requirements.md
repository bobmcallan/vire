# Requirements: OAuth endpoints for portal authentication

**Date:** 2026-02-15
**Requested:** The portal requires OAuth endpoints as per `/home/bobmc/development/vire-portal/docs/authentication.md`

## Scope

### In scope
- `POST /api/auth/oauth` — exchange provider credentials for signed JWT (supports `google`, `github`, `dev` providers)
- `GET /api/auth/login/{provider}` — redirect to provider OAuth page (google, github)
- `GET /api/auth/callback/{provider}` — receive OAuth callback, exchange code, redirect to portal with JWT
- `POST /api/auth/validate` — validate JWT, return user profile
- Modify `POST /api/auth/login` — add signed JWT to response (email/password already works)
- JWT signing with HMAC-SHA256 using `github.com/golang-jwt/jwt/v5`
- Auth config: `jwt_secret`, `token_expiry`, OAuth provider configs
- Dev login flow: `provider: "dev"` → skip external exchange, create/return test user (dev mode only)
- InternalUser gets `Provider` field to track auth source

### Out of scope
- Portal-side changes (separate codebase)
- Email/password registration (users created via import or management API)
- OAuth token refresh
- Multi-factor authentication

## Approach

### Architecture: Server-Side Redirect (Recommended in auth spec)

The portal doesn't handle OAuth at all. vire-server owns the full redirect dance:

```
Browser → GET /api/auth/login/google
Portal  → 302 to vire-server: GET {api_url}/api/auth/login/google?callback={portal_callback_url}
Server  → 302 to Google OAuth (with server's client_id, redirect_uri pointing back to server)
Google  → callback to server with code
Server  → exchanges code, creates user, generates JWT
Server  → 302 to portal callback: GET {portal_callback_url}?token={jwt}
Portal  → sets vire_session cookie, 302 to /dashboard
```

### JWT Design

```json
{
  "alg": "HS256",
  "typ": "JWT"
}
{
  "sub": "username",
  "email": "user@example.com",
  "name": "Display Name",
  "provider": "google|github|email|dev",
  "iss": "vire-server",
  "iat": 1739750400,
  "exp": 1739836800
}
```

- Signing: HMAC-SHA256 with configurable secret
- Expiry: 24 hours (configurable via `token_expiry`)
- Issuer: `vire-server`
- New dependency: `github.com/golang-jwt/jwt/v5`

### Config Changes

Add `AuthConfig` to `Config`:

```go
type AuthConfig struct {
    JWTSecret   string        `toml:"jwt_secret"`
    TokenExpiry string        `toml:"token_expiry"` // duration string, default "24h"
    Google      OAuthProvider `toml:"google"`
    GitHub      OAuthProvider `toml:"github"`
}

type OAuthProvider struct {
    ClientID     string `toml:"client_id"`
    ClientSecret string `toml:"client_secret"`
}
```

Environment overrides:
- `VIRE_AUTH_JWT_SECRET`
- `VIRE_AUTH_TOKEN_EXPIRY`
- `VIRE_AUTH_GOOGLE_CLIENT_ID`, `VIRE_AUTH_GOOGLE_CLIENT_SECRET`
- `VIRE_AUTH_GITHUB_CLIENT_ID`, `VIRE_AUTH_GITHUB_CLIENT_SECRET`

### New Endpoints

| Route | Method | Description |
|-------|--------|-------------|
| `POST /api/auth/oauth` | POST | Exchange `{provider, code, state}` for `{token, user}`. Branches: google→Google exchange, github→GitHub exchange, dev→create test user (dev mode only) |
| `GET /api/auth/login/google` | GET | Redirect to Google OAuth. Accepts `?callback=` for portal return URL |
| `GET /api/auth/login/github` | GET | Redirect to GitHub OAuth. Accepts `?callback=` for portal return URL |
| `GET /api/auth/callback/google` | GET | Google OAuth callback → exchange code → redirect to portal `?callback=` with `?token=` |
| `GET /api/auth/callback/github` | GET | GitHub OAuth callback → exchange code → redirect to portal `?callback=` with `?token=` |
| `POST /api/auth/validate` | POST | Validate `Authorization: Bearer <jwt>`, return user profile |

### Modified Endpoints

| Route | Change |
|-------|--------|
| `POST /api/auth/login` | Add signed JWT (`token` field) to response alongside existing user data |

### InternalUser Change

Add `Provider` field to track how the user was created:

```go
type InternalUser struct {
    UserID       string    `json:"user_id"`
    Email        string    `json:"email"`
    PasswordHash string    `json:"password_hash"`
    Provider     string    `json:"provider"` // "email", "google", "github", "dev"
    Role         string    `json:"role"`
    CreatedAt    time.Time `json:"created_at"`
    ModifiedAt   time.Time `json:"modified_at"`
}
```

### Implementation: New File `internal/server/handlers_auth.go`

All OAuth/JWT logic goes in a new handlers_auth.go file to keep it separate from user CRUD:

- `handleAuthOAuth` — `POST /api/auth/oauth` dispatcher
- `handleOAuthLoginGoogle` — `GET /api/auth/login/google`
- `handleOAuthLoginGitHub` — `GET /api/auth/login/github`
- `handleOAuthCallbackGoogle` — `GET /api/auth/callback/google`
- `handleOAuthCallbackGitHub` — `GET /api/auth/callback/github`
- `handleAuthValidate` — `POST /api/auth/validate`
- `signJWT(user, provider)` — helper to create signed JWT
- `validateJWT(tokenString)` — helper to parse and validate JWT
- `exchangeGoogleCode(code)` — exchange code with Google
- `exchangeGitHubCode(code)` — exchange code with GitHub

### State Management for OAuth

Use HMAC-signed state parameter for CSRF protection:
- Server generates `state = HMAC(callback_url + timestamp, jwt_secret)`
- State includes the portal callback URL and a nonce
- Stored temporarily in a short-lived in-memory map (or encoded in the state value itself)
- Validated on callback

## Files Expected to Change

### New files
- `internal/server/handlers_auth.go` — OAuth endpoints, JWT signing/validation
- `internal/server/handlers_auth_test.go` — Tests

### Modified files
- `internal/common/config.go` — Add `AuthConfig`, `OAuthProvider` structs, env overrides
- `internal/models/storage.go` — Add `Provider` field to `InternalUser`
- `internal/server/routes.go` — Register new auth routes
- `internal/server/handlers_user.go` — Add JWT token to `handleAuthLogin` response
- `config/vire-service.toml.example` — Add `[auth]` section
- `go.mod` / `go.sum` — Add `github.com/golang-jwt/jwt/v5`
- `README.md` — Document new auth endpoints
- `.claude/skills/develop/SKILL.md` — Update auth endpoint table
