# Requirements: Embed Capital Performance in get_portfolio Response

**Date:** 2026-02-24
**Requested:** fb_91eee964 — Include capital performance metrics (XIRR, simple return, capital in/out) directly in the `get_portfolio` response instead of requiring a separate `get_capital_performance` call.

## Scope

### In scope
- Add `CapitalPerformance *models.CapitalPerformance` field to `Portfolio` struct (pointer, omitempty — nil when no cash transactions exist)
- Compute capital performance in `handlePortfolioGet` after loading the portfolio, using the existing `CashFlowService.CalculatePerformance()`
- Update `get_portfolio` MCP tool description to mention the new field
- Unit tests for the new field population
- Integration tests

### Out of scope
- Changing the `CapitalPerformance` struct itself
- Removing the separate `get_capital_performance` endpoint (keep for backward compat)
- Caching capital performance in the stored portfolio record (compute on response)

## Approach

**Compute-on-response, not compute-on-sync.** Capital performance depends on both the portfolio value (changes with market) and cash transactions (change independently). Embedding it in the stored portfolio would create staleness. Instead:

1. **Model change** (`internal/models/portfolio.go`): Add `CapitalPerformance *CapitalPerformance` field to `Portfolio` struct with `json:"capital_performance,omitempty"`.

2. **Handler change** (`internal/server/handlers.go`): In `handlePortfolioGet`, after loading the portfolio and before writing the JSON response, call `s.app.CashFlowService.CalculatePerformance(ctx, name)`. If it returns a non-empty result (TransactionCount > 0), set `portfolio.CapitalPerformance = perf`. If the call fails or returns no data, leave it nil (don't fail the entire portfolio request).

3. **Catalog change** (`internal/server/catalog.go`): Update the `get_portfolio` tool description to mention capital performance is included.

4. **No interface changes needed** — `CashFlowService` is already accessible via `s.app.CashFlowService`.

5. **Note on `CalculatePerformance` circular dependency**: `CalculatePerformance` calls `s.portfolioService.GetPortfolio()` internally to get current portfolio value. In `handlePortfolioGet`, we already have the portfolio loaded, so we're calling `GetPortfolio()` twice. This is acceptable because `GetPortfolio()` returns cached data (no re-sync), so the second call is cheap. An alternative would be a new method that accepts the portfolio value directly, but that's over-engineering for this task.

## Files Expected to Change
- `internal/models/portfolio.go` — add CapitalPerformance field to Portfolio struct
- `internal/server/handlers.go` — compute and attach capital performance in handlePortfolioGet
- `internal/server/catalog.go` — update get_portfolio description
- `internal/server/handlers_portfolio_test.go` — unit tests
- `internal/services/portfolio/service_test.go` — if needed
- `tests/api/portfolio_indicators_test.go` or new test file — integration tests
