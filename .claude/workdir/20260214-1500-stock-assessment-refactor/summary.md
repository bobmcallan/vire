# Summary: Stock Assessment Refactor — 3-Layer Architecture

**Date:** 2026-02-14
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/market.go` | Removed broken v0.3.14 types (EarningsRecord, UpcomingEvent, ForecastTracking, ForecastEntry, EPS estimate fields). Added FilingSummary, CompanyTimeline, PeriodSummary, TimelineEvent types. Extended MarketData/StockData with new fields. Removed CanSupport10PctPA and StrategyNotes from FilingsIntelligence. |
| `internal/models/portfolio.go` | Updated HoldingReview: removed EarningsHistory/UpcomingEvents, added FilingSummaries/Timeline |
| `internal/services/market/filings.go` | Major refactor: replaced generateFilingsIntelligence with summarizeNewFilings (incremental per-filing Gemini extraction) and generateCompanyTimeline (structured timeline). Dedup key uses date+headline hash. |
| `internal/services/market/service.go` | Updated CollectMarketData: removed UpcomingEvents block, added incremental filing summarization and timeline generation. Updated GetStockData to populate new fields. |
| `internal/clients/eodhd/client.go` | Removed GetEarningsCalendar method, earningsCalendarEntry/earningsCalendarResponse types, EPS estimate parsing, Earnings.History extraction. Kept AnalystRatings and flexString/flexFloat64. |
| `internal/interfaces/clients.go` | Removed GetEarningsCalendar from EODHDClient interface |
| `internal/common/freshness.go` | Removed FreshnessUpcomingEvents, FreshnessFilingsIntel, FreshnessEarnings. Added FreshnessTimeline (7 days). |
| `cmd/vire-mcp/formatters.go` | Added formatCompanyReleases and formatCompanyTimeline. Removed old earnings/calendar sections. Moved Analyst Consensus after Fundamentals. |
| `internal/services/market/filings_test.go` | Replaced all old tests with 9 new tests for per-filing extraction parsers and helpers |
| `internal/services/market/service_test.go` | Removed TestGetStockData_SurfacesEarningsData, added TestGetStockData_SurfacesFilingSummaries, removed GetEarningsCalendar from mocks |
| `internal/clients/eodhd/earnings_test.go` | Removed GetEarningsCalendar tests, kept AnalystRatings tests |
| `internal/services/portfolio/service_test.go` | Removed GetEarningsCalendar from mock client |
| `internal/services/quote/service_test.go` | Removed GetEarningsCalendar from stub client |
| `README.md` | Updated feature descriptions and tool tables to reference 3-layer architecture instead of broken v0.3.14 features |
| `.claude/skills/vire-stock-report/SKILL.md` | Updated to document 3-layer assessment (company releases, company timeline, analyst consensus) |
| `.claude/skills/vire-collect/SKILL.md` | Updated to document filing summaries and company timeline in collected data |
| `.claude/skills/vire-portfolio-review/SKILL.md` | Updated to reference per-filing summaries and company timeline |

## Tests
- 9 new tests added in `filings_test.go` for per-filing extraction parsing
- 1 new test in `service_test.go` for FilingSummaries surfacing in StockData
- All existing tests updated to remove broken v0.3.14 interfaces
- `go test ./...` — PASS
- `go vet ./...` — PASS

## Documentation Updated
- `README.md` — Lines 14, 29, 39 updated from "earnings history, analyst ratings, upcoming events" to 3-layer references
- `.claude/skills/vire-stock-report/SKILL.md` — Documented 3-layer report structure
- `.claude/skills/vire-collect/SKILL.md` — Documented filing summaries and company timeline in output
- `.claude/skills/vire-portfolio-review/SKILL.md` — Documented per-filing summaries and company timeline

## Review Findings
- Reviewer raised: per-filing extraction needs actual PDF text, not just headlines — implemented (up to 15K chars per filing)
- Reviewer raised: dedup should use date+headline hash not DocumentKey — implemented
- Reviewer raised: limit Company Releases to 15-20 in MCP response — implemented (capped at 15)
- Reviewer raised: remove CanSupport10PctPA and StrategyNotes — implemented
- Reviewer raised: remove dead FreshnessFilingsIntel constant — implemented
- Reviewer raised: move Analyst Consensus after Fundamentals — implemented

## Validation — SKS.AU Test

- **Layer 1 (Technical Profile)**: PASS — price, fundamentals, signals all correct
- **Layer 2 (Company Releases)**: PASS — FY2025 Revenue $261.7M (+92%), Profit $14.0M (+112%), $130M NEXTDC contract, Delta Elcom acquisition $13.75-15M all correctly extracted
- **Layer 3 (Company Timeline)**: PASS — guidance $340M/$34M captured, work on hand $560M, business model and period summaries present
- **Broken v0.3.14 features**: REMOVED — no earnings history, calendar, or upcoming events in output

## Notes
- FY2023/FY2024 financial data not present in Company Releases — this is a data limitation (ASX filings window doesn't go back that far), not a code bug. Timeline may fill this from fundamentals data when available.
- FilingsIntelligence retained as deprecated for backward compatibility — existing cached data still loads
- Per-filing Gemini extraction sends batches of 5 filings with PDF text (up to 15K chars each)
- Company Timeline rebuilds when new summaries are added or every 7 days
