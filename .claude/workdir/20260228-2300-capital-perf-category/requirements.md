# Requirements: Fix TotalDeposited/TotalWithdrawn Category Filtering

**Feedback:** fb_7ffa974f
**Also fixes:** fb_f26501fd (total_deposited was 0 — now fixed by signed amounts, but category filtering still wrong)

## Summary

`TotalDeposited()` and `TotalWithdrawn()` in `internal/models/cashflow.go` filter by **amount sign only** — all positive amounts count as deposits, all negative as withdrawals. This is wrong: transfer credits and dividends are being counted as deposits. Only `category == contribution` should count.

## The Fix

### 1. Fix TotalDeposited (cashflow.go lines 128-137)

**Before:**
```go
func (l *CashFlowLedger) TotalDeposited() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Amount > 0 {
			total += tx.Amount
		}
	}
	return total
}
```

**After:**
```go
// TotalDeposited returns the sum of all positive contribution amounts.
// Only category=contribution counts as capital deposited into the fund.
// Dividends, transfers, and fees are not deposits.
func (l *CashFlowLedger) TotalDeposited() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Category == CashCatContribution && tx.Amount > 0 {
			total += tx.Amount
		}
	}
	return total
}
```

### 2. Fix TotalWithdrawn (cashflow.go lines 139-148)

**Before:**
```go
func (l *CashFlowLedger) TotalWithdrawn() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Amount < 0 {
			total += math.Abs(tx.Amount)
		}
	}
	return total
}
```

**After:**
```go
// TotalWithdrawn returns the sum of absolute values of negative contribution amounts.
// Only category=contribution counts as capital withdrawn from the fund.
// Transfer debits, fees, and dividends are not withdrawals of capital.
func (l *CashFlowLedger) TotalWithdrawn() float64 {
	var total float64
	for _, tx := range l.Transactions {
		if tx.Category == CashCatContribution && tx.Amount < 0 {
			total += math.Abs(tx.Amount)
		}
	}
	return total
}
```

### 3. Update ALL tests with wrong assertions

The new semantics: **only `category=contribution` counts for deposited/withdrawn.**

**Rule for recalculating test expected values:**
- TotalDeposited = sum of `Amount` where `Category == "contribution"` AND `Amount > 0`
- TotalWithdrawn = sum of `|Amount|` where `Category == "contribution"` AND `Amount < 0`
- Transfer credits/debits, dividends, fees, other → DO NOT count

**Files with tests to update (read each file, find every TotalDeposited/TotalWithdrawn assertion, recalculate):**

1. `internal/models/cashflow_test.go` — if any direct TotalDeposited/TotalWithdrawn tests exist
2. `internal/services/cashflow/service_test.go` — multiple CalculatePerformance tests
3. `internal/services/cashflow/signed_amounts_stress_test.go` — lines 185-192, 638-645, 803-807, 832-836, 940-944
4. `internal/services/cashflow/capital_perf_stress_test.go` — lines 319-320, 365-367 (DividendInflowCounted test is WRONG — dividends should NOT count)
5. `internal/services/cashflow/cleanup_stress_test.go` — lines 200-210 (transfer credits counted as deposits = WRONG)
6. `internal/services/cashflow/set_transactions_stress_test.go` — lines 809-816
7. `internal/services/cashflow/internal_transfer_stress_test.go` — MANY assertions, all need recalculation
8. `tests/data/cashflow_test.go` — lines 342-343, 370-371, 457-458

**For each test:**
1. Read the test to understand what transactions are created
2. Identify which ones have `Category: models.CashCatContribution`
3. Sum only those for the new expected values
4. Update the assertion

**Example recalculation — capital_perf_stress_test.go DividendInflowCounted:**
- Contribution: +80,000 (deposit)
- Dividend: +5,000 (NOT a deposit)
- New TotalDeposited = 80,000 (not 85,000)

**Example — cleanup_stress_test.go transfer test:**
- Contribution: +100,000 (deposit)
- Transfer credit: +20,000 (NOT a deposit)
- Transfer debit: -20,000 (NOT a withdrawal)
- New TotalDeposited = 100,000 (not 120,000)
- New TotalWithdrawn = 0 (not 20,000)

**Example — internal_transfer_stress_test.go:**
- Each test creates various transactions. Only count contributions.
- Transfer debits should NOT count as withdrawals anymore.
- Fee debits should NOT count as withdrawals anymore.

### 4. Fix test names/comments that describe wrong behavior

- `TestCalculatePerformance_DividendInflowCounted` → rename or update comment to reflect that dividends are NOT counted as deposits
- `cleanup_stress_test.go` comments like "TotalDeposited = 100000 (contribution) + 20000 (transfer credit) = 120000" → update
- `internal_transfer_stress_test.go` comments like "transfer debit is a real withdrawal" → update

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Fix TotalDeposited and TotalWithdrawn (add category filter) |
| `internal/services/cashflow/service_test.go` | Update assertions |
| `internal/services/cashflow/signed_amounts_stress_test.go` | Update assertions |
| `internal/services/cashflow/capital_perf_stress_test.go` | Update assertions + rename test |
| `internal/services/cashflow/cleanup_stress_test.go` | Update assertions + comments |
| `internal/services/cashflow/set_transactions_stress_test.go` | Update assertions |
| `internal/services/cashflow/internal_transfer_stress_test.go` | Update assertions + comments |
| `tests/data/cashflow_test.go` | Update assertions |

## Not in Scope

- `NetDeployedImpact()` — correct as-is (already filters by category), though could be reviewed separately
- `NetFlowForPeriod()` — correct as-is
- Capital timeline (growth.go) — correct as-is (uses NetDeployedImpact)
- `fb_20ac6ee8` (net_flow sign) — separate issue, needs investigation
- `fb_7d8dafdb` (net_deployed flat) — separate issue
