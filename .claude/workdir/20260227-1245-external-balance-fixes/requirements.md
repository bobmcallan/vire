# Requirements: External Balance Fixes for Capital Performance

**Feedback**: fb_7e9b3139, fb_2f9c18fe, fb_65070e71, fb_5d5e7e5e
**Date**: 2026-02-27

## Problem

Four related bugs around how external balances interact with capital performance:

1. **fb_65070e71** (ROOT CAUSE): `transfer_out` with category "accumulate" is treated as a capital withdrawal in `CalculatePerformance`. These are internal reallocations to external balance accounts, not money leaving the fund. Three SMSF transfers total $60,600 out, understating `net_capital_deployed` by ~$50K.

2. **fb_7e9b3139**: `current_portfolio_value` in capital performance includes external balances ($476K) but should be holdings-only ($426K) for investment return metrics. Combined with bug #1, this causes capital_gain to show +$48K instead of -$2K, and return to show +11% instead of -0.4%.

3. **fb_2f9c18fe**: Capital timeline growth data points exclude external balances (correct), but when a transfer_out to accumulate fires, the running cash balance drops, making the chart show a false crash. The internal transfer should be excluded from cash flow calculations in the timeline.

4. **fb_5d5e7e5e**: All external balance types (cash, accumulate, term_deposit, offset) are cash-equivalents. Add an `AssetCategory()` method returning "cash" for portfolio allocation logic.

## Root Cause Analysis

The `category` field on `CashTransaction` is stored but **never inspected** in any calculation. All `transfer_out` is treated identically to `withdrawal`. When money moves from holdings to an accumulate external balance, it's double-counted: the external balance appears in `ExternalBalanceTotal`, but the transfer is also subtracted from `net_capital_deployed`.

## Fixes

### Fix 1: Add `IsInternalTransfer()` helper
**File**: `internal/models/cashflow.go`

Add a method/function that identifies transactions representing internal transfers to/from external balance accounts:
```go
// ExternalBalanceCategories are the valid external balance types.
var ExternalBalanceCategories = map[string]bool{
    "cash": true, "accumulate": true, "term_deposit": true, "offset": true,
}

// IsInternalTransfer returns true if this transaction represents an internal
// move to/from an external balance account (not real capital flowing in/out).
func (tx CashTransaction) IsInternalTransfer() bool {
    if tx.Type != CashTxTransferOut && tx.Type != CashTxTransferIn {
        return false
    }
    return ExternalBalanceCategories[tx.Category]
}
```

### Fix 2: Capital performance — exclude internal transfers, use holdings-only value
**File**: `internal/services/cashflow/service.go`

In `CalculatePerformance()` (line ~256):
- Change `currentValue := portfolio.TotalValueHoldings + portfolio.ExternalBalanceTotal`
  to `currentValue := portfolio.TotalValueHoldings`
- In the deposit/withdrawal loop, skip transactions where `tx.IsInternalTransfer()` is true
- In the XIRR cashflow construction, also skip internal transfers

In `deriveFromTrades()` fallback (line ~349):
- Same change: use `TotalValueHoldings` only

### Fix 3: Capital timeline — exclude internal transfers from cash balance
**File**: `internal/services/portfolio/growth.go`

In `GetDailyGrowth()`, the cash balance loop (lines ~189-204):
- Skip transactions where `tx.IsInternalTransfer()` is true
- These are internal moves, not real cash flows affecting the portfolio's cash position

### Fix 4: External balance asset category
**File**: `internal/models/portfolio.go`

Add method on `ExternalBalance`:
```go
func (eb ExternalBalance) AssetCategory() string {
    return "cash"
}
```

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Add `IsInternalTransfer()`, `ExternalBalanceCategories` |
| `internal/models/portfolio.go` | Add `AssetCategory()` on ExternalBalance |
| `internal/services/cashflow/service.go` | Fix CalculatePerformance + deriveFromTrades to use TotalValueHoldings and skip internal transfers |
| `internal/services/portfolio/growth.go` | Skip internal transfers in cash balance calculation |

## Design Decisions

1. **Holdings-only for investment return**: Capital performance measures investment performance, not total wealth. External balances (cash equivalents) earn interest separately and aren't stock investments.
2. **Category-based detection**: Internal transfers identified by `tx.Category` matching external balance types. This is backward-compatible — existing transactions with "accumulate" category will be correctly reclassified.
3. **No model changes to CashTransaction**: The `Category` field already exists and is populated. We just need to inspect it.
4. **All external balance types are cash**: Per fb_5d5e7e5e, there is no scenario where an external balance type is an equity/investment. All four types are cash-equivalents.
