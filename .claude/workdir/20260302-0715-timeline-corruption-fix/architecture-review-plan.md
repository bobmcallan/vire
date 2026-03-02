# Architecture Review Plan — Timeline Corruption Fix

**Task**: #7 (Architect) — Review implementation from task #1 (Implementer)
**Feedback Items**: fb_b4387bb2, fb_5b41213e (HIGH severity)
**Architecture Docs**: docs/architecture/services.md (MarketService section)

---

## Pre-Review: Bug Root Cause Analysis ✅

Both `CollectMarketData()` and `collectCoreTicker()` in `service.go` have identical bugs:

**Current Code Pattern:**
```go
if !force && existing != nil && len(existing.EOD) > 0 {
    // Incremental fetch + merge
} else {
    // Full fetch (blind replacement) — BUG when force=true with existing data
    marketData.EOD = eodResp.Data
}
```

**Problem:**
1. **History Truncation**: When `force=true && existing != nil && len(existing.EOD) > 500+`, EODHD's 3-year limit cuts off older bars
2. **Data Corruption**: When `force=true && eodResp.Data` is empty/partial (API error), the `else` block replaces good data with bad
3. **Downstream Impact**: `GetDailyGrowth()` filters holdings by EOD availability. Truncated EOD → timeline gaps

**Locations:**
- `CollectMarketData()`: lines 96-124 (the `else` block at lines 114-124)
- `collectCoreTicker()`: lines 341-365 (the `else` block at lines 356-365)

---

## Fix Requirements (from requirements.md)

The implementation must create THREE conditional paths:

### Path 1: Incremental Fetch (no force, has existing)
```go
if !force && existing != nil && len(existing.EOD) > 0 {
    // Fetch only new bars after latest stored date
    // Merge with existing
    // Guard: else if len(eodResp.Data) > 0
}
```

### Path 2: Force Refresh with Merge (force + has existing) ← NEW
```go
else if force && existing != nil && len(existing.EOD) > 0 {
    // Full 3-year fetch (because force means re-fetch, not incremental)
    // Merge with existing to preserve older history
    // Guard: else if len(eodResp.Data) > 0 (preserve if API fails)
}
```

### Path 3: Full Fetch for New Data (no existing)
```go
else {
    // No existing data: do blind replacement (new ticker or first collection)
    marketData.EOD = eodResp.Data
}
```

---

## Architecture Review Checklist

### 1. Data Ownership & Separation of Concerns ✅/❌

**Requirement**: `mergeEODBars()` is the single source of truth for combining old and new bars.

**Review Steps**:
1. Find all calls to `mergeEODBars()` (should be in both CollectMarketData and collectCoreTicker)
2. Verify NO other code reimplements merge logic
3. Verify NO raw `marketData.EOD = append(...)` outside the merge function
4. Check that signal computation only uses merged data

**Expected Findings**:
- `mergeEODBars()` implementation unchanged (internal detail)
- Called exactly 4 times: 2× in CollectMarketData (incremental + force+merge), 2× in collectCoreTicker
- Signal recomputation at `eodChanged` flag

---

### 2. Three-Path Pattern Implementation ✅/❌

**Requirement**: Both functions implement identical three-path logic.

**Review Steps**:

#### CollectMarketData() — lines 96-124
- [ ] Line 97: `if !force && existing != nil && len(existing.EOD) > 0` — Path 1 (incremental)
  - [ ] Fetches only new bars: `interfaces.WithDateRange(fromDate, now)` where `fromDate = latestDate + 1 day`
  - [ ] Merges: `marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)`
  - [ ] Guard: `else if len(eodResp.Data) > 0` (preserve existing if empty)

- [ ] NEW: `else if force && existing != nil && len(existing.EOD) > 0` — Path 2 (force+merge)
  - [ ] Fetches full 3 years: `interfaces.WithDateRange(now.AddDate(-3, 0, 0), now)`
  - [ ] Merges: `marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)`
  - [ ] Guard: `else if len(eodResp.Data) > 0` (preserve existing if empty or error)
  - [ ] Logging: mentions "force" in message to distinguish from Path 1

- [ ] `else` — Path 3 (no existing)
  - [ ] Full fetch: `interfaces.WithDateRange(now.AddDate(-3, 0, 0), now)`
  - [ ] Blind replacement: `marketData.EOD = eodResp.Data` (safe: no existing to corrupt)
  - [ ] Returns error on fetch failure: `continue` (line 119 pattern preserved)

#### collectCoreTicker() — lines 341-365
- [ ] Same three-path pattern in the `else if s.eodhd != nil` block
- [ ] Line 343: Path 1 condition and logic
- [ ] NEW condition for Path 2 (force+merge)
- [ ] Line 356: Path 3 (else) — full fetch with blind replacement (safe)

---

### 3. Empty Response Handling ✅/❌

**Requirement**: Empty EODHD responses must never overwrite good data.

**Review Steps**:
1. Verify ALL eodResp assignments are guarded with `else if len(eodResp.Data) > 0`
2. Verify Path 3 (no existing) does NOT have this guard (acceptable: no data to corrupt)
3. Verify logs differentiate between empty response and fetch error

**Critical Locations**:
- CollectMarketData() Path 2: `else if len(eodResp.Data) > 0`
- collectCoreTicker() Path 2: `else if len(eodResp.Data) > 0`

