# Requirements: True Net P&L, Break-even Price & Price Targets

**Date:** 2026-02-22
**Requested:** Implement derived P&L fields per `docs/portfolio/portfolio-stock-calculation.md`

## Scope
- Add 7 new derived fields to `Holding` struct
- Compute them server-side from existing data (no new external data needed)
- Update MCP tool descriptions
- Unit and integration tests

## Out of Scope
- User-configurable target percentages
- Portfolio-level breakeven aggregation
- Any changes to existing return % calculations

## Approach

### 1. Model (`internal/models/portfolio.go`)
Add 7 fields to `Holding` struct after line 85 (`LastUpdated`):

```go
NetPnlIfSoldToday   *float64 `json:"net_pnl_if_sold_today"`    // realized + unrealized (nil when units=0)
NetReturnPct         *float64 `json:"net_return_pct"`            // net_pnl / total_invested * 100 (nil when units=0)
TrueBreakevenPrice   *float64 `json:"true_breakeven_price"`      // (total_cost - realized_gain_loss) / units (nil when units=0)
PriceTarget15Pct     *float64 `json:"price_target_15pct"`        // true_breakeven * 1.15 (nil when units=0)
StopLoss5Pct         *float64 `json:"stop_loss_5pct"`            // true_breakeven * 0.95 (nil when units=0)
StopLoss10Pct        *float64 `json:"stop_loss_10pct"`           // true_breakeven * 0.90 (nil when units=0)
StopLoss15Pct        *float64 `json:"stop_loss_15pct"`           // true_breakeven * 0.85 (nil when units=0)
```

Use `*float64` (pointer) so they serialize as `null` for closed positions (units=0).

### 2. Service (`internal/services/portfolio/service.go`)
After line 267 (after holdingMetrics population), add derived field computation:

```go
if holdings[i].Units > 0 {
    netPnl := holdings[i].RealizedGainLoss + holdings[i].UnrealizedGainLoss
    holdings[i].NetPnlIfSoldToday = &netPnl

    if holdings[i].TotalInvested > 0 {
        netReturnPct := netPnl / holdings[i].TotalInvested * 100
        holdings[i].NetReturnPct = &netReturnPct
    }

    trueBreakeven := (holdings[i].TotalCost - holdings[i].RealizedGainLoss) / holdings[i].Units
    holdings[i].TrueBreakevenPrice = &trueBreakeven

    pt15 := trueBreakeven * 1.15
    sl5 := trueBreakeven * 0.95
    sl10 := trueBreakeven * 0.90
    sl15 := trueBreakeven * 0.85
    holdings[i].PriceTarget15Pct = &pt15
    holdings[i].StopLoss5Pct = &sl5
    holdings[i].StopLoss10Pct = &sl10
    holdings[i].StopLoss15Pct = &sl15
}
```

### 3. Catalog (`internal/server/catalog.go`)
Update `get_portfolio` and `get_portfolio_stock` descriptions to mention:
- True breakeven price accounting for realized P&L
- Net P&L if sold today
- Price targets and stop losses based on true breakeven

### 4. Tests
- Unit tests in `internal/services/portfolio/service_test.go`:
  - Simple hold (no prior sells): breakeven = avg cost
  - Partial sell with loss: breakeven increases
  - Partial sell with profit: breakeven decreases
  - Closed position (units=0): all fields nil
  - Net P&L = realized + unrealized
- Integration tests in `tests/api/portfolio_stock_test.go`

## Files Expected to Change
- `internal/models/portfolio.go` — add 7 fields
- `internal/services/portfolio/service.go` — compute derived values
- `internal/server/catalog.go` — update tool descriptions
- `internal/services/portfolio/service_test.go` — unit tests
- `tests/api/portfolio_stock_test.go` — integration tests
