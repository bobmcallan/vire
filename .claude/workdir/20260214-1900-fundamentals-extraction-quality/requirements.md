# Requirements: Fundamentals Expansion, Filing Extraction Quality, Historical Backfill

**Date:** 2026-02-14
**Requested:** Fix three data quality gaps identified in SKS.AU comparison assessment (docs/stock-assessment-comparison-v3.md). Prioritized P0 → P1 → P2.

## Context

The 3-layer stock assessment architecture is structurally sound but the data quality within it is poor:
- Fundamentals only parse 4 of ~20 available EODHD fields
- Per-filing Gemini extraction produced meaningful data for only 1 of 15 displayed filings
- Company Timeline has no historical data for FY2022-2024

## P0 — Expand EODHD Fundamentals Parsing

**Problem:** The `fundamentalsResponse.Highlights` struct in `internal/clients/eodhd/client.go` only parses MarketCapitalization, PERatio, EarningsShare, DividendYield. EODHD returns many more fields in the same API response.

**Solution:** Parse additional fields from the existing API response (no new API calls):

| Field | EODHD JSON Key | Why Critical |
|-------|----------------|-------------|
| Forward P/E | `Highlights.ForwardPE` | Trailing 31x vs forward 20x is THE valuation story |
| Profit Margin | `Highlights.ProfitMargin` | Thin-margin contractor context (5.36%) |
| Operating Margin | `Highlights.OperatingMarginTTM` | Core operating efficiency |
| Gross Margin | `Highlights.GrossProfitTTM` / Revenue | Gross margin trend |
| ROE | `Highlights.ReturnOnEquityTTM` | Capital efficiency (76% for SKS) |
| ROA | `Highlights.ReturnOnAssetsTTM` | Asset efficiency |
| Revenue TTM | `Highlights.RevenueTTM` | Strategy compliance needs this |
| EBITDA | `Highlights.EBITDA` | Cash earnings proxy |
| PEG Ratio | `Highlights.PEGRatio` | Growth-at-reasonable-price metric |
| FCF (derived) | `Highlights.OperatingCashFlowTTM` - CapEx | Free cash flow |
| Target Price | `Highlights.WallStreetTargetPrice` | Analyst consensus (may overlap AnalystRatings) |
| Next Earnings | `Highlights.MostRecentQuarter` or `General.UpdatedAt` | Upcoming catalyst |

**Changes required:**
1. `internal/clients/eodhd/client.go` — Expand `fundamentalsResponse.Highlights` and `Valuation` structs to parse new fields
2. `internal/models/market.go` — Add new fields to `Fundamentals` struct
3. `cmd/vire-mcp/formatters.go` — Display new fields in Fundamentals table (only show non-zero values)
4. Tests — Verify new fields parsed correctly

**Approach:** Add fields to the Fundamentals model in a group called "Extended" to keep the existing basic fields stable. Display conditionally — only show non-zero/non-empty values.

## P1 — Fix Gemini Filing Extraction Quality

**Problem:** Per-filing Gemini extraction only produced data for the FY25 Results Announcement. 14 of 15 other displayed filings (including the critical Feb 5 guidance upgrade) have empty revenue/profit/guidance fields.

**Root causes to investigate:**
1. PDF text may not be available for many filings (download failures, short ASX releases without PDFs)
2. Gemini prompt may be optimized for financial results format but not contract/acquisition announcements
3. Filing headline/type may need to influence the extraction prompt

**Solution:**
1. **Audit PDF text availability** — In `summarizeNewFilings`, log how many filings have PDF text vs just headline
2. **Use headline + filing metadata when no PDF** — For filings without PDF text, send the headline, date, type, and any available summary text to Gemini instead of skipping
3. **Type-aware extraction prompt** — Adjust the Gemini prompt to ask different questions based on filing type (contract: ask for value/customer; acquisition: ask for price/target/revenue; guidance: ask for forward estimates)
4. **Ensure PDF download success** — Check `downloadFilingPDFs` for silent failures, especially for ASX.com.au announcement URLs

**Changes required:**
1. `internal/services/market/filings.go` — Improve extraction prompt, handle missing PDF text, add logging
2. Potentially `internal/services/market/filings.go` PDF download functions — fix silent failures

## P2 — Backfill Historical Financials in Company Timeline

**Problem:** CompanyTimeline shows FY2025 data but FY2022-2024 are empty. ASX filings don't go back far enough, but EODHD may have historical income statement data.

**Solution:**
1. **Check EODHD fundamentals response** for historical financial data (Income_Statement, Balance_Sheet sections)
2. **Pass historical fundamentals to `generateCompanyTimeline`** — If EODHD provides yearly revenue/profit for prior years, include it in the Gemini prompt so the timeline can be populated
3. **Alternative:** Parse EODHD's `Financials.Income_Statement.yearly` data directly into PeriodSummary entries without Gemini

**Changes required:**
1. `internal/clients/eodhd/client.go` — Parse Income_Statement yearly data from fundamentals response
2. `internal/models/market.go` — Add historical financials storage (or pass through to timeline generation)
3. `internal/services/market/filings.go` — Include historical data in timeline generation prompt
4. `cmd/vire-mcp/formatters.go` — No change needed (timeline format already handles multiple periods)

## Files Expected to Change

### P0
- `internal/clients/eodhd/client.go` — Expand Highlights/Valuation parsing
- `internal/models/market.go` — Add fields to Fundamentals
- `cmd/vire-mcp/formatters.go` — Display new fundamentals
- `internal/clients/eodhd/realtime_test.go` or new test file — Test parsing

### P1
- `internal/services/market/filings.go` — Improve extraction prompt, handle missing PDFs, add logging

### P2
- `internal/clients/eodhd/client.go` — Parse Income_Statement
- `internal/models/market.go` — Historical financials model
- `internal/services/market/filings.go` — Include in timeline generation

## Scope

### In Scope
- All three priorities (P0, P1, P2)
- Bump SchemaVersion to "3" after changes (triggers re-collection of fundamentals with new fields)
- Update MCP formatter for new fundamentals fields

### Out of Scope
- New API integrations beyond EODHD
- Changing freshness TTLs
- Strategy alignment logic
