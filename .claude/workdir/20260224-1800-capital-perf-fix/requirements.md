# Requirements: Fix capital performance ignoring external balances

**Date:** 2026-02-24
**Requested:** Fix fb_af390eb7 (HIGH) and dismiss fb_b36f8821 (MEDIUM, out-of-scope)

## Scope
- Fix `get_capital_performance` returning equity-only value instead of equity + external balances
- Fix `SyncPortfolio` losing external balances on schema version bumps
- Dismiss fb_b36f8821 (portal repo, not vire-server)

## Root Cause Analysis

**fb_af390eb7:** Two interconnected bugs:

1. **SyncPortfolio loses external balances on schema bump** (`internal/services/portfolio/service.go:363`):
   `getPortfolioRecord()` validates schema version. When schema is bumped, this returns an error,
   causing `existingExternalBalances` and `existingExternalBalanceTotal` to default to nil/0.
   The portfolio is saved WITHOUT external balances, and `TotalValue` = equity only.

2. **CalculatePerformance uses TotalValue directly** (`internal/services/cashflow/service.go:251`):
   `currentValue := portfolio.TotalValue` — if SyncPortfolio lost external balances,
   TotalValue = equity only, producing wildly wrong returns (-46% vs +0.9%).

**fb_b36f8821:** `/mcp-info` page is in the vire-portal repository, not vire-server. Out of scope.

## Approach

**Fix 1 — SyncPortfolio: bypass schema validation for external balance preservation**
In `SyncPortfolio` (lines 360-366), replace `getPortfolioRecord` with a raw `UserDataStore.Get`
that unmarshals external balances without validating schema version. External balances are simple
data (type, label, value) that don't depend on schema.

**Fix 2 — CalculatePerformance: use explicit field sum**
Change line 251 from `currentValue := portfolio.TotalValue` to
`currentValue := portfolio.TotalValueHoldings + portfolio.ExternalBalanceTotal`.
This is more explicit about intent and resilient to any TotalValue inconsistency.

## Files Expected to Change
- `internal/services/portfolio/service.go` — SyncPortfolio external balance loading
- `internal/services/cashflow/service.go` — CalculatePerformance value calculation
- `internal/services/portfolio/service_test.go` — unit test for schema-resilient balance preservation
- `internal/services/cashflow/service_test.go` — unit test for explicit field sum
- `internal/services/portfolio/fx_stress_test.go` — stress test if needed
- `tests/api/portfolio_capital_test.go` — integration test update if needed
- `.claude/skills/develop/SKILL.md` — document the fix
