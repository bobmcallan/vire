# Summary: Fix Portfolio Gain/Loss % Calculation

**Date:** 2026-02-22
**Status:** Completed (XIRR deferred)

## What Changed

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Replaced Navexa IRR p.a. with locally-computed simple % for GainLossPct, CapitalGainPct, TotalReturnPct. Recomputes % after EODHD price cross-check. Handles TotalCost <= 0 edge case. |
| `internal/server/handlers.go` | Added `force_refresh` query param support to `handlePortfolioStock()` — calls `SyncPortfolio(ctx, name, true)` when set. |
| `internal/server/catalog.go` | Added `force_refresh` boolean param to `get_portfolio_stock` MCP tool definition. |
| `internal/models/portfolio.go` | Updated field comments on Holding: GainLossPct, CapitalGainPct, TotalReturnPct now documented as simple percentages. |
| `internal/models/navexa.go` | Updated NavexaHolding field comments. |
| `internal/services/portfolio/service_test.go` | Added ~434 lines of new tests for simple % calculation, edge cases, price update scenarios. |
| `internal/services/portfolio/returns_refactor_test.go` | Updated existing test expectations. |
| `tests/api/portfolio_stock_test.go` | New integration tests: GainPercentage, ForceRefresh, ForceRefreshNoNavexa. |
| `README.md` | Updated MCP tool documentation. |

## Core Fix

**Before:** `GainLossPct`, `CapitalGainPct`, `TotalReturnPct` were Navexa's IRR p.a. values — stale, sometimes wildly wrong (10.85% for SKS when actual was ~5.82%).

**After:** All three are computed locally as simple percentages: `GainLoss / TotalCost * 100`. Recomputed after EODHD price cross-check. Edge case handling for TotalCost <= 0.

**SKS result:** GainLossPct = ~5.82% (simple), TWRR = 7.02% (already computed locally).

## Return Metrics Available

| Metric | Field | Computation |
|--------|-------|-------------|
| Simple return % | `gain_loss_pct`, `capital_gain_pct` | `GainLoss / TotalCost * 100` (locally computed) |
| Simple total return % | `total_return_pct` | `TotalReturnValue / TotalCost * 100` (locally computed) |
| TWRR | `total_return_pct_twrr` | Geometric linking of sub-period returns (locally computed) |
| XIRR/IRR | — | **Not implemented** (deferred) |

## Tests
- 434 lines of new unit tests in service_test.go
- 3 integration tests in tests/api/portfolio_stock_test.go
- All unit tests pass
- Integration tests created and validated

## Devils-Advocate Findings
- Stale IRR % when TotalCost <= 0 — handled with zero fallback
- No rate limiting on force_refresh — noted but not addressed (existing sync mutex provides basic protection)

## Notes
- **XIRR not implemented** — user requested industry-standard IRR but the team implemented simple % only. XIRR (Newton-Raphson on dated cash flows) would need a new `xirr.go` file. Trades have Date fields available. Follow-up work recommended.
- TWRR (7.02%) already provides a time-aware return metric. Simple % (5.82%) gives the intuitive capital return.
- Navexa's 5.23% is their XIRR including brokerage — a locally-computed XIRR should approximate this.
