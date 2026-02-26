# Requirements: Price Momentum Fields for get_stock_data and get_quote

**Date:** 2026-02-27
**Requested:** Add day and weekly change fields to `get_stock_data` and `get_quote` tools (fb_cfa52eb5, fb_565969a2)

## Scope

**In Scope:**
- Add `yesterday_close`, `yesterday_pct`, `last_week_close`, `last_week_pct` to `get_stock_data` response (PriceData)
- Add same fields to `get_quote` response (RealTimeQuote)
- Reuse existing EOD data from storage to calculate historical percentages
- Use same calculation pattern as `get_portfolio` (offset 1 for yesterday, offset 5 for last week)

**Out of Scope:**
- Changes to `get_portfolio` (already implemented)
- New API endpoints
- Changes to signal computation

## Approach

1. **Add fields to PriceData model** (internal/models/market.go)
   - YesterdayClose, YesterdayPct, LastWeekClose, LastWeekPct

2. **Add fields to RealTimeQuote model** (internal/models/market.go)
   - Same 4 fields for `get_quote` consistency

3. **Modify MarketService.GetStockData** (internal/services/market/service.go)
   - After populating current price data, calculate historical percentages from EOD bars
   - Use `eodClosePrice()` helper (prefers AdjClose) for consistency with portfolio
   - Yesterday = EOD[1] (second bar), Last week = EOD[5] (6th bar)

4. **Modify QuoteService.GetRealTimeQuote** (internal/services/quote/service.go)
   - Fetch EOD data from storage to calculate historical percentages
   - Same offset pattern: 1 for yesterday, 5 for last week

## Files Expected to Change

- `internal/models/market.go` - Add fields to PriceData and RealTimeQuote
- `internal/services/market/service.go` - Populate historical fields in GetStockData
- `internal/services/quote/service.go` - Populate historical fields in GetRealTimeQuote
- `internal/services/market/service_test.go` - Unit tests for new functionality
- `internal/services/quote/service_test.go` - Unit tests for new functionality
