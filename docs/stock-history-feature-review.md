# Stock History Feature Review: SKS.AU Validation

**Date:** 2026-02-14
**Version:** 0.3.14
**Feature:** Earnings History & Forecast Tracking
**Test Ticker:** SKS Technologies Group (ASX:SKS)

## Methodology

Vire `get_stock_data` output for SKS.AU was compared against external sources:
- [StockAnalysis.com](https://stockanalysis.com/quote/asx/SKS/)
- [Yahoo Finance](https://finance.yahoo.com/quote/SKS.AX/)
- [Simply Wall St](https://simplywall.st/stocks/au/capital-goods/asx-sks/sks-technologies-group-shares)
- [ASX Company Page](https://www.asx.com.au/markets/company/SKS)

---

## Price Data — Grade: A

| Metric | Vire | StockAnalysis | Yahoo | Verdict |
|--------|------|---------------|-------|---------|
| Current Price | $4.08 | $4.08 | $3.92* | Match |
| Open | $4.15 | — | $3.87* | — |
| High | $4.23 | — | $3.99* | — |
| Low | $4.04 | — | $3.70* | — |
| Volume | 104,653 | 104,653 | 320,232* | Match |
| 52-Week High | $4.48 | $4.48 | $4.48 | Match |
| 52-Week Low | $1.39 | $1.385 | $1.385 | Match |
| Avg Volume | 275,484 | 275,484 | — | Match |

*Yahoo showed a different trading day's data. Vire and StockAnalysis align on Feb 13, 2026 close.

All price data is accurate and consistent with external sources.

---

## Fundamentals — Grade: B+

| Metric | Vire | External | Verdict |
|--------|------|----------|---------|
| Market Cap | $487.81M | $452-470M | High (~5-8% above external) |
| P/E Ratio | 35.25 | 32.67 | Differs — likely different earnings basis |
| EPS | $0.12 | $0.12 | Match |
| Dividend Yield | 1.43% | 1.42% | Match |
| Beta | 0.34 | 0.34 | Match |
| Sector | Industrials | Industrials | Match |
| Industry | Electrical Equipment & Parts | Electrical Equipment & Parts | Match |
| P/B Ratio | 20.28 | — | No external comparison |

### Notes

- **Market Cap discrepancy**: Vire computes shares x current price, while external sources may use trailing or diluted share counts. The ~$20M difference is within acceptable bounds.
- **P/E discrepancy**: Vire shows 35.25 vs external 32.67. EODHD may use a different EPS basis (diluted vs basic, or different trailing period). This warrants investigation — a 7.5% variance on P/E affects valuation assessments.

---

## Technical Signals — Grade: A-

| Signal | Vire | External | Verdict |
|--------|------|----------|---------|
| RSI | 57.98 | 56.18 (StockAnalysis) | Close — minor window difference |
| Trend | Neutral | — | Reasonable |
| SMA20 | $3.83 (+6.5%) | — | — |
| SMA50 | $3.93 (+3.8%) | — | — |
| SMA200 | $3.06 (+33.4%) | — | — |
| MACD | 0.0992 (positive) | — | — |
| Volume | 0.38x (low) | — | Consistent with 104K vs 275K avg |

### Risk Flags

- `high_volatility` — ATR 5.47%, correctly flagged (threshold: >5%)
- `extended_from_mean` — 33% above SMA200, correctly flagged (threshold: >20%)

Both flags are appropriate for a stock that has nearly tripled in the past year.

---

## Filings Intelligence — Grade: B+

**68 filings analyzed** | Financial Health: stable | Growth Outlook: positive

### Key Events Correctly Identified

| Event | Date | Vire Captured | External Confirmed |
|-------|------|---------------|-------------------|
| Major Contract Award + FY26 Profit Upgrade | 2026-02-05 | Yes (HIGH relevance) | Yes |
| Half Year Results Webinar | 2026-02-02 | Yes (HIGH) | Yes |
| Delta Elcom Acquisition Complete | 2026-01-12 | Yes (HIGH) | Yes |
| Delta Sale Agreement Execution | 2025-12-18 | Yes (HIGH) | Yes |
| $130M Data Centre Project | 2025-11-19 | Yes (HIGH) | Yes |
| Delta Elcom Acquisition Announced | 2025-11-18 | Yes (HIGH) | Yes |
| AGM 2025 Presentation | 2025-11-20 | Yes (HIGH) | Yes |

### Strengths

- Correctly identifies data centre contracts as primary growth driver
- Correctly identifies Delta Elcom acquisition as strategic expansion
- 10% annual growth assessment ("yes") aligns with analyst "Strong Buy" consensus
- Risk factors (high P/E, contract dependency) are appropriate

### Gaps

- Revenue and profit figures not explicitly stated in intelligence summary — says "increased" but not $261.66M (+92%) or $14.03M (+112%)
- Dividend 10x increase ($0.01 to $0.06) not highlighted
- Forward P/E (20.13) not calculated or mentioned
- Year-over-year section lacks specific numbers, just "Likely increased"

---

## Earnings History — Grade: F (Not Present)

**Expected:** 2-3 years of quarterly EPS actual vs estimate data.
**Actual:** Section entirely absent from output.

External data confirms this data exists:
- FY2025 EPS missed analyst estimates by 2.4%
- Revenue $261.66M (+92% YoY)
- Net Income $14.03M (+112% YoY)
- Forward PE 20.13 (implies analyst estimates available)

### Possible Causes

1. **Stale cache**: Fundamentals data cached before v0.3.14 deployment. 7-day freshness TTL may not have expired, so `GetFundamentals` was not re-called with the new parsing logic.
2. **EODHD data gap**: The EODHD fundamentals endpoint may not return `Earnings.History` for small-cap ASX stocks. Needs verification with a raw API call.
3. **Parsing failure**: The `fundamentalsResponse` struct extension may not be matching the actual JSON structure returned by EODHD for AU-listed stocks.

### Recommended Fix

1. Force-refresh fundamentals for SKS.AU to test with fresh API data
2. Log the raw EODHD fundamentals response to verify `Earnings.History` presence
3. Test with a large-cap ASX stock (BHP.AU, CBA.AU) where EODHD is more likely to have full data

---

## Analyst Ratings — Grade: F (Not Present)

**Expected:** Consensus rating, target price, buy/hold/sell breakdown.
**Actual:** Section entirely absent from output.

External data confirms analyst coverage exists:
- Consensus: "Strong Buy"
- Target Prices: Low $3.85, Average $4.24, High $4.62
- Forward PE: 20.13

### Root Cause

Same as Earnings History — likely stale cache or EODHD data gap. The `AnalystRatings` section in the EODHD fundamentals response may be empty for SKS.AU.

---

## Upcoming Events — Grade: F (Critical Bug)

**Expected:** SKS-specific upcoming earnings dates.
**Actual:** Returned thousands of global earnings events across all exchanges (TSE, US, LSE, Frankfurt, etc.). None are for SKS.AU.

### Bug Details

The `GetEarningsCalendar` method calls EODHD `/calendar/earnings` but is not correctly filtering by the `symbols` parameter. The output contains entries for:
- Japanese stocks (TSE)
- US stocks (NYSE/NASDAQ)
- German stocks (Frankfurt/Xetra)
- Korean stocks (KQ/KO)
- Nordic stocks (OL/ST/HE)
- And many more

SKS's actual next earnings date (Feb 23-24, 2026 per StockAnalysis) does not appear.

### Impact

- Response size inflated to 2.2MB (vs typical ~50KB)
- Unusable data — consumers cannot find the ticker's events
- API cost wasted on irrelevant data

### Recommended Fix

1. Verify the EODHD `/calendar/earnings?symbols=SKS.AU` parameter is being passed correctly
2. Check if EODHD requires a different ticker format for the calendar endpoint (e.g., `SKS` without exchange suffix)
3. Add response filtering: if the API returns unrelated tickers, filter to only the requested symbol
4. Add a response size guard — if >100 events returned for a single ticker query, something is wrong

---

## Forecast Tracking — Grade: F (Not Present)

**Expected:** Gemini-generated forecast-vs-actual analysis in `FilingsIntelligence`.
**Actual:** No `forecast_tracking` or `upcoming_outlook` fields in the filings intelligence output.

### Possible Causes

1. **Stale filings intelligence cache**: 90-day TTL on `FilingsIntelligence`. If generated before v0.3.14, the old prompt (without forecast tracking sections) was used.
2. **Gemini prompt not triggering**: The enhanced prompt may not include earnings data if `MarketData.Fundamentals.EarningsHistory` is empty (circular dependency with bug #2).
3. **JSON parsing**: Gemini may not be producing the new `forecast_tracking` field, or `parseFilingsIntelResponse` may not be extracting it.

### Recommended Fix

1. Force-refresh filings intelligence for SKS.AU
2. Test the enhanced prompt in isolation with known earnings data
3. Verify `parseFilingsIntelResponse` handles the new fields

---

## Summary

| Category | Grade | Status |
|----------|-------|--------|
| Price Data | **A** | Production-ready |
| Fundamentals | **B+** | Production-ready (minor P/E variance to investigate) |
| Technical Signals | **A-** | Production-ready |
| Filings Intelligence | **B+** | Production-ready (could improve with specific figures) |
| Earnings History | **F** | Not functional — data absent |
| Analyst Ratings | **F** | Not functional — data absent |
| Upcoming Events | **F** | Critical bug — returns global calendar |
| Forecast Tracking | **F** | Not functional — data absent |

### Action Items

| Priority | Issue | Action |
|----------|-------|--------|
| P0 | Calendar API returns global events | Fix `GetEarningsCalendar` ticker filtering + add response size guard |
| P1 | Earnings History missing | Force-refresh and verify EODHD returns data for ASX stocks |
| P1 | Analyst Ratings missing | Force-refresh and verify EODHD returns data for ASX stocks |
| P1 | Forecast Tracking missing | Force-refresh FilingsIntelligence with new prompt |
| P2 | P/E ratio discrepancy (35.25 vs 32.67) | Investigate EODHD earnings basis vs external sources |
| P2 | FilingsIntelligence lacks specific figures | Enhance prompt to extract actual revenue/profit numbers |
| P3 | Market Cap 5-8% high | Investigate share count source |

### Conclusion

The existing Vire features (price, fundamentals, signals, filings intelligence) are accurate and production-quality. The newly deployed earnings history, analyst ratings, and upcoming events features (v0.3.14) have three bugs that prevent them from producing useful data. The most critical is the calendar API returning unfiltered global data, inflating the response to 2.2MB. All three issues likely stem from either stale cached data or API parameter handling and should be addressable without architectural changes.