**Expected Error Behavior**:
- Fetch error in Path 1/2 → log warning, preserve existing (no assignment)
- Empty response in Path 1/2 → log warning, preserve existing (guarded assignment)
- Fetch error in Path 3 → log error, return/continue (normal error handling)

---

### 4. EODChanged Signal Flag ✅/❌

**Requirement**: `eodChanged` must only be set when merge actually produces different data.

**Review Steps**:
1. Find all assignments to `eodChanged = true`
2. Verify ALL are inside `else if len(eodResp.Data) > 0` guards (only when data is fetched)
3. Verify Path 2 (force+merge) sets `eodChanged = true` only when merge succeeds
4. Verify Path 3 always sets `eodChanged = true` (safe: new data)

**Expected Pattern**:
```go
} else if len(eodResp.Data) > 0 {
    marketData.EOD = mergeEODBars(...)
    eodChanged = true  // ← Only here
}
```

**Impact**: Downstream signal recomputation (line 221-226) won't trigger spuriously on API failures.

---

### 5. No Architectural Violations ✅/❌

**Checklist**:
- [ ] No new dependencies introduced (uses existing mergeEODBars, interfaces, logger)
- [ ] No changes to portfolio service (fix is at data layer only)
- [ ] No changes to GetDailyGrowth() or timeline logic (not in scope)
- [ ] No backward-compatibility shims or deprecated types added
- [ ] Logger messages are consistent with codebase style (Str, Bool, Err fields)

---

### 6. Test Coverage ✅/❌

**Requirement**: 4 unit tests (2 per function) + integration tests

#### Unit Tests (internal/services/market/service_test.go)

**Test 1: CollectCoreMarketData_ForceRefreshMergesExistingEOD**
- [ ] Setup: existing MarketData with 500 EOD bars
- [ ] Mock: EODHD returns 200 bars (3-year window)
- [ ] Call: `CollectCoreMarketData(ctx, []string{"BHP.AU"}, true)` with force=true
- [ ] Assert: result EOD > 200 bars (preserving older history)
- [ ] Assert: stored data uses merge (dates from both old and new)

**Test 2: CollectCoreMarketData_ForceRefreshEmptyResponsePreservesExisting**
- [ ] Setup: existing MarketData with 100 EOD bars
- [ ] Mock: EODHD returns empty response (0 bars)
- [ ] Call: `CollectCoreMarketData(ctx, [...], true)` with force=true
- [ ] Assert: result EOD == 100 bars (not overwritten)
- [ ] Assert: `eodChanged = false` (no signal recomputation)

**Test 3: CollectMarketData_ForceRefreshMergesExistingEOD** (CollectMarketData path)
- [ ] Similar to Test 1 but via `CollectMarketData()` instead
- [ ] Verifies non-core path has same merge behavior

**Test 4: CollectMarketData_ForceRefreshEmptyResponsePreservesExisting** (CollectMarketData path)
- [ ] Similar to Test 2 but via `CollectMarketData()` instead

#### Integration Tests (tests/api/timeline_corruption_test.go)

**Test 1: ForceRefresh_PreservesTimelineIntegrity**
- [ ] Get portfolio timeline point count before
- [ ] Force refresh stock data for a holding
- [ ] Get timeline after
- [ ] Assert: point count did not decrease

**Test 2: ForceRefresh_EODBarCount**
- [ ] Get stock data (note candle count)
- [ ] Force refresh same stock
- [ ] Get stock data again
- [ ] Assert: candle count preserved/increased (not decreased)

---

### 7. Documentation Updates ✅/❌

**Scope**: Likely NO changes needed (internal implementation detail, not public API)

**Check**:
- [ ] `docs/architecture/services.md` — MarketService section
  - [ ] No changes needed (mergeEODBars already documented pattern)
  - [ ] OR if unclear, add note: "Force refresh merges with existing to preserve history"

- [ ] `internal/services/market/service.go` comments
  - [ ] Line 56 (CollectMarketData): Update comment if needed
  - [ ] Line 232 (CollectCoreMarketData): Update comment if needed
  - [ ] mergeEODBars() function: Verify documentation is clear

- [ ] Changelog / release notes: NOT in scope (will be summarized in PR)

---

## Review Output

### Approval Criteria (ALL must pass)

1. ✅ **Data Ownership**: mergeEODBars is single source of truth, no duplication
2. ✅ **Three-Path Pattern**: Both functions implement identical, clear logic
3. ✅ **Data Preservation**: Empty responses never overwrite good data
4. ✅ **Signal Safety**: eodChanged only set when merge produces actual data
5. ✅ **No Violations**: No new dependencies, no portfolio changes, no backward-compat hacks
6. ✅ **Tests**: 4 unit tests + 2 integration tests cover scenarios + error handling
7. ✅ **Docs**: Any necessary updates made (or justification why none needed)

### Findings Template

**Status**: APPROVED / REJECTED / NEEDS REVISION

**Strengths**:
- (List pattern-compliant aspects)

**Issues** (if any):
- Issue 1: (description) → Recommendation: (fix)
- Issue 2: ...

**Questions** (if any):
- Question 1: (clarify design choice)

---

## Related Code References

- `mergeEODBars()` implementation: `service.go:419-453`
- `CollectMarketData()`: `service.go:58-230`
- `CollectCoreMarketData()`: `service.go:235-304`
- `collectCoreTicker()`: `service.go:307-417`
- Signal computation: `service.go:220-226` (eodChanged trigger)
- GetDailyGrowth() (downstream): `internal/services/portfolio/growth.go:206`
