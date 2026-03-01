# Requirements: Fix Portfolio Value & Dashboard Fields

## Context

Five feedback items report that `get_portfolio` returns incorrect top-level fields. Root cause: `total_value` naively adds `total_cash` (full ledger balance) to equity value, double-counting capital already deployed into stocks. The cash ledger doesn't track trade settlements, so available cash must be derived: `total_cash - net capital in equities`.

The portfolio-level `total_cost` currently sums `AvgCost * Units` for open positions — a meaningless number. It should be the NET capital deployed into equities, computed from trade history (buys - sells), FX-adjusted.

### Correct Numbers (SMSF example)
- `total_value_holdings` = $427,561 (equity market value) — correct, unchanged
- `total_cash` = $477,985 (ledger balance) — correct, unchanged
- `total_cost` = ~$427k (net capital in equities: sum buy costs - sum sell proceeds, FX-adjusted) — **REDEFINED**
- `available_cash` = total_cash - total_cost ~ $51k — **NEW**
- `total_value` = total_value_holdings + available_cash ~ $479k — **FIXED** (was $906k)
- `capital_gain` = total_value - net_capital_deployed — **NEW**
- `capital_gain_pct` = capital_gain / net_capital_deployed * 100 — **NEW**

### Feedback items to resolve after verification:
fb_858ac27f, fb_39bb6c2b, fb_fda3bd07, fb_e0aeb97d, fb_948b4fee

---

## Scope

**In scope:**
- Add `TotalProceeds` field to Holding model
- Store `totalProceeds` in `holdingCalcMetrics` and assign to holdings
- FX-convert TotalProceeds alongside TotalInvested
- Redefine portfolio-level `TotalCost` to use trade-derived net capital (TotalInvested - TotalProceeds)
- Add `AvailableCash` field to Portfolio model
- Fix `TotalValue` = equity + available_cash (not equity + total_cash)
- Add `CapitalGain` and `CapitalGainPct` fields to Portfolio model
- Compute CapitalGain in handler after attaching CapitalPerformance
- Fix historical value aggregates (yesterday/lastweek) to use AvailableCash
- Fix ReviewPortfolio to use `liveTotal + portfolio.AvailableCash`
- Update glossary terms
- Update catalog description
- Update all affected tests
- Write docs/features/20260301-portfolio-value-definitions.md

**Out of scope:**
- Changes to per-holding TotalCost (remains AvgCost * Units for breakeven calculation)
- Changes to capital performance calculation
- Portal changes

---

## Files to Change

### 1. `docs/features/20260301-portfolio-value-definitions.md` — NEW FILE
Documentation of all portfolio response fields with formulas.

### 2. `internal/models/portfolio.go`

**Line 89 area — Add TotalProceeds to Holding struct:**
```go
TotalInvested       float64        `json:"total_invested"`        // Sum of all buy costs + fees (total capital deployed)
TotalProceeds       float64        `json:"total_proceeds"`        // Sum of all sell proceeds (units × price − fees)
```
Add after `TotalInvested` (line 89).

**Lines 49-59 — Add AvailableCash, CapitalGain, CapitalGainPct to Portfolio struct:**
```go
TotalValue               float64             `json:"total_value"`                      // equity holdings + available cash
TotalCost                float64             `json:"total_cost"`                       // net capital in equities (buys - sells, FX-adjusted)
TotalNetReturn           float64             `json:"total_net_return"`
TotalNetReturnPct        float64             `json:"total_net_return_pct"`
Currency                 string              `json:"currency"`
FXRate                   float64             `json:"fx_rate,omitempty"`
TotalRealizedNetReturn   float64             `json:"total_realized_net_return"`
TotalUnrealizedNetReturn float64             `json:"total_unrealized_net_return"`
CalculationMethod        string              `json:"calculation_method,omitempty"`
DataVersion              string              `json:"data_version,omitempty"`
TotalCash                float64             `json:"total_cash"`
AvailableCash            float64             `json:"available_cash"`                   // total_cash - total_cost (uninvested cash)
CapitalGain              float64             `json:"capital_gain,omitempty"`           // total_value - net_capital_deployed
CapitalGainPct           float64             `json:"capital_gain_pct,omitempty"`       // capital_gain / net_capital_deployed × 100
CapitalPerformance       *CapitalPerformance `json:"capital_performance,omitempty"`
```

### 3. `internal/services/portfolio/service.go`

