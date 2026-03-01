# Requirements: Portfolio & Cash Field Naming Refactor

**Source Document**: `docs/refactor-portfolio-field-naming.md`
**Breaking Changes**: Yes — no backward compat required
**Scope**: 73 field renames + 4 endpoint consolidations across all Go source

---

## Strategy

This is a massive mechanical refactor. The safest approach:

1. **Rename all struct fields and JSON tags in models** — this intentionally breaks the build
2. **Fix all compilation errors** — `go build ./...` will surface every reference
3. **Endpoint consolidation** — remove/merge endpoints per the refactor doc
4. **Update MCP catalog descriptions** — tool descriptions reference field names
5. **Update glossary** — term names, labels, formulas, examples
6. **Update all tests** — both unit and API integration tests
7. **Build + vet + verify**

---

## Part 1: Model Field Renames

### File: `internal/models/portfolio.go`

#### Portfolio struct (17 renames)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 48 | `TotalValueHoldings` | `total_value_holdings` | `EquityValue` | `equity_value` |
| 49 | `TotalValue` | `total_value` | `PortfolioValue` | `portfolio_value` |
| 50 | `TotalCost` | `total_cost` | `NetEquityCost` | `net_equity_cost` |
| 51 | `TotalNetReturn` | `total_net_return` | `NetEquityReturn` | `net_equity_return` |
| 52 | `TotalNetReturnPct` | `total_net_return_pct` | `NetEquityReturnPct` | `net_equity_return_pct` |
| 55 | `TotalRealizedNetReturn` | `total_realized_net_return` | `RealizedEquityReturn` | `realized_equity_return` |
| 56 | `TotalUnrealizedNetReturn` | `total_unrealized_net_return` | `UnrealizedEquityReturn` | `unrealized_equity_return` |
| 59 | `TotalCash` | `total_cash` | `GrossCashBalance` | `gross_cash_balance` |
| 60 | `AvailableCash` | `available_cash` | `NetCashBalance` | `net_cash_balance` |
| 61 | `CapitalGain` | `capital_gain` | `NetCapitalReturn` | `net_capital_return` |
| 62 | `CapitalGainPct` | `capital_gain_pct` | `NetCapitalReturnPct` | `net_capital_return_pct` |
| 69 | `YesterdayTotal` | `yesterday_total` | `PortfolioYesterdayValue` | `portfolio_yesterday_value` |
| 70 | `YesterdayTotalPct` | `yesterday_total_pct` | `PortfolioYesterdayChangePct` | `portfolio_yesterday_change_pct` |
| 71 | `LastWeekTotal` | `last_week_total` | `PortfolioLastWeekValue` | `portfolio_last_week_value` |
| 72 | `LastWeekTotalPct` | `last_week_total_pct` | `PortfolioLastWeekChangePct` | `portfolio_last_week_change_pct` |
| 75 | `YesterdayNetFlow` | `yesterday_net_flow` | `NetCashYesterdayFlow` | `net_cash_yesterday_flow` |
| 76 | `LastWeekNetFlow` | `last_week_net_flow` | `NetCashLastWeekFlow` | `net_cash_last_week_flow` |

Also update the comments on each field to match the new naming.

#### Holding struct (13 renames)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 90 | `Weight` | `weight` | `PortfolioWeightPct` | `portfolio_weight_pct` |
| 91 | `TotalCost` | `total_cost` | `CostBasis` | `cost_basis` |
| 92 | `TotalInvested` | `total_invested` | `GrossInvested` | `gross_invested` |
| 93 | `TotalProceeds` | `total_proceeds` | `GrossProceeds` | `gross_proceeds` |
| 94 | `RealizedNetReturn` | `realized_net_return` | `RealizedReturn` | `realized_return` |
| 95 | `UnrealizedNetReturn` | `unrealized_net_return` | `UnrealizedReturn` | `unrealized_return` |
| 97 | `CapitalGainPct` | `capital_gain_pct` | `AnnualizedCapitalReturnPct` | `annualized_capital_return_pct` |
| 98 | `NetReturnPctIRR` | `net_return_pct_irr` | `AnnualizedTotalReturnPct` | `annualized_total_return_pct` |
| 99 | `NetReturnPctTWRR` | `net_return_pct_twrr` | `TimeWeightedReturnPct` | `time_weighted_return_pct` |
| 111 | `YesterdayClose` | `yesterday_close` | `YesterdayClosePrice` | `yesterday_close_price` |
| 112 | `YesterdayPct` | `yesterday_pct` | `YesterdayPriceChangePct` | `yesterday_price_change_pct` |
| 113 | `LastWeekClose` | `last_week_close` | `LastWeekClosePrice` | `last_week_close_price` |
| 114 | `LastWeekPct` | `last_week_pct` | `LastWeekPriceChangePct` | `last_week_price_change_pct` |

