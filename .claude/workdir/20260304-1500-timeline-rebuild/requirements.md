# Requirements: Portfolio Timeline Cash Integration & Force Rebuild

## Feedback Items
- fb_25304b2f (HIGH): Timeline portfolio_value == equity_value — not incorporating cash
- fb_22ce519f (HIGH): Force rebuild + cash transaction invalidation
- fb_bf00d30f (ACKNOWLEDGED): Timeline not recalculating correctly after cash changes

## Root Cause

Three internal callers invoke `GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})` with
**empty GrowthOptions** (no cash transactions). This causes `hasCashTxs = false` at
`growth.go:320`, setting `portfolioVal = totalValue` (equity only, no cash).
These incorrect snapshots get persisted via `persistTimelineSnapshots` and served from
the timeline cache, producing `portfolio_value == equity_value` for the entire history.

**Broken callers:**
1. `triggerTimelineRebuildAsync()` — service.go:98
2. `backfillTimelineIfEmpty()` — service.go:1221
3. `onLedgerChange` callback — app.go:215

**The handler path is correct**: `handlePortfolioHistory()` (handlers.go:516-519) loads
the ledger and passes `opts.Transactions`, so fresh computes include cash. But cached
data (written by the broken callers) does not.

## Scope

### In Scope
1. **Fix**: All internal timeline rebuilds must load cash transactions
2. **Fix**: Dedup concurrent timeline rebuilds (no point running consecutively)
3. **Fix**: Cash ledger changes trigger efficient timeline invalidation + rebuild
4. **New**: Admin-only force rebuild endpoint (delete all + recompute from scratch)
5. **New**: `InvalidateAndRebuildTimeline` and `ForceRebuildTimeline` on PortfolioService

### Out of Scope
- Stock timeline (GetStockTimeline) persistence/caching — remains on-demand
- Incremental portfolio timeline updates (optimization for later)
- Company timeline (collect_timeline job) force rebuild — separate feature

