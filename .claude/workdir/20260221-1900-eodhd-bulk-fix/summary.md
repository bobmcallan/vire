# Summary: Fix EODHD Bulk Download and Batch EOD Collection

**Date:** 2026-02-21
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/clients/eodhd/client.go` | Added `flexFloat64`/`flexInt64` types with custom JSON unmarshaling for string/number values; NaN/Inf guard; null handling; updated `bulkEODResponse` struct and `GetBulkEOD` |
| `internal/models/jobs.go` | Added `JobTypeCollectEODBulk = "collect_eod_bulk"`, `PriorityCollectEODBulk = 10`, updated `DefaultPriority()` and `TimestampFieldForJobType()` |
| `internal/interfaces/services.go` | Added `CollectBulkEOD(ctx, exchange, force) error` to `MarketService` interface |
| `internal/services/market/collect.go` | New `CollectBulkEOD` method — lists tickers by exchange, calls bulk API, merges bars, falls back to individual fetch for new tickers, updates stock index timestamps per-ticker |
| `internal/services/jobmanager/executor.go` | Added `JobTypeCollectEODBulk` dispatch to `CollectBulkEOD` |
| `internal/services/jobmanager/watcher.go` | Changed EOD job creation from per-ticker to per-exchange bulk; added `eohdExchangeFromTicker` helper |
| `internal/services/market/service_test.go` | 5 new unit tests for `CollectBulkEOD` (merge, fallback, fresh skip, exchange filter, nil client) |
| `internal/services/jobmanager/manager_test.go` | Updated mock, tests for bulk EOD job type |
| `internal/services/jobmanager/devils_advocate_test.go` | Updated to expect `collect_eod_bulk` instead of `collect_eod` |
| `internal/services/report/devils_advocate_test.go` | Added `CollectBulkEOD` stub to mock |
| `README.md` | Updated job types from 8 to 9, added `collect_eod_bulk`, documented exchange-level grouping |
| `.claude/skills/develop/SKILL.md` | Updated job types table, MarketService methods, watcher/executor descriptions |

## Tests
- 5 new unit tests for `CollectBulkEOD` (merge, fallback, fresh skip, exchange filter, nil client)
- `flexFloat64`/`flexInt64` unmarshal tests (numbers, strings, empty, null, NaN, Inf)
- 30 stress tests from devils-advocate (NaN/Inf, malformed strings, edge cases)
- 19 data layer tests passed (2.784s) including 7 new `TestCollectBulkEOD_*`
- 19 API integration tests passed (125.246s)
- All unit tests pass (`go test ./internal/...`)
- `go vet ./...` clean
- Build succeeds
- Test feedback rounds: 0 (all passed first run)

## Documentation Updated
- `README.md` — new job type, exchange-level grouping
- `.claude/skills/develop/SKILL.md` — job types, MarketService methods, watcher behavior

## Devils-Advocate Findings
- **NaN/Infinity vulnerability**: `flexFloat64.UnmarshalJSON` could pass NaN/Inf from `strconv.ParseFloat` — fixed with `math.IsNaN`/`math.IsInf` guard coercing to 0
- **flexInt64 null handling**: `null` JSON value caused error — fixed with explicit null check
- **Exchange code mismatch**: ASX vs AU in stock index filtering — fixed to use ticker suffix
- **Minor**: No ctx.Err() check in per-ticker loop, duplicate tickers not deduped in API request (noted, not blocking)

## Key Fixes

### 1. Bulk API JSON Deserialization (Root Cause)
The EODHD bulk API `/eod-bulk-last-day/AU` returns price fields as strings for the AU exchange (e.g., `"open": "1.23"`), but the `bulkEODResponse` struct used `float64`. Added `flexFloat64` and `flexInt64` custom types that handle both JSON numbers and strings transparently.

### 2. Batch EOD Collection in Job Manager
The watcher now groups stale-EOD tickers by exchange and enqueues one `collect_eod_bulk` job per exchange instead of N individual `collect_eod` jobs. The executor dispatches to `CollectBulkEOD` which makes a single bulk API call per exchange, then merges bars into each ticker's market data. New tickers (no existing EOD history) fall back to individual `CollectEOD` for full 3-year fetch.

### 3. NaN/Infinity Guard
`flexFloat64.UnmarshalJSON` rejects NaN and Infinity values from `strconv.ParseFloat`, coercing them to 0 to prevent downstream computation corruption.

## Notes
- `golangci-lint` couldn't run (built with Go 1.24, project uses Go 1.25)
- Individual `CollectEOD` still works for admin manual enqueue
- `CollectCoreMarketData` (report fast path) still uses `GetBulkEOD` directly — unaffected by job manager changes
- Stock index timestamps for bulk jobs are updated per-ticker inside `CollectBulkEOD`, not by the job manager's `updateStockIndexTimestamp`
