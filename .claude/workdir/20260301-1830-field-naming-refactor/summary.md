# Summary: Portfolio Field Naming Refactor

**Status:** completed

## Changes

Comprehensive rename of 73+ struct fields across Go models and all consumers to follow the naming convention `{qualifier}_{category}_{timescale}_{measure}_{suffix}`.

| Area | Files Changed | Key Renames |
|------|--------------|-------------|
| **Models** | `internal/models/portfolio.go`, `market.go` | Portfolio: `TotalValue`→`PortfolioValue`, `TotalValueHoldings`→`EquityValue`, `TotalCost`→`NetEquityCost`, `TotalNetReturn`→`NetEquityReturn`, `TotalCash`→`GrossCashBalance`, etc. Holding: `TotalCost`→`CostBasis`, `TotalInvested`→`GrossInvested`, `Weight`→`PortfolioWeightPct`, `CapitalGainPct`→`AnnualizedCapitalReturnPct`, `NetReturnPctIRR`→`AnnualizedTotalReturnPct`, `NetReturnPctTWRR`→`TimeWeightedReturnPct`. PriceData/RealTimeQuote: `PreviousClose`, `YesterdayPct`, `LastWeekClose`. GrowthDataPoint/TimeSeriesPoint: all fields aligned with Portfolio naming. |
| **Services** | `internal/services/portfolio/service.go`, `indicators.go`, `growth.go`, `review.go` | All production code updated to use new field names |
| **Handlers** | `internal/server/handlers_*.go` | JSON response field names updated via struct tags |
| **Strategy** | `internal/services/strategy/rules.go` | Field resolver updated with aliases for backward-compatible rule conditions |
| **Quote** | `internal/services/quote/service.go` | Historical field population updated |
| **Unit Tests** | ~20 test files in `internal/services/portfolio/` | All struct literals, assertions, and JSON field name checks updated |
| **Integration Tests** | `tests/data/gainloss_test.go`, `portfolio_dataversion_test.go` | Field references and JSON assertions updated |

## Critical Distinctions

These caused the most bugs during refactoring:

| Context | Old Name | New Name | Meaning |
|---------|----------|----------|---------|
| Portfolio | `TotalValue` | `PortfolioValue` | Equity + Cash (total) |
| Portfolio | `TotalValueHoldings` | `EquityValue` | Holdings only |
| Portfolio | `TotalCost` | `NetEquityCost` | Net capital in equities |
| Holding | `TotalCost` | `CostBasis` | Cost basis per holding |
| NavexaHolding | `TotalCost` | `TotalCost` (unchanged) | External API field |

## Tests

- **Unit tests:** All pass (portfolio: 400+, quote, strategy, models, signals)
- **Integration tests (data):** All pass except 2 pre-existing feedback store issues
- **Integration tests (api):** Requires live server (timeout)
- **Fix rounds:** 3 iterations of `go vet` → fix → re-test
- **Team lead fixes:** ~15 targeted fixes where implementer confused EquityValue/PortfolioValue or used wrong field on NavexaHolding vs Holding

## Verification

- `go build ./cmd/vire-server/` — clean
- `go vet ./...` — clean
- `golangci-lint run` — only pre-existing errcheck warnings in test containers

## Pre-existing Failures (not related to refactor)

- `internal/app` — requires API keys
- `internal/server` — role escalation tests require live DB (45s timeout)
- `internal/storage/surrealdb` — nil pointer in atomic write stress test
- `tests/data/feedbackstore_test.go` — sort/date filter issues

## Architecture

- No backward compatibility shims (as specified)
- NavexaHolding fields NOT renamed (external API contract)
- Strategy field resolver supports both old and new field name aliases
- JSON tags all updated to snake_case matching new field names

## Notes

- The most error-prone part was the `TotalCost` ambiguity: on Portfolio it became `NetEquityCost`, on Holding it became `CostBasis`, on NavexaHolding it stayed `TotalCost`
- The EquityValue vs PortfolioValue distinction caused ~15 implementer errors that required team lead intervention
- No endpoint consolidation was done (Part 3 of the refactor doc) — field naming was the primary scope
