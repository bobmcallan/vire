# Summary: Feedback Batch Fix (7 Items)

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | Renamed `CapitalPerformance.EquityValue` → `CurrentValue` (JSON: `current_value`) |
| `internal/services/cashflow/service.go` | `simple_capital_return_pct` uses `portfolio.PortfolioValue` instead of `EquityValue` |
| `internal/server/handlers.go` | `net_capital_return` guard changed from `> 0` to `!= 0` |
| `internal/server/glossary.go` | Fixed D/W/M: `EquityValue` → `PortfolioValue`, removed duplicate terms, fixed definitions/formulas |
| `internal/server/handlers_trade.go` | Added `extractNavexaTrades` helper for Navexa-sourced portfolios |
| `internal/models/market.go` | Added `Advisory []string` field to `StockData` |
| `internal/services/market/service.go` | Set advisory when price data unavailable from EODHD |

## Tests
- Unit tests: all passing (implementer verified)
- Integration tests created by test-creator (task #6)
- Test execution completed (task #5)
- Fix rounds: 2 (reviewer found 3 build errors, devils-advocate found 3 test failures — both resolved)

## Architecture
- Architect approved all 6 fixes
- `extractNavexaTrades` correctly placed in handler layer (not TradeService) — separation of concerns maintained
- `CapitalPerformance.CurrentValue` rename is clean — no legacy shims

## Devils-Advocate
- Found 3 test failures from Fix 1 (wrong expected values after rename) — fixed by implementer
- Stress-tested edge cases: division by zero, negative deployed capital, nil arrays

## Feedback Items Resolved
| ID | Issue | Fix |
|----|-------|-----|
| fb_51aee182 | simple_capital_return_pct -44% (should be -1.27%) | Use PortfolioValue not EquityValue |
| fb_cedb56ee | net_capital_return $0.00 | Relax guard to `!= 0` |
| fb_1bf1e2f0 | D/W/M deltas show false $200k swings | Compare like-for-like PortfolioValue |
| fb_dda6e7e0 | Glossary duplicates and wrong formulas | Remove duplicates, fix definitions |
| fb_9e063cf1 | trade_list empty for Navexa | Extract trades from holdings |
| fb_6cbc1eea | DFND.AU missing price/signals | Advisory when EODHD data unavailable |
| fb_e343c626 | SXE.AU/SRG.AU missing price/signals | Same advisory fix |
