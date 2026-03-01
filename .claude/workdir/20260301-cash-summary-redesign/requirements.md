# Cash Flow Summary Redesign

**Feedback:** `fb_4e848a89` (prod)
**Replaces:** The `CashFlowSummary` shipped in `b7eb4ed`

## Problem

The current summary (`total_credits`, `total_debits`, `net_cash_flow`) is misleading:
- Transfers are double-counted — a $50k transfer appears as a credit to one account and a debit from another, inflating both totals
- `net_cash_flow` is therefore meaningless in an account-based ledger
- The summary tells you nothing actionable

## New Design

Replace the flat credits/debits summary with two things:

### 1. Per-account `balance` field

Add a computed `balance` field to each account object in the `accounts` array.

**Before:**
```json
"accounts": [
  {"name": "Trading", "type": "trading", "is_transactional": true},
  {"name": "Stake Accumulate", "type": "accumulate", "is_transactional": false}
]
```

**After:**
```json
"accounts": [
  {"name": "Trading", "type": "trading", "is_transactional": true, "balance": 428004.88},
  {"name": "Stake Accumulate", "type": "accumulate", "is_transactional": false, "balance": 50000.00}
]
```

Balance = sum of all signed amounts for that account's transactions.

### 2. Redesigned `summary` object

```json
"summary": {
  "total_cash": 478004.88,
  "transaction_count": 47,
  "by_category": {
    "contribution": 477014.62,
    "dividend": 1019.79,
    "transfer": 0.00,
    "fee": -29.53,
    "other": 0.00
  }
}
```

- `total_cash` = sum of all account balances (same as `TotalCashBalance()`)
- `transaction_count` = total number of transactions
- `by_category` = net amount per category across all transactions. Transfers should net to zero (paired credit/debit). Each value is the sum of signed amounts for that category.

### 3. Remove old fields

Remove `total_credits`, `total_debits`, `net_cash_flow` from `CashFlowSummary`.

## Files to Change

### 1. `internal/models/cashflow.go`

**Replace `CashFlowSummary` struct** (lines 103-109):

```go
// CashFlowSummary contains server-computed aggregate totals for the ledger.
type CashFlowSummary struct {
	TotalCash        float64            `json:"total_cash"`        // Sum of all account balances
	TransactionCount int                `json:"transaction_count"` // Total number of transactions
	ByCategory       map[string]float64 `json:"by_category"`       // Net amount per category
}
```

**Replace `Summary()` method** (lines 112-127):

```go
// Summary computes aggregate totals across all transactions in the ledger.
func (l *CashFlowLedger) Summary() CashFlowSummary {
	byCategory := make(map[string]float64)
	for _, tx := range l.Transactions {
		byCategory[string(tx.Category)] += tx.Amount
	}
	// Ensure all known categories are present (even if zero).
	for _, cat := range []CashCategory{CashCatContribution, CashCatDividend, CashCatTransfer, CashCatFee, CashCatOther} {
		if _, ok := byCategory[string(cat)]; !ok {
			byCategory[string(cat)] = 0
		}
	}
	return CashFlowSummary{
		TotalCash:        l.TotalCashBalance(),
		TransactionCount: len(l.Transactions),
		ByCategory:       byCategory,
	}
}
```

**Add `Balance` field to `CashAccount`** (line 36-40):

Change `CashAccount` to:
```go
type CashAccount struct {
	Name            string  `json:"name"`
	Type            string  `json:"type"`             // trading (default), accumulate, term_deposit, offset
	IsTransactional bool    `json:"is_transactional"`
	Balance         float64 `json:"balance,omitempty"` // Computed: sum of signed amounts for this account
}
```

Note: `omitempty` so it's not stored as 0 in the ledger — it's computed on output only.

### 2. `internal/server/handlers.go`

**Update `cashFlowResponse` struct** (lines 1644-1654):

The `Accounts` field needs to be populated with computed balances. Update `newCashFlowResponse`:

```go
func newCashFlowResponse(ledger *models.CashFlowLedger) cashFlowResponse {
	// Compute per-account balances.
	accounts := make([]models.CashAccount, len(ledger.Accounts))
	copy(accounts, ledger.Accounts)
	for i := range accounts {
		accounts[i].Balance = ledger.AccountBalance(accounts[i].Name)
	}
	return cashFlowResponse{
		PortfolioName: ledger.PortfolioName,
		Version:       ledger.Version,
		Accounts:      accounts,
		Transactions:  ledger.Transactions,
		Summary:       ledger.Summary(),
		Notes:         ledger.Notes,
		CreatedAt:     ledger.CreatedAt,
		UpdatedAt:     ledger.UpdatedAt,
	}
}
```

`AccountBalance()` already exists on `CashFlowLedger` (line 130) — reuse it.

### 3. `internal/server/catalog.go`

**Update `list_cash_transactions` description** (line 372):

```go
Description: "List all cash accounts and transactions for a portfolio. Response includes per-account balances and a summary with total_cash and net amounts by_category (contribution, dividend, transfer, fee, other). Accounts with is_transactional=true have trade settlements auto-applied to their balance.",
```

### 4. `internal/models/cashflow_test.go`

**Replace `TestCashFlowLedger_Summary`** (lines 431-531) with new test covering the redesigned fields:

```
TestCashFlowLedger_Summary subtests:
1. empty_ledger — all zeros, all categories present in by_category
2. single_category — contribution only: total_cash = sum, by_category.contribution = sum, others = 0
3. mixed_categories — contributions, dividends, fees: verify each category net
4. transfers_net_to_zero — paired +/- transfer entries: by_category.transfer = 0, total_cash unchanged
5. multi_account — 2 accounts: total_cash = sum of both account balances
```

### 5. `internal/models/cashflow_stress_test.go`

**Update stress tests** — all references to `TotalCredits`, `TotalDebits`, `NetCashFlow` must be replaced with `TotalCash`, `TransactionCount`, `ByCategory` assertions. There are 11 stress tests referencing the old fields.

### 6. `tests/api/cashflow_test.go`

**Update `TestCashFlowResponseSummary`** (lines 815-921+):

Replace assertions on `total_credits`/`total_debits`/`net_cash_flow` with:
- `summary.total_cash` = net of all amounts
- `summary.transaction_count` = count
- `summary.by_category.contribution` = net contributions
- `summary.by_category.fee` = net fees
- Verify `accounts[*].balance` fields present and correct

### 7. `docs/features/20260228-cash-transaction-response-totals.md`

Update the "Required Response" section to reflect the new summary structure. Update status to note the redesign.

## Out of Scope

- Portal changes (separate repo)
- Changes to `CapitalPerformance` or `get_capital_performance`
- Changes to `add_cash_transaction` or `set_cash_transactions` input formats
- The `total_cash` replacing `external_balance_total` in portfolio response (fb_d895f8f9 — separate task)

## Test Cases

### Unit Tests (`TestCashFlowLedger_Summary`)
1. **empty_ledger** — Summary of empty ledger: TotalCash=0, TransactionCount=0, all 5 categories present with value 0
2. **single_category** — 3 contributions: TotalCash=sum, by_category.contribution=sum, others=0
3. **mixed_categories** — contributions + dividends + fees: each category correct
4. **transfers_net_to_zero** — Transfer pair (+1000, -1000): by_category.transfer=0
5. **multi_account** — Transactions across 2 accounts: TotalCash = sum of all

### Integration Tests (`TestCashFlowResponseSummary`)
1. Add contribution, fee, and transfer pair
2. GET response has `summary.total_cash`, `summary.by_category.*`
3. Each account in `accounts` array has `balance` field
4. POST response also has redesigned summary
