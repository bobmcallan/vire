# Requirements: Filings Refactor

**Date:** 2026-02-25
**Requested:** User requested implementation of the plan from `docs/features/20260225-fillings-refactor.md` with these notes:
- Backwards compatibility is NOT required,- Redundant functions/code should be removed

## Scope
- Split `CollectFilings` into fast (`CollectFilingsIndex`) and slow (`CollectFilingPdfs`) methods
- Add filing index to fast path (`CollectCoreMarketData`)
- Update job types, executor, watcher, manager
- Update tests
- Remove backward compatibility code

- Update documentation

## Approach

Since user noted that backwards compatibility is NOT required, I will:
- Remove the old wrapper `CollectFilings` that calls both index + PDF methods
- Only expose `CollectFilingsIndex` and `CollectFilingPdfs` publicly
- Remove `FilingsUpdatedAt` legacy field from MarketData (use only `FilingsIndexUpdatedAt` and `FilingsPdfsUpdatedAt`
- Update test mocks to handle new methods and timestamp field

## Files Expected to Change

- `internal/services/market/collect.go` - Split methods, remove wrapper
- `internal/services/market/service.go` - Add filing index to fast path
- `internal/models/jobs.go` - New job type, update constants
- `internal/models/market.go` - Remove legacy timestamp,- `internal/services/jobmanager/executor.go` - Handle new job type
- `internal/services/jobmanager/watcher.go` - Update EnqueueSlowDataJobs
- `internal/services/jobmanager/manager.go` - Update isHeavyJob
- `internal/storage/surrealdb/stockindex.go` - Already has new timestamp field
- `internal/interfaces/services.go` - Update interface
- `internal/services/jobmanager/manager_test.go` - Update mocks
- `docs/features/20260225-fillings-refactor.md` - Update plan document

