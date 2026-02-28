# Requirements: Signed Amounts & Account-Based External Balances

**Feedback:** fb_94f33577
**Also closes:** fb_f26501fd, fb_20ac6ee8, fb_05720af4

## Summary

Redesign cash transactions: remove `Direction` field, use signed `Amount` (+/-). Replace external balances with non-transactional account balances derived from the ledger. Add `update_account` tool.

## Changes

### 1. Remove Direction from CashTransaction

**File: `internal/models/cashflow.go`**

- Delete `CashDirection` type, constants (`CashCredit`, `CashDebit`), `validCashDirections`, `ValidCashDirection()`
- Remove `Direction CashDirection` field from `CashTransaction` struct
- Remove `IsCredit()` method
- Simplify `SignedAmount()` → `return tx.Amount` (keep method for API clarity)
- Update `NetDeployedImpact()`: use `tx.Amount > 0` / `tx.Amount < 0` instead of Direction checks
- Update `TotalDeposited()`: `Amount > 0` instead of `Direction == CashCredit`
- Update `TotalWithdrawn()`: `Amount < 0`, return `math.Abs(tx.Amount)` instead of Direction check
- Comment on Amount: "Positive = money in (credit), negative = money out (debit)"

### 2. Update CashAccount Model

**File: `internal/models/cashflow.go`**

- Add `Type` field to `CashAccount`: trading (default), accumulate, term_deposit, offset
- `IsTransactional` derived from Type: only "trading" is transactional
- Add `NonTransactionalBalance()` method to CashFlowLedger — sums balances of non-transactional accounts
- Remove `ExternalBalancePerformance`, `ExternalBalanceCategories` — replaced by account-based logic

### 3. Update CashFlow Service

**File: `internal/services/cashflow/service.go`**

- Remove Direction validation in `validateCashTransaction()` — validate Amount != 0 instead
- `AddTransaction()`: remove Direction assignment, amount is already signed
- `AddTransfer()`: create -amount on from_account, +amount on to_account (no Direction)
- `UpdateTransaction()`: remove Direction merge logic
- `CalculatePerformance()`: use Amount sign instead of Direction checks. Replace ExternalBalancePerformance with account-based aggregation.
- `deriveFromTrades()`: buys = negative amount, sells = positive amount (no Direction)
- `computeXIRR()`: Amount is already signed, no conversion needed

### 4. Update Consumers

**`internal/services/portfolio/growth.go`** (lines 216-217):
- `SignedAmount()` still works (returns tx.Amount), no change needed here

**`internal/services/portfolio/service.go`**:
- `populateNetFlows()` uses `ledger.NetFlowForPeriod()` which uses `SignedAmount()` — no change needed
- Update portfolio assembly to compute `ExternalBalanceTotal` from ledger non-transactional account balances instead of Portfolio.ExternalBalances

### 5. Replace External Balances with Account Balances

**`internal/services/portfolio/external_balances.go`** — DELETE entirely
**`internal/services/portfolio/external_balances_test.go`** — DELETE
**`internal/services/portfolio/external_balances_stress_test.go`** — DELETE
**`internal/services/portfolio/sync_external_balance_stress_test.go`** — DELETE

**`internal/models/portfolio.go`**:
- Remove `ExternalBalances []ExternalBalance` and `ExternalBalanceTotal float64` from Portfolio
- Remove `ExternalBalance` struct and `ValidExternalBalanceTypes`
- Keep `ExternalBalanceTotal float64` in Portfolio — now populated from ledger

**`internal/interfaces/services.go`**:
- Remove external balance methods from PortfolioService if present

**`internal/server/handlers.go`**:
- Remove external balance HTTP handlers
- Remove external balance routes from routes.go

### 6. Update MCP Tool Catalog

**`internal/server/catalog.go`**:
- `add_cash_transaction`: remove `direction` param, update `amount` description to "Positive for deposits/credits, negative for withdrawals/debits"
- `update_cash_transaction`: remove `direction` param
- Remove external balance tools: `get_external_balances`, `set_external_balances`, `add_external_balance`, `remove_external_balance`
- Add `update_account` tool: POST `/api/portfolios/{portfolio_name}/cash-accounts/{account_name}` with `is_transactional` (bool) and `type` (string) params

### 7. Add update_account Endpoint

**`internal/services/cashflow/service.go`**:
- Add `UpdateAccount()` method to update account properties
- Add to `CashFlowService` interface

**`internal/server/handlers.go`**:
- Add `handleUpdateAccount()` handler
- Add route in routes.go

### 8. Portfolio Response: External Balance from Ledger

**`internal/services/portfolio/service.go`**:
- After loading ledger, compute `ExternalBalanceTotal` = sum of non-transactional account balances
- Add to `total_value` = holdings + non-transactional account balances

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Remove Direction, update Amount semantics, add account Type |
| `internal/models/portfolio.go` | Remove ExternalBalance struct, keep ExternalBalanceTotal field |
| `internal/services/cashflow/service.go` | Remove Direction handling, add UpdateAccount, update perf calc |
| `internal/services/portfolio/growth.go` | Minimal — SignedAmount() still works |
| `internal/services/portfolio/service.go` | Compute ExternalBalanceTotal from ledger |
| `internal/services/portfolio/external_balances.go` | DELETE |
| `internal/interfaces/services.go` | Remove external balance methods, add UpdateAccount |
| `internal/server/catalog.go` | Update tool schemas |
| `internal/server/handlers.go` | Remove external balance handlers, add update_account |
| `internal/server/routes.go` | Update routes |
| All test files with CashTransaction | Remove Direction field usage |

## Key Design Decisions

1. **SignedAmount() stays** as `return tx.Amount` — keeps the API clear for consumers
2. **No legacy format support** — old stored data with Direction field will be ignored by json.Unmarshal (Direction field silently dropped). Old data needs to be re-entered.
3. **Account auto-creation** unchanged — accounts created on first transaction use
4. **is_transactional defaults to true** — only explicitly non-transactional types are false
5. **External balance total** derived from non-transactional account balances in the ledger
