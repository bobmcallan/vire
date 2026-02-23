# Requirements: Market Scan — Flexible Query Engine

**Date:** 2026-02-23
**Requested:** Implement the market scan feature per `docs/features/20260223-market-scan-requirements.md` (feedback `fb_33b18b75`)
**Spec:** `docs/features/20260223-market-scan-requirements.md`

## Scope

### In Scope
- New `market_scan` MCP tool — flexible GraphQL-style query with caller-specified fields, filters, sort, limit
- New `market_scan_fields` MCP tool — field introspection (returns all available fields with types, operators, descriptions)
- `POST /api/scan` endpoint — executes scan queries
- `GET /api/scan/fields` endpoint — returns field metadata
- New scan service in `internal/services/market/scan.go`
- Scan models in `internal/models/scan.go`
- Unit tests for filter evaluation, field extraction, sorting
- Integration tests for the API endpoints

### Out of Scope
- Strategy-aware scoring (Claude's job, not the scan's)
- News sentiment in scan results (that's the deep-dive layer)
- Real-time data — scan uses cached EOD + fundamentals + signals
- Caching layer for scan results (can add later if needed)

## Approach

### Architecture

The scan is a **read-only query engine** over cached data. It does not fetch new data or compute signals — it reads what the job manager has already collected.

**Data flow:**
1. Caller sends a scan query (exchange, filters, fields, sort, limit)
2. Scan service loads all tickers for the exchange from `StockIndexStore.List()`
3. For each ticker, loads cached `MarketData` via `MarketDataStorage.GetMarketData()` and `TickerSignals` via `SignalStorage.GetSignals()` — using batch methods where possible
4. Builds a unified field map per ticker from MarketData.Fundamentals, MarketData.EOD, and TickerSignals
5. Applies filters (AND by default, OR groups supported)
6. Extracts only the requested fields
7. Sorts by requested field(s)
8. Limits and returns results with metadata

### Key Design Decisions

1. **Field registry pattern** — A `ScanFieldRegistry` defines all ~70 available fields with their types, extractors, filter operators, and sortability. Each field has a `func(md *MarketData, sig *TickerSignals) interface{}` extractor. This makes adding new fields trivial and powers both the scan engine and the `/api/scan/fields` introspection endpoint.

2. **Filter evaluation** — Filters are evaluated per-ticker against the field registry. Each filter references a field name, operator, and value. The evaluator extracts the field value and applies the operator. `null` fields fail non-null filters (except `is_null`). OR groups are supported via a nested `or` array.

3. **Batch loading** — Use `GetMarketDataBatch()` and `GetSignalsBatch()` to load data for all exchange tickers in one call, avoiding N+1 reads.

4. **Computed fields from EOD** — Some fields (52-week return, 30-day return, volume averages, Bollinger bands, stochastic, ADX, CCI, Williams %R) need to be derived from the EOD bar array at scan time. The field registry extractors handle this computation. These are lightweight calculations over cached data.

5. **Performance target** — < 5 seconds for limit ≤ 20 on a single exchange. Since we're reading cached data (no API calls), this should be achievable.

### New Files

| File | Purpose |
|------|---------|
| `internal/models/scan.go` | ScanQuery, ScanFilter, ScanResult, ScanMeta, ScanFieldDef models |
| `internal/services/market/scan.go` | Scanner service — field registry, filter evaluation, query execution |
| `internal/services/market/scan_fields.go` | Field registry definitions (~70 fields with extractors) |
| `internal/services/market/scan_test.go` | Unit tests for filter evaluation, field extraction, sorting |
| `internal/server/handlers_scan.go` | HTTP handlers for POST /api/scan and GET /api/scan/fields |
| `tests/api/scan_test.go` | Integration tests for scan API endpoints |

### Files to Modify

| File | Change |
|------|--------|
| `internal/interfaces/services.go` | Add `ScanMarket()` and `ScanFields()` to MarketService interface |
| `internal/services/market/service.go` | Wire Scanner into MarketService, implement interface methods |
| `internal/server/routes.go` | Register `/api/scan` and `/api/scan/fields` routes |
| `internal/server/catalog.go` | Add `market_scan` and `market_scan_fields` MCP tool definitions |

### ScanQuery Model

```go
type ScanQuery struct {
    Exchange string       `json:"exchange"`          // "AU", "US", "ALL"
    Filters  []ScanFilter `json:"filters"`           // AND by default
    Fields   []string     `json:"fields"`            // requested output fields
    Sort     interface{}  `json:"sort,omitempty"`     // single ScanSort or []ScanSort
    Limit    int          `json:"limit,omitempty"`    // max 50, default 20
}

type ScanFilter struct {
    Field    string      `json:"field,omitempty"`
    Op       string      `json:"op,omitempty"`
    Value    interface{} `json:"value,omitempty"`
    Or       []ScanFilter `json:"or,omitempty"`       // OR group
}

type ScanSort struct {
    Field string `json:"field"`
    Order string `json:"order"` // "asc" or "desc"
}
```

### Field Categories

From the requirements doc, ~70 fields across 7 categories:
- **Identity** (8): ticker, name, exchange, sector, industry, country, market_cap, currency
- **Price & Momentum** (13): price, 52w high/low, returns (7d/30d/52w/YTD), weighted_alpha, etc.
- **Moving Averages** (10): SMA/EMA 20/50/200, price_vs_sma_*, crossover booleans
- **Oscillators & Indicators** (18): RSI, MACD, ATR, Bollinger, Stochastic, ADX, CCI, Williams %R, support/resistance
- **Volume & Liquidity** (6): volume, avg_volume_30d/90d, volume_ratio, volume_trend, relative_volume
- **Fundamentals** (30): PE, PB, PS, EV/EBITDA, PEG, margins, growth, dividends, FCF, beta, short interest
- **Analyst Sentiment** (8): ratings counts, consensus, target price, upside %

Many of these map directly to existing `Fundamentals` and `TickerSignals` struct fields. Some (returns, volume averages, Bollinger, Stochastic, ADX, CCI, Williams %R, EMA) need computation from EOD bars.