#### PortfolioReview struct (6 renames)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 127 | `TotalValue` | `total_value` | `PortfolioValue` | `portfolio_value` |
| 128 | `TotalCost` | `total_cost` | `NetEquityCost` | `net_equity_cost` |
| 129 | `TotalNetReturn` | `total_net_return` | `NetEquityReturn` | `net_equity_return` |
| 130 | `TotalNetReturnPct` | `total_net_return_pct` | `NetEquityReturnPct` | `net_equity_return_pct` |
| 131 | `DayChange` | `day_change` | `PortfolioDayChange` | `portfolio_day_change` |
| 132 | `DayChangePct` | `day_change_pct` | `PortfolioDayChangePct` | `portfolio_day_change_pct` |

#### PortfolioSnapshot struct (4 renames, internal - no JSON)

| Line | Old Go Field | New Go Field |
|------|-------------|-------------|
| 191 | `TotalValue` | `EquityValue` |
| 192 | `TotalCost` | `NetEquityCost` |
| 193 | `TotalNetReturn` | `NetEquityReturn` |
| 194 | `TotalNetReturnPct` | `NetEquityReturnPct` |

#### SnapshotHolding struct (1 rename, internal - no JSON)

| Line | Old Go Field | New Go Field |
|------|-------------|-------------|
| 200 | `TotalCost` | `CostBasis` |

Note: SnapshotHolding also has `Weight` field — but it's not part of the Holding struct (different struct). Keep `Weight` in SnapshotHolding as-is since it doesn't have JSON tags and is only used internally in snapshot.go.

#### GrowthDataPoint struct (7 renames, internal - no JSON)

| Line | Old Go Field | New Go Field |
|------|-------------|-------------|
| 210 | `TotalValue` | `EquityValue` |
| 211 | `TotalCost` | `NetEquityCost` |
| 212 | `NetReturn` | `NetEquityReturn` |
| 213 | `NetReturnPct` | `NetEquityReturnPct` |
| 215 | `CashBalance` | `GrossCashBalance` |
| 216 | `TotalCapital` | `PortfolioValue` |
| 217 | `NetDeployed` | `NetCapitalDeployed` |

#### TimeSeriesPoint struct (7 renames)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 223 | `TotalValue` | `total_value` | `EquityValue` | `equity_value` |
| 224 | `TotalCost` | `total_cost` | `NetEquityCost` | `net_equity_cost` |
| 225 | `NetReturn` | `net_return` | `NetEquityReturn` | `net_equity_return` |
| 226 | `NetReturnPct` | `net_return_pct` | `NetEquityReturnPct` | `net_equity_return_pct` |
| 228 | `TotalCash` | `total_cash` | `GrossCashBalance` | `gross_cash_balance` |
| 229 | `AvailableCash` | `available_cash` | `NetCashBalance` | `net_cash_balance` |
| 230 | `TotalCapital` | `total_capital` | `PortfolioValue` | `portfolio_value` |

#### PortfolioIndicators struct (1 rename)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 238 | `CurrentValue` | `current_value` | `PortfolioValue` | `portfolio_value` |

### File: `internal/models/cashflow.go`

#### CashFlowSummary struct (3 renames)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 105 | `TotalCash` | `total_cash` | `GrossCashBalance` | `gross_cash_balance` |
| 106 | `TotalCashByCurrency` | `total_cash_by_currency` | `GrossCashBalanceByCurrency` | `gross_cash_balance_by_currency` |
| 108 | `ByCategory` | `by_category` | `NetCashByCategory` | `net_cash_by_category` |

#### CapitalPerformance struct (5 renames)

