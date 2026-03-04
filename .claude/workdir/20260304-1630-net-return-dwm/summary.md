# Summary: Net Return D/W/M Period Breakdowns

**Status:** completed

## Feedback Items Addressed
- fb_83fcf232 (MEDIUM): Net Return $ and Net Return % show no D/W/M breakdown — **FIXED**
- fb_9537e416 (MEDIUM): D/W/M on Holdings Value not useful, move to Net Return — **FIXED**

## Changes
| File | Change |
|------|--------|
| `internal/models/portfolio.go` | PeriodChanges: removed `EquityValue`, added `NetEquityReturn` + `NetEquityReturnPct` |
| `internal/services/portfolio/service.go` | `computePeriodChanges`: swapped EquityValue for NetEquityReturn/Pct; new `buildSignedMetricChange` helper for negative P&L |
| `internal/services/portfolio/period_changes_test.go` | NEW: 7 unit tests for buildSignedMetricChange + computePeriodChanges |
| `internal/services/portfolio/changes_stress_test.go` | Updated: references to EquityValue → NetEquityReturn |
| `internal/services/portfolio/service_test.go` | Updated: references to EquityValue → NetEquityReturn |
| `internal/server/catalog_test.go` | Fixed: tool count 75→76 (pre-existing from holding notes) |
| `tests/data/period_changes_test.go` | NEW: 6 integration tests |

## Tests
- Unit tests: 7 in `internal/services/portfolio/period_changes_test.go` — ALL PASS
- Integration tests: 6 in `tests/data/period_changes_test.go` — ALL PASS
- Portfolio tests: 503/503 PASS
- No regressions
- Fix rounds: 0

## Architecture
- Architect review: APPROVED
- Separation of concerns: data sourced from TimelineSnapshot (correct owner)
- No legacy compatibility shims
- JSON field naming consistent (snake_case)

## Devils-Advocate
- APPROVED — no blocking findings
- Negative P&L edge cases covered by buildSignedMetricChange + 5 unit tests
- Zero-division guard present (previous != 0 check)

## Key Design Decision
`buildSignedMetricChange` vs `buildMetricChange`: P&L values can be negative, so `HasPrevious` cannot be derived from `previous > 0`. The new helper takes an explicit `hasPrevious` bool and uses `math.Abs(previous)` for the PctChange denominator.

## Portal Impact
The portal needs updating to consume new JSON fields:
- `changes.yesterday.net_equity_return` (was `equity_value`)
- `changes.yesterday.net_equity_return_pct` (new)
- Same for `.week` and `.month` periods