**holdingCalcMetrics struct (~line 1640) — Add totalProceeds:**
```go
type holdingCalcMetrics struct {
	totalInvested      float64
	totalProceeds      float64  // NEW: sum of sell proceeds
	realizedGainLoss   float64
	unrealizedGainLoss float64
}
```

**Line 214 area — Store totalProceeds in metrics:**
The `calculateGainLossFromTrades` already returns `totalProceeds` as its second return value. Store it:
```go
holdingMetrics[h.Ticker] = &holdingCalcMetrics{
	totalInvested:      totalInvested,
	totalProceeds:      totalProceeds,  // ADD THIS LINE
	realizedGainLoss:   realizedGL,
	unrealizedGainLoss: unrealizedGL,
}
```

**Line 312-316 area — Assign TotalProceeds to holding from metrics:**
After line 315 (`holdings[i].UnrealizedNetReturn = m.unrealizedGainLoss`), add:
```go
holdings[i].TotalProceeds = m.totalProceeds
```

**Line 374 area — FX-convert TotalProceeds for USD holdings:**
In the USD FX conversion block (after `holdings[i].TotalInvested /= fxDiv`), add:
```go
holdings[i].TotalProceeds /= fxDiv
```

**Lines 387-441 — Replace portfolio build loop:**

Current (WRONG):
```go
var totalValue, totalCost, totalGain, totalDividends float64
var totalRealizedNetReturn, totalUnrealizedNetReturn float64
for _, h := range holdings {
	totalValue += h.MarketValue
	totalDividends += h.DividendReturn
	totalGain += h.NetReturn
	totalRealizedNetReturn += h.RealizedNetReturn
	totalUnrealizedNetReturn += h.UnrealizedNetReturn
	if h.Units > 0 {
		totalCost += h.TotalCost
	}
}
totalGain += totalDividends
```

New (CORRECT):
```go
var totalValue, totalCost, totalGain, totalDividends float64
var totalRealizedNetReturn, totalUnrealizedNetReturn float64
for _, h := range holdings {
	totalValue += h.MarketValue
	totalDividends += h.DividendReturn
	totalGain += h.NetReturn
	totalRealizedNetReturn += h.RealizedNetReturn
	totalUnrealizedNetReturn += h.UnrealizedNetReturn
	// Net capital in equities: buys - sells (all holdings, open + closed)
	totalCost += h.TotalInvested - h.TotalProceeds
}
totalGain += totalDividends
```

Key change: `totalCost` now sums `TotalInvested - TotalProceeds` for ALL holdings (not just open ones), giving net capital deployed into equities from trade history.

Then compute available cash and fix the portfolio construction:
```go
// Compute total cash balance from cashflow ledger (all accounts)
var totalCash float64
if s.cashflowSvc != nil {
	if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
		totalCash = ledger.TotalCashBalance()
	}
}

// Available cash = ledger balance minus capital locked in equities
availableCash := totalCash - totalCost

// Calculate weights using total value + available cash as denominator
weightDenom := totalValue + availableCash
for i := range holdings {
	if weightDenom > 0 {
		holdings[i].Weight = (holdings[i].MarketValue / weightDenom) * 100
	}
}

totalGainPct := 0.0
if totalCost > 0 {
	totalGainPct = (totalGain / totalCost) * 100
}

portfolio := &models.Portfolio{
	ID:                       name,
	Name:                     name,
	NavexaID:                 navexaPortfolio.ID,
	Holdings:                 holdings,
	TotalValueHoldings:       totalValue,
	TotalValue:               totalValue + availableCash,  // FIXED: was totalValue + totalCash
	TotalCost:                totalCost,                   // REDEFINED: net equity capital from trades
	TotalNetReturn:           totalGain,
	TotalNetReturnPct:        totalGainPct,
	Currency:                 navexaPortfolio.Currency,
	FXRate:                   fxRate,
	TotalRealizedNetReturn:   totalRealizedNetReturn,
	TotalUnrealizedNetReturn: totalUnrealizedNetReturn,
	CalculationMethod:        "average_cost",
	TotalCash:                totalCash,
	AvailableCash:            availableCash,  // NEW
	LastSynced:               time.Now(),
}
```

