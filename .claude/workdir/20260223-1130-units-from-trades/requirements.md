# Requirements: Recompute holding Units from trades instead of trusting Navexa

**Date:** 2026-02-23
**Requested:** Fix feedback item fb_19e84225 — `force_refresh` fetches fresh trades from Navexa but holding aggregates (units, avg_cost, net_return) are not recalculated atomically. Units showing 5,594 from Navexa's EnrichedHoldings API while trades array shows 4 buys totalling 7,470. The root cause is that `h.Units` is taken directly from Navexa, not recomputed from trades.

## Scope
- **In scope:** Recompute `Units` (and derived `MarketValue`) from the trades array when trades are available, instead of trusting Navexa's potentially-stale value. Add a warning log when Navexa units diverge from trade-derived units.
- **Out of scope:** Changes to Navexa client, MCP tool handlers, storage layer, or API response shapes.

## Approach

**Root cause:** `calculateAvgCostFromTrades()` in `internal/services/portfolio/service.go:1005` already computes `totalUnits` internally (tracking buys/sells) but only returns `(avgCost, totalCost)` — discarding the units. Line 239 then uses `h.Units` directly from Navexa's EnrichedHoldings API.

**Fix (3 changes in `internal/services/portfolio/service.go`):**

1. **Change `calculateAvgCostFromTrades` signature** to return `(avgCost, totalCost, totalUnits float64)` — expose the units it already computes.

2. **In the sync loop (lines ~128-170):** After calling `calculateAvgCostFromTrades`, override `h.Units` with the trade-derived units when they differ. Recompute `h.MarketValue = h.CurrentPrice * h.Units`. Log a warning when Navexa units diverge from trade-derived units (for monitoring).

3. **Update all call sites of `calculateAvgCostFromTrades`** — the only caller is in `SyncPortfolio` around line 136. Use `_` for the new return value at any other call site if needed.

**Downstream effects that are automatically handled:**
- `h.GainLoss` and `h.GainLossPct` are computed *after* units/market value are set (lines 133-154), using `calculateGainLossFromTrades` which takes `h.MarketValue` — so the corrected MarketValue flows through.
- Price cross-check (lines 176-218) uses `h.Units` and `h.MarketValue` — corrected values flow through.
- `TrueBreakevenPrice` (line 262) divides by `holdings[i].Units` — corrected value flows through.

**Note:** The `calculateGainLossFromTrades` call at line 133 happens *before* the avgCost call at line 136 and uses `h.MarketValue`. We need to reorder: compute units first, update MarketValue, *then* compute gain/loss. This reordering is the key change.

## Files Expected to Change
- `internal/services/portfolio/service.go` — the only file that needs changes
