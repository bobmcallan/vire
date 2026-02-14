# SKS Technologies — Vire vs External Assessment (Post Cache Invalidation)

**Date:** 2026-02-14
**Version:** v0.3.15 + SchemaVersion 2 (cache invalidation sprint)
**Previous:** docs/stock-assessment-comparison-v2.md

## 1. Technical Profile (Layer 1) — Price & Fundamentals

| Metric | Vire | External | Match |
|--------|------|----------|-------|
| Price | $4.08 | $4.08 (StockAnalysis) | EXACT |
| Market Cap | $470.51M | $470.51M | EXACT |
| P/E Ratio | 31.38 | 32.67 (StockAnalysis) | Close |
| Forward P/E | NOT PROVIDED | 20.13 | **MISSING** |
| P/B Ratio | 20.09 | 19.17 | Close |
| EPS | $0.13 | $0.12 | Close |
| Dividend Yield | 1.42% | 1.42% | EXACT |
| Beta | 0.34 | 0.34 | EXACT |
| 52W High | $4.48 | $4.48 | EXACT |
| 52W Low | $1.39 | $1.385 | EXACT |
| RSI | 57.98 | 56.18 | Close |
| Volume | 104,653 | 104,653 | EXACT |

### Missing Fundamentals — Available Externally

| Metric | External Value | Source | Useful? |
|--------|---------------|--------|---------|
| Forward P/E | 20.13 | StockAnalysis | CRITICAL — trailing 31x vs forward 20x is the valuation story |
| ROE | 76.47% | StockAnalysis | HIGH — exceptional capital efficiency |
| ROA | 14.59% | StockAnalysis | MODERATE |
| Profit Margin | 5.36% | StockAnalysis | HIGH — thin-margin contractor context |
| Operating Margin | 7.59% | StockAnalysis | MODERATE |
| Gross Margin | 52.85% | StockAnalysis | MODERATE |
| Free Cash Flow | $32.58M | StockAnalysis | HIGH — real cash generation |
| FCF Yield | 6.92% | StockAnalysis | HIGH |
| Debt/Equity | 0.34 | StockAnalysis | MODERATE |
| Current Ratio | 1.20 | StockAnalysis | LOW |
| Revenue TTM | $261.66M | StockAnalysis | HIGH — strategy needs this |
| PEG Ratio | 0.81 | StockAnalysis | HIGH — growth at reasonable price |
| Next Earnings Date | Feb 24, 2026 | StockAnalysis | HIGH — 10 days away |

**NOTE:** Most of these are available from EODHD's `Highlights` API response but Vire only parses 4 of ~20 available fields. The `fundamentalsResponse.Highlights` struct currently parses: MarketCapitalization, PERatio, EarningsShare, DividendYield. EODHD also provides: ForwardPE, ProfitMargin, OperatingMarginTTM, ReturnOnEquityTTM, ReturnOnAssetsTTM, RevenueTTM, GrossProfitTTM, EBITDA, PEGRatio, etc.

**Layer 1 accuracy grade: 9/10** — core metrics exact. But missing high-value fields that are already available from the same API call.

**Improvement opportunity: 9 → 9.5/10** by parsing additional EODHD Highlights fields (no new API calls needed).

## 2. Company Releases (Layer 2) — Per-Filing Extracted Data

### Quality Assessment

| Filing | Date | Revenue | Profit | Guidance | Key Detail | Grade |
|--------|------|---------|--------|----------|------------|-------|
| FY25 Results Announcement | 2025-08-26 | $261.7M (+92%) | $14.0M net (+112%) | — | Contract: $130M (NEXTDC) | A |
| FY25 Results Presentation | 2025-08-26 | — | — | — | — | F |
| H1 FY26 Results Webinar | 2026-02-02 | — | — | — | — | F |
| Major Contract + FY26 Upgrade | 2026-02-05 | — | — | — | Contract: $130M (NEXTDC) | D |
| Delta Elcom Acquisition | 2025-11-18 | — | — | — | Headline only | F |
| Delta Elcom Complete | 2026-01-12 | — | — | — | Headline only | F |
| $130M data centre project | 2025-11-19 | — | — | — | Contract: $130M | C |
| AGM Presentation/Address | 2025-11-20 | — | — | — | — | F |
| Annual Report | 2025-09-19 | — | — | — | — | F |

### Critical Missing Data

The **Feb 5 "Major Contract Award and FY26 Profit Upgrade"** filing is the most important recent announcement. External sources show it contains:
- $60M in new contracts (NEXTDC M3 Stage 4 + EY Melbourne)
- Revenue guidance: $320M → $340M
- PBT guidance: $28.8M → $34M
- Margin improvement: 9% → 10%
- Work on hand: $325M

Vire extracted only "Contract: $130M (NEXTDC)" — which is from the older Nov 2025 filing, not the Feb 5 content.

The **Delta Elcom acquisition** ($13.75-15M, ~$25M revenue business) has no financial details extracted at all.

Only 1 of 15 displayed filings has meaningful financial extraction (FY25 Results Announcement). The rest are classified but have empty data fields.

**Layer 2 accuracy grade: 5/10** — FY25 results correctly extracted, but 14 of 15 other filings have no financial data despite containing material information. The Gemini extraction prompts or PDF text are not producing structured data for most filings.

