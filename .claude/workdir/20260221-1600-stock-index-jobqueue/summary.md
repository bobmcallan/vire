# Summary: Stock Index Table & Priority Job Queue

**Date:** 2026-02-21
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/jobs.go` | **New** — StockIndexEntry, Job, JobEvent models with constants and priorities |
| `internal/interfaces/storage.go` | Added StockIndexStore and JobQueueStore interfaces to StorageManager |
| `internal/storage/surrealdb/stockindex.go` | **New** — SurrealDB stock index implementation (Upsert, Get, List, UpdateTimestamp, Delete) |
| `internal/storage/surrealdb/jobqueue.go` | **New** — SurrealDB job queue with atomic Dequeue, priority ordering, dedup |
| `internal/storage/surrealdb/manager.go` | Added stock_index and job_queue table creation, new store accessors |
| `internal/services/market/collect.go` | **New** — 7 individual collection methods (CollectEOD, CollectFundamentals, CollectFilings, CollectNews, CollectFilingSummaries, CollectTimeline, CollectNewsIntelligence) |
| `internal/interfaces/services.go` | Added 7 individual collection methods to MarketService interface |
| `internal/services/jobmanager/manager.go` | **Rewritten** — Queue-based with watcher loop + processor pool + WebSocket hub |
| `internal/services/jobmanager/watcher.go` | **New** — Scans stock index for stale data, auto-enqueues collection jobs |
| `internal/services/jobmanager/executor.go` | **New** — Dispatches jobs to market service methods, updates stock index timestamps |
| `internal/services/jobmanager/queue.go` | **New** — In-process queue wrapper with WebSocket event broadcasting |
| `internal/services/jobmanager/websocket.go` | **New** — WebSocket hub for real-time job events (admin only) |
| `internal/services/jobmanager/jobs.go` | **Rewritten** — Legacy LastJobRun compat only |
| `internal/services/portfolio/service.go` | Upserts tickers to stock index during SyncPortfolio |
| `internal/server/handlers_admin.go` | **New** — 7 admin handlers (jobs list, queue, priority, cancel, enqueue, stock-index, WebSocket) |
| `internal/server/routes.go` | Registered admin routes under /api/admin/ |
| `internal/app/app.go` | Updated job manager constructor and wiring |
| `internal/common/config.go` | Extended JobManagerConfig (watcher_interval, max_retries, purge_after) |
| `config/vire-service.toml` | Updated [jobmanager] config section |
| `go.mod` | gorilla/websocket promoted to direct dependency |
| `README.md` | Added stock index, job queue, admin endpoints documentation |
| `.claude/skills/develop/SKILL.md` | Updated Key Directories, Storage Architecture, Job Manager, MarketService, Admin API sections |

## Tests
- 20 job manager unit tests — all passing
- 10 stress tests (concurrency, edge cases) — all passing
- `go build ./...` — clean
- `go vet ./...` — clean

## Documentation Updated
- `README.md` — features, architecture, endpoints
- `.claude/skills/develop/SKILL.md` — comprehensive updates across multiple sections

## Devils-Advocate Findings
1. **Retry logic bug** — Fixed: failed jobs now properly re-enqueue with incremented attempts
2. **WebSocket auth bypass** — Fixed: admin role check added to WebSocket upgrade handler
3. **Hub goroutine leak** — Fixed: proper cleanup on Stop() with client disconnect
4. **Lock race condition** — Fixed: mutex ordering in concurrent queue access

## Notes
- **Deployment**: Old root-owned vire-server (PID 211) holds port 8882. Container restart needed to deploy.
- **Stock index auto-population**: Tickers are automatically added when any user syncs a portfolio. The watcher then detects missing data and queues collection jobs.
- **Job priority system**: 8 job types with default priorities (EOD=10 highest, timeline=2 lowest). New stocks get elevated priority (15). Push-to-top sets priority to max+1.
- **Concurrency**: Configurable processor pool (default 5). EODHD rate limiter (10 req/sec) still handles API throttling independently.
- **WebSocket**: Admin-only real-time feed at `/api/admin/ws/jobs`. Broadcasts job_queued, job_started, job_completed, job_failed events.
- **3 minor doc inaccuracies** found by reviewer in SKILL.md job tables — sent to implementer for correction.
