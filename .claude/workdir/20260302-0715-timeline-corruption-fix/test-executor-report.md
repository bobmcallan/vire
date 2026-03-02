# Test Executor Report: Timeline Corruption Fix

**Date:** 2026-03-02
**Status:** BLOCKED - Implementation Incomplete
**Phase:** Structural Validation & Gap Analysis

## Summary

Attempted to validate test structure and execute tests for the timeline corruption fix (fbs: fb_b4387bb2, fb_5b41213e). Found that the implementation is incomplete and tests are missing.

## Implementation Status

### ✅ Completed
- `CollectMarketData()` — Has full three-path logic with force refresh merge

### ❌ Incomplete
- `collectCoreTicker()` — Missing `else if force && existing != nil` merge path
- All unit tests — Missing (4 required tests)
- Integration tests — Missing (2 required tests)

## Detailed Findings

### Code Gap in `service.go` (line 356-366)

**Current behavior:** When `force=true && existing != nil && len(existing.EOD) > 0`:
- Falls through to `else` branch at line 356
- Does full 3-year fetch but REPLACES all data (blind replacement)
- Does NOT merge with existing bars
- **BUG PRESERVED:** Older history still lost on force refresh

**Required fix:** Split the `else` block into two paths:
1. `else if force && existing != nil && len(existing.EOD) > 0` — full fetch + merge (using mergeEODBars)
2. `else` — full fetch for new tickers (no merge needed)

### Missing Tests

**Unit tests (internal/services/market/service_test.go):**
1. `TestCollectCoreMarketData_ForceRefreshMergesExistingEOD` — verify merge preserves history
2. `TestCollectCoreMarketData_ForceRefreshEmptyResponsePreservesExisting` — verify guard against empty responses
3. `TestCollectMarketData_ForceRefreshMergesExistingEOD` — non-core path merge
4. `TestCollectMarketData_ForceRefreshEmptyResponsePreservesExisting` — non-core path guard

**Integration tests (tests/api/timeline_corruption_test.go):**
1. `TestForceRefresh_PreservesTimelineIntegrity` — verify timeline data point count preserved
2. `TestForceRefresh_EODBarCount` — verify candle count not reduced

## Blocking Issue

Cannot proceed with test execution until:
1. `collectCoreTicker()` merge path is implemented
2. All 4 unit tests are created
3. Integration test file is created with 2 tests

## Next Steps

1. Implementer: Complete the collectCoreTicker merge fix
2. Test-creator: Add all required tests (or implementer adds inline)
3. Test-executor: Re-validate structure and execute full test suite

## Test Structure Notes

Tests will need to follow mandatory rules from `/test-common`:
- Rule 1: Independent of Claude (standard go test)
- Rule 2: Common containerized setup (use mocks for unit, Docker for integration)
- Rule 3: Test output saved to `tests/logs/{datetime}-{TestName}/`
- Rule 4: Test files not modified by test-executor

Ready to validate and execute once implementation is complete.

---

## Check 2: 2026-03-02 ~07:55 UTC

Re-validated codebase after initial gap notification:

- ✅ CollectMarketData() has correct three-path logic
- ❌ collectCoreTicker() still missing force+merge path (lines 356-366)
  - Current: falls through to `else` which does blind replacement
  - Required: `else if force && existing != nil` path with mergeEODBars()
- ❌ No unit tests found (0 of 4 created)
- ❌ No integration test file (tests/api/timeline_corruption_test.go does not exist)

Sent detailed code fix guidance to implementer with exact line numbers and replacement code blocks.

**Status: STILL BLOCKED** — Waiting for implementation.
