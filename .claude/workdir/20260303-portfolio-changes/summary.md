# Summary: Portfolio Changes Section

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `MetricChange`, `PeriodChanges`, `PortfolioChanges` structs; Added `Changes *PortfolioChanges` field to Portfolio; Added `CumulativeDividendReturn` field to TimelineSnapshot |
| `internal/services/portfolio/service.go` | Added `populateChanges()`, `computePeriodChanges()`, `buildMetricChange()`, `cumulativeDividendsByDate()` functions; Updated `writeTodaySnapshot()` to include dividend |
| `internal/services/portfolio/service_test.go` | Added 7 unit tests for changes functionality |
| `internal/services/portfolio/changes_stress_test.go` | Added 28 stress tests for edge cases |
| `tests/api/portfolio_changes_test.go` | Added 2 integration tests |

## Tests
- Unit tests: 7 new tests in service_test.go
- Stress tests: 28 new tests in changes_stress_test.go
- Integration tests: 2 new tests in portfolio_changes_test.go
- Test results: ALL PASS
- Fix rounds: 2 (syntax error in stress test, integration test helper functions)

## Architecture
- Separation of concerns maintained: dividend computation delegates to `CashFlowLedger` methods
- No legacy compatibility shims added
- Pattern consistent with existing timeline/cash flow code

## Devils-Advocate
- 28 stress tests covering: zero values, negative values, infinity, NaN, concurrency, nil portfolio, very large numbers
- All edge cases handled correctly
- No critical bugs found

## Notes
- The `changes` section is computed on response from timeline snapshots (fast path) or ledger (fallback)
- TimelineSnapshot now includes `CumulativeDividendReturn` for future optimization
- Pre-existing lint issues in test files (errcheck) not addressed - out of scope
