# Architecture Review Status — Timeline Corruption Fix

**Status**: AWAITING SERVICE.GO IMPLEMENTATION
**Timestamp**: 2026-03-02 08:00 UTC
**Reviewer**: architect

---

## Summary

Tests have been written (✅ COMPLETE):
- 4 unit tests in `internal/services/market/service_test.go`
- 2 integration tests in `tests/api/timeline_corruption_test.go`

But the actual service.go fix is still pending (⏳ AWAITING):
- `CollectMarketData()` — lines 96-124 need three-path pattern
- `collectCoreTicker()` — lines 341-365 need three-path pattern

---

## Test Quality Review (Pre-Implementation)

### Unit Tests

**TestCollectCoreMarketData_ForceRefreshMergesExistingEOD** ✅
- Setup: 500 existing bars (simulates >3 year history)
- Mock: EODHD returns 200 fresh bars (3-year window)
- Call: `CollectCoreMarketData(..., true)` with force=true
- Assert: Result EOD > 200 (preserving older history)
- Quality: ✅ Correct pattern, proper mock setup, clear assertion message

**TestCollectCoreMarketData_ForceRefreshEmptyResponsePreservesExisting** ✅
- Setup: 100 existing bars
- Mock: EODHD returns empty response (0 bars)
- Call: `CollectCoreMarketData(..., true)` with force=true
- Assert: Result EOD == 100 (preserved, not overwritten)
- Quality: ✅ Tests critical error handling path, proper assertion

**TestCollectMarketData_ForceRefreshMergesExistingEOD** ✅
- Setup: 500 existing bars
- Mock: EODHD returns 200 fresh bars
- Call: `CollectMarketData(..., includeNews=false, force=true)`
- Assert: Result EOD > 200
- Quality: ✅ Tests non-core path (full CollectMarketData), consistent with core test

**TestCollectMarketData_ForceRefreshEmptyResponsePreservesExisting** ✅
- Setup: 100 existing bars
- Mock: EODHD returns empty
- Call: `CollectMarketData(..., includeNews=false, force=true)`
- Assert: Result EOD == 100
- Quality: ✅ Tests error handling in non-core path

### Integration Tests

**TestForceRefresh_PreservesTimelineIntegrity** ✅
- Get portfolio timeline before
- Force refresh a holding
- Get timeline after
- Assert: data_points count preserved (doesn't decrease)
- Quality: ✅ Real E2E test, directly validates bug fix impact, comprehensive logging

**TestForceRefresh_EODBarCount** ✅
- Get stock data (candles count)
- Force refresh same stock
- Get stock data again
- Assert: candle count preserved (doesn't decrease)
- Quality: ✅ Tests direct API surface, validates candles array preservation

---

## Implementation Checklist (Pending)

### CollectMarketData() Fix (lines 96-124)

**Current (BUGGY)**:
```go
if !force && existing != nil && len(existing.EOD) > 0 {
    // Path 1: incremental
} else {
    // Path 2 & 3 combined: full fetch (BLIND REPLACEMENT BUG)
    marketData.EOD = eodResp.Data
}
```

**Required (THREE PATHS)**:
```go
if !force && existing != nil && len(existing.EOD) > 0 {
    // Path 1: incremental
} else if force && existing != nil && len(existing.EOD) > 0 {
    // Path 2: force + merge (NEW)
    eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
    if err != nil {
        s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data (force)")
    } else if len(eodResp.Data) > 0 {
        marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
        eodChanged = true
    }
    marketData.EODUpdatedAt = now
} else {
    // Path 3: no existing (safe to replace)
    eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
    // ... error handling, blind replacement OK
}
```

### collectCoreTicker() Fix (lines 341-365)

**Same pattern as above but in the `else if s.eodhd != nil` block**

---

## Architecture Validation Points (for review once implemented)

### ✅ Data Ownership
- `mergeEODBars()` is single source of truth for combining old/new bars
- Called consistently from both functions
- No duplication of merge logic

### ✅ Three-Path Pattern
- Path 1 (incremental): `!force && existing && len(EOD) > 0`
- Path 2 (force+merge): `force && existing && len(EOD) > 0` ← NEW
- Path 3 (full fetch): `else` (safe: no existing to corrupt)

### ✅ Empty Response Handling
- All Path 1 & 2 assignments guarded: `else if len(eodResp.Data) > 0`
- Path 3 can do blind replacement (acceptable: new data)
- Errors don't corrupt existing data

### ✅ Signal Flag Safety
- `eodChanged = true` only set when data actually changes
- Inside `else if len(eodResp.Data) > 0` guards
- Signal recomputation won't trigger on API failures

### ✅ No Architectural Violations
- Uses existing `mergeEODBars()` (no new abstractions)
- No new dependencies
- No changes to portfolio service or GetDailyGrowth()
- No backward-compatibility shims

---

## Awaiting Implementation

Once service.go is fixed, will verify:
1. Both functions have identical three-path logic
2. Empty response guards are in place
3. Signal flag only set on actual data change
4. Logging messages distinguish between paths
5. No duplication of logic across paths
6. Architecture docs are accurate (likely no changes needed)

Then will mark task #7 COMPLETE.
