# Summary: Embed Capital Performance in get_portfolio Response

**Date:** 2026-02-24
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `CapitalPerformance *CapitalPerformance` field to Portfolio struct (pointer, omitempty) |
| `internal/server/handlers.go` | In `handlePortfolioGet`, compute and attach capital performance after loading portfolio. Non-fatal — errors are swallowed, empty results skipped. Nil guard for safety. |
| `internal/server/catalog.go` | Updated `get_portfolio` MCP tool description to mention `capital_performance` inclusion |
| `internal/server/handlers_portfolio_test.go` | Added `mockCashFlowService`, `newTestServerWithCashFlow`, 3 unit tests + 8 stress tests (nil return, extreme values, negative returns, JSON omit/present, backward compat, concurrency) |
| `tests/api/portfolio_capital_test.go` | New integration test: 5 subtests (absent without transactions, add transaction, present with transactions, matches standalone endpoint, cleanup) |
| `.claude/skills/develop/SKILL.md` | Updated Portfolio model and Cash Flow sections to document the new field. Removed `run.sh`/`localhost:8501` references per user request. |

## Tests
- 3 unit tests (happy path, empty ledger, error handling)
- 8 stress tests (nil return, extreme values, negative returns, JSON serialization, backward compatibility, concurrency)
- 5 integration subtests (containerized, full lifecycle)
- All pass

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — Portfolio model and Cash Flow sections

## Devils-Advocate Findings
- Nil safety: `CalculatePerformance` could theoretically return `(nil, nil)` — added nil guard (`perf != nil`)
- NaN/Inf: XIRR already guards against these in `computeXIRR`
- JSON serialization: pointer + omitempty correctly omits when nil
- Concurrency: each request gets its own portfolio copy — no shared mutable state
- Backward compatibility: old JSON without `capital_performance` deserializes cleanly

## Notes
- The `get_capital_performance` standalone endpoint is preserved for backward compatibility
- Capital performance is computed on every `get_portfolio` call (not cached) — this is deliberate since portfolio value changes with market data
- The computation involves two cheap storage reads (ledger + cached portfolio)
