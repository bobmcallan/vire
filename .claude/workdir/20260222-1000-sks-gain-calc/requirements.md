# Requirements: Fix Portfolio Gain/Loss % Calculation

**Date:** 2026-02-22
**Requested:** Fix incorrect capital gain % calculation for SKS position in Vire MCP

## Problem

SKS position shows 10.85% capital gain but the correct figure is ~5.82% (simple return) or ~5.23% (Navexa, including brokerage and realised losses).

**Transaction history:**
- Buy 24/12/2025: 4,925 @ $4.0248 = $19,825.14
- Sell 22/01/2026: 1,333 @ $3.7627 = $5,012.68 (loss)
- Sell 27/01/2026: 819 @ $3.680 = $3,010.92 (loss)
- Sell 29/01/2026: 2,773 @ $3.4508 = $9,566.07 (loss)
- Buy 05/02/2026: 2,511 @ $3.980 = $9,996.78
- Buy 13/02/2026: 2,456 @ $4.070 = $9,998.92

**Current holding:** 4,967 units, correct cost basis = $19,995.70
**Avg cost shown by Vire:** $4.0257 (appears to be original buy price, not current cost basis)
**Capital gain % shown:** 10.85% (wrong — uses wrong cost basis)
**Expected:** ~5.82% simple return on current holdings ($19,995.70 cost basis)

## Root Cause Analysis

Three issues identified:

### Issue 1: GainLossPct and CapitalGainPct are Navexa IRR values, never overwritten

In `internal/services/portfolio/service.go` lines 130-150:
- `GainLoss` (dollar amount) is correctly recalculated from trades via `calculateGainLossFromTrades()`
- `TotalCost` (remaining cost basis) is correctly calculated via `calculateAvgCostFromTrades()`
- But `GainLossPct`, `CapitalGainPct`, and `TotalReturnPct` are left as Navexa's IRR p.a. values (set in `GetEnrichedHoldings()` at `client.go:312-316`)
- These IRR values are annualized internal rates of return, NOT simple gain percentages
- The fix: compute simple % locally: `GainLossPct = GainLoss / TotalCost * 100` (when TotalCost > 0)
- Similarly for `CapitalGainPct` and `TotalReturnPct`

### Issue 2: No force-refresh on get_portfolio_stock MCP tool

- `handlePortfolioStock()` calls `GetPortfolio()` which only syncs if stale (>30 min per `FreshnessPortfolio`)
- There's no `force_refresh` parameter on the `get_portfolio_stock` MCP tool
- The `/api/portfolios/{name}/sync` endpoint supports `force: true`, but the stock endpoint doesn't
- The fix: add optional `force_refresh` boolean param to `get_portfolio_stock` MCP tool and handler

### Issue 3: AvgCost calculation uses running average across all trades

The `calculateAvgCostFromTrades()` function uses a running weighted average across ALL buys/sells.
For SKS: the first buy at $4.0248 is blended with the proportional reduction from sells, then the Feb re-buys.
This gives $4.0257 as avg cost — but the current position was entirely created by the Feb buys at $3.980 and $4.070.

The avg cost of the current position should be: $19,995.70 / 4,967 = $4.0257... Actually this IS correct.
The running average correctly reduces cost proportionally on sells and adds on buys.

Let me verify: After buying 4925 @ $4.0248 (+ fees), selling all 4925, then buying 2511 @ $3.98 and 2456 @ $4.07:
- After first buy: cost = $19,825.14, units = 4925, avg = $4.0254
- After sells: cost reduces proportionally to 0, units = 0
- After 2nd buy: cost = $9,996.78, units = 2511, avg = $3.9812
- After 3rd buy: cost = $19,995.70, units = 4967, avg = $4.0257

So AvgCost of $4.0257 is actually correct for the remaining position's cost basis.
The issue is the % calculation: $3,398.87 / $19,995.70 = 17.0% or using the wrong divisor.

Actually re-checking: if `TotalCost` = `remainingCost` = $19,995.70 and `GainLoss` = $1,163.40:
- GainLossPct should be: $1,163.40 / $19,995.70 * 100 = 5.82%
- But it shows 10.85% from Navexa IRR

The core fix is: **replace Navexa's IRR % with locally-computed simple %**.

## Scope
- Fix GainLossPct, CapitalGainPct, TotalReturnPct calculations in SyncPortfolio
- Add force_refresh param to get_portfolio_stock MCP tool
- Unit tests for the % calculation fix

## Approach

1. In `SyncPortfolio()` (service.go), after computing `GainLoss` and `TotalCost` from trades, compute simple percentages:
   ```go
   if h.TotalCost > 0 {
       h.GainLossPct = (h.GainLoss / h.TotalCost) * 100
       h.CapitalGainPct = h.GainLossPct  // same for capital gain
   }
   if h.TotalCost > 0 {
       h.TotalReturnPct = (h.TotalReturnValue / h.TotalCost) * 100
   }
   ```

2. Also recompute % after the EODHD price cross-check (lines 153-185)

3. Add `force_refresh` to `get_portfolio_stock`:
   - Add param to MCP catalog definition
   - Parse query param in handler
   - If true, call `SyncPortfolio(ctx, name, true)` instead of `GetPortfolio()`

4. Update model comments to clarify these are now simple % not IRR

## Files Expected to Change
- `internal/services/portfolio/service.go` — fix % calculation
- `internal/server/handlers.go` — add force_refresh to handlePortfolioStock
- `internal/server/catalog.go` — add force_refresh param to MCP tool definition
- `internal/models/portfolio.go` — update field comments
- `internal/services/portfolio/service_test.go` — add/update tests
