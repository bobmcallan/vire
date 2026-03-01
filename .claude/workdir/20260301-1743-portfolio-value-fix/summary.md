# Summary: Fix Portfolio Value & Dashboard Fields

**Status:** completed

## Changes
| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `TotalProceeds` to Holding, `AvailableCash`/`CapitalGain`/`CapitalGainPct` to Portfolio |
| `internal/services/portfolio/service.go` | Added `totalProceeds` to holdingCalcMetrics, redefined portfolio TotalCost from trades, computed AvailableCash, fixed TotalValue/weights/historical aggregates, fixed ReviewPortfolio |
| `internal/server/handlers.go` | Compute CapitalGain/CapitalGainPct after attaching CapitalPerformance |
| `internal/server/glossary.go` | Updated total_value/total_cost definitions, added available_cash/capital_gain/capital_gain_pct terms |
| `internal/server/glossary_test.go` | Added new terms to expected list |
| `internal/server/catalog.go` | Updated get_portfolio tool description with new field semantics |
| `internal/services/portfolio/service_test.go` | 323 lines added — new tests for TotalCost from trades, AvailableCash, TotalValue fix, FX conversion |
| `internal/services/portfolio/currency_test.go` | Updated for TotalProceeds FX conversion |
| `internal/services/portfolio/fx_stress_test.go` | Updated for new field assertions |
| `internal/services/portfolio/fx_test.go` | Updated for TotalProceeds FX conversion |
| `docs/features/20260301-portfolio-value-definitions.md` | New field definitions doc with formulas and examples |

## Tests
- Unit tests: 409 passing (portfolio package)
- Stress tests: 15 adversarial tests added by devils-advocate
- Integration tests: created in tests/api/portfolio_value_test.go
- Build: `go build ./cmd/vire-server` — PASS
- Vet: `go vet ./...` — PASS
- Pre-existing failures: TestHandlePortfolioSync_MissingNavexaKey_Returns400 (unrelated error message change)

## Architecture
- Architect reviewed: separation of concerns correct (totalCost computed in portfolio service from trades)
- CapitalGain computed in handler (correct — needs CapitalPerformance from cashflow service)
- No legacy compatibility shims

## Devils-Advocate
- 15 stress tests all pass
- Edge cases covered: negative availableCash, zero totalCost, FX conversion, closed positions
- 4 FX test assertions updated to match new formula

## Feedback Items
- Still need to resolve: fb_858ac27f, fb_39bb6c2b, fb_fda3bd07, fb_e0aeb97d, fb_948b4fee
- Portal feedback items needed for new/changed JSON fields
