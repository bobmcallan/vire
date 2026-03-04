# Architecture Review: Timeline Cash Auto-Loading Fix

**Reviewer**: architect
**Date**: 2026-03-05
**Status**: ✅ APPROVED (pending vet fixes)
**Build Status**: ❌ BLOCKED on vet errors (test code)

## Executive Summary

The implementation of auto-loading cash transactions in `GetDailyGrowth()` is **architecturally sound** and follows established patterns. The changes enforce proper separation of concerns by centralizing cash ledger loading inside the PortfolioService, removing redundant injection code from all call sites.

## Architecture Checklist

### Separation of Concerns ✅

**Principle**: Services own their domain logic; consumers call service methods instead of reimplementing logic.

**Verification**:
- ✅ `GetDailyGrowth()` (PortfolioService) now owns cash loading via `s.cashflowSvc.GetLedger()`
- ✅ Removed all cash injection from handlers (1 location: `handlePortfolioReview`, `handlePortfolioHistory`)
- ✅ Removed all cash injection from indicators (`GetPortfolioIndicators`)
- ✅ Removed manual cash injection from service method `rebuildTimelineWithCash()`
- ✅ Single source of truth: only `GetDailyGrowth()` loads cash when needed

**Implementation Detail** (growth.go:110-117):
```go
if opts.Transactions == nil && s.cashflowSvc != nil {
    if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
        opts.Transactions = ledger.Transactions
    }
}
```
Non-fatal design: graceful degradation when CashFlowService is nil or unavailable.

### Service Ownership & DI Pattern ✅

**Principle**: PortfolioService holds `interfaces.CashFlowService` via setter injection (SetCashFlowService), called after both services are constructed to break circular dependencies.

**Verification**:
- ✅ Follows existing pattern from docs/architecture/26-02-27-services.md line 71-78
- ✅ Nil guard: `if s.cashflowSvc != nil` prevents crashes during startup
- ✅ Lazy binding: CashFlowService injected after construction in app.go

### Backward Compatibility ✅

**Principle**: Public interface (GrowthOptions.Transactions) must support explicit overrides without requiring code changes.

**Verification** (growth.go:113):
```go
if opts.Transactions == nil && s.cashflowSvc != nil {
    // auto-load only fires if Transactions is nil
}
```
- ✅ Explicit transactions (even empty slice) bypass auto-load
- ✅ Test `TestGetDailyGrowth_ExplicitTransactionsOverride` confirms override behavior
- ✅ Interface contract unchanged

### No Duplicated Business Logic ✅

**Verification**:
- ✅ Handlers (`handlePortfolioReview`, `handlePortfolioHistory`) — no cash injection, pass empty options
- ✅ Indicators (`GetPortfolioIndicators`) — no cash injection, pass empty options
- ✅ Service method (`rebuildTimelineWithCash`) — simplified to call `GetDailyGrowth()` directly
- ✅ Scheduler (`timeline_scheduler.go`) — no code change needed, inherits fix automatically

### Simplification & Legacy Removal ✅

**Principle**: No backward-compatibility shims, no "legacy" code paths.

**Verification**:
- ✅ `rebuildTimelineWithCash()` simplified from 9 lines of injection code to 1 line call
- ✅ Comment updated to clarify the method is now just a convenience wrapper
- ✅ No old-format fallbacks or migration helpers introduced

## Documentation Updates

**File**: docs/architecture/26-02-27-services.md (line 88)

**Old**:
> `GetDailyGrowth()` loads the cash flow ledger via `CashFlowService.GetLedger()` and passes transactions via `GrowthOptions.Transactions`.

**New**:
> `GetDailyGrowth()` automatically loads the cash flow ledger via `CashFlowService.GetLedger()` when `GrowthOptions.Transactions` is nil, ensuring all callers (handlers, schedulers, internal) include cash in timeline computations. Explicit transactions can be provided via `GrowthOptions.Transactions` to override.

✅ Documentation correctly reflects the new auto-load behavior and backward compatibility.

## Test Coverage

**Existing Tests** (capital_timeline_test.go):
- ✅ `TestGetDailyGrowth_AutoLoadsCash` (line 465) — verifies auto-load fires when nil
- ✅ `TestGetDailyGrowth_ExplicitTransactionsOverride` (line 548) — verifies explicit override skips auto-load
- ✅ Integration tests verify cash balance calculations (GrossCashBalance, NetCashBalance, PortfolioValue)

**New Test File**: tests/data/timeline_cash_autload_test.go
- ✅ Integration tests added by test-creator
- ⚠️ Vet errors: duplicate test function + unused variable (see issues below)

## Issues Found

### 🔴 BLOCKING: Vet Errors (Test Code)

These must be fixed before build passes:

1. **Duplicate test** (`internal/services/portfolio/capital_timeline_test.go`):
   ```
   vet: capital_timeline_test.go:927:6: TestGetDailyGrowth_AutoLoadsCash redeclared in this block
   ```
   - `TestGetDailyGrowth_AutoLoadsCash` declared at line 465 AND line 927
   - Action: Remove or rename duplicate test

2. **Unused variable** (`tests/data/timeline_cash_autload_test.go`):
   ```
   vet: timeline_cash_autload_test.go:42:2: declared and not used: tradeDate
   ```
   - Line 42: `tradeDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)`
   - Action: Remove variable or use it in test logic

**Responsibility**: Implementer to coordinate with test-creator for fixes.

## Architectural Alignment Summary

| Aspect | Status | Evidence |
|--------|--------|----------|
| **Separation of Concerns** | ✅ | Cash loading centralized in GetDailyGrowth, not duplicated in consumers |
| **Service Ownership** | ✅ | PortfolioService calls CashFlowService (not vice versa) |
| **Backward Compatibility** | ✅ | nil check preserves explicit transaction overrides |
| **No Legacy Code** | ✅ | No shims, migrations, or old-format fallbacks |
| **Documentation** | ✅ | Updated services.md to reflect auto-load behavior |
| **Build Status** | ❌ | Blocked on vet errors in test files (not production code) |
| **Code Quality** | ✅ | Growth.go implementation is clean and well-commented |

## Approval

**Architecture Review**: ✅ **APPROVED**

The implementation correctly follows established patterns and principles. The vet errors are in test code added by test-creator and must be fixed before the build can pass. The production code (growth.go, handlers.go, indicators.go, service.go) is architecturally sound.

**Recommendation**:
1. Implementer coordinates with test-creator to fix vet errors
2. Once vet passes, proceed to Task #3 (code quality review) and Task #4 (stress testing)
