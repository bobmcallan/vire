# Plan: Implement `get_quote` MCP Tool

## Summary

Add a new `get_quote` MCP tool that fetches real-time OHLCV quotes from EODHD for any ticker (forex, commodities, stocks). The tool follows the existing architecture: MCP tool -> REST endpoint -> EODHD client.

## Architecture

The existing pattern is:
1. **MCP tool definition** (`cmd/vire-mcp/tools.go`) - `createGetQuoteTool()` defines the tool schema
2. **MCP handler** (`cmd/vire-mcp/handlers.go`) - `handleGetQuote(p)` calls the REST API via proxy
3. **REST route** (`internal/server/routes.go`) - registers `GET /api/market/quote/{ticker}`
4. **REST handler** (`internal/server/handlers.go`) - calls `s.app.EODHDClient.GetRealTimeQuote(ctx, ticker)`
5. **Formatter** (`cmd/vire-mcp/formatters.go`) - `formatQuote()` renders markdown
6. **Tool registration** (`cmd/vire-mcp/tools.go`) - `s.AddTool(createGetQuoteTool(), handleGetQuote(p))`

## Changes by File

### 1. `internal/server/routes.go`
Add route:
```go
mux.HandleFunc("/api/market/quote/", s.handleMarketQuote)
```

### 2. `internal/server/handlers.go`
Add handler `handleMarketQuote` that:
- Extracts ticker from URL path (`/api/market/quote/{ticker}`)
- Validates ticker has exchange suffix (reuse `validateTicker`)
- Checks `s.app.EODHDClient != nil` (returns 503 if not configured)
- Calls `s.app.EODHDClient.GetRealTimeQuote(ctx, ticker)`
- Returns the `RealTimeQuote` struct as JSON

### 3. `cmd/vire-mcp/tools.go`
- Add `createGetQuoteTool()` - defines a tool named `get_quote` with one required `ticker` string param
- Register it in `registerTools()`: `s.AddTool(createGetQuoteTool(), handleGetQuote(p))`

### 4. `cmd/vire-mcp/handlers.go`
Add `handleGetQuote(p)` handler that:
- Requires `ticker` param
- Calls `p.get("/api/market/quote/{ticker}")`
- Unmarshals into `models.RealTimeQuote`
- Calls `formatQuote()` to render markdown
- Returns `textResult(markdown)`

### 5. `cmd/vire-mcp/formatters.go`
Add `formatQuote(q *models.RealTimeQuote) string` that renders:
```
# Quote: XAGUSD.FOREX

| Field     | Value        |
|-----------|--------------|
| Price     | $31.25       |
| Open      | $31.10       |
| High      | $31.50       |
| Low       | $30.90       |
| Volume    | 12345        |
| Timestamp | 2026-02-13 09:30 |
```

## Design Decisions

- **GET method** for the REST endpoint since it's a read-only data fetch (matches `handleMarketStocks` pattern)
- **No change/change% fields** - the EODHD real-time API only returns OHLCV + timestamp; change% would require computing from previous close, which isn't available in this call. Keep the tool focused on what the API provides.
- **Ticker with exchange suffix required** - consistent with existing tools like `get_stock_data`
- **No caching** - real-time quotes should always be fresh
- **Tool description** emphasizes speed ("FAST") and lists example tickers to help the LLM understand what formats are accepted

## Tool Definition

```go
func createGetQuoteTool() mcp.Tool {
    return mcp.NewTool("get_quote",
        mcp.WithDescription("FAST: Get a real-time price quote for any ticker. Supports stocks (BHP.AU, AAPL.US), forex (AUDUSD.FOREX, EURUSD.FOREX), and commodities (XAUUSD.FOREX for gold, XAGUSD.FOREX for silver)."),
        mcp.WithString("ticker", mcp.Required(), mcp.Description("Ticker with exchange suffix (e.g., 'BHP.AU', 'AAPL.US', 'AUDUSD.FOREX', 'XAUUSD.FOREX')")),
    )
}
```

## Test Plan

Tests will be added in task #4:
- `cmd/vire-mcp/formatters_test.go` - test `formatQuote()` output
- `cmd/vire-mcp/handlers_test.go` (new) - test `handleGetQuote` with mock server
- `internal/server/handlers_test.go` or integration test - test REST endpoint
