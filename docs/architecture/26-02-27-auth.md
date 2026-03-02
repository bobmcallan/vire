# Authentication & Authorization

## JWT

HMAC-SHA256 signed using `github.com/golang-jwt/jwt/v5`. Claims: sub (user_id), email, name, provider, role, iss ("vire-server"), iat, exp. Dev provider blocked in production via `config.IsProduction()`.

## OAuth Providers (Google, GitHub)

Login via `findOrCreateOAuthUser`: lookup by provider-specific user_id first, then by email (case-insensitive) for cross-provider account linking, create new if neither match. State parameters use HMAC-signed base64 payloads with 10-minute expiry for CSRF protection. Callback errors redirect with `?error={code}`.

## MCP OAuth 2.1 Provider

Vire acts as an OAuth 2.1 authorization server for MCP clients (Claude.ai, ChatGPT). Implements RFC 9728, RFC 8414, RFC 7591. All endpoints in `internal/server/handlers_oauth.go`.

**Flow:**
1. Client discovers `/.well-known/oauth-protected-resource` → `authorization_servers`
2. Client fetches `/.well-known/oauth-authorization-server` → endpoints, PKCE methods
3. Client calls `POST /oauth/register` → client_id + client_secret
4. Client redirects to `GET /oauth/authorize` with PKCE code_challenge
5. User authenticates on consent page, grants access
6. Server validates, generates auth code, redirects to redirect_uri
7. Client calls `POST /oauth/token` with code + code_verifier → access_token + refresh_token
8. Client uses `Authorization: Bearer` on API requests
9. Refresh token grant rotates tokens (old revoked, new issued)

**Access tokens:** JWTs with jti, sub, email, name, role, client_id, scope, iss, iat, exp. Signed via `signOAuthAccessToken`.

**Auth codes:** SurrealDB-stored, single-use, configurable expiry (default 10m). PKCE code_challenge required (S256 only).

**Consent page:** `internal/server/oauth_consent.go`. Dark-themed HTML template.

**Security:** PKCE required. Unknown client_id returns 400 (never redirects). DCR limits: name max 200, URIs max 10, http/https only. Scope normalized to "vire".

## MCP Session Persistence

vire-portal is stateless (Fly.io restarts wipe in-memory OAuth state). vire-server persists OAuth state in SurrealDB via an internal REST API so portal restarts don't break active MCP client sessions.

**`OAuthSession`** (`models.OAuthSession`) stores pending auth sessions (before user logs in):
- Fields: session_id, client_id, redirect_uri, state, code_challenge, code_method, scope, user_id (filled after login), created_at
- TTL: 10 minutes, enforced at application level in `GetSession`
- Table: `mcp_auth_session`

**`OAuthClient`** extended with portal-facing fields (`grant_types`, `response_types`, `token_endpoint_auth_method` — all `omitempty`, backward compatible).

**Data flow:** portal → `POST /api/internal/oauth/sessions` → `OAuthStore.SaveSession` → SurrealDB `mcp_auth_session`. Tokens sent as plaintext in POST body; server hashes with SHA-256 (same algorithm as `hashRefreshToken`) before storing. Internal API has no auth — restricted to internal Docker/Fly network.

**Store methods added to `OAuthStore`:** `SaveSession`, `GetSession`, `GetSessionByClientID`, `UpdateSessionUserID`, `DeleteSession`, `PurgeExpiredSessions`.

## Break-Glass Admin

Emergency admin account created at startup when `breakglass = true` in `[auth]` config. Credentials are logged at WARN level for visibility.

**Bootstrap logic** (`internal/app/breakglass.go`):
1. Check if `breakglass-admin` user exists — if found, log info and return (idempotent)
2. Generate 24-char cryptographically random password (`crypto/rand`, base64)
3. bcrypt hash (cost 10, truncate to 72 bytes)
4. Save user: UserID=`breakglass-admin`, Email=`admin@vire.local`, Name=`Break-Glass Admin`, Provider=`system`, Role=`admin`
5. Log at WARN level with `email` and `password` fields (cleartext in logs for emergency access)

**Multi-instance safety**: Only set `breakglass = true` on the primary instance. The check-before-create pattern means secondary instances skip silently if the admin already exists. No distributed locking needed.

**Config**: `[auth] breakglass = true` or env `VIRE_AUTH_BREAKGLASS=true`.

## Service Registration

Portal instances register as service users with vire-server using a shared key (`VIRE_SERVICE_KEY`). Service users can list users and update roles (for admin sync at startup) but cannot login via password or OAuth.

**Registration** (`POST /api/services/register`):
1. Portal sends `service_id`, `service_key`, `service_type` in JSON body
2. Server validates: key configured (501), key >= 32 chars (400), key match via constant-time compare (403), service_id non-empty (400)
3. Creates/updates service user: UserID=`service:<service_id>`, Email=`<service_id>@service.vire.local`, Provider=`service`, Role=`service`
4. Idempotent: re-registration only updates `modified_at` (heartbeat)
5. Returns `{"status":"ok","service_user_id":"service:...","registered_at":"..."}`

**Authentication**: After registration, portal sends `X-Vire-Service-ID: service:<id>` header. Middleware resolves service identity (lowest priority after Bearer token and X-Vire-User-ID).

**Permissions**: `requireAdminOrService()` grants access to `GET /api/admin/users` and `PATCH /api/admin/users/{id}/role`. All other admin endpoints remain admin-only. `PATCH /api/admin/users/{id}/role` rejects `"service"` as a target role.

**Login block**: `POST /api/auth/login` rejects users with `provider == "service"` with 403.

**Tidy** (`POST /api/admin/services/tidy`): Admin-only. Purges service users with `modified_at` older than 7 days. Returns `{"purged": N, "remaining": M}`.

**Config**: `[auth] service_key = "..."` or env `VIRE_SERVICE_KEY=<32+ char string>`. If unset, registration returns 501.

**Implementation**: `internal/server/handlers_service.go` (handlers), `internal/server/handlers_admin.go` (`requireAdminOrService`), `internal/server/middleware.go` (X-Vire-Service-ID resolution).

## Config

```toml
[auth]
jwt_secret = "change-me-in-production"
token_expiry = "24h"
breakglass = true  # create break-glass admin on startup
service_key = ""   # shared key for service registration (env: VIRE_SERVICE_KEY)

[auth.google]
client_id = ""
client_secret = ""

[auth.github]
client_id = ""
client_secret = ""

[auth.oauth2]
issuer = "https://api.vire.au"
code_expiry = "10m"
access_token_expiry = "1h"
refresh_token_expiry = "720h"
```

Env overrides: `VIRE_AUTH_JWT_SECRET`, `VIRE_AUTH_TOKEN_EXPIRY`, `VIRE_AUTH_BREAKGLASS`, `VIRE_SERVICE_KEY`, `VIRE_AUTH_GOOGLE_CLIENT_ID`, `VIRE_AUTH_GOOGLE_CLIENT_SECRET`, `VIRE_AUTH_GITHUB_CLIENT_ID`, `VIRE_AUTH_GITHUB_CLIENT_SECRET`, `VIRE_OAUTH2_ISSUER`.