**Lines 567-577 — Fix historical value aggregates:**
Change from `+ portfolio.TotalCash` to `+ portfolio.AvailableCash`:
```go
// Set portfolio-level aggregates
if yesterdayTotal > 0 {
	portfolio.YesterdayTotal = yesterdayTotal + portfolio.AvailableCash
	if portfolio.YesterdayTotal > 0 {
		portfolio.YesterdayTotalPct = ((portfolio.TotalValue - portfolio.YesterdayTotal) / portfolio.YesterdayTotal) * 100
	}
}
if lastWeekTotal > 0 {
	portfolio.LastWeekTotal = lastWeekTotal + portfolio.AvailableCash
	if portfolio.LastWeekTotal > 0 {
		portfolio.LastWeekTotalPct = ((portfolio.TotalValue - portfolio.LastWeekTotal) / portfolio.LastWeekTotal) * 100
	}
}
```

**Lines 410-416 — Fix weight denominator:**
Change from `totalValue + totalCash` to `totalValue + availableCash` — shown in the build loop replacement above.

**Line 814 — Fix ReviewPortfolio TotalValue:**
Current: `review.TotalValue = liveTotal`
New: `review.TotalValue = liveTotal + portfolio.AvailableCash`

### 4. `internal/server/handlers.go`

**Lines 134-137 area — Compute CapitalGain after attaching CapitalPerformance:**

Current code (lines 134-137):
```go
// Attach capital performance if cash transactions exist (non-fatal on error)
if perf, err := s.app.CashFlowService.CalculatePerformance(ctx, name); err == nil && perf != nil && perf.TransactionCount > 0 {
	portfolio.CapitalPerformance = perf
}
```

Add after:
```go
// Attach capital performance if cash transactions exist (non-fatal on error)
if perf, err := s.app.CashFlowService.CalculatePerformance(ctx, name); err == nil && perf != nil && perf.TransactionCount > 0 {
	portfolio.CapitalPerformance = perf
	// Compute portfolio-level capital gain from deployed capital
	if perf.NetCapitalDeployed > 0 {
		portfolio.CapitalGain = portfolio.TotalValue - perf.NetCapitalDeployed
		portfolio.CapitalGainPct = (portfolio.CapitalGain / perf.NetCapitalDeployed) * 100
	}
}
```

### 5. `internal/server/glossary.go`

**buildValuationCategory function (lines 67-119) — Update terms:**

Replace the existing terms array with updated definitions. The key changes:
- `total_value` definition: "Portfolio value: equity holdings plus available (uninvested) cash."
  - Formula: "total_value_holdings + available_cash"
  - Example: uses TotalValueHoldings + AvailableCash
- `total_cost` definition: "Net capital deployed in equities (buy costs minus sell proceeds, FX-adjusted)."
  - Formula: "sum(total_invested - total_proceeds) for all holdings"
- Add new term `available_cash`:
  - Definition: "Uninvested cash: total cash ledger balance minus capital locked in equities."
  - Formula: "total_cash - total_cost"
  - Value: p.AvailableCash
- Update `total_capital` term:
  - Definition: "Total value of all assets: equity holdings plus total cash (ledger balance)."
  - Value: p.TotalValueHoldings + p.TotalCash (unchanged)
- Add `capital_gain` term:
  - Definition: "Overall portfolio gain: total value minus net capital deployed."
  - Formula: "total_value - net_capital_deployed"
  - Value: p.CapitalGain
- Add `capital_gain_pct` term:
  - Definition: "Overall portfolio gain as a percentage of net capital deployed."
  - Formula: "(capital_gain / net_capital_deployed) × 100"
  - Value: p.CapitalGainPct

### 6. `internal/server/catalog.go`

**Line 254 — Update get_portfolio description:**
Update to mention new fields:
```
"FAST: Get current portfolio holdings — tickers, names, values, weights, and net returns. Return percentages use total capital invested as denominator (average cost basis for partial sells). Includes realized/unrealized net return breakdown and true breakeven price (accounts for prior realized P&L). Includes portfolio and per-holding historical values (yesterday_total, yesterday_pct, last_week_total, last_week_pct from EOD data). Includes yesterday_net_flow and last_week_net_flow (net cash deposits minus withdrawals for adjusting daily/weekly change). Includes capital_performance (XIRR annualized return, simple return, total capital in/out) from manual transactions or auto-derived from trade history. Key value fields: total_value (equity + available cash), total_cost (net capital in equities from trades), available_cash (total_cash - total_cost), capital_gain/capital_gain_pct (vs net capital deployed). Trades are excluded from portfolio response; use get_portfolio_stock for trade history. No signals, charts, or AI analysis. Use portfolio_compliance for full analysis."
```

### 7. Tests to Update

**`internal/services/portfolio/service_test.go`:**

