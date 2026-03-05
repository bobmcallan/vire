# Summary: Portfolio Breadth Summary (Server-Side)

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `PortfolioBreadth` struct (counts, dollar-weighted proportions, trend label/score, today change) and `Breadth` field on Portfolio |
| `internal/services/portfolio/service.go` | Added `computeBreadth()` method + `breadthTrendLabel()` helper, called from `populateHistoricalValues` |
| `internal/services/portfolio/breadth_test.go` | 7 unit tests covering all scenarios |
| `tests/data/portfolio_breadth_test.go` | 5 integration tests, 12 subtests |

## Tests

- Unit tests: 7 added (all rising, mixed, no holdings, no trend data, dollar weighting, today change, label boundaries)
- Integration tests: 5 created (12 subtests — JSON round-trip, counts, weights sum, omitempty, score range)
- Test results: 203+ pass, 0 fail
- Fix rounds: 0

## Architecture

- Architect approved: all 7 checkpoints passed
- Pure aggregation of existing holding fields — no new service dependencies
- Not persisted — computed on response, omitempty when nil

## Devils-Advocate

- Stress tests passed
- No fixes needed

## Notes

- Feature 1 of fb_411cb26f (image attachments) already shipped in v0.3.171
- Portal feedback to be created for breadth bar UI rendering
