# Requirements: Auto-refresh portfolio on stale cache

**Date:** 2026-02-13
**Requested:** When an LLM requests portfolio data via `get_portfolio`, it should get fresh data automatically — not stale cached data from hours/days ago. Currently the LLM sees stale data and asks "want me to refresh?" which is a bad UX.

## Problem

- `GET /api/portfolios/{name}` reads directly from cache with NO freshness check
- `get_portfolio` MCP tool calls this endpoint and returns whatever is cached
- At 10:25 AEST (market open), portfolio still shows previous day's sync (21:42)
- The LLM then wastes a turn asking "should I refresh?" instead of having fresh data

## Key constraints

- EODHD/Navexa ASX data is ~20 minutes delayed
- US shares are live via EODHD
- `FreshnessPortfolio` is currently 1 hour — reasonable for auto-sync
- `SyncPortfolio` already has force/non-force logic and mutex serialization
- `review_portfolio` already fetches real-time quotes (correct behavior)

## Scope

### In scope
- Make `GET /api/portfolios/{name}` handler auto-sync when cached portfolio is stale (LastSynced > FreshnessPortfolio)
- Reduce `FreshnessPortfolio` from 1 hour to 30 minutes to better match ASX 20-min delay reality
- Add tests for auto-refresh behavior

### Out of scope
- Changing `review_portfolio` — it already fetches real-time quotes
- Changing `sync_portfolio` — it already has the right freshness logic
- WebSocket/streaming updates
- Changing the Navexa or EODHD delay characteristics

## Approach

1. **Server handler change:** In `handlePortfolioGet`, check `common.IsFresh(portfolio.LastSynced, common.FreshnessPortfolio)`. If stale and app has portfolio service, call `SyncPortfolio(ctx, name, false)` before returning.
2. **Freshness constant:** Reduce `FreshnessPortfolio` from 1h to 30min.
3. **Tests:** Verify auto-sync triggers on stale data, skips on fresh data.

## Files Expected to Change
- `internal/server/handlers.go` — auto-sync in GET handler
- `internal/common/freshness.go` — reduce FreshnessPortfolio to 30 minutes
- `internal/server/handlers_test.go` or new test file — test auto-refresh
