# Plan: Separate Filings Collection into Fast (Index) and Slow (PDFs + AI)

## Context

The current `get_stock_data` MCP tool times out for new tickers with many filings because:
1. `CollectFilings` combines fast (HTML index ~1s) and slow (PDF downloads ~5s+) operations
2. `CollectFilingSummaries` takes 30s+ for AI summarization of many filings
3. Even with background jobs, the filing INDEX is not available until the slow job completes

**Goal**: Return filing index immediately as part of core data, while PDF downloads and AI summarization happen asynchronously.

## Key Observations

### Current Flow (Problem)
```
get_stock_data(force_refresh=true)
  -> CollectCoreMarketData() [EOD + Fundamentals ~600ms] - INLINE
  -> EnqueueSlowDataJobs() [collect_filings, collect_filing_summaries, ...] - BACKGROUND
  -> Return with advisory

But: collect_filings job must complete before filing index is available!
```

### Desired Flow
```
get_stock_data(force_refresh=true)
  -> CollectCoreMarketData() [EOD + Fundamentals + Filing Index ~2s] - INLINE
  -> EnqueueSlowDataJobs() [collect_filing_pdfs, collect_filing_summaries, ...] - BACKGROUND
  -> Return with filing index (81 filings visible), PDFs/summaries pending
```

## Implementation Plan

### 1. Split `CollectFilings` into Index and PDFs

**File**: `internal/services/market/collect.go`

Create two new methods:

```go
// CollectFilingsIndex - Fast: fetch ASX HTML index only (~1s)
func (s *Service) CollectFilingsIndex(ctx context.Context, ticker string, force bool) error {
    // 1. Check freshness via FilingsIndexUpdatedAt
    // 2. Call collectFilings() for HTML parsing
    // 3. Merge with existing PDF paths (preserve downloaded files)
    // 4. Save with FilingsIndexUpdatedAt timestamp
}

// CollectFilingPdfs - Slow: download PDFs (~5s+)
func (s *Service) CollectFilingPdfs(ctx context.Context, ticker string, force bool) error {
    // 1. Get existing filings from MarketData
    // 2. Call downloadFilingPDFs() for files not yet downloaded
    // 3. Save with FilingsPdfsUpdatedAt timestamp
}
```

Keep `CollectFilings` as backward-compatible wrapper:
```go
func (s *Service) CollectFilings(ctx context.Context, ticker string, force bool) error {
    if err := s.CollectFilingsIndex(ctx, ticker, force); err != nil {
        return err
    }
    return s.CollectFilingPdfs(ctx, ticker, force)
}
```

### 2. Add Filing Index to Fast Path (CollectCoreMarketData)

**File**: `internal/services/market/service.go` (line ~165)

Add filing index collection to `CollectMarketData` fast path:

```go
// In CollectMarketData, after fundamentals:
if include.Filings && (force || !common.IsFresh(existing.FilingsIndexUpdatedAt, common.FreshnessFilings)) {
    filings, err := s.collectFilings(ctx, ticker)
    if err != nil {
        s.logger.Warn().Err(err).Msg("Failed to collect filings index")
    } else {
        // Merge with existing PDF paths
        if existing != nil && len(existing.Filings) > 0 {
            filings = s.mergeFilingPDFPaths(existing.Filings, filings)
        }
        marketData.Filings = filings
        marketData.FilingsIndexUpdatedAt = now
    }
}
```

### 3. New Job Types and Timestamps

**File**: `internal/models/jobs.go`

Add new job type:
```go
const (
    // ... existing ...
    JobTypeCollectFilingPdfs = "collect_filing_pdfs"  // NEW: Download PDFs only
)

const (
    // ... existing ...
    PriorityCollectFilingPdfs = 4  // Same level as summaries (heavy job)
)
```

Update `TimestampFieldForJobType`:
```go
case JobTypeCollectFilings:
    return "filings_index_collected_at"  // Changed: now only index
case JobTypeCollectFilingPdfs:
    return "filings_pdfs_collected_at"   // NEW
```

