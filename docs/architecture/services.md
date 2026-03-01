# Services

## MarketService

Interface in `internal/interfaces/services.go`. Collection methods in `internal/services/market/collect.go`.

### Composite Methods

| Method | Scope | Used By |
|--------|-------|---------|
| `CollectMarketData` | Full: EOD + fundamentals + filings + news + AI | Job manager, manual |
| `CollectCoreMarketData` | Fast: EOD (bulk) + fundamentals only | GenerateReport, GenerateTickerReport |

### Individual Methods

| Method | Data | Used By |
|--------|------|---------|
| `CollectEOD` | EOD bars (incremental) + signals | Job manager (fallback) |
| `CollectBulkEOD` | Last-day EOD for all tickers on exchange | Job manager (collect_eod_bulk) |
| `CollectFundamentals` | Company fundamentals | Job manager |
| `CollectFilings` | ASX announcements | Job manager |
| `CollectNews` | News articles | Job manager |
| `CollectFilingSummaries` | AI filing summaries (Gemini) | Job manager |
| `CollectTimeline` | Structured company timeline | Job manager |
| `CollectNewsIntelligence` | AI news sentiment (Gemini) | Job manager |
| `ReadFiling` | Extract text from filing PDF | MCP tool |

### GetStockData

Serves filing summaries, timeline, quality assessment from cached MarketData. No Gemini calls. Quality assessment computed on demand if fundamentals exist. `force_refresh=true` triggers inline CollectCoreMarketData + background EnqueueSlowDataJobs, response includes advisory.

Handler applies a 90s context timeout before calling GetStockData and CollectCoreMarketData. GetStockData applies a 60s timeout on the CollectMarketData fallback (triggered when market data is missing from storage). These bounds account for multiple EODHD requests at 30s each.

### Filing Summaries

- Prompt versioning via SHA-256 hash (`FilingSummaryPromptHash`). Changed prompt triggers regeneration.
- Memory management: nil unused fields during processing, batch size 2, intermediate saves, runtime.GC() between batches.
- `FilingSummary` includes `financial_summary` and `performance_commentary`.

### QualityAssessment

Computed from fundamentals. 7 scored metrics (ROE, GrossMargin, FCFConversion, NetDebtToEBITDA, EarningsStability, RevenueGrowth, MarginTrend). Overall ratings: "High Quality" / "Quality" / "Average" / "Below Average" / "Speculative".

## Portfolio Service

`internal/services/portfolio/`

### Dependencies

Holds `interfaces.CashFlowService` via setter injection (`SetCashFlowService`). Setter is called in `app.go` after both services are constructed — necessary to break the mutual dependency (cashflow service also holds `interfaces.PortfolioService`). The nil guard in all cashflow-dependent methods makes them non-fatal when called before the setter is invoked.

### Account-Based Cash Balances

Non-transactional accounts (accumulate, term_deposit, offset) replace the former ExternalBalance struct. `CashAccount.Type` identifies the account type; `CashAccount.IsTransactional` controls whether Navexa trade settlements flow into the account. `SyncPortfolio` calls `ledger.TotalCashBalance()` to compute `TotalCash` from the cashflow ledger (sum of ALL account balances, not just non-transactional) — no raw UserDataStore.Get fallback needed. `recomputeHoldingWeights` uses `totalMarketValue + TotalCash` as the denominator for weight calculations.

### ReviewPortfolio TotalValue

`ReviewPortfolio.TotalValue` at `service.go:814` is set to `liveTotal` (sum of active holding market values) only — cash is NOT added. Cash data is available separately via `get_cash_summary`. This prevents double-counting when the cash ledger contains deposits that have already been deployed into holdings. `Portfolio.TotalValue` (from `GetPortfolio`) remains `totalValue + totalCash` as documented — it's used for weight calculations and explicitly covers holdings + external balances.

### Indicators and Capital Allocation Timeline (`indicators.go`, `growth.go`)

Portfolio treated as single instrument. Computes EMA/RSI/SMA/trend on daily value time series. `growthToBars` converts GrowthDataPoint to EODBar using `TotalValue` only (external balance no longer added). `GetPortfolioIndicators` exposes raw daily portfolio value time series via `TimeSeries` field (array of TimeSeriesPoint).

