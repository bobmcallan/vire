# Architecture Review: Net Return D/W/M Period Changes (Task #2)

**Date**: 2026-03-04
**Reviewer**: architect
**Status**: PENDING IMPLEMENTATION FIX — 90% complete, 1 blocking issue

## Summary

The Net Return D/W/M feature implementation is architecturally sound and follows established patterns. However, there is a **critical blocking issue**: existing test files have not been updated to reflect the breaking change to the PeriodChanges struct (removal of EquityValue field).

The code is otherwise ready for architecture sign-off once test files are fixed.

---

## Compliance Checklist

### 1. PeriodChanges Struct Follows MetricChange Patterns ✅

**File**: `internal/models/portfolio.go:97-103`

```go
type PeriodChanges struct {
	PortfolioValue     MetricChange `json:"portfolio_value"`      // Total portfolio (equity + cash)
	NetEquityReturn    MetricChange `json:"net_equity_return"`    // Unrealized P&L (equity_value - net_equity_cost)
	NetEquityReturnPct MetricChange `json:"net_equity_return_pct"` // Return % on equity cost
	GrossCash          MetricChange `json:"gross_cash"`           // Cash balance
	Dividend           MetricChange `json:"dividend"`             // Cumulative dividends received
}
```

**Verification**:
- ✅ Each field is type `MetricChange` (consistent with existing pattern)
- ✅ JSON tags use snake_case (portfolio_value, net_equity_return, net_equity_return_pct)
- ✅ Field names match Portfolio struct field names (correct naming convention)
- ✅ Documentation is clear and precise
- ✅ Removed EquityValue field (no backward-compatibility shim — correct per "No Legacy Compatibility" rule)

**Pattern Adherence**: PASS

---

### 2. buildSignedMetricChange Properly Handles Negative P&L ✅

**File**: `internal/services/portfolio/service.go:1171-1185`

```go
func buildSignedMetricChange(current, previous float64, hasPrevious bool) models.MetricChange {
	mc := models.MetricChange{
		Current:     current,
		Previous:    previous,
		HasPrevious: hasPrevious,
		RawChange:   current - previous,
	}
	if hasPrevious && previous != 0 {
		mc.PctChange = ((current - previous) / math.Abs(previous)) * 100
	}
	return mc
}
```

**Verification**:
- ✅ Takes explicit `hasPrevious` flag (not derived from `previous > 0` like buildMetricChange)
- ✅ Uses `math.Abs(previous)` for PctChange denominator (handles negative P&L correctly)
- ✅ Zero-division guard: `previous != 0` check
- ✅ math module already imported (line 9)
- ✅ Complements existing buildMetricChange without duplication
- ✅ Properly positioned in file (after buildMetricChange for logical grouping)

**Calculation Correctness**:
- ✅ Positive → Positive: (100 - 50) / abs(50) * 100 = 100% ✓
- ✅ Negative → Negative: (-2000 - (-1000)) / abs(-1000) * 100 = -100% ✓
- ✅ Positive → Negative: (-500 - 1000) / abs(1000) * 100 = -150% ✓
- ✅ Zero previous: PctChange not computed (no division by zero)

**Pattern Adherence**: PASS

---

### 3. computePeriodChanges Uses Correct Data Source ✅

**File**: `internal/services/portfolio/service.go:1108-1160`

**Verification of data flow**:

```go
// Lines 1114-1120: Initialize with current portfolio values
NetEquityReturn: models.MetricChange{
	Current:     portfolio.NetEquityReturn,
	HasPrevious: false,
},
NetEquityReturnPct: models.MetricChange{
	Current:     portfolio.NetEquityReturnPct,
	HasPrevious: false,
},

// Lines 1138-1139: Read from TimelineSnapshot (correct owner)
current.NetEquityReturn = buildSignedMetricChange(portfolio.NetEquityReturn, snap.NetEquityReturn, true)
current.NetEquityReturnPct = buildSignedMetricChange(portfolio.NetEquityReturnPct, snap.NetEquityReturnPct, true)
```

**Verification**:
- ✅ TimelineSnapshot is the owning store for historical portfolio values
- ✅ Reads snap.NetEquityReturn and snap.NetEquityReturnPct (these fields exist in TimelineSnapshot per portfolio.go:323-325)
- ✅ No data transformation or business logic in consumer code
- ✅ Proper separation of concerns: PortfolioService calls TimelineStore (not reimplementing logic)
- ✅ No duplication: only one code path computes PeriodChanges
- ✅ Dividend fallback correctly delegates to cashflowSvc (per original pattern)
- ✅ No EquityValue references remain (breaking change fully applied in function)

**Data Ownership**: PASS

---

### 4. No Legacy Compatibility Shims ✅

