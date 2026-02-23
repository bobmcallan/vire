# Vire Market Scan — Requirements v2

## Overview

The market scan is a **flexible data retrieval layer** that enables Claude to query the market across any combination of technical, fundamental, and momentum dimensions. It is not strategy-aware — strategy application is Claude's responsibility, using the portfolio strategy and plan as context when composing queries.

The scan is analogous to GraphQL: the caller specifies exactly what fields they want, what filters to apply, and how to sort and limit results. The scan engine executes against EODHD data and returns structured results. Claude then interprets, ranks, and presents candidates through the lens of the strategy.

---

## Design Principles

1. **Dumb pipe, smart consumer.** The scan has no knowledge of strategy, risk rules, or portfolio state. Claude brings that context.
2. **Caller-defined fields.** The caller specifies exactly which dimensions to return — no fixed output schema.
3. **Caller-defined filters.** Any returnable field can be used as a filter. Filters are composable.
4. **GraphQL-style query model.** Fields, filters, sort, and limit in a single query object.
5. **Fast.** Phase 1 scan must complete in under 5 seconds. Uses EODHD EOD + fundamentals cache.
6. **Rich data.** Return as much technical, fundamental, and momentum data as EODHD provides — Claude decides what's relevant.

---

## Query Model

### Structure

```graphql
scan(exchange, filters, fields, sort, limit) {
  results {
    [requested fields]
  }
  meta {
    total_matched
    exchange
    executed_at
    query_time_ms
  }
}
```

### Full Example

```json
{
  "exchange": "AU",
  "filters": [
    { "field": "pe_ratio", "op": "<=", "value": 25 },
    { "field": "beta", "op": "<=", "value": 1.3 },
    { "field": "earnings_positive", "op": "==", "value": true },
    { "field": "market_cap", "op": ">=", "value": 200000000 },
    { "field": "price_vs_sma_200_pct", "op": ">=", "value": 0 }
  ],
  "fields": [
    "ticker", "name", "sector", "market_cap",
    "pe_ratio", "beta", "revenue_growth_1yr_pct",
    "dividend_yield_pct", "earnings_growth_next_yr_pct",
    "price", "sma_20", "sma_50", "sma_200",
    "price_vs_sma_20_pct", "price_vs_sma_50_pct", "price_vs_sma_200_pct",
    "rsi_14", "macd", "macd_signal", "macd_histogram",
    "volume", "avg_volume_30d", "volume_ratio",
    "52_week_high", "52_week_low", "52_week_return_pct",
    "30d_return_pct", "new_highs_30d",
    "atr_pct", "weighted_alpha"
  ],
  "sort": { "field": "52_week_return_pct", "order": "desc" },
  "limit": 20
}
```

---

## Available Fields

### Identity

| Field | Type | Description |
|-------|------|-------------|
| `ticker` | string | Exchange ticker |
| `name` | string | Company name |
| `exchange` | string | AU / NYSE / NASDAQ |
| `sector` | string | GICS sector |
| `industry` | string | GICS industry |
| `country` | string | Country of domicile |
| `market_cap` | float | Market capitalisation (AUD or USD) |
| `currency` | string | Native currency |

### Price & Momentum

| Field | Type | Description |
|-------|------|-------------|
| `price` | float | Last close price |
| `price_open` | float | Open |
| `price_high` | float | Day high |
| `price_low` | float | Day low |
| `52_week_high` | float | 52-week high |
| `52_week_low` | float | 52-week low |
| `52_week_return_pct` | float | Price return over 52 weeks |
| `30d_return_pct` | float | Price return over 30 days |
| `7d_return_pct` | float | Price return over 7 days |
| `ytd_return_pct` | float | Year-to-date return |
| `new_highs_30d` | int | Number of new highs in last 30 days |
| `weighted_alpha` | float | Barchart-style weighted price momentum |
| `distance_to_52w_high_pct` | float | % below 52-week high |
| `distance_to_52w_low_pct` | float | % above 52-week low |