**Capital Allocation Timeline**: `GetPortfolioIndicators` loads the cash flow ledger via `CashFlowService.GetLedger()` and passes transactions to `GetDailyGrowth()` via `GrowthOptions.Transactions`. In the date iteration loop, a cursor-based single pass merges date-sorted transactions into each `GrowthDataPoint`, computing `CashBalance` (running credits minus debits across all transaction types) and `NetDeployed` (contributions credited, plus other/fee/transfer debits subtracted). These propagate to `TimeSeriesPoint` with additional derived field `TotalCapital = Value + CashBalance`. All new `TimeSeriesPoint` fields use `omitempty` — absent when no cash transactions exist.

TimeSeriesPoint fields: `date`, `value` (holdings value — `TotalValue`), `cost`, `net_return`, `net_return_pct`, `holding_count`, `cash_balance` (omitempty), `external_balance` (omitempty — deprecated, always 0), `total_capital` (omitempty — `value + cash_balance`), `net_deployed` (omitempty).

### Historical Values and Net Flow

`SyncPortfolio` and `GetPortfolio` populate portfolio and per-holding historical values from EOD market data: portfolio-level `yesterday_total`, `yesterday_pct`, `last_week_total`, `last_week_pct` and per-holding `yesterday_close`, `yesterday_pct`, `last_week_close`, `last_week_pct`. Computed from EOD bars (index 1 for yesterday, offset 5 for ~5 trading days back). Gracefully handles missing market data (logs warning, fields remain zero).

`populateNetFlows()` adds `yesterday_net_flow` and `last_week_net_flow` to the Portfolio response: delegates to `ledger.NetFlowForPeriod()` for 1-day and 7-day windows respectively. Dividends excluded (investment returns, not capital movements). Non-fatal: skipped when `CashFlowService` is nil or ledger is empty.

### Price Refresh

Prefers AdjClose over Close via `eodClosePrice()`. Divergence sanity check (50% threshold). Falls back to Close if AdjClose is zero, negative, Inf, NaN.

### Watchlist Review

Same signal/compliance pipeline as ReviewPortfolio but for watchlist tickers. No FX conversion or position weights. Passes nil holding to action/compliance checks.

## Signal Service

`internal/services/signal/service.go`

Overlays real-time quotes onto cached EOD bars before computing indicators. `overlayLiveQuote()` updates today's bar or prepends synthetic bar. Non-fatal: nil client or failed fetch uses cached data.

## Report Service

`internal/services/report/`

`GenerateReport`: Navexa sync → CollectCoreMarketData (fast path) → portfolio review → format → store to BadgerDB. `GenerateTickerReport`: single-ticker CollectCoreMarketData.

Report markdown wraps EODHD data under `## EODHD Market Analysis`. Non-EODHD sections at `##` level.

## Cash Flow Service

`internal/services/cashflow/service.go`

Uses UserDataStore subject "cashflow", key = portfolio name. Transactions sorted by date ascending. `CalculatePerformance` computes XIRR (Newton-Raphson with bisection fallback). Terminal value = `TotalValueHoldings` only (equity holdings — external balances excluded from investment return metrics).

**Signed Amounts Model**: Each transaction has a signed `Amount` (positive = money in / credit, negative = money out / debit) and a `Category` (contribution, dividend, transfer, fee, other) against a named `Account`. All transactions — including transfers — are treated as real cash flows. A transfer from Trading to Accumulate is `-amount` on Trading and `+amount` on Accumulate; both affect their respective account balances. Paired transfer entries are linked via `LinkedID`. There is no `Direction` field — sign is the sole indicator.

**CalculatePerformance**: Delegates to `ledger.TotalDeposited()` (sum of positive amounts where `category=contribution` only) and `ledger.TotalWithdrawn()` (sum of absolute values of negative amounts where `category=contribution` only). Dividends, transfer credits/debits, and fees do not count as deposits or withdrawals. `FirstTransactionDate` uses the earliest ledger entry. `TransactionCount` reflects all ledger entries. XIRR is computed from actual trade history via `computeXIRRFromTrades()` — not from cash transactions.

**Trade-Based Fallback**: When no manual cash transactions exist, `CalculatePerformance` attempts to auto-derive capital metrics from portfolio trade history via `deriveFromTrades()`. Sums buy/opening balance trades as total deposited (units × price + fees) and sell trades as total withdrawn (units × price - fees). Uses `TotalValueHoldings` only as terminal value. Computes simple return and XIRR from synthetic cash flows. Returns empty struct if no trades available (non-fatal). Manual transactions take precedence over trade-based fallback.

