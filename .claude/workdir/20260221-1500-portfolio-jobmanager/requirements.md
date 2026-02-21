# Requirements: Portfolio Report Split, EODHD Batch Optimization, and Job Manager

**Date:** 2026-02-21
**Requested:** Split portfolio report into fast report + detail, optimize EODHD downloads with batch/concurrent fetching, introduce a background job manager for data collection, and fix container issues (context cancellation, zlib panic).

## Scope

### In Scope
1. **Split report into Report (fast) + Detail (per-ticker on-demand)**
2. **EODHD batch/concurrent optimization** in `CollectMarketData`
3. **Background job manager** running every minute
4. **Caching/freshness rules** ensuring no redundant work
5. **Container bug fixes** from production logs (context cancellation, zlib panic)
6. **SurrealDB job tracking** table for job run history

### Out of Scope
- SurrealDB vectorisation (no clear use case for this change set)
- SurrealDB calculated columns (freshness logic is better in Go for testability)
- Changes to Navexa client or auth flow
- Frontend/portal changes

## Container Issues (from logs)

**Container ID:** 298fdfcbdc66...

1. **Context cancellation cascade** — HTTP request context propagates into filing summarization (Gemini API calls). When client disconnects or request times out, all in-flight Gemini calls fail with `context canceled`. The job manager solves this by moving heavy work to a background context.

2. **Panic: `zlib: invalid header`** on `/api/portfolios/SMSF/report` — A corrupt PDF triggers an unrecovered panic in filing PDF processing. Need `recover()` guard in PDF processing.

## Approach

### 1. Split Report into Report + Detail

**Current flow** (`report/service.go:GenerateReport`):
```
SyncPortfolio → CollectMarketData (ALL: EOD+fundamentals+news+filings+AI) → DetectSignals → ReviewPortfolio → buildReport
```

**New flow — Report (fast path)**:
```
SyncPortfolio → CollectCoreMarketData (EOD+fundamentals only, batch) → buildReport (holdings table, no signals/compliance)
```
- Report shows: holdings, prices, fundamentals summary, position weights, returns
- No: filings, filing summaries, AI analysis, signals, compliance checks, news intelligence
- Returns in seconds instead of minutes

**New flow — Detail (per-ticker, on-demand or via job manager)**:
```
CollectDetailedMarketData (filings+news+AI) → DetectSignals → ReviewPortfolio (single ticker context)
```
- This is what `GenerateTickerReport` already partially does
- The job manager will proactively run this for all tickers in the background

### 2. EODHD Batch/Concurrent Optimization

**New method: `CollectCoreMarketData(ctx, tickers)`** in `market/service.go`:
- Uses `GetBulkEOD` for last-day EOD across all tickers in one API call
- Concurrent goroutines (bounded by `sync.WaitGroup` + semaphore matching rate limit) for:
  - Full EOD history (only for tickers with no existing data)
  - Fundamentals (only when stale per `FreshnessFundamentals`)
- Respects existing EODHD rate limiter (10 req/sec)
- Skips: filings, news, AI summaries, news intelligence

**Existing `CollectMarketData`** remains for detailed/full collection, called by:
- Job manager for deep analysis
- `GetStockData` for on-demand single-ticker detail
- `GenerateTickerReport` for per-ticker refresh

### 3. Job Manager

**New package: `internal/services/jobmanager/`**

**JobManager struct:**
- Runs on configurable interval (default: 1 minute)
- Uses `context.Background()` — not tied to HTTP request lifecycle
- Tracks job runs in SurrealDB `job_runs` table

**Job cycle (every tick):**
1. **Portfolio refresh**: Read default portfolio from storage (no Navexa sync — that needs user API key from HTTP context)
2. **Core market data**: Call `CollectCoreMarketData` for all portfolio tickers (batch EOD + concurrent fundamentals)
3. **Deep analysis queue**: For each ticker, if detailed data is stale:
   - Collect filings (if stale per `FreshnessFilings`)
   - Summarize new filings only (skip already-summarized, incremental)
   - Rebuild company timeline (if summaries changed)
   - Collect news (if stale per `FreshnessNews`)
   - Generate news intelligence (if news changed)
4. **Signal refresh**: Recompute signals for tickers with changed EOD data
5. **Log job completion** with timing and per-ticker status

**Caching rules:**
- EOD bars: Skip if fresh (< `FreshnessTodayBar`), incremental merge if stale
- Fundamentals: Skip if fresh (< `FreshnessFundamentals`)
- Filings: Skip if fresh (< `FreshnessFilings`)
- Filing summaries: Only process new filings (compare against existing summaries)
- News: Skip if fresh (< `FreshnessNews`)
- News intelligence: Skip if fresh (< `FreshnessNewsIntel`)
- Signals: Recompute only when EOD data changed
- Analysis (in ReviewPortfolio): Always redo — prices/currency change constantly

### 4. Integration Points

**`internal/app/app.go`:**
- Add `JobManager` field to `App` struct
- Initialize in `NewApp`
- Add `StartJobManager()` / stop in `Close()`
- Replace or complement existing `StartPriceScheduler` and `StartWarmCache`

**`internal/common/config.go`:**
- Add `[jobmanager]` config section: `interval`, `enabled`, `max_concurrent`

**`internal/server/handlers.go`:**
- `handlePortfolioReport`: Call `GenerateReport` (fast path, no detail)
- `handlePortfolioTickerReport`: Unchanged (already per-ticker detail)
- New: `handleJobStatus` GET endpoint showing last job run info

**SurrealDB:**
- New `job_runs` table: `{ id, started_at, completed_at, status, tickers_processed, errors, duration_ms }`
- Queryable for monitoring/debugging

### 5. Bug Fixes

**Context cancellation (`market/filings.go`):**
- `summarizeFilingBatch` and `downloadFilingPDFs` should use a detached context when called from job manager
- The job manager naturally solves this by using `context.Background()`
- For HTTP-triggered paths, the existing behavior is acceptable (user cancels → work stops)

**Zlib panic (`market/filings.go`):**
- Add `recover()` in `downloadFilingPDFs` around PDF processing
- Log corrupt PDFs and skip them instead of panicking
- Mark corrupt filings so they're not retried

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/services/market/service.go` | Add `CollectCoreMarketData`, refactor `CollectMarketData` |
| `internal/services/market/filings.go` | Add panic recovery for corrupt PDFs |
| `internal/services/report/service.go` | Refactor `GenerateReport` to use fast path |
| `internal/services/jobmanager/manager.go` | **New** — Job manager core |
| `internal/services/jobmanager/jobs.go` | **New** — Individual job implementations |
| `internal/app/app.go` | Wire up JobManager, add `StartJobManager()` |
| `internal/app/scheduler.go` | Integrate with or replace with JobManager |
| `internal/common/config.go` | Add JobManager config |
| `internal/interfaces/services.go` | Add `CollectCoreMarketData` to MarketService |
| `internal/server/handlers.go` | Add job status endpoint |
| `internal/server/routes.go` | Register job status route |
| `config/vire-service.toml` | Add `[jobmanager]` section |
| `internal/storage/surrealdb/manager.go` | Create `job_runs` table |
