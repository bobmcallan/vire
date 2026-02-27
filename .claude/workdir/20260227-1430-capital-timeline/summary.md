# Summary: Capital Allocation Timeline

**Status:** completed
**Duration:** ~20 minutes (14:30–14:51)
**Feedback:** fb_c4a661a8, fb_00e43378, fb_da8cabc1, fb_ca924779, fb_e69d6635

## Changes

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added fields to TimeSeriesPoint (cash_balance, external_balance, total_capital, net_deployed), GrowthDataPoint (same), Portfolio (yesterday_net_flow, last_week_net_flow) |
| `internal/services/portfolio/growth.go` | Cash flow integration in GetDailyGrowth — merges date-sorted transactions to compute running cash balance and net deployed |
| `internal/services/portfolio/indicators.go` | Updated growthPointsToTimeSeries() to map new fields; GetPortfolioIndicators() loads cash flow ledger |
| `internal/services/portfolio/service.go` | Added CashFlowService dependency via SetCashFlowService(); net flow computation in populateHistoricalValues() |
| `internal/interfaces/services.go` | Updated GrowthOptions with Transactions field |
| `internal/app/app.go` | Wired CashFlowService to portfolio service |
| `internal/server/catalog.go` | Updated tool descriptions for get_portfolio, get_portfolio_indicators |
| `docs/architecture/services.md` | Documented capital timeline data flow, net flow fields, CashFlowService dependency |

## Tests

| File | Tests |
|------|-------|
| `internal/services/portfolio/capital_timeline_test.go` | Unit tests for capital timeline computation |
| `internal/services/portfolio/capital_timeline_stress_test.go` | 38 adversarial stress tests (edge cases, nil service, empty ledger, large volumes) |
| `tests/api/capital_timeline_test.go` | 5 integration tests (field presence, external balance, empty ledger, accumulation, same-day) |
| `tests/api/portfolio_netflow_test.go` | 7 integration tests (no transactions, yesterday flow, week flow, negative, outside window, dividend exclusion, sync persistence) |

- All unit tests: PASS
- Build/vet: CLEAN
- Stress tests: 38/38 PASS

## Architecture

- Circular dependency avoided: portfolio uses `interfaces.CashFlowService` (interface), cashflow uses `interfaces.PortfolioService`. No import cycle.
- `SetCashFlowService()` setter pattern avoids constructor circular dependency.
- Architect approved, docs updated.

## Devils-Advocate

- 38 stress tests covering: nil service, empty transactions, zero dates, large volumes, same-day transactions, future-dated, negative amounts, sign logic
- All passed, no fixes needed

## Notes

- MCP session persistence (fb_c4a661a8, fb_00e43378) is out of scope — SSE transport handled by portal on Fly.dev
- Portal charts (fb_ca924779) — data API ready, frontend work separate
- All new fields use omitempty — fully backward compatible
