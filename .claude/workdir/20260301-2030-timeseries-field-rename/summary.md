# Summary: TimeSeriesPoint Field Rename for Portfolio Consistency

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Renamed TimeSeriesPoint fields (Value→TotalValue, Cost→TotalCost, CashBalance→TotalCash, NetDeployed→NetCapitalDeployed), added AvailableCash, removed ExternalBalance. Removed ExternalBalance from GrowthDataPoint. |
| `internal/services/portfolio/indicators.go` | Updated GrowthPointsToTimeSeries: new field names + AvailableCash computation |
| `internal/services/portfolio/growth.go` | Removed ExternalBalance: 0 from GrowthDataPoint construction |
| `internal/services/portfolio/*_test.go` | Updated all unit test field references (5 files) |
| `tests/api/*_test.go` | Updated all integration test JSON field names (5 files) |
| `internal/server/catalog.go` | Updated MCP tool descriptions |

## Tests

- Unit tests: 409+ portfolio tests PASS
- Integration tests: New history/review tests PASS (5/5). Pre-existing CapitalTimeline test failures (bad payloads, Docker timeouts) — unrelated.
- Build: PASS
- go vet: PASS
- Fix rounds: 0

## Architecture

- Reviewed by team-lead: field names now consistent with portfolio response
- ExternalBalance fully removed (deprecated, was always 0)
- GrowthPointsToTimeSeries remains the single conversion point

## Devils-Advocate

- Proactive analysis sent to implementer (5 categories reviewed)

## Notes

- Pre-existing integration test failures in TestCapitalTimeline_* (bad POST payloads using "type" instead of "category"/"account") — separate fix needed
