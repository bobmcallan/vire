# Summary: Separate Filings Collection into Fast (Index) and Slow (PDFs + AI)

**Date:** 2026-02-25
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/market.go` | Removed legacy `FilingsUpdatedAt` field, added `FilingsIndexUpdatedAt` and `FilingsPdfsUpdatedAt` |
| `internal/models/jobs.go` | Added `FilingsPdfsCollectedAt` timestamp field to `StockIndexEntry` |
| `internal/services/market/collect.go` | Removed `CollectFilings` backward-compatible wrapper; only `CollectFilingsIndex` and `CollectFilingPdfs` remain |
| `internal/services/market/service.go` | Updated to use `FilingsIndexUpdatedAt` instead of legacy field |
| `internal/services/jobmanager/executor.go` | Updated `JobTypeCollectFilings` to call `CollectFilingsIndex` (fast path) |
| `internal/services/jobmanager/manager.go` | Updated `isHeavyJob` to return `true` for `collect_filing_pdfs` and `collect_filing_summaries`, but NOT `collect_filings` |
| `internal/services/jobmanager/watcher.go` | Updated `EnqueueSlowDataJobs` to use `collect_filing_pdfs` instead of `collect_filings` |
| `internal/storage/surrealdb/stockindex.go` | Added `filings_pdfs_collected_at` to valid timestamp fields |
| `internal/interfaces/services.go` | Removed `CollectFilings` from interface, keeping only `CollectFilingsIndex` and `CollectFilingPdfs` |
| `internal/services/jobmanager/*_test.go` | Updated all tests to account for new job type and timestamps |
| `internal/services/market/*_test.go` | Updated tests to use `FilingsIndexUpdatedAt` instead of legacy field |

## Tests
- All unit tests pass: `go test ./internal/services/jobmanager/... ./internal/services/market/...`
- Updated `isHeavyJob` test to reflect `collect_filings` is NOT heavy anymore
- Updated stale/fresh data tests to account for 9 job types instead of 8 (added filing_pdfs)
- Updated `TestCollectCoreMarketData_CollectsFilingsIndex` to verify filing index is collected in fast path

## Documentation Updated
- `docs/features/20260225-fillings-refactor.md` - Cleaned up from external table format

## Notes
- Backward compatibility was NOT required per user request
- The `CollectFilings` wrapper method was removed - callers must use `CollectFilingsIndex` or `CollectFilingPdfs` directly
- The legacy `FilingsUpdatedAt` field was removed from `MarketData`
- Job type `collect_filings` now only collects the HTML index (fast ~1s)
- Job type `collect_filing_pdfs` downloads PDFs (slow ~5s+)
- `collect_filing_pdfs` is now rate-limited by the heavy job semaphore (OOM prevention)
