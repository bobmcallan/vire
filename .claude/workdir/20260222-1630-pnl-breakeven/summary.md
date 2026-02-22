# Summary: True Net P&L, Break-even Price & Price Targets

**Date:** 2026-02-22
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added 7 derived `*float64` fields to `Holding` struct: `NetPnlIfSoldToday`, `NetReturnPct`, `TrueBreakevenPrice`, `PriceTarget15Pct`, `StopLoss5Pct`, `StopLoss10Pct`, `StopLoss15Pct` |
| `internal/services/portfolio/service.go` | Compute derived P&L/breakeven fields after holdingMetrics population (lines 269-287). Only for open positions (units > 0). |
| `internal/server/catalog.go` | Updated `get_portfolio` and `get_portfolio_stock` MCP tool descriptions to mention true breakeven, net P&L, price targets, stop losses |
| `internal/services/portfolio/service_test.go` | 7 new unit tests: simple hold, partial sell loss/profit, closed position nil, net P&L, SKS-like scenario, price targets/stop losses |
| `tests/api/portfolio_stock_test.go` | 3 new integration tests: open position fields, closed position null fields, all holdings validation |
| `docs/portfolio/portfolio-stock-calculation.md` | Updated status from Proposed to Implemented |

## Formulas

- `net_pnl_if_sold_today` = `realized_gain_loss + unrealized_gain_loss`
- `net_return_pct` = `net_pnl_if_sold_today / total_invested * 100`
- `true_breakeven_price` = `(total_cost - realized_gain_loss) / units`
- `price_target_15pct` = `true_breakeven_price * 1.15`
- `stop_loss_5pct` / `10pct` / `15pct` = `true_breakeven_price * 0.95 / 0.90 / 0.85`

All fields are `*float64` (nullable) — `null` in JSON for closed positions (units = 0).

## Tests
- Unit tests: 113 pass (7 new breakeven tests)
- Integration tests: 3 new tests (open position, closed position, all holdings) — all pass
- 0 feedback loop rounds needed
- `go vet ./...` clean
- 2 pre-existing failures (not related): `TestStress_WriteRaw_AtomicWrite`, `TestPortfolioStock_GainPercentage/capital_gain_pct`

## Documentation Updated
- `docs/portfolio/portfolio-stock-calculation.md` — status updated to Implemented
- `internal/server/catalog.go` — MCP tool descriptions updated

## Devils-Advocate Findings
- Division by zero when units=0 — handled by `if holdings[i].Units > 0` guard
- Zero totalInvested with positive units — `NetReturnPct` stays nil, other fields still computed
- JSON serialization of `*float64` nil → `null` confirmed correct
- No NaN/Inf possible given the guards

## Notes
- All inputs (`TotalCost`, `RealizedGainLoss`, `UnrealizedGainLoss`, `TotalInvested`, `Units`) were already available on the Holding struct from the prior return % fix
- No handler changes needed — handlers return full Holding structs
- No new external data required — all fields are server-side derived
- Prior losses raise breakeven (must recover loss), prior profits lower it (profit offsets cost)