| Line | Old Go Field | Old JSON Tag | New Go Field | New JSON Tag |
|------|-------------|-------------|-------------|-------------|
| 273 | `TotalDeposited` | `total_deposited` | `GrossCapitalDeposited` | `gross_capital_deposited` |
| 274 | `TotalWithdrawn` | `total_withdrawn` | `GrossCapitalWithdrawn` | `gross_capital_withdrawn` |
| 276 | `CurrentPortfolioValue` | `current_portfolio_value` | `EquityValue` | `equity_value` |
| 277 | `SimpleReturnPct` | `simple_return_pct` | `SimpleCapitalReturnPct` | `simple_capital_return_pct` |
| 278 | `AnnualizedReturnPct` | `annualized_return_pct` | `AnnualizedCapitalReturnPct` | `annualized_capital_return_pct` |

---

## Part 2: Fix All Service Layer References

After renaming struct fields, fix every reference. Use `go build ./...` to find them all.

### File: `internal/services/portfolio/service.go`

Key locations:
- **SyncPortfolio method** (~line 385-446): All Portfolio field assignments
  - `p.TotalValueHoldings` → `p.EquityValue`
  - `p.TotalValue` → `p.PortfolioValue`
  - `p.TotalCost` → `p.NetEquityCost`
  - `p.TotalCash` → `p.GrossCashBalance`
  - `p.AvailableCash` → `p.NetCashBalance`
  - `p.TotalNetReturn` → `p.NetEquityReturn`
  - `p.TotalNetReturnPct` → `p.NetEquityReturnPct`
  - `p.TotalRealizedNetReturn` → `p.RealizedEquityReturn`
  - `p.TotalUnrealizedNetReturn` → `p.UnrealizedEquityReturn`
- **Holding field assignments** (~line 294-325):
  - `h.TotalCost` → `h.CostBasis`
  - `h.TotalInvested` → `h.GrossInvested`
  - `h.TotalProceeds` → `h.GrossProceeds`
  - `h.RealizedNetReturn` → `h.RealizedReturn`
  - `h.UnrealizedNetReturn` → `h.UnrealizedReturn`
  - `h.NetReturnPctIRR` → `h.AnnualizedTotalReturnPct`
  - `h.CapitalGainPct` → `h.AnnualizedCapitalReturnPct` (if set here)
- **Weight calculation** (~line 420): `holdings[i].Weight` → `holdings[i].PortfolioWeightPct`
- **populateHistoricalValues**: All yesterday/lastWeek field assignments

### File: `internal/services/portfolio/growth.go`

- **holdingGrowthState struct** (line 21): `TotalCost` field — this is an internal struct, keep as-is since it's only used within growth.go
- **GrowthDataPoint construction** (lines 230-240):
  - `TotalValue: totalValue` → `EquityValue: totalValue`
  - `TotalCost: totalCost` → `NetEquityCost: totalCost`
  - `NetReturn: gainLoss` → `NetEquityReturn: gainLoss`
  - `NetReturnPct: gainLossPct` → `NetEquityReturnPct: gainLossPct`
  - `CashBalance: runningCashBalance` → `GrossCashBalance: runningCashBalance`
  - `TotalCapital: totalValue + runningCashBalance` → `PortfolioValue: totalValue + runningCashBalance`
  - `NetDeployed: runningNetDeployed` → `NetCapitalDeployed: runningNetDeployed`
- **Outlier detection** (line 200): `points[len(points)-1].TotalValue` → `points[len(points)-1].EquityValue`

### File: `internal/services/portfolio/indicators.go`

- **GrowthPointsToTimeSeries** (lines 14-38): All field mappings
  - `p.TotalValue` → `p.EquityValue`
  - `p.CashBalance` → `p.GrossCashBalance`
  - `TotalValue: totalValue` → `EquityValue: totalValue` (in TimeSeriesPoint construction)
  - `TotalCost: p.TotalCost` → `NetEquityCost: p.NetEquityCost`
  - `NetReturn: p.NetReturn` → `NetEquityReturn: p.NetEquityReturn`
  - `NetReturnPct: p.NetReturnPct` → `NetEquityReturnPct: p.NetEquityReturnPct`
  - `TotalCash: totalCash` → `GrossCashBalance: totalCash`
  - `TotalCapital: totalValue + totalCash` → `PortfolioValue: totalValue + totalCash`
  - `AvailableCash: totalCash - p.TotalCost` → `NetCashBalance: totalCash - p.NetEquityCost`
  - `NetCapitalDeployed: p.NetDeployed` → `NetCapitalDeployed: p.NetCapitalDeployed`
