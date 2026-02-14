# SKS Technologies — Vire vs External Assessment (Post-Refactor)

**Date:** 2026-02-14
**Version:** v0.3.15 (3-layer stock assessment refactor)
**Previous:** docs/stock-history-feature-review.md (v0.3.14 — pre-refactor)

## 1. Technical Profile (Layer 1) — Price & Fundamentals

| Metric | Vire | StockAnalysis | Web Search | Match |
|--------|------|---------------|------------|-------|
| Price | $4.08 | $4.08 | $4.11 | Exact (vs StockAnalysis) |
| Market Cap | $487.81M | $470.51M | $471-481M | Close (~3% variance, share count diff) |
| P/E Ratio | 35.25 | 32.67 | 20.90 | Mixed — see note |
| Forward P/E | — | 20.13 | — | Not provided by Vire |
| EPS | $0.12 | $0.12 | $0.12 | Exact |
| Dividend Yield | 1.43% | 1.42% | — | Exact |
| Beta | 0.34 | 0.34 | — | Exact |
| 52W High | $4.48 | $4.48 | $4.24 | Exact (vs StockAnalysis) |
| 52W Low | $1.39 | $1.385 | $1.35 | Exact |
| RSI | 57.98 | 56.18 | — | Close (calculation window diff) |
| Volume | 104,653 | 104,653 | — | Exact |
| Avg Volume | 275,484 | 275,484 | — | Exact |

**P/E note:** Vire reports 35.25 (trailing, from EODHD fundamentals). StockAnalysis reports 32.67 trailing and 20.13 forward. Web search returned 20.90 (likely a forward or blended figure). The trailing P/E variance between Vire and StockAnalysis is likely due to different earnings dates or share counts in the denominator.

**Missing from Vire:** Forward P/E. This would be valuable — the gap between trailing (35x) and forward (20x) is the key valuation story for SKS.

**Layer 1 accuracy grade: 9/10** — all core metrics match within normal variance. Forward P/E is the only meaningful gap.

## 2. Company Releases (Layer 2) — Per-Filing Extracted Data

### What Vire extracted (from 49 analyzed filings):

| Date | Filing | Extracted Numbers |
|------|--------|-------------------|
| 2025-08-26 | FY25 Results Announcement | Revenue $261.7M (+92%), Profit $14.0M net (+112%), Contract $130M (NEXTDC) |
| 2025-08-26 | FY25 Results Presentation | Revenue $261.7M (+92%), Profit $14.0M net (+112%), Contract $130M (NEXTDC) |
| 2025-11-19 | $130M data centre project | Contract: $130M |
| 2025-11-18 | Delta Elcom acquisition | Identified as acquisition |
| 2025-12-18 | Delta Execute Sale Agreement | Identified as business_change |
| 2026-01-12 | Completes Delta Elcom Acquisition | Identified as acquisition |
| 2026-02-02 | H1 FY26 Results Presentation Webinar | Identified as half_year_report |
| 2026-02-05 | Major Contract Award + FY26 Profit Upgrade | Identified as contract |

### What external sources report for the same period:

| Date | Event | Key Numbers (External) | Vire Captured? |
|------|-------|----------------------|----------------|
| 2025-08-26 | FY25 Results | Revenue $261.7M (+92%), Net profit $14.0M (+112%), Data centre rev $140.7M (53.8%), Work on hand $200M, 94% repeat business | Revenue/profit: YES. Data centre breakdown: NO. Work on hand: YES (in timeline). Repeat business: NO in releases |
| 2025-11-19 | $130M data centre project | $130M NEXTDC contract | YES |
| 2025-11-18 | Delta Elcom acquisition | $13.75-15M price, ~$25M revenue business, NSW data centre market entry | Identified as acquisition, but NO price/revenue extracted |
| 2026-01-12 | Acquisition completed | 612,501 new shares, $10.5M cash, $2M shares | Identified, but NO financial details extracted |
| 2026-02-05 | Contract award + guidance upgrade | $60M new contracts (NEXTDC M3 Stage 4 + EY Melbourne), Revenue guidance $320M→$340M, PBT $28.8M→$34M, Margin 9%→10%, Work on hand $325M | Identified as "contract" but financial details NOT extracted in releases table |

### Assessment

The FY25 results are well-extracted with actual revenue and profit numbers. However, the more recent filings (Nov 2025 — Feb 2026) are identified and classified correctly but **lack extracted financial detail** in the releases table. The Feb 5 filing — arguably the most important — shows no revenue, profit, or key detail in the structured fields despite containing $340M revenue guidance, $34M PBT, and $325M work on hand.

This is likely because:
1. These filings may have been analyzed before the 3-layer refactor was deployed (cached from v0.3.14)
2. The PDF text extraction may not have captured the numbers from these specific announcement formats

The Delta Elcom acquisition price ($13.75-15M) and target revenue ($25M) are also missing — these are material facts for assessing the acquisition.

**Layer 2 accuracy grade: 7/10** — FY25 financials correctly extracted. Recent filings correctly identified and classified. But the most important recent filing (Feb 5 guidance upgrade) has no financial numbers extracted. Delta Elcom acquisition terms missing.

