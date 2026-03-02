# Summary: Fix Timeline PortfolioValue for No-Cash-Transactions Portfolios

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/services/portfolio/growth.go` | Added `hasCashTxs` flag; when no cash transactions, `NetCashBalance=0` and `PortfolioValue=EquityValue` instead of using trade settlement cash flows |
| `internal/services/portfolio/capital_timeline_test.go` | Updated `TestGetDailyGrowth_NoTransactions` to assert `NetCashBalance=0` and `PortfolioValue=EquityValue` |

## Root Cause

When a portfolio has no cash transactions, `runningNetCash` in the timeline calculation accumulated trade settlements (buys subtract, sells add) without any cash deposits to offset them. This made `NetCashBalance` deeply negative (e.g., -$27k after buying $27k of stock), causing `PortfolioValue = EquityValue + NetCashBalance` to show near-zero values on the chart.

The portfolio sync in `service.go` already handled this correctly (forces `availableCash=0` when `totalCash==0`). The growth pipeline was inconsistent.

## Fix

Added a `hasCashTxs := len(txs) > 0` flag. When false, the timeline sets `NetCashBalance=0` and `PortfolioValue=EquityValue`, matching the portfolio sync behavior. When true, existing calculation is unchanged.

## Tests

- 4/4 growth tests pass (including updated `TestGetDailyGrowth_NoTransactions`)
- All unit tests pass (except pre-existing surrealdb stress test)
- `go vet` clean, build clean

## Notes

- The identical Capital Return % = Simple Return % (90.47%) in the screenshot is mathematically correct when PortfolioValue = EquityValue (no cash). Not a bug.
- Closed positions ($0 value) showing despite "Show closed positions" unchecked is a portal rendering issue — the server correctly sets `status: "closed"` on those holdings.
