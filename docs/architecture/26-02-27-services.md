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
| `CollectLivePrices` | Live OHLCV snapshots for all tickers on exchange | Job manager (collect_live_prices), live price scheduler |
| `CollectFundamentals` | Company fundamentals | Job manager |
| `CollectFilings` | ASX announcements | Job manager |
| `CollectNews` | News articles | Job manager |
| `CollectFilingSummaries` | AI filing summaries (Gemini) | Job manager |
| `CollectTimeline` | Structured company timeline | Job manager |
| `CollectNewsIntelligence` | AI news sentiment (Gemini) | Job manager |
| `ReadFiling` | Extract text from filing PDF | MCP tool |

### GetStockData

Serves filing summaries, timeline, quality assessment from cached MarketData. No Gemini calls. Quality assessment computed on demand if fundamentals exist. `force_refresh=true` triggers inline CollectCoreMarketData + background EnqueueSlowDataJobs, response includes advisory.

**Historical OHLC Candles** (feature fb_799b5844): When `include.Price=true` and MarketData.EOD exists, populates `StockData.Candles` with up to 200 historical EODBar entries (most recent first). Candles are omitted when Price is not requested. This enables candlestick pattern analysis without requiring separate endpoints.

Handler applies a 90s context timeout before calling GetStockData and CollectCoreMarketData. GetStockData applies a 60s timeout on the CollectMarketData fallback (triggered when market data is missing from storage). These bounds account for multiple EODHD requests at 30s each.

### Filing Summaries

- Prompt versioning via SHA-256 hash (`FilingSummaryPromptHash`). Changed prompt triggers regeneration.
- Memory management: nil unused fields during processing, batch size 2, intermediate saves, runtime.GC() between batches.
- `FilingSummary` includes `financial_summary` and `performance_commentary`.

### QualityAssessment

Computed from fundamentals. 7 scored metrics (ROE, GrossMargin, FCFConversion, NetDebtToEBITDA, EarningsStability, RevenueGrowth, MarginTrend). Overall ratings: "High Quality" / "Quality" / "Average" / "Below Average" / "Speculative".

### Live Price Collection (feature fb_fa72a550)

`CollectLivePrices(ctx, exchange)` fetches live OHLCV snapshots for all tickers on an exchange via `GetBulkRealTimeQuotes` (batches of 20). Stores ephemeral `LivePrice` (*RealTimeQuote) and `LivePriceUpdatedAt` on `MarketData`. Does NOT modify EOD bars or trigger signal recomputation. Updates `live_price_collected_at` on stock index per-ticker.

Scheduled via `startLivePriceScheduler` (15min interval, `FreshnessLivePrice`). Also enqueued on-demand by `handleStockDataRefresh` for affected exchanges. Watcher checks `LivePriceCollectedAt` staleness and enqueues `collect_live_prices` per exchange.

## EODHD Client

`internal/clients/eodhd/client.go` — HTTP client wrapping the EODHD API with error handling and date formatting.

### Methods

| Method | Endpoint | Response | Used By |
|--------|----------|----------|---------|
| `GetRealTimeQuote` | `/quote/{ticker}` | RealTimeQuote | MarketService (price overlay in GetStockData) |
| `GetEOD` | `/eod/{ticker}` | EODResponse (bars) | CollectEOD |
| `GetBulkEOD` | `/eod-bulk-last-day/{exchange}` | Map of ticker → EODBar | CollectBulkEOD (job manager) |
| `GetBulkRealTimeQuotes` | `/real-time/{ticker}?s=...` | Map of ticker → RealTimeQuote | CollectLivePrices (batch of 20) |
| `GetFundamentals` | `/fundamentals/{ticker}` | Fundamentals | CollectFundamentals |
| `GetTechnicals` | `/technical/{ticker}` | TechnicalResponse | Signal computer |
| `GetNews` | `/news/{ticker}` | NewsItem array | CollectNews |
| `GetExchangeSymbols` | `/exchange-symbol-list/{exchange}` | Symbol array | Admin tools, market scan |
| `ScreenStocks` | `/screener` | ScreenerResult array | ScreenStocks service |
| `GetDividends` | `/div/{ticker}` | DividendEvent array | Market data collection (feature fb_827739dd part b) |

