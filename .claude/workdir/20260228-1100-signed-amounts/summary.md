# Summary: Signed Amounts & Account-Based External Balances

**Status:** completed

## Changes
| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Removed Direction type/field, signed Amount model, CashAccount.Type, NonTransactionalBalance() |
| `internal/models/cashflow_test.go` | Updated all tests for signed amounts |
| `internal/models/portfolio.go` | Removed ExternalBalance struct, ExternalBalancePerformance |
| `internal/models/portfolio_test.go` | Removed ExternalBalance test data |
| `internal/services/cashflow/service.go` | Direction removal, signed AddTransfer, UpdateAccount(), simplified validation |
| `internal/services/cashflow/service_test.go` | Updated for signed amounts |
| `internal/services/cashflow/service_stress_test.go` | Updated for signed amounts |
| `internal/services/cashflow/cleanup_stress_test.go` | Updated for signed amounts |
| `internal/services/cashflow/capital_perf_stress_test.go` | Updated for signed amounts |
| `internal/services/cashflow/derive_trades_stress_test.go` | Updated for signed amounts |
| `internal/services/cashflow/internal_transfer_stress_test.go` | Updated for signed amounts |
| `internal/services/cashflow/signed_amounts_stress_test.go` | NEW: 53 adversarial tests |
| `internal/services/portfolio/service.go` | ExternalBalanceTotal from NonTransactionalBalance() |
| `internal/services/portfolio/service_test.go` | Removed external balance tests |
| `internal/services/portfolio/external_balances.go` | DELETED |
| `internal/services/portfolio/external_balances_test.go` | DELETED |
| `internal/services/portfolio/external_balances_stress_test.go` | DELETED |
| `internal/services/portfolio/sync_external_balance_stress_test.go` | DELETED |
| `internal/services/portfolio/capital_timeline_test.go` | Updated for signed amounts |
| `internal/services/portfolio/capital_timeline_stress_test.go` | Updated for signed amounts |
| `internal/services/portfolio/growth_internal_transfer_stress_test.go` | Updated for signed amounts |
| `internal/services/portfolio/indicators_test.go` | Updated for signed amounts |
| `internal/services/portfolio/indicators_stress_test.go` | Updated for signed amounts |
| `internal/interfaces/services.go` | Added UpdateAccount to CashFlowService |
| `internal/server/catalog.go` | Removed external balance tools, added update_account |
| `internal/server/catalog_test.go` | Updated tool count |
| `internal/server/handlers.go` | Removed external balance handlers, added handleUpdateAccount |
| `internal/server/routes.go` | Removed external balance routes, added cash-accounts route |
| `internal/server/handlers_portfolio_test.go` | Updated mock with UpdateAccount |
| `internal/server/glossary.go` | Updated for removed ExternalBalancePerformance |
| `internal/server/glossary_test.go` | Updated test data |
| `internal/server/glossary_stress_test.go` | Updated test data |
| `internal/services/report/devils_advocate_test.go` | Removed ExternalBalance references |
| `tests/data/cashflow_test.go` | Updated for signed amounts |
| `tests/api/signed_amounts_test.go` | NEW: 11 integration tests |
| `docs/architecture/services.md` | Updated with signed amounts model |

## Tests
- Unit tests: ALL PASSING (models, cashflow, portfolio, report, server)
- Stress tests: 53 new adversarial tests + all existing updated
- Integration tests: 11 new tests created
- go vet: CLEAN
- Build: CLEAN
- Pre-existing failures only: SurrealDB connection (no local DB)

## Architecture
- Architect reviewed and approved (Task #2)
- services.md updated with signed amounts model, SoC documentation, account type semantics
- No legacy compatibility code

## Devils-Advocate
- 53 stress tests covering: boundary amounts, concurrent operations, sign manipulation, account type transitions, ledger corruption, transfer edge cases
- All passing

## Key Design Decisions
- Amount sign is the sole indicator of credit/debit (no Direction field)
- Accounts auto-created on first transaction (non-transactional by default)
- NonTransactionalBalance() replaces ExternalBalance struct
- UpdateAccount endpoint for changing account type/transactional status
- XIRR computed from trade history, not cash transactions

## Net Impact
- 34 files changed
- ~810 lines added, ~3772 lines removed (net -2962)
- 4 files deleted (external balance code)
- 2 new test files
