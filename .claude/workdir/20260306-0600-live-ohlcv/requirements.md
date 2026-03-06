# Live OHLCV Price Collection

**Feedback**: fb_fa72a550
**Scope**: Add bulk live price collection from EODHD `/real-time` API, scheduled every 15 minutes and available on-demand for portfolio holdings.

## Problem

Daily return shows stale prior-day move during market hours because Vire only calls EOD historical batch (~4:58am AEST). Today's timeline row has same close as yesterday, so daily return reflects the prior day's move not intraday movement.

## Solution

Add a new `CollectLivePrices` method on MarketService that fetches live OHLCV snapshots in bulk from EODHD's `/real-time` API using the `s=` multi-ticker parameter. This data is stored ephemerally on the MarketData record (NOT merged into EOD bars) and is used by consumers to overlay current prices during market hours.

### Architecture Decisions

1. **Ephemeral live data** — Live prices are stored in a new `LivePrice` field on `MarketData`, NOT appended to the EOD bar array. EOD bars remain the authoritative history. LivePrice is overwritten on each refresh.
2. **Bulk API** — Use EODHD `/real-time/{ticker}?s=ticker2,ticker3,...` to fetch up to 20 tickers per request (EODHD recommended batch size). Group tickers by exchange for the primary ticker in the URL path.
3. **New job type** — `collect_live_prices` is a special bulk job (like `collect_eod_bulk`) that runs for an exchange, not per-ticker.
4. **Scheduled** — The existing price scheduler (`startPriceScheduler` in `app/scheduler.go`) already runs on `FreshnessTodayBar` (1 hour). Add a separate live price scheduler that runs every 15 minutes (new `FreshnessLivePrice` constant).
5. **On-demand** — The `market_refresh_stock_data` handler already enqueues refresh jobs. Extend it to also enqueue a `collect_live_prices` job for the affected exchanges.
6. **No signal recomputation** — Live prices do NOT trigger signal recomputation (signals use EOD bars only).

---

## Scope

### In Scope
- New `LivePrice` field on `MarketData` model
- New `GetBulkRealTimeQuotes` method on EODHD client (batch via `s=` parameter)
- New `CollectLivePrices` method on MarketService
- New `collect_live_prices` job type + executor dispatch
- New live price scheduler (15-minute interval)
- Stock index timestamp field `live_price_collected_at`
- Freshness constant `FreshnessLivePrice = 15 * time.Minute`
- On-demand live refresh via existing refresh handler

### Out of Scope
- WebSocket push to portal (future)
- Intraday charting
- Changes to signal computation pipeline
- Changes to EOD bar storage or timeline snapshots

---

## Files to Change

### 1. `internal/models/market.go` — Add LivePrice field to MarketData

Add after line 61 (QualityAssessment field):

```go
// LivePrice holds the most recent intraday price snapshot from the EODHD real-time API.
// Ephemeral: overwritten on each live price collection cycle. NOT part of EOD history.
// Used by consumers to show current intraday movement during market hours.
LivePrice        *RealTimeQuote `json:"live_price,omitempty"`
LivePriceUpdatedAt time.Time    `json:"live_price_updated_at"`
```

### 2. `internal/clients/eodhd/client.go` — Add bulk real-time method

Add new method after `GetRealTimeQuote` (after line 297):

```go
// GetBulkRealTimeQuotes fetches live OHLCV snapshots for multiple tickers in a single
// API call using the EODHD ?s= parameter. The primary ticker goes in the URL path,
// additional tickers in the s= query parameter. Returns a map of ticker -> RealTimeQuote.
// Tickers that fail or are missing from the response are silently skipped.
// Max recommended batch size: 20 tickers per call.
func (c *Client) GetBulkRealTimeQuotes(ctx context.Context, tickers []string) (map[string]*models.RealTimeQuote, error) {
    // 1. If len(tickers) == 0, return empty map
    // 2. Primary ticker = tickers[0] in URL path: /real-time/{primary}
    // 3. Additional tickers = tickers[1:] joined with comma in s= param
    // 4. Response is a JSON array when s= is used (not a single object)
    // 5. Parse each element as realTimeResponse, convert to RealTimeQuote
    // 6. Key the map by resp.Code (EODHD returns the full ticker code)
    // 7. Rate limiter: 1 call to c.limiter.Wait (counts as 1 HTTP request)
    //    Note: EODHD charges per-ticker for API call count, but it's 1 HTTP request
}
```

**Pattern reference**: Follow `GetRealTimeQuote` at line 269-297 for response parsing. Follow `GetBulkEOD` for the multi-ticker response handling pattern.

