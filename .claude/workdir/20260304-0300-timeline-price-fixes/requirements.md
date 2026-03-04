# Requirements: Timeline Rebuild Trigger, Rebuild Visibility & EODHD Price Divergence Guard

**Work dir:** `.claude/workdir/20260304-0300-timeline-price-fixes/`
**Feedback items:** fb_bf00d30f, fb_b7001891, fb_82594b9c

---

## Scope

Three targeted fixes. No new services, no new routes, no schema bump.

**In scope:**
1. **fb_bf00d30f** — When `SyncPortfolio` detects a trade hash change, explicitly trigger a full timeline rebuild goroutine (not relying on `backfillTimelineIfEmpty` alone, which has a bug where `writeTodaySnapshot` runs first and gives it 1 snapshot, potentially confusing the sparse-history check).
2. **fb_b7001891** — Surface a `timeline_rebuilding` advisory field in `portfolio_get_status`, `portfolio_get`, and `portfolio_get_timeline` responses so clients know when to expect updated data.
3. **fb_82594b9c** — Add a price divergence guard in the EODHD cross-check: if EODHD price diverges from Navexa by >50%, reject EODHD price (log warning) and keep Navexa price. Prevents wrong-instrument ticker mapping from corrupting portfolio valuations.

**Out of scope:**
- New routes or MCP tools
- Schema version bump
- Navexa-side ticker overrides (future work)
- Portfolio source types (already implemented)

---

## Root Cause Analysis

### fb_bf00d30f

