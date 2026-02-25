# Summary: Demand-Driven Background Market Data Collection

**Date:** 2026-02-25
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/services/jobmanager/queue.go` | Renamed `enqueueIfNeeded` → `EnqueueIfNeeded` (public export) |
| `internal/services/jobmanager/watcher.go` | Updated callers to `EnqueueIfNeeded`; added `EnqueueTickerJobs` (demand-driven, freshness-respecting) and `EnqueueSlowDataJobs` (force mode, bypasses freshness, empty ticker guard) |
| `internal/server/handlers.go` | `handlePortfolioGet`: fire-and-forget `EnqueueTickerJobs` with panic recovery after response; `handlePortfolioReview`: same pattern, hoisted tickers var; `handleMarketStocks`: added `force_refresh` support — inline core data refresh + background slow data enqueue + consistent `{data, advisory}` response envelope |
| `internal/server/catalog.go` | Added `force_refresh` boolean param to `get_stock_data` tool definition |
| `internal/services/jobmanager/manager_test.go` | Updated refs to `EnqueueIfNeeded`; added 14 unit tests for new methods |
| `internal/services/jobmanager/devils_advocate_test.go` | Updated refs; added 22 stress tests (DA-27 through DA-48) |
| `tests/api/demand_driven_test.go` | New: 4 API integration tests for force_refresh and demand-driven enqueue |
| `.claude/skills/develop/SKILL.md` | Updated Job Manager, MarketService, and GetStockData documentation sections |

## Tests

- **Unit tests added**: 14 new tests in `manager_test.go` (EnqueueTickerJobs: stale data, fresh data, missing tickers, bulk EOD grouping, multi-exchange, dedup, empty/nil slices, partial staleness, exchange extraction; EnqueueSlowDataJobs: all types, dedup, priorities)
- **Stress tests added**: 22 new tests in `devils_advocate_test.go` (DA-27 to DA-48: nil/empty inputs, hostile tickers, concurrent access, TOCTOU race, large ticker lists, cancelled context, response schema, goroutine safety)
- **API integration tests added**: 4 new tests in `demand_driven_test.go` (force_refresh with/without flag, blank config graceful handling, demand-driven enqueue doesn't affect response)
- **All 66 tests pass**: 55 jobmanager + 4 API integration + 7 catalog
- **Test feedback rounds**: 0 (all tests passed on first run)

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — Job Manager section (EnqueueTickerJobs, EnqueueSlowDataJobs), MarketService section (force_refresh), GetStockData caching note

## Devils-Advocate Findings

| # | Finding | Resolution |
|---|---------|------------|
| DA-39 | Fire-and-forget goroutines have no recover() | **Fixed**: wrapped with `defer func() { recover() }()` |
| DA-35 | EnqueueSlowDataJobs accepts empty ticker | **Fixed**: added early return for empty ticker |
| DA-40 | Response schema inconsistency | **Fixed**: force_refresh always returns `{data, advisory}` envelope (advisory is null when no jobs) |
| DA-37 | EnqueueIfNeeded count inflation (dedup counted as success) | Accepted: benign, count is for logging/advisory only |
| DA-38 | TOCTOU race in concurrent dedup | Accepted: known limitation, duplicates are harmless (executor handles gracefully) |
| DA-45 | Fire-and-forget goroutines not tracked for graceful shutdown | Accepted: lightweight operations, acceptable trade-off |

## Notes
- Pre-existing server tests that require local SurrealDB or real API keys continue to fail as before — not related to this change
- The demand-driven pattern is best-effort: failures in background job enqueue don't affect the user-facing response
- `get_portfolio force_refresh=true` only forces Navexa portfolio sync, never forces market data — by design
- `get_stock_data force_refresh=true` is the only way to force-rebuild individual stock data
