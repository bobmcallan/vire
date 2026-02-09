# Vire MCP Performance & Architecture Plan

**Date:** 2026-02-10
**Status:** Draft
**Author:** Code review of current codebase

---

## Problem Statement

Vire MCP delivers slow, stale data. Portfolio reviews take 5-10 seconds cold, market scans take 35-65 seconds. Current prices are end-of-day only despite an active EODHD live subscription. No observability exists to diagnose or measure improvements. The development loop is unproductive — changes can't be validated against performance baselines.

## Architecture Principle

Clean separation between:

1. **Personal data** — portfolio holdings, trades, values (Navexa)
2. **Strategy & plan** — investment rules, watchlist, action items (local storage)
3. **Market data** — prices, fundamentals, signals, news (EODHD)
4. **Analysis** — reports, AI summaries, recommendations (computed layer)

Market data should be **fast and current**. Personal data should be **cached and consistent**. Analysis should be **lazy and composable**.

---

## Phase 1: Observability — Replace Zerolog with Arbor (COMPLETED 2026-02-10)

**Goal:** See what's slow before optimising blind. Replace zerolog with arbor to get correlation tracking, in-memory queryable logs, and multi-writer support — eliminating the need for a custom metrics layer.

**Status:** All sub-phases (1.1-1.6) completed and verified. Docker build passing, all tests green, MCP integration validated end-to-end.

### 1.1 Replace Zerolog with Arbor

Replace `github.com/rs/zerolog` with `github.com/ternarybob/arbor` across the codebase.

**Why arbor over zerolog:**

| Capability | Zerolog | Arbor | Impact on Vire |
|------------|---------|-------|----------------|
| Correlation IDs | No | `.WithCorrelationId(id)` | Trace slow MCP request through all layers |
| In-memory log store | No | `GetMemoryLogsForCorrelation(id)` | Diagnostics tool queries logs directly — no custom metrics.go |
| Multi-writer | Manual plumbing | Built-in (console + file + memory) | Simultaneous stderr + file + queryable store |
| Log streaming | No | `SetChannel("name")` | Future real-time diagnostics endpoint |
| Fluent API | `.Info().Str().Msg()` | `.Info().Str().Msg()` | 99% compatible — 212 of 214 log statements unchanged |

**API compatibility:** Arbor's `ILogEvent` interface provides identical methods: `Str`, `Int`, `Int64`, `Float64`, `Bool`, `Err`, `Dur`, `Msg`, `Msgf`. Only `.Time()` is missing (used in 2 places — convert to `.Str(key, t.Format(time.RFC3339))`).

**Migration steps:**

1. Update `go.mod` — add `github.com/ternarybob/arbor`, remove `github.com/rs/zerolog`
2. Rewrite `internal/common/logging.go` (82 lines) — create arbor logger with console + memory writers
3. Fix 2 `.Time()` call sites — convert to `.Str()` with formatted timestamp
4. All other log statements (212) work unchanged — same fluent API
5. Constructor injection pattern stays identical — `*common.Logger` wrapper type remains

**Files to modify:**
- `go.mod` — swap dependency
- `internal/common/logging.go` — rewrite factory functions to create arbor logger with console writer (stderr) + memory writer
- 2 files with `.Time()` calls — trivial conversion to `.Str()`

**Estimated effort:** 1-2 hours for the swap itself.

### 1.2 Add Correlation IDs to MCP Handlers

Each MCP tool handler creates a correlated logger for the request. All downstream service calls, API requests, and storage operations log under that correlation ID.

**Pattern:**
```go
func handlePortfolioReview(...) server.ToolHandlerFunc {
    return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        reqLogger := logger.WithCorrelationId(generateRequestID())
        // pass reqLogger to all service calls
    }
}
```

**Impact:** Every log entry from a single MCP request is traceable. "Why was this portfolio_review slow?" becomes a query, not a guess.

**Files to modify:**
- `cmd/vire-mcp/handlers.go` — add correlation ID creation at entry point of each handler, pass correlated logger downstream

**Estimated effort:** 1 hour.

### 1.3 Add `get_diagnostics` MCP Tool

New tool that reports runtime health and performance data by querying arbor's memory writer directly.

