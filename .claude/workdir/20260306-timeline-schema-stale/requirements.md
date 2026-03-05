# Fix: Timeline Cache Serves Stale Snapshots After Schema Rename (fb_bbb8b91d)

## Problem

After the canonical field rename (v0.3.166, SchemaVersion bump), timeline snapshots persisted in SurrealDB still contain old JSON field names (e.g. `equity_value` instead of `equity_holdings_value`). When `tryTimelineCache()` reads these snapshots, the SurrealDB driver deserializes using current JSON tags — renamed fields unmarshal as zero. This causes:

1. **Chart: Equity Value flatlined at $0.00** — `equity_holdings_value` reads as 0 from stale snapshots
2. **Changes block: has_previous=false** — `computePeriodChanges` queries timeline for reference dates, gets zero values, `buildMetricChange` sets `HasPrevious: previous > 0` → false
3. **Dashed line spike at selldown** — partial/corrupt snapshot during position selldown not corrected by outlier guard (guard only runs during trade replay, not cache reads)

## Root Cause

`tryTimelineCache()` in `growth.go:641` does NOT check `DataVersion` on cached snapshots. It only validates date coverage. Stale snapshots with old field names pass the date check but return zero-valued fields.

The pattern already exists elsewhere: `getPortfolioRecord()` in `service.go:2423` rejects portfolios with stale `DataVersion`. Timeline cache needs the same guard.

## Fix

**Single change in `growth.go`**: In `tryTimelineCache()`, after fetching the latest snapshot (line 647), check `latest.DataVersion != common.SchemaVersion`. If stale, return cache miss. The existing trade replay path will recompute all points and `persistTimelineSnapshots()` will overwrite old records with current field names and version.

### File: `internal/services/portfolio/growth.go`

In `tryTimelineCache()`, after line 648 (`if err != nil || latest == nil`), add:

```go
// Reject stale snapshots — field names may have changed between schema versions.
// Cache miss forces full trade replay, which re-persists with current field names.
if latest.DataVersion != common.SchemaVersion {
    s.logger.Info().
        Str("cached_version", latest.DataVersion).
        Str("current_version", common.SchemaVersion).
        Msg("Timeline cache stale: schema version mismatch, forcing rebuild")
    return nil, false
}
```

This is inserted between the nil check (line 648) and the date coverage check (line 652).

## Scope

### In scope
- Add DataVersion check to `tryTimelineCache()`
- Integration tests for timeline schema staleness
- Integration tests for timeline field completeness

### Out of scope
- Portal field rename migration (separate feedback items)
- Manual timeline purge/rebuild commands
- Changing `buildMetricChange` to use `HasPrevious` differently (works correctly once data is valid)

## Test Cases

### Unit test (in existing test file or new)
1. `TestTryTimelineCache_RejectsStaleVersion` — mock timeline store returns snapshot with old DataVersion, verify cache miss
2. `TestTryTimelineCache_AcceptsCurrentVersion` — snapshot with current version, verify cache hit

### Integration tests (tests/data/)
3. `TestTimeline_StaleSchemaVersionForcesRebuild` — save snapshot with old version, call GetDailyGrowth, verify it recomputes (doesn't return stale data)
4. `TestTimeline_AllFieldsPopulated` — after GetDailyGrowth, verify every field in the returned GrowthDataPoints is non-zero for dates with active holdings
5. `TestTimeline_FieldPersistence_RoundTrip` — save snapshots via persistTimelineSnapshots, read back via tryTimelineCache, verify all fields match
6. `TestTimeline_PeriodChanges_WithValidSnapshots` — verify computePeriodChanges returns HasPrevious=true when valid snapshots exist
7. `TestTimeline_PeriodChanges_WithStaleSnapshots` — verify computePeriodChanges falls back gracefully when snapshots are stale
8. `TestTimeline_SelldownPeriod_NoSpike` — timeline through a selldown period should show smooth transition to zero, no spikes

## Integration Points

- `growth.go:648` — insert DataVersion check after nil check
- No other production code changes needed
- Self-healing: the cache miss triggers trade replay → persistTimelineSnapshots → old records overwritten

## Patterns to Follow

- `service.go:2423-2424` — existing DataVersion check pattern for portfolio records
- `tests/data/timeline_cash_autload_test.go` — existing timeline integration test patterns using testManager(t)
- `tests/data/portfolio_dataversion_test.go` — existing DataVersion staleness test patterns
