# Requirements: Fix OOM kills in filing summarization pipeline

**Date:** 2026-02-24
**Requested:** Fix vire-server OOM kills on Fly.io at 2048MB with VIRE_JOBS_MAX_CONCURRENT=2 (fb_a7858449)

## Scope

### In Scope
1. **Memory-efficient PDF processing in `buildFilingSummaryPrompt`** — process PDFs one at a time, nil references after use
2. **Reduce batch size** — lower `filingSummaryBatchSize` from 5 to 2-3
3. **Per-batch persistence in `summarizeNewFilings`** — save summaries to MarketData after each batch, release batch results
4. **Explicit GC hints** — call `runtime.GC()` after each batch in memory-heavy paths
5. **Job-type concurrency limiting** — prevent two PDF-heavy job types from running simultaneously
6. **Startup storm mitigation** — stagger job enqueuing on startup instead of all-at-once
7. **Partial MarketData loading** — avoid loading full EOD/news when only filings+summaries are needed

### Out of Scope
- Replacing base64 storage with binary blob (larger refactor)
- Streaming Gemini responses (SDK limitation)
- Portal changes

## Root Cause Analysis

Multiple compounding memory issues:

| Location | Problem | Peak Memory |
|----------|---------|------------|
| `buildFilingSummaryPrompt` | 5 PDFs loaded simultaneously (base64 decode + pdf.Reader = ~50-73MB each) | 250-365MB per batch |
| `FileStore.GetFile` | base64 string + decoded bytes both alive | 23MB per 10MB PDF |
| `ExtractPDFTextFromBytes` | raw data + temp file + pdf.Reader all coexist | 40-60MB per PDF |
| `summarizeNewFilings` | All batch results accumulated in memory, 3 copies at return | Grows unbounded |
| `CollectFilingSummaries` | Loads full MarketData (EOD, news, etc.) — unused fields waste memory | 0.1-1MB+ per ticker |
| Startup storm | 7*N jobs enqueued immediately | Queue pressure |
| 2 concurrent processors | All above x2 | 500-800MB combined |

**Worst case with 2 concurrent filing summary jobs: ~550-840MB for the pipeline alone.**

## Approach

### Fix 1: Memory-efficient PDF processing (`filings.go`)
- In `buildFilingSummaryPrompt`, nil out `data` after text extraction in each loop iteration
- Reduce `filingSummaryBatchSize` from 5 to 2
- Add `runtime.GC()` hint after each batch in `summarizeNewFilings`

### Fix 2: Per-batch persistence (`filings.go`)
- In `summarizeNewFilings`, save accumulated summaries to MarketData after each batch
- Release the batch results slice before starting next batch
- Eliminates the 3-copy problem at return

### Fix 3: Job-type concurrency limiting (`manager.go`)
- Add a "heavy job" semaphore (capacity 1) for PDF-heavy job types: `collect_filings`, `collect_filing_summaries`
- Only one heavy job runs at a time, regardless of `max_concurrent`
- Light jobs (EOD, fundamentals, signals) run freely

### Fix 4: Startup staggering (`watcher.go`)
- Add configurable startup delay before first scan (env: `VIRE_WATCHER_STARTUP_DELAY`, default 10s)
- Batch the initial scan: process N tickers at a time with a short sleep between batches

### Fix 5: Partial MarketData for summaries (`collect.go`)
- When loading MarketData for `CollectFilingSummaries`, only deserialize `Filings` and `FilingSummaries` fields
- Or: load full MarketData but nil out unused fields immediately after load

## Files Expected to Change
- `internal/services/market/filings.go` — batch size, per-batch persistence, nil references, GC hints
- `internal/services/market/collect.go` — partial loading or field pruning
- `internal/services/jobmanager/manager.go` — heavy job semaphore
- `internal/services/jobmanager/watcher.go` — startup delay, staggered scanning
- `internal/services/jobmanager/executor.go` — pass semaphore to heavy jobs
- `internal/common/config.go` — new config fields (startup delay, heavy job limit)
- `config/vire-service.toml` — new config entries