**Output includes:**
- Service uptime, version, config summary
- EODHD API: reachable (HEAD request), last call timestamp, avg response time
- Navexa API: reachable, last sync timestamp
- Gemini API: reachable, last call timestamp
- Cache stats per domain: portfolio (last sync, freshness), market data (ticker count, oldest/newest), signals (count, freshness), reports (count, last generated)
- Recent errors: `logger.GetMemoryLogsWithLimit(n)` filtered to error level
- Per-request timing: `logger.GetMemoryLogsForCorrelation(id)` shows full breakdown

**Implementation:**
- No custom `metrics.go` needed — arbor's memory writer *is* the metrics store
- Query arbor's memory writer for recent logs, errors, and per-correlation breakdowns
- Add lightweight health checks for external services (EODHD, Navexa, Gemini)
- Cache stats read from storage manager

**Files to modify:**
- `cmd/vire-mcp/handlers.go` — add `handleDiagnostics` handler
- `cmd/vire-mcp/main.go` — register `get_diagnostics` tool

### 1.4 Change Default Log Level

Change default from `warn` to `info`. The timing instrumentation already exists in portfolio_review but is invisible at warn level.

**Files to modify:**
- `cmd/vire-mcp/main.go` — change logger initialisation

### 1.5 Add Timing to All Handlers

Currently only `handlePortfolioReview` has phase timing. Add `handlerStart` / elapsed logging to every handler, especially:
- `handleStockScreen`
- `handleFunnelScreen`
- `handleMarketSnipe`
- `handleGetStockData`
- `handleCollectMarketData`
- `handleDetectSignals`

All timing entries are correlated (via 1.2) and queryable (via 1.1).

**Files to modify:**
- `cmd/vire-mcp/handlers.go` — add timing to all handler functions

### 1.6 Add API Call Logging to EODHD Client

Log every outbound request with method, path, duration, status code at `info` level. Correlated to the parent MCP request automatically via the logger instance passed from the handler.

**Files to modify:**
- `internal/clients/eodhd/client.go` — add logging to `get()` method

**Validation:** After Phase 1:
- `get_diagnostics` tool call shows full system health and recent request timings
- Any slow request can be diagnosed: `GetMemoryLogsForCorrelation(id)` returns the full trace
- Logs show per-request timing for every EODHD call with correlation to the parent tool call
- Cache hit/miss rates are visible in log output

---

## Phase 2: Live Prices

**Goal:** Use EODHD's real-time API for current prices. Keep EOD endpoint for historical bars.

### 2.1 Add Real-Time Price Method to EODHD Client

EODHD live endpoint: `GET /real-time/{ticker}`

Returns: open, high, low, close, volume, timestamp — intraday values.

**New method:**
```go
func (c *Client) GetRealTimeQuote(ctx context.Context, ticker string) (*models.RealTimeQuote, error)
```

**New model:**
```go
type RealTimeQuote struct {
    Open      float64
    High      float64
    Low       float64
    Close     float64   // current/last price
    Volume    int64
    Timestamp time.Time
}
```

**Files to modify:**
- `internal/models/market.go` — add `RealTimeQuote` struct
- `internal/clients/eodhd/client.go` — add `GetRealTimeQuote()` method
- `internal/interfaces/clients.go` — add to `EODHDClient` interface

### 2.2 Use Live Price in GetStockData

When `get_stock_data` is called, prefer real-time quote for current price over `EOD[0].Close`.

**Logic:**
1. Fetch real-time quote (fast, single call)
2. Use EOD bars for historical context (cached)
3. Return real-time price as `Price.Current`, EOD bars for history

**Files to modify:**
- `internal/services/market/service.go` — modify `GetStockData()` to call `GetRealTimeQuote()`

### 2.3 Use Live Price in Portfolio Review

During `ReviewPortfolio`, fetch real-time quotes for all holdings to show current values, not yesterday's close.

**Approach:** Batch fetch real-time quotes concurrently (Phase 3 prerequisite or sequential initially), update holding values before signal analysis.

**Files to modify:**
- `internal/services/portfolio/service.go` — add real-time price fetch in `ReviewPortfolio()`

### 2.4 Use Live Price in Plan Event Checks

`check_plan_status` evaluates event triggers (e.g., "buy if RSI < 30"). Use real-time price for the current price field in condition evaluation.

**Files to modify:**
- `internal/services/plan/service.go` — fetch real-time price for ticker conditions

**Validation:** `get_stock_data` for XAGUSD.FOREX returns intraday price. Portfolio review shows current market values. Plan triggers evaluate against live prices.

---