### Moving Averages

| Field | Type | Description |
|-------|------|-------------|
| `sma_20` | float | 20-day simple moving average |
| `sma_50` | float | 50-day simple moving average |
| `sma_200` | float | 200-day simple moving average |
| `ema_20` | float | 20-day exponential moving average |
| `ema_50` | float | 50-day exponential moving average |
| `price_vs_sma_20_pct` | float | % above/below SMA20 |
| `price_vs_sma_50_pct` | float | % above/below SMA50 |
| `price_vs_sma_200_pct` | float | % above/below SMA200 |
| `sma_20_above_sma_50` | bool | Golden/death cross short-term |
| `sma_50_above_sma_200` | bool | Golden/death cross long-term |

### Oscillators & Indicators

| Field | Type | Description |
|-------|------|-------------|
| `rsi_14` | float | RSI 14-period |
| `rsi_signal` | string | overbought / neutral / oversold |
| `macd` | float | MACD line |
| `macd_signal` | float | Signal line |
| `macd_histogram` | float | Histogram |
| `macd_crossover` | string | bullish / bearish / none |
| `atr` | float | Average True Range |
| `atr_pct` | float | ATR as % of price (volatility proxy) |
| `bollinger_upper` | float | Bollinger upper band |
| `bollinger_lower` | float | Bollinger lower band |
| `bollinger_pct_b` | float | Position within bands (0=lower, 1=upper) |
| `stoch_k` | float | Stochastic %K |
| `stoch_d` | float | Stochastic %D |
| `adx` | float | Average Directional Index (trend strength) |
| `cci` | float | Commodity Channel Index |
| `williams_r` | float | Williams %R |
| `near_support` | bool | Price within 2% of support level |
| `near_resistance` | bool | Price within 2% of resistance level |
| `support_level` | float | Nearest support price |
| `resistance_level` | float | Nearest resistance price |

### Volume & Liquidity

| Field | Type | Description |
|-------|------|-------------|
| `volume` | int | Last session volume |
| `avg_volume_30d` | int | 30-day average volume |
| `avg_volume_90d` | int | 90-day average volume |
| `volume_ratio` | float | volume / avg_volume_30d |
| `volume_trend` | string | accumulating / distributing / neutral |
| `relative_volume` | float | vs 90-day average |

### Fundamentals

| Field | Type | Description |
|-------|------|-------------|
| `pe_ratio` | float | Trailing PE |
| `pe_forward` | float | Forward PE (next 12 months) |
| `pb_ratio` | float | Price to book |
| `ps_ratio` | float | Price to sales |
| `ev_ebitda` | float | EV/EBITDA |
| `peg_ratio` | float | PEG ratio |
| `earnings_positive` | bool | Positive trailing earnings |
| `eps_ttm` | float | Earnings per share (trailing 12m) |
| `eps_next_yr` | float | Estimated EPS next year |
| `earnings_growth_next_yr_pct` | float | Estimated earnings growth next year |
| `revenue_ttm` | float | Revenue trailing 12 months |
| `revenue_growth_1yr_pct` | float | Revenue growth YoY |
| `revenue_growth_3yr_pct` | float | Revenue CAGR 3 years |
| `gross_margin_pct` | float | Gross margin % |
| `operating_margin_pct` | float | Operating margin % |
| `net_margin_pct` | float | Net margin % |
| `roe` | float | Return on equity |
| `roa` | float | Return on assets |
| `debt_to_equity` | float | D/E ratio |
| `current_ratio` | float | Current ratio |
| `free_cash_flow` | float | Free cash flow (TTM) |
| `fcf_yield_pct` | float | FCF yield % |
| `dividend_yield_pct` | float | Trailing dividend yield |
| `dividend_growth_3yr_pct` | float | Dividend CAGR 3 years |
| `payout_ratio` | float | Dividend payout ratio |
| `buyback_yield_pct` | float | Share buyback yield |
| `beta` | float | Beta vs market index |
| `short_interest_pct` | float | Short interest % of float |
| `days_to_cover` | float | Short interest days to cover |

