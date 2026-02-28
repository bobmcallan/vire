# Documentation Validation Report - Task #8

**Status**: IN PROGRESS - Awaiting implementer to fix 3 documentation issues

**Date**: 2026-02-28 10:43

## Summary

Architecture documentation (services.md) has been correctly updated and all claims verified against implementation. However, three documentation gaps were found that need to be fixed:

## ✅ Accurate Documentation

### docs/architecture/services.md
**Status**: PERFECT - All claims verified against implementation

Verified claims:
- ✅ CashBalance = "running credits minus debits across all transaction types"
- ✅ NetDeployed = "contributions credited, plus other/fee/transfer debits subtracted"
- ✅ Dividends excluded from net flows (only dividends, NOT transfers)
- ✅ All transactions treated as real cash flows (including transfers)
- ✅ CalculatePerformance: "Sums all credits as TotalDeposited and all debits as TotalWithdrawn"
- ✅ Capital Timeline processes all transactions including transfers
- ✅ Trade-based fallback described correctly
- ✅ ExternalBalance tracking documented accurately

## ⚠️ Documentation Issues Found

### Issue #1: docs/architecture/api.md (Lines 37-39)

**Problem**: Missing transfer endpoint documentation

**Current**:
```
| `/api/portfolios/{name}/cash-transactions` | GET/POST | Cash transactions |
| `/api/portfolios/{name}/cash-transactions/{id}` | PUT/DELETE | Transaction CRUD |
| `/api/portfolios/{name}/cash-transactions/performance` | GET | Capital performance (XIRR) |
```

**Required Addition**:
```
| `/api/portfolios/{name}/cash-transactions/transfer` | POST | Create paired transfer entries |
```

**Evidence**: 
- Implementation: `internal/server/handlers.go:1781` (handleCashFlowTransfer)
- Tests: `tests/api/cashflow_cleanup_test.go` (9 integration tests)
- Routes: `internal/server/routes.go:251-252`

---

### Issue #2: README.md (Lines 228-231)

**Problem**: Outdated endpoint paths (uses `cashflows` instead of `cash-transactions`)

**Current (WRONG)**:
```
| `/api/portfolios/{name}/cashflows` | GET | List cash flow transactions...
| `/api/portfolios/{name}/cashflows` | POST | Add cash flow transaction...
| `/api/portfolios/{name}/cashflows/{id}` | PUT | Update cash flow transaction...
| `/api/portfolios/{name}/cashflows/{id}` | DELETE | Remove cash flow transaction...
```

**Correct**:
```
| `/api/portfolios/{name}/cash-transactions` | GET | List cash flow transactions...
| `/api/portfolios/{name}/cash-transactions` | POST | Add cash flow transaction...
| `/api/portfolios/{name}/cash-transactions/{id}` | PUT | Update cash flow transaction...
| `/api/portfolios/{name}/cash-transactions/{id}` | DELETE | Remove cash flow transaction...
```

**Evidence**: Endpoint routing uses `cash-transactions` (routes.go:241)

---

### Issue #3: README.md (Line 74)

**Problem**: Transaction type documentation reflects old type-based model, not new direction-based model

**Current (WRONG)**:
```
| `add_cash_transaction` | Add a cash flow transaction (deposit, withdrawal, contribution, transfer_in, transfer_out, dividend)
```

**Correct**:
```
| `add_cash_transaction` | Add a cash flow transaction with direction (credit/debit) and category (contribution, dividend, transfer, fee, other)
```

**Evidence**: 
- Implementation uses `Direction` (credit/debit) not `Type` (deposit/withdrawal/etc.)
- Categories: contribution, dividend, transfer, fee, other (no transfer_in/transfer_out)
- models/cashflow.go:8-49

---

## Recommended Action

Task #7 (Build verification and docs update) must update three locations before being marked complete:

1. ✅ services.md - Already complete and accurate
2. ⚠️ api.md - Add transfer endpoint row to table
3. ⚠️ README.md - Fix two entries (endpoint paths + transaction types)

All three are high-visibility documentation that users will read first.

## Completion Criteria

Task #8 will be marked complete when:
- api.md includes transfer endpoint documentation ✓
- README.md endpoints corrected to use `cash-transactions` ✓
- README.md transaction types reflect new direction/category model ✓
- All other documentation accurate and matches implementation ✓

---

**Reviewer**: reviewer
**Task**: #8 Validate docs match implementation
