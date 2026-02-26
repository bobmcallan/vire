# Summary: Portfolio Historical Values

**Date:** 2026-02-26
**Status:** completed

## Problem

The `get_portfolio` endpoint lacked historical comparison data. Users wanted to see:
- Yesterday's close value for each holding and the portfolio total
- Last week's (Friday) close value for each holding and the portfolio total
- Percentage changes from these historical points to today

## Solution

Added historical value fields to both `Holding` and `Portfolio` models, and implemented calculation logic in `GetPortfolio` that:
1. Batch loads market data for all active holdings
2. Uses EOD bars to find previous trading day close (offset 1) and last week close (offset 5)
3. Applies FX conversion for USD holdings
4. Aggregates holding values for portfolio-level totals
5. Includes external balances in portfolio aggregates

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added historical fields to Holding: `yesterday_close`, `yesterday_pct`, `last_week_close`, `last_week_pct` |
| `internal/models/portfolio.go` | Added aggregate fields to Portfolio: `yesterday_total`, `yesterday_total_pct`, `last_week_total`, `last_week_total_pct` |
| `internal/services/portfolio/service.go` | Added `populateHistoricalValues()` method to compute historical values |
| `internal/services/portfolio/service.go` | Added `findEODBarByOffset()` helper function |
| `internal/services/portfolio/service.go` | Modified `GetPortfolio()` to call `populateHistoricalValues()` |
| `internal/services/portfolio/service_test.go` | Fixed `GetMarketDataBatch()` in stub to return actual data |
| `internal/services/portfolio/service_test.go` | Added 6 new unit tests for historical values functionality |

## Tests

- **Unit tests added:** 6 new tests in `internal/services/portfolio/service_test.go`
  - TestFindEODBarByOffset - tests EOD bar offset lookup
  - TestPopulateHistoricalValues - basic functionality
  - TestPopulateHistoricalValues_WithUSDHolding - FX conversion for USD
  - TestPopulateHistoricalValues_WithExternalBalances - includes external balances
  - TestPopulateHistoricalValues_SkipsClosedPositions - skips units=0 holdings
  - TestPopulateHistoricalValues_InsufficientEODData - handles missing data gracefully

- **Test results:** All pass
- **Build:** Clean
- **Go vet:** Clean

## API Changes

### New Response Fields

**Holding level:**
```json
{
  "ticker": "BHP",
  "yesterday_close": 48.00,
  "yesterday_pct": 4.17,
  "last_week_close": 46.00,
  "last_week_pct": 8.70
}
```

**Portfolio level:**
```json
{
  "yesterday_total": 54800.00,
  "yesterday_total_pct": 0.36,
  "last_week_total": 54600.00,
  "last_week_total_pct": 0.73
}
```

All new fields are optional (omitempty) and only populated when sufficient EOD data exists.

## Notes

- EOD bars are sorted descending (index 0 = most recent), so offset 1 = yesterday, offset 5 = ~last week
- "Last week" is approximated as 5 trading days back, not calendar weeks
- Holiday handling is out of scope - trading calendar is derived from available EOD bars
- FX conversion uses the same rate stored on the portfolio (fxRate from sync time)
- External balances are assumed constant and included in portfolio-level historical totals
- Closed positions (units = 0) are skipped
