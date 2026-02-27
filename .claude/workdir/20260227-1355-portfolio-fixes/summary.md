# Summary: Fix 3 Portfolio Feedback Items

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Call `populateHistoricalValues()` from `SyncPortfolio()` |
| `internal/services/portfolio/service_test.go` | Tests for historical field population |
| `internal/services/portfolio/historical_values_stress_test.go` | 25 stress tests for edge cases |
| `internal/services/cashflow/service.go` | Add `deriveFromTrades()` fallback for empty ledger |
| `internal/services/cashflow/service_test.go` | Tests for trade-based capital derivation |
| `internal/services/cashflow/derive_trades_stress_test.go` | 18 stress tests |
| `internal/models/portfolio.go` | Add `TimeSeriesPoint` model, `TimeSeries` field on `PortfolioIndicators` |
| `internal/services/portfolio/indicators.go` | Add `growthPointsToTimeSeries()`, populate TimeSeries |
| `internal/services/portfolio/timeseries_stress_test.go` | 17 stress tests |
| `internal/server/catalog.go` | Updated descriptions for indicators/capital_performance tools |
| `docs/architecture/services.md` | Added historical values, time series, trade fallback docs |
| `README.md` | Updated tool descriptions |
| `tests/api/portfolio_fixes_test.go` | 6 integration tests |

## Bugs/Features Fixed

### Bug 1: Missing Yesterday/Week Fields After Restart (fb_fb956a5e)
- **Root cause:** `SyncPortfolio()` didn't call `populateHistoricalValues()`
- **Fix:** Added call before return, fields now persist through sync cycle
- **Impact:** 8 fields (4 portfolio-level + 4 per-holding) now always populated

### Feature 2: Capital Performance Auto-Derive from Trades (fb_742053d8)
- **Root cause:** Empty cash ledger returned all-zero struct
- **Fix:** When ledger empty, fall back to deriving from Navexa trade history
- **Implementation:** `deriveFromTrades()` sums buy trades as deposited, sell as withdrawn, computes returns
- **Precedence:** Manual transactions take priority when present

### Feature 3: Expose Portfolio Value Time Series (fb_cafb4fa0)
- **Root cause:** 65+ daily values computed internally but not returned
- **Fix:** Added `TimeSeries []TimeSeriesPoint` to `PortfolioIndicators` response
- **Fields:** date, value (includes external balances), cost, net_return, net_return_pct, holding_count
- **Backward compatible:** `omitempty` tag, no handler changes needed

### Bonus: Nil Dereference Fix
- `findEODBarByOffset` panicked on negative offset — fixed by devils-advocate finding

## Tests
- 60+ stress tests added (historical values, trade derivation, time series)
- 6 integration tests created
- All modified packages pass
- Pre-existing failures: feedback sorting, storage nil pointer, config (unrelated)

## Architecture
- Architect reviewed: no circular dependencies, trade derivation reuses XIRR pattern
- Docs updated: services.md, catalog.go, README.md

## Devils-Advocate
- Found negative offset panic in `findEODBarByOffset` — fixed
- Verified FX rate handling, XIRR cap, currency edge cases
- 60 stress tests passing

## Notes
- Trade-based fallback is non-fatal — returns empty struct if Navexa unavailable
- Time series uses `omitempty` — absent when no growth data exists
- Historical values depend on market data being collected — fields will be zero until first EOD collection