### Analyst Sentiment

| Field | Type | Description |
|-------|------|-------------|
| `analyst_strong_buy` | int | Count of Strong Buy ratings |
| `analyst_buy` | int | Count of Buy ratings |
| `analyst_hold` | int | Count of Hold ratings |
| `analyst_sell` | int | Count of Sell ratings |
| `analyst_consensus` | string | strong_buy / buy / hold / sell |
| `analyst_target_price` | float | Mean analyst price target |
| `analyst_target_upside_pct` | float | % upside to mean target |
| `analyst_count` | int | Total analysts covering |

---

## Filter Operators

| Operator | Description |
|----------|-------------|
| `==` | Equals |
| `!=` | Not equals |
| `<` | Less than |
| `<=` | Less than or equal |
| `>` | Greater than |
| `>=` | Greater than or equal |
| `between` | Range inclusive — value is `[min, max]` |
| `in` | Value in list |
| `not_in` | Value not in list |
| `is_null` | Field is null/unavailable |
| `not_null` | Field has a value |

### Filter Composition

Filters are AND by default. OR groups can be expressed:

```json
"filters": [
  { "field": "pe_ratio", "op": "<=", "value": 25 },
  {
    "or": [
      { "field": "revenue_growth_1yr_pct", "op": ">=", "value": 15 },
      { "field": "sector", "op": "in", "value": ["Materials", "ETFs"] }
    ]
  }
]
```

---

## Sort & Limit

```json
"sort": { "field": "weighted_alpha", "order": "desc" },
"limit": 15
```

Multiple sort fields supported:

```json
"sort": [
  { "field": "52_week_return_pct", "order": "desc" },
  { "field": "pe_ratio", "order": "asc" }
]
```

---

## Field Introspection

Before composing a query, Claude calls the fields endpoint to discover what is available. This ensures queries are always valid — no guessing at field names, types, or operator support.

### Endpoint

```
GET /api/v1/scan/fields
```

### Response Schema

```json
{
  "version": "1.2.0",
  "groups": [
    {
      "name": "Identity",
      "fields": [
        {
          "field": "ticker",
          "type": "string",
          "description": "Exchange ticker symbol",
          "filterable": true,
          "sortable": false,
          "operators": ["==", "in", "not_in"],
          "example": "BHP.AU"
        },
        {
          "field": "sector",
          "type": "string",
          "description": "GICS sector classification",
          "filterable": true,
          "sortable": false,
          "operators": ["==", "!=", "in", "not_in"],
          "enum": ["Financials", "Materials", "Industrials", "Technology", "Energy", "Healthcare", "Consumer Discretionary", "Consumer Staples", "Utilities", "Real Estate", "Communication Services"],
          "example": "Industrials"
        }
      ]
    },
    {
      "name": "Fundamentals",
      "fields": [
        {
          "field": "pe_ratio",
          "type": "float",
          "description": "Trailing price-to-earnings ratio",
          "filterable": true,
          "sortable": true,
          "operators": ["<", "<=", ">", ">=", "==", "between", "not_null", "is_null"],
          "nullable": true,
          "example": 18.5
        },
        {
          "field": "revenue_growth_1yr_pct",
          "type": "float",
          "description": "Revenue growth year-over-year as a percentage",
          "filterable": true,
          "sortable": true,
          "operators": ["<", "<=", ">", ">=", "==", "between"],
          "nullable": true,
          "example": 14.2
        }
      ]
    }
  ],
  "exchanges": ["AU", "US", "ALL"],
  "max_limit": 50,
  "generated_at": "2026-02-23T10:00:00Z"
}
```

### Per-Field Properties

