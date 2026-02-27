# Summary: MCP Session Persistence API

**Status:** completed
**Feedback:** fb_c4a661a8, fb_00e43378
**Duration:** ~18 minutes

## Problem
vire-portal stores all OAuth state (sessions, auth codes, refresh tokens, client registrations) in-memory. Every Fly.io deployment wipes this state, breaking all active MCP client sessions with 404 "Link not found".

## Solution
Added internal REST API endpoints on vire-server backed by SurrealDB, enabling the portal to persist OAuth state across restarts.

## Changes
| File | Change |
|------|--------|
| `internal/models/oauth.go` | Added `OAuthSession` struct, extended `OAuthClient` with `GrantTypes`, `ResponseTypes`, `TokenEndpointAuthMethod` |
| `internal/interfaces/storage.go` | Added 6 session methods to `OAuthStore` interface |
| `internal/storage/surrealdb/manager.go` | Added `mcp_auth_session` table |
| `internal/storage/surrealdb/oauthstore.go` | Implemented session CRUD + client field extensions |
| `internal/server/oauth_internal.go` | **NEW** — 17 REST endpoints for sessions, clients, codes, tokens |
| `internal/server/routes.go` | Registered `/api/internal/oauth/` route |
| `internal/server/handlers_oauth_test.go` | Updated mock store with session methods |
| `internal/storage/surrealdb/testhelper_test.go` | Added `mcp_auth_session` table to test setup |
| `docs/architecture/api.md` | Documented internal OAuth API endpoints |
| `docs/architecture/auth.md` | Added MCP session persistence section |

## Tests
- **Unit tests**: 28 handler tests in `internal/server/oauth_internal_test.go` — ALL PASS
- **Store tests**: 12 SurrealDB tests in `internal/storage/surrealdb/oauthstore_session_test.go`
- **Integration tests**: 10 tests in `tests/api/oauth_internal_test.go` — 10/10 PASS
- **Fix rounds**: 1 (status code 200→201 in client tests)

## Architecture
- Docs updated by architect: `api.md`, `auth.md`, `storage.md`
- Internal API follows existing REST patterns (WriteJSON, WriteError, RequireMethod)
- Token hashing: plaintext in → SHA-256 stored (uses existing `hashRefreshToken()`)
- Session TTL: 10 minutes (enforced at query time)
- No auth on internal endpoints (internal network only)

## Reviews
- **Code quality** (reviewer): A+ across all 3 layers
- **Architecture** (architect): 2 issues found and fixed (duplicate hash fn, session purge route)
- **Security** (devils-advocate): 2 issues found and fixed (unrouted purge, status mismatch)

## Follow-up
- **vire-portal**: Replace in-memory `SessionStore`, `CodeStore`, `TokenStore`, `ClientStore` with HTTP client calls to these new endpoints
- **Auth gating**: Optional shared API key for internal endpoints