**Important**: When `s=` is used, the EODHD API returns a JSON **array** of objects, not a single object. Handle both cases: single ticker returns an object, multiple tickers returns an array.

### 3. `internal/interfaces/clients.go` — Add to EODHDClient interface

Add after `GetRealTimeQuote` (after line 14):

```go
// GetBulkRealTimeQuotes fetches live OHLCV snapshots for multiple tickers in one call.
GetBulkRealTimeQuotes(ctx context.Context, tickers []string) (map[string]*models.RealTimeQuote, error)
```

### 4. `internal/services/market/collect.go` — Add CollectLivePrices method

Add new method at end of file:

```go
// CollectLivePrices fetches live OHLCV snapshots for all tickers in the stock index
// for the given exchange and stores them on the MarketData record's LivePrice field.
// Uses bulk real-time API (batches of 20). Does NOT modify EOD bars or trigger signals.
func (s *Service) CollectLivePrices(ctx context.Context, exchange string) error {
    // 1. List stock index entries for exchange (same pattern as CollectBulkEOD lines 24-34)
    // 2. Filter to tickers that have existing MarketData (no point collecting live for new tickers)
    // 3. Batch into groups of 20
    // 4. For each batch: call s.eodhd.GetBulkRealTimeQuotes(ctx, batch)
    // 5. For each result: load existing MarketData, set LivePrice + LivePriceUpdatedAt, save
    // 6. Update stock index timestamp "live_price_collected_at" per ticker
    // 7. Log: exchange, tickers processed, elapsed
}
```

**Pattern reference**: Follow `CollectBulkEOD` (lines 16-121 in collect.go) for the exchange-based bulk pattern.

### 5. `internal/interfaces/services.go` — Add to MarketService interface

Add after `CollectBulkEOD` (around line 102):

```go
// CollectLivePrices fetches live OHLCV snapshots for all tickers on an exchange
// via the bulk real-time API. Stores on MarketData.LivePrice (ephemeral, not EOD bars).
CollectLivePrices(ctx context.Context, exchange string) error
```

### 6. `internal/models/jobs.go` — Add job type and priority

Add constant after `JobTypeComputeSignals` (line 59):

```go
JobTypeCollectLivePrices = "collect_live_prices"
```

Add priority after `PriorityNewStock` (line 83):

```go
PriorityCollectLivePrices = 11 // Higher than EOD (10) — live data is more urgent
```

Update `DefaultPriority()` switch (add case after `JobTypeComputeSignals`):

```go
case JobTypeCollectLivePrices:
    return PriorityCollectLivePrices
```

Update `TimestampFieldForJobType()` switch (add case):

```go
case JobTypeCollectLivePrices:
    return "live_price_collected_at"
```

### 7. `internal/models/jobs.go` — Add StockIndexEntry field

Add to `StockIndexEntry` struct (after `SignalsCollectedAt`, line 23):

```go
LivePriceCollectedAt time.Time `json:"live_price_collected_at"`
```

### 8. `internal/services/jobmanager/executor.go` — Add dispatch case

Add case in `executeJob` switch (after `JobTypeComputeSignals` case, line 33):

```go
case models.JobTypeCollectLivePrices:
    return jm.market.CollectLivePrices(ctx, job.Ticker) // Ticker = exchange code (e.g. "AU")
```

### 9. `internal/common/freshness.go` — Add live price freshness

Add constant after `FreshnessRealTimeQuote` (line 28):

```go
FreshnessLivePrice = 15 * time.Minute // live OHLCV batch collection interval
```

### 10. `internal/app/scheduler.go` — Add live price scheduler

Add new function after `refreshPrices`:

```go
// startLivePriceScheduler refreshes live OHLCV prices on a 15-minute interval.
// Collects for all exchanges that have tickers in the stock index.
func startLivePriceScheduler(ctx context.Context, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger) {
    ticker := time.NewTicker(common.FreshnessLivePrice)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            logger.Info().Msg("Live price scheduler: stopped")
            return
        case <-ticker.C:
            refreshLivePrices(ctx, marketService, storage, logger)
        }
    }
}

func refreshLivePrices(ctx context.Context, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger) {
    start := time.Now()

    // Get unique exchanges from stock index
    entries, err := storage.StockIndexStore().List(ctx)
    if err != nil || len(entries) == 0 {
        return
    }

    exchanges := make(map[string]bool)
    for _, e := range entries {
        if e.Exchange != "" {
            exchanges[e.Exchange] = true
        }
    }

    for exchange := range exchanges {
        if err := marketService.CollectLivePrices(ctx, exchange); err != nil {
            logger.Warn().Str("exchange", exchange).Err(err).Msg("Live price refresh failed")
        }
    }

    logger.Info().
        Int("exchanges", len(exchanges)).
        Dur("elapsed", time.Since(start)).
        Msg("Live price refresh: complete")
}
```

