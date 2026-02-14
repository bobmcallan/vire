# Summary: Fresh Intraday Stock Pricing — Staleness Detection

**Date:** 2026-02-13
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/common/freshness.go` | Added `FreshnessRealTimeQuote = 15 * time.Minute` constant |
| `internal/server/handlers.go` | `handleMarketQuote()` returns JSON envelope with `quote`, `data_age_seconds`, `is_stale` |
| `cmd/vire-mcp/handlers.go` | `handleGetQuote()` unmarshals quote from envelope |
| `cmd/vire-mcp/formatters.go` | `formatQuote()` computes staleness via `common.IsFresh()`, shows STALE DATA warning + Data Age row, added `formatDuration()` helper |
| `cmd/vire-mcp/formatters_test.go` | 5 new tests: stale data, fresh data, days-old, zero timestamp, duration formatting |
| `cmd/vire-mcp/handlers_test.go` | Updated 3 tests + added 2 for stale/fresh quote end-to-end via envelope |
| `internal/models/market.go` | No changes (model stays pure DTO) |
| `internal/clients/eodhd/client.go` | No changes (client stays pure API wrapper) |

## Tests
- 5 new formatter tests for staleness scenarios
- 2 new handler tests for stale/fresh end-to-end
- 7-case table-driven test for `formatDuration`
- All 17 packages pass `go test ./...`
- Docker rebuild successful, containers healthy

## Documentation Updated
- No README or skill file changes needed (staleness is self-documenting in output)

## Review Findings
- Reviewer required staleness computation at presentation layer (not model or client) — adopted
- Reviewer required use of existing `IsFresh()` pattern — adopted
- 15-minute threshold chosen over initial 5-minute proposal
- `formatDuration` doesn't handle days (shows "48h" not "2d") — accepted as clearer
- `data_age_seconds` truncates fractionally — accepted as correct behavior

## Notes
- The original user problem was EODHD `/real-time/` returning delayed data for XAGUSD ($79.84 vs actual $74.93). This fix doesn't change what EODHD returns, but ensures stale data is visibly flagged so users know to verify with a live source.
- Follow-up: propagate staleness to `GetStockData` / `PriceData` (deferred)
