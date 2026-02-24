# Summary: Split total_value (fb_6cac832d) & Portfolio-level indicators (fb_79536bca)

**Date:** 2026-02-24
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Renamed `TotalValue` → `TotalValueHoldings` on Portfolio; added new `TotalValue` (equity + external); added `PortfolioIndicators` struct; added `PortfolioIndicators` field to `PortfolioReview` |
| `internal/services/portfolio/indicators.go` | New file: `GetPortfolioIndicators`, `growthToBars`, `detectEMACrossover` |
| `internal/services/portfolio/service.go` | Updated `SyncPortfolio` to set both `TotalValueHoldings` and `TotalValue`; `ReviewPortfolio` computes and attaches portfolio indicators |
| `internal/services/portfolio/external_balances.go` | Updated to recompute `TotalValue` after external balance changes |
| `internal/services/cashflow/service.go` | Simplified to use `portfolio.TotalValue` directly (no manual addition) |
| `internal/interfaces/services.go` | Added `GetPortfolioIndicators` to `PortfolioService` interface |
| `internal/server/handlers.go` | Added `handlePortfolioIndicators` handler; updated `slimPortfolioReview` with `TotalValueHoldings` and `PortfolioIndicators` |
| `internal/server/routes.go` | Added `indicators` route |
| `internal/server/catalog.go` | Added `get_portfolio_indicators` MCP tool (tool #50) |
| `internal/server/catalog_test.go` | Updated tool count 49 → 50 |
| `internal/common/version.go` | Bumped SchemaVersion 8 → 9 |
| `internal/server/handlers_portfolio_test.go` | Added `GetPortfolioIndicators` to mock |
| `internal/services/cashflow/service_test.go` | Updated mock for `TotalValueHoldings` |
| `internal/services/report/devils_advocate_test.go` | Added `GetPortfolioIndicators` to mock |
| `internal/services/portfolio/service_test.go` | Updated tests for TotalValue split |
| `internal/services/portfolio/fx_stress_test.go` | Updated for TotalValueHoldings |
| `README.md` | Added `get_portfolio_indicators` tool, documented total_value split |
| `.claude/skills/develop/SKILL.md` | Added Portfolio Indicators reference section, updated Portfolio model docs |

## Tests

- 68 unit tests (`internal/services/portfolio/`): ALL PASS
  - `indicators_test.go`: growthToBars, detectEMACrossover, RSI boundaries, trend classification, EMA/SMA thresholds, TotalValue invariant
  - `indicators_stress_test.go`: NaN/Inf, large values, concurrent access, flat data, negative values
- 3 API integration tests (`tests/api/portfolio_indicators_test.go`): PASS (method validation, non-existent portfolio)
- Catalog regression: PASS (50 tools)
- Full `go test ./internal/...`: PASS
- `go vet ./...`: clean
- Test feedback rounds: 2 (empty trend/rsi_signal defaults for 0-data-points path)

## Documentation Updated

- `README.md` — added `get_portfolio_indicators` tool, documented total_value split
- `.claude/skills/develop/SKILL.md` — added Portfolio Indicators section, updated Portfolio model reference

## Devils-Advocate Findings

- Race condition in stress test fixed (unused import cleanup)
- RSI threshold note: boundary values (0, 30, 50, 70, 100) all handled correctly
- NaN/Inf in growth data: bars conversion handles gracefully
- Concurrent access: no races detected
- No actionable issues in production code

## Feedback Resolved

- `fb_6cac832d` (medium) — total_value split implemented
- `fb_79536bca` (medium) — portfolio-level indicators implemented

## Notes

- Portfolio indicators are computed on-demand from `GetDailyGrowth` + external balance total — no persistent daily snapshots needed
- Reuses existing `signals.EMA()`, `signals.RSI()`, `signals.SMA()` functions — no new indicator code
- External balance total is added as a constant to each daily growth point (external balances don't have daily history)
- EMA 200 requires 200+ calendar days of trade history — new portfolios will have zeros until sufficient data accumulates
- SchemaVersion bumped to 9 — forces re-sync for all cached portfolios to pick up the field rename
- 20 files changed, +195/-79 lines
