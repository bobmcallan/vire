# Requirements: Fix GainLoss calculation when EODHD price cross-check fires

**Date:** 2026-02-21
**Requested:** Vire calculations for SKS are incorrect — positions with partial sells and re-entries show wrong gain/loss because realised losses from sells are dropped when EODHD price cross-check updates the price.

## The Bug

In `internal/services/portfolio/service.go:182`, when the EODHD price cross-check fires (EODHD has a more recent close than Navexa), the code recalculates GainLoss:

```go
h.GainLoss = h.MarketValue - h.TotalCost  // BUG: drops realised gains/losses
```

But `h.TotalCost` is the **remaining cost basis** (cost of currently-held units after proportional reduction from sells), not the total invested. For positions with sells, this formula ignores all realised gains/losses.

### SKS.AU Example

Trades: Buy 4,925 → Sell 1,333+819+2,773 (at a loss) → Re-buy 2,511+2,456

- `totalInvested` = $39,820.84 (all 3 buys)
- `totalProceeds` = $17,589.67 (all 3 sells, below cost)
- `TotalCost` (remaining) = $19,995.70 (cost of the 4,967 units currently held)
- At $4.71: `MarketValue` = $23,394.57

Correct GainLoss: `(17,589.67 + 23,394.57) - 39,820.84 = $1,163.40`
Bug GainLoss: `23,394.57 - 19,995.70 = $3,398.87` — overstated by ~$2,235

The $2,235 difference is exactly the realised loss from the January sells that gets dropped.

## Scope
- Fix the EODHD price cross-check recalculation at service.go:178-184
- Add unit tests for partial sell + re-entry GainLoss scenarios
- Verify holdingTrades map doesn't overwrite when multiple Navexa holdings share a ticker

## Approach

### Fix 1: Preserve realised component on price update
Instead of recalculating GainLoss from scratch with the wrong formula, adjust by the price delta:

```go
oldMarketValue := h.MarketValue
h.CurrentPrice = latestBar.Close
h.MarketValue = h.CurrentPrice * h.Units
h.GainLoss += h.MarketValue - oldMarketValue  // preserves realised component
h.TotalReturnValue = h.GainLoss + h.DividendReturn
```

This works because the GainLoss was correctly calculated earlier by `calculateGainLossFromTrades` using the full formula `(proceeds + marketValue) - totalInvested`. Adjusting by the market value delta preserves the realised portion.

Remove the `if h.TotalCost > 0` guard — the delta approach works for all positions.

### Fix 2: Defensive — holdingTrades map append
At service.go:128, `holdingTrades[h.Ticker] = trades` overwrites if Navexa returns multiple holdings for the same ticker. Change to append:
```go
holdingTrades[h.Ticker] = append(holdingTrades[h.Ticker], trades...)
```

## Files Expected to Change
- `internal/services/portfolio/service.go` — fix lines 178-184, fix holdingTrades map
- `internal/services/portfolio/service_test.go` — add test for partial sell + re-entry GainLoss
