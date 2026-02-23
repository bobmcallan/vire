# Requirements: ACDC Price Fix + Cash Flow Tracking

**Date:** 2026-02-24
**Requested:** Fix ACDC.AU bad price (fb_52547389) and implement cash flow tracking with true performance calculation (fb_6337c9d1)

## Issue 1: ACDC.AU Bad Price (fb_52547389, HIGH)

**Problem:** ACDC.AU showing $5.11 in portfolio instead of ~$146. Causes ~$40k equity understatement.

**Root Cause:** Portfolio sync price refresh at `service.go:210` uses `latestBar.Close` (unadjusted) instead of `latestBar.AdjClose`. After ACDC's unit consolidation, the unadjusted close still reflects pre-consolidation prices, while `AdjClose` (and the real-time quote API) return the correct post-consolidation price.

**Fix:**
- Change `service.go:210` to prefer `AdjClose` over `Close`
- Add fallback: if `AdjClose` is 0 or unavailable, fall back to `Close`
- Update logging to show both Close and AdjClose for visibility
- Add unit test for the price preference logic

**Files to Change:**
- `internal/services/portfolio/service.go` — lines 202-210

## Issue 2: Cash Flow Tracking & True Performance (fb_6337c9d1, MEDIUM)

**Problem:** Portfolio performance only reflects Navexa trading P&L. No visibility into actual capital flows (deposits, contributions, transfers), so the displayed net return doesn't reflect true return on capital deployed.

**Approach:** Add a CashTransaction model stored via UserDataStore. Use existing XIRR calculation (already in portfolio service) to compute annualised return on capital deployed. Separate from trading P&L — this measures total SMSF performance.

### New Model

```go
type CashTransaction struct {
    ID          string    `json:"id"`           // ct_ prefix + 8 hex chars
    Type        string    `json:"type"`         // deposit, withdrawal, transfer_in, transfer_out, dividend, contribution
    Date        time.Time `json:"date"`
    Amount      float64   `json:"amount"`       // always positive; Type determines direction
    Description string    `json:"description"`
    Category    string    `json:"category,omitempty"`
    Notes       string    `json:"notes,omitempty"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type CapitalPerformance struct {
    TotalDeposited      float64   `json:"total_deposited"`
    TotalWithdrawn      float64   `json:"total_withdrawn"`
    NetCapitalDeployed  float64   `json:"net_capital_deployed"`
    CurrentPortfolioValue float64 `json:"current_portfolio_value"` // equity + external balances
    SimpleReturnPct     float64   `json:"simple_return_pct"`       // (current - net) / net * 100
    AnnualizedReturnPct float64   `json:"annualized_return_pct"`   // XIRR
    FirstTransactionDate *time.Time `json:"first_transaction_date"`
    TransactionCount    int       `json:"transaction_count"`
}
```

### Storage
- UserDataStore with subject `"cashflow"`, key = portfolio name
- Value = JSON array of CashTransaction

### Service
- `CashFlowService` in `internal/services/cashflow/`
- Methods: `AddTransaction`, `ListTransactions`, `UpdateTransaction`, `RemoveTransaction`, `CalculatePerformance`
- `CalculatePerformance` loads transactions + portfolio total value (equity + external balances), runs XIRR

### API Endpoints
| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/portfolios/{name}/cashflows` | GET | List transactions + summary |
| `/api/portfolios/{name}/cashflows` | POST | Add transaction |
| `/api/portfolios/{name}/cashflows/{id}` | PUT | Update transaction |
| `/api/portfolios/{name}/cashflows/{id}` | DELETE | Remove transaction |
| `/api/portfolios/{name}/cashflows/performance` | GET | Capital performance metrics |

### MCP Tools
- `add_cash_transaction` — POST single transaction
- `list_cash_transactions` — GET all transactions for portfolio
- `update_cash_transaction` — PUT update by ID
- `remove_cash_transaction` — DELETE by ID
- `get_capital_performance` — GET performance metrics (XIRR, total capital in/out)

### Transaction Types
| Type | Direction | Description |
|------|-----------|-------------|
| `deposit` | inflow | Cash deposit into SMSF |
| `withdrawal` | outflow | Cash withdrawal from SMSF |
| `contribution` | inflow | Regular employer/member contribution |
| `transfer_in` | inflow | Transfer from external (e.g. Accumulate → Cash) |
| `transfer_out` | outflow | Transfer to external (e.g. Cash → Accumulate) |
| `dividend` | inflow | Dividend received into cash |

### XIRR Calculation
Reuse `CalculateXIRR` from `internal/services/portfolio/xirr.go`. Cash flows:
- Each deposit/contribution/transfer_in/dividend → negative XIRR cash flow (money going in)
- Each withdrawal/transfer_out → positive XIRR cash flow (money going out)
- Terminal: current portfolio value (equity + external balances) as positive at today's date

## Scope
- In scope: ACDC price fix, CashTransaction CRUD, capital performance calculation, MCP tools
- Out of scope: CSV import, portal UI, automatic dividend detection

## Files Expected to Change
- `internal/services/portfolio/service.go` — AdjClose fix
- `internal/models/cashflow.go` — new model
- `internal/services/cashflow/service.go` — new service
- `internal/services/cashflow/service_test.go` — unit tests
- `internal/interfaces/services.go` — CashFlowService interface
- `internal/app/app.go` — wire CashFlowService
- `internal/server/handlers.go` — cash flow handlers
- `internal/server/routes.go` — cash flow routes
- `internal/server/catalog.go` — MCP tool definitions
