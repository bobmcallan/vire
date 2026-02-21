# Requirements: Stock Index Table & Priority Job Queue

**Date:** 2026-02-21
**Requested:** Create a shared stock index table for cross-user stock tracking, refactor the job manager into a priority-based job queue with discrete job types, and add admin API + WebSocket for queue monitoring.

## Scope

### In Scope
1. **Stock Index Table** (`stock_index` SurrealDB table) — shared ticker registry across all users
2. **Priority Job Queue** (`job_queue` SurrealDB table) — persistent, priority-ordered work queue
3. **Discrete Job Types** — all data collection broken into individual queued jobs
4. **Stock Index Watcher** — scans index for stale data, auto-queues collection jobs
5. **Push-to-top** — API to prioritise specific jobs
6. **Admin API endpoints** — queue listing, status, priority management
7. **WebSocket** — real-time queue status stream (admin only)
8. **Portfolio sync integration** — upsert tickers to stock index during portfolio sync

### Out of Scope
- User-specific data in stock index (portfolios, watchlists stored separately)
- Authentication/authorization changes (admin check uses existing role system)
- Frontend/portal WebSocket consumer

## Approach

### 1. Stock Index Table

**SurrealDB table: `stock_index`**

The stock index is a shared, user-agnostic registry of all stocks the system should track. Any user process that touches a ticker upserts it to the index. The job manager watches this table to drive data collection.

```go
type StockIndexEntry struct {
    Ticker    string    `json:"ticker"`     // EODHD format: "BHP.AU"
    Code      string    `json:"code"`       // Base code: "BHP"
    Exchange  string    `json:"exchange"`   // Exchange: "AU"
    Name      string    `json:"name"`       // Company name (populated from fundamentals)
    Source    string    `json:"source"`     // How it was added: "portfolio", "watchlist", "search", "manual"

    // Data freshness timestamps (mirrors market_data component timestamps)
    EODCollectedAt             time.Time `json:"eod_collected_at"`
    FundamentalsCollectedAt    time.Time `json:"fundamentals_collected_at"`
    FilingsCollectedAt         time.Time `json:"filings_collected_at"`
    NewsCollectedAt            time.Time `json:"news_collected_at"`
    FilingSummariesCollectedAt time.Time `json:"filing_summaries_collected_at"`
    TimelineCollectedAt        time.Time `json:"timeline_collected_at"`
    SignalsCollectedAt         time.Time `json:"signals_collected_at"`

    // Lifecycle
    AddedAt    time.Time `json:"added_at"`     // First added
    LastSeenAt time.Time `json:"last_seen_at"` // Last referenced by a user process
}
```

**Upsert function** — called from portfolio sync, watchlist add, search, manual add:
```go
func UpsertStockIndex(ctx context.Context, ticker, source string) error
```
- If ticker doesn't exist: insert with AddedAt = now, all collection timestamps zero (triggers jobs)
- If ticker exists: update LastSeenAt, Source (keep most recent)
- Collection timestamps are updated by the job manager when jobs complete

**Sync integration** — in `portfolio/service.go` `SyncPortfolio()`:
After extracting holdings, upsert each ticker to the stock index:
```go
for _, h := range holdings {
    stockIndex.Upsert(ctx, h.EODHDTicker(), "portfolio")
}
```

### 2. Priority Job Queue

**SurrealDB table: `job_queue`**

```go
type Job struct {
    ID          string    `json:"id"`           // SurrealDB record ID
    JobType     string    `json:"job_type"`     // e.g., "collect_eod", "collect_filings"
    Ticker      string    `json:"ticker"`       // Target ticker (empty for global jobs)
    Priority    int       `json:"priority"`     // Higher = first. Default 0, push-to-top = 100
    Status      string    `json:"status"`       // "pending", "running", "completed", "failed", "cancelled"
    CreatedAt   time.Time `json:"created_at"`
    StartedAt   time.Time `json:"started_at"`
    CompletedAt time.Time `json:"completed_at"`
    Error       string    `json:"error,omitempty"`
    Attempts    int       `json:"attempts"`
    MaxAttempts int       `json:"max_attempts"` // Default 3
    DurationMS  int64     `json:"duration_ms"`
}
```

**Job Types** — each maps to a discrete data collection operation:

| JobType | Description | Estimated Duration | Default Priority |
|---------|-------------|-------------------|-----------------|
| `collect_eod` | Fetch EOD bars (incremental) | Fast (~1s) | 10 |
| `collect_fundamentals` | Fetch fundamentals | Fast (~1s) | 8 |
| `collect_filings` | Fetch + download filing PDFs | Slow (~30s) | 5 |
| `collect_news` | Fetch news articles | Fast (~1s) | 7 |
| `collect_filing_summaries` | AI summarize new filings | Very slow (~2-5min) | 3 |
| `collect_timeline` | Generate company timeline | Slow (~30s) | 2 |
| `collect_news_intel` | AI news intelligence | Slow (~30s) | 4 |
| `compute_signals` | Compute technical signals | Fast (~0.1s) | 9 |

**Queue operations:**
- `Enqueue(job)` — add job, skip if identical pending job exists (dedup by type+ticker+status=pending)
- `Dequeue()` — get highest-priority pending job, mark as running (atomic)
- `Complete(id, error)` — mark completed/failed, update timestamps
- `PushToTop(id)` — set priority to max(current_max) + 1
- `EnqueueWithPriority(job, priority)` — enqueue with specific priority
- `ListPending()` — return pending jobs ordered by priority desc, created_at asc
- `ListAll(limit)` — return recent jobs including completed/failed
- `PurgeCompleted(olderThan)` — cleanup old completed jobs
- `CancelByTicker(ticker)` — cancel all pending jobs for a ticker