- **growthToBars** (line 45): `p.TotalValue` → `p.EquityValue`
- **GetPortfolioIndicators** (lines 77-166):
  - Line 101: `portfolio.TotalValue` → `portfolio.PortfolioValue`
  - Line 107: `CurrentValue: portfolio.TotalValue` → `PortfolioValue: portfolio.PortfolioValue`
  - Line 122: `CurrentValue: portfolio.TotalValue` → `PortfolioValue: portfolio.PortfolioValue`
  - Lines 128, 132, 136, 152: `portfolio.TotalValue` → `portfolio.PortfolioValue`
  - Line 163: `ind.TimeSeries = timeSeries` — **REMOVE** (Part 3: endpoint consolidation)

### File: `internal/services/portfolio/snapshot.go`

- **SnapshotHolding construction** (line 141): `TotalCost: totalCost` → `CostBasis: totalCost`
- **Snapshot totals** (lines 148-161):
  - `snapshot.TotalValue` → `snapshot.EquityValue`
  - `snapshot.TotalCost` → `snapshot.NetEquityCost`
  - `snapshot.TotalNetReturn` → `snapshot.NetEquityReturn`
  - `snapshot.TotalNetReturnPct` → `snapshot.NetEquityReturnPct`
  - `snapshot.Holdings[i].Weight` — this is SnapshotHolding.Weight, keep as-is

### File: `internal/services/cashflow/service.go`

- **CalculatePerformance**: All CapitalPerformance field assignments
  - `.TotalDeposited` → `.GrossCapitalDeposited`
  - `.TotalWithdrawn` → `.GrossCapitalWithdrawn`
  - `.CurrentPortfolioValue` → `.EquityValue`
  - `.SimpleReturnPct` → `.SimpleCapitalReturnPct`
  - `.AnnualizedReturnPct` → `.AnnualizedCapitalReturnPct`
  - References to `portfolio.TotalValueHoldings` → `portfolio.EquityValue`
- **CashFlowLedger.Summary()**: CashFlowSummary construction
  - `.TotalCash` → `.GrossCashBalance`
  - `.TotalCashByCurrency` → `.GrossCashBalanceByCurrency`
  - `.ByCategory` → `.NetCashByCategory`

### File: `internal/services/report/formatter.go`

- All references to PortfolioReview fields:
  - `review.TotalValue` → `review.PortfolioValue`
  - `review.TotalCost` → `review.NetEquityCost`
  - `review.TotalNetReturn` → `review.NetEquityReturn`
  - `review.TotalNetReturnPct` → `review.NetEquityReturnPct`
  - `review.DayChange` → `review.PortfolioDayChange`
  - `review.DayChangePct` → `review.PortfolioDayChangePct`
- All references to Holding fields within review formatting

### File: `internal/services/strategy/compliance.go` & `rules.go`

- Any references to portfolio/holding fields used in compliance checks

---

## Part 3: Endpoint Consolidation

### 3a. Strip `time_series` from `get_portfolio_indicators`

**File: `internal/models/portfolio.go`**
- Remove `TimeSeries` field from `PortfolioIndicators` struct (line 261)

**File: `internal/services/portfolio/indicators.go`**
- Remove `ind.TimeSeries = timeSeries` (line 163)
- Remove the `timeSeries := GrowthPointsToTimeSeries(growth)` call (line 117) — BUT keep the function itself since it's used by the timeline handler

### 3b. Remove `get_capital_performance` endpoint

**File: `internal/server/handlers.go`**
- Remove `handleCashFlowPerformance` function (~lines 1909-1926)

**File: `internal/server/routes.go`**
- Remove the `sub == "performance"` case (lines 248-249) in `routePortfolios`

### 3c. Remove `get_cash_summary` + add `summary_only` param to `list_cash_transactions`

**File: `internal/server/handlers.go`**
- Remove `handleCashSummary` function (~lines 1792-1828)
- Modify `handleCashFlows` (~lines 1717-1790): check for `?summary_only=true` query param. When set, return the response with `Transactions` as nil/omitted (use `omitempty` on the field or conditionally build the response)

**File: `internal/server/handlers.go`** (response struct)
- Update `cashFlowResponse` struct (~lines 1677-1687): add `omitempty` to Transactions field if not already present

**File: `internal/server/routes.go`**
- Remove `case "cash-summary"` (lines 239-240)

### 3d. Merge `strategy_scanner` + `stock_screen` → `screen_stocks`

