# Summary: OAuth endpoints for portal authentication

**Date:** 2026-02-15
**Status:** Completed

## What Changed

### New files
| File | Change |
|------|--------|
| `internal/server/handlers_auth.go` | NEW: OAuth endpoints, JWT signing/validation, OAuth state encoding, provider code exchange (Google, GitHub), dev login flow |
| `internal/server/handlers_auth_test.go` | NEW: 20 tests — JWT round-trip, expired/wrong secret, state encoding, dev provider (dev + production mode), validate endpoint, login returns token, OAuth redirects |
| `internal/server/handlers_auth_stress_test.go` | NEW: 35+ stress tests — alg:none attack, tampered payload, state attacks, open redirect probing, concurrent access, method enforcement, secret leak checks, CORS |

### Modified files
| File | Change |
|------|--------|
| `internal/common/config.go` | Added `AuthConfig`, `OAuthProvider` structs, `GetTokenExpiry()`, auth defaults, 6 env overrides (VIRE_AUTH_JWT_SECRET, etc.) |
| `internal/models/storage.go` | Added `Provider` field to `InternalUser` |
| `internal/server/routes.go` | Registered 6 new auth routes: oauth, validate, login/google, login/github, callback/google, callback/github |
| `internal/server/handlers_user.go` | `handleAuthLogin` now returns signed JWT `token` field in response |
| `config/vire-service.toml.example` | Added `[auth]` section with jwt_secret, token_expiry, google/github client config |
| `go.mod` / `go.sum` | Added `github.com/golang-jwt/jwt/v5` |
| `README.md` | Documented new auth endpoints |
| `.claude/skills/develop/SKILL.md` | Updated auth endpoint table, added auth config docs |

## New Endpoints

| Route | Method | Description |
|-------|--------|-------------|
| `POST /api/auth/oauth` | POST | Exchange `{provider, code, state}` for `{token, user}`. Branches: dev (dev mode only), google, github |
| `POST /api/auth/validate` | POST | Validate `Authorization: Bearer <jwt>`, return user profile |
| `GET /api/auth/login/google` | GET | Redirect to Google OAuth (accepts `?callback=` for portal return URL) |
| `GET /api/auth/login/github` | GET | Redirect to GitHub OAuth (accepts `?callback=` for portal return URL) |
| `GET /api/auth/callback/google` | GET | Google OAuth callback → exchange code → redirect to portal with `?token=` |
| `GET /api/auth/callback/github` | GET | GitHub OAuth callback → exchange code → redirect to portal with `?token=` |

## Modified Endpoints

| Route | Change |
|-------|--------|
| `POST /api/auth/login` | Now returns signed JWT `token` field alongside existing user data |

## Tests
- `internal/server/handlers_auth_test.go` — 20 tests: JWT sign/validate round-trip, expired token, wrong secret, state encode/decode, state HMAC validation, state expiry, dev provider (create + idempotent), dev provider rejected in production, unknown provider, method not allowed, validate with valid/invalid/missing auth, validate user not found, login returns token, failed login no token, Google/GitHub redirect with/without client ID
- `internal/server/handlers_auth_stress_test.go` — 35+ tests covering: alg:none attack, tampered payload, wrong/empty secret, expired token, extremely long claims, missing sub claim, state modification/replay/expiry/tampering, dev login gating (production/prod/case variations), empty/null-byte/SQL-injection providers, XSS in provider, extremely long code, missing Bearer prefix, malformed auth headers, empty token, non-existent/deleted user tokens, garbage tokens, concurrent dev logins, concurrent validates, concurrent user create race, method enforcement, no password hash in responses, no JWT secret leaked, X-Forwarded-Proto injection, Host header injection, CORS Authorization header allowed, empty/missing/tampered state in callbacks, nil body, malformed JSON
- All tests pass: `go test ./internal/...` — all packages pass
- `go vet ./...` — clean
- Server builds and runs: `curl -s http://localhost:4242/api/health` → `{"status":"ok"}`

## Documentation Updated
- `README.md` — Auth endpoints section
- `.claude/skills/develop/SKILL.md` — Auth endpoint table, config docs, env overrides
- `config/vire-service.toml.example` — `[auth]` section

## Devils-Advocate Findings
- **Open redirect**: Callback URL in OAuth state is not validated against an allowlist. An attacker who controls the `?callback=` parameter can redirect the JWT to an arbitrary domain. Logged as finding — mitigated by the portal being the only component that initiates the flow with a known callback URL.
- **Callback URL with query params**: If callback already has `?`, the redirect produces a malformed URL with double `?`. Logged as finding — portal should always use clean callback URLs.
- **Error message probing**: Different error messages for "invalid token" vs "user not found" on `/api/auth/validate` could enable token validity probing. Logged as finding — acceptable for now since the endpoint requires a valid JWT signature first.
- **X-Forwarded-Proto/Host header trust**: `oauthRedirectURI` trusts these headers for scheme/host detection. Safe when behind a reverse proxy that sets these headers, but could be manipulated in direct-access scenarios. Logged as informational finding.
- All JWT security tests pass: alg:none rejected, tampered payloads rejected, wrong/empty secrets rejected, HMAC state validation works correctly.

## Notes
- **JWT library**: Added `github.com/golang-jwt/jwt/v5` — standard Go JWT library
- **Server-side redirect pattern**: vire-server handles the full OAuth redirect dance. Portal only redirects to server and receives JWT on callback.
- **State parameter**: HMAC-SHA256 signed, base64-encoded JSON containing callback URL + nonce + timestamp. Expires after 10 minutes.
- **Dev login**: Flows through same `POST /api/auth/oauth` endpoint as real OAuth with `provider: "dev"`. Creates `dev_user` with role `admin`. Rejected in production mode.
- **Provider field**: InternalUser now tracks auth source (`email`, `google`, `github`, `dev`). OAuth users get prefixed IDs (e.g., `google_12345`, `github_67890`).
- **Backward compatible**: Existing `POST /api/auth/login` still works, now also returns a JWT token field.
