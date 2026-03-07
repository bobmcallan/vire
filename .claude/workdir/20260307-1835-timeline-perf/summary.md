# Summary: Timeline Data Fix + Dashboard Performance

**Status:** completed

## Root Cause Analysis

1. **Trading portfolio "flat" timeline**: WES.AU had a bad EOD bar from EODHD (Mar 6: close=41.71 instead of ~75.82). This poisoned `yesterday_close_price` and all percentage calculations. The `eodClosePrice()` divergence guard only checks AdjClose vs Close on the same bar, not bar-to-bar divergence.

2. **Dashboard load optimization**: N+1 signal queries in `populateFromMarketData` and duplicate per-holding `GetMarketData` calls in `SyncPortfolio` TWRR/country loop.

## Changes

| File | Change |
|------|--------|
| `internal/services/market/service.go` | Added `filterBadEODBars()` package-level function (>40% bar-to-bar divergence guard, checks both neighbors). Called at all 9 EOD assignment sites. Added Fix B: recalculate YesterdayPct/LastWeekPct after live quote override. |
| `internal/services/market/collect.go` | Added `filterBadEODBars()` calls at 3 EOD assignment sites (CollectBulkEOD, CollectEOD) |
| `internal/services/market/screen.go` | Added `filterBadEODBars()` call in Screener EOD assignment |
| `internal/services/portfolio/service.go` | Fix C: batch signal queries via `GetSignalsBatch` before holdings loop. Fix D: batch market data via `GetMarketDataBatch` before TWRR/country loop. |
| `internal/services/market/service_test.go` | 6 new unit tests: 5 for filterBadEODBars, 1 for live quote pct recalculation |
| `internal/services/market/eod_divergence_stress_test.go` | Stress tests from devils-advocate |
| `internal/services/market/pct_recalc_stress_test.go` | Stress tests from devils-advocate |
| `tests/data/eod_divergence_test.go` | 4 integration tests: storage roundtrip, divergent first bar, legitimate move, too few bars |
| `tests/data/stockdata_pct_test.go` | 3 integration tests: live quote recalc, no-live-quote baseline, zero historical close |
| `tests/api/portfolio_batch_fixes_test.go` | API-level batch fixes tests |

## Tests

- 6 unit tests added in `internal/services/market/service_test.go` — all PASS
- 7 integration tests added in `tests/data/` — all PASS
- API tests in `tests/api/` — PASS (container tests skipped)
- Stress tests in `internal/services/market/` — PASS
- Full suite `go test ./internal/...` — all 25 packages PASS
- `go vet ./...` — clean
- `go build ./cmd/vire-server/` — clean

## Architecture

- `filterBadEODBars` is a package-level function in market package (shared by Service and Screener)
- Bar is marked divergent only if it disagrees with BOTH neighbors (reduces false positives)
- Batch queries use existing `GetSignalsBatch` and `GetMarketDataBatch` interfaces
- No new cross-package dependencies

## Devils-Advocate

- Edge case: all bars divergent → returns empty (acceptable, better than poisoned data)
- Edge case: legitimate >40% moves (penny stocks, splits) → checked both neighbors mitigates
- Division by zero: guarded with `> 0` checks in pct recalculation
- Nil pointer: fixed MarketDataStorage() nil check in SyncPortfolio batch path

## Notes

- After deploying, force re-collect WES.AU to purge the bad bar from cache
- The 40% threshold is conservative but catches the WES.AU case (41.71 vs 75.82 = 45% drop)
- Checking both neighbors reduces false positives for legitimate large moves
