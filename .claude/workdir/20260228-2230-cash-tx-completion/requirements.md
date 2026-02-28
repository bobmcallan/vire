# Requirements: Bulk set_cash_transactions Tool

**Feedback:** fb_0ac33209 (item 2)
**Previous work:** 7ee0acf (signed amounts refactor)

## Summary

Add a `set_cash_transactions` MCP tool that replaces all transactions at once, following the `set_portfolio_plan` / `set_portfolio_watchlist` bulk-replace pattern.

## Scope

**In scope:**
- `set_cash_transactions` tool: PUT on existing `/api/portfolios/{portfolio_name}/cash-transactions`
- Service method: `SetTransactions`
- Interface update, handler update, catalog entry, unit tests

**Out of scope:**
- Portal changes (portal reads `category` for TYPE column — portal fix, not server)
- Transfer display changes (server already returns `account`, `category`, `linked_id` — portal uses these)
- Direction removal (already done in 7ee0acf)
- Capital performance fixes (already done in 7ee0acf)

## Changes

### 1. Add SetTransactions to CashFlowService interface

**File: `internal/interfaces/services.go` — line ~260 (inside CashFlowService interface)**

Add after `RemoveTransaction`:
```go
SetTransactions(ctx context.Context, portfolioName string, transactions []models.CashTransaction, notes string) (*models.CashFlowLedger, error)
```

### 2. Implement SetTransactions in cashflow service

**File: `internal/services/cashflow/service.go`**

Add after `RemoveTransaction` (around line 385). Follow AddTransaction pattern (lines 120-157) for ID generation, account auto-creation, and validation.

```go
// SetTransactions replaces all transactions in the ledger.
// Preserves existing accounts. Auto-creates accounts referenced by new transactions.
func (s *Service) SetTransactions(ctx context.Context, portfolioName string, transactions []models.CashTransaction, notes string) (*models.CashFlowLedger, error) {
	// 1. Get existing ledger (preserves accounts)
	ledger, err := s.GetLedger(ctx, portfolioName)
	if err != nil {
		return nil, err
	}

	// 2. Validate each transaction
	for i, tx := range transactions {
		if err := validateCashTransaction(tx); err != nil {
			return nil, fmt.Errorf("invalid cash transaction at index %d: %w", i, err)
		}
	}

	// 3. Assign IDs, timestamps, trim whitespace, auto-create accounts
	//    Follow AddTransaction pattern at lines 130-148
	now := time.Now()
	for i := range transactions {
		transactions[i].ID = generateCashTransactionID()
		transactions[i].Account = strings.TrimSpace(transactions[i].Account)
		transactions[i].Description = strings.TrimSpace(transactions[i].Description)
		transactions[i].CreatedAt = now
		transactions[i].UpdatedAt = now

		// Auto-create account — follow pattern at lines 139-144
		if !ledger.HasAccount(transactions[i].Account) {
			ledger.Accounts = append(ledger.Accounts, models.CashAccount{
				Name:            transactions[i].Account,
				Type:            "other",
				IsTransactional: false,
			})
		}
	}

	// 4. Replace all transactions
	ledger.Transactions = transactions
	if notes != "" {
		ledger.Notes = notes
	}
	sortTransactionsByDate(ledger)

	// 5. Save
	if err := s.saveLedger(ctx, ledger); err != nil {
		return nil, err
	}

	s.logger.Info().Str("portfolio", portfolioName).
		Int("count", len(transactions)).Msg("Cash transactions replaced (bulk set)")
	return ledger, nil
}
```

### 3. Add PUT case to handleCashFlows handler

**File: `internal/server/handlers.go` — inside handleCashFlows (line 1644)**

The handler currently has GET and POST cases. Add PUT case before `default:`.
Follow the `handlePortfolioPlan` PUT pattern (lines 1237-1265):

