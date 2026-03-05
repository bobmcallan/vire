# Summary: Timeline Cache Stale Schema Version Fix (fb_bbb8b91d)

**Status:** completed

## Root Cause

After the v0.3.166 canonical field rename, timeline snapshots in SurrealDB still had old JSON keys (e.g. `equity_value` instead of `equity_holdings_value`). `tryTimelineCache()` didn't check `DataVersion`, so it served stale snapshots where renamed fields deserialized as zero. This caused:
- Chart: Equity Value flatlined at $0.00
- Changes block: `has_previous=false` for all periods
- Dashed line spike during selldown (uncorrected by outlier guard on cache path)

## Changes

| File | Change |
|------|--------|
| `internal/services/portfolio/growth.go` | Added DataVersion != SchemaVersion check in `tryTimelineCache()` at line 652. Cache miss on stale version forces trade replay + re-persist. |
| `internal/services/portfolio/timeline_test.go` | 3 unit tests: RejectsStaleVersion, AcceptsCurrentVersion, RejectsEmptyVersion |
| `internal/services/portfolio/timeline_cache_stress_test.go` | 10 stress tests: empty/future/stale versions, concurrency, persist failure, early return optimization |
| `tests/data/timeline_schema_version_test.go` | 8 integration tests: stale rebuild, field completeness, round-trip persistence, period changes, selldown transition, cache hit after rebuild |

## Tests

- Unit tests: 3 added, all pass
- Stress tests: 10 added, all pass (including race detector)
- Integration tests: 8 added, all pass
- Full suite: **972 pass, 0 fail** (361 API skipped — no Docker)
- Fix rounds: 3 (test-creator fixed 2 integration test setup issues)

## Architecture

- Architect approved: follows existing DataVersion pattern from `getPortfolioRecord()`
- Self-healing: cache miss → trade replay → `persistTimelineSnapshots()` overwrites stale records with current SchemaVersion
- No new dependencies, no legacy shims

## Devils-Advocate

- 10 edge cases tested, no issues found
- Concurrent access safe (50 goroutines, race detector clean)
- Empty DataVersion (legacy data) correctly rejected
- No infinite rebuild loop on persist failure

## Notes

- The fix is self-healing: first `GetDailyGrowth` call after the schema bump will cache-miss, recompute, and re-persist all snapshots with correct field names
- Timeline scheduler's rebuild loop (runs on startup) will also trigger the self-heal automatically
- No manual intervention needed for existing portfolios