**Dividend Endpoint** (feature fb_827739dd part b): `GetDividends(ctx, ticker, from, to)` returns historical dividend events from EODHD. Endpoint: `/div/{ticker}?from=YYYY-MM-DD&to=YYYY-MM-DD&fmt=json`. Response maps to `[]models.DividendEvent` with date parsing. Currently available for manual queries; integration with automatic dividend collection is deferred (out of scope).

## Portfolio Service

`internal/services/portfolio/`

### Dependencies

Holds `interfaces.CashFlowService` via setter injection (`SetCashFlowService`). Setter is called in `app.go` after both services are constructed — necessary to break the mutual dependency (cashflow service also holds `interfaces.PortfolioService`). The nil guard in all cashflow-dependent methods makes them non-fatal when called before the setter is invoked.

### Account-Based Cash Balances

Non-transactional accounts (accumulate, term_deposit, offset) replace the former ExternalBalance struct. `CashAccount.Type` identifies the account type; `CashAccount.IsTransactional` controls whether Navexa trade settlements flow into the account. `SyncPortfolio` calls `ledger.TotalCashBalance()` to compute `TotalCash` from the cashflow ledger (sum of ALL account balances, not just non-transactional) — no raw UserDataStore.Get fallback needed. `recomputeHoldingWeights` uses `totalMarketValue + TotalCash` as the denominator for weight calculations.

**Ledger Dividend Return** (feature fb_a89d4d22): `SyncPortfolio` also aggregates confirmed dividends from the cash flow ledger via `ledger.Summary().NetCashByCategory[string(models.CashCatDividend)]` and populates `Portfolio.LedgerDividendReturn`. This is distinct from `Portfolio.DividendReturn` (Navexa-calculated accruals on holdings). Portal uses both fields: `DividendReturn` for projected/accrued amounts, `LedgerDividendReturn` for actual received cash. The ledger access is guarded by `if s.cashflowSvc != nil` — safe when cash flow service is not yet initialized.

### ReviewPortfolio TotalValue

`ReviewPortfolio.PortfolioValue` at `service.go:814` is set to `liveTotal` (sum of active holding market values) only — cash is NOT added. Cash data is available separately via `list_cash_transactions?summary_only=true`. This prevents double-counting when the cash ledger contains deposits that have already been deployed into holdings. `Portfolio.PortfolioValue` (from `GetPortfolio`) is `equityValue + netCashBalance` — it's used for weight calculations and explicitly covers holdings + net available cash.

### Indicators and Capital Allocation Timeline (`indicators.go`, `growth.go`)

Portfolio treated as single instrument. Computes EMA/RSI/SMA/trend on daily value time series. `growthToBars` converts GrowthDataPoint to EODBar using `EquityValue` only. `GetPortfolioIndicators` returns indicators only (RSI, EMA, trend) without time_series data.

**Capital Allocation Timeline**: `GetDailyGrowth()` automatically loads the cash flow ledger via `CashFlowService.GetLedger()` when `GrowthOptions.Transactions` is nil, ensuring all callers (handlers, schedulers, internal) include cash in timeline computations. Explicit transactions can be provided via `GrowthOptions.Transactions` to override. In the date iteration loop, a cursor-based single pass merges date-sorted transactions into each `GrowthDataPoint`, computing `GrossCashBalance` (running credits minus debits across all transaction types) and `NetCapitalDeployed` (contributions credited, plus other/fee/transfer debits subtracted). These propagate to `TimeSeriesPoint` with additional derived field `PortfolioValue = EquityValue + GrossCashBalance`. All `TimeSeriesPoint` fields use `omitempty` — absent when no cash transactions exist.

TimeSeriesPoint fields: `date`, `equity_value` (holdings value), `net_equity_cost`, `net_equity_return`, `net_equity_return_pct`, `holding_count`, `gross_cash_balance` (omitempty), `net_cash_balance` (omitempty), `portfolio_value` (omitempty — `equity_value + gross_cash_balance`), `net_capital_deployed` (omitempty).