**Verification**:
- ✅ EquityValue field completely removed (not aliased, deprecated, or marked for removal)
- ✅ No dual-format unmarshallers for old JSON
- ✅ No backward-compatible wrappers
- ✅ No "legacy" variants of buildMetricChange
- ✅ buildMetricChange preserved (not replaced, used for positive-only values like PortfolioValue)
- ✅ Clean breakage for clients to update

**Legacy Compatibility**: PASS

---

### 5. JSON Field Naming Consistency ✅

**PeriodChanges JSON fields**:
- portfolio_value ✅
- net_equity_return ✅
- net_equity_return_pct ✅
- gross_cash ✅
- dividend ✅

**Portfolio struct field equivalents**:
- PortfolioValue → portfolio_value ✓
- NetEquityReturn → net_equity_return ✓
- NetEquityReturnPct → net_equity_return_pct ✓
- GrossCashBalance → gross_cash ✓
- LedgerDividendReturn → dividend ✓

**Snake Case Consistency**: PASS

---

### 6. Separation of Concerns: Changes Computation ✅

**Verification**:
- ✅ PortfolioService owns computePeriodChanges (service-level function)
- ✅ TimelineStore owns historical snapshots (no logic in store)
- ✅ MetricChange struct is a pure data model (no logic)
- ✅ No calculation of Net Return values (those are computed elsewhere by PortfolioService)
- ✅ No cross-service logic leakage
- ✅ Clear function boundaries

**Separation of Concerns**: PASS

---

## ✅ COMPLETED WORK SUMMARY

| Item | Status | Notes |
|------|--------|-------|
| PeriodChanges struct | ✅ | Correctly updated, EquityValue removed, NetEquityReturn/NetEquityReturnPct added |
| buildSignedMetricChange | ✅ | Correct implementation, handles negative P&L, zero-division guard |
| computePeriodChanges | ✅ | Uses new fields, correct data source (TimelineSnapshot), no EquityValue refs |
| Unit tests | ✅ | 7 tests created (period_changes_test.go) — not yet executed |
| Build (go build) | ✅ | Succeeds at main package level |
| Vet (go vet) | ❓ | Not yet run (blocked by test file compilation errors) |

---

## ❌ BLOCKING ISSUE: Test Files Not Updated

**Severity**: CRITICAL — prevents verification and blocks downstream tasks

### Files Affected:
1. **internal/services/portfolio/service_test.go**
   - Lines reference: `portfolio.Changes.Yesterday.EquityValue`
   - Count: 11+ occurrences (Yesterday, Week, Month variants)

2. **internal/services/portfolio/changes_stress_test.go**
   - Lines reference: `pc.EquityValue`
   - Count: 10+ occurrences

### Root Cause:
These test files were not updated when the PeriodChanges struct was changed. They still reference the now-deleted `EquityValue` field.

### Impact:
- `go test ./internal/services/portfolio/...` fails to compile
- Prevents verification of buildSignedMetricChange unit tests
- Blocks Task #3 (stress testing)
- Blocks Task #7 (integration tests)

### Required Fix:
Update both test files to use `NetEquityReturn` (or appropriate field) instead of `EquityValue`. This is part of completing Task #1.

---

## Unit Test Status

**File**: `internal/services/portfolio/period_changes_test.go`

All 7 required tests created:
1. TestBuildSignedMetricChange_PositiveValues — not yet executed
2. TestBuildSignedMetricChange_NegativeValues — not yet executed
3. TestBuildSignedMetricChange_CrossZero — not yet executed
4. TestBuildSignedMetricChange_ZeroPrevious — not yet executed
5. TestBuildSignedMetricChange_NoPrevious — not yet executed
6. TestComputePeriodChanges_HasNetEquityReturn — not yet executed
7. TestComputePeriodChanges_NoSnapshot — not yet executed

**Status**: Tests exist but cannot execute until blocking issue is fixed.

---

## Architecture Recommendation

**CONDITIONAL APPROVAL**:

The implementation is **architecturally sound and follows all established patterns**:
- ✅ Correct data ownership and separation of concerns
- ✅ No legacy compatibility cruft
- ✅ Proper use of helper functions
- ✅ Clean JSON naming
- ✅ Follows MetricChange pattern

**APPROVAL IS CONDITIONAL** on fixing the test file references. Once the blocking issue is resolved, this feature is ready for production.

---

## Next Steps

1. **Implementer**: Fix service_test.go and changes_stress_test.go to use NetEquityReturn instead of EquityValue
2. **Implementer**: Run `go test ./internal/services/portfolio/...` to verify all tests pass
3. **Architect**: Complete final architecture sign-off once tests pass
4. **Test Creator**: Proceed with Task #7 (integration tests) integration test creation

---

**Reviewed by**: architect
**Timestamp**: 2026-03-04
**Reference**: `.claude/workdir/20260304-1630-net-return-dwm/requirements.md`
