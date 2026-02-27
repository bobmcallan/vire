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

**Capital Allocation Timeline**: `GetPortfolioIndicators` loads the cash flow ledger via `CashFlowService.GetLedger()` and passes transactions to `GetDailyGrowth()` via `GrowthOptions.Transactions`. In the date iteration loop, a cursor-based single pass merges date-sorted transactions into each `GrowthDataPoint`, computing `CashBalance` (running inflow minus outflow) and `NetDeployed` (cumulative deposits+contributions minus withdrawals). These propagate to `TimeSeriesPoint` with additional derived field `TotalCapital = Value + CashBalance`. All new `TimeSeriesPoint` fields use `omitempty` — absent when no cash transactions exist.

TimeSeriesPoint fields: `date`, `value` (holdings + external balances), `cost`, `net_return`, `net_return_pct`, `holding_count`, `cash_balance` (omitempty), `external_balance` (omitempty), `total_capital` (omitempty), `net_deployed` (omitempty).

### Historical Values and Net Flow

`SyncPortfolio` and `GetPortfolio` populate portfolio and per-holding historical values from EOD market data: portfolio-level `yesterday_total`, `yesterday_pct`, `last_week_total`, `last_week_pct` and per-holding `yesterday_close`, `yesterday_pct`, `last_week_close`, `last_week_pct`. Computed from EOD bars (index 1 for yesterday, offset 5 for ~5 trading days back). Gracefully handles missing market data (logs warning, fields remain zero).

`populateNetFlows()` adds `yesterday_net_flow` and `last_week_net_flow` to the Portfolio response: sums signed transaction amounts (inflows positive, outflows negative) within a 1-day and 7-day window respectively. Non-fatal: skipped when `CashFlowService` is nil or ledger is empty.

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

Transaction types: deposit, withdrawal, contribution, transfer_in, transfer_out, dividend. Inflows: deposit, contribution, transfer_in, dividend.

**Internal Transfer Detection**: `ExternalBalanceCategories` maps all external balance types ("cash", "accumulate", "term_deposit", "offset") to `true`. `CashTransaction.IsInternalTransfer()` returns true when `Type` is `transfer_in` or `transfer_out` AND `Category` matches an external balance type. Internal transfers are skipped in `CalculatePerformance` deposit/withdrawal sums and in `computeXIRR` flow construction — they represent rebalancing between portfolio cash and external accounts, not real capital flows. `FirstTransactionDate` still uses the earliest ledger entry (including internal transfers). `TransactionCount` reflects all ledger entries.

**Trade-Based Fallback**: When no manual cash transactions exist, `CalculatePerformance` attempts to auto-derive capital metrics from portfolio trade history via `deriveFromTrades()`. Sums buy/opening balance trades as total deposited (units × price + fees) and sell trades as total withdrawn (units × price - fees). Uses `TotalValueHoldings` only as terminal value. Computes simple return and XIRR from synthetic cash flows. Returns empty struct if no trades available (non-fatal). Manual transactions take precedence over trade-based fallback.

**Capital Timeline**: `GetDailyGrowth()` skips internal transfers in the cash balance cursor loop — transfer_in/transfer_out to external balance accounts do not affect `runningCashBalance` or `runningNetDeployed` in the timeline.

**ExternalBalance.AssetCategory()**: Returns `"cash"` for all external balance types. All four types (cash, accumulate, term_deposit, offset) are cash-equivalents for portfolio allocation logic.

Capital performance embedded in `get_portfolio` response (non-fatal errors swallowed).
