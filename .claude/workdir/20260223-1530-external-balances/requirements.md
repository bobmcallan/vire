# Requirements: External Balances for Portfolios

**Date:** 2026-02-23
**Requested:** Add external balance support to portfolios (fb_23b15640). Initial data: Stake Accumulate $50,000 (~5% p.a., accumulate type) and ANZ Cash $44,000 (cash type) for SMSF portfolio. Total external balance $94,000 AUD.

## Scope

### In Scope
- `ExternalBalance` model struct with type (cash, accumulate, term_deposit, offset), label, value, rate, notes
- `ExternalBalances` field on `Portfolio` model — persisted as part of portfolio JSON in UserDataStore
- MCP tools: `get_external_balances`, `set_external_balances`, `add_external_balance`, `remove_external_balance`
- API endpoints: GET/PUT/POST/DELETE on `/api/portfolios/{name}/external-balances`
- Weight recalculation: include external balance total in denominator so weights reflect true portfolio allocation
- Portfolio response: include `external_balances` and `external_balance_total` fields
- Portfolio review: include external balance total in total_value and weight calculations
- Unit tests for model, service methods, and weight calculation
- Integration tests for API endpoints

### Out of Scope
- Navexa integration for external balances (these are manually managed)
- Strategy rules referencing external balances (cash floor compliance etc.) — future work
- Interest accrual calculations
- Historical tracking of external balance changes
- Portal UI changes

## Approach

### Data Model
Add to `internal/models/portfolio.go`:
```go
type ExternalBalance struct {
    ID    string  `json:"id"`              // Short unique ID (eb_ prefix)
    Type  string  `json:"type"`            // "cash", "accumulate", "term_deposit", "offset"
    Label string  `json:"label"`           // e.g. "ANZ Cash", "Stake Accumulate"
    Value float64 `json:"value"`           // Current value in portfolio currency
    Rate  float64 `json:"rate,omitempty"`  // Annual rate (e.g. 0.05 for 5%)
    Notes string  `json:"notes,omitempty"` // Free-form notes
}
```

Add to `Portfolio` struct:
```go
ExternalBalances     []ExternalBalance `json:"external_balances,omitempty"`
ExternalBalanceTotal float64           `json:"external_balance_total"`
```

### Weight Calculation Change
Currently: `weight = holding.MarketValue / sum(holdings.MarketValue) * 100`
After: `weight = holding.MarketValue / (sum(holdings.MarketValue) + externalBalanceTotal) * 100`

This means holding weights will no longer sum to 100% — the remainder is the external balance allocation. This is correct: a portfolio with $100k in stocks and $50k cash should show stocks at ~67% weight, not 100%.

### Storage
External balances are stored as part of the Portfolio JSON in UserDataStore (subject: "portfolio"). No new storage layer needed — they serialise/deserialise with the existing Portfolio struct.

A separate service layer manages CRUD operations. The portfolio service delegates external balance operations to dedicated methods that load the portfolio, modify the external balances slice, recompute totals and weights, and save.

### API Endpoints
- `GET /api/portfolios/{name}/external-balances` — list external balances
- `PUT /api/portfolios/{name}/external-balances` — replace all external balances
- `POST /api/portfolios/{name}/external-balances` — add one external balance
- `DELETE /api/portfolios/{name}/external-balances/{id}` — remove one external balance

### MCP Tools
- `get_external_balances` — GET, returns external balances for a portfolio
- `set_external_balances` — PUT, replaces all external balances (bulk set)
- `add_external_balance` — POST, adds a single external balance
- `remove_external_balance` — DELETE, removes a single external balance by ID

### Schema Version
Bump `SchemaVersion` in `internal/common/version.go` to trigger re-sync for cached portfolios. The new `ExternalBalances` field uses `omitempty` so existing portfolios deserialise cleanly with nil/empty slice.

## Files Expected to Change
- `internal/models/portfolio.go` — ExternalBalance struct, Portfolio fields
- `internal/services/portfolio/service.go` — weight recalculation, external balance CRUD methods
- `internal/services/portfolio/external_balances.go` — new file for external balance service methods
- `internal/interfaces/services.go` — PortfolioService interface additions
- `internal/server/handlers.go` — API endpoint handlers for external balances
- `internal/server/routes.go` — route registration
- `internal/server/catalog.go` — MCP tool definitions
- `internal/common/version.go` — schema version bump
- `internal/services/portfolio/service_test.go` — unit tests (existing or new)
- `internal/services/portfolio/external_balances_test.go` — unit tests for external balance logic
- `tests/api/` — integration tests
