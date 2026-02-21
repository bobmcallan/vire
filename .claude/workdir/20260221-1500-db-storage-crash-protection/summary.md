# Summary: Database storage for files + crash protection

**Date:** 2026-02-21
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/interfaces/storage.go` | Added `FileStore` interface and `FileStore()` to `StorageManager` |
| `internal/storage/surrealdb/filestore.go` | New — `FileStore` implementation backed by SurrealDB `files` table |
| `internal/storage/surrealdb/filestore_test.go` | New — unit tests for FileStore |
| `internal/storage/surrealdb/filestore_stress_test.go` | New — stress tests for FileStore |
| `internal/storage/surrealdb/manager.go` | Added `files` table to init, `FileStore()` accessor, `WriteRaw` delegates to FileStore |
| `internal/storage/surrealdb/marketstore.go` | `PurgeCharts` updated to use FileStore |
| `internal/storage/surrealdb/jobqueue.go` | Added `ResetRunningJobs()` — resets orphaned jobs on startup |
| `internal/services/market/filings.go` | PDF download → FileStore, `extractPDFText` reads from DB |
| `internal/services/jobmanager/manager.go` | Added `safeGo()` panic recovery wrapper, `ResetRunningJobs` on startup |
| `internal/services/jobmanager/websocket.go` | Added `done` channel + `Stop()`, fixed broadcast lock escalation race |
| `internal/services/jobmanager/watcher.go` | Removed duplicate `wg.Done()` (now handled by `safeGo`) |
| `internal/services/jobmanager/devils_advocate_test.go` | Stress tests for crash recovery, fixed WaitGroup in test goroutines |
| `internal/services/jobmanager/manager_test.go` | Tests for ResetRunningJobs, panic recovery |
| `internal/services/jobmanager/manager_stress_test.go` | Fixed WaitGroup in test goroutines |

## Tests
- jobmanager tests: PASS (9.3s)
- report tests: PASS
- portfolio tests: PASS
- plan tests: PASS
- go vet: clean
- go build: clean
- Market tests with network deps timeout (pre-existing, not related to changes)

## What Was Built

### 1. FileStore — SurrealDB file storage
- New `files` table stores binary data as base64 in SurrealDB
- Record ID: `files:{category}_{sanitized_key}`
- Filing PDFs saved via `SaveFile("filing_pdf", ...)` instead of filesystem
- `extractPDFText` loads PDF bytes from DB, writes to temp file for pdf library
- `WriteRaw` delegates to `SaveFile("chart", ...)` for chart PNGs
- `PurgeCharts` deletes from DB instead of filesystem

### 2. Crash Protection
- `safeGo()` wrapper: all job manager goroutines recover from panics with stack trace logging
- `ResetRunningJobs()`: orphaned "running" jobs reset to "pending" on startup
- WebSocket hub: added `done` channel + `Stop()` for graceful shutdown
- Fixed broadcast race: RLock→Lock escalation during slow client cleanup

## Notes
- Some market tests timeout on real HTTP calls to ASX/EODHD (pre-existing issue — tests should mock external APIs)
- SurrealDB container tests require Docker (skipped locally without SurrealDB)
- WSL crashed during team execution; compilation errors and WaitGroup double-Done fixed directly by team lead
