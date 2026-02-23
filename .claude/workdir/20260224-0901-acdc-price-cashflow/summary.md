# Summary: ACDC Price Fix + Cash Flow Tracking

**Date:** 2026-02-24
**Status:** completed

## What Changed

### Part A: ACDC Price Fix (fb_52547389)

| File | Change |
|------|--------|
| `internal/services/portfolio/service.go` | Added `eodClosePrice()` helper that prefers `AdjClose` over `Close`, with Inf/NaN guard. Applied in price refresh loop. |
| `internal/services/portfolio/service_test.go` | Unit tests for `eodClosePrice` including edge cases (zero, Inf, NaN AdjClose) |

### Part B: Cash Flow Tracking (fb_6337c9d1)

| File | Change |
|------|--------|
| `internal/models/cashflow.go` | New `CashTransaction`, `CashFlowLedger`, `CapitalPerformance` models with validation helpers |
| `internal/models/cashflow_test.go` | Model validation tests |
| `internal/interfaces/services.go` | New `CashFlowService` interface |
| `internal/services/cashflow/service.go` | Full CRUD service + XIRR-based performance calculation |
| `internal/services/cashflow/service_test.go` | 40+ unit tests |
| `internal/services/cashflow/service_stress_test.go` | 30+ stress tests (injection, edge cases, DoS) |
| `internal/app/app.go` | Wired `CashFlowService` into App |
| `internal/server/handlers.go` | HTTP handlers for cashflow CRUD + performance endpoint |
| `internal/server/routes.go` | Routes for `/api/portfolios/{name}/cashflows` |
| `internal/server/catalog.go` | 5 new MCP tools registered |
| `tests/api/cashflow_test.go` | API integration tests (9 tests, 35+ subtests) |
| `tests/data/cashflow_test.go` | Data integration tests (10 tests) |

## New MCP Tools

| Tool | Method | Description |
|------|--------|-------------|
| `add_cash_transaction` | POST | Add a cash transaction to a portfolio |
| `list_cash_transactions` | GET | List all cash transactions for a portfolio |
| `update_cash_transaction` | PUT | Update a cash transaction by ID |
| `remove_cash_transaction` | DELETE | Remove a cash transaction by ID |
| `get_capital_performance` | GET | Calculate true return on capital deployed (XIRR) |

## Tests
- Unit tests: 40+ in `internal/services/cashflow/service_test.go`
- Stress tests: 30+ in `internal/services/cashflow/service_stress_test.go`
- API integration: 9 tests (35+ subtests) in `tests/api/cashflow_test.go`
- Data integration: 10 tests in `tests/data/cashflow_test.go`
- All tests pass after 3 feedback rounds (auth config, 404 handling)

## Documentation Updated
- README.md — new MCP tools added
- `.claude/skills/develop/SKILL.md` — CashFlowService reference added

## Devils-Advocate Findings
- Dead code in POST handler — fixed
- `eodClosePrice` accepted +Inf AdjClose — Inf/NaN guard added
- No max transaction count limit — mitigated by 1MB request body limit (acceptable)
- Read-modify-write race on ledger — consistent with existing patterns (acceptable)
- Cross-user isolation — verified correct

## Deployment
- Server deployed as v0.3.65 (build 20260224093922)
- All endpoints verified live
- Portfolio value now ~$428k (up from ~$330k with bad ACDC price)

## Notes
- XIRR calculation reuses Newton-Raphson solver from `portfolio/xirr.go` pattern
- Transaction types: deposit, withdrawal, contribution, transfer_in, transfer_out, dividend
- Transactions stored via UserDataStore with subject "cashflow"
- CSV import is out of scope — can be added later
