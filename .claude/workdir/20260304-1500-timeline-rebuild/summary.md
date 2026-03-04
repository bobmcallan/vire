# Summary: Portfolio Timeline Cash Integration & Force Rebuild

**Status:** completed

## Feedback Items Addressed
- fb_25304b2f (HIGH): Timeline portfolio_value == equity_value — **FIXED**
- fb_22ce519f (HIGH): Force rebuild + cash transaction invalidation — **FIXED**
- fb_bf00d30f (ACKNOWLEDGED): Timeline not recalculating after cash changes — **FIXED**

## Root Cause
Three internal callers (`triggerTimelineRebuildAsync`, `backfillTimelineIfEmpty`, `onLedgerChange` callback) invoked `GetDailyGrowth` with empty `GrowthOptions{}` — no cash transactions loaded. This caused `hasCashTxs=false`, setting `portfolioVal = totalValue` (equity only). Wrong snapshots got persisted and served from cache.

## Changes
| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | New `rebuildTimelineWithCash()` helper; dedup in `triggerTimelineRebuildAsync()`; new `InvalidateAndRebuildTimeline()` and `ForceRebuildTimeline()` methods; backfill dedup + cash fix |
| `internal/interfaces/services.go` | Added `InvalidateAndRebuildTimeline` and `ForceRebuildTimeline` to PortfolioService interface |
| `internal/app/app.go` | Simplified onLedgerChange callback from 13 lines to 2 (delegates to InvalidateAndRebuildTimeline) |
| `internal/server/handlers_admin.go` | New `handleAdminRebuildTimeline` handler + `handleAdminPortfolioRoutes` dispatcher |
| `internal/server/routes.go` | New route: `POST /api/admin/portfolios/{name}/rebuild-timeline` |
| `internal/server/catalog.go` | New MCP tool: `admin_rebuild_timeline` |
| `internal/server/handlers_portfolio_test.go` | Mock stubs for 2 new interface methods |
| `internal/services/cashflow/service_test.go` | Mock stubs for 2 new interface methods |
| `internal/services/report/devils_advocate_test.go` | Mock stubs for 2 new interface methods |

## Tests
- Unit tests: 8 in `internal/services/portfolio/timeline_rebuild_test.go`
- Integration tests: 10 in `tests/data/timeline_rebuild_integration_test.go`
- Total: 25/25 timeline-specific tests PASSING
- No regressions: all existing tests pass
- Fix rounds: 0

## Architecture
- Architect review: APPROVED (10/10 alignment)
- Separation of concerns: all timeline store access encapsulated in PortfolioService
- app.go no longer accesses TimelineStore directly
- No legacy compatibility shims

## Devils-Advocate
- 3/3 prior findings resolved (race condition, TOCTOU dedup, missing mocks)
- TOCTOU in dedup assessed as acceptable risk (idempotent operations, nanosecond window)
- 2 informational findings (empty portfolio name, force rebuild self-race) — no action needed

## Notes
- Stock timeline (GetStockTimeline) remains on-demand — no changes needed
- Incremental portfolio timeline updates deferred (optimization for later)
- Company timeline (collect_timeline job) force rebuild is a separate feature