**Improvement from v0.3.14: +1 point** — v0.3.14 scored 7/10 for price-sensitive info. The structured per-filing approach now surfaces FY25 numbers that were previously buried in narrative, but recent filings still lack detail.

## 3. Company Timeline (Layer 3) — Structured Summary

### What Vire provides:

| Field | Vire Output | External Verification | Correct? |
|-------|-------------|----------------------|----------|
| Business Model | Electrical equipment, project contracts, data centres | Correct — design/supply/install, heavily pivoted to DC | YES (but generic) |
| FY2025 Revenue | $261.7M | $261.66M (StockAnalysis) | YES |
| FY2025 Growth | +92% | +91.96% (StockAnalysis) | YES |
| FY2025 Profit | $14.0M net | $14.03M (StockAnalysis) | YES |
| FY2025 Profit Growth | +112% | +111.8% (StockAnalysis) | YES |
| FY2024 | Empty | $136.3M revenue, $6.6M profit | MISSING |
| FY2023 | Empty | $83.3M revenue, $0.6M profit | MISSING |
| FY2025 Guidance | Rev $340M / Profit $34M PBT | Confirmed (Feb 5 announcement) | YES — but this is FY26 guidance, mislabeled as FY2025 |
| FY2024 Guidance | "FY 25 Revenue Forecast" | — | Vague |
| Work on Hand | $560M | $325M (Feb 5), $200M (FY25 end) | INCONSISTENT — $560M not verified |
| Key Events | 4 events (acquisition + contract) | All confirmed | YES |
| Next Reporting Date | Not provided | Feb 24, 2026 (StockAnalysis) | MISSING |
| Repeat Business Rate | Not provided | 94% (FY25) | MISSING |

### Assessment

The timeline correctly captures FY2025 financials and the key events sequence (Delta Elcom acquisition, contract awards). The guidance figures ($340M/$34M) are present, which is a major improvement over v0.3.14.

**Issues:**
1. **FY2024 and FY2023 are empty** — external sources show $136.3M and $83.3M revenue respectively. This multi-year trajectory is critical for assessing growth sustainability.
2. **Guidance mislabeled** — "$340M/$34M" is FY2026 guidance (given during FY25), not FY2025 guidance. The label says "FY2025 Guidance" but the numbers are for FY2026.
3. **Work on hand inconsistency** — timeline says $560M but the most recent external figure is $325M (Feb 5, 2026). The $560M may be a cumulative/lifetime figure or an extraction error.
4. **Missing operational metrics** — repeat business rate (94%) and next reporting date (Feb 24) are fields in the model but not populated.
5. **Business model description is generic** — doesn't mention data centre pivot (53.8% of revenue), which is the key strategic narrative.

**Layer 3 accuracy grade: 6.5/10** — FY25 data correct, guidance captured (mislabeled), key events accurate. But missing FY23/24 history, work on hand inconsistent, operational metrics empty, business model too generic.

## Overall Comparison Summary

| Area | v0.3.14 Grade | v0.3.15 Grade | Change |
|------|--------------|--------------|--------|
| Technical Profile (Layer 1) | 9/10 | 9/10 | — (was already strong) |
| Company Releases (Layer 2) | 7/10 | 7/10 | Structural improvement (per-filing vs blob), but recent filings still lack detail |
| Company Timeline (Layer 3) | 6/10 | 6.5/10 | +0.5 — guidance figures now captured, structured format better for LLM consumption |
| **Overall** | **7.3/10** | **7.5/10** | **+0.2** |

## What the LLM Now Gets vs Needs

### The LLM now receives (improvement):
- Structured per-filing data with extracted numbers for FY25
- Guidance figures ($340M/$34M) in timeline
- Clear event timeline with impact classification
- Separate technical/releases/timeline sections for targeted reasoning

### The LLM still needs (gaps):
1. **Multi-year financials** — FY23/24 data is critical for growth trajectory assessment. The filings window doesn't go back far enough. Consider using EODHD Income_Statement or external sources to backfill.
2. **Recent filing detail extraction** — The Feb 5 guidance upgrade has no numbers in the releases table. This may be a stale cache issue or PDF extraction gap.
3. **Forward P/E** — The trailing vs forward P/E gap (35x vs 20x) is the central valuation question. Vire should provide forward P/E when analyst estimates exist.
4. **Accurate work on hand** — $560M vs $325M needs investigation.
5. **Next reporting date** — Feb 24, 2026 should be populated.

## Recommendation

The 3-layer architecture is structurally sound and better organized for LLM consumption than v0.3.14. The per-filing extraction approach is correct in design. The gaps are data quality issues, not architectural ones:

1. **Force re-analyze recent filings** — the Nov 2025 - Feb 2026 filings may have been cached pre-refactor without structured extraction. A forced re-collection should populate the missing fields.
2. **Backfill historical financials** — consider supplementing ASX filing data with EODHD Income_Statement data for FY22-FY24, or extending the filings window.
3. **Fix guidance labeling** — the period label should indicate which period the guidance is FOR, not which period it was given IN.
4. **Investigate $560M work on hand** — verify source and correct if extraction error.
