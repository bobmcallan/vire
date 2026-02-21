# Requirements: Database storage for files + crash protection

**Date:** 2026-02-21
**Requested:** (1) All files stored in database, including filings and charts. (2) Investigate and fix container crash loop.

## Scope
- **In scope:**
  - Store filing PDFs in SurrealDB instead of filesystem
  - Store charts in SurrealDB instead of filesystem
  - Add panic recovery to all job manager goroutines
  - Reset orphaned "running" jobs on startup
  - Fix WebSocket hub shutdown path
- **Out of scope:**
  - S3/GCS object storage (future Phase 2)
  - Changing the SurrealDB client library
  - Restructuring existing SurrealDB tables

## Investigation Findings

### Filesystem Storage (current)
| Data | Location | Size | Reader |
|------|----------|------|--------|
| Filing PDFs | `data/filings/{TICKER}/*.pdf` | 200KB-5MB each | `extractPDFText()` for Gemini |
| Charts | `data/market/charts/*.png` | 50-500KB | Not wired up in production yet |

### Crash Root Cause
- Container: 49 restarts, 8.3 GiB memory, 445% CPU
- No panic recovery on any job manager goroutine (processLoop, watchLoop, hub.Run)
- "running" jobs never reset on startup → orphaned forever
- 5 concurrent processors dequeuing filing summary jobs → Gemini + PDF extraction → memory explosion
- Silent crash: no shutdown log, no panic log (unrecovered panic kills process)

## Approach

### Part 1: File storage in SurrealDB

**New `files` table** in SurrealDB for binary blobs:
```
files:{category}:{key}  →  { category, key, content_type, size, data (base64), created_at, updated_at }
```

Categories: `"filing_pdf"`, `"chart"`

**New interface:** Add `FileStore` to `StorageManager`:
```go
type FileStore interface {
    SaveFile(ctx context.Context, category, key string, data []byte, contentType string) error
    GetFile(ctx context.Context, category, key string) ([]byte, string, error)  // data, contentType, error
    DeleteFile(ctx context.Context, category, key string) error
    HasFile(ctx context.Context, category, key string) (bool, error)
}
```

**Migration path for filing PDFs:**
- `downloadFilingPDFs()` saves to DB via `FileStore.SaveFile("filing_pdf", "{ticker}/{date}-{dockey}.pdf", data, "application/pdf")`
- `extractPDFText()` reads from DB via `FileStore.GetFile()` → writes to temp file → extracts → deletes temp file
- `CompanyFiling.PDFPath` and `FilingSummary.PDFPath` field semantics change: store the DB key instead of filesystem path
- `WriteRaw()` delegates to `FileStore.SaveFile("chart", "{subdir}/{key}", data, contentType)`
- Remove `DataPath()` from StorageManager (or keep as temp dir fallback)
- Remove `PurgeCharts()` filesystem logic, replace with DB delete

### Part 2: Crash protection

1. **Panic recovery wrapper** — Add `safeGo(name string, fn func())` helper:
   ```go
   func (jm *JobManager) safeGo(name string, fn func()) {
       go func() {
           defer func() {
               if r := recover(); r != nil {
                   jm.logger.Error().Str("goroutine", name).Interface("panic", r).
                       Str("stack", string(debug.Stack())).Msg("Recovered from panic")
               }
           }()
           fn()
       }()
   }
   ```
   Apply to: `processLoop`, `watchLoop`, `hub.Run()`, websocket pumps.

2. **Reset orphaned jobs on startup** — Add `ResetRunningJobs()` to JobQueueStore:
   ```sql
   UPDATE job_queue SET status = 'pending', started_at = NONE WHERE status = 'running'
   ```
   Called from `JobManager.Start()` before launching goroutines.

3. **WebSocket hub done channel** — Add `done chan struct{}` to allow `hub.Stop()`.

## Files Expected to Change

### New files
- `internal/storage/surrealdb/filestore.go` — FileStore implementation
- `internal/storage/surrealdb/filestore_test.go` — tests

### Modified files
- `internal/interfaces/storage.go` — add FileStore interface
- `internal/storage/surrealdb/manager.go` — implement FileStore(), update WriteRaw, add `files` table to init
- `internal/models/market.go` — no structural change, PDFPath semantics change (filesystem path → DB key)
- `internal/services/market/filings.go` — use FileStore for PDF save/read, update downloadFilingPDFs and extractPDFText
- `internal/services/jobmanager/manager.go` — add safeGo(), panic recovery, call ResetRunningJobs on Start()
- `internal/storage/surrealdb/jobqueue.go` — add ResetRunningJobs()
- `internal/services/jobmanager/websocket.go` — add done channel, fix broadcast race
