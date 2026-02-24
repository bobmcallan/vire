# Requirements: Fix price data pipeline (fb_7a4c19e9, fb_b4134cbf, fb_52547389, fb_7d22e342)

**Date:** 2026-02-24
**Requested:** Fix stale/incorrect prices in portfolio sync, compute_indicators, and force_refresh

## Feedback Items

| ID | Severity | Issue |
|----|----------|-------|
| fb_7a4c19e9 | high | DFND.AU compute_indicators shows $43.855, get_quote returns $40.00 |
| fb_b4134cbf | high | force_refresh on get_portfolio doesn't trigger Navexa sync |
| fb_52547389 | high | ACDC.AU price stuck at $5.11 (should be ~$146) |
| fb_7d22e342 | high | Duplicate of ACDC + capital flow (capital flow already implemented) |

## Root Cause Analysis

### Bug 1: `get_portfolio` has no `force_refresh` (fb_b4134cbf)

- `handlePortfolioGet` in `internal/server/handlers.go:107` calls `GetPortfolio()` which returns cached data if within `FreshnessPortfolio` (30min).
- There is NO `force_refresh` parameter on the MCP tool definition, the HTTP handler, or the service call.
- `get_portfolio_stock` does have `force_refresh` (handlers.go:138), but `get_portfolio` does not.
- User expectation: passing `force_refresh=true` should trigger `SyncPortfolio(ctx, name, true)`.

### Bug 2: ACDC AdjClose is wrong (fb_52547389)

- During `SyncPortfolio`, the EODHD cross-check (service.go:188-235) overrides Navexa prices with EODHD's AdjClose via `eodClosePrice()`.
- `eodClosePrice()` (service.go:1327) prefers AdjClose when positive — no sanity check.
- ACDC underwent a corporate action (consolidation). EODHD returns AdjClose=$5.11 (pre-consolidation adjusted) while Close=~$146 (actual trading price).
- For the MOST RECENT bar, AdjClose should converge with Close. A massive divergence indicates bad data.
- The cross-check replaces Navexa's correct ~$146 with EODHD's wrong $5.11 → $40k portfolio understatement.

### Bug 3: `compute_indicators` uses stale cached EOD data (fb_7a4c19e9)

- `compute_indicators` → `DetectSignals()` → reads from `MarketDataStorage().GetMarketData()` (cached MarketFS).
- Signals are computed from `bars[0].Close` — the most recent cached EOD bar.
- If EOD data was collected hours/days ago, the price and all derived indicators (SMA, RSI, MACD) are wrong.
- `get_quote` uses the real-time EODHD API, so it shows the correct price.
- The signals pipeline has no live price overlay or staleness warning.

## Scope

### In scope
1. Add `force_refresh` parameter to `get_portfolio` MCP tool and HTTP handler
2. Fix `eodClosePrice()` to detect bad AdjClose values (divergence from Close)
3. Overlay real-time quote on signals pipeline so compute_indicators uses current prices
4. Mark fb_7d22e342 resolved (ACDC covered by fix, capital flow already implemented)

### Out of scope
- Auto-refresh during market hours (separate feature, needs watcher changes)
- Capital flow TWRR/MWRR calculation (already implemented)

## Approach

### Fix 1: Add `force_refresh` to `get_portfolio`

**Files:**
- `internal/server/catalog.go` — Add `force_refresh` boolean param to `get_portfolio` tool definition
- `internal/server/handlers.go` — `handlePortfolioGet()`: read `force_refresh` query param, call `SyncPortfolio(ctx, name, true)` when set

### Fix 2: AdjClose sanity check in `eodClosePrice()`

**Files:**
- `internal/services/portfolio/service.go` — Modify `eodClosePrice()`:
  - For the latest bar, if AdjClose diverges from Close by more than 50%, fall back to Close
  - Log a warning when this happens so bad data can be tracked
  - The function currently takes a single bar — add a logger parameter or make it a method on Service

Implementation: Change `eodClosePrice()` to also take Close as a reference. If `|AdjClose - Close| / Close > 0.5`, prefer Close. This catches consolidation/split data errors while still using AdjClose for legitimate adjustments (which are typically small, <5%).

### Fix 3: Live price overlay in signals pipeline

**Files:**
- `internal/services/signal/service.go` — `DetectSignals()` or `ComputeSignals()`:
  - Before computing signals, fetch real-time quote for each ticker
  - If the latest EOD bar date is today and quote is available, update `bars[0].Close` with the live price
  - If the latest EOD bar is from a previous day, prepend a synthetic bar for today using the live quote
  - This ensures indicators reflect current market conditions

Alternative (simpler): Add the EODHD client to the signal service. In `DetectSignals`, fetch a real-time quote per ticker and overlay it onto the cached bars before computing.

**Dependencies:**
- Signal service needs access to EODHD client (currently it only has storage)
- Pass EODHD client through constructor: `NewService(storage, eodhd, logger)`
- The interface is `interfaces.EODHDClient` — `GetRealTimeQuote(ctx, ticker) (*models.RealTimeQuote, error)`

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/server/catalog.go` | Add `force_refresh` param to `get_portfolio` |
| `internal/server/handlers.go` | `handlePortfolioGet` reads force_refresh, calls SyncPortfolio |
| `internal/services/portfolio/service.go` | Fix `eodClosePrice()` with divergence check |
| `internal/services/signal/service.go` | Add EODHD client, overlay live quotes |
| `internal/services/signal/service_test.go` | New: unit tests for live overlay |
| `internal/services/portfolio/service_test.go` | Unit tests for `eodClosePrice()` divergence |
| `internal/app/app.go` | Pass EODHD client to signal service constructor |

## Acceptance Criteria

1. `get_portfolio` with `force_refresh=true` triggers a fresh Navexa sync and returns updated data
2. ACDC price displays correctly (~$146, not $5.11) after sync
3. `compute_indicators` for DFND returns price matching `get_quote` (within reasonable tolerance)
4. All existing unit tests pass
5. All existing integration tests pass
6. No regressions in signal computation accuracy