**Bug**: In `service.go:SyncPortfolio`, when `existingTradeHash != tradeHash`:
1. `tl.DeleteAll(ctx, userID, name)` removes all historical snapshots ✓
2. `s.writeTodaySnapshot(ctx, portfolio)` writes 1 snapshot for today ← runs BEFORE backfill check
3. `s.backfillTimelineIfEmpty(ctx, portfolio)` calls `GetRange(earliest, yesterday)` → returns 0 results (today's snapshot is excluded from the yesterday range) → triggers `GetDailyGrowth` goroutine

The triggering SHOULD work, but the goroutine path in `backfillTimelineIfEmpty` uses `context.Background()` + user context but **no Navexa client**. If `GetDailyGrowth` needs a Navexa client, it fails silently. Additionally, `backfillTimelineIfEmpty` is designed for "schema rebuild / first sync" scenarios, not the "trade changed" scenario — the intent is unclear. The explicit fix is a dedicated rebuild goroutine when the trade hash changes.

**Fix**: After `tl.DeleteAll`, directly call a new private method `s.triggerTimelineRebuildAsync(ctx, name)` that:
- Sets `rebuilding[name] = true`
- Spawns a goroutine calling `GetDailyGrowth`
- Clears `rebuilding[name] = false` when done
- Does NOT rely on `backfillTimelineIfEmpty` for this path (backfillTimelineIfEmpty remains for schema-rebuild / first-sync)

### fb_b7001891

**Missing**: No `rebuilding` state exposed anywhere. The `portfolio_get_status` `timeline` section has `snapshots` and `last_computed` but no rebuilding flag.

**Fix**: Track rebuild state in `Service` with a `sync.Map`. Expose via `IsTimelineRebuilding(name string) bool` added to the `PortfolioService` interface. Add to status response, portfolio response, and timeline response.

### fb_82594b9c

**Bug**: In `SyncPortfolio` EODHD price cross-check (service.go ~line 263), the code accepts any EODHD price that is within 24 hours and different from Navexa. ACDC.AU on EODHD resolves to a different instrument than the ASX-listed Global X Battery Tech & Lithium ETF, giving `$5.01` vs Navexa's correct ~`$140.65` (96% divergence).

**Fix**: Before using the EODHD price, check: `abs(eodhd - navexa) / navexa > 0.5` → skip EODHD override, log warning with divergence %.

---

## Files to Change

### 1. `internal/interfaces/services.go`

**Add to `PortfolioService` interface** (after `RefreshTodaySnapshot`):

```go
// IsTimelineRebuilding returns true when a full timeline rebuild is in progress
// for the named portfolio. Safe for concurrent calls.
IsTimelineRebuilding(name string) bool
```

### 2. `internal/models/portfolio.go`

**Add to `Portfolio` struct** (after `Changes *PortfolioChanges` line ~83, in the "computed on response, not persisted" section):

```go
// TimelineRebuilding is set when a full timeline rebuild is in progress.
// Computed on response, not persisted.
TimelineRebuilding bool `json:"timeline_rebuilding,omitempty"`
```

### 3. `internal/services/portfolio/service.go`

**A. Add `timelineRebuilding sync.Map` to `Service` struct** (the struct is defined early in the file; add alongside `syncMu sync.Mutex`):

```go
timelineRebuilding sync.Map // map[string]bool — true while a rebuild goroutine runs
```

**B. Add `IsTimelineRebuilding` method** (after `SetTradeService`):

```go
// IsTimelineRebuilding returns true when a full timeline rebuild is in progress
// for the named portfolio.
func (s *Service) IsTimelineRebuilding(name string) bool {
    v, ok := s.timelineRebuilding.Load(name)
    if !ok {
        return false
    }
    b, _ := v.(bool)
    return b
}
```

**C. Add `triggerTimelineRebuildAsync` method** (near `backfillTimelineIfEmpty`):

```go
// triggerTimelineRebuildAsync spawns a background goroutine to fully recompute
// the portfolio timeline. Sets the rebuilding flag for the duration.
// Call when the trade hash changes and the timeline cache has been invalidated.
func (s *Service) triggerTimelineRebuildAsync(ctx context.Context, name string) {
    s.timelineRebuilding.Store(name, true)
    go func() {
        defer func() {
            s.timelineRebuilding.Store(name, false)
            if r := recover(); r != nil {
                s.logger.Warn().Str("portfolio", name).Msgf("Timeline rebuild panic recovered: %v", r)
            }
        }()
        bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
        defer cancel()
        bgCtx = common.WithUserContext(bgCtx, common.UserContextFromContext(ctx))
        if _, err := s.GetDailyGrowth(bgCtx, name, interfaces.GrowthOptions{}); err != nil {
            s.logger.Warn().Err(err).Str("portfolio", name).Msg("Timeline rebuild after trade change failed")
            return
        }
        s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild after trade change complete")
    }()
}
```

**D. Modify `SyncPortfolio` trade-hash-change block** (~line 506-514):

Replace:
```go
if existingTradeHash != "" && existingTradeHash != tradeHash {
    s.logger.Info().Str("portfolio", name).Msg("Trade data changed — invalidating timeline cache")
    userID := common.ResolveUserID(ctx)
    if tl := s.storage.TimelineStore(); tl != nil {
        if _, err := tl.DeleteAll(ctx, userID, name); err != nil {
            s.logger.Warn().Err(err).Str("portfolio", name).Msg("Failed to invalidate timeline cache")
        }
    }
}
```

With:
```go
tradeHashChanged := existingTradeHash != "" && existingTradeHash != tradeHash
if tradeHashChanged {
    s.logger.Info().Str("portfolio", name).Msg("Trade data changed — invalidating timeline cache")
    userID := common.ResolveUserID(ctx)
    if tl := s.storage.TimelineStore(); tl != nil {
        if _, err := tl.DeleteAll(ctx, userID, name); err != nil {
            s.logger.Warn().Err(err).Str("portfolio", name).Msg("Failed to invalidate timeline cache")
        }
    }
}
```

Then after `s.savePortfolioRecord(ctx, portfolio)` and BEFORE `s.writeTodaySnapshot` call (~line 517-540), add:

```go
// Trigger async rebuild when trades changed. Must happen after savePortfolioRecord
// so GetDailyGrowth reads the new trade data. writeTodaySnapshot runs after so
// the rebuild goroutine sees a clean slate (no stale today-snapshot).
if tradeHashChanged {
    s.triggerTimelineRebuildAsync(ctx, name)
}
```

Note: Keep `backfillTimelineIfEmpty` call after `writeTodaySnapshot` for the first-sync / schema-rebuild path. The `tradeHashChanged` path is a separate, explicit rebuild.

**E. Modify EODHD price cross-check** (~line 263, inside `for _, h := range navexaHoldings`):

Find the block:
```go
eodhPrice := eodClosePrice(latestBar)
if time.Since(latestBar.Date) < 24*time.Hour && eodhPrice != h.CurrentPrice {
    s.logger.Info()...
    oldMarketValue := h.MarketValue
    h.CurrentPrice = eodhPrice
    ...
}
```

Replace with:
```go
eodhPrice := eodClosePrice(latestBar)
if time.Since(latestBar.Date) < 24*time.Hour && eodhPrice != h.CurrentPrice {
    // Guard: reject EODHD price if it diverges >50% from Navexa — indicates wrong
    // instrument mapping in EODHD (e.g. ticker resolves to different/delisted security).
    if h.CurrentPrice > 0 {
        divergencePct := math.Abs(eodhPrice-h.CurrentPrice) / h.CurrentPrice * 100
        if divergencePct > 50.0 {
            s.logger.Warn().
                Str("ticker", h.Ticker).
                Float64("navexa_price", h.CurrentPrice).
                Float64("eodhd_price", eodhPrice).
                Float64("divergence_pct", divergencePct).
                Msg("EODHD price rejected: >50% divergence suggests wrong instrument mapping")
            continue
        }
    }
    s.logger.Info()...   // existing log
    oldMarketValue := h.MarketValue
    h.CurrentPrice = eodhPrice
    ...
}
```

**F. Populate `TimelineRebuilding` in `GetPortfolio`** (line ~553):

After `GetPortfolio` fetches the portfolio record and before returning, add:
```go
portfolio.TimelineRebuilding = s.IsTimelineRebuilding(name)
```

Find the return paths in `GetPortfolio` and add this line before each `return portfolio, nil`.

### 4. `internal/server/handlers.go`

**A. Modify `handlePortfolioStatus`** (~line 2521):

In the `"timeline"` map, add `"rebuilding"` key:
```go
"timeline": map[string]interface{}{
    "snapshots":     len(snapshots),
    "last_computed": lastComputed,
    "rebuilding":    s.app.PortfolioService.IsTimelineRebuilding(name),  // NEW
},
```

**B. Modify `handlePortfolioHistory`** (timeline endpoint — find by searching for `handlePortfolioHistory` or `GetDailyGrowth` call in handlers.go):

After writing the response, add an advisory if rebuilding:
```go
result := map[string]interface{}{
    "portfolio": name,
    "format":    format,
    "data_points": points,
    "count":     len(points),
}
if s.app.PortfolioService.IsTimelineRebuilding(name) {
    result["advisory"] = "Timeline is being rebuilt after trade changes — data may be incomplete"
}
WriteJSON(w, http.StatusOK, result)
```

### 5. Mock updates — add `IsTimelineRebuilding` stub to all mock structs

All these files have a `mockPortfolioService` or similar struct that implements `PortfolioService`. Add:

```go
func (m *mockPortfolioService) IsTimelineRebuilding(_ string) bool { return false }
```

Files to update:
- `internal/server/handlers_portfolio_test.go` — `mockPortfolioService`
- `internal/services/report/devils_advocate_test.go` — `mockPortfolioService`
- `internal/services/cashflow/service_test.go` — `mockPortfolioService`
- `internal/server/glossary_test.go` — find mock struct, add stub
- `internal/server/handlers_status_test.go` — find mock struct, add stub
- `internal/services/report/service_test.go` — find mock struct, add stub

For each file: grep for the PortfolioService mock struct, add the stub method.

---

## Test Cases

### Unit tests — `internal/services/portfolio/` (add to existing test files or new file `rebuild_test.go`)

```
TestIsTimelineRebuilding_FalseByDefault
  - Create service with no rebuilds in progress
  - Call IsTimelineRebuilding("SMSF") → should return false

TestIsTimelineRebuilding_TrueWhenRebuildActive
  - Manually store "SMSF" → true in timelineRebuilding sync.Map
  - Call IsTimelineRebuilding("SMSF") → should return true

TestSyncPortfolio_EODHDPriceDivergence_KeepsNavexaPrice
  - Set up holding with Navexa price = 140.65
  - Set up EODHD bar with price = 5.01, date = today (within 24h)
  - Call the divergence check logic
  - Assert: holding.CurrentPrice is still 140.65 (EODHD rejected)
  - Assert: warning was logged with divergence_pct

TestSyncPortfolio_EODHDPriceDivergence_AcceptsSmallDiff
  - Set up holding with Navexa price = 45.00
  - Set up EODHD bar with price = 45.50, date = today (within 24h)
  - Assert: holding.CurrentPrice becomes 45.50 (EODHD accepted)

TestTriggerTimelineRebuildAsync_SetsRebuildingFlag
  - Call triggerTimelineRebuildAsync on a service
  - Check IsTimelineRebuilding returns true immediately after call
  - (goroutine will eventually clear it)
```

### Integration tests — `tests/data/` (new file: `timeline_rebuild_test.go`)

Use `testManager(t)` and `testContext()` from `tests/data/helpers_test.go`.

```
TestTimelineRebuildAfterTradeChange_PortfolioStatus
  - Note: This tests the rebuilding flag mechanic, not live Navexa integration
  - Create a portfolio service, manually set rebuilding flag
  - Assert IsTimelineRebuilding returns true
  - Clear flag, assert false

TestEODHDDivergenceGuard_RejectsLargeDeviation
  - Unit-level test (may live in internal/ if it needs internal access)
  - Verify the 50% threshold is applied correctly
```

---

## Integration Points (exact code locations)

| Change | File | Approximate Location |
|--------|------|---------------------|
| Interface method added | `internal/interfaces/services.go` | After line 54 (RefreshTodaySnapshot) |
| Portfolio struct field | `internal/models/portfolio.go` | After line 83 (Changes field) |
| Service struct field | `internal/services/portfolio/service.go` | Near `syncMu sync.Mutex` in struct |
| IsTimelineRebuilding method | `internal/services/portfolio/service.go` | After `SetTradeService` (~line 63) |
| triggerTimelineRebuildAsync | `internal/services/portfolio/service.go` | Near `backfillTimelineIfEmpty` (~line 1114) |
| SyncPortfolio: `tradeHashChanged` var | `internal/services/portfolio/service.go` | Lines 506-514 |
| SyncPortfolio: call rebuild | `internal/services/portfolio/service.go` | After `savePortfolioRecord` (~line 517) |
| EODHD divergence guard | `internal/services/portfolio/service.go` | ~line 263 (inside EODHD cross-check loop) |
| Populate TimelineRebuilding | `internal/services/portfolio/service.go` | In `GetPortfolio`, before each return |
| Status handler: rebuilding | `internal/server/handlers.go` | ~line 2521 (timeline map) |
| Timeline handler: advisory | `internal/server/handlers.go` | `handlePortfolioHistory` response |
| Mock stubs | 6 test files | Add `IsTimelineRebuilding` stub |

---

## Constraints

- No new HTTP routes
- No MCP tool changes (the existing `portfolio_get_status` MCP tool reads the status endpoint already)
- `math.Abs` is already imported in service.go (used for unit mismatch check)
- The 50% divergence threshold is a constant — use `const eodhPriceDivergenceThreshold = 50.0` near the function
- `triggerTimelineRebuildAsync` uses 5-minute timeout (longer than `backfillTimelineIfEmpty`'s 2-minute, since this is the primary rebuild path)
- All new code follows existing error/logging patterns (zerolog builder, `s.logger.Warn()`, etc.)
