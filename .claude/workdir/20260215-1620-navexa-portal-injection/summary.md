# Summary: Navexa API Key and User ID Portal Injection

**Date:** 2026-02-15
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/common/config.go` | Removed `APIKey` field from `NavexaConfig` |
| `internal/common/userctx.go` | Added `UserID` field to `UserContext` |
| `internal/common/userctx_test.go` | Added test for UserID in context |
| `internal/server/middleware.go` | Extract `X-Vire-User-ID` header, added to CORS allowed headers |
| `internal/server/middleware_test.go` | Tests for user ID extraction in middleware |
| `internal/server/handlers.go` | Added `requireNavexaContext` validation with whitespace trimming |
| `internal/server/handlers_portfolio_test.go` | Tests for handler validation (missing user ID, missing key, both present) |
| `internal/server/routes.go` | Minor route adjustment |
| `internal/app/app.go` | Removed navexa key resolution, removed default client, updated `InjectNavexaClient` |
| `internal/services/portfolio/service.go` | Removed fallback to default navexa client, returns error when no context client |
| `internal/services/portfolio/service_test.go` | Updated tests for new client resolution behavior |
| `internal/services/portfolio/currency_test.go` | Updated tests |
| `internal/services/portfolio/returns_refactor_test.go` | Updated tests |
| `tests/docker/vire-blank.toml` | Removed `api_key` from navexa config |
| `docs/storage-separation.md` | Minor update |
| `README.md` | Documented portal injection requirements |

## Tests
- Added middleware tests for `X-Vire-User-ID` header extraction
- Added handler tests for `requireNavexaContext` validation (missing user ID, missing key, whitespace-only values)
- Added portfolio service tests for error when no context client
- Updated existing tests to work with new client resolution
- All tests pass (`go test ./...`)

## Documentation Updated
- `README.md` — documented that Navexa API key and user ID are injected by portal, not stored in backend

## Devils-Advocate Findings
- **Whitespace bypass:** Identified that whitespace-only `X-Vire-User-ID` or `X-Vire-Navexa-Key` could bypass validation. Fixed with `strings.TrimSpace` in `requireNavexaContext`.

## Notes
- 18 files changed, 436 insertions, 173 deletions
- The `[clients.navexa]` config section retains `base_url`, `rate_limit`, and `timeout` — only `api_key` was removed
- Full multi-tenant storage (keying data by user ID) is future work — this change establishes the user ID plumbing
