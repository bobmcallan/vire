# Summary: External Balance Fixes for Capital Performance

**Status:** completed
**Feedback:** fb_7e9b3139, fb_2f9c18fe, fb_65070e71, fb_5d5e7e5e

## Changes

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | `IsInternalTransfer()` + `ExternalBalanceCategories` (pre-existing) |
| `internal/models/portfolio.go` | Added `AssetCategory()` on `ExternalBalance` returning `"cash"` |
| `internal/services/cashflow/service.go` | `CalculatePerformance`: uses `TotalValueHoldings` only, skips internal transfers in deposit/withdrawal loop and XIRR |
| `internal/services/cashflow/service.go` | `deriveFromTrades`: uses `TotalValueHoldings` only |
| `internal/services/portfolio/growth.go` | `GetDailyGrowth`: skips internal transfers from cash balance cursor |
| `internal/services/portfolio/service.go` | `populateNetFlows`: skips internal transfers (bug found by devils-advocate) |

## Tests

- **Unit tests**: All passing in models, cashflow, portfolio packages
- **Stress tests**: 25+ tests covering edge cases (empty category, only-internal-transfers, XIRR with no flows, asymmetric transfers, SMSF scenario)
- **Integration tests**: 7 tests in `tests/api/external_balance_fixes_test.go`
- **AssetCategory test**: Added to `external_balances_test.go`
- Fix rounds: 1 (populateNetFlows bug found and fixed)

## Architecture

- `docs/architecture/services.md` already updated (from prior commit) to reflect holdings-only capital performance
- `internal/server/catalog.go` tool description accurate

## Devils-Advocate

- **BUG FOUND**: `populateNetFlows` in `service.go` was missing `IsInternalTransfer()` check — internal transfers were affecting `YesterdayNetFlow` and `LastWeekNetFlow`. Fixed and tested.
- Edge cases verified: empty category, unknown category, only-internal-transfers (no division by zero), XIRR returns 0 when all flows filtered, first transaction as internal transfer

## Notes

- Fix 1 (`IsInternalTransfer` + `ExternalBalanceCategories`) already existed from prior commit — tests confirmed correctness
- Key insight: transfer_in/transfer_out maintain total cash, just allocate to different areas (portfolio cash vs external balance accounts)
- Capital performance now measures investment returns only (holdings), not total wealth
