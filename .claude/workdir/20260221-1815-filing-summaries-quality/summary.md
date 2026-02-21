# Summary: Filing Summary Caching, Financial Focus & Quality Assessment

**Date:** 2026-02-21
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/market.go` | Added `FinancialSummary`, `PerformanceCommentary` to `FilingSummary`; added `QualityAssessment` and `QualityMetric` structs; added `QualityAssessment` and `FilingSummaryPromptHash` fields to `MarketData` and `StockData` |
| `internal/services/market/service.go` | Removed inline `summarizeNewFilings` and `generateCompanyTimeline` from `GetStockData` — now serves stored data only; added quality assessment computation on read |
| `internal/services/market/filings.go` | Added `filingSummaryPromptHash()` and `filingSummaryPromptTemplate` constant; updated prompt with `financial_summary` and `performance_commentary` fields; updated `filingSummaryRaw` parsing |
| `internal/services/market/collect.go` | Added prompt hash comparison in `CollectFilingSummaries` — auto-regenerates when prompt changes |
| `internal/services/market/quality.go` | **New** — `computeQualityAssessment()` scoring ROE, gross margin, FCF conversion, debt/EBITDA, earnings stability, revenue growth, margin trend with red flags and overall rating |
| `internal/services/market/quality_test.go` | **New** — unit tests for quality assessment computation |
| `internal/common/version.go` | Bumped `SchemaVersion` from "5" to "6" |
| `internal/server/routes.go` | Changed `handleMarketStocks` to `routeMarketStocks` with sub-route dispatch for `/filing-summaries` |
| `internal/server/handlers.go` | Added `handleFilingSummaries` handler returning filing summaries + quality assessment |
| `README.md` | Documented new endpoint and quality assessment |
| `.claude/skills/develop/SKILL.md` | Updated reference section with new structs, endpoint, schema version |

## Tests
- `quality_test.go` — tests for quality assessment computation (high quality, below average, nil/empty fundamentals edge cases)
- `service_test.go` — fixed pre-existing test timeouts by adding `FilingsUpdatedAt` to prevent auto-collect triggering ASX network calls
- All unit tests pass (`go test ./internal/...` — 0.220s)
- `go vet ./...` clean
- Build succeeds

## Key Fixes

### 1. Caching Bug Fixed
`GetStockData` no longer calls `summarizeNewFilings` or `generateCompanyTimeline` inline. Filing summaries are served from storage only. The job manager handles all generation via `CollectFilingSummaries`. This eliminates:
- Race condition between GetStockData and job manager
- Context cancellation mid-batch causing lost work
- Redundant Gemini API calls on every deep dive

### 2. Prompt Versioning
A SHA-256 hash of the prompt template is stored as `FilingSummaryPromptHash` on `MarketData`. When `CollectFilingSummaries` detects a prompt change, it clears existing summaries and regenerates. This ensures summaries stay current when instructions are updated.

### 3. Financial Performance Focus
Each `FilingSummary` now includes:
- `financial_summary` — one-line financial performance summary with key numbers
- `performance_commentary` — significant management commentary on financial performance

### 4. Quality Company Assessment
New `QualityAssessment` computed from EODHD fundamentals:
- **Metrics:** ROE (vs 15%), Gross Margin (vs 35%), Revenue Growth, Earnings Stability, Margin Trend
- **Red flags:** Negative ROE, thin margins, revenue decline, negative earnings growth
- **Rating bands:** High Quality (80-100), Quality (60-79), Average (40-59), Below Average (20-39), Speculative (0-19)
- Stored on `MarketData`, served via API

### 5. New API Endpoint
`GET /api/market/stocks/{ticker}/filing-summaries` returns:
```json
{
  "ticker": "SRG.AU",
  "filing_summaries": [...],
  "quality_assessment": {...},
  "summary_count": 58,
  "last_updated": "2026-02-21T..."
}
```

## Documentation Updated
- README.md — new endpoint documented
- .claude/skills/develop/SKILL.md — updated reference section

## Devils-Advocate Findings
- Route double-suffix edge case (e.g. `/filing-summaries/filing-summaries`) — handled by route dispatch, returns 404
- Test timeouts from auto-collect filings condition — fixed by setting `FilingsUpdatedAt`
- `DataVersionStampedOnSave` test missing `FilingsUpdatedAt` — fixed
- Division-by-zero in quality assessment with zero revenue — handled with nil checks

## Notes
- `golangci-lint` couldn't run (built with Go 1.24, project uses Go 1.25)
- Server startup requires SurrealDB — couldn't test live endpoint in this environment
- Schema version bumped to "6" — existing derived data will be cleared on first load
