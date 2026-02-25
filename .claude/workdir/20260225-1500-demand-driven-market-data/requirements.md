# Requirements: Demand-Driven Background Market Data Collection

**Date:** 2026-02-25
**Requested:** Test and verify the demand-driven market data collection feature

## Scope
- Test `EnqueueIfNeeded` (renamed from `enqueueIfNeeded`) in `internal/services/jobmanager/queue.go`
- Test `EnqueueTickerJobs` and `EnqueueSlowDataJobs` new methods in `internal/services/jobmanager/watcher.go`
- Test `handlePortfolioGet` demand-driven background enqueue in `internal/server/handlers.go`
- Test `handlePortfolioReview` demand-driven background enqueue in `internal/server/handlers.go`
- Test `handleMarketStocks` force_refresh support in `internal/server/handlers.go`
- Verify MCP catalog update for `get_stock_data` force_refresh param

## What Was Built

### Files Changed
- `internal/services/jobmanager/queue.go` — `enqueueIfNeeded` renamed to `EnqueueIfNeeded` (public export)
- `internal/services/jobmanager/watcher.go` — Updated callers; added `EnqueueTickerJobs` (demand-driven, freshness-respecting) and `EnqueueSlowDataJobs` (force, bypasses freshness)
- `internal/server/handlers.go` — `handlePortfolioGet`: fire-and-forget `EnqueueTickerJobs` after response; `handlePortfolioReview`: hoisted tickers var + fire-and-forget; `handleMarketStocks`: added `force_refresh` support with inline core data refresh + background slow data enqueue + advisory response
- `internal/server/catalog.go` — Added `force_refresh` param to `get_stock_data` tool definition
- `internal/services/jobmanager/manager_test.go` — Updated refs to `EnqueueIfNeeded`
- `internal/services/jobmanager/devils_advocate_test.go` — Updated refs to `EnqueueIfNeeded`

### Key Behaviors to Test
1. `EnqueueTickerJobs` respects freshness TTLs (only enqueues stale components)
2. `EnqueueTickerJobs` deduplicates via `HasPendingJob`
3. `EnqueueTickerJobs` groups stale EOD by exchange for bulk jobs
4. `EnqueueSlowDataJobs` bypasses freshness, enqueues all slow job types
5. `EnqueueSlowDataJobs` still deduplicates via `HasPendingJob`
6. `handlePortfolioGet` fires background enqueue after response write
7. `handlePortfolioReview` fires background enqueue after response write
8. `handleMarketStocks` with force_refresh=true refreshes core data inline, enqueues slow jobs, returns advisory
9. `handleMarketStocks` without force_refresh returns data as before (no behavior change)

## Approach
- Unit tests for new JobManager methods (existing test infrastructure in jobmanager package)
- Integration tests via containerized test environment for server handlers
- Verify all existing tests still pass
