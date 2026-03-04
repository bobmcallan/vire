# Timeline Cash Fix — Requirements

## Bug Summary

Portfolio timeline data shows `portfolio_value = equity_value` for all historical dates. Cash transactions are not incorporated into persisted timeline snapshots. Only the live-computed "today" data point has correct cash data.

**Root cause**: `GetDailyGrowth()` requires callers to pass cash transactions via `opts.Transactions`. Several callers pass empty `GrowthOptions{}`, producing snapshots without cash. The timeline scheduler (`timeline_scheduler.go:63`) runs every 12h and on startup, calling `GetDailyGrowth` with empty options — overwriting the timeline cache with bad data every time.

**Callers without cash (bugs)**:
1. `internal/app/timeline_scheduler.go:63` — `rebuildTimeline()` passes empty `GrowthOptions{}`
2. `internal/services/portfolio/growth.go:355` — `GetPortfolioGrowth()` passes empty `GrowthOptions{}`

**Callers with cash (correct)**:
- `internal/server/handlers.go:522` — `handlePortfolioHistory` injects cash ✅
- `internal/server/handlers.go:291` — compliance review handler injects cash ✅
- `internal/services/portfolio/indicators.go:80-86` — `GetPortfolioIndicators` injects cash ✅
- `internal/services/portfolio/service.go:96` — `rebuildTimelineWithCash` injects cash ✅

## Fix Approach

**Principle**: `GetDailyGrowth` should automatically load cash transactions internally when `opts.Transactions` is nil (not provided). The PortfolioService already has a `cashflowSvc` reference. Callers should NOT need to inject cash — the service owns this responsibility.

This follows the separation-of-concerns pattern: the service that owns the data flow should handle its own dependencies, not push that responsibility to every caller.

## Scope

### In scope
- Fix `GetDailyGrowth` to auto-load cash when `opts.Transactions` is nil
- Remove manual cash injection from all callers (they become redundant)
- Fix `GetPortfolioGrowth` (inherits the fix via `GetDailyGrowth`)
- Fix timeline scheduler (inherits the fix via `GetDailyGrowth`)
- Add/update unit tests
- Add integration test verifying timeline data has correct cash values

### Out of scope
- No changes to timeline storage, models, or API response format
- No changes to `writeTodaySnapshot` (it uses portfolio header values, separate path)
- No new MCP tools

## Files to Change

### 1. `internal/services/portfolio/growth.go`

**Change**: In `GetDailyGrowth()` (line ~93), after loading the portfolio (phase 1), add auto-loading of cash transactions when `opts.Transactions` is nil.

Insert AFTER line 108 (after portfolio load phase) and BEFORE Phase 2 (date range):

```go
// Auto-load cash transactions if not provided by caller.
// This ensures all code paths (handler, scheduler, internal) include cash
// in timeline computations without requiring explicit injection.
if opts.Transactions == nil && s.cashflowSvc != nil {
    if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
        opts.Transactions = ledger.Transactions
    }
}
```

**Also change**: `GetPortfolioGrowth()` (line 354-360) — no change needed, inherits fix automatically.

### 2. `internal/server/handlers.go`

**Change**: Remove redundant cash injection in `handlePortfolioHistory()` (lines 515-520) and compliance review handler (lines 285-290). Since `GetDailyGrowth` now handles this internally, the explicit injection is dead code.

In `handlePortfolioHistory()` (around line 515-520), remove:
```go
// Load cash transactions for capital timeline (cash balance, net deployed)
if s.app.CashFlowService != nil {
    if ledger, err := s.app.CashFlowService.GetLedger(ctx, name); err == nil && ledger != nil {
        opts.Transactions = ledger.Transactions
    }
}
```

In the compliance review handler (around line 285-290), remove:
```go
growthOpts := interfaces.GrowthOptions{}
if s.app.CashFlowService != nil {
    if ledger, err := s.app.CashFlowService.GetLedger(ctx, name); err == nil && ledger != nil {
        growthOpts.Transactions = ledger.Transactions
    }
}
```
And change line 291 to: `dailyPoints, _ := s.app.PortfolioService.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})`

### 3. `internal/services/portfolio/indicators.go`

**Change**: Remove redundant cash injection in `GetPortfolioIndicators()` (lines 78-84). Since `GetDailyGrowth` now handles this internally.

Remove:
```go
opts := interfaces.GrowthOptions{}
if s.cashflowSvc != nil {
    if ledger, err := s.cashflowSvc.GetLedger(ctx, name); err == nil && ledger != nil {
        opts.Transactions = ledger.Transactions
    }
}
```
And change line 86 to: `growth, err := s.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})`

### 4. `internal/services/portfolio/service.go`

**Change**: `rebuildTimelineWithCash()` (lines 89-97) becomes redundant. The name is misleading since `GetDailyGrowth` now always includes cash. Options:
- **Simplify**: Replace body with just `return s.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})`
- Update comments to reflect the new behavior
- Keep the method name for existing callers but simplify the body

### 5. `internal/app/timeline_scheduler.go` — NO CHANGE NEEDED

The scheduler calls `GetDailyGrowth(ctx, portfolioName, interfaces.GrowthOptions{})` which will now auto-load cash. Bug fixed by inheritance.

## Test Plan

### Unit tests (in `internal/services/portfolio/`)

**File**: `internal/services/portfolio/capital_timeline_test.go` (existing, extend)

1. **TestGetDailyGrowth_AutoLoadsCash** — Verify that when `opts.Transactions` is nil, `GetDailyGrowth` auto-loads cash from the cashflow service and produces correct `net_cash_balance` and `portfolio_value`.

2. **TestGetDailyGrowth_ExplicitTransactionsOverride** — Verify that when `opts.Transactions` is explicitly provided (even empty slice), the auto-load is skipped and the provided transactions are used.

3. **TestGetDailyGrowth_NoCashflowService** — Verify that when `cashflowSvc` is nil, `GetDailyGrowth` degrades gracefully (no cash, portfolio_value = equity_value).

### Integration tests (in `tests/data/`)

4. **TestTimelineDataIncludesCash** — End-to-end: create portfolio with trades and cash transactions, call GetDailyGrowth, verify every historical data point has `net_cash_balance > 0` and `portfolio_value = equity_value + net_cash_balance`.

## Verification

After implementation:
1. `go build ./cmd/vire-server/` — must succeed
2. `go vet ./...` — must pass
3. `go test ./internal/services/portfolio/... -timeout 120s` — all pass
4. `go test ./tests/data/... -timeout 300s` — all pass
5. Deploy and call `admin_rebuild_timeline` → verify timeline output has correct cash values
