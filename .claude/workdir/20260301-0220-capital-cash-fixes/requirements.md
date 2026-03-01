# Requirements: Capital & Cash Calculation Fixes

## Feedback Items
- fb_d895f8f9 (HIGH): external_balance_total uses NonTransactionalBalance — should use TotalCashBalance
- fb_7ffa974f (HIGH): total_deposited counts wrong categories — **ALREADY FIXED**, mark resolved
- fb_60bddec8 (HIGH): time_series ExternalBalance is static, causing wrong values and double-counting
- fb_7d8dafdb (MEDIUM): net_deployed flat — NetDeployedImpact doesn't count negative contributions
- fb_20ac6ee8 (MEDIUM): last_week_net_flow sign inverted — likely old test format issue; existing tests broken

## Scope

**In scope:**
1. Replace `NonTransactionalBalance()` with `TotalCashBalance()` in portfolio service
2. Rename `ExternalBalanceTotal` field to `TotalCash` across the codebase
3. Fix growth.go and indicators.go to eliminate ExternalBalance double/triple-counting
4. Fix `NetDeployedImpact()` to count negative contributions as capital withdrawn
5. Update broken portfolio_netflow_test.go tests to use current API format
6. Unit tests for all changes

**Out of scope:**
- Portal changes (portal will be updated separately)
- Trade settlement auto-apply to transactional accounts
- Historical data repair

## Analysis

### Problem 1: ExternalBalanceTotal uses wrong calculation (fb_d895f8f9)

**Current code** — `internal/services/portfolio/service.go:406`:
```go
externalBalanceTotal = ledger.NonTransactionalBalance()
```
This only sums non-transactional accounts (Stake Accumulate, etc.), excluding Trading.

**Fix**: Replace with `TotalCashBalance()` which sums ALL accounts.

**Field rename**: `ExternalBalanceTotal` → `TotalCash` (JSON: `external_balance_total` → `total_cash`).
The "external balance" name is a legacy concept from when cash was tracked outside the portfolio.
Now that we have a proper ledger, the field represents total cash across all accounts.

### Problem 2: Triple-counting ExternalBalance in time_series (fb_60bddec8)

The ExternalBalance value is applied in THREE places:

1. **growth.go:238-239** — static `p.ExternalBalanceTotal` added to `TotalCapital`:
   ```go
   ExternalBalance: p.ExternalBalanceTotal,  // STATIC for all dates
   TotalCapital:    totalValue + runningCashBalance + p.ExternalBalanceTotal,
   ```

2. **indicators.go:18** — `externalBalanceTotal` added to `Value`:
   ```go
   value := p.TotalValue + externalBalanceTotal
   ```

3. **indicators.go:27** — static `externalBalanceTotal` set on every point:
   ```go
   ExternalBalance: externalBalanceTotal,
   ```

Meanwhile, `runningCashBalance` in growth.go already includes ALL cash transactions (both transactional and non-transactional accounts) via `opts.Transactions`. So non-transactional account amounts are counted in BOTH `runningCashBalance` AND `ExternalBalanceTotal`.

**Fix**:
- In `growth.go`: Remove `ExternalBalance` from `TotalCapital`. Set `ExternalBalance = 0`.
  ```go
  TotalCapital: totalValue + runningCashBalance,
  ```
- In `indicators.go` (`growthPointsToTimeSeries`): Remove `externalBalanceTotal` parameter.
  ```go
  value := p.TotalValue  // no longer add external balance
  TotalCapital: value + p.CashBalance,
  ExternalBalance: 0,  // deprecated, keep field for API compat
  ```
- In `indicators.go` (`growthToBars`): Same — remove external balance addition.
  ```go
  value := p.TotalValue  // no longer add external balance
  ```
- In `GetPortfolioIndicators`: Remove `portfolio.ExternalBalanceTotal` argument from both calls.

### Problem 3: NetDeployedImpact ignores negative contributions (fb_7d8dafdb)

**Current code** — `internal/models/cashflow.go:78-90`:
```go
func (tx CashTransaction) NetDeployedImpact() float64 {
    switch tx.Category {
    case CashCatContribution:
        if tx.Amount > 0 {
            return tx.Amount
        }
    case CashCatOther, CashCatFee, CashCatTransfer:
        if tx.Amount < 0 {
            return tx.Amount
        }
    }
    return 0
}
```

Negative contributions (capital withdrawals, category=contribution, amount<0) return 0.
This means net_deployed never decreases when capital is withdrawn.