**Timeline Endpoint (`/api/portfolios/{name}/timeline`)** (renamed from `/history`): `handlePortfolioHistory` calls `GetDailyGrowth`, applies optional downsampling via `format` query param (daily=no-op, weekly=`DownsampleToWeekly`, monthly=`DownsampleToMonthly`, auto=weekly if >365 points then monthly if still >200), then converts to `TimeSeriesPoint` via `GrowthPointsToTimeSeries` (exported from `indicators.go`). Response: `{ portfolio, format, data_points: []TimeSeriesPoint, count }`. The `"growth"` field in `handlePortfolioReview` also uses `GrowthPointsToTimeSeries` for consistent snake_case output.

### Historical Values and Net Flow

`SyncPortfolio` and `GetPortfolio` populate portfolio and per-holding historical values. Portfolio-level aggregates (`portfolio_yesterday_value`, `portfolio_yesterday_change_pct`, `portfolio_last_week_value`, `portfolio_last_week_change_pct`) are sourced from persisted timeline snapshots first, falling back to EOD market data when no timeline data exists. Per-holding prices (`yesterday_close_price`, `yesterday_price_change_pct`, `last_week_close_price`, `last_week_price_change_pct`) always come from EOD bars. See `docs/architecture/26-03-02-portfolio-timeline-centralization.md` for the full timeline design.

`populateNetFlows()` adds `net_cash_yesterday_flow` and `net_cash_last_week_flow` to the Portfolio response: delegates to `ledger.NetFlowForPeriod()` for 1-day and 7-day windows respectively. Dividends excluded (investment returns, not capital movements). Non-fatal: skipped when `CashFlowService` is nil or ledger is empty.

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

Uses UserDataStore subject "cashflow", key = portfolio name. Transactions sorted by date ascending. `CalculatePerformance` computes XIRR (Newton-Raphson with bisection fallback). Terminal value = `EquityValue` only (equity holdings — external balances excluded from investment return metrics).

**Signed Amounts Model**: Each transaction has a signed `Amount` (positive = money in / credit, negative = money out / debit) and a `Category` (contribution, dividend, transfer, fee, other) against a named `Account`. All transactions — including transfers — are treated as real cash flows. A transfer from Trading to Accumulate is `-amount` on Trading and `+amount` on Accumulate; both affect their respective account balances. Paired transfer entries are linked via `LinkedID`. There is no `Direction` field — sign is the sole indicator.

**Dividend Attribution** (feature fb_827739dd part a): Optional `Ticker` field on `CashTransaction` enables dividend linking to holdings. When category=dividend, the ticker identifies which holding the dividend came from (e.g., "BHP.AU"). Field is informational and not validated — present on all categories but only meaningful for dividends. No aggregate logic performs ticker-based filtering; the field is for human reference and future dividend-matching features (out of scope).

**CalculatePerformance**: Delegates to `ledger.TotalDeposited()` (sum of positive amounts where `category=contribution` only) and `ledger.TotalWithdrawn()` (sum of absolute values of negative amounts where `category=contribution` only). Dividends, transfer credits/debits, and fees do not count as deposits or withdrawals. `FirstTransactionDate` uses the earliest ledger entry. `TransactionCount` reflects all ledger entries. XIRR is computed from actual trade history via `computeXIRRFromTrades()` — not from cash transactions.

**Trade-Based Fallback**: When no manual cash transactions exist, `CalculatePerformance` attempts to auto-derive capital metrics from portfolio trade history via `deriveFromTrades()`. Sums buy/opening balance trades as total deposited (units × price + fees) and sell trades as total withdrawn (units × price - fees). Uses `TotalValueHoldings` only as terminal value. Computes simple return and XIRR from synthetic cash flows. Returns empty struct if no trades available (non-fatal). Manual transactions take precedence over trade-based fallback.