**Change from v2: 7 → 5** — Schema purge re-analyzed all filings but extraction quality didn't improve for non-financial-results filings.

## 3. Company Timeline (Layer 3) — Structured Summary

| Field | Vire | External | Correct? |
|-------|------|----------|----------|
| Business Model | Generic electrical/data centres description | Data centre pivot = 53.8% of revenue | WEAK |
| FY2025 Revenue | $261.7M (+92%) | $261.66M (+91.96%) | YES |
| FY2025 Profit | $14.0M net (+112%) | $14.03M (+111.8%) | YES |
| FY2024 | EMPTY | $136.3M revenue, $6.6M profit | **MISSING** |
| FY2023 | EMPTY | $83.3M revenue, $0.6M profit | **MISSING** |
| FY2022 | NOT SHOWN | $67.3M revenue, $3.0M profit | **MISSING** |
| FY2026 Guidance | $340M rev / $34M PBT | $340M rev / $34M PBT | YES |
| Work on Hand | $560M | $325M (Feb 5 filing) | **WRONG** |
| Next Reporting Date | NOT PROVIDED | Feb 24, 2026 | **MISSING** |
| Repeat Business Rate | NOT PROVIDED | 94% | **MISSING** |
| Key Events | 6 events, correctly classified | All verified | YES |

**Layer 3 accuracy grade: 6/10** — FY25 data and guidance correct. Key events timeline good. But FY22-24 history empty, work on hand figure wrong ($560M vs $325M), and operational metrics not populated.

**Change from v2: 6.5 → 6** — Work on hand still incorrect, multi-year history still empty.

## 4. Claude Desktop Regression Assessment

The Claude Desktop feedback reported "Company Filings Intelligence" section completely missing. Analysis:

| Section | Old (FilingsIntelligence) | New (3-Layer) | Status |
|---------|--------------------------|---------------|--------|
| Financial health | Was narrative summary | Now per-filing structured data | **PRESENT** but quality poor |
| Growth outlook | Was narrative | Now in Company Timeline | **PRESENT** |
| Key metrics | Were embedded in prose | Now in FilingSummary fields | **PRESENT** but mostly empty |
| Year-over-year | Was narrative comparison | Now in Timeline periods | **PRESENT** but FY23/24 empty |
| Strategy analysis | Was included | Intentionally removed | **BY DESIGN** |

The sections ARE present in the MCP output. The Claude Desktop test likely ran before the schema purge re-collected data, or the user was comparing against the old narrative format which was more visible/readable even though it lacked specific numbers.

## Overall Comparison

| Area | v0.3.14 | v0.3.15 (v2) | v0.3.15 (v3) | Issue |
|------|---------|--------------|--------------|-------|
| Technical Profile | 9/10 | 9/10 | 9/10 | Stable. Missing EODHD fields already available. |
| Company Releases | 7/10 | 7/10 | 5/10 | Regression — extraction quality poor for non-financial-results filings |
| Company Timeline | 6/10 | 6.5/10 | 6/10 | Work on hand wrong, multi-year empty |
| **Overall** | **7.3/10** | **7.5/10** | **6.7/10** | **Net regression from v2** |

## Root Cause Analysis

### Why extraction quality is poor for most filings

The per-filing Gemini extraction processes each filing's PDF text. However:

1. **Many filings have no PDF text** — Announcements like "SKS Completes Delta Elcom Acquisition" may be short ASX releases without downloadable PDFs, or PDF download may have failed silently.

2. **Gemini prompt may not handle short-form announcements well** — The extraction prompt expects financial report format (revenue tables, profit statements). Short contract or acquisition announcements have different formats.

3. **Type classification vs data extraction mismatch** — Filings are correctly classified (e.g., "acquisition", "contract") but the structured fields (AcqPrice, ContractValue) are often empty. The Gemini prompt may not be asking the right questions for each filing type.

### Why multi-year history is empty

ASX filings typically go back ~2 years. FY2023 and FY2024 results are from 2023-08 and 2024-08 respectively. If the filing window doesn't cover these dates, the data simply isn't available from filings alone. However, EODHD's fundamentals API likely has historical income statement data that could fill this gap.

## Recommendations

### P0 — Expand EODHD Fundamentals Parsing (no new API calls)

Parse additional fields from the existing `Highlights` API response:
- ForwardPE, ProfitMargin, OperatingMarginTTM, ReturnOnEquityTTM, ReturnOnAssetsTTM
- RevenueTTM, GrossProfitTTM, EBITDA, PEGRatio
- WallStreetTargetPrice (if different from AnalystRatings.TargetPrice)

This is purely additive — same API call, just parse more fields.

### P1 — Fix Gemini Filing Extraction Quality

- Audit PDF text availability: log which filings have PDF text and which don't
- Adjust extraction prompt for different filing types (contract announcements need different questions than financial results)
- Consider using the filing headline + any available summary text when PDF is missing

### P2 — Backfill Historical Financials

- Use EODHD Income_Statement data (if available for ASX) to populate FY22-24 in CompanyTimeline
- Or use the fundamentals response which may contain historical data
