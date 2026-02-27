# Capital Allocation Timeline

**Feedback**: fb_c4a661a8, fb_00e43378, fb_da8cabc1, fb_ca924779, fb_e69d6635
**Date**: 2026-02-27

## Scope

Two server-side features from consolidated feedback:

### Feature 1: Capital Allocation Timeline (fb_da8cabc1, fb_ca924779)

Extend the portfolio time series with capital flow tracking so clients can plot:
- **Line 1**: Total capital (holdings + cash + accumulate)
- **Line 2**: Net deployed (cumulative deposits - withdrawals)
- **Gap** = true P&L

**What to change:**

1. **`internal/models/portfolio.go` — `TimeSeriesPoint` struct** (line 212)
   Add fields:
   ```go
   CashBalance       float64 `json:"cash_balance,omitempty"`       // Running cash balance as of this date
   ExternalBalance   float64 `json:"external_balance,omitempty"`   // External balances (accumulate, term deposits)
   TotalCapital      float64 `json:"total_capital,omitempty"`      // Value + CashBalance + ExternalBalance
   NetDeployed       float64 `json:"net_deployed,omitempty"`       // Cumulative deposits - withdrawals to date
   ```

2. **`internal/models/portfolio.go` — `GrowthDataPoint` struct** (line 200)
   Add same capital flow fields so they propagate through the pipeline.

3. **`internal/services/portfolio/growth.go` — `GetDailyGrowth()`** (line 78)
   - Accept a `CashFlowLedger` parameter (or `[]CashTransaction`)
   - In the date iteration loop (line 157), compute running cash balance and net deployed:
     - For each date, sum all transactions up to that date
     - `cash_balance` = running sum of (deposits + dividends + sell_proceeds - withdrawals - transfers_out - buy_costs)
     - `net_deployed` = cumulative (deposits + contributions) - (withdrawals)
   - Add these to each `GrowthDataPoint`

4. **`internal/services/portfolio/indicators.go` — `growthPointsToTimeSeries()`** (line 15)
   - Map the new GrowthDataPoint fields to TimeSeriesPoint fields
   - `total_capital` = Value + CashBalance + ExternalBalance

5. **`internal/services/portfolio/indicators.go` — `GetPortfolioIndicators()`** (line 69)
   - Load the cash flow ledger (needs CashFlowService access)
   - Pass transactions to `GetDailyGrowth()`

6. **`internal/services/portfolio/service.go` — Service struct** (line 21)
   - Add `cashflowSvc interfaces.CashFlowService` field
   - Update `NewService()` to accept it
   - Update all callers (main.go, tests)

**Data sources**: Cash transactions from `CashFlowLedger` + trade history already in growth computation. Both exist.

**Computation**: Merge cash transactions into the existing date iteration in `GetDailyGrowth()`. O(n) — single pass since both are date-sorted.

### Feature 2: Net Flow on Daily/Weekly Change (fb_e69d6635)

Add `yesterday_net_flow` and `last_week_net_flow` to the Portfolio response so daily/weekly change can be adjusted for capital movements.

**What to change:**

1. **`internal/models/portfolio.go` — `Portfolio` struct** (line 42)
   Add fields:
   ```go
   YesterdayNetFlow float64 `json:"yesterday_net_flow,omitempty"` // Net cash flow yesterday (deposits - withdrawals)
   LastWeekNetFlow  float64 `json:"last_week_net_flow,omitempty"` // Net cash flow last 7 days
   ```

2. **`internal/services/portfolio/service.go` — `populateHistoricalValues()`** (line 500)
   - Accept or load `CashFlowLedger`
   - Sum transactions within yesterday and last-week windows
   - Set the new fields on the portfolio

### Out of Scope

- **MCP session persistence** (fb_c4a661a8, fb_00e43378): SSE/MCP transport is handled by the portal service on Fly.dev, not this codebase. Noted for separate work.
- **Portal charts** (fb_ca924779): Frontend work — this ticket provides the data API only.
- **Accumulate interest accrual**: Complex modeling, defer to follow-up.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Add fields to TimeSeriesPoint, GrowthDataPoint, Portfolio |
| `internal/services/portfolio/growth.go` | Cash flow integration in GetDailyGrowth |
| `internal/services/portfolio/indicators.go` | Pass cash flow data, map new fields |
| `internal/services/portfolio/service.go` | Add CashFlowService dep, net flow in populateHistoricalValues |
| `internal/services/portfolio/service_test.go` | Update tests for new Service constructor |
| `internal/services/portfolio/growth_test.go` | Tests for cash flow timeline |
| `internal/services/portfolio/historical_values_stress_test.go` | Update for new constructor |
| `cmd/vire-server/main.go` | Pass CashFlowService to portfolio.NewService |
| `internal/server/catalog.go` | Update tool descriptions |
| `docs/architecture/services.md` | Document capital timeline feature |

## Key Design Decisions

1. **CashFlowService as dependency**: Portfolio service needs cash flow data. Add it as a constructor parameter — follows existing pattern (navexa, eodhd, gemini clients).
2. **Non-breaking**: All new fields use `omitempty` — existing clients see no change when no cash data exists.
3. **Fallback**: When no cash flow ledger exists, timeline fields are zero/omitted. Existing time series data unchanged.
4. **Single pass**: Cash transactions are date-sorted. Merge into the existing date loop in GetDailyGrowth rather than creating a separate pass.
