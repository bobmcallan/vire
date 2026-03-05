# Fix: Timeline Cache Stale Historical Snapshots (fb_bbb8b91d, Round 2)

## Problem

The previous fix (c5edb52) added a DataVersion check on `latest` snapshot in `tryTimelineCache()`. But `latest` is **today's** snapshot — written by `writeTodaySnapshot()` with current SchemaVersion. Historical snapshots still have old JSON field names from before the rename. The cache hit serves stale historical data with zero equity fields.

**Evidence from production data:**
- `holding_count` (NOT renamed) = 19, 20, 22 → deserializes correctly from old snapshots
- `portfolio_value` (NOT renamed) = ~$471K → deserializes correctly
- `equity_holdings_value` (RENAMED from `equity_value`) = 0 → zero because old JSON key doesn't match new struct tag
- Only today (2026-03-05) has correct equity values — written by `writeTodaySnapshot` with new field names

## Root Cause

`tryTimelineCache()` line 654 checks `latest.DataVersion` where latest = today's snapshot (always current version via `writeTodaySnapshot`). This check always passes. The actual stale snapshots are the historical ones returned by `GetRange`.

## Fix

### File: `internal/services/portfolio/growth.go`

**Change 1**: After `GetRange` returns snapshots (line 673), add a DataVersion check on the **first** (oldest) snapshot in the range. This catches the case where today is current but historical snapshots are stale.

After line 676 (`if err != nil || len(snapshots) == 0 { return nil, false }`), add:

```go
// Check oldest snapshot for stale schema — writeTodaySnapshot may have updated
// today to current version while historical snapshots retain old field names.
// Stale field names cause renamed fields to deserialize as zero.
if snapshots[0].DataVersion != common.SchemaVersion {
    s.logger.Info().
        Str("cached_version", snapshots[0].DataVersion).
        Str("current_version", common.SchemaVersion).
        Str("oldest_date", snapshots[0].Date.Format("2006-01-02")).
        Msg("Timeline cache stale: oldest snapshot has old schema version, forcing rebuild")
    return nil, false
}
```

Insert this BEFORE the existing first-snapshot date coverage check (line 678).

**Change 2**: Update the existing `latest` DataVersion check comment (line 652) to clarify its role as an optimization (avoids GetRange call when entire cache is stale), vs the new check which catches mixed-version scenarios:

```go
// Quick check: if even the latest snapshot is stale, skip the GetRange call entirely.
// This doesn't catch mixed-version scenarios (today current, historical stale) —
// that's handled after GetRange below.
```

## Unit Test Updates

### File: `internal/services/portfolio/timeline_test.go`

Add test case:

```go
func TestTryTimelineCache_RejectsStaleHistoricalSnapshots(t *testing.T) {
    // Scenario: latest snapshot (today) has current DataVersion (written by writeTodaySnapshot),
    // but historical snapshots have old DataVersion. Cache must miss.
    // Setup: stubTimelineStore with GetLatest returning current version,
    // GetRange returning mix where first snapshot has old version.
}
```

### File: `internal/services/portfolio/timeline_cache_stress_test.go`

Add stress test:

```go
func TestStress_TryTimelineCache_MixedVersionSnapshots(t *testing.T) {
    // Latest = current version, historical = old version.
    // Verify cache miss and GetRange IS called (unlike the early-return optimization test).
}
```

## Integration Test Updates

### File: `tests/data/timeline_schema_version_test.go`

Update `TestTimeline_StaleSchemaVersionForcesRebuild` to explicitly test the mixed-version scenario:
- Seed multiple historical snapshots with old DataVersion
- Seed today's snapshot with current DataVersion
- Call GetDailyGrowth
- Assert equity_holdings_value > 0 for data points with holding_count > 0

Add new test:

```go
func TestTimeline_TodayCurrentButHistoricalStale(t *testing.T) {
    // This is the exact production failure mode:
    // 1. Save 5 historical snapshots with DataVersion="old", zero equity, non-zero holding_count
    // 2. Save today's snapshot with DataVersion=SchemaVersion, non-zero equity
    // 3. Call GetDailyGrowth
    // 4. Verify ALL returned points with holding_count > 0 have equity_holdings_value > 0
    // 5. Verify no data point has holding_count > 0 AND equity_holdings_value == 0
}
```

Add chart-output validation test:

```go
func TestTimeline_ChartOutput_NoZeroEquityWithActiveHoldings(t *testing.T) {
    // The fundamental invariant: if a data point has holding_count > 0,
    // then equity_holdings_value MUST be > 0.
    // This test should catch any future regression that produces the zero-equity chart bug.
}
```

## Scope

### In scope
- Fix DataVersion check to examine first snapshot in range
- Update/add unit tests for mixed-version scenario
- Update/add integration tests for the exact production failure mode
- Add invariant test: holding_count > 0 implies equity_holdings_value > 0

### Out of scope
- Portal changes
- Data migration tooling
