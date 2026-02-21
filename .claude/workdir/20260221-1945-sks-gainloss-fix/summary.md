# Summary: Fix GainLoss calculation for partial sell + re-entry positions

**Date:** 2026-02-21
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Fixed EODHD price cross-check to preserve realised gains/losses using delta adjustment instead of `MarketValue - TotalCost`; fixed holdingTrades map to append instead of overwrite |
| `internal/services/portfolio/service_test.go` | Added 795 lines of tests: SKS partial sell + re-entry scenario, price update preservation, pure buy-and-hold regression, holdingTrades multi-holding, stress tests |

## Tests
- Unit tests: 80+ portfolio tests pass
- Data integration tests: 22 pass (including 3 new GainLoss tests)
- API integration tests: 20 pass (including 40 GainLoss subtests)
- Test feedback rounds: 1 (hasSells case-insensitive check fixed)
- `go vet ./...` clean
- Build succeeds

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — updated if applicable

## Devils-Advocate Findings
- Edge cases with zero-unit positions, negative prices, large trade counts — all handled correctly by delta approach
- No race conditions identified (sync is single-threaded per portfolio)

## Key Fix

### EODHD Price Cross-Check Bug (Root Cause)

**Before (broken):**
```go
h.GainLoss = h.MarketValue - h.TotalCost  // drops realised gains/losses
```

**After (fixed):**
```go
oldMarketValue := h.MarketValue
h.CurrentPrice = latestBar.Close
h.MarketValue = h.CurrentPrice * h.Units
h.GainLoss += h.MarketValue - oldMarketValue  // preserves realised component
h.TotalReturnValue = h.GainLoss + h.DividendReturn
```

When `calculateGainLossFromTrades` runs, it correctly computes `GainLoss = (proceeds + marketValue) - totalInvested`, which includes realised gains/losses from sells. The old code then **overwrote** this with `MarketValue - TotalCost` (remaining cost basis only) when the EODHD price differed from Navexa. The fix adjusts by the price delta, preserving the realised component.

### holdingTrades Map Overwrite (Defensive Fix)

Changed `holdingTrades[h.Ticker] = trades` to `holdingTrades[h.Ticker] = append(holdingTrades[h.Ticker], trades...)` to prevent trade loss if Navexa returns multiple holdings for the same ticker.

## SKS.AU Verification

With the user's trades:
- Buys: 4,925 + 2,511 + 2,456 = 9,892 units for $39,820.84
- Sells: 1,333 + 819 + 2,773 = 4,925 units for $17,589.67 (all below cost)
- Current: 4,967 units, remaining cost = $19,995.70

At $4.71:
- **Correct:** GainLoss = ($17,589.67 + $23,394.57) - $39,820.84 = **$1,163.40**
- **Old bug:** GainLoss = $23,394.57 - $19,995.70 = $3,398.87 (overstated by $2,235)

## Notes
- The bug only manifested when EODHD had a more recent close than Navexa (price cross-check fired)
- Pure buy-and-hold positions were unaffected (TotalCost = totalInvested when no sells)
- `golangci-lint` couldn't run (Go version mismatch)
