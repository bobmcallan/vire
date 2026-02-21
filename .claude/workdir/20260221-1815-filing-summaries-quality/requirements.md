# Requirements: Filing Summary Caching, Financial Focus & Quality Assessment

**Date:** 2026-02-21
**Requested:** Fix filing summary re-processing on deep dive, enhance financial performance extraction, add quality company assessment with rating.

## Problem Analysis (from container logs)

Container `aef1fa4a...` shows SRG.AU filing summaries being re-generated from scratch on every `GetStockData` call:

```
Summarizing new filings  existing=0  new=58  ticker=SRG.AU
```

**Root cause:** `GetStockData()` at `internal/services/market/service.go:540` calls `summarizeNewFilings()` inline on every request. When the job manager is also running `CollectFilingSummaries` concurrently, there's a race condition. The context gets cancelled due to HTTP timeout mid-batch, so results never save, and the next request starts over.

The job manager already handles scheduled summarization — the inline call in `GetStockData` is redundant and wasteful.

## Scope

### In Scope
1. **Fix caching bug** — Remove inline `summarizeNewFilings` and `generateCompanyTimeline` from `GetStockData`. Serve stored summaries/timeline only. Job manager handles generation.
2. **Prompt version tracking** — Hash the filing summary prompt template. Store alongside summaries. If prompt changes (developer updates instructions), mark summaries stale so job manager regenerates.
3. **Financial performance focus** — Enhance the Gemini prompt and `FilingSummary` struct to extract a financial performance summary and significant performance commentary per filing.
4. **Quality company assessment** — New `QualityAssessment` struct computed from EODHD fundamentals (ROE, Gross Margin, FCF Conversion, Debt/EBITDA, Earnings Variability, red flags). Overall quality rating. Stored on `MarketData`, recalculated when fundamentals change.
5. **API endpoints** — `GET /api/market/stocks/{ticker}/filing-summaries` returns filing summaries + quality assessment (read-only).

### Out of Scope
- Write/update endpoints for filing summaries (read-only for now)
- UI changes
- Changes to the job manager watcher/scheduler

## Approach

### A. Fix GetStockData Caching (service.go)

Remove lines 536-555 (inline `summarizeNewFilings`) and lines 558-570 (inline `generateCompanyTimeline`) from `GetStockData()`. Replace with:

```go
// Serve stored filing summaries — generation handled by job manager
stockData.Filings = marketData.Filings
stockData.FilingSummaries = marketData.FilingSummaries
stockData.Timeline = marketData.CompanyTimeline
```

### B. Prompt Version Tracking

1. Add `FilingSummaryPromptHash string` field to `MarketData` model.
2. Create `filingSummaryPromptHash()` function that hashes the template portion of the prompt (excluding per-filing content).
3. In `CollectFilingSummaries`, compare stored hash to current. If different, set `force=true` to clear and regenerate.
4. Bump `SchemaVersion` to "6".

### C. Enhanced Financial Performance Extraction

Add two fields to `FilingSummary`:
- `FinancialSummary string` — one-line financial performance summary (e.g. "Revenue grew 92% to $261.7M with net profit doubling to $14.0M")
- `PerformanceCommentary string` — significant management commentary on financial performance

Update `buildFilingSummaryPrompt` output schema to include these fields.
Update `filingSummaryRaw` parsing struct.

### D. Quality Company Assessment

New file: `internal/services/market/quality.go`

```go
type QualityAssessment struct {
    ROE              QualityMetric `json:"roe"`
    GrossMargin      QualityMetric `json:"gross_margin"`
    FCFConversion    QualityMetric `json:"fcf_conversion"`
    NetDebtToEBITDA  QualityMetric `json:"net_debt_to_ebitda"`
    EarningsStability QualityMetric `json:"earnings_stability"`
    RevenueGrowth    QualityMetric `json:"revenue_growth"`
    MarginTrend      QualityMetric `json:"margin_trend"`
    RedFlags         []string      `json:"red_flags,omitempty"`
    Strengths        []string      `json:"strengths,omitempty"`
    OverallRating    string        `json:"overall_rating"`  // "High Quality", "Quality", "Average", "Below Average", "Speculative"
    OverallScore     int           `json:"overall_score"`   // 0-100
    AssessedAt       time.Time     `json:"assessed_at"`
}

type QualityMetric struct {
    Value     float64 `json:"value"`
    Benchmark string  `json:"benchmark"`
    Rating    string  `json:"rating"` // "excellent", "good", "average", "poor"
    Score     int     `json:"score"`  // 0-25
}
```

Computed from `Fundamentals` fields: `ReturnOnEquityTTM`, `GrossProfitTTM/RevenueTTM`, `HistoricalFinancials` for trends.

### E. API Endpoint

`GET /api/market/stocks/{ticker}/filing-summaries`

Response:
```json
{
  "ticker": "SRG.AU",
  "filing_summaries": [...],
  "quality_assessment": {...},
  "summary_count": 58,
  "last_updated": "2026-02-21T06:45:00Z"
}
```

Routed from existing `handleMarketStocks` by checking for the `/filing-summaries` suffix.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/market.go` | Add `FinancialSummary`, `PerformanceCommentary` to `FilingSummary`; add `QualityAssessment` struct; add `FilingSummaryPromptHash`, `QualityAssessment` to `MarketData` |
| `internal/services/market/service.go` | Remove inline summarization from `GetStockData` |
| `internal/services/market/filings.go` | Add prompt hash function; update prompt template with financial focus; update `filingSummaryRaw` parsing |
| `internal/services/market/collect.go` | Add prompt hash comparison in `CollectFilingSummaries` |
| `internal/services/market/quality.go` | **New** — quality assessment computation |
| `internal/common/version.go` | Bump `SchemaVersion` to "6" |
| `internal/server/routes.go` | Add filing-summaries route |
| `internal/server/handlers.go` | Add `handleFilingSummaries` handler |