**Capital Timeline**: `GetDailyGrowth()` processes all transactions (including transfers) in the cash balance cursor loop. Uses `tx.SignedAmount()` to update `runningCashBalance` and `tx.NetDeployedImpact()` to update `runningNetDeployed`. No inline direction checks in the consumer — both methods are authoritative on the model.

**Separation of Concerns**: `CashTransaction` owns two calculation primitives: `SignedAmount()` (positive for credit, negative for debit — single source of truth for balance effects) and `NetDeployedImpact()` (all contributions affect net deployed — positive deposits increase it, negative withdrawals decrease it; other/fee/transfer debits decrease it; dividends are zero). `CashFlowLedger` owns all aggregate calculations: `TotalDeposited()` (contribution credits only), `TotalWithdrawn()` (contribution debits only), `TotalCashBalance()` (sum of all account balances), `Summary()` (returns `NetCashByCategory` map for all category totals), `NetFlowForPeriod(from, to, excludeCategories...)`, `FirstTransactionDate()`. Consumer code (`growth.go`, `portfolio/service.go`, `cashflow/service.go`) delegates entirely to these methods — no inline category or direction checks appear outside `models/cashflow.go` and `services/cashflow/service.go`.

**Ledger Dividend Integration** (feature fb_a89d4d22): `PortfolioService.SyncPortfolio()` computes ledger dividend via `ledger.Summary().NetCashByCategory[string(models.CashCatDividend)]` — reads from the ledger's computed map, never iterates transactions directly. The `Summary()` method ensures all known categories exist with zero fallback, preventing nil-pointer panics. Portfolio populates `LedgerDividendReturn` from this delegate call — no category filtering in portfolio service code.

**Account Type Semantics**: `CashAccount.Type` values: `"trading"` (default transactional account), `"accumulate"`, `"term_deposit"`, `"offset"`. All account balances (transactional and non-transactional) contribute to `TotalCash` via `ledger.TotalCashBalance()` — the portfolio field `TotalCash` reflects the total cash across all named accounts.

**Currency on CashAccount**: `CashAccount.Currency` (ISO 4217, default `"AUD"`) identifies the native currency of each named account. No FX conversion is performed — balances are stored and reported in native currency. All auto-create locations set `Currency: "AUD"`. `UpdateAccount` applies `CashAccountUpdate.Currency` when non-empty. `CashFlowSummary` exposes both `TotalCash` (aggregate, all currencies) and `TotalCashByCurrency map[string]float64` (per-currency breakdown). The per-currency total is derived from account-to-currency mapping in `CashFlowLedger.Summary()` — consumers read from `Summary()`, never reimplement the loop. Handler response (`cashAccountWithBalance`) includes `Currency` with `"AUD"` fallback for legacy accounts that predate the field.

**Bulk Replace — SetTransactions**: `PUT /api/portfolios/{name}/cash-transactions` replaces all ledger transactions atomically. Existing accounts are preserved; new account names referenced by incoming transactions are auto-created (type `"other"`, non-transactional, currency `"AUD"`). All incoming transactions are validated before any are written. IDs are always reassigned — client-supplied IDs are ignored. Follows the same bulk-replace contract as `set_portfolio_plan` and `set_portfolio_watchlist`. MCP tool: `set_cash_transactions`.

**Cash Summary Endpoint**: `GET /api/portfolios/{name}/cash-summary` (`handleCashSummary`) returns a lightweight response — per-account balances with currency, and `CashFlowSummary` including `TotalCashByCurrency`. No transaction list. MCP tool: `get_cash_summary`. Distinct from `list_cash_transactions` which returns the full ledger.

Capital performance embedded in `get_portfolio` response (non-fatal errors swallowed).

## Trade Service

`internal/services/trade/service.go`

Uses UserDataStore subject "trades", key = portfolio name. Stores a full `TradeBook` document (array of all trades for the portfolio). Trades are sorted by date ascending on write. `TradeBook.TradesForTicker(ticker)` filters by exact case-sensitive ticker match. `TradeBook.UniqueTickers()` returns deduplicated ticker list.

