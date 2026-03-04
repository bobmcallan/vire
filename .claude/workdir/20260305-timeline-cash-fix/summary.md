# Summary: Timeline Cash Auto-Loading Fix

**Status:** completed

## Root Cause

`GetDailyGrowth()` required callers to explicitly pass cash transactions via `opts.Transactions`. The timeline scheduler (`timeline_scheduler.go:63`) ran every 12h and on startup, calling `GetDailyGrowth` with empty options — persisting timeline snapshots with `portfolio_value = equity_value` (no cash). Even after `ForceRebuildTimeline`, the scheduler would overwrite correct data with cashless data.

## Fix

Added 9 lines to `GetDailyGrowth()` that auto-load cash transactions from `cashflowSvc` when `opts.Transactions` is nil. Removed redundant manual injection from all callers.

## Changes

| File | Change |
|------|--------|
| `internal/services/portfolio/growth.go` | Added auto-load of cash transactions in GetDailyGrowth (lines 110-117) |
| `internal/server/handlers.go` | Removed redundant cash injection from handlePortfolioHistory and handlePortfolioReview |
| `internal/services/portfolio/indicators.go` | Removed redundant cash injection from GetPortfolioIndicators |
| `internal/services/portfolio/service.go` | Simplified rebuildTimelineWithCash to delegate to GetDailyGrowth |
| `internal/services/portfolio/capital_timeline_test.go` | Added 3 unit tests: auto-load, explicit override, no cashflow service |
| `internal/services/portfolio/timeline_rebuild_test.go` | Updated tests to match new auto-load behavior |
| `docs/architecture/26-02-27-services.md` | Updated documentation |

## Tests

- Unit tests: 3 new (auto-load, explicit override, graceful degradation) + existing updated
- Stress tests: 12 (by devils-advocate) — all pass
- Integration tests: created in tests/data/
- All internal tests pass (pre-existing TestPurgeCharts failure unrelated)
- Build: OK | Vet: OK

## Architecture

- Approved by architect — separation of concerns enforced
- Cash injection centralized in PortfolioService (growth.go), not duplicated in consumers
- Backward compatible: explicit `opts.Transactions` still works as override

## Devils-Advocate

- Race conditions: thread-safe (GetLedger is read-only)
- Nil vs empty slice semantics: correct (nil triggers auto-load, empty slice does not)
- No new attack surface
