# Summary: External Balances for Portfolios

**Date:** 2026-02-23
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Added `ExternalBalance` struct (ID, Type, Label, Value, Rate, Notes) and `ValidateExternalBalanceType()`. Added `ExternalBalances` and `ExternalBalanceTotal` fields to `Portfolio`. |
| `internal/services/portfolio/external_balances.go` | New file: CRUD methods (`GetExternalBalances`, `SetExternalBalances`, `AddExternalBalance`, `RemoveExternalBalance`), validation, weight recomputation, ID generation (crypto/rand, eb_ prefix). |
| `internal/services/portfolio/external_balances_test.go` | Unit tests for validation, ID generation, total recomputation, weight recalculation, CRUD operations, sync preservation. |
| `internal/services/portfolio/service.go` | Weight calculation now uses `totalValue + ExternalBalanceTotal` as denominator. `SyncPortfolio` preserves external balances across re-syncs. |
| `internal/interfaces/services.go` | Added `GetExternalBalances`, `SetExternalBalances`, `AddExternalBalance`, `RemoveExternalBalance` to `PortfolioService` interface. |
| `internal/server/handlers.go` | Added `handleExternalBalances` (GET/PUT/POST dispatch) and `handleExternalBalanceDelete` (DELETE). |
| `internal/server/routes.go` | Registered `/api/portfolios/{name}/external-balances` and `.../external-balances/{id}` routes. |
| `internal/server/catalog.go` | Added 4 MCP tools: `get_external_balances`, `set_external_balances`, `add_external_balance`, `remove_external_balance`. |
| `internal/common/version.go` | Schema version bumped from "7" to "8". |
| `tests/api/external_balances_test.go` | 7 integration test functions (33 subtests): CRUD lifecycle, validation, not-found, delete non-existent, persistence across sync, weight recalculation, all types. |
| `README.md` | Documented external balance API endpoints and MCP tools. |
| `.claude/skills/develop/SKILL.md` | Updated Reference section with external balance model, endpoints, and schema version. |

## Tests
- Unit tests: `internal/services/portfolio/external_balances_test.go` — all pass
- Integration tests: `tests/api/external_balances_test.go` — 7 functions, 33 subtests, all pass
- Test results saved to `tests/logs/20260223-*-TestExternalBalance*/`
- Test feedback rounds: 1 (missing user headers in HTTP calls, fixed by implementer)
- Regression check: all existing unit tests pass (no regressions)

## Documentation Updated
- `README.md` — new API endpoints and MCP tools
- `.claude/skills/develop/SKILL.md` — updated Reference section

## Devils-Advocate Findings
- Handler error matching for validation errors uses `strings.Contains` on "external balance" — fragile but functional since all validation errors include that substring. Consistent with existing handler patterns.
- Input validation is thorough: type, label (non-empty, max 200), notes (max 1000), value (non-negative, finite, max 1e15), rate (non-negative, finite).
- ID generation uses `crypto/rand` — no collision risk.
- Auth: handlers inherit user context middleware from route group — no bypass.

## Notes
- `golangci-lint` not installed locally; `go vet` passed clean.
- Server deployed and health check confirmed (v0.3.59 on :8882).
- External balances are manually managed (not synced from Navexa).
- Future work: strategy cash floor rules, interest accrual, portal UI.
