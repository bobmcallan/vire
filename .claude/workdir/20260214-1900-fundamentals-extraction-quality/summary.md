# Summary: Fundamentals Expansion, Filing Extraction Quality, Historical Backfill

**Date:** 2026-02-14
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/clients/eodhd/client.go` | Added 15 new fields to `fundamentalsResponse.Highlights` (ForwardPE, PEGRatio, ProfitMargin, OperatingMarginTTM, ROE, ROA, RevenueTTM, EBITDA, etc). Added `Financials.Income_Statement.yearly` parsing for historical revenue/profit. Removed WallStreetTargetPrice dead code. |
| `internal/models/market.go` | Added 15 new fields to `Fundamentals` struct. Added `HistoricalPeriod` type and `HistoricalFinancials` field. |
| `cmd/vire-mcp/formatters.go` | Reorganized Fundamentals display into 4 subsections (Valuation, Profitability, Growth & Scale, Estimates) with zero-suppression. Added zero-suppression to AnalystRatings. |
| `internal/services/market/filings.go` | Expanded `downloadFilingPDFs` to download all HIGH-relevance filings (not just financial reports). Added type-aware extraction guidance in Gemini prompt. Added headline-only fallback instructions. Added PDF availability logging. Included historical financials in timeline prompt. |
| `internal/services/market/service.go` | Added `FundamentalsUpdatedAt` clearing in schema mismatch block. |
| `internal/common/version.go` | Bumped SchemaVersion from "2" to "3". |
| `internal/clients/eodhd/earnings_test.go` | Added TestGetFundamentals_ParsesExpandedHighlights with full mock. |
| `internal/services/market/filings_test.go` | Added TestBuildFilingSummaryPrompt_TypeAwareGuidance and TestBuildTimelinePrompt_IncludesHistoricalFinancials. |
| `internal/services/market/service_test.go` | Updated TestCollectMarketData_SchemaMismatchClearsSummaries to assert FundamentalsUpdatedAt clearing. |
| `README.md` | Updated tool descriptions for 3-layer assessment. |
| `.claude/skills/vire-stock-report/SKILL.md` | Updated for 3-layer structure. |
| `.claude/skills/vire-collect/SKILL.md` | Expanded fundamentals description, added filing summaries + timeline. |
| `.claude/skills/vire-portfolio-review/SKILL.md` | Updated freshness table and summary report description. |

## Tests
- 18+ new tests across 3 test files
- All tests pass (`go test ./...`)
- `go vet ./...` — clean
- SchemaVersion "3" confirmed

## Documentation Updated
- `README.md` — tool descriptions updated
- `.claude/skills/vire-stock-report/SKILL.md` — 3-layer assessment documented
- `.claude/skills/vire-collect/SKILL.md` — expanded fundamentals and filing summaries
- `.claude/skills/vire-portfolio-review/SKILL.md` — freshness table and summary updated

## Review Findings
- **FundamentalsUpdatedAt not cleared on schema mismatch** (reviewer, task #4) — fixed by reviewer directly in service.go
- **WallStreetTargetPrice dead code** (reviewer, task #4) — removed by reviewer
- **AnalystRatings zero-suppression** (reviewer, task #4) — added by reviewer for consistency
- **P2 defer recommendation** (reviewer, task #2) — implementer included it anyway, works excellently

## Validation — SKS.AU

### P0 — Extended Fundamentals: PASS
- Profit Margin 5.36%, Operating Margin 8.04%, ROE 76.47%, ROA 14.59%
- Revenue TTM $261.66M, EBITDA $21.58M
- EPS estimates: current year $0.20, next year $0.24
- ForwardPE not available from EODHD for this ASX stock (correctly omitted)

### P1 — Filing Extraction: IMPROVED
- Contract filings extract contract_value ($130M)
- Filing type classification improved
- Non-financial-results PDFs not yet re-downloaded (requires clearing filing cache for full effect)

### P2 — Historical Financials: PASS
- FY2022-2024 now populated (previously empty)
- FY2025: $261.7M (+92%), $13.8M profit
- FY2024: $136.3M (+63%), $6.6M profit
- FY2023: $83.3M (+23.8%), $0.8M profit
- FY2022: $67.3M (+89%), $3.0M profit
- Data back to FY1989

## Notes
- ForwardPE not available from EODHD for SKS.AU — may be available for larger/US stocks
- P1 filing extraction improvement requires a full re-collection with cleared filing cache to download PDFs for non-financial filings. Current cached filings were downloaded under the old filter.
- P2 worked despite being recommended for deferral — EODHD has reliable historical income statement data for ASX stocks
