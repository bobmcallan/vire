# Summary: Timeline Cache Stale Historical Snapshots (fb_bbb8b91d, Round 2)

**Status:** completed

## Problem

Round 1 fix (c5edb52) checked `latest.DataVersion` in `tryTimelineCache()`, but `latest` is today's snapshot — always current version via `writeTodaySnapshot()`. Historical snapshots with old JSON field names (pre-rename) passed undetected, causing equity fields to deserialize as zero.

## Root Cause

Field rename changed JSON tags (e.g. `equity_value` → `equity_holdings_value`). Old snapshots in SurrealDB still have old keys. Fields that were NOT renamed (`portfolio_value`, `holding_count`) deserialized correctly; renamed fields (`equity_holdings_value`, `equity_holdings_cost`, etc.) deserialized as zero.

## Changes

| File | Change |
|------|--------|
| `internal/services/portfolio/growth.go` | Added `snapshots[0].DataVersion` check after GetRange (line 679-689). Updated comment on existing latest check to clarify dual-check strategy. |
| `internal/services/portfolio/timeline_test.go` | Added `TestTryTimelineCache_RejectsStaleHistoricalSnapshots` |
| `internal/services/portfolio/timeline_cache_stress_test.go` | Added 6 stress tests: mixed-version, legitimate zero equity, partial rebuild, today-only, concurrent access |
| `tests/data/timeline_schema_version_test.go` | Updated `TestTimeline_StaleSchemaVersionForcesRebuild` + added 3 new tests: TodayCurrentButHistoricalStale, ChartOutput_NoZeroEquityWithActiveHoldings, ChartOutput_AllFieldsConsistent |

## Tests

- Unit tests: 1 added (mixed-version scenario)
- Stress tests: 6 added (all pass with -race)
- Integration tests: 3 added + 1 updated (chart output validation)
- Full suite: **721 pass, 0 fail** (361 API skipped — no Docker)
- Fix rounds: 0

## Architecture

- Architect approved: dual checks serve distinct purposes (optimization vs correctness)
- Self-healing confirmed: cache miss → trade replay → persist overwrites stale records

## Devils-Advocate

- 6 edge cases tested, no issues
- Known limitation documented: only snapshots[0] checked, not mid-range. Accepted because SaveBatch is atomic.
- Concurrent access safe (50 goroutines, race detector clean)

## Key Test: Production Failure Invariant

`TestTimeline_ChartOutput_NoZeroEquityWithActiveHoldings` — the test that would have caught the original bug:
- **Invariant**: `holding_count > 0` implies `equity_holdings_value > 0`
- Tests both fresh computation and cache paths
