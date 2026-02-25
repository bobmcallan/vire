# Summary: MCP OAuth 2.1 Provider (fb_19ceb8a1 & fb_9ca054e3)

**Date:** 2026-02-25
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/oauth.go` | New: OAuthClient, OAuthCode, OAuthRefreshToken models |
| `internal/interfaces/storage.go` | Added OAuthStore interface to StorageManager |
| `internal/storage/surrealdb/oauthstore.go` | New: SurrealDB implementation (3 tables: oauth_client, oauth_code, oauth_refresh_token) |
| `internal/storage/surrealdb/manager.go` | Added oauthStore field, init, accessor, table definitions |
| `internal/server/handlers_oauth.go` | New: All OAuth 2.1 endpoints (metadata, DCR, authorize, token) |
| `internal/server/oauth_consent.go` | New: HTML login+consent page template (dark theme) |
| `internal/server/handlers_oauth_test.go` | New: 25 unit tests (metadata, DCR, full auth flow, PKCE, bearer, errors) |
| `internal/server/handlers_oauth_stress_test.go` | New: 40 stress tests (PKCE bypass, code replay, redirect manipulation, DCR abuse, token attacks, XSS, info leakage) |
| `tests/api/oauth_test.go` | New: 17 API integration tests (full flow, metadata, DCR, error paths, bearer) |
| `internal/server/routes.go` | Registered /.well-known/* and /oauth/* routes |
| `internal/server/middleware.go` | Added bearerTokenMiddleware (JWT → UserContext) |
| `internal/server/server.go` | Pass config to applyMiddleware for bearer auth |
| `internal/server/handlers.go` | requireNavexaContext returns 401 + WWW-Authenticate when OAuth2 configured |
| `internal/common/config.go` | Added OAuth2Config struct, getter methods, env overrides |
| `config/vire-service.toml.example` | Added [auth.oauth2] section |
| `.claude/skills/develop/SKILL.md` | Added OAuth 2.1 Provider documentation section |
| `README.md` | Added OAuth 2.1 mention |

## Tests

- **Unit tests added**: 25 in handlers_oauth_test.go
- **Stress tests added**: 40 in handlers_oauth_stress_test.go
- **API integration tests added**: 17 in tests/api/oauth_test.go
- **All 82 tests pass** (25 unit + 40 stress + 17 integration)
- **Test feedback rounds**: 3 (Docker config, requireNavexaContext error differentiation, jti claim)

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — OAuth 2.1 Provider section (endpoints, models, config, flow)
- `README.md` — OAuth 2.1 mention

## Devils-Advocate Findings

| # | Finding | Resolution |
|---|---------|------------|
| DA-1 | Unknown client_id redirects to untrusted URI | **Fixed**: Returns 400 error page per RFC 6749 §4.1.2.1 |
| DA-2 | DCR accepts unlimited client_name length and redirect_uris | **Fixed**: client_name capped at 200 chars, redirect_uris capped at 10, scheme restricted to http/https |
| DA-3 | MarkCodeUsed failure silently logged, code replay possible on DB error | **Fixed**: Token exchange aborts if code can't be marked used |
| DA-4 | Arbitrary scope values embedded in JWT | **Fixed**: All scopes normalized to "vire" |
| DA-5 | No rate limiting on DCR or /oauth/token | Accepted: standard rate limiting at reverse proxy level |
| DA-6 | TOCTOU race in concurrent code exchange | Accepted: mitigated by in-memory store atomicity; SurrealDB atomic UPDATE prevents double-use in production |

## Reviewer Findings

| # | Finding | Resolution |
|---|---------|------------|
| R-1 | 5 issues found in initial review | All addressed by implementer before test execution |
| R-2 | Documentation review | Zero corrections needed — docs match implementation |

## Notes
- Access tokens are JWTs (reuse existing HMAC-SHA256 signing) with added `client_id`, `scope`, `jti` claims
- Refresh tokens are opaque (SHA-256 hashed in DB), rotated on each refresh
- Bearer middleware coexists with X-Vire-* header auth — backward compatible with portal
- OAuth2 is opt-in: endpoints return 404 if `[auth.oauth2] issuer` is not configured
- Consent page is a minimal HTML form with inline CSS (dark theme), no JS dependencies
- PKCE S256 is required for all authorization requests
- Pre-existing test failures in server package (SurrealDB-dependent) are unrelated to this change
- Feedback items fb_19ceb8a1 and fb_9ca054e3 marked as resolved
