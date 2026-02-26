# Job Manager

`internal/services/jobmanager/`

Queue-driven background service with three components.

## Architecture

- **Watcher** (`watcher.go`): Configurable startup delay (default 10s), then scans stock index on interval (default 1m). Checks per-component freshness against TTLs. Deduplicates via HasPendingJob. New stocks (< 5min) get elevated priority. EOD grouped per-exchange as `collect_eod_bulk`. `compute_signals` is skipped when `EODCollectedAt.IsZero()` — prerequisite guard, not a TTL check.
- **Processor Pool** (`manager.go`): N concurrent goroutines (default 5). PDF-heavy jobs rate-limited by semaphore (default 1).
- **Executor** (`executor.go`): Dispatches by job type to MarketService methods. Updates stock index timestamps on success only. `compute_signals` returns an error (not nil) when market data or EOD is absent — this prevents the freshness timestamp from being updated and allows the watcher to re-enqueue.
- **Queue** (`queue.go`): Thin wrappers around JobQueueStore. Broadcasts JobEvent via WebSocket.
- **WebSocket Hub** (`websocket.go`): gorilla/websocket broadcasting to admin clients at `/api/admin/ws/jobs`.

## Constructor

`NewJobManager(market, signal, storage, logger, config)` — operates on stock index, not portfolios.

## Flow

1. Portfolio sync upserts tickers to stock index
2. Watcher scans stock index, enqueues for stale data
3. Processor pool dequeues by priority, executes via MarketService
4. Admin API allows manual enqueue, priority changes, cancellation
5. WebSocket broadcasts real-time job events
6. Demand-driven: `handlePortfolioGet` and `handlePortfolioReview` fire-and-forget `EnqueueTickerJobs`
7. Force refresh: `handleMarketStocks` with `force_refresh=true` calls CollectCoreMarketData inline + EnqueueSlowDataJobs background

## Job Types

| Constant | Value | Priority |
|----------|-------|----------|
| `JobTypeCollectEOD` | `collect_eod` | 10 |
| `JobTypeCollectEODBulk` | `collect_eod_bulk` | 10 |
| `JobTypeCollectFundamentals` | `collect_fundamentals` | 8 |
| `JobTypeCollectFilings` | `collect_filings` | 5 |
| `JobTypeCollectNews` | `collect_news` | 5 |
| `JobTypeCollectFilingSummaries` | `collect_filing_summaries` | 3 |
| `JobTypeCollectTimeline` | `collect_timeline` | 3 |
| `JobTypeCollectNewsIntelligence` | `collect_news_intelligence` | 3 |
| `JobTypeComputeSignals` | `compute_signals` | 7 |

## Priority Constants

| Constant | Value | Usage |
|----------|-------|-------|
| `PriorityNewStock` | 15 | New stocks (< 5min) |
| `PriorityManual` | 20 | Admin API enqueue |
| `PriorityUrgent` | 50 | Push-to-top |

## Config

```toml
[jobmanager]
enabled = true
watcher_interval = "1m"
max_concurrent = 5
max_retries = 3
purge_after = "24h"
watcher_startup_delay = "10s"
heavy_job_limit = 1
```

Env overrides: `VIRE_WATCHER_STARTUP_DELAY`, `VIRE_JOBS_HEAVY_LIMIT`.
