# Requirements: Job Resilience — Signal Compute and Stock Data Timeout Fixes

**Date:** 2026-02-27
**Requested:** Fix two feedback items: fb_778aa1d2 (compute_signals failing for missing market data) and fb_9054a751 (get_stock_data timeout for SGI.AU)

## Scope
- Fix silent success in `computeSignals` when market data is missing (executor.go:45-46)
- Add EOD data prerequisite check in watcher before enqueuing compute_signals jobs
- Add context timeout to `GetStockData` and `CollectMarketData` to prevent indefinite hangs
- Add per-operation timeout in the handleMarketStocks handler

## Out of Scope
- Job dependency framework (long-term solution)
- Circuit breaker pattern (future enhancement)
- Changes to job priority ordering

## Approach

### Bug 1: compute_signals failing (fb_778aa1d2)

**Root cause:** Two issues combine:
1. `executor.go:45-46` — `computeSignals()` returns `nil` (success) when market data is missing, causing the stock index `signals_collected_at` timestamp to be updated. The watcher sees fresh signals and never retries.
2. `watcher.go:146` — The watcher enqueues `compute_signals` without checking if EOD data has been collected. For new stocks, all jobs get `PriorityNewStock=15`, which is higher than `PriorityCollectEODBulk=10`. The signals job runs before EOD data exists.

**Fix:**
1. In `executor.go`: Return an error when market data is nil or has no EOD bars, so the job fails and is retried.
2. In `watcher.go:enqueueStaleJobs`: Skip `compute_signals` if `EODCollectedAt` is zero (no EOD data has ever been collected).

### Bug 2: get_stock_data timeout (fb_9054a751)

**Root cause:** `GetStockData` (service.go:456) uses the HTTP request context with no timeout. When market data is missing, it calls `CollectMarketData` which performs sequential API calls (EOD, fundamentals, filings, news) each with individual 30s timeouts but no aggregate timeout. For tickers that trigger multiple slow paths (e.g., filing PDF downloads at line 576, news intelligence generation at line 558), the total can exceed MCP tool timeout.

**Fix:**
1. In `handleMarketStocks` (handlers.go:579): Wrap `r.Context()` with a 90-second timeout before calling `GetStockData`.
2. In `GetStockData` (service.go:465): Wrap the `CollectMarketData` fallback call with a 60-second timeout so a single collection pass can't hang.

## Files Expected to Change
- `internal/services/jobmanager/executor.go` — Return error on missing market data
- `internal/services/jobmanager/watcher.go` — Skip signals when no EOD data
- `internal/server/handlers.go` — Add timeout to handleMarketStocks
- `internal/services/market/service.go` — Add timeout to CollectMarketData fallback in GetStockData
