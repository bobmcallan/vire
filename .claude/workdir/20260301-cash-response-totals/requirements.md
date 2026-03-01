# Cash Transaction Response: Server-Side Summary Totals

**Feature doc:** `docs/features/20260228-cash-transaction-response-totals.md`
**Status:** Implementation

## Scope

The feature doc lists 4 changes. Items 2-4 are **already complete** (commit `7ee0acf`):
- ~~Remove `direction` field~~ ✅ Done — `CashTransaction` has no Direction field
- ~~Replace `type` with `category`~~ ✅ Done — model uses `Category CashCategory`
- ~~Confirm `account` field~~ ✅ Done — `Account string` field present
- ~~MCP `add_cash_transaction` tool updated~~ ✅ Done — uses `category` + signed amounts

**Remaining work:** Add server-side `summary` object to the `list_cash_transactions` (GET) response.

## Current Behavior

`GET /api/portfolios/{name}/cash-transactions` returns `CashFlowLedger` directly:

```json
{
  "portfolio_name": "SMSF",
  "version": 1,
  "accounts": [...],
  "transactions": [...],
  "notes": "",
  "created_at": "...",
  "updated_at": "..."
}
```

## Required Behavior

Add a `summary` field to the response. Preserve all existing fields for backward compatibility:

```json
{
  "portfolio_name": "SMSF",
  "version": 1,
  "accounts": [...],
  "transactions": [...],
  "summary": {
    "total_credits": 477014.62,
    "total_debits": 618326.00,
    "net_cash_flow": -141311.38,
    "transaction_count": 47
  },
  "notes": "",
  "created_at": "...",
  "updated_at": "..."
}
```

## Files to Change

### 1. `internal/models/cashflow.go` — Add summary struct and method

After `CashFlowLedger` (line 101), add:

```go
// CashFlowSummary contains server-computed aggregate totals across all transactions.
type CashFlowSummary struct {
	TotalCredits     float64 `json:"total_credits"`     // Sum of all positive amounts
	TotalDebits      float64 `json:"total_debits"`      // Sum of abs(negative amounts)
	NetCashFlow      float64 `json:"net_cash_flow"`     // total_credits - total_debits
	TransactionCount int     `json:"transaction_count"` // Total number of transactions
}

// Summary computes aggregate totals across all transactions in the ledger.
func (l *CashFlowLedger) Summary() CashFlowSummary {
	var credits, debits float64
	for _, tx := range l.Transactions {
		if tx.Amount > 0 {
			credits += tx.Amount
		} else if tx.Amount < 0 {
			debits += math.Abs(tx.Amount)
		}
	}
	return CashFlowSummary{
		TotalCredits:     credits,
		TotalDebits:      debits,
		NetCashFlow:      credits - debits,
		TransactionCount: len(l.Transactions),
	}
}
```

Note: `math` is already imported (line 4).

### 2. `internal/server/handlers.go` — Wrap GET response with summary

**Line 1658**: Change the GET handler response from returning the raw ledger to wrapping it with a summary.

Replace (line 1658):
```go
WriteJSON(w, http.StatusOK, ledger)
```

With:
```go
WriteJSON(w, http.StatusOK, newCashFlowResponse(ledger))
```

Add a helper (at top of handler or as a package-level function near `handleCashFlows`):

```go
// cashFlowResponse wraps CashFlowLedger with computed summary totals.
type cashFlowResponse struct {
	PortfolioName string                  `json:"portfolio_name"`
	Version       int                     `json:"version"`
	Accounts      []models.CashAccount    `json:"accounts"`
	Transactions  []models.CashTransaction `json:"transactions"`
	Summary       models.CashFlowSummary  `json:"summary"`
	Notes         string                  `json:"notes,omitempty"`
	CreatedAt     time.Time               `json:"created_at"`
	UpdatedAt     time.Time               `json:"updated_at"`
}

func newCashFlowResponse(ledger *models.CashFlowLedger) cashFlowResponse {
	return cashFlowResponse{
		PortfolioName: ledger.PortfolioName,
		Version:       ledger.Version,
		Accounts:      ledger.Accounts,
		Transactions:  ledger.Transactions,
		Summary:       ledger.Summary(),
		Notes:         ledger.Notes,
		CreatedAt:     ledger.CreatedAt,
		UpdatedAt:     ledger.UpdatedAt,
	}
}
```

**Why explicit struct instead of embedding:** Embedding `*CashFlowLedger` would work for marshaling but the field order and `omitempty` behavior could differ. Explicit fields ensure the JSON output matches the spec exactly and keeps the response contract clear.

Also apply to POST (line 1674) and PUT (line 1700) responses for consistency — all three return the ledger:
- Line 1674: `WriteJSON(w, http.StatusCreated, newCashFlowResponse(ledger))`
- Line 1700: `WriteJSON(w, http.StatusOK, newCashFlowResponse(ledger))`

### 3. `internal/server/catalog.go` — Update tool description

**Line 371-372**: Update `list_cash_transactions` description to mention summary:

```go
Description: "List all cash accounts and transactions for a portfolio. Response includes a summary object with server-computed totals (total_credits, total_debits, net_cash_flow, transaction_count). Each transaction is a credit (positive amount) or debit (negative amount) to a named account.",
```

### 4. `docs/features/20260228-cash-transaction-response-totals.md` — Update status

**Line 4**: Change `Status: Requirements` to `Status: Implemented`

## Unit Tests

Add to `internal/models/cashflow_test.go` (create if needed, or add to existing model tests).
If no `cashflow_test.go` exists in `internal/models/`, add tests to `internal/services/cashflow/service_test.go` in a clearly labeled section.

### Test: `TestCashFlowLedger_Summary`

Subtests:
1. **empty_ledger** — Summary of empty transactions: all zeros
2. **credits_only** — 3 positive transactions: TotalCredits = sum, TotalDebits = 0, NetCashFlow = sum, Count = 3
3. **debits_only** — 3 negative transactions: TotalCredits = 0, TotalDebits = sum(abs), NetCashFlow = -sum, Count = 3
4. **mixed** — Mix of positive and negative: verify all 4 fields
5. **zero_amount** — Transaction with amount=0: counts in TransactionCount, doesn't affect totals

## Integration Tests

Add a test to `tests/api/cashflow_test.go`:

### Test: `TestCashFlowResponseSummary`

1. Create portfolio, add 3 transactions (2 credits, 1 debit)
2. GET cash-transactions
3. Assert response contains `summary` object
4. Assert `summary.total_credits` matches sum of positive amounts
5. Assert `summary.total_debits` matches abs of negative amount
6. Assert `summary.net_cash_flow` = total_credits - total_debits
7. Assert `summary.transaction_count` = 3
8. Assert existing fields still present (portfolio_name, accounts, transactions, etc.)

## Out of Scope

- Pagination — not currently implemented, not adding it
- Filtering the summary by account/category — future feature
- Changing POST/PUT request formats — already correct
- Portal changes — separate repo, separate task
