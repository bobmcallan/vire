# Cash Flow Cleanup: Remove Legacy Migration & Transfer Exclusions

## Context

The cash transaction system was just refactored from type-based (deposit/withdrawal/etc.) to account-based (direction + account + category). Two pieces of the refactor need to be cleaned up:

## Requirement 1: Remove Legacy Migration Code

There is only one service and one portal — no backward compatibility needed. The data will be migrated once manually if needed.

### What to remove:

**`internal/models/cashflow.go`** (lines ~186-377):
- `CashTransactionType` type and constants (`CashTxDeposit`, `CashTxWithdrawal`, etc.)
- `LegacyTransaction` struct
- `LegacyLedger` struct
- `MigrateLegacyLedger()` function
- `migrateLegacyTransaction()` function
- `inferAccountName()` function

**`internal/services/cashflow/service.go`** (lines ~112-134 in `GetLedger()`):
- Auto-migration detection and conversion logic (the `if len(ledger.Accounts) == 0` block)

**Tests**: Remove any test cases that test legacy migration (e.g., `TestMigrateLegacyLedger_*` in cashflow_test.go).

## Requirement 2: Remove Transfer Exclusion Logic

**Key insight**: With the account-based model, transfers are just normal credits and debits to named accounts. A debit from Trading and credit to Accumulate should both be reflected in their respective account balances. The total portfolio cash (sum of all account balances) stays the same. There is NO reason to exclude transfers.

### What to change:

**`internal/models/cashflow.go`**:
- Remove `IsTransfer()` method (or keep for informational purposes, but it must not be used for exclusion)
- Fix `TotalContributions()` — remove the `if tx.Category == CashCatTransfer { continue }` skip

**`internal/services/cashflow/service.go`** — `CalculatePerformance()`:
- Remove the `if tx.Category == models.CashCatTransfer { continue }` exclusion
- Transfers are just normal flows — credits count as deposits, debits count as withdrawals

**`internal/services/portfolio/growth.go`** — `GetDailyGrowth()`:
- Remove the `if tx.Category == models.CashCatTransfer { continue }` skip
- All transactions (including transfers) affect the running cash balance

**`internal/services/portfolio/service.go`** — `populateNetFlows()`:
- Remove the `if tx.Category == models.CashCatTransfer` exclusion
- All transactions are real flows

### Test updates:
- `growth_internal_transfer_stress_test.go` — Update expectations: transfers DO affect per-account balances
- `internal_transfer_stress_test.go` — Update expectations: transfers count as normal flows
- `capital_timeline_test.go` — Update expectations: transfers affect cash balance
- `cashflow_test.go` — Update TotalContributions test expectations
- Any other test asserting transfer exclusion

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Remove legacy types + migration functions + transfer exclusion |
| `internal/models/cashflow_test.go` | Remove legacy migration tests, fix transfer assertions |
| `internal/services/cashflow/service.go` | Remove auto-migration, remove transfer exclusion |
| `internal/services/cashflow/internal_transfer_stress_test.go` | Update transfer expectations |
| `internal/services/portfolio/growth.go` | Remove transfer skip |
| `internal/services/portfolio/service.go` | Remove transfer skip in populateNetFlows |
| `internal/services/portfolio/growth_internal_transfer_stress_test.go` | Rewrite - transfers affect balances |
| `internal/services/portfolio/capital_timeline_test.go` | Update transfer assertions |

## Approach

This is a cleanup/simplification — strictly removing code and updating tests. No new features.