**File**: `internal/models/market.go`

Add new timestamp field:
```go
type MarketData struct {
    // ... existing ...
    FilingsIndexUpdatedAt time.Time `json:"filings_index_updated_at"` // NEW
    FilingsPdfsUpdatedAt  time.Time `json:"filings_pdfs_updated_at"`  // NEW
    // Keep FilingsUpdatedAt for backward compat, deprecate gradually
}
```

### 4. Update Job Executor

**File**: `internal/services/jobmanager/executor.go`

```go
case models.JobTypeCollectFilingPdfs:
    return jm.market.CollectFilingPdfs(ctx, job.Ticker, false)
```

### 5. Update EnqueueSlowDataJobs

**File**: `internal/services/jobmanager/watcher.go`

Replace `JobTypeCollectFilings` with `JobTypeCollectFilingPdfs`:
```go
slowJobs := []struct{...}{
    {models.JobTypeCollectFilingPdfs, models.PriorityCollectFilingPdfs},  // Changed
    {models.JobTypeCollectFilingSummaries, models.PriorityCollectFilingSummaries},
    // ... rest unchanged
}
```

### 6. Smart Filing Caching (Incremental Collection)

**File**: `internal/services/market/filings.go`

The `collectFilings` function already fetches Y1 + Y3 years. For incremental updates:
- Only fetch current year if `FilingsIndexUpdatedAt` is fresh
- Merge new filings with existing ones
- Preserve PDF paths from previous downloads

```go
func (s *Service) mergeFilingPDFPaths(oldFilings, newFilings []models.CompanyFiling) []models.CompanyFiling {
    oldByKey := make(map[string]models.CompanyFiling)
    for _, f := range oldFilings {
        oldByKey[f.DocumentKey] = f
    }
    for i := range newFilings {
        if old, ok := oldByKey[newFilings[i].DocumentKey]; ok {
            newFilings[i].PDFPath = old.PDFPath
            newFilings[i].FileSize = old.FileSize
        }
    }
    return newFilings
}
```

### 7. Update isHeavyJob

**File**: `internal/services/jobmanager/manager.go`

```go
func isHeavyJob(jobType string) bool {
    return jobType == models.JobTypeCollectFilingPdfs ||
           jobType == models.JobTypeCollectFilingSummaries
    // Remove JobTypeCollectFilings since it's now fast (index only)
}
```

## Critical Files to Modify

1. `internal/services/market/collect.go` - Split CollectFilings into Index + PDFs
2. `internal/services/market/service.go` - Add filing index to fast path
3. `internal/models/jobs.go` - New job type + timestamps
4. `internal/models/market.go` - New timestamp fields
5. `internal/services/jobmanager/executor.go` - Handle new job type
6. `internal/services/jobmanager/watcher.go` - Update EnqueueSlowDataJobs
7. `internal/services/jobmanager/manager.go` - Update isHeavyJob
8. `internal/storage/surrealdb/stockindex.go` - Add new timestamp fields to allowed list

## Verification

1. **Test with SGI.AU**:
   - Request `get_stock_data` with `force_refresh=true`
   - Verify response returns within 3s with filing index (81 filings visible)
   - Verify background jobs process PDFs and summaries

2. **Check logs**:
   ```bash
   docker logs vire-server 2>&1 | grep -E "(SGI|filings|Job)"
   ```

3. **Verify filing index returned**:
   - Response should include `filings` array with all 81 entries
   - `pdf_path` may be empty initially (PDFs pending)
   - `filing_summaries` and `company_timeline` may be empty (AI pending)

4. **Run tests**:
   ```bash
   go test ./internal/services/market/... ./internal/services/jobmanager/...
   ```

## Migration Notes

- Existing `FilingsUpdatedAt` field kept for backward compatibility
- New fields `FilingsIndexUpdatedAt` and `FilingsPdfsUpdatedAt` added
- Old jobs with `JobTypeCollectFilings` still work (wrapper calls both)
- Freshness check uses new timestamps for granular control
