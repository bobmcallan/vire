# Requirements: Sync Cooldown

## Problem
`SyncPortfolio(ctx, name, force=true)` has zero rate limiting. Every portal REFRESH click triggers a full Navexa API round-trip (portfolios + holdings + trades). Rapid clicks hammer the external API for zero benefit.

## Scope
- **In scope**: Add cooldown to `SyncPortfolio` for `force=true` calls
- **Out of scope**: Handler-level rate limiting, per-user throttling, 429 responses

## Design

When `force=true`, check `LastSynced` against a new cooldown TTL. If within cooldown, return cached portfolio (same as `force=false` path). The caller gets fresh-enough data silently — no error, no 429.

The existing `force=false` path already uses `FreshnessPortfolio` (30 min). The new `force=true` path uses a shorter `FreshnessSyncCooldown` (5 min).

## Files to Change

### 1. `internal/common/freshness.go` — Add constant
Add after `FreshnessRealTimeQuote`:
```go
FreshnessSyncCooldown  = 5 * time.Minute     // minimum interval between forced re-syncs
```

### 2. `internal/services/portfolio/service.go` — Apply cooldown in SyncPortfolio
Lines 66-85. Currently:
```go
func (s *Service) SyncPortfolio(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
    s.syncMu.Lock()
    defer s.syncMu.Unlock()
    ...
    if !force {
        existing, err := s.getPortfolioRecord(ctx, name)
        if err == nil && common.IsFresh(existing.LastSynced, common.FreshnessPortfolio) {
            s.logger.Debug().Str("name", name).Msg("Portfolio recently synced, skipping")
            return existing, nil
        }
    }
```

Replace the freshness check block (lines 78-85) with:
```go
    // Check freshness: force=false uses standard TTL (30 min),
    // force=true uses shorter cooldown (5 min) to prevent rapid re-syncs.
    if existing, err := s.getPortfolioRecord(ctx, name); err == nil {
        ttl := common.FreshnessPortfolio
        if force {
            ttl = common.FreshnessSyncCooldown
        }
        if common.IsFresh(existing.LastSynced, ttl) {
            s.logger.Debug().Str("name", name).Bool("force", force).
                Dur("ttl", ttl).Msg("Portfolio within sync cooldown, returning cached")
            s.populateHistoricalValues(ctx, existing)
            return existing, nil
        }
    }
```

Key differences from current code:
- Both `force=true` and `force=false` check freshness (previously only `force=false` did)
- `force=true` uses 5 min cooldown instead of 30 min TTL
- Calls `populateHistoricalValues` on the cached portfolio (matches `GetPortfolio` behavior at line 504)

### 3. No interface changes
`SyncPortfolio(ctx, name, force)` signature stays the same. The cooldown is an internal optimization.

## Test Cases

### Unit tests in `internal/services/portfolio/`

**Test 1: `TestSyncPortfolio_ForceCooldown_ReturnsCachedWhenRecent`**
- Sync once (creates portfolio with LastSynced = now)
- Immediately call SyncPortfolio(force=true) again
- Assert: returns cached portfolio (no second Navexa API call)
- Verify: check Navexa client call count == 1

**Test 2: `TestSyncPortfolio_ForceCooldown_SyncsAfterExpiry`**
- Sync once, then manually set LastSynced to 6 min ago (past cooldown)
- Call SyncPortfolio(force=true)
- Assert: full sync happens (Navexa client called again)
- Verify: check Navexa client call count == 2

**Test 3: `TestSyncPortfolio_NonForce_Uses30MinTTL`**
- Sync once, set LastSynced to 10 min ago (past 5 min cooldown but within 30 min TTL)
- Call SyncPortfolio(force=false)
- Assert: returns cached (still within 30 min TTL)
- Verify: Navexa client call count == 1

**Test 4: `TestSyncPortfolio_Force_SyncsWhenNoExistingRecord`**
- Call SyncPortfolio(force=true) with no existing portfolio
- Assert: full sync happens (no cache to check)

## Integration Points
- `service.go:67-85` — SyncPortfolio freshness check (the only change)
- `freshness.go:28` — after last constant (add new constant)
- No handler changes needed — cooldown is transparent to callers