**File: `internal/server/handlers.go`**
- Create new `handleScreenStocks` function that:
  1. Reads `mode` param (required: "fundamental" or "technical")
  2. If `mode == "fundamental"`: runs existing stock_screen logic (calls `s.app.MarketService.ScreenStocks`)
  3. If `mode == "technical"`: runs existing strategy_scanner logic (calls `s.app.MarketService.FindSnipeBuys`)
  4. All other params pass through unchanged
- Keep `handleScreen` and `handleScreenSnipe` functions for now (the new handler delegates to the same service methods)
- Actually simpler: just create `handleScreenStocks` that reads mode and dispatches to the appropriate existing handler internally

**File: `internal/server/routes.go`**
- Remove `mux.HandleFunc("/api/screen/snipe", s.handleScreenSnipe)` (line 94)
- Remove `mux.HandleFunc("/api/screen", s.handleScreen)` (line 96)
- Add `mux.HandleFunc("/api/screen/stocks", s.handleScreenStocks)`
- Keep `/api/screen/funnel` as-is (not part of consolidation)

### 3e. Rename `get_capital_timeline` → `get_portfolio_timeline`

**File: `internal/server/routes.go`**
- Change `case "history":` → `case "timeline":` (line 221)
- Route changes from `/api/portfolios/{name}/history` → `/api/portfolios/{name}/timeline`

---

## Part 4: Handler Layer Updates

### File: `internal/server/handlers.go`

- **slimPortfolioReview struct** (lines 31-47): Rename all fields and JSON tags to match PortfolioReview renames
- **toSlimReview function** (lines 49-83): Update all field copies
- **handlePortfolioGet** (~lines 125-155): Update capital gain computation
  - `portfolio.TotalValue` → `portfolio.PortfolioValue`
  - `portfolio.CapitalGain` → `portfolio.NetCapitalReturn`
  - `portfolio.CapitalGainPct` → `portfolio.NetCapitalReturnPct`
- **cashFlowResponse struct** (~lines 1677-1687): Add `omitempty` to Transactions for summary_only support

---

## Part 5: Glossary Updates

### File: `internal/server/glossary.go`

Update ALL glossary terms — term names (the string keys), display labels, definitions, formulas, and live examples. This is a heavy file because every term string and every struct field reference changes.

Key changes:
- `term="total_value"` → `term="portfolio_value"`
- `term="total_cost"` → `term="net_equity_cost"`
- `term="net_return"` → `term="net_equity_return"`
- `term="net_return_pct"` → `term="net_equity_return_pct"`
- `term="total_cash"` → `term="gross_cash_balance"`
- `term="available_cash"` → `term="net_cash_balance"`
- `term="total_capital"` → keep as `portfolio_value` or merge with existing
- `term="capital_gain"` → `term="net_capital_return"`
- `term="capital_gain_pct"` → `term="net_capital_return_pct"`
- `term="weight"` → `term="portfolio_weight_pct"`
- `term="total_deposited"` → `term="gross_capital_deposited"`
- `term="total_withdrawn"` → `term="gross_capital_withdrawn"`
- `term="simple_return_pct"` → `term="simple_capital_return_pct"`
- `term="annualized_return_pct"` → `term="annualized_capital_return_pct"`
- All Example references to Go struct fields (e.g. `p.TotalValue` → `p.PortfolioValue`)
- All Example references to indicators fields (e.g. `ind.CurrentValue` → `ind.PortfolioValue`)

---

## Part 6: MCP Catalog Updates

### File: `internal/server/catalog.go`

1. **Rename `get_capital_timeline` → `get_portfolio_timeline`**: Update tool name and description
2. **Remove `get_capital_performance`** tool definition
3. **Remove `get_cash_summary`** tool definition
4. **Add `summary_only` param** to `list_cash_transactions` tool
5. **Remove `stock_screen`** tool definition
6. **Remove `strategy_scanner`** tool definition
7. **Add `screen_stocks`** tool definition with `mode` param
8. **Update `get_portfolio_indicators` description**: Remove time_series references
9. **Update ALL tool descriptions** that reference renamed fields:
   - `get_portfolio`: update field name references in description
   - `get_portfolio_stock`: update holding field references
   - `get_portfolio_timeline` (was capital_timeline): update field references
   - `get_portfolio_indicators`: remove time_series mention, update field refs
   - `portfolio_compliance`: update field references
   - `get_cash_summary` references in `list_cash_transactions`
   - `get_capital_performance` references in `get_portfolio`

