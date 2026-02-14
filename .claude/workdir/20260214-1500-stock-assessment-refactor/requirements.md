# Requirements: Stock Assessment Refactor — 3-Layer Architecture

**Date:** 2026-02-14
**Requested:** Refactor stock assessment from narrative-driven to data-driven, structured into 3 layers. Remove broken v0.3.14 EODHD earnings/calendar features. Make Vire provide structured data (numbers, facts, extracted figures) that LLMs can reason over, rather than stories.

**Context:**
- `docs/stock-history-feature-review.md` — v0.3.14 review showing EODHD earnings/calendar features are broken
- `docs/stock-assessment-example.md` — example of what good assessment looks like (SKS with actual revenue/profit numbers per year, specific guidance figures, contract values)
- Previous sprint `.claude/workdir/20260214-1200-stock-history/` added broken features that need replacing

## Design

### Three Layers

**Layer 1: Technical Profile** (from EODHD — refreshes frequently)
- Price data, moving averages, RSI, MACD, volume, ATR
- Fundamentals: market cap, P/E, P/B, EPS, dividend yield, beta
- Computed signals: PBAS, VLI, regime, trend, support/resistance
- Risk flags
- Freshness: existing TTLs (1h for EOD, 7d for fundamentals)

**Layer 2: Company Releases** (from ASX filings + Gemini extraction — append-only)
- Each price-sensitive filing summarised with SPECIFIC NUMBERS extracted:
  - Financial results: revenue, profit, margins, EPS, dividends
  - Guidance/forecasts: revenue target, profit target, margin target
  - Contracts: value, customer, scope
  - Acquisitions: price, target revenue, strategic rationale
  - Business changes: new markets, restructuring, leadership
- Stored per-filing (not as a single blob), so new filings are added incrementally
- Old filings never re-analyzed (append-only)
- Freshness: filings list refreshes per existing 30-day TTL; intelligence is per-filing (once analyzed, done)

**Layer 3: Company Timeline** (Gemini-generated from layers 1+2 — periodic rebuild)
- Structured yearly/quarterly summary combining financials + events
- Per-period entries with: revenue, profit, key events, guidance given, guidance outcome
- Key dates: next reporting date, recent catalysts
- Business model summary (extracted from filings, not invented)
- This is the "LLM-ready" summary — structured data, not narrative
- Freshness: rebuild when new filings are analyzed or quarterly

### Key Principles

1. **Strategy-independent** — no user strategy incorporated. Data is for all users.
2. **Data, not stories** — extract numbers, provide structured fields. The LLM adds interpretation.
3. **Incremental** — filings intelligence is per-filing and append-only. Only new filings get analyzed.
4. **Dynamic freshness** — technicals refresh quickly (EODHD call), releases are permanent, timeline rebuilds periodically.

## Scope

### In Scope