**Source-Typed Portfolios** (schema version 14): `Portfolio.SourceType` determines how `GetPortfolio` assembles holdings. `SourceManual` → `assembleManualPortfolio` (derives holdings from trade history via `TradeService.DeriveHoldings()`). `SourceSnapshot` → `assembleSnapshotPortfolio` (reads `TradeBook.SnapshotPositions` directly). Empty/`SourceNavexa` → existing Navexa sync path. `CreatePortfolio` enforces `ValidPortfolioSourceTypes` (manual, snapshot, hybrid) — Navexa portfolios cannot be created manually.

**Position Derivation**: `DeriveHolding(trades, currentPrice)` uses running average cost method. On each buy: `runningCost += units * price + fees`, `runningUnits += units`. On each sell: `avgCostAtSell = runningCost / runningUnits`, `realizedPnL += proceeds - costOfSold`, `runningCost -= costOfSold`. Returns `DerivedHolding` with `Units`, `AvgCost`, `CostBasis`, `RealizedReturn`, `UnrealizedReturn`, `MarketValue`, `GrossInvested`, `GrossProceeds`, `TradeCount`.

**Separation of Concerns**: TradeService is the sole owner of trade logic. PortfolioService has a `tradeService interfaces.TradeService` field set via `SetTradeService()` and calls `DeriveHoldings()` / `GetTradeBook()` — it never reimplements trade or position logic. `DerivedHolding` is defined in `internal/models/trade.go` (not the service package) to avoid circular imports with the interface package.

**Validation**: `validateTrade()` enforces: non-empty trimmed ticker (≤20 chars), action must be buy/sell, units > 0 and finite and < 1e15, price ≥ 0 and finite and < 1e15, fees ≥ 0 and finite, date required, notes ≤ 5000 chars, source_ref ≤ 200 chars. Ticker trimmed before storage. Sell validation: units must not exceed current position. `UpdateTrade` re-validates all ticker positions after update to prevent negative positions.

**Snapshot Import**: `SnapshotPositions(ctx, name, positions, mode, sourceRef, snapshotDate)` supports `"replace"` (clears and replaces) and `"merge"` (updates matching tickers by ticker key, adds new, leaves unmatched). Used by snapshot-type portfolios to bulk-import external position data.

**HTTP Endpoints** (registered in `internal/server/routes.go`):
- `POST /api/portfolios` → `handlePortfolioCreate`
- `GET /api/portfolios/{name}/trades` → `handleTrades` (list)
- `POST /api/portfolios/{name}/trades` → `handleTrades` (add)
- `PUT /api/portfolios/{name}/trades/{id}` → `handleTradeItem` (update)
- `DELETE /api/portfolios/{name}/trades/{id}` → `handleTradeItem` (remove)
- `POST /api/portfolios/{name}/snapshot` → `handlePortfolioSnapshotImport`

**MCP Tools** (in `internal/server/catalog.go`): `portfolio_create`, `trade_add`, `trade_list`, `trade_update`, `trade_remove`, `portfolio_snapshot`.

## Glossary Endpoint

`internal/server/glossary.go` — no dedicated service layer.

`GET /api/portfolios/{portfolio_name}/glossary` returns an active glossary of portfolio terms with live computed values and examples. Handler loads data from three existing sources (all non-fatal beyond the portfolio itself):

1. `PortfolioService.GetPortfolio` — required; returns 404 if missing
2. `CashFlowService.CalculatePerformance` — optional; capital performance categories only shown when `TransactionCount > 0`
3. `PortfolioService.GetPortfolioIndicators` — optional; indicator category only shown when `DataPoints > 0`

Categories conditionally included: Portfolio Valuation (always), Holding Metrics (always), Capital Performance (when cash transactions exist), External Balance Performance (when external balances in capital perf), Technical Indicators (when indicators available), Growth Metrics (when yesterday/last-week data populated).

`buildGlossary()` is a pure function that accepts portfolio, capital performance, and indicators — testable without HTTP machinery. Helper functions (`fmtMoney`, `fmtHoldingCalc`, etc.) are file-local to `glossary.go`. Top 3 holdings by weight are used for per-holding examples.