## Phase 3: Concurrency

**Goal:** Parallelise API calls and computation. Target 5x speedup on screening, 2x on portfolio review.

### 3.1 Concurrent Market Data Collection

`CollectMarketData` currently loops tickers sequentially. Refactor to use `errgroup` with a semaphore.

**Design:**
```go
g, ctx := errgroup.WithContext(ctx)
sem := make(chan struct{}, 5) // 5 concurrent workers

for _, ticker := range tickers {
    ticker := ticker
    g.Go(func() error {
        sem <- struct{}{}
        defer func() { <-sem }()
        return s.collectSingleTicker(ctx, ticker, includeNews, force)
    })
}
return g.Wait()
```

Worker count of 5 stays within EODHD's rate limits (10 req/sec) since each worker does 2-3 calls.

**Impact:**
- Portfolio review (10 holdings, cold): 10 sequential → 2 batches = ~2s (from ~5s)
- stock_screen (25 tickers): 25 sequential → 5 batches = ~5s (from ~25s)
- market_snipe (100 tickers): 100 sequential → 20 batches = ~20s (from ~50s)

**Files to modify:**
- `internal/services/market/service.go` — refactor `CollectMarketData()` to use errgroup
- `go.mod` — add `golang.org/x/sync` if not present (for errgroup)

### 3.2 Concurrent Signal Computation

Signal computation in `ReviewPortfolio` loops holdings sequentially. Each holding's signals are independent.

**Design:** Same errgroup pattern with mutex on shared result slice.

**Impact:** 20 holdings × 50ms = 1s → ~100ms

**Files to modify:**
- `internal/services/portfolio/service.go` — parallelise holdings loop in `ReviewPortfolio()`

### 3.3 Concurrent Gemini Calls in Screening

`stock_screen` and `market_snipe` call Gemini sequentially per candidate. Parallelise with errgroup (3 workers — Gemini rate limits are more aggressive).

**Impact:** 5 candidates × 4s = 20s → ~8s (limited by Gemini rate)

**Files to modify:**
- `internal/services/market/screen.go` — parallelise Gemini calls in `ScreenStocks()`, `FunnelScreen()`
- `internal/services/market/snipe.go` — parallelise Gemini calls in `FindSnipeBuys()`

### 3.4 Concurrent Storage Batch Reads

`GetMarketDataBatch` reads files sequentially. Minor gain but easy fix.

**Files to modify:**
- `internal/storage/file.go` — parallelise `GetMarketDataBatch()` with goroutines

**Validation (measured via Phase 1 diagnostics):**
- stock_screen: 35s → ~10s
- market_snipe: 65s → ~25s
- portfolio_review cold: 5s → ~2.5s

---

## Phase 4: EODHD Technical Indicators (Optional Optimisation)

**Goal:** Evaluate whether EODHD's `/technical/` endpoint reduces portfolio review latency by eliminating local signal computation.

### 4.1 Assessment

Currently Vire computes RSI, SMA(20/50/200), MACD, Bollinger Bands, PBAS, VLI, regime detection locally from EOD bars. This requires 200+ days of historical data per ticker.

EODHD's `/technical/{ticker}?function=rsi&period=14` returns pre-computed indicators. One call per indicator per ticker.

**Trade-offs:**

| Aspect | Local Computation | EODHD Technical API |
|--------|-------------------|---------------------|
| Speed | ~50ms per ticker (CPU) | ~200ms per ticker (HTTP) |
| Data dependency | Needs 200+ EOD bars cached | No bar cache needed for signals |
| Custom indicators | PBAS, VLI, regime (custom) | Not available via API |
| Flexibility | Full control over parameters | Limited to EODHD's defaults |
| API cost | Zero (local CPU) | Additional API calls |