- `TestPopulateHistoricalValues` (line 3556): TotalCash=0 so AvailableCash=0, test expectations unchanged (yesterday=4800, lastweek=4600, no cash added)
- `TestPopulateHistoricalValues_WithExternalBalances` (line 3720): This test sets TotalCash=50000 and TotalValue=55000. After our fix, the portfolio must also have AvailableCash set. Update:
  - Set `AvailableCash: 50000.00` on the portfolio (since TotalCost is 0 in this test, available_cash = total_cash = 50000)
  - Update `TotalValue: 55000.00` to `TotalValue: 5000.00 + 50000.00` (this is correct, unchanged conceptually)
  - Expected yesterday total: 48*100 + 50000 = 54800 (unchanged)
  - Expected last week total: 46*100 + 50000 = 54600 (unchanged)

- Any `SyncPortfolio` tests that check `TotalCost` or `TotalValue` — update expectations. The totalCost in sync tests now comes from TotalInvested - TotalProceeds, not AvgCost * Units. For tests with a single buy trade (no sells), TotalInvested = buy cost and TotalProceeds = 0, so net capital = TotalInvested which is the same as the old totalCost if Units > 0. Verify each test carefully.

- For `TestSyncPortfolio_ZeroTotalCost_NoPercentDivByZero` (line 2576): This test has Units=0, TotalCost=0. With the new formula, totalCost = TotalInvested - TotalProceeds for ALL holdings. If the holding has no trades (no holdingMetrics entry), TotalInvested and TotalProceeds will both be 0, so totalCost = 0. Should still pass.

**`internal/server/glossary_test.go`:**
- Line 192: Add "available_cash", "capital_gain", "capital_gain_pct" to expected terms list

**`internal/server/handlers_portfolio_test.go`:**
- `TestHandlePortfolioGet_ReturnsPortfolio` (line 132): Sets TotalValue=200.0 directly on a pre-built portfolio (not via SyncPortfolio). This just checks the value round-trips. No change needed.
- `TestPortfolio_BackwardCompatibility_NoCapitalPerformance` (line 621): Deserializes old JSON. AvailableCash will be 0 (zero value). No change needed.

**`internal/server/catalog_test.go`:**
- Tool count remains 53 (no new tools added, just description change)

---

## Test Cases (Unit)

### New tests to write:

1. **TestSyncPortfolio_TotalCostFromTrades** — Verify totalCost = sum(TotalInvested - TotalProceeds) across all holdings
   - Setup: 2 holdings, one with buys only (TotalInvested=5000, TotalProceeds=0), one with partial sell (TotalInvested=3000, TotalProceeds=1000)
   - Expected: totalCost = 5000 + 2000 = 7000

2. **TestSyncPortfolio_AvailableCash** — Verify availableCash = totalCash - totalCost
   - Setup: totalCash=10000 from ledger, totalCost=7000 from trades
   - Expected: availableCash=3000, totalValue = equity + 3000

3. **TestSyncPortfolio_TotalValueFixed** — Verify totalValue uses availableCash not totalCash
   - Setup: equity=5000, totalCash=10000, totalCost=7000
   - Expected: totalValue = 5000 + 3000 = 8000 (NOT 5000 + 10000 = 15000)

4. **TestHolding_TotalProceeds_FXConverted** — Verify TotalProceeds gets FX-converted for USD holdings

5. **TestPopulateHistoricalValues_UsesAvailableCash** — Verify yesterday/lastweek use AvailableCash not TotalCash

---

## Integration Points

1. **holdingCalcMetrics** (service.go:1640): Add `totalProceeds` field
2. **metrics population** (service.go:214): Store `totalProceeds` from `calculateGainLossFromTrades`
3. **holding model population** (service.go:312-316): Assign `TotalProceeds` from metrics
4. **FX conversion** (service.go:374): Convert `TotalProceeds` alongside `TotalInvested`
5. **portfolio build loop** (service.go:387-441): Compute totalCost from trades, availableCash, fix TotalValue
6. **weight denominator** (service.go:410-416): Use availableCash not totalCash
7. **historical values** (service.go:567-577): Use AvailableCash not TotalCash
8. **ReviewPortfolio** (service.go:814): Add AvailableCash to liveTotal
9. **handler** (handlers.go:134-137): Compute CapitalGain after CapitalPerformance
10. **glossary** (glossary.go:67-119): Update/add terms
11. **catalog** (catalog.go:254): Update description
