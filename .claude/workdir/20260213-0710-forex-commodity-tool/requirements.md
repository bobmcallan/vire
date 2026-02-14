# Requirements: Forex/Commodity MCP Tool

**Date:** 2026-02-13
**Requested:** New MCP tool for real-time forex and commodity pricing via EODHD, so the LLM gets accurate prices instead of relying on stale web search results.

## Problem
When asked about silver (XAGUSD), the LLM got $84.59 from web search (wrong), $69.48 from SLV ETF (wrong), while EODHD had the correct $79.84. The LLM needs direct access to EODHD real-time quotes for forex/commodity tickers.

## Scope
- **In scope:** New MCP tool `get_quote` (or similar) that fetches EODHD real-time quotes for any ticker (forex pairs like AUDUSD.FOREX, commodities like XAGUSD.FOREX, XAUUSD.FOREX, and regular stocks)
- **In scope:** REST endpoint on vire-server to support it
- **In scope:** Formatter for clean output
- **Out of scope:** Historical data, charts, signals for forex/commodities

## Approach
1. Add REST endpoint: `GET /api/market/quote/{ticker}` on vire-server
2. Add MCP tool: `get_quote` that proxies to the REST endpoint
3. Format output with price, change, range, volume, timestamp
4. Works for any EODHD ticker format (XAGUSD.FOREX, AUDUSD.FOREX, BHP.AU, etc.)

## Files Expected to Change
- `internal/server/routes.go` — new route
- `internal/server/handlers.go` — new handler
- `cmd/vire-mcp/main.go` — register new MCP tool
- `cmd/vire-mcp/formatters.go` — format quote output
- `README.md` — document new tool
