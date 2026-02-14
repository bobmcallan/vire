# Phase 2: Live Prices Integration Plan

## Investigation Summary

### EODHD Real-Time API
- **Endpoint:** `GET /api/real-time/{TICKER}?api_token={TOKEN}&fmt=json`
- **Response fields:** `code`, `timestamp` (unix), `open`, `high`, `low`, `close`, `volume`, `bid`, `ask`, `bid_volume`, `ask_volume`, `bid_timestamp`, `ask_timestamp`
- **Multi-ticker:** `?s=TICKER2,TICKER3` appended to primary ticker request
- **Rate:** 100k requests/day, 1 call per ticker, recommend 15-20 tickers per batch
- **Delay:** Stocks 15-20min, Forex ~1min, Crypto similar
- **Supported:** Stocks, forex, crypto across global exchanges

### Current Architecture
- **EODHD client** (`internal/clients/eodhd/client.go`): Uses `get()` helper with rate limiter, API key injection, JSON decode, and logging. All methods follow the same pattern.
- **EODHDClient interface** (`internal/interfaces/clients.go`): Lists GetEOD, GetFundamentals, GetTechnicals, GetNews, GetExchangeSymbols, ScreenStocks.
- **GetStockData** (`internal/services/market/service.go:215`): Sets `Price.Current = EOD[0].Close` (yesterday's close). This is the primary target for live price.
- **ReviewPortfolio** (`internal/services/portfolio/service.go:260`): Uses `marketData.EOD[0].Close` for overnight movement. Also has a Navexa-EODHD price cross-check in SyncPortfolio (line 149) that already patches holding prices from EODHD EOD bars.
- **CheckPlanEvents** (`internal/services/plan/service.go:206`): Evaluates conditions against `signals` and `fundamentals` fields. Does NOT currently use a raw `price.current` field — price-based conditions use `signals.price.distance_to_sma*` fields from cached signals. No change needed for plan service in this phase.
- **Mock test file** (`cmd/vire-mcp/mocks_test.go`): Has mock EODHDClient pattern but it is not directly defined there — mocks are in individual test files.

## Proposed Changes

### 2.1 New Model: `RealTimeQuote`

Add to `internal/models/market.go`:

```go
type RealTimeQuote struct {
    Code      string
    Open      float64
    High      float64
    Low       float64
    Close     float64   // current/last price
    Volume    int64
    Timestamp time.Time
}
```

### 2.2 New Client Method: `GetRealTimeQuote()`

Add to `internal/clients/eodhd/client.go`:

```go
func (c *Client) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
```

Implementation:
- Uses the existing `get()` helper (it already handles rate limiting, API key, JSON decode, logging)
- Path: `/real-time/{ticker}` (no extra params needed beyond the defaults set by `get()`)
- Maps the JSON response struct (with unix timestamp) to `models.RealTimeQuote`
- Internal response struct handles `timestamp` as int64 (unix seconds), converted to `time.Time`

### 2.3 Interface Update

Add to `EODHDClient` in `internal/interfaces/clients.go`:

```go
GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
```

### 2.4 Integration: GetStockData (market service)

In `internal/services/market/service.go`, `GetStockData()`:

**Current behavior (line 237-257):** Sets `PriceData.Current = EOD[0].Close`

**New behavior:**
1. After loading market data from storage, attempt `GetRealTimeQuote(ticker)`
2. If real-time succeeds: use real-time `Close` as `PriceData.Current`, real-time `Open/High/Low/Volume`
3. If real-time fails: fall back to existing `EOD[0].Close` behavior (log at warn level)
4. Keep `PreviousClose`, `Change`, `ChangePct` calculated from EOD[1] as before, but recompute Change using live price
5. Keep `AvgVolume`, `High52Week`, `Low52Week` from EOD bars (historical context)

### 2.5 Integration: ReviewPortfolio (portfolio service)

In `internal/services/portfolio/service.go`, `ReviewPortfolio()`:

**Current behavior:** Uses `marketData.EOD[0].Close - marketData.EOD[1].Close` for overnight movement.

**New behavior:**
1. After batch-loading market data (Phase 2, line 301), fetch real-time quotes for all active holding tickers
2. For each holding, if a real-time quote is available:
   - Recompute `overnightMove = realtime.Close - EOD[1].Close` (live vs previous close)
   - Update the holding's `CurrentPrice` and `MarketValue` for the review
3. If real-time fails for a ticker: fall back to existing EOD behavior (no change)
4. The eodhd client field already exists on the portfolio service struct

### 2.6 Integration: CheckPlanEvents (plan service) — NO CHANGE NEEDED

**Rationale:** Plan conditions use `signals.rsi`, `signals.price.distance_to_sma*`, and `fundamentals.*` fields. These are computed from EOD bars and cached signals. The plan service does NOT resolve a raw "current price" — it uses computed signal fields. Adding live price to plan conditions would require:
- A new condition namespace (e.g., `price.current`)
- The plan service to have an EODHD client dependency (currently it only has storage + strategy)

This is a separate feature and not required for Phase 2. Signal-based conditions already work correctly with EOD data. If the user wants "buy when price < X", they can use `fundamentals.pe` or define a signals-based condition.

### 2.7 Mock Updates

Update mock implementations in test files that implement `EODHDClient`:
- `cmd/vire-mcp/mocks_test.go` — if there's a mock EODHDClient
- `internal/services/portfolio/service_test.go` — has mock EODHD
- `internal/services/plan/service_test.go` — does NOT use EODHD (no change)
- Any other test files with mock EODHDClient implementations

### Fallback Strategy

All real-time price integrations follow the same pattern:
1. Attempt real-time fetch
2. On ANY error (network, API, rate limit, decode): log at warn level, continue with EOD data
3. Never block or fail a request because real-time is unavailable
4. Log the fallback so diagnostics can track real-time success rates

### Files Modified (Summary)

| File | Change |
|------|--------|
| `internal/models/market.go` | Add `RealTimeQuote` struct |
| `internal/interfaces/clients.go` | Add `GetRealTimeQuote` to `EODHDClient` |
| `internal/clients/eodhd/client.go` | Add `GetRealTimeQuote()` method + internal response struct |
| `internal/services/market/service.go` | Modify `GetStockData()` to prefer live price |
| `internal/services/portfolio/service.go` | Modify `ReviewPortfolio()` to use live prices for overnight moves |
| Mock files in test packages | Add `GetRealTimeQuote` stub |

### What We Are NOT Changing
- `internal/services/plan/service.go` — plan conditions don't use raw price
- `CollectMarketData` — EOD collection remains unchanged (live supplements, doesn't replace)
- Signal computation — signals are computed from EOD bars, not live price
- SyncPortfolio — the Navexa-EODHD price cross-check stays as-is (it handles stale Navexa data)