1. **Remove broken v0.3.14 features:**
   - Remove `GetEarningsCalendar` method and `FreshnessUpcomingEvents` (calendar API returns global data, unusable)
   - Remove `EarningsHistory`, `EarningsRecord`, `UpcomingEvent` types from models
   - Remove `ForecastTracking`, `ForecastEntry` types
   - Remove EPS estimate fields from `Fundamentals` (EODHD doesn't reliably return these for ASX stocks)
   - Keep `AnalystRatings` on Fundamentals (may work for some stocks, harmless if empty)
   - Keep `flexString`, `flexFloat64` null fix (good defensive code)

2. **Refactor FilingsIntelligence into per-filing extraction:**
   - New model: `FilingSummary` — per-filing extracted data with specific numbers
   - New Gemini prompt: extract structured data from each filing (or small batches), not one big narrative
   - Store `[]FilingSummary` on MarketData alongside raw filings
   - Incremental: track which filings have been summarized (by DocumentKey or date+headline hash)
   - Only send unsummarized filings to Gemini

3. **New CompanyTimeline model:**
   - Replaces the old `FilingsIntelligence` narrative blob
   - Structured per-period entries (yearly, with quarterly where available)
   - Generated from `[]FilingSummary` + fundamentals data
   - Includes: business model description, financial track record, key events timeline, next reporting dates
   - Built by Gemini from the structured filing summaries

4. **Restructure StockData response:**
   - `technical` section: existing price/signals/fundamentals (Layer 1)
   - `releases` section: `[]FilingSummary` (Layer 2)
   - `timeline` section: `CompanyTimeline` (Layer 3)
   - Remove or restructure old `FilingsIntelligence` fields

5. **Update HoldingReview** to surface the same 3 layers

### Out of Scope
- Strategy alignment logic (that's the LLM's job)
- Non-ASX filings sources (future work)
- Financial statement API data from EODHD (unreliable for ASX)
- Dividend calendar, IPO calendar

## Approach

### New Models (`internal/models/market.go`)

```go
// FilingSummary is a per-filing structured data extraction
type FilingSummary struct {
    Date           time.Time         `json:"date"`
    Headline       string            `json:"headline"`
    Type           string            `json:"type"`           // "financial_results", "guidance", "contract", "acquisition", "business_change", "other"
    PriceSensitive bool              `json:"price_sensitive"`
    // Extracted financial data (zero values mean not applicable)
    Revenue        string            `json:"revenue,omitempty"`         // e.g. "$261.7M"
    RevenueGrowth  string            `json:"revenue_growth,omitempty"`  // e.g. "+92%"
    Profit         string            `json:"profit,omitempty"`          // e.g. "$14.0M" (net or PBT, labeled)
    ProfitGrowth   string            `json:"profit_growth,omitempty"`   // e.g. "+112%"
    Margin         string            `json:"margin,omitempty"`          // e.g. "10%"
    EPS            string            `json:"eps,omitempty"`             // e.g. "$0.12"
    Dividend       string            `json:"dividend,omitempty"`        // e.g. "$0.06 fully franked"
    // Extracted event data
    ContractValue  string            `json:"contract_value,omitempty"`  // e.g. "$130M"
    Customer       string            `json:"customer,omitempty"`        // e.g. "NEXTDC"
    AcqTarget      string            `json:"acq_target,omitempty"`      // e.g. "Delta Elcom"
    AcqPrice       string            `json:"acq_price,omitempty"`       // e.g. "$13.75-15M"
    // Guidance/forecast
    GuidanceRevenue string           `json:"guidance_revenue,omitempty"` // e.g. "$340M"
    GuidanceProfit  string           `json:"guidance_profit,omitempty"`  // e.g. "$34M PBT"
    // Key facts — up to 5 bullet points of specific, factual statements
    KeyFacts       []string          `json:"key_facts"`
    // Metadata
    Period         string            `json:"period,omitempty"`          // e.g. "FY2025", "H1 FY2026", "Q3 2025"
    DocumentKey    string            `json:"document_key,omitempty"`
    AnalyzedAt     time.Time         `json:"analyzed_at"`
}

// CompanyTimeline is the LLM-ready structured summary
type CompanyTimeline struct {
    BusinessModel    string              `json:"business_model"`     // 2-3 sentences: what they do, how they make money
    Sector           string              `json:"sector"`
    Industry         string              `json:"industry"`
    Periods          []PeriodSummary     `json:"periods"`            // yearly/half-yearly, most recent first
    KeyEvents        []TimelineEvent     `json:"key_events"`         // significant events in date order
    NextReportingDate string             `json:"next_reporting_date,omitempty"`
    WorkOnHand       string              `json:"work_on_hand,omitempty"`      // latest backlog figure
    RepeatBusinessRate string            `json:"repeat_business_rate,omitempty"` // e.g. "94%"
    GeneratedAt      time.Time           `json:"generated_at"`
}

// PeriodSummary is a single reporting period's financials
type PeriodSummary struct {
    Period        string  `json:"period"`          // "FY2025", "H1 FY2026"
    Revenue       string  `json:"revenue"`         // "$261.7M"
    RevenueGrowth string  `json:"revenue_growth"`  // "+92%"
    Profit        string  `json:"profit"`          // "$14.0M net profit"
    ProfitGrowth  string  `json:"profit_growth"`   // "+112%"
    Margin        string  `json:"margin"`          // "5.4%"
    EPS           string  `json:"eps"`             // "$0.12"
    Dividend      string  `json:"dividend"`        // "$0.06"
    GuidanceGiven string  `json:"guidance_given"`  // what guidance was given FOR NEXT PERIOD
    GuidanceOutcome string `json:"guidance_outcome"` // how prior guidance tracked vs actual
}

// TimelineEvent is a significant company event
type TimelineEvent struct {
    Date     string `json:"date"`      // "2026-02-05"
    Event    string `json:"event"`     // "Major contract award + FY26 profit upgrade"
    Detail   string `json:"detail"`    // "$60M new contracts. Revenue guidance: $320M→$340M. PBT: $28.8M→$34M."
    Impact   string `json:"impact"`    // "positive", "negative", "neutral"
}
```

### Service Changes

**FilingsIntelligence refactor** (`internal/services/market/filings.go`):
- Replace single `generateFilingsIntelligence(ctx, marketData)` with:
  1. `summarizeNewFilings(ctx, ticker, filings, existingSummaries) []FilingSummary` — only processes filings not yet in existingSummaries
  2. `generateCompanyTimeline(ctx, ticker, summaries, fundamentals) *CompanyTimeline` — builds timeline from all summaries
- Gemini prompt for filing summaries: send filings in small batches (3-5), request structured JSON per filing
- Gemini prompt for timeline: send all summaries + fundamentals, request structured PeriodSummary + TimelineEvent arrays

**MarketData changes:**
- Add `FilingSummaries []FilingSummary` field + `FilingSummariesUpdatedAt`
- Add `CompanyTimeline *CompanyTimeline` field + `CompanyTimelineUpdatedAt`
- Keep `FilingsIntelligence` temporarily for backwards compatibility, deprecate

**CollectMarketData changes** (`internal/services/market/service.go`):
- After filings collection, run incremental `summarizeNewFilings`
- After summaries updated, rebuild `CompanyTimeline` if needed

**StockData changes:**
- Add `FilingSummaries []FilingSummary`
- Add `Timeline *CompanyTimeline`

**HoldingReview changes:**
- Add `FilingSummaries []FilingSummary`
- Add `Timeline *CompanyTimeline`

### Freshness

| Data | TTL | Rationale |
|------|-----|-----------|
| FilingSummaries | Per-filing (once done, never re-done) | Releases don't change |
| CompanyTimeline | Rebuild when new summaries added, or 7 days | Periodic refresh |

### MCP Response Format

The `get_stock_data` response will have 3 clear sections:

```
## Technical Profile
[price table, fundamentals table, signals table — existing format]

## Company Releases
[table of FilingSummary entries with extracted numbers]

## Company Timeline
[PeriodSummary table + KeyEvents list + business model]
```

## Files Expected to Change

- `internal/models/market.go` — New types, remove broken v0.3.14 types, restructure StockData
- `internal/models/portfolio.go` — Update HoldingReview
- `internal/services/market/filings.go` — Major refactor: per-filing extraction + timeline generation
- `internal/services/market/service.go` — Update CollectMarketData flow, GetStockData assembly
- `internal/clients/eodhd/client.go` — Remove GetEarningsCalendar, clean up broken features
- `internal/interfaces/clients.go` — Remove GetEarningsCalendar from interface
- `internal/common/freshness.go` — Remove FreshnessUpcomingEvents, add FreshnessTimeline
- `internal/services/market/filings_test.go` — New tests for per-filing extraction
- `internal/services/market/service_test.go` — Updated tests
- Mock files — Interface changes
- `cmd/vire-mcp/formatters.go` — Update response formatting for 3-layer structure