---

## Part 7: Test Updates

### Unit tests (`internal/services/portfolio/*_test.go`)

All files constructing Portfolio, Holding, or GrowthDataPoint structs need field name updates. Key files:
- `service_test.go` — heaviest, most Portfolio construction
- `fx_test.go`, `fx_stress_test.go` — FX conversion tests
- `currency_test.go`
- `indicators_test.go`, `indicators_stress_test.go`
- `capital_timeline_test.go`, `capital_timeline_stress_test.go`
- `capital_cash_fixes_stress_test.go`
- `growth_test.go`, `growth_internal_transfer_stress_test.go`
- `timeseries_stress_test.go`
- `snapshot_test.go`
- `portfolio_value_stress_test.go`
- `historical_values_stress_test.go`
- `review_portfolio_totalvalue_stress_test.go`
- `returns_refactor_test.go`

### Unit tests (`internal/services/cashflow/*_test.go`)

- `service_test.go` — CapitalPerformance assertions
- `capital_perf_stress_test.go`
- `capital_perf_category_stress_test.go`

### API integration tests (`tests/api/*_test.go`)

These reference JSON field names in string assertions. Key files:
- `portfolio_value_test.go` — main portfolio fields
- `portfolio_stock_test.go` — holding fields
- `portfolio_fx_test.go` — FX tests
- `portfolio_capital_test.go` — capital performance
- `portfolio_indicators_test.go` — indicators endpoint
- `portfolio_review_test.go` — review/compliance fields
- `portfolio_netflow_test.go` — net flow fields
- `external_balance_fixes_test.go`, `external_balances_test.go`
- `gainloss_test.go` — gain/loss fields
- `cash_summary_test.go` — **REMOVE or update** (endpoint being removed)
- `cashflow_test.go` — cashflow summary fields
- `capital_timeline_test.go` — timeline endpoint (route change)
- `history_endpoint_test.go` — timeline endpoint (route change)
- `capital_cash_fixes_test.go`
- `capital_perf_category_test.go` — **UPDATE** (endpoint being removed, references need to use get_portfolio instead)
- `glossary_test.go` — term name assertions
- `market_snipe_test.go` — screener endpoint (tool name changes)
- `portfolio_fixes_test.go`

### Data tests (`tests/data/*_test.go`)

- `gainloss_test.go`
- `portfolio_dataversion_test.go`
- `cashflow_test.go`

---

## Implementation Order

The implementer should follow this exact order:

1. **Models first**: Rename all struct fields and JSON tags in `portfolio.go` and `cashflow.go`
2. **Run `go build ./...`**: This will produce compilation errors for every reference
3. **Fix references file by file**, starting with:
   a. `services/portfolio/service.go`
   b. `services/portfolio/growth.go`
   c. `services/portfolio/indicators.go`
   d. `services/portfolio/snapshot.go`
   e. `services/cashflow/service.go`
   f. `services/report/formatter.go`
   g. `services/strategy/compliance.go` and `rules.go`
   h. `server/handlers.go`
   i. `server/glossary.go`
4. **Run `go build ./...`** — should compile with zero errors at this point (tests excluded)
5. **Endpoint consolidation** (Part 3):
   a. Strip time_series from indicators
   b. Remove capital_performance endpoint
   c. Remove cash_summary + add summary_only param
   d. Merge screeners → screen_stocks
   e. Rename history → timeline route
6. **Update catalog.go** (Part 6)
7. **Run `go build ./...`** again — verify clean
8. **Fix all test files** (Part 7) — use `go test ./... 2>&1 | head -100` iteratively
9. **Final verify**: `go build ./...`, `go vet ./...`, `go test ./internal/...`

---

## Important Notes

- **No backward compatibility**: Old field names are removed immediately. No deprecated aliases.
- **holdingGrowthState.TotalCost** in `growth.go` — this is an INTERNAL struct field, not part of the models. Keep as-is (it's not ambiguous in its local context).
- **SnapshotHolding.Weight** — keep as `Weight` since it's internal and not part of the Holding struct's rename scope.
- **Local variables** — local vars like `totalValue`, `totalCost`, `totalCash` in functions are fine as-is. Only struct field names and JSON tags change.
- **Navexa structs** — excluded from refactor (NavexaPortfolio, NavexaHolding, etc.)
