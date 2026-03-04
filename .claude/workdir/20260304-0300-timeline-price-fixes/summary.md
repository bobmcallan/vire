# Summary: Timeline Rebuild Trigger, Visibility & EODHD Price Divergence Guard

**Status:** completed
**Feedback items:** fb_bf00d30f, fb_b7001891, fb_82594b9c

## Changes

| File | Change |
|------|--------|
| `internal/interfaces/services.go` | Added `IsTimelineRebuilding(name string) bool` to `PortfolioService` interface |
| `internal/models/portfolio.go` | Added `TimelineRebuilding bool` field to `Portfolio` struct |
| `internal/services/portfolio/service.go` | Added `timelineRebuilding sync.Map` to `Service` struct; added `IsTimelineRebuilding()` and `triggerTimelineRebuildAsync()` methods; modified `SyncPortfolio()` to call explicit rebuild on trade hash change; added EODHD >50% divergence guard in price cross-check; populated `TimelineRebuilding` in `GetPortfolio` |
| `internal/server/handlers.go` | Added `"rebuilding"` to portfolio status `timeline` section; added `"advisory"` to timeline response when rebuilding |
| `internal/server/handlers_portfolio_test.go` | Added `IsTimelineRebuilding` mock stub |
| `internal/services/report/devils_advocate_test.go` | Added `IsTimelineRebuilding` mock stub |
| `internal/services/cashflow/service_test.go` | Added `IsTimelineRebuilding` mock stub |
| `internal/services/portfolio/service_test.go` | Updated `TestSyncPortfolio_ZeroPriceEODHD` to reflect divergence guard behavior |

## Tests
- All portfolio unit tests pass (0.2s)
- All cashflow unit tests pass
- All report unit tests pass
- All server handler tests pass (excluding pre-existing timeout failures)
- `go vet ./...` clean
- `go build ./cmd/vire-server/` successful

## Pre-existing Failures (unchanged)
- `internal/app` — test infrastructure
- `internal/server` — TestRoleEscalation_* (Docker timeout)
- `internal/storage/surrealdb` — TestPurgeCharts

## How It Works

### fb_bf00d30f: Timeline Rebuild Trigger
When `SyncPortfolio` detects that `existingTradeHash != tradeHash`:
1. Deletes all timeline snapshots (existing behavior)
2. Saves the updated portfolio record
3. **NEW**: Calls `triggerTimelineRebuildAsync(ctx, name)` which spawns a goroutine to:
   - Set `rebuilding[name] = true`
   - Call `GetDailyGrowth` to recompute the full timeline (5-min timeout)
   - Clear `rebuilding[name] = false` on completion or failure

### fb_b7001891: Timeline Rebuild Visibility
- `portfolio_get_status` response includes `timeline.rebuilding: true/false`
- `portfolio_get` response includes `timeline_rebuilding: true/false`
- `portfolio_get_timeline` response includes `advisory: "Timeline is being rebuilt..."` when rebuilding

### fb_82594b9c: EODHD Price Divergence Guard
In the EODHD price cross-check loop, before accepting an EODHD price:
- Computes `divergencePct = |eodhd - navexa| / navexa × 100`
- If `divergencePct > 50%`, rejects the EODHD price and logs a warning
- Keeps Navexa's price as the authoritative value
- Prevents wrong-instrument mapping from corrupting portfolio valuations (e.g., ACDC.AU)
