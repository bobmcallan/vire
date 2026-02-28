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

### External Balances (`external_balances.go`)

Cash accounts, term deposits, accumulate, offset accounts. Stored on Portfolio model. `recomputeExternalBalanceTotal` sums values; `recomputeHoldingWeights` uses `totalMarketValue + ExternalBalanceTotal` as denominator.

SyncPortfolio preserves external balances across re-syncs via raw UserDataStore.Get.

### Indicators and Capital Allocation Timeline (`indicators.go`, `growth.go`)

Portfolio treated as single instrument. Computes EMA/RSI/SMA/trend on daily value time series. `growthToBars` converts GrowthDataPoint to EODBar adding external balance total. `GetPortfolioIndicators` exposes raw daily portfolio value time series via `TimeSeries` field (array of TimeSeriesPoint).

**Capital Allocation Timeline**: `GetPortfolioIndicators` loads the cash flow ledger via `CashFlowService.GetLedger()` and passes transactions to `GetDailyGrowth()` via `GrowthOptions.Transactions`. In the date iteration loop, a cursor-based single pass merges date-sorted transactions into each `GrowthDataPoint`, computing `CashBalance` (running credits minus debits across all transaction types) and `NetDeployed` (contributions credited, plus other/fee/transfer debits subtracted). These propagate to `TimeSeriesPoint` with additional derived field `TotalCapital = Value + CashBalance`. All new `TimeSeriesPoint` fields use `omitempty` — absent when no cash transactions exist.

TimeSeriesPoint fields: `date`, `value` (holdings + external balances), `cost`, `net_return`, `net_return_pct`, `holding_count`, `cash_balance` (omitempty), `external_balance` (omitempty), `total_capital` (omitempty), `net_deployed` (omitempty).

### Historical Values and Net Flow

`SyncPortfolio` and `GetPortfolio` populate portfolio and per-holding historical values from EOD market data: portfolio-level `yesterday_total`, `yesterday_pct`, `last_week_total`, `last_week_pct` and per-holding `yesterday_close`, `yesterday_pct`, `last_week_close`, `last_week_pct`. Computed from EOD bars (index 1 for yesterday, offset 5 for ~5 trading days back). Gracefully handles missing market data (logs warning, fields remain zero).

`populateNetFlows()` adds `yesterday_net_flow` and `last_week_net_flow` to the Portfolio response: sums signed transaction amounts (credits positive, debits negative) within a 1-day and 7-day window respectively. Dividends excluded (investment returns, not capital movements). Non-fatal: skipped when `CashFlowService` is nil or ledger is empty.

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

**Account-Based Model**: Each transaction has a `Direction` (credit/debit) and `Category` (contribution, dividend, transfer, fee, other) against a named `Account`. All transactions — including transfers — are treated as real cash flows: credits count as deposits, debits count as withdrawals. A transfer from Trading to Accumulate is a debit on Trading and a credit on Accumulate; both affect their respective account balances and the total deposited/withdrawn tallies. Paired transfer entries are linked via `LinkedID`.

**CalculatePerformance**: Sums all credits as `TotalDeposited` and all debits as `TotalWithdrawn`. For non-transactional accounts receiving transfers, per-account `ExternalBalancePerformance` is tracked (TotalOut/TotalIn/NetTransferred/GainLoss) alongside current external balance values. Dividends are included in flows. `FirstTransactionDate` uses the earliest ledger entry. `TransactionCount` reflects all ledger entries.

**Trade-Based Fallback**: When no manual cash transactions exist, `CalculatePerformance` attempts to auto-derive capital metrics from portfolio trade history via `deriveFromTrades()`. Sums buy/opening balance trades as total deposited (units × price + fees) and sell trades as total withdrawn (units × price - fees). Uses `TotalValueHoldings` only as terminal value. Computes simple return and XIRR from synthetic cash flows. Returns empty struct if no trades available (non-fatal). Manual transactions take precedence over trade-based fallback.

**Capital Timeline**: `GetDailyGrowth()` processes all transactions (including transfers) in the cash balance cursor loop. Every credit increases `runningCashBalance`; every debit decreases it. `runningNetDeployed` tracks contributions (credit) and debits under other/fee/transfer categories. Dividends are excluded from net deployed.

**ExternalBalance.AssetCategory()**: Returns `"cash"` for all external balance types. All four types (cash, accumulate, term_deposit, offset) are cash-equivalents for portfolio allocation logic.

Capital performance embedded in `get_portfolio` response (non-fatal errors swallowed).

## Glossary Endpoint

`internal/server/glossary.go` — no dedicated service layer.

`GET /api/portfolios/{portfolio_name}/glossary` returns an active glossary of portfolio terms with live computed values and examples. Handler loads data from three existing sources (all non-fatal beyond the portfolio itself):

1. `PortfolioService.GetPortfolio` — required; returns 404 if missing
2. `CashFlowService.CalculatePerformance` — optional; capital performance categories only shown when `TransactionCount > 0`
3. `PortfolioService.GetPortfolioIndicators` — optional; indicator category only shown when `DataPoints > 0`

Categories conditionally included: Portfolio Valuation (always), Holding Metrics (always), Capital Performance (when cash transactions exist), External Balance Performance (when external balances in capital perf), Technical Indicators (when indicators available), Growth Metrics (when yesterday/last-week data populated).

`buildGlossary()` is a pure function that accepts portfolio, capital performance, and indicators — testable without HTTP machinery. Helper functions (`fmtMoney`, `fmtHoldingCalc`, etc.) are file-local to `glossary.go`. Top 3 holdings by weight are used for per-holding examples.
