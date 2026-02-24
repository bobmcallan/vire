# Requirements: Resolve fb_6337c9d1, implement review_watchlist (fb_761d7753)

**Date:** 2026-02-24
**Requested:** Resolve capital flow feedback, implement watchlist review feature

## Feedback Items

| ID | Severity | Issue |
|----|----------|-------|
| fb_6337c9d1 | medium | Capital flow tracking — already implemented, resolve feedback |
| fb_761d7753 | low | Watchlist review — run signals/compliance across watchlist tickers |

## Scope

### In scope
1. Resolve fb_6337c9d1 (capital flow tracking is already implemented)
2. Implement `ReviewWatchlist` — run the same signal/compliance pipeline as `portfolio_compliance` but for watchlist tickers instead of portfolio holdings
3. New API endpoint: `POST /api/portfolios/{name}/watchlist/review`
4. New MCP tool: `review_watchlist`

### Out of scope
- Changes to existing watchlist CRUD (already working)
- Changes to existing cash flow tools (already working)

## Approach

### ReviewWatchlist Implementation

Add a `ReviewWatchlist` method to the portfolio service (`internal/services/portfolio/service.go`). The portfolio service already has all required dependencies: storage (market data, signals), eodhd (live quotes), signalComputer, and strategy access.

**Flow** (mirrors ReviewPortfolio phases but for watchlist tickers):

1. Load watchlist via storage (UserDataStore, subject "watchlist")
2. Load strategy for the portfolio
3. Extract EODHD tickers from watchlist items (item.Ticker is already in EODHD format like "BHP.AU")
4. Batch load market data for all watchlist tickers
5. Fetch real-time quotes for all watchlist tickers
6. For each watchlist item with market data:
   - Get or compute signals (same as ReviewPortfolio)
   - Calculate overnight movement from EOD bars / live quotes
   - Determine action using `determineAction()` (pass nil for holding — watchlist items aren't held)
   - Run compliance check via `strategypkg.CheckCompliance()` (pass nil holding)
   - Build a `WatchlistItemReview` struct
7. Return a `WatchlistReview` with all item reviews

**New model structs** in `internal/models/watchlist.go`:

```go
type WatchlistItemReview struct {
    Item           WatchlistItem    `json:"item"`
    Signals        *TickerSignals   `json:"signals,omitempty"`
    Fundamentals   *Fundamentals    `json:"fundamentals,omitempty"`
    OvernightMove  float64          `json:"overnight_move"`
    OvernightPct   float64          `json:"overnight_pct"`
    ActionRequired string           `json:"action_required"`
    ActionReason   string           `json:"action_reason"`
    Compliance     *ComplianceResult `json:"compliance,omitempty"`
}

type WatchlistReview struct {
    PortfolioName string               `json:"portfolio_name"`
    ReviewDate    time.Time            `json:"review_date"`
    ItemReviews   []WatchlistItemReview `json:"item_reviews"`
    Alerts        []Alert              `json:"alerts,omitempty"`
    Summary       string               `json:"summary,omitempty"`
}
```

**Key difference from ReviewPortfolio:** Watchlist items have no units, no cost basis, no market value. The `determineAction()` function accepts a `*Holding` parameter — pass nil for watchlist items. The function already nil-checks the holding for position sizing rules (line 744), so this is safe.

Actually, `determineAction` takes `*models.Holding` but only uses it for `holding.Weight` in position sizing check (line 745). Passing nil is safe since the strategy check guards with `holding != nil` (line 744). Similarly, `generateAlerts` takes `models.Holding` (not pointer) for similar checks. For watchlist items, we can construct a minimal Holding with just the Ticker for alert generation.

### API Endpoint

**`POST /api/portfolios/{name}/watchlist/review`**

Handler in `internal/server/handlers.go` (or a dedicated `handlers_watchlist.go`):
```go
func (s *Server) handleWatchlistReview(w http.ResponseWriter, r *http.Request, name string) {
    // Parse request body (same as portfolio review)
    var req struct {
        FocusSignals []string `json:"focus_signals"`
        IncludeNews  bool     `json:"include_news"`
    }
    // Call ReviewWatchlist
    // Return WatchlistReview
}
```

Route: Add to `routeWatchlist` in routes.go:
```go
case subpath == "review":
    s.handleWatchlistReview(w, r, portfolioName)
```

### MCP Tool

Add to `catalog.go`:
```go
{
    Name:        "review_watchlist",
    Description: "Review watchlist stocks for signals, overnight movement, and actionable observations. Runs the same signal/compliance pipeline as portfolio_compliance but for watchlist tickers.",
    Method:      "POST",
    Path:        "/api/portfolios/{portfolio_name}/watchlist/review",
    Params: []models.ParamDefinition{
        portfolioParam,
        {Name: "focus_signals", Type: "array", In: "body"},
        {Name: "include_news", Type: "boolean", In: "body"},
    },
}
```

### Interface Update

Add `ReviewWatchlist` to the `PortfolioService` interface in `internal/interfaces/services.go`:
```go
ReviewWatchlist(ctx context.Context, name string, options ReviewOptions) (*models.WatchlistReview, error)
```

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/watchlist.go` | Add WatchlistItemReview, WatchlistReview structs |
| `internal/services/portfolio/service.go` | Add ReviewWatchlist method |
| `internal/interfaces/services.go` | Add ReviewWatchlist to PortfolioService interface |
| `internal/server/routes.go` | Add watchlist/review route |
| `internal/server/handlers.go` | Add handleWatchlistReview handler |
| `internal/server/catalog.go` | Add review_watchlist MCP tool |
| `internal/services/portfolio/service_test.go` | Unit tests for ReviewWatchlist |

## Acceptance Criteria

1. `review_watchlist` MCP tool returns signal analysis for each watchlist ticker
2. Each item review includes: signals, overnight movement, action/compliance status
3. Strategy-aware compliance checks work the same as portfolio_compliance
4. Missing market data for a ticker is non-fatal (item included with "Market data unavailable")
5. fb_6337c9d1 marked resolved
6. fb_761d7753 marked resolved
7. All existing tests pass
