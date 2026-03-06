# Summary: Live OHLCV Price Collection

**Status:** completed
**Feedback:** fb_fa72a550

## Changes

| File | Change |
|------|--------|
| `internal/models/market.go` | Added `LivePrice *RealTimeQuote` and `LivePriceUpdatedAt` to MarketData |
| `internal/models/jobs.go` | Added `JobTypeCollectLivePrices`, `PriorityCollectLivePrices = 11`, `LivePriceCollectedAt` on StockIndexEntry, updated DefaultPriority + TimestampFieldForJobType |
| `internal/interfaces/clients.go` | Added `GetBulkRealTimeQuotes` to EODHDClient interface |
| `internal/interfaces/services.go` | Added `CollectLivePrices` to MarketService interface |
| `internal/clients/eodhd/client.go` | Implemented `GetBulkRealTimeQuotes` (batch via `s=` param, handles single object + multi-ticker array) |
| `internal/services/market/collect.go` | Implemented `CollectLivePrices` (exchange-based bulk, batches of 20, ephemeral storage) |
| `internal/services/jobmanager/executor.go` | Added dispatch case for `collect_live_prices` |
| `internal/services/jobmanager/watcher.go` | Added staleness check loop for live prices |
| `internal/common/freshness.go` | Added `FreshnessLivePrice = 15 * time.Minute` |
| `internal/app/scheduler.go` | Added `startLivePriceScheduler` + `refreshLivePrices` |
| `internal/app/app.go` | Wired live price scheduler |
| `internal/server/handlers.go` | Extended refresh handler to enqueue live price jobs + `extractExchange` helper |

## Tests

- **Unit tests**: 7 new (4 client + 3 service) — all pass
- **Stress tests**: 30 new (18 service + 12 client) — all pass
- **Integration tests**: 4 new (3 data + 1 API) — all pass
- **Full suite**: 25 internal pkgs pass, 325 data tests pass, 19 API tests pass, vet clean
- **Fix rounds**: 0

## Architecture

- Docs updated: `docs/architecture/26-02-27-services.md`, `docs/architecture/26-02-27-jobs.md`
- All 7 architecture checks passed (separation of concerns, data ownership, job pattern, naming)

## Devils-Advocate

- 30 stress tests covering: nil client, empty index, partial failures, EOD integrity, batch boundaries, concurrency, malformed JSON, timeouts
- Observations (no action): double MarketData fetch (matches existing pattern), zero-price quotes stored as-is

## Notes

- LivePrice is ephemeral — overwritten each cycle, NOT merged into EOD bars
- Signals are NOT recomputed from live prices (EOD-only)
- 2 pre-existing test failures unrelated to this feature (TestHealthEndpoint env vars, flaky date filter)
