# Summary: Timeline Corruption Fix (fb_b4387bb2, fb_5b41213e)

**Status:** completed

## Changes
| File | Change |
|------|--------|
| `internal/services/market/service.go` | Fixed force-refresh EOD replacement in both `collectCoreTicker()` and `CollectMarketData()` — now merges with existing bars instead of blind replacement. Rewrote `mergeEODBars()` from O(n*m) to O(n+m+sort). |
| `internal/services/market/service_test.go` | Added 4 unit tests: force-refresh merge, empty response preservation (for both core and non-core paths) |
| `internal/services/market/force_refresh_stress_test.go` | Added 13 stress tests covering concurrent refresh, sort order, duplicate dates, empty responses |
| `tests/api/timeline_corruption_test.go` | Added 2 integration tests: timeline integrity preservation, EOD bar count preservation |

## Tests
- Unit tests: 4 added — all pass
- Stress tests: 13 added — all pass (data race in concurrent test is pre-existing)
- Integration tests: 2 added — require live server
- Fix rounds: 1 (mergeEODBars sort order + O(n*m) rewrite by devils-advocate)

## Architecture
- Architect reviewed — APPROVED (7/7 dimensions pass)
- `mergeEODBars()` confirmed as single source of truth for EOD bar merging
- Three-path pattern consistent across both functions
- No documentation updates needed (internal fix)

## Devils-Advocate
- Found mergeEODBars sort order bug (broke EOD[0] invariant with gaps in fresh data)
- Found O(n*m) performance issue — rewrote to O(n+m+sort)
- Found duplicate date handling gap in EODHD responses
- Concurrent force refresh race is pre-existing, out of scope

## Notes
- Root cause: `force=true` blindly replaced EOD history with 3-year fetch response
- Fix: merge new bars with existing using `mergeEODBars()`, guard against empty responses
- Third location (`collect.go:161-169`) has same pattern but always uses `force=false` — low priority