**Deduplication:** Before enqueuing, check for existing pending job with same (job_type, ticker). If exists, skip. This prevents the watcher from flooding the queue.

### 3. Refactored Job Manager

The job manager becomes two cooperating loops:

**Watcher loop** (runs on configurable schedule, default 1 min):
1. Read all entries from `stock_index`
2. For each entry, check freshness of each data component against freshness TTLs
3. For stale components, enqueue the corresponding job type (with dedup)
4. New stocks (zero timestamps) get all job types queued with elevated priority

**Processor loop** (runs continuously):
1. Dequeue highest-priority pending job
2. Execute the job (call corresponding market service method)
3. On success: update stock_index timestamps, mark job completed
4. On failure: increment attempts, re-queue if under max_attempts, mark failed otherwise
5. Respect concurrency limit (configurable, default 5)
6. If queue is empty, sleep briefly (1 second) before re-checking

**Job execution** — each job type maps to an existing service method:

```go
func (m *JobManager) executeJob(ctx context.Context, job *Job) error {
    switch job.JobType {
    case "collect_eod":
        return m.market.CollectEOD(ctx, job.Ticker, false)
    case "collect_fundamentals":
        return m.market.CollectFundamentals(ctx, job.Ticker, false)
    case "collect_filings":
        return m.market.CollectFilings(ctx, job.Ticker, false)
    // ... etc
    }
}
```

This requires splitting `CollectMarketData` into individual public methods on MarketService. These methods already exist as inline blocks within `CollectMarketData` — they just need to be extracted.

### 4. MarketService Decomposition

Extract individual collection methods from `CollectMarketData`:

```go
// Individual collection methods (extracted from CollectMarketData)
CollectEOD(ctx, ticker, force) error
CollectFundamentals(ctx, ticker, force) error
CollectFilings(ctx, ticker, force) error
CollectNews(ctx, ticker, force) error
CollectFilingSummaries(ctx, ticker, force) error
CollectTimeline(ctx, ticker, force) error
CollectNewsIntelligence(ctx, ticker, force) error
```

`CollectMarketData` becomes a convenience wrapper that calls all of these.
`CollectCoreMarketData` calls only `CollectEOD` + `CollectFundamentals` (with batch optimization).

### 5. Admin API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/admin/jobs` | GET | List jobs (query: status, limit, ticker) |
| `/api/admin/jobs/queue` | GET | List pending jobs ordered by priority |
| `/api/admin/jobs/{id}/priority` | PUT | Set job priority (push to top: `{"priority": "top"}`) |
| `/api/admin/jobs/{id}/cancel` | POST | Cancel a pending job |
| `/api/admin/jobs/enqueue` | POST | Manually enqueue a job |
| `/api/admin/stock-index` | GET | List all stocks in index |
| `/api/admin/stock-index` | POST | Manually add a ticker to index |
| `/api/admin/ws/jobs` | GET | WebSocket: real-time job queue updates |

**Admin check:** Use existing user role system. Check `role == "admin"` from JWT claims or user context.

### 6. WebSocket

**Pattern:** Hub + Client model

```go
type JobWSHub struct {
    clients    map[*JobWSClient]bool
    broadcast  chan JobEvent
    register   chan *JobWSClient
    unregister chan *JobWSClient
}

type JobEvent struct {
    Type string      `json:"type"` // "job_queued", "job_started", "job_completed", "job_failed"
    Job  *Job        `json:"job"`
    Timestamp time.Time `json:"timestamp"`
}
```

- Hub runs as a goroutine, started with the job manager
- Job manager publishes events to the hub when jobs change state
- WebSocket handler upgrades HTTP connection, registers client with hub
- Hub broadcasts events to all connected clients
- Use `gorilla/websocket` (widely used, stable)

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/jobs.go` | **New** — StockIndexEntry, Job, JobEvent models |
| `internal/services/jobmanager/stockindex.go` | **New** — Stock index CRUD operations |
| `internal/services/jobmanager/queue.go` | **New** — Priority job queue with SurrealDB persistence |
| `internal/services/jobmanager/executor.go` | **New** — Job execution dispatch (type → service method) |
| `internal/services/jobmanager/watcher.go` | **New** — Stock index watcher (scans for stale data, enqueues jobs) |
| `internal/services/jobmanager/websocket.go` | **New** — WebSocket hub for real-time job events |
| `internal/services/jobmanager/manager.go` | **Refactor** — Watcher + processor loops, hub lifecycle |
| `internal/services/jobmanager/jobs.go` | **Refactor** — Replace cycle-based approach with discrete job definitions |
| `internal/services/market/service.go` | **Refactor** — Extract individual collection methods |
| `internal/interfaces/services.go` | Add individual collection methods to MarketService |
| `internal/interfaces/storage.go` | Add StockIndexStore interface |
| `internal/storage/surrealdb/manager.go` | Create stock_index, job_queue tables |
| `internal/storage/surrealdb/stockindex.go` | **New** — SurrealDB StockIndexStore implementation |
| `internal/storage/surrealdb/jobqueue.go` | **New** — SurrealDB JobQueueStore implementation |
| `internal/services/portfolio/service.go` | Upsert tickers to stock index during SyncPortfolio |
| `internal/server/handlers_admin.go` | **New** — Admin job queue + stock index handlers |
| `internal/server/routes.go` | Register admin routes |
| `internal/app/app.go` | Wire up refactored job manager |
| `internal/common/config.go` | Update JobManagerConfig (add watcher_interval, processor fields) |
| `config/vire-service.toml` | Update [jobmanager] config |
| `go.mod` / `go.sum` | Add gorilla/websocket dependency |
