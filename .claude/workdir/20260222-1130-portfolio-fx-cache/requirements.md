# Requirements: Fix portfolio FX conversion and stale cache zeros

**Date:** 2026-02-22
**Requested:** Two bugs in portfolio API response:
1. CBOE holding reports values in USD but portfolio is AUD — no per-holding currency conversion
2. All `net_return`, `realized_net_return`, `unrealized_net_return` fields are 0 despite correct percentage values

## Scope
- Fix per-holding FX conversion (USD → AUD) for monetary values
- Fix stale cache deserialization causing zero values after JSON field rename
- Unit tests for both fixes
- Integration tests
- Out of scope: Navexa API changes, new FX pairs beyond AUDUSD

## Root Cause Analysis

### Bug 1: CBOE in USD
The Navexa client uses `showLocalCurrency=false` which returns values in each holding's native currency. For CBOE (NYSE), values are in USD. Portfolio-level totals correctly convert to AUD using the AUDUSD FX rate, but per-holding values (`MarketValue`, `AvgCost`, `TotalCost`, `NetReturn`, etc.) stay in USD.

**Fix:** After computing all values, convert USD holdings' monetary fields to AUD using the FX rate. Store the original currency in a new field for reference.

### Bug 2: net_return = 0
The refactor commit `8054b79` renamed Holding JSON tags:
- `gain_loss` → `net_return`
- `gain_loss_pct` → `net_return_pct`
- `realized_gain_loss` → `realized_net_return`
- `unrealized_gain_loss` → `unrealized_net_return`
- `total_gain` → `total_net_return` (portfolio level)

The cached portfolio in the UserDataStore was serialized with old JSON keys. When `GetPortfolio` loads and deserializes with the new struct, renamed fields default to 0. The `net_return_pct` appeared correct by coincidence: the old struct had a `*float64 NetReturnPct json:"net_return_pct"` derived field with the same JSON key.

**Fix:** Add a `DataVersion` field to the Portfolio model. When loading a cached portfolio, if `DataVersion` doesn't match the current `common.SchemaVersion`, force a re-sync regardless of freshness.

## Approach

### Fix 1: Per-holding FX conversion
In `SyncPortfolio`, after the holdings conversion loop (line 265), add FX conversion for USD holdings:
- Convert monetary fields: `CurrentPrice`, `AvgCost`, `MarketValue`, `TotalCost`, `TotalInvested`, `NetReturn`, `RealizedNetReturn`, `UnrealizedNetReturn`, `DividendReturn`, `TrueBreakevenPrice`
- Set holding `Currency` to portfolio currency (AUD) after conversion
- Store original currency in a new `OriginalCurrency` field on the Holding model
- The portfolio-level totals loop already handles FX — with per-holding conversion, the totals loop multiplier becomes 1.0 for all holdings (since they're now all in AUD)

### Fix 2: Cache invalidation on schema change
- Add `DataVersion string json:"data_version,omitempty"` to Portfolio struct
- In `savePortfolioRecord`, set `DataVersion = common.SchemaVersion`
- In `getPortfolioRecord`, after unmarshal, check if `DataVersion != common.SchemaVersion` — if so, return an error to trigger re-sync
- Bump `common.SchemaVersion` to "7"

## Files Expected to Change
- `internal/models/portfolio.go` — add `DataVersion`, `OriginalCurrency` fields
- `internal/services/portfolio/service.go` — FX conversion logic, data version save/check
- `internal/common/version.go` — bump SchemaVersion to "7"
- `internal/services/portfolio/service_test.go` or new test file — unit tests
