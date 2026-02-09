# Implementation Plan: Portfolio Performance Refactoring

## Part A: New `get_portfolio` MCP Tool

**Goal:** Lightweight tool returning cached portfolio holdings without signals, AI summary, or growth chart. Target <500ms.

### Files to change:

1. **`cmd/vire-mcp/tools.go`** — Add `createGetPortfolioTool()` function
   - Parameters: `portfolio_name` (optional, uses default)
   - Description: "FAST: Get current portfolio holdings — tickers, names, values, weights, and gains. No signals, charts, or AI analysis. Use this for quick portfolio lookups."

2. **`cmd/vire-mcp/handlers.go`** — Add `handleGetPortfolio()` function
   - Pattern: follows `handleGetPortfolioSnapshot` but simpler
   - Reads portfolio from `storage.PortfolioStorage().GetPortfolio(ctx, name)` (single file read)
   - Returns formatted markdown table with: Ticker, Name, Units, Avg Cost, Current Price, Market Value, Weight%, Gain/Loss, Gain/Loss%
   - Separates active vs closed positions (reuse `filterClosedPositions` from service.go — but since that's in portfolio package, we'll inline the filter logic or just check `h.Units > 0`)
   - Portfolio-level totals: TotalValue, TotalCost, TotalGain, TotalGainPct, LastSynced
   - No signals, no market data reads, no Gemini call, no growth computation

3. **`cmd/vire-mcp/formatters.go`** — Add `formatPortfolioHoldings(p *models.Portfolio) string`
   - Markdown table with holdings sorted by weight descending
   - Active holdings table, closed positions summary
   - Portfolio totals at top

4. **`cmd/vire-mcp/main.go`** — Register the new tool
   - Add: `mcpServer.AddTool(createGetPortfolioTool(), handleGetPortfolio(portfolioService, storageManager, defaultPortfolio, logger))`

5. **`cmd/vire-mcp/mocks_test.go`** — Add `getPortfolioFn` to `mockPortfolioService`
   - Update `GetPortfolio` method to use the fn if set

6. **`cmd/vire-mcp/harness_test.go`** — Register `get_portfolio` tool in harness (or register in individual test)

### Note on interface:
The `PortfolioService.GetPortfolio` method already exists in the interface and delegates to storage. The handler just needs `portfolioService.GetPortfolio(ctx, name)` — no new interface methods needed. However, looking at the handler pattern, we actually just need storage directly since `GetPortfolio` is a thin wrapper. Using `portfolioService` for consistency with the handler pattern.

---

## Part B: Performance Instrumentation

**Goal:** Add timing logs to identify bottlenecks precisely.

### Files to change:

1. **`cmd/vire-mcp/handlers.go`** — Add timing to `handlePortfolioReview`
   - Wrap the handler body with total timing: `start := time.Now()`
   - Time each phase:
     - `ReviewPortfolio` call (signals + market data + AI summary combined)
     - `formatPortfolioReview` (should be negligible)
     - Strategy load (should be negligible)
     - `GetDailyGrowth` call
     - `RenderGrowthChart` call
     - Chart save
   - Use `logger.Info().Dur("elapsed", time.Since(phaseStart)).Msg(...)` pattern
   - Log total at end: `logger.Info().Dur("total", time.Since(start)).Msg("portfolio_review complete")`

2. **`internal/services/portfolio/service.go`** — Add timing to `ReviewPortfolio`
   - Time: portfolio read, market data loop (per-ticker and total), signal computation (total), AI summary
   - Use same `logger.Info().Dur(...)` pattern

3. **`internal/services/portfolio/growth.go`** — Add timing to `GetDailyGrowth`
   - Time: portfolio load, bulk market data load, date iteration loop
   - This is the suspected main bottleneck

---

## Part C: Performance Fixes

### C1: `findClosingPriceAsOf` — Binary Search

**Current behavior:** Linear scan of EOD bars (descending by date). For N bars and D dates, this is O(N*D) per holding.

**Fix:** Binary search on the sorted-descending bars array. Bars are already sorted by date descending. Use `sort.Search` to find the first bar at or before the target date. This changes O(N) per lookup to O(log N).

**File:** `internal/services/portfolio/snapshot.go`
- Rewrite `findClosingPriceAsOf` to use binary search
- Bars are sorted descending (newest first), so we need to find the first index where `bar.Date <= target`
- Use `sort.Search` with appropriate comparison

### C2: `replayTradesAsOf` — Optimization

**Current behavior:** Called once per holding per day in `GetDailyGrowth`. Iterates all trades each time with string date comparison.

**Fix:** Pre-compute trade events sorted by date, then for each holding, use a running state that advances date-by-date instead of replaying from scratch. This is a bigger refactor — for now, the simpler fix is to move the cutoff string formatting outside the loop (already done per-call, not per-trade, so this is already efficient). The real fix is in the growth loop structure.