## User Notes
1. Timeline builder must be efficient. Background job. Skip if rebuild already in progress.
2. Stock timelines: out of scope for this task (no changes needed — they're computed on-demand).
3. `portfolio_get --force` is a user function. Admin-only function = full rebuild from scratch.

---

## Files to Change

### 1. `internal/services/portfolio/service.go`

**A. New helper: `rebuildTimelineWithCash()`** (add after line 104)

```go
// rebuildTimelineWithCash loads the cash ledger and recomputes the full portfolio
// timeline with cash transactions included. This ensures persisted snapshots have
// correct portfolio_value = equity_value + net_cash_balance.
//
// All internal rebuild paths MUST use this instead of bare GetDailyGrowth with
// empty GrowthOptions — otherwise cash data is excluded from persisted snapshots.
func (s *Service) rebuildTimelineWithCash(ctx context.Context, name string) ([]models.GrowthDataPoint, error) {
	opts := interfaces.GrowthOptions{}
	if s.cashflowSvc != nil {
		if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
			opts.Transactions = ledger.Transactions
		}
	}
	return s.GetDailyGrowth(ctx, name, opts)
}
```

**B. Fix `triggerTimelineRebuildAsync()`** (modify existing, lines 83-104)

Two changes:
1. Add dedup: if already rebuilding, skip (log + return)
2. Call `rebuildTimelineWithCash` instead of `GetDailyGrowth(bgCtx, name, interfaces.GrowthOptions{})`

```go
func (s *Service) triggerTimelineRebuildAsync(ctx context.Context, name string) {
	// Dedup: skip if a rebuild is already in progress for this portfolio.
	// Prevents concurrent full recomputes which waste resources and produce the same result.
	if s.IsTimelineRebuilding(name) {
		s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild already in progress — skipping")
		return
	}
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
		// CRITICAL: use rebuildTimelineWithCash to include cash transactions.
		// Bare GetDailyGrowth with empty GrowthOptions excludes cash from persisted snapshots.
		if _, err := s.rebuildTimelineWithCash(bgCtx, name); err != nil {
			s.logger.Warn().Err(err).Str("portfolio", name).Msg("Timeline rebuild after trade change failed")
			return
		}
		s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild after trade change complete")
	}()
}
```

**C. Fix `backfillTimelineIfEmpty()`** (modify line 1221)

Replace:
```go
if _, err := s.GetDailyGrowth(bgCtx, portfolio.Name, interfaces.GrowthOptions{}); err != nil {
```
With:
```go
if _, err := s.rebuildTimelineWithCash(bgCtx, portfolio.Name); err != nil {
```

Also add dedup check before spawning goroutine (after the sparse check, before logging):
```go
// Skip backfill if a rebuild is already in progress
if s.IsTimelineRebuilding(portfolio.Name) {
	return
}
```

**D. New method: `InvalidateAndRebuildTimeline()`** (add after `rebuildTimelineWithCash`)

```go
// InvalidateAndRebuildTimeline deletes persisted timeline data and triggers an
// async rebuild with cash transactions. Used when external data (e.g. cash ledger)
// changes that affect historical portfolio values.
func (s *Service) InvalidateAndRebuildTimeline(ctx context.Context, name string) {
	if s.IsTimelineRebuilding(name) {
		s.logger.Info().Str("portfolio", name).Msg("Timeline rebuild already in progress — skipping invalidation")
		return
	}

	userID := common.ResolveUserID(ctx)
	if tl := s.storage.TimelineStore(); tl != nil {
		if _, err := tl.DeleteAll(ctx, userID, name); err != nil {
			s.logger.Warn().Err(err).Str("portfolio", name).Msg("Timeline invalidation: delete failed")
		}
	}

	s.triggerTimelineRebuildAsync(ctx, name)
}
```

**E. New method: `ForceRebuildTimeline()`** (add after `InvalidateAndRebuildTimeline`)

```go
// ForceRebuildTimeline is an admin-only operation that deletes ALL timeline data
// for a portfolio and triggers a full from-scratch recompute. Unlike
// InvalidateAndRebuildTimeline, this does NOT skip if a rebuild is in progress —
// it forcefully clears everything and starts fresh.
func (s *Service) ForceRebuildTimeline(ctx context.Context, name string) error {
	userID := common.ResolveUserID(ctx)
	tl := s.storage.TimelineStore()
	if tl == nil {
		return fmt.Errorf("timeline store not available")
	}

	deleted, err := tl.DeleteAll(ctx, userID, name)
	if err != nil {
		return fmt.Errorf("failed to delete timeline data: %w", err)
	}
	s.logger.Info().Str("portfolio", name).Int("deleted", deleted).Msg("Admin force rebuild: timeline data deleted")

	// Force: bypass dedup check by resetting flag first
	s.timelineRebuilding.Store(name, false)
	s.triggerTimelineRebuildAsync(ctx, name)
	return nil
}
```

### 2. `internal/interfaces/services.go`

Add to `PortfolioService` interface (after `IsTimelineRebuilding`):

```go
// InvalidateAndRebuildTimeline deletes the timeline cache and triggers an async
// rebuild. Safe to call from external services (e.g. cash flow on ledger change).
InvalidateAndRebuildTimeline(ctx context.Context, name string)

// ForceRebuildTimeline admin-only: deletes ALL timeline data and forces a full
// from-scratch recompute, even if a rebuild is already in progress.
ForceRebuildTimeline(ctx context.Context, name string) error
```

### 3. `internal/app/app.go`

Simplify the `onLedgerChange` callback (replace lines 206-219):

```go
cashflowService.SetOnLedgerChange(func(cbCtx context.Context, portfolioName string) {
	portfolioService.InvalidateAndRebuildTimeline(cbCtx, portfolioName)
	logger.Info().Str("portfolio", portfolioName).Msg("Timeline invalidation triggered by cash flow change")
})
```

### 4. `internal/server/handlers_admin.go`

New handler (add after `handleAdminJobEnqueue`, around line 288):

```go
// handleAdminRebuildTimeline handles POST /api/admin/portfolios/{name}/rebuild-timeline.
// Admin-only: force-delete all timeline data and trigger a full recompute from scratch.
func (s *Server) handleAdminRebuildTimeline(w http.ResponseWriter, r *http.Request, name string) {
	if !RequireMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	if err := s.app.PortfolioService.ForceRebuildTimeline(ctx, name); err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to rebuild timeline: "+err.Error())
		return
	}

	WriteJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":    "rebuilding",
		"portfolio": name,
		"message":   "Timeline data deleted and full rebuild triggered in background",
	})
}
```

### 5. `internal/server/routes.go`

Add route for admin timeline rebuild. Find the admin route section and add:

Look at the existing pattern for admin routes. There should be a section handling
`/api/admin/` prefixed routes. Add a case for `portfolios/{name}/rebuild-timeline`.

In the portfolio sub-routing section (around line 275-278 area), add a check for
`rebuild-timeline` suffix. OR add it as a dedicated admin mux route.

**Approach**: Add as a dedicated mux route since admin routes use `mux.HandleFunc`:

Find where admin routes are registered (around lines 108-116) and add:

```go
// In the admin route handlers section, add a pattern for rebuild-timeline.
// Since mux.HandleFunc doesn't support path params, we need dynamic routing.
```

Actually, looking at the routing pattern in routes.go, admin routes with path params
are handled in `handleAdminRoutes` or via path stripping. Check the existing pattern
for `/api/admin/users/{id}` — it uses path suffix stripping at lines 183-192.

The cleanest approach: use the portfolio route dispatcher. Find the existing portfolio
routes section. The admin rebuild-timeline endpoint should be at:
`POST /api/admin/portfolios/{name}/rebuild-timeline`

Since the admin section at lines 108-116 uses explicit mux.HandleFunc, we need a new
handler function for the `/api/admin/portfolios/` prefix, or we can add it to the
existing admin routing logic.

**Implementation**: Add in the admin routes section (after line 116):
```go
mux.HandleFunc("/api/admin/portfolios/", s.handleAdminPortfolioRoutes)
```

Then create the handler:
```go
// handleAdminPortfolioRoutes dispatches admin portfolio sub-routes.
func (s *Server) handleAdminPortfolioRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	// Path: /api/admin/portfolios/{name}/rebuild-timeline
	rest := strings.TrimPrefix(r.URL.Path, "/api/admin/portfolios/")
	if strings.HasSuffix(rest, "/rebuild-timeline") {
		name := strings.TrimSuffix(rest, "/rebuild-timeline")
		s.handleAdminRebuildTimeline(w, r, name)
		return
	}
	WriteError(w, http.StatusNotFound, "Unknown admin portfolio route")
}
```

### 6. `internal/server/catalog.go`

Add MCP tool definition for admin_rebuild_timeline. Add in the admin tools section
(near the other admin_* tools, around line 317):

```go
{
	Name:        "admin_rebuild_timeline",
	Description: "Force-rebuild a portfolio's timeline from scratch. Deletes all persisted timeline data and triggers a full recompute including cash balance integration. Admin access required. This is an async operation — the timeline rebuilds in the background.",
	Method:      "POST",
	Path:        "/api/admin/portfolios/{portfolio_name}/rebuild-timeline",
	Params: []models.ParamDefinition{
		portfolioParam,
	},
},
```

### 7. Mock updates (test files)

Any mock implementing `PortfolioService` needs the two new methods. Search for
mock implementations and add stubs:

```go
func (m *mockPortfolioService) InvalidateAndRebuildTimeline(ctx context.Context, name string) {}
func (m *mockPortfolioService) ForceRebuildTimeline(ctx context.Context, name string) error { return nil }
```

Files to update (grep for `PortfolioService` mock implementations):
- `internal/server/handlers_portfolio_test.go` — mockPortfolioService
- Any other test files implementing the interface

---

## Unit Tests

### File: `internal/services/portfolio/timeline_rebuild_test.go` (NEW)

| # | Test Name | Verifies |
|---|-----------|----------|
| 1 | `TestRebuildTimelineWithCash_LoadsTransactions` | rebuildTimelineWithCash loads cash ledger and passes transactions to GetDailyGrowth |
| 2 | `TestTriggerTimelineRebuildAsync_DedupSkipsWhenRebuilding` | If IsTimelineRebuilding returns true, triggerTimelineRebuildAsync returns without spawning goroutine |
| 3 | `TestTriggerTimelineRebuildAsync_SetsFlag` | Flag is set to true during rebuild and false after completion |
| 4 | `TestInvalidateAndRebuildTimeline_DeletesAndRebuilds` | Calls DeleteAll on TimelineStore then triggers rebuild |
| 5 | `TestInvalidateAndRebuildTimeline_SkipsWhenRebuilding` | If already rebuilding, does not delete or rebuild |
| 6 | `TestForceRebuildTimeline_BypassesDedup` | Even if rebuilding flag is set, force clears it and triggers rebuild |
| 7 | `TestForceRebuildTimeline_DeletesAllData` | Calls DeleteAll on TimelineStore |
| 8 | `TestBackfillTimelineIfEmpty_IncludesCashTransactions` | Backfill loads cash transactions (not empty GrowthOptions) |

---

## Integration Points

| What | Where | How |
|------|-------|-----|
| rebuildTimelineWithCash | service.go after line 104 | New private method |
| triggerTimelineRebuildAsync fix | service.go lines 83-104 | Modify: add dedup + use rebuildTimelineWithCash |
| backfillTimelineIfEmpty fix | service.go line 1221 | Replace GetDailyGrowth call with rebuildTimelineWithCash |
| InvalidateAndRebuildTimeline | service.go after rebuildTimelineWithCash | New public method |
| ForceRebuildTimeline | service.go after InvalidateAndRebuildTimeline | New public method |
| Interface additions | interfaces/services.go after line 61 | Add 2 methods to PortfolioService |
| onLedgerChange simplification | app/app.go lines 206-219 | Replace with InvalidateAndRebuildTimeline call |
| Admin handler | handlers_admin.go after line 288 | New handleAdminRebuildTimeline |
| Admin routing | routes.go after line 116 | New mux route + dispatcher |
| MCP catalog | catalog.go admin section ~line 317 | New tool definition |
| Mock stubs | handlers_portfolio_test.go | 2 method stubs |

---

## Verification Checklist

- [ ] `go build ./cmd/vire-server/` succeeds
- [ ] `go vet ./...` clean
- [ ] `go test ./internal/services/portfolio/...` passes (including new tests)
- [ ] `go test ./internal/server/...` passes (mock stubs compile)
- [ ] Timeline data points show `portfolio_value != equity_value` when cash transactions exist
- [ ] Concurrent rebuild requests are deduplicated (second request skipped)
- [ ] Cash ledger add/update/delete triggers timeline invalidation + rebuild
- [ ] Admin force rebuild endpoint deletes all data and rebuilds
- [ ] MCP tool `admin_rebuild_timeline` appears in catalog
