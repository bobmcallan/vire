# Clear Cash Transactions MCP Tool

**Feedback:** `fb_5bcf0b2d` (prod)

## Scope

Add a dedicated `clear_cash_transactions` MCP tool that wipes all transactions AND accounts for a portfolio, returning an empty ledger with only the default Trading account.

**In scope:**
- New `ClearLedger()` method on cashflow service
- New `DELETE` handler case in `handleCashFlows`
- New tool definition in catalog
- Interface update
- Unit + integration tests

**Out of scope:**
- No changes to other cash flow tools
- No portal changes

## Files to Change

### 1. `internal/interfaces/services.go` — Add interface method

After line 265 (after `SetTransactions`), add:

```go
// ClearLedger wipes all transactions and accounts, returning an empty ledger with default Trading account
ClearLedger(ctx context.Context, portfolioName string) (*models.CashFlowLedger, error)
```

### 2. `internal/services/cashflow/service.go` — Implement ClearLedger

Add after `SetTransactions` method. Follow the same pattern: get → modify → save.

```go
// ClearLedger wipes all transactions and accounts for a portfolio,
// returning an empty ledger with only the default Trading account.
func (s *Service) ClearLedger(ctx context.Context, portfolioName string) (*models.CashFlowLedger, error) {
	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	ledger.Accounts = []models.CashAccount{
		{Name: models.DefaultTradingAccount, Type: "trading", IsTransactional: true},
	}
	ledger.Transactions = []models.CashTransaction{}

	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Warn().Str("portfolio", portfolioName).
		Msg("Cash ledger cleared — all transactions and accounts removed")
	return ledger, nil
}
```

Note: Use `Warn()` level because this is a destructive operation — important for audit trail.

### 3. `internal/server/handlers.go` — Add DELETE case

In `handleCashFlows` (currently around line 1745), add a `case http.MethodDelete:` before the `default:` case:

```go
case http.MethodDelete:
	ledger, err := s.app.CashFlowService.ClearLedger(ctx, name)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error clearing cash ledger: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, newCashFlowResponse(ledger))
```

Update the `default:` line to include `http.MethodDelete`:
```go
RequireMethod(w, r, http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete)
```

### 4. `internal/server/catalog.go` — Add tool definition

Add after the last cash flow tool (around line 562, after `remove_cash_transaction`):

```go
{
	Name:        "clear_cash_transactions",
	Description: "Completely wipe all transactions and accounts for a portfolio. Returns an empty ledger with only the default Trading account. This is a destructive operation — use with caution.",
	Method:      "DELETE",
	Path:        "/api/portfolios/{portfolio_name}/cash-transactions",
	Params: []models.ParamDefinition{
		portfolioParam,
	},
},
```

Update tool count comment if one exists.

### 5. Mock updates

If there's a mock `CashFlowService` in test files, add the `ClearLedger` method to it. Check:
- `internal/server/handlers_oauth_test.go` or similar files for mock implementations
- Any test file that implements the `CashFlowService` interface

## Unit Tests

Add to `internal/services/cashflow/service_test.go`:

### `TestClearLedger`

Subtests:
1. **clears_all** — Add 3 transactions to 2 accounts, call ClearLedger, verify: transactions empty, only default Trading account remains, version incremented
2. **empty_ledger** — Call ClearLedger on empty ledger: returns default state, no error
3. **preserves_portfolio_name** — After clear, portfolio_name matches original

## Integration Tests

Add to `tests/api/cashflow_test.go`:

### `TestCashFlowClear`

1. Create portfolio, add 3 transactions across 2 accounts
2. `DELETE /api/portfolios/{name}/cash-transactions`
3. Assert status 200
4. Assert response has empty transactions array
5. Assert response has only default Trading account
6. Assert summary shows total_cash=0, all by_category values=0
7. GET to confirm persistence — ledger is truly empty
