# Quick Reference — Timeline Corruption Fix Architecture Review

**Reviewer**: architect (task #7)
**Status**: READY TO EXECUTE
**Blocks**: Tasks #3 (stress test), #6 (execute all tests)

---

## What Needs Fixing

**Bug Location**: `internal/services/market/service.go`
- `CollectMarketData()` — lines 96-124 (currently `else` block does blind replacement)
- `collectCoreTicker()` — lines 341-365 (currently `else` block does blind replacement)

**Root Cause**: When `force=true`, both functions replace EOD data entirely instead of merging with existing history.

---

## The Fix (Three-Path Pattern)

### Path 1: Incremental Fetch (no force, has existing)
```
if !force && existing != nil && len(existing.EOD) > 0 {
    // Fetch only new bars after latest date
    // Merge with existing
}
```

### Path 2: Force Refresh with Merge (NEW) ← CRITICAL
```
else if force && existing != nil && len(existing.EOD) > 0 {
    // Full 3-year fetch (but merge instead of replace)
    // eodResp, err := s.eodhd.GetEOD(ctx, ticker, WithDateRange(-3yr, now))
    // if err { log warning } else if len(eodResp.Data) > 0 {
    //     marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
    //     eodChanged = true
    // }
}
```

### Path 3: Full Fetch (no existing) ← SAFE TO REPLACE
```
else {
    // No existing: full fetch + blind replacement OK
    // marketData.EOD = eodResp.Data
}
```

---

## Critical Guards

1. **Empty Response Protection**: `else if len(eodResp.Data) > 0`
   - Prevents overwriting good data with empty EODHD responses
   - Must guard Path 1 & 2 assignments

2. **Signal Flag Safety**: Only set `eodChanged = true` inside the data guard
   - Prevents spurious signal recomputation on API failures

---

## Architecture Review Checklist

### 1. Data Ownership ✅
- [ ] `mergeEODBars()` is single source of truth
- [ ] Called exactly 4 times (1× incremental core, 1× force+merge core, 1× incremental full, 1× force+merge full)
- [ ] No duplication of merge logic across paths

### 2. Three-Path Pattern ✅
- [ ] Both functions have identical logic structure
- [ ] Path 1 condition: `!force && existing && len(EOD) > 0`
- [ ] Path 2 condition: `force && existing && len(EOD) > 0` (NEW)
- [ ] Path 3: `else` (safe: no existing to corrupt)

### 3. Data Preservation ✅
- [ ] Path 1: `else if len(eodResp.Data) > 0` guard
- [ ] Path 2: `else if len(eodResp.Data) > 0` guard (CRITICAL)
- [ ] Path 3: No guard needed (acceptable: new data)
- [ ] Errors don't corrupt existing data

### 4. Signal Safety ✅
- [ ] `eodChanged = true` only inside `else if len(eodResp.Data) > 0`
- [ ] Not set when API fails or returns empty

### 5. No Architectural Violations ✅
- [ ] Uses existing `mergeEODBars()` (no new abstractions)
- [ ] No new dependencies
- [ ] No portfolio service changes
- [ ] No GetDailyGrowth() changes
- [ ] No backward-compatibility shims

### 6. Test Coverage ✅
- [ ] TestCollectCoreMarketData_ForceRefreshMergesExistingEOD
- [ ] TestCollectCoreMarketData_ForceRefreshEmptyResponsePreservesExisting
- [ ] TestCollectMarketData_ForceRefreshMergesExistingEOD
- [ ] TestCollectMarketData_ForceRefreshEmptyResponsePreservesExisting
- [ ] TestForceRefresh_PreservesTimelineIntegrity (integration)
- [ ] TestForceRefresh_EODBarCount (integration)

### 7. Documentation ✅
- [ ] `docs/architecture/services.md` — likely no changes needed
- [ ] Log messages distinguish between paths (mention "force" in Path 2 message)

---

## Expected Approval Criteria

✅ ALL of the above must pass for "APPROVED"

---

## Files to Check

**Implementation**:
- `internal/services/market/service.go:96-124` (CollectMarketData)
- `internal/services/market/service.go:341-365` (collectCoreTicker)

**Tests** (already written):
- `internal/services/market/service_test.go` (+4 unit tests)
- `tests/api/timeline_corruption_test.go` (+2 integration tests)

**Docs** (check for accuracy):
- `docs/architecture/services.md` (MarketService section)

---

## Review Decision

Once all 7 checklist items are confirmed ✅, report:
- **Status**: APPROVED
- **No issues**: Data ownership clear, patterns consistent, no violations
- **Mark task #7 COMPLETE**