```go
case http.MethodPut:
	var raw struct {
		Items json.RawMessage `json:"items"`
		Notes string          `json:"notes"`
	}
	if !DecodeJSON(w, r, &raw) {
		return
	}
	var transactions []models.CashTransaction
	if len(raw.Items) > 0 {
		if err := UnmarshalArrayParam(raw.Items, &transactions); err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid items: "+err.Error())
			return
		}
	}
	ledger, err := s.app.CashFlowService.SetTransactions(ctx, name, transactions, raw.Notes)
	if err != nil {
		if strings.Contains(err.Error(), "invalid cash transaction") {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Error setting cash transactions: %v", err))
		return
	}
	WriteJSON(w, http.StatusOK, ledger)
```

Update the `default:` RequireMethod call to include `http.MethodPut`:
```go
default:
	RequireMethod(w, r, http.MethodGet, http.MethodPost, http.MethodPut)
```

### 4. Add set_cash_transactions to MCP catalog

**File: `internal/server/catalog.go`**

Add after `list_cash_transactions` (around line 377). Follow `set_portfolio_plan` pattern (lines 810-833):

```go
{
	Name:        "set_cash_transactions",
	Description: "Replace all cash transactions for a portfolio. Existing transactions are removed and replaced with the provided items. Accounts are preserved; new accounts are auto-created for any account names not already present.",
	Method:      "PUT",
	Path:        "/api/portfolios/{portfolio_name}/cash-transactions",
	Params: []ToolParam{
		{Name: "portfolio_name", Type: "string", Description: "Portfolio name", Required: true, In: "path"},
		{Name: "items", Type: "array", Description: "Array of cash transactions. Each: {account (string, required), category (contribution|dividend|transfer|fee|other, required), date (YYYY-MM-DD, required), amount (number, required — positive for credits, negative for debits), description (string, required), notes (string, optional)}.", Required: true, In: "body"},
		{Name: "notes", Type: "string", Description: "Free-form ledger notes.", Required: false, In: "body"},
	},
},
```

### 5. Update catalog_test.go tool count

**File: `internal/server/catalog_test.go`**

Find the test that checks total tool count and increment by 1.

### 6. Update mock CashFlowService in test helpers

**File: `internal/server/handlers_portfolio_test.go`**

Find `memCashFlowService` mock struct and add `SetTransactions` method:
```go
func (m *memCashFlowService) SetTransactions(ctx context.Context, portfolioName string, transactions []models.CashTransaction, notes string) (*models.CashFlowLedger, error) {
	return &models.CashFlowLedger{PortfolioName: portfolioName, Transactions: transactions, Notes: notes}, nil
}
```

## Unit Tests

**File: `internal/services/cashflow/service_test.go`**

Add these test cases:

| Test Name | What it verifies |
|-----------|-----------------|
| `TestSetTransactions_Empty` | Setting empty array clears all transactions, preserves accounts |
| `TestSetTransactions_ReplacesExisting` | New transactions replace old ones completely |
| `TestSetTransactions_ValidationError` | Invalid transaction (missing account) returns error, nothing saved |
| `TestSetTransactions_AutoCreatesAccounts` | New account names auto-create CashAccount entries |
| `TestSetTransactions_PreservesExistingAccounts` | Accounts from before set are kept even if no tx references them |
| `TestSetTransactions_AssignsIDs` | All transactions get new IDs (input IDs ignored) |
| `TestSetTransactions_SortsByDate` | Transactions sorted by date after set |
| `TestSetTransactions_Notes` | Ledger notes updated when provided |

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/interfaces/services.go` | Add `SetTransactions` to CashFlowService |
| `internal/services/cashflow/service.go` | Implement `SetTransactions` method |
| `internal/services/cashflow/service_test.go` | 8 unit tests for SetTransactions |
| `internal/server/catalog.go` | Add `set_cash_transactions` tool definition |
| `internal/server/catalog_test.go` | Update tool count |
| `internal/server/handlers.go` | Add PUT case to handleCashFlows |
| `internal/server/handlers_portfolio_test.go` | Add SetTransactions to mock |
