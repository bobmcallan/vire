# Summary: Fix capital performance ignoring external balances

**Date:** 2026-02-24
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | SyncPortfolio: replaced `getPortfolioRecord` (validates schema version) with raw `UserDataStore.Get` for loading existing external balances — external balances now survive schema version bumps |
| `internal/services/cashflow/service.go` | CalculatePerformance: changed `portfolio.TotalValue` to `portfolio.TotalValueHoldings + portfolio.ExternalBalanceTotal` (explicit field sum, resilient to stale TotalValue) |
| `internal/services/portfolio/service_test.go` | Added `TestSyncPortfolio_PreservesExternalBalances_StaleSchema` — verifies external balances survive sync with stale schema version |
| `internal/services/cashflow/service_test.go` | Added `TestCalculatePerformance_UsesExplicitFieldSum` — verifies explicit field sum over stale TotalValue |
| `internal/services/cashflow/capital_perf_stress_test.go` | New file: 10 stress tests covering NaN/Inf/negative/zero/large values, micro-transactions, dividends, transfers, boundary input lengths |
| `tests/api/portfolio_capital_test.go` | Added 3 integration test suites (17 subtests): external balance inclusion, preservation across sync, multiple external balances |
| `.claude/skills/develop/SKILL.md` | Updated External Balances and Cash Flow sections to document the schema-resilient read and explicit field sum |

## Tests
- 2 new unit tests (schema-resilient balance preservation, explicit field sum)
- 10 new stress tests (NaN, Inf, negative, zero, large values, micro-transactions, dividends, transfers, boundary inputs)
- 3 new integration test suites (17 subtests total, containerized)
- All pass

## Documentation Updated
- `.claude/skills/develop/SKILL.md` — External Balances and Cash Flow sections

## Devils-Advocate Findings
- NaN/Inf propagation: explicit sum propagates NaN/Inf from TotalValueHoldings/ExternalBalanceTotal — correct behavior (caller swallows errors)
- Negative ExternalBalanceTotal: validation prevents this, but explicit sum handles it correctly if corrupted data gets through
- Float precision: tested with 1000 micro-deposits ($10.01 each) — no precision issues
- Concurrency: SyncPortfolio holds sync mutex, AddExternalBalance reads current record — no race

## Notes
- Root cause: `getPortfolioRecord` validates schema version. After a schema bump, external balance loading fails silently, causing `TotalValue` = equity only. The -46% annualized return vs +0.9% was caused by missing $50K external balances from the terminal value.
- fb_b36f8821 (hardcoded MCP URL on /mcp-info): dismissed as out-of-scope — page is in vire-portal repo, not vire-server.
