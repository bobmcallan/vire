# Summary: Auto-refresh portfolio on stale cache

**Date:** 2026-02-13
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/common/freshness.go` | `FreshnessPortfolio`: 1h → 30min |
| `internal/server/handlers.go` | Added auto-refresh with stale fallback in `handlePortfolioGet` (lines 45-50) |
| `internal/server/handlers_portfolio_test.go` | New file: 4 tests covering fresh/stale/failure/not-found paths |

## Tests
- `TestHandlePortfolioGet_FreshPortfolio_NoSync` — fresh data returns without sync
- `TestHandlePortfolioGet_StalePortfolio_TriggersSync` — stale data triggers auto-sync
- `TestHandlePortfolioGet_SyncFails_ReturnsStaleData` — sync failure falls back to stale data
- `TestHandlePortfolioGet_NotFound` — missing portfolio returns 404
- All tests pass

## Documentation Updated
- No README or skill file changes needed (transparent internal optimization)

## Review Findings
- Reviewer required synchronous get-then-refresh with stale fallback — adopted
- Reviewer confirmed no race conditions (SyncPortfolio mutex handles concurrent requests)
- Zero LastSynced edge case is safe (IsFresh returns false, triggers sync attempt)
- No handler-level logging needed (service logs internally)

## Notes
- The 30-minute TTL matches ASX's ~20-minute data delay reality
- US shares are live via EODHD so the 30-min TTL is conservative for those
- SyncPortfolio takes 10-30s when stale (N+2 Navexa API calls), but is a one-time cost per window
- Subsequent calls within the 30-min window return instantly from cache
