# Summary: Price Momentum Fields for get_stock_data and get_quote

**Date:** 2026-02-27
**Status:** completed
**Feedback Items:** fb_cfa52eb5, fb_565969a2

## Problem

`get_stock_data` and `get_quote` lacked day and weekly price change fields that `get_portfolio` already provides. This meant users had to call `get_portfolio` just to get price momentum data for a single ticker.

## Solution

Added 4 historical price fields to both `PriceData` (for `get_stock_data`) and `RealTimeQuote` (for `get_quote`):
- `yesterday_close` - Previous trading day's close (EOD[1])
- `yesterday_pct` - % change from yesterday's close to current
- `last_week_close` - Close from ~5 trading days ago (EOD[5])
- `last_week_pct` - % change from last week's close to current

## What Changed

| File | Change |
|------|--------|
| `internal/models/market.go` | Added 4 historical fields to `PriceData` and `RealTimeQuote` |
| `internal/services/market/service.go` | Added calculation logic in `GetStockData` to populate historical fields from EOD bars |
| `internal/services/quote/service.go` | Added storage dependency, `populateHistoricalFields()` method to fetch EOD data and calculate historical percentages |
| `internal/app/app.go` | Updated `NewService` call to pass storage to quote service |
| `internal/services/market/service_test.go` | Added 3 unit tests for historical fields |
| `internal/services/quote/service_test.go` | Added mock storage and 3 unit tests for historical fields |

## Tests

**Unit tests added:** 6
- `TestGetStockData_HistoricalFields` - Basic functionality
- `TestGetStockData_HistoricalFields_InsufficientEOD` - Graceful handling when not enough EOD bars
- `TestGetStockData_HistoricalFields_ZeroClose` - Division by zero guard
- `TestPopulateHistoricalFields` - Quote service historical calculation
- `TestPopulateHistoricalFields_NoStorage` - Graceful handling when storage unavailable
- `TestPopulateHistoricalFields_NoEODData` - Graceful handling when no EOD data

**Test results:** All pass
**Build:** Clean
**Go vet:** Clean

## API Changes

### get_stock_data Response

Price section now includes:
```json
{
  "price": {
    "current": 50.00,
    "previous_close": 48.00,
    "change": 2.00,
    "change_pct": 4.17,
    "yesterday_close": 48.00,
    "yesterday_pct": 4.17,
    "last_week_close": 44.00,
    "last_week_pct": 13.64
  }
}
```

### get_quote Response

```json
{
  "code": "BHP.AU",
  "close": 50.00,
  "previous_close": 48.00,
  "yesterday_close": 48.00,
  "yesterday_pct": 4.17,
  "last_week_close": 44.00,
  "last_week_pct": 13.64
}
```

## Notes

- Fields are optional (`omitempty`) - only populated when sufficient EOD data exists
- Uses same calculation pattern as portfolio (EOD[1] for yesterday, EOD[5] for last week)
- Quote service gracefully handles missing storage or EOD data by omitting the fields
- Division by zero guard prevents errors when historical close is 0
