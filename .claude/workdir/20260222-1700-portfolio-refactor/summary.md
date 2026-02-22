# Summary: Portfolio Field Rename, Cleanup & Portfolio-Level Breakeven

**Date:** 2026-02-22
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | 6 field renames, 7 field removals on Holding; Portfolio gets `TotalNetReturn`, `TotalNetReturnPct`, `TotalRealizedNetReturn`, `TotalUnrealizedNetReturn` |
| `internal/models/navexa.go` | NavexaHolding Go field names updated (JSON tags unchanged for Navexa API compat) |
| `internal/services/portfolio/service.go` | Field mapping updates, removed price target/stop loss computation, added portfolio-level realized/unrealized totals with FX conversion |
| `internal/services/strategy/rules.go` | `resolveField` updated with new names + backward compat aliases |
| `internal/services/strategy/service.go` | validFields map updated |
| `internal/server/handlers.go` | Trades stripped from portfolio GET response (kept in stock GET) |
| `internal/server/catalog.go` | MCP tool descriptions updated |
| `internal/services/report/formatter.go` | Updated field references |
| `internal/services/portfolio/service_test.go` | All field name references updated |
| `internal/services/strategy/rules_test.go` | Field name updates |
| `tests/api/portfolio_stock_test.go` | JSON field name updates |
| `tests/api/gainloss_test.go` | Field name updates |
| `tests/data/gainloss_test.go` | Field name updates |
| `tests/fixtures/portfolio_smsf.json` | JSON key renames |
| `docs/portfolio/portfolio-stock-calculation.md` | Field names updated |

## Field Changes

### Renames
| Old | New |
|-----|-----|
| `gain_loss` | `net_return` |
| `gain_loss_pct` | `net_return_pct` |
| `realized_gain_loss` | `realized_net_return` |
| `unrealized_gain_loss` | `unrealized_net_return` |
| `total_return_pct_irr` | `net_return_pct_irr` |
| `total_return_pct_twrr` | `net_return_pct_twrr` |
| `total_gain` | `total_net_return` |
| `total_gain_pct` | `total_net_return_pct` |

### Removed
- `total_return_value`, `total_return_pct` (redundant)
- `net_pnl_if_sold_today` (same as `net_return`)
- `price_target_15pct`, `stop_loss_5pct`, `stop_loss_10pct`, `stop_loss_15pct` (derivable from `true_breakeven_price`)

### Added (Portfolio level)
- `total_realized_net_return` — sum of all realized P&L (FX-converted)
- `total_unrealized_net_return` — sum of all unrealized P&L (FX-converted)

### Other
- Trades removed from portfolio GET output (kept in individual stock GET)
- Strategy rules accept both old and new field names for backward compatibility

## Tests
- All unit tests pass (portfolio, strategy, server)
- `go vet ./...` clean
- `go build ./cmd/vire-server/` compiles
- Devils-advocate found 3 compile errors from go vet — fixed by implementer
- 0 feedback loop rounds in test execution

## Devils-Advocate Findings
- Found remaining compile errors from incomplete renames — fixed
- Verified NavexaHolding JSON tags unchanged
- Verified strategy rules backward compatibility

## Notes
- Implementer agent ran out of context but completed all tasks before failing
- MCP tool hits remote server which needs redeployment to show new field names
- Local server verified correct via JSON marshaling tests