Actually, looking more carefully at `GetDailyGrowth`, `replayTradesAsOf` IS called once per holding per date. For 30 holdings x 1000 days x ~5 trades = 150,000 iterations. The string comparison is already O(1) per trade. The bigger issue is calling it per-day-per-holding instead of maintaining running state. But refactoring to incremental state is a larger change.

**Simpler approach:** Pre-sort trades by date and use a cursor-based approach within the daily growth loop. For each holding, maintain a cursor that advances as dates progress, accumulating units/cost. This avoids replaying all trades from scratch for each date.

**File:** `internal/services/portfolio/growth.go`
- Add a `tradeReplayState` struct that maintains running units/cost
- In the daily loop, advance the state forward instead of replaying from scratch
- This changes O(dates * trades_per_holding) to O(dates + trades_per_holding) per holding

### C3: Persist Computed Signals in `ReviewPortfolio`

**Current behavior:** In `ReviewPortfolio`, line 260-262, if signals aren't found in storage, they're computed but never saved.

**Fix:** After computing signals, save them to storage.

**File:** `internal/services/portfolio/service.go`
- After `tickerSignals = s.signalComputer.Compute(marketData)` on line 261, add:
  ```go
  if saveErr := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); saveErr != nil {
      s.logger.Warn().Err(saveErr).Str("ticker", ticker).Msg("Failed to persist computed signals")
  }
  ```

### C4: Parallelize Market Data Reads in `ReviewPortfolio`

**Current behavior:** Sequential loop over holdings, each calling `s.storage.MarketDataStorage().GetMarketData(ctx, ticker)`.

**Fix:** Use a batch read. The `MarketDataStorage` interface already has `GetMarketDataBatch`. Replace the sequential per-ticker reads with a single batch read, then index by ticker.

**File:** `internal/services/portfolio/service.go`
- Before the holding loop, collect all tickers and call `GetMarketDataBatch`
- Index results by ticker in a map
- In the loop, look up from the map instead of individual reads
- This replaces N file reads with 1 batch read (which itself is N reads but can be optimized at storage level, and removes per-iteration overhead)

### C5: Skip Gemini on Stale/Cached Reviews (Optional / Lower Priority)

The Gemini call takes ~3s. We could make it optional via a `skip_ai` parameter. However, this changes the tool's API contract. Instead, consider: if the same review data would produce the same summary, cache the summary. But this is complex to detect.

**Simpler approach:** Add `include_ai_summary` boolean parameter (default: true) to `portfolio_review` tool. When false, skip the Gemini call. This gives users control.

**Files:**
- `cmd/vire-mcp/tools.go` — Add `include_ai_summary` parameter to portfolio_review tool
- `cmd/vire-mcp/handlers.go` — Pass through to ReviewOptions
- `internal/interfaces/services.go` — Add `IncludeAISummary` to ReviewOptions (default true)
- `internal/services/portfolio/service.go` — Check option before calling Gemini

**Decision:** Skip this for now. It changes the tool API and the Gemini call is already optional (only runs if gemini client is configured). The other fixes target the more impactful bottleneck (GetDailyGrowth at ~5s).

---

## Summary of Files to Change

| File | Changes |
|------|---------|
| `cmd/vire-mcp/tools.go` | Add `createGetPortfolioTool()` |
| `cmd/vire-mcp/handlers.go` | Add `handleGetPortfolio()`, timing instrumentation in `handlePortfolioReview` |
| `cmd/vire-mcp/formatters.go` | Add `formatPortfolioHoldings()` |
| `cmd/vire-mcp/main.go` | Register `get_portfolio` tool |
| `cmd/vire-mcp/mocks_test.go` | Add `getPortfolioFn` to mock |
| `internal/services/portfolio/service.go` | Timing in `ReviewPortfolio`, persist computed signals, batch market data reads |
| `internal/services/portfolio/growth.go` | Timing in `GetDailyGrowth`, incremental trade replay |
| `internal/services/portfolio/snapshot.go` | Binary search in `findClosingPriceAsOf` |
| `internal/interfaces/services.go` | No changes needed (all methods already exist) |

## Test Plan

Tests will be added in Task #4 (separate task):
- Unit test: `handleGetPortfolio` returns correct holdings data
- Unit test: `handleGetPortfolio` with empty portfolio
- Unit test: `handleGetPortfolio` with no default portfolio
- Unit test: Binary search `findClosingPriceAsOf` matches linear scan
- Unit test: Incremental trade replay matches `replayTradesAsOf`
- Unit test: Timing logs are emitted (verify logger output)
- Integration test in `tests/api/portfolio_review_test.go`: actual `get_portfolio` tool call
