# Summary: Include Closed Positions in Portfolio Get

**Status:** completed

## Changes
| File | Change |
|------|--------|
| `internal/server/handlers.go` | Added `include_closed` query param parsing + holdings filtering (default: exclude closed) |
| `internal/services/portfolio/service.go` | `assembleManualPortfolio` now includes closed positions (status="closed") without affecting aggregates |
| `internal/server/catalog.go` | Added `include_closed` param to `portfolio_get` MCP tool + updated description |
| `internal/server/handlers_portfolio_test.go` | Added 2 unit tests: ExcludesClosedByDefault, IncludesClosedWhenRequested |
| `tests/data/portfolio_closed_positions_test.go` | Added 2 integration tests: IncludesClosedPositions, ClosedPositionHasRealizedReturn |

## Tests
- 2 unit tests added (handlers_portfolio_test.go)
- 2 integration tests created (portfolio_closed_positions_test.go)
- Full suite: ALL PASS (internal 96s, data 12.8s, API 3.5s)
- Fix rounds: 1 (missing catalog param — caught by devils-advocate + reviewer, fixed by implementer)

## Architecture
- Architect APPROVED: all 7 checks verified
- Filtering in handler layer (presentation concern), aggregates in service layer (business logic)
- No new dependencies, interfaces, or stores

## Devils-Advocate
- 1 bug found: missing catalog param (fixed)
- 9 adversarial scenarios passed: input validation, empty/all-closed, job enqueue, race conditions

## Notes
- Navexa portfolios already stored closed positions — this change only affects filtering at API level
- Manual portfolios now also store closed positions in the holdings array
- Default behavior changed: API previously returned closed positions for Navexa portfolios, now excludes them by default
