# Summary: Portfolio Report Split, EODHD Batch Optimization, and Job Manager

**Date:** 2026-02-21
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/services/market/service.go` | Added `CollectCoreMarketData` — bulk EOD + concurrent fundamentals (no filings/news/AI) |
| `internal/services/market/filings.go` | Added `recover()` in `extractPDFText` for zlib panic from corrupt PDFs |
| `internal/services/market/service_test.go` | Added tests for `CollectCoreMarketData` and batch EOD grouping |
| `internal/services/report/service.go` | Refactored `GenerateReport` to use `CollectCoreMarketData` fast path, removed signal detection step |
| `internal/services/jobmanager/manager.go` | **New** — Job manager with Start/Stop lifecycle, 1-minute tick cycle |
| `internal/services/jobmanager/jobs.go` | **New** — Cycle execution: core data refresh + detailed data collection + signal refresh |
| `internal/services/jobmanager/manager_test.go` | **New** — 12 tests including lifecycle, cycle execution, error handling |
| `internal/services/jobmanager/stress_test.go` | **New** — 23 stress tests for concurrency, edge cases, resource leaks |
| `internal/app/app.go` | Added `JobManager` field, `StartJobManager()`, wired into `Close()` |
| `internal/app/scheduler.go` | Kept but superseded by job manager |
| `internal/app/warmcache.go` | Kept but superseded by job manager |
| `internal/common/config.go` | Added `JobManagerConfig` struct (enabled, interval, max_concurrent) |
| `internal/interfaces/services.go` | Added `CollectCoreMarketData` to `MarketService` interface |
| `internal/server/handlers.go` | Added `handleJobStatus` GET handler |
| `internal/server/routes.go` | Registered `GET /api/jobs/status` |
| `internal/storage/surrealdb/manager.go` | Added `job_runs` table creation |
| `config/vire-service.toml` | Added `[jobmanager]` config section |
| `cmd/vire-server/main.go` | Replaced `StartWarmCache` + `StartPriceScheduler` with `StartJobManager` |
| `README.md` | Updated report description |
| `.claude/skills/develop/SKILL.md` | Added Job Manager to Key Directories, report pipeline docs |

## Tests
- 12 job manager unit tests — all passing
- 23 stress tests (concurrency, edge cases, resource leaks) — all passing
- 39 market service tests — all passing
- `go build ./...` — clean
- `go vet ./...` — clean

## Documentation Updated
- `README.md` — report description updated
- `.claude/skills/develop/SKILL.md` — Key Directories, Job Manager section, Report Pipeline section

## Devils-Advocate Findings
1. **Double-Start goroutine leak** — Fixed: `Start()` now stops existing loop before starting new one
2. **Semaphore blocking without context** — Fixed: added `select` with `ctx.Done()` in semaphore acquire
3. **Error swallowing in CollectCoreMarketData** — Fixed: returns `errors.Join(errs...)`
4. **Long cycle warning missing** — Fixed: logs warning when cycle duration exceeds configured interval
5. **Concurrent map write in test mock** — Fixed: added mutex to `mockMarketDataStorage`

## Notes
- **Deployment**: Old root-owned vire-server (PID 211) holds port 8882, preventing restart. Requires `sudo` or container restart to deploy.
- **Report speed**: `GenerateReport` now skips filings, news, AI summaries, and signal detection. Should return in seconds instead of minutes.
- **Job manager**: Replaces both `StartWarmCache` and `StartPriceScheduler`. Runs every 1 minute with configurable `max_concurrent` (default 5) for parallel EODHD calls.
- **Container bug fix**: The `zlib: invalid header` panic from corrupt PDFs is now caught by `recover()` — the corrupt PDF is skipped and processing continues.
- **Context cancellation**: No longer an issue for background work — the job manager uses `context.Background()`, not HTTP request context.
