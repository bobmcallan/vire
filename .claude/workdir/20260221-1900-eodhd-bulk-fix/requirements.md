# Requirements: Fix EODHD Bulk Download and Batch EOD Collection

**Date:** 2026-02-21
**Requested:** Review container 0911a2dd... logs. There is an issue with EODHD bulk download.

## Scope

### Issue 1: Bulk API JSON Deserialization (Required Fix)
The EODHD bulk API (`/eod-bulk-last-day/AU`) returns price fields as strings for the AU exchange, but `bulkEODResponse` in `internal/clients/eodhd/client.go:326-336` declares them as `float64`. This causes:
```
json: cannot unmarshal string into Go struct field bulkEODResponse.open of type float64
```
The US exchange bulk API returns numbers, so it works fine. The AU exchange returns strings like `"1.23"`.

### Issue 2: Job Manager Batch EOD (Optimization)
The job manager's watcher creates N individual `collect_eod` jobs (one per ticker), and the executor calls `CollectEOD(ctx, ticker, false)` which makes individual `/eod/TICKER` API calls. This defeats the purpose of the bulk API. The watcher should create one bulk EOD job per exchange, and the executor should use the bulk API.

## Approach

### Fix 1: FlexFloat64 type for EODHD bulk response
- Add a `FlexFloat64` type in `internal/clients/eodhd/client.go` with custom `json.Unmarshaler`
- Handles both `1.23` (number) and `"1.23"` (string) JSON values
- Also add `FlexInt64` for volume (defensive — may also arrive as string)
- Update `bulkEODResponse` struct to use these types
- Update `GetBulkEOD` to convert flex types to standard types when building `models.EODBar`

### Fix 2: Batch EOD collection in job manager
- Add `JobTypeCollectEODBulk = "collect_eod_bulk"` to `models/jobs.go`
- Add `CollectBulkEOD(ctx, exchange, force) error` to `MarketService` interface and implementation
- Implementation: list tickers from stock index for the exchange, call `GetBulkEOD`, merge bars into each ticker's market data, compute signals for changed tickers, update stock index timestamps per-ticker
- New tickers (no existing EOD data) fall back to individual `CollectEOD` for full 3-year history
- In watcher: group stale-EOD tickers by exchange, enqueue one `collect_eod_bulk` per exchange instead of per-ticker `collect_eod` jobs
- In executor: dispatch `collect_eod_bulk` to `CollectBulkEOD(ctx, job.Ticker, false)` where `job.Ticker` holds the exchange code
- `updateStockIndexTimestamp` skips bulk jobs — `CollectBulkEOD` updates per-ticker timestamps internally

## Files Expected to Change
- `internal/clients/eodhd/client.go` — FlexFloat64/FlexInt64 types, update bulkEODResponse
- `internal/models/jobs.go` — new job type, priority, mapping functions
- `internal/interfaces/services.go` — add CollectBulkEOD to MarketService
- `internal/services/market/collect.go` — CollectBulkEOD implementation
- `internal/services/jobmanager/executor.go` — dispatch collect_eod_bulk
- `internal/services/jobmanager/watcher.go` — batch EOD job creation