**Fix**: Return `tx.Amount` for ALL contributions regardless of sign:
```go
case CashCatContribution:
    return tx.Amount  // positive = deposit increases, negative = withdrawal decreases
```

### Problem 4: Broken portfolio_netflow_test.go (fb_20ac6ee8)

**Current test code** uses the OLD API format:
```go
"type": "deposit", "amount": 15000
"type": "withdrawal", "amount": 20000
"type": "contribution", "amount": 10000
"type": "dividend", "amount": 5000
```

The current API expects:
```go
"account": "Trading", "category": "contribution", "amount": 15000
"account": "Trading", "category": "contribution", "amount": -20000  // withdrawal
"account": "Trading", "category": "dividend", "amount": 5000
```

The `type` field doesn't map to any field on `CashTransaction`. The handler decodes JSON directly
into `models.CashTransaction`, so `type: "deposit"` is ignored and `Category` is empty.
These tests silently fail or test the wrong behavior.

**Fix**: Update ALL test requests in `tests/api/portfolio_netflow_test.go` to use the current format:
- `type: "deposit"` → `account: "Trading", category: "contribution"` (positive amount)
- `type: "withdrawal"` → `account: "Trading", category: "contribution"` (negative amount)
- `type: "contribution"` → `account: "Trading", category: "contribution"` (positive amount)
- `type: "dividend"` → `account: "Trading", category: "dividend"` (positive amount)

---

## Files to Change

### 1. `internal/models/portfolio.go` — Rename ExternalBalanceTotal → TotalCash

**Line 59**: Rename field and JSON tag
```go
// Before:
ExternalBalanceTotal     float64             `json:"external_balance_total"`
// After:
TotalCash                float64             `json:"total_cash"`
```

**Lines 212-213**: Update GrowthDataPoint comment (ExternalBalance is now deprecated)
```go
ExternalBalance float64 // Deprecated — cash is tracked in CashBalance
```

### 2. `internal/services/portfolio/service.go` — Use TotalCashBalance

**Lines 402-411**: Change variable name and calculation
```go
// Before:
var externalBalanceTotal float64
if s.cashflowSvc != nil {
    if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
        externalBalanceTotal = ledger.NonTransactionalBalance()
    }
}
weightDenom := totalValue + externalBalanceTotal

// After:
var totalCash float64
if s.cashflowSvc != nil {
    if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
        totalCash = ledger.TotalCashBalance()
    }
}
weightDenom := totalValue + totalCash
```

**Lines 414, 430, 439**: Update all references from `externalBalanceTotal` to `totalCash`,
and from `ExternalBalanceTotal` to `TotalCash`.

### 3. `internal/services/portfolio/growth.go` — Fix TotalCapital double-counting

**Line 238-239**: Remove ExternalBalance from TotalCapital
```go
// Before:
ExternalBalance: p.ExternalBalanceTotal,
TotalCapital:    totalValue + runningCashBalance + p.ExternalBalanceTotal,

// After:
ExternalBalance: 0,
TotalCapital:    totalValue + runningCashBalance,
```

### 4. `internal/services/portfolio/indicators.go` — Remove ExternalBalance additions

**`growthPointsToTimeSeries` function (lines 15-33)**: Remove `externalBalanceTotal` parameter
```go
// Before:
func growthPointsToTimeSeries(points []models.GrowthDataPoint, externalBalanceTotal float64) []models.TimeSeriesPoint {
    ...
    value := p.TotalValue + externalBalanceTotal
    ...
    ExternalBalance: externalBalanceTotal,
    TotalCapital:    value + p.CashBalance,

// After:
func growthPointsToTimeSeries(points []models.GrowthDataPoint) []models.TimeSeriesPoint {
    ...
    value := p.TotalValue
    ...
    ExternalBalance: 0,
    TotalCapital:    value + p.CashBalance,
```

**`growthToBars` function (lines 38+)**: Remove `externalBalanceTotal` parameter
```go
// Before:
func growthToBars(points []models.GrowthDataPoint, externalBalanceTotal float64) []models.EODBar {
    ...
    value := p.TotalValue + externalBalanceTotal

// After:
func growthToBars(points []models.GrowthDataPoint) []models.EODBar {
    ...
    value := p.TotalValue
```

**`GetPortfolioIndicators` function (lines 112-113)**: Update calls
```go
// Before:
bars := growthToBars(growth, portfolio.ExternalBalanceTotal)
timeSeries := growthPointsToTimeSeries(growth, portfolio.ExternalBalanceTotal)

// After:
bars := growthToBars(growth)
timeSeries := growthPointsToTimeSeries(growth)
```

