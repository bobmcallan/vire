# Requirements: Fresh Intraday Stock Pricing

**Date:** 2026-02-13
**Requested:** Vire returns stale price data for stocks/commodities, causing misleading answers. Ensure data freshness is visible and warned about.

## Problem

EODHD `/real-time/{ticker}` endpoint sometimes returns delayed data (e.g., XAGUSD showed $79.84 when actual was $74.93). The current code:
1. Shows the timestamp but does NOT warn when data is stale
2. The `get_stock_data` MCP tool uses EOD (end-of-day) data for prices, which is inherently yesterday's close during market hours
3. No staleness metadata is returned in the API response for consumers to assess data freshness

## Scope

### In scope
- Add staleness detection to `GetRealTimeQuote` response — calculate data age from timestamp
- Add visible staleness warning in `formatQuote` when data is >5 minutes old
- Add `data_age` and `stale` fields to the server JSON response for `/api/market/quote/`
- Update `get_stock_data` to fetch real-time price (via GetRealTimeQuote) instead of relying solely on EOD data when the EODHD client is available
- Add tests for staleness detection and formatting

### Out of scope
- WebSocket streaming / persistent connections
- Intraday historical (minute bars) API — separate feature
- Changing subscription tiers

## Approach

1. **Model change:** Add `DataAge` (duration) and `Stale` (bool) fields to `RealTimeQuote`
2. **Client change:** Compute data age in `GetRealTimeQuote` by comparing response timestamp to `time.Now()`
3. **Formatter change:** `formatQuote` adds a warning banner when `Stale` is true, showing how old the data is
4. **Server handler:** Include `data_age_seconds` and `stale` in JSON response
5. **Stock data integration:** In the market service's `GetStockData`, also fetch real-time price and populate `PriceData.LastUpdated` with the real-time timestamp

## Files Expected to Change
- `internal/models/market.go` — add staleness fields to RealTimeQuote
- `internal/clients/eodhd/client.go` — compute data age in GetRealTimeQuote
- `cmd/vire-mcp/formatters.go` — staleness warning in formatQuote
- `cmd/vire-mcp/formatters_test.go` — test staleness formatting
- `internal/server/handlers.go` — include staleness in JSON response
- `internal/server/handlers_quote_test.go` — test staleness in response