**Capital Timeline**: `GetDailyGrowth()` processes all transactions (including transfers) in the cash balance cursor loop. Uses `tx.SignedAmount()` to update `runningCashBalance` and `tx.NetDeployedImpact()` to update `runningNetDeployed`. No inline direction checks in the consumer — both methods are authoritative on the model.

**Separation of Concerns**: `CashTransaction` owns two calculation primitives: `SignedAmount()` (positive for credit, negative for debit — single source of truth for balance effects) and `NetDeployedImpact()` (all contributions affect net deployed — positive deposits increase it, negative withdrawals decrease it; other/fee/transfer debits decrease it; dividends are zero). `CashFlowLedger` owns all aggregate calculations: `TotalDeposited()` (contribution credits only), `TotalWithdrawn()` (contribution debits only), `TotalCashBalance()` (sum of all account balances), `NetFlowForPeriod(from, to, excludeCategories...)`, `FirstTransactionDate()`. Consumer code (`growth.go`, `portfolio/service.go`, `cashflow/service.go`) delegates entirely to these methods — no inline category or direction checks appear outside `models/cashflow.go` and `services/cashflow/service.go`.

**Account Type Semantics**: `CashAccount.Type` values: `"trading"` (default transactional account), `"accumulate"`, `"term_deposit"`, `"offset"`. All account balances (transactional and non-transactional) contribute to `TotalCash` via `ledger.TotalCashBalance()` — the portfolio field `TotalCash` reflects the total cash across all named accounts.

**Currency on CashAccount**: `CashAccount.Currency` (ISO 4217, default `"AUD"`) identifies the native currency of each named account. No FX conversion is performed — balances are stored and reported in native currency. All auto-create locations set `Currency: "AUD"`. `UpdateAccount` applies `CashAccountUpdate.Currency` when non-empty. `CashFlowSummary` exposes both `TotalCash` (aggregate, all currencies) and `TotalCashByCurrency map[string]float64` (per-currency breakdown). The per-currency total is derived from account-to-currency mapping in `CashFlowLedger.Summary()` — consumers read from `Summary()`, never reimplement the loop. Handler response (`cashAccountWithBalance`) includes `Currency` with `"AUD"` fallback for legacy accounts that predate the field.

**Bulk Replace — SetTransactions**: `PUT /api/portfolios/{name}/cash-transactions` replaces all ledger transactions atomically. Existing accounts are preserved; new account names referenced by incoming transactions are auto-created (type `"other"`, non-transactional, currency `"AUD"`). All incoming transactions are validated before any are written. IDs are always reassigned — client-supplied IDs are ignored. Follows the same bulk-replace contract as `set_portfolio_plan` and `set_portfolio_watchlist`. MCP tool: `set_cash_transactions`.

**Cash Summary Endpoint**: `GET /api/portfolios/{name}/cash-summary` (`handleCashSummary`) returns a lightweight response — per-account balances with currency, and `CashFlowSummary` including `TotalCashByCurrency`. No transaction list. MCP tool: `get_cash_summary`. Distinct from `list_cash_transactions` which returns the full ledger.

Capital performance embedded in `get_portfolio` response (non-fatal errors swallowed).

## Glossary Endpoint

`internal/server/glossary.go` — no dedicated service layer.

`GET /api/portfolios/{portfolio_name}/glossary` returns an active glossary of portfolio terms with live computed values and examples. Handler loads data from three existing sources (all non-fatal beyond the portfolio itself):

1. `PortfolioService.GetPortfolio` — required; returns 404 if missing
2. `CashFlowService.CalculatePerformance` — optional; capital performance categories only shown when `TransactionCount > 0`
3. `PortfolioService.GetPortfolioIndicators` — optional; indicator category only shown when `DataPoints > 0`

Categories conditionally included: Portfolio Valuation (always), Holding Metrics (always), Capital Performance (when cash transactions exist), External Balance Performance (when external balances in capital perf), Technical Indicators (when indicators available), Growth Metrics (when yesterday/last-week data populated).

`buildGlossary()` is a pure function that accepts portfolio, capital performance, and indicators — testable without HTTP machinery. Helper functions (`fmtMoney`, `fmtHoldingCalc`, etc.) are file-local to `glossary.go`. Top 3 holdings by weight are used for per-holding examples.
