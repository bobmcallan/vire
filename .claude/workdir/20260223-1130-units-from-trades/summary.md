# Summary: Recompute holding Units from trades instead of trusting Navexa

**Date:** 2026-02-23
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Added `math` import; updated `calculateAvgCostFromTrades` to return `(avgCost, totalCost, units)` instead of `(avgCost, totalCost)`; reordered sync loop to compute trade-derived units before gain/loss; added divergence warning log |
| `internal/services/portfolio/service_test.go` | Updated 8 existing call sites to capture 3 return values; added 3 new test functions (8 test cases total) |

## Root Cause

`calculateAvgCostFromTrades()` already computed `totalUnits` internally but discarded it, only returning `(avgCost, totalCost)`. The sync loop used `h.Units` directly from Navexa's EnrichedHoldings API, which could be stale. When Navexa's portfolio API and transaction API had different sync times, units showed 5,594 from the portfolio API while trades totalled 7,470.

## Fix

1. **Exposed trade-derived units** from `calculateAvgCostFromTrades` as a third return value
2. **Reordered the sync loop**: compute units/avgCost first, override `h.Units` with trade-derived value, recompute `h.MarketValue`, then compute gain/loss using corrected MarketValue
3. **Added warning log** when Navexa units diverge from trade-derived units (threshold: 0.01)

## Tests
- `TestAvgCost_ReturnsUnits` — 6 table-driven cases: buy only, buy+sell, fully closed, multiple buys+sell, opening balance, empty trades
- `TestAvgCost_UnitsMatchSnapshotReplay` — verifies `calculateAvgCostFromTrades` and `replayTradesAsOf` produce identical units
- `TestUnitsFromTrades_OverridesNavexaUnits` — end-to-end test demonstrating corrected gain/loss vs old inflated values
- All existing portfolio tests pass (0 regressions)
- Build and vet clean

## Notes
- The `replayTradesAsOf` function in `snapshot.go` already returned units correctly — no changes needed
- Feedback item: fb_19e84225
