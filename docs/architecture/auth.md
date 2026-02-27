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

## Config

```toml
[auth]
jwt_secret = "change-me-in-production"
token_expiry = "24h"

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

Env overrides: `VIRE_AUTH_JWT_SECRET`, `VIRE_AUTH_TOKEN_EXPIRY`, `VIRE_AUTH_GOOGLE_CLIENT_ID`, `VIRE_AUTH_GOOGLE_CLIENT_SECRET`, `VIRE_AUTH_GITHUB_CLIENT_ID`, `VIRE_AUTH_GITHUB_CLIENT_SECRET`, `VIRE_OAUTH2_ISSUER`.
