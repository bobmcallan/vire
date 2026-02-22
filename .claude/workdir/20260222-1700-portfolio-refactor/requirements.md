# Requirements: Portfolio Field Rename, Cleanup & Portfolio-Level Breakeven

**Date:** 2026-02-22
**Requested:** 11-point refactoring of portfolio Holding/Portfolio fields

## User Requirements (verbatim)

1. Portfolio totals needs to calculate `true_breakeven_price` for the complete portfolio
2. Remove `price_target_15pct`, `stop_loss_5pct`, `stop_loss_10pct`, `stop_loss_15pct` (derivable from `true_breakeven_price`)
3. Portfolio data from Navexa has currency — return it in portfolio total, convert any USD ticker (e.g. CBOE) to portfolio currency
4. `gain_loss` → `net_return`
5. `gain_loss_pct` → `net_return_pct` (note: conflicts with existing `*float64 NetReturnPct` — see approach)
6. Remove `net_pnl_if_sold_today` (same as `gain_loss` / `net_return`)
7. `realized_gain_loss` → `realized_net_return`
8. `unrealized_gain_loss` → `unrealized_net_return`
9. Remove `total_return_value` and `total_return_pct`
10. `total_return_pct_irr` → `net_return_pct_irr`
11. `total_return_pct_twrr` → `net_return_pct_twrr`

## Approach

### Field Rename Map (Holding struct)

| Old Go Field | Old JSON | New Go Field | New JSON | Action |
|---|---|---|---|---|
| `GainLoss` | `gain_loss` | `NetReturn` | `net_return` | Rename |
| `GainLossPct` | `gain_loss_pct` | `NetReturnPct` | `net_return_pct` | Rename (absorbs old `*float64 NetReturnPct`) |
| `RealizedGainLoss` | `realized_gain_loss` | `RealizedNetReturn` | `realized_net_return` | Rename |
| `UnrealizedGainLoss` | `unrealized_gain_loss` | `UnrealizedNetReturn` | `unrealized_net_return` | Rename |
| `TotalReturnPctIRR` | `total_return_pct_irr` | `NetReturnPctIRR` | `net_return_pct_irr` | Rename |
| `TotalReturnPctTWRR` | `total_return_pct_twrr` | `NetReturnPctTWRR` | `net_return_pct_twrr` | Rename |
| `TotalReturnValue` | `total_return_value` | — | — | Remove |
| `TotalReturnPct` | `total_return_pct` | — | — | Remove |
| `NetPnlIfSoldToday` | `net_pnl_if_sold_today` | — | — | Remove |
| `PriceTarget15Pct` | `price_target_15pct` | — | — | Remove |
| `StopLoss5Pct` | `stop_loss_5pct` | — | — | Remove |
| `StopLoss10Pct` | `stop_loss_10pct` | — | — | Remove |
| `StopLoss15Pct` | `stop_loss_15pct` | — | — | Remove |

### Key design decision: NetReturnPct type

Old `GainLossPct` was `float64`. Old `NetReturnPct` was `*float64` (nil for closed).
The new `NetReturnPct` should be `float64` (like the old `GainLossPct`), since it replaces `GainLossPct` which is always populated. The old `*float64 NetReturnPct` derived field is removed (it was `net_pnl / total_invested * 100`, similar purpose).

### Portfolio-level changes

Add to `Portfolio` struct:
- `TrueBreakevenPrice *float64` (`true_breakeven_price`) — portfolio-level breakeven
- Formula: `sum(TotalCost) - sum(RealizedNetReturn)` / `sum(Units * CurrentPrice != 0 holdings equivalent units)` — actually for a portfolio this doesn't have a single "price". Better approach: **portfolio true breakeven = total cost of open positions adjusted for realized P&L**. Since portfolio holds multiple tickers at different prices, a single breakeven price doesn't make sense. Instead, provide:
  - `TotalRealizedNetReturn` (`total_realized_net_return`) — sum of all realized P&L (FX-converted)
  - `TotalUnrealizedNetReturn` (`total_unrealized_net_return`) — sum of all unrealized P&L (FX-converted)
  - `TotalNetReturn` (`total_net_return`) — realized + unrealized (replaces `TotalGain`)
  - `TotalNetReturnPct` (`total_net_return_pct`) — replaces `TotalGainPct`

Rename on Portfolio struct:
- `TotalGain` → `TotalNetReturn`
- `TotalGainPct` → `TotalNetReturnPct`

### Trades removal from portfolio output

Remove `Trades` from the `Holding` struct serialization in portfolio list output. Trades should only appear in the individual stock endpoint (`get_portfolio_stock`). Approach: Add `omitempty` to the `Trades` JSON tag and set `Trades` to nil in holdings before building the portfolio response. Or better: the `Holding` struct already has `json:"trades,omitempty"` — just don't populate it in the portfolio-level output. Check where trades are set and ensure they're only populated for single-stock queries.

### Currency in portfolio totals

Portfolio already stores `Currency` from Navexa (line 372: `Currency: navexaPortfolio.Currency`). FX conversion already happens for USD holdings. This is already working — just verify it's returned correctly.

### NavexaHolding struct

The NavexaHolding struct in `navexa.go` has matching field names that map from Navexa API JSON. These fields are intermediate — they get mapped to the Holding struct. The NavexaHolding JSON tags must stay matching Navexa's API response. Only the Go struct field names that reference them in code need updating.

Actually — NavexaHolding JSON tags match what Navexa returns, so they should NOT change. Only the Go field names need renaming for consistency, and the mapping code in service.go needs updating.

### Strategy rules engine

`resolveField` in `rules.go` uses JSON-style field names like `gain_loss_pct`, `total_return_pct`, `total_return_pct_twrr`. These need updating to new names. For backward compatibility, keep old names as aliases.

## Files to Change

### Core:
- `internal/models/portfolio.go` — Holding struct renames, removals; Portfolio struct additions
- `internal/models/navexa.go` — NavexaHolding Go field renames (JSON tags unchanged)
- `internal/services/portfolio/service.go` — field assignment updates, portfolio totals computation, remove price target/stop loss computation
- `internal/services/strategy/rules.go` — resolveField case updates
- `internal/services/strategy/service.go` — validFields map update
- `internal/server/catalog.go` — MCP tool descriptions
- `internal/services/report/formatter.go` — display formatting

### Tests:
- `internal/services/portfolio/service_test.go` — all gain/loss field references
- `internal/services/portfolio/returns_refactor_test.go` — if exists
- `internal/services/strategy/rules_test.go` — field name updates
- `tests/api/portfolio_stock_test.go` — JSON field name updates
- `tests/api/gainloss_test.go` — field name updates
- `tests/data/gainloss_test.go` — field name updates
- `tests/fixtures/portfolio_smsf.json` — JSON field name updates

### Other:
- `internal/services/portfolio/growth.go` — if references renamed fields
- `internal/services/portfolio/snapshot.go` — if references renamed fields
- `internal/services/report/formatter_test.go` — if references renamed fields
- `internal/server/handlers_review_test.go` — if references renamed fields
- `docs/portfolio/portfolio-stock-calculation.md` — update field names