### 5. `internal/models/cashflow.go` — Fix NetDeployedImpact

**Lines 78-90**: Fix negative contribution handling
```go
// Before:
case CashCatContribution:
    if tx.Amount > 0 {
        return tx.Amount
    }

// After:
case CashCatContribution:
    return tx.Amount  // positive deposits and negative withdrawals both affect net deployed
```

### 6. `tests/api/portfolio_netflow_test.go` — Fix broken tests

Update ALL HTTP POST requests to use the current API format. Every instance of:
- `"type": "deposit"` → `"account": "Trading", "category": "contribution"` (keep positive amount)
- `"type": "withdrawal"` → `"account": "Trading", "category": "contribution"` (negate the amount)
- `"type": "contribution"` → `"account": "Trading", "category": "contribution"` (keep amount as-is)
- `"type": "dividend"` → `"account": "Trading", "category": "dividend"` (keep positive amount)

Remove the `"type"` field from all request bodies.

Specific changes needed in each test:
- **TestPortfolioNetFlow_YesterdayFlow** (line 77): `"type": "deposit"` → `"account": "Trading", "category": "contribution"`
- **TestPortfolioNetFlow_LastWeekFlow** (lines 147-177): Update all 4 transaction bodies
  - `"type": "deposit"` → `"account": "Trading", "category": "contribution"`
  - `"type": "contribution"` → `"account": "Trading", "category": "contribution"`
  - `"type": "withdrawal"` → `"account": "Trading", "category": "contribution"` AND negate amount to `-5000`
  - `"type": "deposit"` → `"account": "Trading", "category": "contribution"`
  - Update `netContrib` for withdrawal to `-5000` (already -5000, OK)
- **TestPortfolioNetFlow_NegativeFlow** (lines 240-259):
  - `"type": "deposit"` → `"account": "Trading", "category": "contribution"`
  - `"type": "withdrawal", "amount": 20000` → `"account": "Trading", "category": "contribution", "amount": -20000`
  - Update assertion: `-15000` → `5000 + (-20000) = -15000` (still correct with signed amounts)
- **TestPortfolioNetFlow_OnlyOutsideWindowTransactions** (line 308):
  - `"type": "deposit"` → `"account": "Trading", "category": "contribution"`
- **TestPortfolioNetFlow_DividendExcluded** (lines 372-391):
  - `"type": "deposit"` → `"account": "Trading", "category": "contribution"`
  - `"type": "dividend"` → `"account": "Trading", "category": "dividend"`
- **TestPortfolioNetFlow_PersistsAfterSync** (line 439):
  - `"type": "deposit"` → `"account": "Trading", "category": "contribution"`

### 7. Grep for all remaining references to update

After the above changes, grep for any remaining references to `ExternalBalanceTotal` or
`external_balance_total` in the codebase and update them. Key locations:

- `internal/server/handlers.go` — portfolio response serialization uses `portfolio.ExternalBalanceTotal`
  Search for and update any handler code that reads this field.
- `internal/services/portfolio/capital_timeline_test.go` — may reference the field
- `internal/services/portfolio/indicators_test.go` — may reference the field
- Any other test files referencing `ExternalBalanceTotal`

Use `grep -r "ExternalBalanceTotal\|external_balance_total\|NonTransactionalBalance" internal/` to find all references.

---

## Unit Tests

### Test: NetDeployedImpact with negative contributions
**File**: `internal/models/cashflow_test.go` or add to existing test file
```go
func TestNetDeployedImpact_NegativeContribution(t *testing.T) {
    tx := CashTransaction{Category: CashCatContribution, Amount: -5000}
    assert.Equal(t, -5000.0, tx.NetDeployedImpact())
}
```

### Test: TotalCash replaces ExternalBalanceTotal
Add unit tests verifying that `TotalCash` on the portfolio response equals `TotalCashBalance()`
from the ledger (sum of all account transactions).

---

## Task: Mark fb_7ffa974f as resolved
The `TotalDeposited()` and `TotalWithdrawn()` functions at `cashflow.go:154-178` already correctly
filter by `category=contribution`. This was fixed in the Capital Performance Category Filter work
(2026-02-28 23:00). Mark fb_7ffa974f as resolved with note:
"Already fixed — TotalDeposited/TotalWithdrawn filter by category=contribution only (cashflow.go:154-178)."
