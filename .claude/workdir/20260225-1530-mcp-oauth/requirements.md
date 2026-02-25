# Requirements: MCP OAuth 2.1 Provider (fb_19ceb8a1 & fb_9ca054e3)

**Date:** 2026-02-25
**Requested:** Implement OAuth 2.1 authorization server endpoints so MCP clients (Claude.ai, ChatGPT) can authenticate. Fix misleading error messages for unauthenticated requests.

## Scope

**In scope:**
- RFC9728: `GET /.well-known/oauth-protected-resource` — resource metadata discovery
- RFC8414: `GET /.well-known/oauth-authorization-server` — authorization server metadata
- RFC7591: `POST /oauth/register` — dynamic client registration (DCR)
- `GET /oauth/authorize` — authorization endpoint with PKCE S256, minimal login+consent HTML page
- `POST /oauth/token` — token endpoint (authorization_code + refresh_token grants)
- OAuth data models: OAuthClient, OAuthCode, OAuthToken (refresh tokens)
- SurrealDB storage for OAuth data (new OAuthStore)
- Bearer token middleware: validate `Authorization: Bearer <jwt>` and populate UserContext
- Improved error messages for unauthenticated requests (fb_9ca054e3)
- Config: `[auth.oauth2]` section with issuer, token expiry, etc.
- Unit tests for all new code

**Out of scope:**
- Token introspection endpoint (`/oauth/introspect`)
- Token revocation endpoint (`/oauth/revoke`)
- Granular scope enforcement (start with single "vire" scope for full access)
- CIMD (Client ID Metadata Documents)
- MCP transport changes (SSE/Streamable HTTP) — server remains REST API

## Approach

### Architecture

Vire becomes an OAuth 2.1 provider. MCP clients discover auth requirements via well-known endpoints, register dynamically, and obtain tokens via the authorization code flow with PKCE.

**Access tokens**: JWTs (HMAC-SHA256) — same signing mechanism as existing portal JWTs. Add `client_id` and `scope` claims. Self-validating, no DB lookup required.

**Refresh tokens**: Opaque tokens stored in SurrealDB (bcrypt-hashed). Keyed by (user_id, client_id). Rotation on each refresh.

**Auth codes**: Stored in SurrealDB. Short-lived (10 min), single-use. Include PKCE code_challenge.

**OAuth clients**: Stored in SurrealDB via DCR. client_secret is bcrypt-hashed. Redirect URIs validated on authorize/token.

### Flow

1. MCP client discovers `/.well-known/oauth-protected-resource` → finds `authorization_servers`
2. MCP client fetches `/.well-known/oauth-authorization-server` → finds endpoints, PKCE methods
3. MCP client calls `POST /oauth/register` → gets client_id + client_secret
4. MCP client redirects user to `GET /oauth/authorize?client_id=...&code_challenge=...&state=...`
5. User sees login+consent HTML page, enters credentials, grants access
6. Server validates credentials, generates auth code, redirects to client's redirect_uri
7. MCP client calls `POST /oauth/token` with code + code_verifier → gets access_token + refresh_token
8. MCP client uses `Authorization: Bearer <access_token>` on API requests
9. Bearer token middleware validates JWT, extracts user_id, populates UserContext
10. When access token expires, client uses refresh_token grant for new tokens

### New Files

| File | Purpose |
|------|---------|
| `internal/models/oauth.go` | OAuthClient, OAuthCode, OAuthRefreshToken models |
| `internal/interfaces/storage.go` | OAuthStore interface (added to StorageManager) |
| `internal/storage/surrealdb/oauthstore.go` | SurrealDB implementation |
| `internal/server/handlers_oauth.go` | All OAuth 2.1 endpoints |
| `internal/server/oauth_consent.go` | HTML consent/login page template |

### Modified Files

| File | Change |
|------|--------|
| `internal/server/routes.go` | Register /.well-known/* and /oauth/* routes |
| `internal/server/middleware.go` | Add bearer token middleware to resolve UserContext from JWT |
| `internal/common/config.go` | Add OAuth2Config struct to AuthConfig |
| `config/vire-service.toml` | Add [auth.oauth2] section |
| `internal/storage/surrealdb/manager.go` | Add oauthStore field, init, accessor, table definition |

### Key Design Decisions

1. **Combined login+consent page**: The authorize endpoint renders a simple HTML form. User enters credentials AND grants access in one action. No session management needed.
2. **Reuse existing JWT signing**: Access tokens use the same `signJWT` / `validateJWT` functions. The existing `jwt_secret` is shared.
3. **Bearer middleware coexists with X-Vire-* headers**: The new middleware checks for `Authorization: Bearer` first. If present and valid, it populates UserContext from the JWT claims and resolves the user's preferences from InternalStore. If absent, falls through to the existing X-Vire-* header resolution. This maintains backward compatibility with the portal.
4. **Issuer from config**: `[auth.oauth2] issuer` is required for metadata endpoints. Defaults to empty (OAuth2 disabled if not set).
5. **PKCE required**: All authorization requests must include `code_challenge` with `code_challenge_method=S256`.
