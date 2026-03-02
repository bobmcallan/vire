# Summary: Portfolio Dividend Return Field (fb_6d01379b)

**Status:** completed

## Changes
| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `DividendReturn float64 \`json:"dividend_return"\`` to Portfolio struct |
| `internal/services/portfolio/service.go` | Set `DividendReturn: totalDividends` in portfolio construction |
| `internal/server/catalog.go` | Updated `get_portfolio` MCP description to mention `dividend_return` |
| `internal/common/version.go` | Bumped SchemaVersion 9 → 10 (forces re-sync of cached portfolios) |
| `internal/services/portfolio/service_test.go` | Added 3 unit tests |
| `internal/services/portfolio/dividend_return_stress_test.go` | Added 11 stress tests |
| `tests/api/portfolio_dividend_return_test.go` | Added 5 integration tests |

## Tests
- Unit tests: 3 added (sum, zero, included-in-net-return) — all pass
- Stress tests: 11 added (negative dividends, FX conversion, JSON round-trip, empty portfolio, large values, closed positions, identity relationship, cache consistency) — all pass
- Integration tests: 5 added (field present, equals holding sum, zero dividends, net equity return, FX conversion) — created, require live server
- Fix rounds: 1 (naming convention fix: `TotalDividendReturn` → `DividendReturn`)

## Architecture
- Architect flagged naming convention violation (rule #4: no `total_` prefix)
- Field renamed from `TotalDividendReturn` / `total_dividend_return` to `DividendReturn` / `dividend_return`
- No architecture docs update needed — additive field, no new interfaces or patterns

## Devils-Advocate
- 11 stress tests covering: negative dividends, FX conversion, JSON round-trip, empty portfolios, very large values, closed positions, identity relationship with net equity return
- All edge cases pass — no issues found

## Notes
- The `totalDividends` variable was already computed in SyncPortfolio (lines 400-404) — this change simply exposes it as a Portfolio struct field
- Currency gain tracking remains a separate pending feature (docs/features/20260223-fx-currency-gain-calculation.md)
- Portal should use the new `dividend_return` field from the portfolio response instead of computing its own sum from holdings
