# Summary: Capital & Cash Calculation Fixes

**Status:** completed

## Feedback Items
| ID | Severity | Issue | Resolution |
|----|----------|-------|------------|
| fb_d895f8f9 | HIGH | external_balance_total uses NonTransactionalBalance | Fixed — uses TotalCashBalance(), renamed to total_cash |
| fb_7ffa974f | HIGH | total_deposited counts wrong categories | Already fixed — marked resolved |
| fb_60bddec8 | HIGH | time_series ExternalBalance static/triple-counted | Fixed — removed from TotalCapital, set to 0 |
| fb_7d8dafdb | MEDIUM | net_deployed flat for negative contributions | Fixed — NetDeployedImpact returns amount for all contributions |
| fb_20ac6ee8 | MEDIUM | last_week_net_flow sign inverted | Fixed — netflow tests updated to current API format |

## Changes
| File | Change |
|------|--------|
| internal/models/portfolio.go:59 | ExternalBalanceTotal → TotalCash (field + JSON tag) |
| internal/models/cashflow.go:78-90 | NetDeployedImpact: negative contributions now return tx.Amount |
| internal/services/portfolio/service.go:402-416 | NonTransactionalBalance() → TotalCashBalance(), variable rename |
| internal/services/portfolio/growth.go:238-239 | TotalCapital = totalValue + runningCashBalance (no ExternalBalance) |
| internal/services/portfolio/indicators.go:15-41 | Removed externalBalanceTotal param from growthPointsToTimeSeries + growthToBars |
| internal/services/portfolio/indicators.go:112-113 | Updated calls to remove ExternalBalanceTotal argument |
| tests/api/portfolio_netflow_test.go | Updated all requests from old "type" format to "account"+"category" |
| 8 test files in internal/ | Renamed ExternalBalanceTotal → TotalCash across all test structs |

## Tests
- Unit tests: ALL PASS (models, cashflow, portfolio packages)
- 35+ new stress tests by devils-advocate
- Integration tests created for total_cash, time_series invariants, net_deployed stepping
- Build: PASS, go vet: PASS

## Architecture
- docs/architecture/services.md updated by architect — TotalCash, NetDeployedImpact documented

## Devils-Advocate
- Created cashflow_capital_stress_test.go (~20 tests) and capital_cash_fixes_stress_test.go (~15 tests)
- Fixed 8 existing test files with stale signatures
- Verified NaN/Inf/concurrent safety, TotalCapital invariant across 365 points

## Notes
- ExternalBalance field kept on GrowthDataPoint/TimeSeriesPoint (set to 0) for API compatibility
- Portal will need updating to use `total_cash` instead of `external_balance_total`
- Trade settlement auto-apply to transactional accounts is a future enhancement