| Property | Type | Description |
|----------|------|-------------|
| `field` | string | Exact field name to use in queries and filters |
| `type` | string | `string`, `float`, `int`, `bool` |
| `description` | string | Plain English description for Claude to understand intent |
| `filterable` | bool | Can be used in the `filters` array |
| `sortable` | bool | Can be used in the `sort` object |
| `operators` | array | Valid operators for this field type |
| `nullable` | bool | Field may be null for some tickers — handle in filters with `not_null` |
| `enum` | array | For string fields — exhaustive list of valid values |
| `example` | any | Representative value to aid query composition |
| `unit` | string | Optional — e.g. `"percent"`, `"AUD"`, `"days"` |

### MCP Tool

```json
{
  "name": "market_scan_fields",
  "description": "Returns all available fields for the market_scan tool, grouped by category. Call this before composing a scan query to get exact field names, types, valid operators, and descriptions. Fields marked nullable should use not_null filter if required.",
  "parameters": {}
}
```

Claude should call `market_scan_fields` once per session (or when uncertain about field availability) before calling `market_scan`. The response is stable within a version and can be reused across multiple scan queries in the same session.

---

## API Endpoint

```
POST /api/v1/scan
Content-Type: application/json

{
  "exchange": "AU" | "US" | "ALL",
  "filters": [...],
  "fields": [...],
  "sort": {...},
  "limit": 20
}
```

### Response

```json
{
  "results": [
    {
      "ticker": "SXE.AU",
      "name": "Southern Cross Electrical Engineering",
      "sector": "Industrials",
      "pe_ratio": 18.2,
      "rsi_14": 58.3,
      "52_week_return_pct": 34.5,
      ...
    }
  ],
  "meta": {
    "total_matched": 47,
    "returned": 15,
    "exchange": "AU",
    "executed_at": "2026-02-23T10:00:00Z",
    "query_time_ms": 312
  }
}
```

---

## MCP Tool Exposure

The scan is exposed as a single `market_scan` MCP tool. Claude composes the query based on:

1. The user's natural language request
2. The portfolio strategy (loaded separately via `get_portfolio_strategy`)
3. The current portfolio holdings (loaded separately via `get_portfolio`)
4. The active investment plan (loaded separately via `get_portfolio_plan`)

Claude presents the composed query to the user before executing if the intent is ambiguous, or executes directly for clear requests.

### Tool Definition

```json
{
  "name": "market_scan",
  "description": "Scan the market using EODHD data. Returns any combination of technical, fundamental, and momentum fields for tickers matching the specified filters. Claude composes queries based on strategy context loaded separately.",
  "parameters": {
    "exchange": "AU | US | ALL",
    "filters": "array of filter objects",
    "fields": "array of field names to return",
    "sort": "sort object or array",
    "limit": "integer 1-50"
  }
}
```

---

## Phase 2 — Deep Dive (unchanged)

User selects candidates from scan results. Claude calls the existing `get_stock_data` tool per ticker for full analysis. This is slower (30–60s) and user-initiated.

Claude then applies strategy compliance, position sizing, and forward outlook assessment using the full data set — not the scan layer.

---

## What the Scan Does NOT Do

- Apply strategy rules — that's Claude's job
- Score or rank by strategy alignment — Claude does this
- Know about the current portfolio — Claude loads that separately
- Provide news or sentiment — that's the deep dive layer
- Make buy/sell recommendations — Claude does this

---

## Implementation Notes

- All fields backed by EODHD EOD + fundamentals endpoints
- Fields unavailable for a ticker return `null` — callers should handle gracefully
- Exchange `AU` maps to ASX. `US` covers NYSE + NASDAQ
- Results cached for 24 hours (EOD data). Cache-busting available via `force_refresh: true`
- Field list is versioned — new fields added without breaking existing queries
- Target query time: < 5 seconds for limit ≤ 20 on a single exchange
