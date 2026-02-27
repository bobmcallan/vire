# Requirements: Capital Timeline Enhancements

**Feedback**: fb_da8cabc1, fb_ca924779, fb_e69d6635
**Date**: 2026-02-27

## Problem

The capital allocation timeline (growth data points) has fields for `CashBalance`, `ExternalBalance`, `TotalCapital`, and `NetDeployed` but they are incomplete:

1. **fb_da8cabc1** (ROOT): `handlePortfolioHistory` passes empty `GrowthOptions{}` with no cash transactions to `GetDailyGrowth`. This means `CashBalance` and `NetDeployed` are always 0 in API responses. The handler needs to load the cashflow ledger and pass transactions.

2. **fb_ca924779**: Growth data needs `TotalCapital` (holdings + cash + external) and `NetDeployed` (cumulative deposits - withdrawals) so the portal can chart "what you have" vs "what you put in". The fields exist on `GrowthDataPoint` but `ExternalBalance` and `TotalCapital` are never populated in `GetDailyGrowth`.

3. **fb_e69d6635**: ALREADY DONE — `yesterday_net_flow` and `last_week_net_flow` added in commit 998db94. Just needs status update.

## Fixes

### Fix 1: Load cash transactions in handlePortfolioHistory
**File**: `internal/server/handlers.go` (~line 445)

The handler creates `GrowthOptions{}` with no transactions. It needs to:
- Get the cashflow service from `s.app`
- Load the ledger: `s.app.CashFlowService.GetLedger(ctx, name)`
- Pass transactions: `opts.Transactions = ledger.Transactions`

```go
// Load cash transactions for capital timeline
if s.app.CashFlowService != nil {
    ledger, err := s.app.CashFlowService.GetLedger(ctx, name)
    if err == nil && ledger != nil {
        opts.Transactions = ledger.Transactions
    }
}
```

### Fix 2: Same fix for handlePortfolioReview
**File**: `internal/server/handlers.go` (~line 278)

The review handler also calls `GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})` with no transactions. Apply the same fix.

### Fix 3: Populate ExternalBalance and TotalCapital in GetDailyGrowth
**File**: `internal/services/portfolio/growth.go`

After the cash cursor loop, compute:
- `ExternalBalance`: use the portfolio's current `ExternalBalanceTotal` (constant — no historical data)
- `TotalCapital`: `totalValue + runningCashBalance + externalBalance`

In the GrowthDataPoint construction (~line 221):
```go
points = append(points, models.GrowthDataPoint{
    ...
    CashBalance:     runningCashBalance,
    ExternalBalance: p.ExternalBalanceTotal,  // NEW
    TotalCapital:    totalValue + runningCashBalance + p.ExternalBalanceTotal,  // NEW
    NetDeployed:     runningNetDeployed,
})
```

### Fix 4: Add MCP catalog entry for history endpoint
**File**: `internal/server/catalog.go`

Add a tool definition for `get_capital_timeline`:
```go
{
    Name:        "get_capital_timeline",
    Description: "Get daily portfolio value timeline with capital allocation (holdings, cash, external balances). Shows total capital vs net deployed for P&L analysis.",
    Method:      "GET",
    Path:        "/api/portfolios/{portfolio_name}/history",
    Params: []models.ParamDefinition{
        portfolioParam,
        {Name: "from", Type: "string", Description: "Start date (YYYY-MM-DD). Defaults to portfolio inception.", In: "query"},
        {Name: "to", Type: "string", Description: "End date (YYYY-MM-DD). Defaults to yesterday.", In: "query"},
        {Name: "format", Type: "string", Description: "Output format: daily, weekly, monthly, auto (default).", In: "query"},
    },
}
```

### Fix 5: Resolve fb_e69d6635 and related duplicates
Update feedback status for items already implemented or consolidated.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/server/handlers.go` | Load cash transactions in handlePortfolioHistory + handlePortfolioReview |
| `internal/services/portfolio/growth.go` | Populate ExternalBalance and TotalCapital in growth points |
| `internal/server/catalog.go` | Add get_capital_timeline tool definition |

## Design Decisions

1. **External balance as constant**: No historical external balance data exists. Use current `ExternalBalanceTotal` as a constant for all data points. This is approximate but better than 0.
2. **CashFlowService access**: The server's `app` struct already has `CashFlowService` — just need to use it in the handlers.
3. **Backward compatible**: All new fields are `omitempty` — existing clients unaffected.
4. **Format parameter**: The handler already accepts `format` but doesn't use it. Could add downsampling (weekly/monthly) as a follow-up.
