# Requirements: Fix Timeline PortfolioValue for No-Cash-Transactions Portfolios

## Problem

When a portfolio has **no cash transactions** (no entries in the cash flow ledger), the timeline/growth calculation produces incorrect `PortfolioValue` values. The chart shows Portfolio Value near $0 while Equity Value is $27k+.

### Root Cause

In `growth.go`, `runningNetCash` accumulates trade settlement cash flows (buys subtract, sells add). When there are no cash transactions to offset these, `NetCashBalance` becomes deeply negative (≈ -$27k). The formula `PortfolioValue = EquityValue + runningNetCash` then produces near-zero or negative portfolio values.

`service.go` (lines 424-427) already handles this correctly: when `totalCash == 0`, it forces `availableCash = 0` so `PortfolioValue = EquityValue`. The growth pipeline needs the same treatment.

### Screenshot Evidence

- Chart tooltip at Oct 26: Portfolio Value = $119.08, Equity Value = $27,082.17
- The "Portfolio Value" line on the chart drops near $0 while "Equity Value" is $27k
- This is because PortfolioValue = 27082 + (-26963) = $119

## Scope

**In scope:**
1. Fix `growth.go` timeline calculation for no-cash-transactions case
2. Update existing test `TestGetDailyGrowth_NoTransactions` to match correct behavior
3. Add test for mixed scenario: portfolio with some cash transactions verifies existing behavior

**Out of scope:**
- Portal rendering changes (closed positions filter, display adjustments)
- Capital performance calculations (already correct via `deriveFromTrades`)
- Portfolio sync in service.go (already correct)

## Files to Change

### 1. `internal/services/portfolio/growth.go`

**Change**: Add a `hasCashTxs` flag based on `len(opts.Transactions) > 0`. When false, timeline points should NOT use `runningNetCash` for PortfolioValue or NetCashBalance.

**Location**: Lines 211-295 (Phase 5 and Phase 6)

**Code template**:

```go
// Phase 5: Prepare cash flow cursor for single-pass merge
txs := opts.Transactions
sort.Slice(txs, func(i, j int) bool { return txs[i].Date.Before(txs[j].Date) })
txCursor := 0
hasCashTxs := len(txs) > 0  // <-- ADD THIS
var runningGrossCash, runningNetCash, runningNetDeployed float64

// ... existing iteration code unchanged ...

// At line 284-295, change the GrowthDataPoint construction:
netCash := runningNetCash
portfolioVal := totalValue + runningNetCash
if !hasCashTxs {
    // Without cash transactions, cash position is unknown.
    // PortfolioValue = EquityValue (consistent with service.go:424-427).
    netCash = 0
    portfolioVal = totalValue
}

points = append(points, models.GrowthDataPoint{
    Date:               date,
    EquityValue:        totalValue,
    NetEquityCost:      totalCost,
    NetEquityReturn:    gainLoss,
    NetEquityReturnPct: gainLossPct,
    HoldingCount:       holdingCount,
    GrossCashBalance:   runningGrossCash,     // 0 when no txs (unchanged)
    NetCashBalance:     netCash,              // 0 when no cash txs
    PortfolioValue:     portfolioVal,         // = EquityValue when no cash txs
    NetCapitalDeployed: runningNetDeployed,   // 0 when no txs (unchanged)
})
```

### 2. `internal/services/portfolio/capital_timeline_test.go`

**Change**: Update `TestGetDailyGrowth_NoTransactions` assertions at lines 178-189.

**Current (wrong)**:
```go
// No cash transactions: GrossCash = 0, NetCash = -5000 (buy trade)
for i, p := range points {
    if p.GrossCashBalance != 0 {
        t.Errorf(...)
    }
    if p.NetCashBalance != -5000 {
        t.Errorf(...)
    }
    if p.NetCapitalDeployed != 0 {
        t.Errorf(...)
    }
}
```

**New (correct)**:
```go
// No cash transactions: all cash fields are zero.
// Without cash transactions, cash position is unknown — PortfolioValue = EquityValue.
for i, p := range points {
    if p.GrossCashBalance != 0 {
        t.Errorf("points[%d].GrossCashBalance = %.2f, want 0 (no cash transactions)", i, p.GrossCashBalance)
    }
    if p.NetCashBalance != 0 {
        t.Errorf("points[%d].NetCashBalance = %.2f, want 0 (no cash transactions — cash position unknown)", i, p.NetCashBalance)
    }
    if p.NetCapitalDeployed != 0 {
        t.Errorf("points[%d].NetCapitalDeployed = %.2f, want 0", i, p.NetCapitalDeployed)
    }
    if p.PortfolioValue != p.EquityValue {
        t.Errorf("points[%d].PortfolioValue = %.2f, want %.2f (should equal EquityValue when no cash transactions)", i, p.PortfolioValue, p.EquityValue)
    }
}
```

### 3. No new files needed

The fix is contained in 2 files with minimal changes.

## Test Cases

| Test | What it verifies |
|------|-----------------|
| `TestGetDailyGrowth_NoTransactions` (updated) | NetCashBalance=0, PortfolioValue=EquityValue when no cash txs |
| `TestGetDailyGrowth_CashTransactions` (existing) | Cash fields correctly populated when cash txs exist |
| `TestGetDailyGrowth_ContributionAndWithdrawal` (existing) | NetCapitalDeployed tracks contributions correctly |

## Verification

After implementation:
1. `go test ./internal/services/portfolio/... -run TestGetDailyGrowth -v` — all growth tests pass
2. `go test ./internal/... -timeout 120s` — no regressions
3. `go vet ./...` — clean
4. `go build ./cmd/vire-server/` — builds successfully
