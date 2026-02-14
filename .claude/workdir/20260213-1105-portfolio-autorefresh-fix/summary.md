# Summary: Fix portfolio auto-refresh to cover all callers

**Date:** 2026-02-13
**Status:** completed

## Root Cause

The auto-refresh added in commit ae22762 was in `handlePortfolioGet` (the HTTP handler for `GET /api/portfolios/{name}`). But `ReviewPortfolio` calls `s.GetPortfolio()` at the service level, completely bypassing the handler. When Claude Desktop called `portfolio_review`, it got stale cached data.

## What Changed

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Moved auto-refresh into `GetPortfolio` — checks `IsFresh`, calls `SyncPortfolio` if stale, falls back to stale data on sync failure |
| `internal/server/handlers.go` | Removed duplicate auto-refresh from `handlePortfolioGet` (now handled by service) |
| `internal/server/handlers_portfolio_test.go` | Simplified to 2 tests (returns portfolio, not found) — auto-refresh logic now tested at service level |
| `internal/services/portfolio/service_test.go` | Added 4 new tests: `TestGetPortfolio_Fresh_NoSync`, `TestGetPortfolio_Stale_TriggersSync`, `TestGetPortfolio_SyncFails_ReturnsStaleData`, `TestGetPortfolio_NotFound`. Fixed 3 existing review tests that had zero `LastSynced` (caused nil pointer panic with new auto-refresh). |

## Tests
- 4 new service-level auto-refresh tests — all pass
- 2 simplified handler tests — all pass
- 3 existing review tests fixed (added `LastSynced`) — all pass
- Full suite: all packages pass

## Notes
- `SyncPortfolio` already has its own internal freshness guard (line 55-61), so the double-check pattern is safe — no redundant Navexa API calls
- The fix means every path through `GetPortfolio` — `get_portfolio`, `portfolio_review`, `generate_report`, etc. — now auto-refreshes stale data