**Conclusion:** Local computation is faster once bars are cached. EODHD technical API would only help if we wanted to avoid caching historical bars at all (e.g., for ad-hoc ticker lookups where we don't have cached data). Not recommended as a primary approach.

### 4.2 Hybrid Approach (If Implemented)

Use EODHD technical API only for `get_stock_data` on tickers not in the portfolio (no cached bars). Continue local computation for portfolio holdings (bars already cached).

**Files to modify:**
- `internal/services/market/service.go` — add fallback to EODHD technical API in `GetStockData()` when no cached EOD bars exist

---

## Phase 5: Clean Up Dead Code

### 5.1 Remove Unused GetTechnicals

`GetTechnicals()` in the EODHD client is defined but never called. Remove it.

**Files to modify:**
- `internal/clients/eodhd/client.go` — remove `GetTechnicals()` method
- `internal/interfaces/clients.go` — remove from `EODHDClient` interface
- `cmd/vire-mcp/mocks_test.go` — remove mock implementation

---

## Decision Matrix: EODHD MCP vs Vire Integrated

| Factor | EODHD MCP (Separate) | Vire Integrated (Current + Fixes) |
|--------|----------------------|-----------------------------------|
| **Live prices** | Yes (out of box) | Yes (Phase 2) |
| **Speed** | Fast (direct API) | Fast (Phase 3 concurrency) |
| **Signal computation** | Lost (no PBAS, VLI, regime) | Retained |
| **Screening pipeline** | Lost (no 3-stage funnel) | Retained |
| **Strategy compliance** | Lost (no position sizing checks) | Retained |
| **Plan triggers** | Lost (no event evaluation) | Retained |
| **Maintenance** | Zero (EODHD maintains) | You maintain |
| **Composability** | Claude orchestrates multi-tool | Single tool per operation |

**Recommendation:** Keep Vire integrated. Fix performance (Phases 1-3). Consider adding EODHD MCP as a **supplement** for ad-hoc market queries that don't need portfolio context — but don't replace Vire's analytical layer with it.

---

## Implementation Order

```
Phase 1: Observability (Arbor)  ✓ COMPLETED 2026-02-10
  1.1 Replace zerolog with arbor (swap dependency, rewrite logging.go) ✓
  1.2 Add correlation IDs to MCP handlers ✓
  1.3 Add get_diagnostics MCP tool (queries arbor memory writer) ✓
  1.4 Change default log level to info ✓
  1.5 Add timing to all handlers ✓
  1.6 Add API call logging to EODHD client ✓

Phase 2: Live Prices            ← Highest user-visible impact
  2.1 EODHD real-time client
  2.2 GetStockData integration
  2.3 Portfolio review integration
  2.4 Plan trigger integration

Phase 3: Concurrency            ← Biggest performance gain
  3.1 Market data collection
  3.2 Signal computation
  3.3 Gemini calls
  3.4 Storage batch reads

Phase 4: Technical API           ← Optional, assess after Phase 3
Phase 5: Clean up               ← Housekeeping
```

**Dependencies:** Phase 1 should complete before Phase 3 so improvements are measurable. Phase 2 and Phase 3 are independent and can be interleaved. Within Phase 1, steps 1.1 and 1.2 must complete before 1.3 (diagnostics tool depends on arbor memory writer and correlation IDs).

---

## Estimated Impact

| Operation | Current | After Phase 2+3 | Improvement |
|-----------|---------|-----------------|-------------|
| `get_stock_data` | 1-2s (EOD) | 200ms (live) | ~5-10x |
| `portfolio_review` (10 holdings) | 5-10s | 2-3s | ~3x |
| `stock_screen` | 35-40s | 8-12s | ~3-4x |
| `market_snipe` | 60-65s | 20-25s | ~3x |
| `funnel_screen` | 35-40s | 8-12s | ~3-4x |
| Price freshness | EOD (stale) | Real-time | Qualitative |
| Diagnosability | None | Full metrics | Qualitative |

---

## Resolved: Arbor Replaces Zerolog (and Custom Metrics)

**Decision:** Replace zerolog with arbor. Assessed in `docs/arbor-assessment.md`.

**Key finding:** Arbor's `ILogEvent` interface is 99% API-compatible with zerolog's fluent API. 212 of 214 log statements work unchanged. The 2 `.Time()` calls convert trivially to `.Str()`.

**What this eliminates:** The original Phase 1.1 called for a custom `internal/common/metrics.go` with thread-safe counters, ring buffers, and timing recorders. Arbor's memory writer provides this out of the box — `GetMemoryLogsForCorrelation(id)` returns all log entries for a request, `GetMemoryLogsWithLimit(n)` returns recent entries. No custom metrics infrastructure needed.

**Performance cost:** Arbor is ~500x slower per log entry than zerolog (~100μs vs ~100ns). With 214 log statements, worst-case overhead is 21ms — invisible against 5-35 second operations.

**Migration effort:** ~3-4 hours total (logging.go rewrite + 2 trivial fixes + correlation IDs + diagnostics tool wiring).
