# Summary: get_cash_summary Tool & Currency on CashAccount

**Status:** completed

## Feedback Items
| ID | Severity | Issue | Resolution |
|----|----------|-------|------------|
| fb_d2eb8712 | MEDIUM | get_summary double-counts holdings + cash | Fixed — ReviewPortfolio.TotalValue = liveTotal (no cash). Added get_cash_summary tool. |
| fb_b6e0e817 | MEDIUM | CashAccount missing currency field | Fixed — Currency field added, default "AUD", per-currency totals in summary |
| fb_98b90af1 | MEDIUM | Portal MCP tool catalog caching | Already resolved (portal-side fix) — out of scope |

## Changes
| File | Change |
|------|--------|
| internal/models/cashflow.go:36-40 | Added `Currency string` to CashAccount struct |
| internal/models/cashflow.go:44-47 | Added `Currency string` to CashAccountUpdate struct |
| internal/models/cashflow.go:102-106 | Added `TotalCashByCurrency map[string]float64` to CashFlowSummary |
| internal/models/cashflow.go:109-145 | Updated Summary() to compute per-currency totals |
| internal/services/cashflow/service.go (6 locations) | All auto-create accounts now set `Currency: "AUD"` |
| internal/services/cashflow/service.go:380-382 | UpdateAccount applies `currency` field updates |
| internal/services/cashflow/service.go:390 | Log line includes currency field |
| internal/server/handlers.go:1646-1650 | Added Currency to cashAccountWithBalance response struct |
| internal/server/handlers.go:1666-1676 | newCashFlowResponse includes currency with fallback |
| internal/server/handlers.go:1765-1808 | New handleCashSummary handler |
| internal/server/routes.go:238-239 | Added "cash-summary" route case |
| internal/server/catalog.go:368-376 | New get_cash_summary tool definition |
| internal/server/catalog.go:583 | Updated update_account description + added currency param |
| internal/server/catalog_test.go:17,22,143-144 | Updated catalog count 52 → 53 |
| internal/models/cashflow_test.go | 3 unit tests: PerCurrencyTotals, EmptyCurrencyDefaults, UnknownAccount |
| internal/services/cashflow/service_test.go | 2 unit tests: UpdateAccount_Currency, Currency_DefaultsToAUD |

## Tests
- Unit tests: ALL PASS (models, cashflow service, portfolio service, catalog)
- 14+ new unit/stress tests for currency and per-currency summary
- 6 integration tests created (cash_summary_test.go, 520 lines)
- Build: PASS, go vet: PASS
- Pre-existing failures: surrealdb FileStore nil ptr, concurrent map race (unrelated)

## Architecture
- Architecture review completed by architect — PASSED
- Separation of concerns: currency belongs in cashflow domain, summary calculation in CashFlowLedger model
- No legacy compatibility shims added

## Devils-Advocate
- 11 stress tests added to cashflow_test.go
- Key findings: empty currency fallback, unknown currency codes, mixed-sign balances, idempotent summary
- All edge cases pass

## Notes
- ReviewPortfolio.TotalValue now shows holdings-only (cash excluded from summary total)
- Portfolio.TotalValue remains holdings + cash (used for weight calculations, documented as such)
- No FX conversion — balances stored and reported in native account currency
- Portal will need updating to display currency on account balances
- get_cash_summary provides lightweight access to cash data without full transaction list
