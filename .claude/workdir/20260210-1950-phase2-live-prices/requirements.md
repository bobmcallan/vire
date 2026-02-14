# Requirements: Phase 2 — Live Prices

**Date:** 2026-02-10
**Requested:** Implement Phase 2 of docs/performance-plan.md — add EODHD real-time API for current prices, integrate into GetStockData, portfolio review, and plan event checks.

## Scope
- Add `GetRealTimeQuote()` method to EODHD client using `/real-time/{ticker}` endpoint
- Add `RealTimeQuote` model
- Update `EODHDClient` interface
- Modify `GetStockData()` to prefer real-time quote for current price
- Modify `ReviewPortfolio()` to fetch real-time quotes for holdings
- Modify `check_plan_status` to use real-time price for event trigger evaluation
- Keep EOD endpoint for historical bars — real-time supplements, doesn't replace

## Out of Scope
- Phase 3 (concurrency) — real-time fetches will be sequential for now
- Caching of real-time quotes (they should always be fresh)
- WebSocket streaming (EODHD real-time is REST, not streaming)

## Approach
Per docs/performance-plan.md Phase 2:
- EODHD live endpoint: `GET /real-time/{ticker}` returns OHLCV + timestamp
- `GetStockData`: fetch real-time quote, use as `Price.Current`, EOD bars for history
- `ReviewPortfolio`: fetch real-time quotes for all holdings before signal analysis
- `check_plan_status`: use real-time price for ticker condition evaluation
- Graceful fallback: if real-time fails, fall back to EOD[0].Close (don't break existing behaviour)

## Files Expected to Change
- `internal/models/market.go` — add `RealTimeQuote` struct
- `internal/clients/eodhd/client.go` — add `GetRealTimeQuote()` method
- `internal/interfaces/clients.go` — add to `EODHDClient` interface
- `internal/services/market/service.go` — modify `GetStockData()` to use real-time
- `internal/services/portfolio/service.go` — real-time quotes in `ReviewPortfolio()`
- `internal/services/plan/service.go` — real-time price in plan event checks
- `cmd/vire-mcp/mocks_test.go` — add mock for `GetRealTimeQuote()`
