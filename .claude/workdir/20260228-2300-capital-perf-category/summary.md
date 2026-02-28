# Summary: Fix TotalDeposited/TotalWithdrawn Category Filtering

**Status:** completed
**Feedback:** fb_7ffa974f

## Changes
| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Fix TotalDeposited and TotalWithdrawn — filter by `category=contribution` only |
| `internal/services/cashflow/service_test.go` | Updated assertion — TransferIn renamed, TotalDeposited 130000→100000 |
| `internal/services/cashflow/signed_amounts_stress_test.go` | Updated 5 assertion blocks — transfers/fees no longer count |
| `internal/services/cashflow/capital_perf_stress_test.go` | DividendInflowCounted→DividendNotCountedAsDeposit, CreditDebitTypes fixed |
| `internal/services/cashflow/cleanup_stress_test.go` | Updated SMSF scenario comments |
| `internal/services/cashflow/internal_transfer_stress_test.go` | All transfer/fee/other assertions fixed, test names updated |
| `internal/services/cashflow/capital_perf_category_stress_test.go` | NEW: 20 adversarial stress tests |
| `tests/api/capital_perf_category_test.go` | NEW: 6 integration tests |
| `tests/api/signed_amounts_test.go` | Test renamed, uses contribution-category debits |

## Tests
- Unit tests: all passing in internal/services/cashflow/
- Stress tests: 20/20 new adversarial tests PASSING
- Integration tests: 6 new tests created
- Pre-existing failures only: SurrealDB, internal/app, internal/server (unrelated)
- go vet: CLEAN
- Build: CLEAN

## Architecture
- Architect reviewed and approved (Task #2)
- NetDeployedImpact() already correct (filters by category)
- Capital timeline (growth.go) correct (uses NetDeployedImpact)
- No separation of concerns issues found

## Devils-Advocate
- 20 stress tests covering: category isolation, mixed categories, zero amounts, large amounts, precision, sign correctness, NetDeployedImpact consistency
- No bugs found in implementation

## Notes
- 2-line code fix + ~20 test assertion updates across 6 files
- Tests that were already correct: cashflow_test.go (no method calls), set_transactions_stress_test.go (already correct), tests/data/cashflow_test.go (serialization only)
- Implementer (Sonnet) completed all phases efficiently
