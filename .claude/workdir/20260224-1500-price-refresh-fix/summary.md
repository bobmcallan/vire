# Summary: Fix price data pipeline (fb_7a4c19e9, fb_b4134cbf, fb_52547389, fb_7d22e342)

**Date:** 2026-02-24
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/server/catalog.go` | Added `force_refresh` boolean param to `get_portfolio` MCP tool |
| `internal/server/handlers.go` | `handlePortfolioGet()` reads `force_refresh` query param, calls `SyncPortfolio` when true |
| `internal/services/portfolio/service.go` | `eodClosePrice()` now validates AdjClose against Close — falls back to Close when divergence >50% |
| `internal/services/portfolio/service_test.go` | 15 unit tests for eodClosePrice divergence scenarios |
| `internal/services/portfolio/eod_price_stress_test.go` | Stress tests for eodClosePrice edge cases |
| `internal/services/signal/service.go` | Added EODHD client to signal service; `overlayLiveQuote()` overlays real-time price on cached EOD bars before computing indicators |
| `internal/services/signal/service_test.go` | 23 unit tests for live quote overlay |
| `internal/services/signal/service_stress_test.go` | Stress tests for NaN, Inf, zero, race conditions |
| `internal/app/app.go` | Passes `eodhdClient` to `signal.NewService()` constructor |
| `.claude/skills/develop/SKILL.md` | Updated AdjClose preference docs with divergence check |
| `README.md` | Version reference update |
| `tests/api/portfolio_force_refresh_test.go` | API integration tests for force_refresh param |
| `internal/storage/surrealdb/main_test.go` | Fix for orphaned SurrealDB test containers (pre-existing) |

## Tests
- 15 eodClosePrice unit tests: PASS
- 23 signal overlay unit tests: PASS (includes NaN/Inf/zero guards)
- 4 API force_refresh integration tests: PASS
- Full `go test ./internal/...`: PASS
- `go vet ./...`: clean
- `golangci-lint run`: clean
- Test feedback rounds: 1 (NaN/+Inf guard fix from devils-advocate)

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — AdjClose preference section updated with divergence check
- `README.md` — version reference

## Devils-Advocate Findings
- **NaN/+Inf leak in overlayLiveQuote**: The initial guard `quote.Close <= 0` didn't catch NaN or +Inf. Fixed by adding `math.IsNaN(quote.Close) || math.IsInf(quote.Close, 0)` to the guard.
- All other edge cases (zero volume synthetic bars, race conditions, hostile inputs) passed.

## Notes
- Auto-refresh during market hours (watcher-based) is out of scope — separate feature
- The 50% divergence threshold in eodClosePrice is conservative; legitimate corporate actions (small adjustments) stay under this threshold while data errors (consolidation bugs like ACDC $5.11 vs $146) are caught
- Signal service now fetches real-time quotes per ticker during compute_indicators, adding ~1 API call per ticker — acceptable for portfolio-sized batches
