# Summary: Cash Flow Summary Redesign

**Status:** completed
**Feedback:** fb_4e848a89

## Changes
| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Replaced `CashFlowSummary` (TotalCredits/TotalDebits/NetCashFlow → TotalCash/TransactionCount/ByCategory). Redesigned `Summary()` method. |
| `internal/server/handlers.go` | Added `cashAccountWithBalance` response struct. Updated `newCashFlowResponse()` to compute per-account balances. |
| `internal/server/catalog.go` | Updated `list_cash_transactions` description to reflect new summary fields. |
| `internal/models/cashflow_test.go` | Replaced `TestCashFlowLedger_Summary` with 5 new subtests covering redesigned fields. |
| `internal/models/cashflow_stress_test.go` | Updated 16 stress tests from old field names to new (TotalCash, ByCategory). |
| `tests/api/cashflow_test.go` | Updated `TestCashFlowResponseSummary` for new fields. Fixed pre-existing tests using old `type`/`direction` fields. |
| `tests/api/cashflow_cleanup_test.go` | Fixed pre-existing tests using old `direction` field — updated to use signed amounts and server-computed balances. |
| `docs/features/20260228-cash-transaction-response-totals.md` | Updated to reflect redesigned summary structure. |

## Tests
- Unit tests: 5 `TestCashFlowLedger_Summary` subtests + 16 stress tests — all PASS
- Integration tests: `TestCashFlowResponseSummary` (7 subtests) — PASS
- All cashflow integration tests fixed and passing (CRUD, validation, transfers, cleanup)
- Fix rounds: 1 (fixed pre-existing tests using old type/direction fields)

## Architecture
- Reviewed and approved by architect
- No separation of concerns issues
- `Balance` kept as response-only field (not stored) via `cashAccountWithBalance` struct

## Devils-Advocate
- Identified `omitempty` issue on float64 Balance field — resolved with response-only struct
- No security issues found

## Notes
- Pre-existing tests in `cashflow_test.go` and `cashflow_cleanup_test.go` were using old `type`/`direction` fields from before the signed amounts refactor (7ee0acf). Fixed as part of this work.
- `TestCashFlowPerformanceEmpty` was incorrectly expecting zero performance on empty cash ledger — performance auto-derives from trade history.
