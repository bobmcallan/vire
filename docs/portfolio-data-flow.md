# Portfolio Request Data Flow

Documents the data flow when a user requests a portfolio report, covering the synchronous pipeline, data sources, and relationship with the background job queue.

## Report Generation Pipeline

`GenerateReport` in `internal/services/report/service.go` runs a **synchronous** 5-step pipeline:

```
SyncPortfolio ─► CollectCoreMarketData ─► ReviewPortfolio ─► buildReport ─► saveReportRecord
     │                    │
     ▼                    ▼
  Navexa API          EODHD API
 (user holdings)    (EOD + fundamentals)
```

| Step | Method | Data Source | Blocking |
|------|--------|-------------|----------|
| 1 | `SyncPortfolio` | Navexa API | Yes — waits for response |
| 2 | `CollectCoreMarketData` | EODHD API | Yes — waits for all tickers |
| 3 | `ReviewPortfolio` | Local storage (cached from steps 1-2) | Yes |
| 4 | `buildReport` | In-memory | Yes |
| 5 | `saveReportRecord` | Local storage | Yes |

The report is returned to the caller only after all steps complete. Both Navexa portfolio data and EODHD stock prices are required to produce the report.

## EOD Price History

`CollectCoreMarketData` (`internal/services/market/service.go`) fetches **36 months (3 years)** of daily EOD bars on first request:

```go
s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
```

On subsequent requests, it fetches incrementally from the last known date forward.

### EODHD API Strategy

| API Call | Scope | Returns | Used For |
|----------|-------|---------|----------|
| `GetBulkEOD` | One call per exchange | Last trading day only | Cheaply append today's bar when history exists |
| `GetEOD` | One call per ticker | Full date range | 3-year history (first fetch), incremental fills (subsequent) |

The bulk API (`/eod-bulk-last-day/{exchange}`) does not support historical date ranges. All historical data (24+ months) uses individual per-ticker `GetEOD` calls (`/eod/{ticker}` with `from`/`to` parameters).

### Concurrency

- Up to **5 concurrent goroutines** process tickers in parallel (`maxConcurrent = 5`)
- EODHD rate limiter (10 req/sec) is applied independently at the client level
- Context cancellation is respected — if the request is cancelled, in-flight goroutines exit

## Relationship to Background Job Queue

The synchronous report pipeline and the background job queue are **independent systems**:

| Concern | Synchronous Path | Background Path |
|---------|-------------------|-----------------|
| Trigger | User requests a report | Job queue watcher (configurable interval) |
| Data | EOD bars + fundamentals | Filings, news, AI summaries, timelines, signals |
| Latency | Blocks until complete | Runs asynchronously, no user waiting |
| Method | `CollectCoreMarketData` (direct call) | Individual `Collect*` methods via job executor |

The job queue watcher scans the stock index for stale detail data and enqueues collection jobs. It does **not** handle EOD or fundamentals for report generation — those are always fetched synchronously by `GenerateReport`.

### Side Effect: Stock Index Population

During `SyncPortfolio`, all portfolio tickers are upserted to the shared stock index (`internal/services/portfolio/service.go`). This triggers the job queue watcher to detect new or stale tickers and enqueue background detail collection jobs (filings, news, AI analysis) on its next scan.

```
User requests report
    │
    ├─► SyncPortfolio (Navexa)
    │       └─► Upserts tickers to stock_index (side effect)
    │
    ├─► CollectCoreMarketData (EODHD — synchronous)
    │
    └─► ReviewPortfolio + buildReport + save
            │
            ▼
        Report returned to user

        Meanwhile (background, independent):
        Job queue watcher detects new tickers in stock_index
            └─► Enqueues detail jobs (filings, news, AI, timeline)
                    └─► Processor pool executes jobs by priority
```

## Key Files

| File | Role |
|------|------|
| `internal/services/report/service.go` | `GenerateReport` — synchronous pipeline |
| `internal/services/market/service.go` | `CollectCoreMarketData`, `collectCoreTicker` — EOD + fundamentals |
| `internal/services/market/collect.go` | Individual collection methods (used by job queue) |
| `internal/clients/eodhd/client.go` | `GetEOD` (per-ticker), `GetBulkEOD` (last day batch) |
| `internal/services/portfolio/service.go` | `SyncPortfolio` — Navexa sync + stock index upsert |
| `internal/services/jobmanager/watcher.go` | Background watcher — scans stock index for stale data |
| `internal/services/jobmanager/executor.go` | Background executor — dispatches jobs to collection methods |
| `internal/common/freshness.go` | TTL constants for data freshness checks |
