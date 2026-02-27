# Requirements: Fix 3 Portfolio Feedback Items

## Feedback Items
- fb_fb956a5e: get_portfolio missing yesterday/week fields after restart
- fb_742053d8: capital_performance returns all zeros (no auto-sync from trades)
- fb_cafb4fa0: Expose raw daily portfolio value time series

---

## Bug 1: Missing Yesterday/Week Fields After Restart (fb_fb956a5e)

**Problem:** `get_portfolio` response has `yesterday_total`, `yesterday_total_pct`, `last_week_total`, `last_week_total_pct` and per-holding `yesterday_close`, `yesterday_pct`, `last_week_close`, `last_week_pct` fields that disappear after server restart. Root cause: `populateHistoricalValues()` computes from EOD market data on-the-fly, but:
1. `SyncPortfolio()` doesn't call `populateHistoricalValues()` (line 468)
2. Market data may not be collected yet after restart
3. Missing market data silently returns zeros (no error)

**Files:**
- `internal/services/portfolio/service.go` — `SyncPortfolio()` (line ~468), `populateHistoricalValues()` (lines 495-578)

**Fix (2 parts):**

### 1a. Call populateHistoricalValues() from SyncPortfolio()
In `SyncPortfolio()`, before returning the portfolio at line ~468, call `populateHistoricalValues()`:
```go
// Before: return portfolio, nil
s.populateHistoricalValues(ctx, portfolio)
return portfolio, nil
```

### 1b. Log when market data is missing instead of silent return
In `populateHistoricalValues()`, when `GetMarketDataBatch()` fails or individual holdings have no market data, log at Warn level (already done for batch, but ensure per-holding skips are logged too).

**Tests:**
- Unit test: `populateHistoricalValues()` with valid EOD data produces non-zero fields
- Unit test: `populateHistoricalValues()` with missing market data doesn't panic

---

## Feature 2: Auto-Derive Capital Performance from Navexa Trades (fb_742053d8)

**Problem:** `get_capital_performance` returns all zeros because no cash transactions exist. The system requires manual entry via `add_cash_transaction`. However, Navexa trade history is available via `GetHoldingTrades()` and contains all buy/sell data needed to auto-derive capital deployed.

**Root cause:** `CashFlowService.CalculatePerformance()` returns empty struct when `len(ledger.Transactions) == 0` (line 242 in `internal/services/cashflow/service.go`).

**Existing code that helps:**
- `GetHoldingTrades()` in `internal/clients/navexa/client.go` (line 358) — fetches per-holding trades
- XIRR calculator in `internal/services/portfolio/xirr.go` — already derives cash flows from trades
- Portfolio sync already has holdings with holding IDs

**Fix: Add trade-based fallback to CalculatePerformance()**

When the cash transaction ledger is empty, fall back to computing capital performance from Navexa trades:

### 2a. Add method to derive cash flows from portfolio trades
**File:** `internal/services/cashflow/service.go`

```go
func (s *Service) deriveFromTrades(ctx context.Context, portfolioName string) (*models.CapitalPerformance, error) {
    // 1. Get portfolio to find holdings with IDs
    // 2. For each holding, get trades from Navexa
    // 3. Sum buy trades as "deposited" (units * price + fees)
    // 4. Sum sell trades as "withdrawn" (units * price - fees)
    // 5. Get current portfolio value
    // 6. Compute simple return and XIRR
    // 7. Return populated CapitalPerformance
}
```

### 2b. Call fallback in CalculatePerformance()
**File:** `internal/services/cashflow/service.go` (line 242)

```go
if len(ledger.Transactions) == 0 {
    // Try deriving from trade history
    derived, err := s.deriveFromTrades(ctx, portfolioName)
    if err != nil || derived == nil {
        return &models.CapitalPerformance{}, nil
    }
    return derived, nil
}
```

### 2c. Add PortfolioService and NaveClient dependencies to CashFlowService
The CashFlowService needs access to portfolio data and Navexa client for trade lookups. Check what's already available via the service's dependencies or pass through the App struct.

**Tests:**
- Unit test: CalculatePerformance with empty ledger but valid trades returns non-zero
- Unit test: CalculatePerformance with empty ledger and no trades returns zeros
- Unit test: deriveFromTrades correctly sums buy/sell trades

---

## Feature 3: Expose Raw Daily Portfolio Value Time Series (fb_cafb4fa0)

**Problem:** `get_portfolio_indicators` computes 65+ daily portfolio value data points internally via `GetDailyGrowth()` but doesn't return them. Only aggregated indicators (EMA, RSI, trend) are returned.

**Existing code:**
- `GetDailyGrowth()` in `internal/services/portfolio/growth.go` (lines 75-203) — computes daily values
- `GrowthDataPoint` model has Date, TotalValue, TotalCost, NetReturn, NetReturnPct, HoldingCount
- `growthToBars()` in `internal/services/portfolio/indicators.go` — converts to bars (adds external balances)
- `GetPortfolioIndicators()` already calls `GetDailyGrowth()` but discards the raw points

**Fix (LOW complexity, purely additive):**

### 3a. Add TimeSeriesPoint model
**File:** `internal/models/portfolio.go`

```go
type TimeSeriesPoint struct {
    Date         time.Time `json:"date"`
    Value        float64   `json:"value"`
    Cost         float64   `json:"cost"`
    NetReturn    float64   `json:"net_return"`
    NetReturnPct float64   `json:"net_return_pct"`
    HoldingCount int       `json:"holding_count"`
}
```

### 3b. Add TimeSeries field to PortfolioIndicators
**File:** `internal/models/portfolio.go`

```go
type PortfolioIndicators struct {
    // ... existing fields ...
    TimeSeries []TimeSeriesPoint `json:"time_series,omitempty"`
}
```

### 3c. Populate TimeSeries in GetPortfolioIndicators()
**File:** `internal/services/portfolio/indicators.go`

After `GetDailyGrowth()` returns, convert growth points to time series and attach to indicators response. Add external balance total to each point's value (same as `growthToBars()` does).

```go
func growthPointsToTimeSeries(points []models.GrowthDataPoint, externalBalance float64) []models.TimeSeriesPoint {
    ts := make([]models.TimeSeriesPoint, len(points))
    for i, p := range points {
        ts[i] = models.TimeSeriesPoint{
            Date:         p.Date,
            Value:        p.TotalValue + externalBalance,
            Cost:         p.TotalCost,
            NetReturn:    p.NetReturn,
            NetReturnPct: p.NetReturnPct,
            HoldingCount: p.HoldingCount,
        }
    }
    return ts
}
```

No handler changes needed — handler already returns the full struct via WriteJSON.

### 3d. Update catalog description (optional)
**File:** `internal/server/catalog.go`

Update `get_portfolio_indicators` description to mention the time series data.

**Tests:**
- Unit test: GetPortfolioIndicators returns non-empty TimeSeries
- Unit test: TimeSeries values include external balance total

---

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Call `populateHistoricalValues()` from `SyncPortfolio()` |
| `internal/services/cashflow/service.go` | Add trade-based fallback for `CalculatePerformance()` |
| `internal/models/portfolio.go` | Add `TimeSeriesPoint` model, add `TimeSeries` field to `PortfolioIndicators` |
| `internal/services/portfolio/indicators.go` | Add `growthPointsToTimeSeries()`, populate `TimeSeries` in response |
| `internal/server/catalog.go` | Update `get_portfolio_indicators` description |
| Test files | Unit tests for all 3 fixes |
