# Portfolio Timeline Centralization

## Problem

Portfolio header values and chart/timeline values diverge because they are computed from different sources:

- **Header**: Live Navexa prices via `SyncPortfolio` (TTL 30 min)
- **Chart**: EOD closing prices via `GetDailyGrowth` (full trade replay)

Additionally, `GetDailyGrowth` recomputes the full trade replay from inception on every request (~200-800ms for 1500 days x 20 holdings), and `populateHistoricalValues` redundantly bulk-loads market data on every GET.

## Solution

Persist daily timeline snapshots in a `portfolio_timeline` table. Use this as the single source of truth for both header and chart values.

## Table Design

**Table**: `portfolio_timeline` (SurrealDB, SCHEMALESS)

**Record ID**: `{userID}:{portfolioName}:{YYYY-MM-DD}`

**Model**: `TimelineSnapshot` in `internal/models/portfolio.go`

Fields mirror `GrowthDataPoint` plus metadata:
- Equity: `EquityValue`, `NetEquityCost`, `NetEquityReturn`, `NetEquityReturnPct`, `HoldingCount`
- Cash: `GrossCashBalance`, `NetCashBalance`, `PortfolioValue`, `NetCapitalDeployed`
- Metadata: `FXRate`, `DataVersion`, `ComputedAt`

No per-holding data — holdings are already in the Portfolio record.

## Storage Interface

`TimelineStore` in `internal/interfaces/storage.go`:
- `GetRange(ctx, userID, portfolioName, from, to)` — date range query
- `GetLatest(ctx, userID, portfolioName)` — most recent snapshot
- `SaveBatch(ctx, snapshots)` — batched UPSERTs
- `DeleteRange(ctx, userID, portfolioName, from, to)` — invalidate date range
- `DeleteAll(ctx, userID, portfolioName)` — full invalidation

## Implementation Phases

### Phase 1: Write-Behind Persistence

Persist timeline snapshots as a side-effect of existing computations. Zero behavioral change — all reads still use the current computation path.

- `GetDailyGrowth`: After computing points, fire-and-forget `SaveBatch` for past dates
- `SyncPortfolio`: After completing, write today's snapshot synchronously
- Lazy backfill: First `GetDailyGrowth` call persists all points; second call benefits from Phase 2

### Phase 2: Read from Persisted Timeline

`GetDailyGrowth` reads historical data from the timeline table and only computes recent/missing dates.

1. Query `GetLatest()` to find most recent persisted snapshot
2. If exists: `GetRange()` for persisted portion + trade replay only for missing dates
3. If not: full trade replay (current behavior) + write-behind all

**Performance**: Warm path ~10-30ms (single range query + today's point). Cold path same as current.

### Phase 3: Retroactive Change Invalidation

Hash-based detection: compute trade hash after `SyncPortfolio`, compare with stored `Portfolio.TradeHash`. If changed, `DeleteAll()` for full recompute on next `GetDailyGrowth`.

### Phase 4: Header-Timeline Alignment

`populateHistoricalValues` reads from timeline instead of bulk market data load. Header shows values derived from the same data source as the chart.

## Intraday Price Updates

Stock prices are collected every 5-15 minutes. Today's timeline snapshot is overwritten on each `SyncPortfolio` call (TTL 30 min), picking up the latest collected prices. Historical rows (yesterday and earlier) are immutable once written.

## Migration

- Schema version bump to "13" triggers `PurgeDerivedData()` + `ResetCollectionTimestamps()`
- No eager backfill — portfolios lazily backfilled on first timeline access
- Rollback: Delete `portfolio_timeline` table, revert code — falls back to full computation