### 11. `internal/app/app.go` — Wire up live price scheduler

In `StartSchedulers` (around line 325), add after the existing `startPriceScheduler` goroutine:

```go
go startLivePriceScheduler(schedulerCtx, a.MarketService, a.Storage, a.Logger)
```

### 12. `internal/server/handlers.go` — Extend refresh handler

In `handleStockDataRefresh` (line 873), after `EnqueueBatchRefresh`, also enqueue live price jobs:

```go
// Also enqueue live price collection for affected exchanges
exchanges := make(map[string]bool)
for _, t := range tickers {
    if ex := extractExchange(t); ex != "" {
        exchanges[ex] = true
    }
}
for ex := range exchanges {
    _ = s.app.JobManager.EnqueueIfNeeded(r.Context(), models.JobTypeCollectLivePrices, ex, models.PriorityCollectLivePrices)
}
```

Need to add an `extractExchange` helper in the handler file (or use the existing one from market package — check if it's exported). If not exported, add a local helper:

```go
func extractExchange(ticker string) string {
    if i := strings.LastIndex(ticker, "."); i >= 0 {
        return ticker[i+1:]
    }
    return ""
}
```

### 13. `internal/services/jobmanager/watcher.go` — Add live price staleness check

In `scanStockIndex` (around line 95), after the bulk EOD exchange loop, add a similar loop for live prices:

```go
// Enqueue live price jobs for exchanges with stale live data
staleLiveExchanges := make(map[string]bool)
for _, entry := range entries {
    if !common.IsFresh(entry.LivePriceCollectedAt, common.FreshnessLivePrice) {
        if ex := eohdExchangeFromTicker(entry.Ticker); ex != "" {
            staleLiveExchanges[ex] = true
        }
    }
}
for exchange := range staleLiveExchanges {
    if err := jm.EnqueueIfNeeded(ctx, models.JobTypeCollectLivePrices, exchange, models.PriorityCollectLivePrices); err != nil {
        jm.logger.Warn().Str("exchange", exchange).Err(err).Msg("Watcher: failed to enqueue live price job")
    } else {
        enqueued++
    }
}
```

---

## Unit Tests

All tests in `internal/` packages alongside the code they test.

### Test 1: `internal/clients/eodhd/client_test.go` — TestGetBulkRealTimeQuotes
- Mock HTTP server returns JSON array of 3 real-time responses
- Verify map has 3 entries with correct ticker keys
- Verify OHLCV fields parsed correctly
- Test with single ticker (returns object not array)
- Test with empty ticker list (returns empty map)

### Test 2: `internal/services/market/collect_test.go` — TestCollectLivePrices
- Mock EODHD client returns quotes for 3 tickers
- Verify LivePrice field is set on each MarketData record
- Verify LivePriceUpdatedAt is set
- Verify EOD bars are NOT modified
- Verify stock index timestamps updated

---

## Integration Tests (for test-creator)

### `tests/data/live_price_test.go`
1. `TestCollectLivePrices_StoresLivePrice` — end-to-end with real storage
2. `TestCollectLivePrices_DoesNotModifyEOD` — verify EOD bars unchanged after live collection
3. `TestLivePriceFreshness` — verify freshness TTL works correctly

### `tests/api/live_price_test.go`
1. `TestRefreshStockData_EnqueuesLivePriceJob` — verify the handler enqueues live price job

---

## Summary of Changes

| File | Change |
|------|--------|
| `internal/models/market.go` | Add `LivePrice`, `LivePriceUpdatedAt` fields to MarketData |
| `internal/models/jobs.go` | Add `JobTypeCollectLivePrices`, priority, timestamp mapping, `LivePriceCollectedAt` on StockIndexEntry |
| `internal/interfaces/clients.go` | Add `GetBulkRealTimeQuotes` to EODHDClient |
| `internal/interfaces/services.go` | Add `CollectLivePrices` to MarketService |
| `internal/clients/eodhd/client.go` | Implement `GetBulkRealTimeQuotes` |
| `internal/services/market/collect.go` | Implement `CollectLivePrices` |
| `internal/services/jobmanager/executor.go` | Add dispatch case for `collect_live_prices` |
| `internal/services/jobmanager/watcher.go` | Add staleness check for live prices |
| `internal/common/freshness.go` | Add `FreshnessLivePrice` constant |
| `internal/app/scheduler.go` | Add `startLivePriceScheduler` |
| `internal/app/app.go` | Wire live price scheduler |
| `internal/server/handlers.go` | Enqueue live price job in refresh handler |
