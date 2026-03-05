# Requirements: Include Closed Positions in Portfolio Get

## Scope

Add an `include_closed` boolean query parameter to the portfolio get API endpoint and MCP tool. Default is `false` — closed positions (units <= 0) are filtered out. When `true`, all holdings are returned including closed ones.

**In scope:**
- Handler: parse `include_closed` query param, filter holdings
- Manual portfolio assembly: include closed positions in holdings array (but don't count in aggregates)
- MCP catalog: add `include_closed` param to `portfolio_get` tool definition
- Unit tests for the handler filtering logic
- Integration test for include_closed behavior

**Out of scope:**
- No changes to Navexa sync (already stores closed positions with status="closed")
- No changes to portfolio review (already has its own closed-position handling)
- No changes to timeline/growth (only uses open positions)

## Current Behavior

1. **Navexa portfolios**: `SyncPortfolio` stores ALL holdings including closed ones (status="closed", units=0). `GetPortfolio` returns them all. The handler (`handlePortfolioGet`) returns everything unfiltered.

2. **Manual portfolios**: `assembleManualPortfolio` (service.go:724-726) skips closed positions entirely with `if dh.Units <= 0 { continue }`. They never appear in the response.

3. **Holding.Status** field (models/portfolio.go:120): already set to `"open"` or `"closed"` by service.go:454-459.

## Files to Change

### 1. `internal/server/handlers.go` — Handler filtering (lines 111-164)

After getting the portfolio (line 125) and stripping trades (lines 132-134), add filtering:

```go
// Parse include_closed query param (default: false)
includeClosed := r.URL.Query().Get("include_closed") == "true"

// Filter out closed positions unless explicitly requested
if !includeClosed {
    open := make([]models.Holding, 0, len(portfolio.Holdings))
    for _, h := range portfolio.Holdings {
        if h.Units > 0 {
            open = append(open, h)
        }
    }
    portfolio.Holdings = open
}
```

Insert this AFTER the trades stripping loop (line 134) and BEFORE the capital performance attachment (line 137). The filtering must happen before WriteJSON but after all service-layer computation is done.

### 2. `internal/services/portfolio/service.go` — Manual portfolio: include closed positions

In `assembleManualPortfolio` (line 713), change the loop to include closed positions in the holdings array but NOT in the aggregate calculations:

Replace lines 724-771 with:

```go
for _, dh := range derived {
    h := models.Holding{
        Ticker:           dh.Ticker,
        Exchange:         models.EodhExchange(tickerExchange(dh.Ticker)),
        Name:             dh.Ticker,
        Units:            dh.Units,
        AvgCost:          dh.AvgCost,
        CostBasis:        dh.CostBasis,
        GrossInvested:    dh.GrossInvested,
        GrossProceeds:    dh.GrossProceeds,
        RealizedReturn:   dh.RealizedReturn,
        UnrealizedReturn: dh.UnrealizedReturn,
        SourceType:       models.SourceManual,
        Currency:         portfolio.Currency,
    }

    if dh.Units > 0 {
        h.Status = "open"
        h.CurrentPrice = dh.AvgCost
        h.MarketValue = dh.AvgCost * dh.Units

        // Try to enrich with current market price
        if s.eodhd != nil {
            if md, err := s.storage.MarketDataStorage().GetMarketData(ctx, dh.Ticker); err == nil && md != nil && len(md.EOD) > 0 {
                latestPrice := md.EOD[len(md.EOD)-1].Close
                if latestPrice > 0 {
                    h.CurrentPrice = latestPrice
                    h.MarketValue = latestPrice * dh.Units
                    h.UnrealizedReturn = h.MarketValue - dh.CostBasis
                }
            }
        }

        h.ReturnNet = h.RealizedReturn + h.UnrealizedReturn
        if h.GrossInvested > 0 {
            h.ReturnNetPct = (h.ReturnNet / h.GrossInvested) * 100
        }

        // Only open positions count toward aggregates
        totalEquityValue += h.MarketValue
        totalCost += h.CostBasis
        totalRealized += h.RealizedReturn
        totalUnrealized += h.UnrealizedReturn
        totalGrossInvested += h.GrossInvested
    } else {
        h.Status = "closed"
        // Closed: realized return is the final P&L
        h.ReturnNet = h.RealizedReturn
        if h.GrossInvested > 0 {
            h.ReturnNetPct = (h.ReturnNet / h.GrossInvested) * 100
        }
    }

    holdings = append(holdings, h)
}
```

Key: closed positions are appended to `holdings` but do NOT affect `totalEquityValue`, `totalCost`, etc.

### 3. `internal/server/catalog.go` — MCP param definition (line 376-390)

Add a new param after `force_refresh`:

```go
{
    Name:        "include_closed",
    Type:        "boolean",
    Description: "Include closed positions (units = 0) in the holdings array (default: false)",
    In:          "query",
},
```

Also update the tool description (line 373) to mention: "By default, only open positions (units > 0) are returned. Set include_closed=true to include closed (fully sold) positions."

### 4. `internal/server/handlers_portfolio_test.go` — Unit tests

Add these test functions after `TestHandlePortfolioGet_ReturnsPortfolio` (line 181):

**Test: closed positions filtered by default**
```go
func TestHandlePortfolioGet_ExcludesClosedByDefault(t *testing.T) {
    portfolio := &models.Portfolio{
        Name: "test",
        Holdings: []models.Holding{
            {Ticker: "BHP.AU", Units: 100, Status: "open"},
            {Ticker: "CBA.AU", Units: 0, Status: "closed"},
            {Ticker: "NAB.AU", Units: 50, Status: "open"},
        },
    }
    svc := &mockPortfolioService{
        getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
            return portfolio, nil
        },
    }
    srv := newTestServer(svc)
    req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test", nil)
    rec := httptest.NewRecorder()
    srv.handlePortfolioGet(rec, req, "test")

    require.Equal(t, http.StatusOK, rec.Code)
    var got models.Portfolio
    require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
    assert.Len(t, got.Holdings, 2, "closed positions should be filtered out by default")
    for _, h := range got.Holdings {
        assert.True(t, h.Units > 0, "all returned holdings should have units > 0")
    }
}
```

**Test: include_closed=true returns all**
```go
func TestHandlePortfolioGet_IncludesClosedWhenRequested(t *testing.T) {
    portfolio := &models.Portfolio{
        Name: "test",
        Holdings: []models.Holding{
            {Ticker: "BHP.AU", Units: 100, Status: "open"},
            {Ticker: "CBA.AU", Units: 0, Status: "closed"},
            {Ticker: "NAB.AU", Units: 50, Status: "open"},
        },
    }
    svc := &mockPortfolioService{
        getPortfolio: func(ctx context.Context, name string) (*models.Portfolio, error) {
            return portfolio, nil
        },
    }
    srv := newTestServer(svc)
    req := httptest.NewRequest(http.MethodGet, "/api/portfolios/test?include_closed=true", nil)
    rec := httptest.NewRecorder()
    srv.handlePortfolioGet(rec, req, "test")

    require.Equal(t, http.StatusOK, rec.Code)
    var got models.Portfolio
    require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
    assert.Len(t, got.Holdings, 3, "all holdings should be returned when include_closed=true")
    // Verify closed position is present
    var foundClosed bool
    for _, h := range got.Holdings {
        if h.Status == "closed" {
            foundClosed = true
        }
    }
    assert.True(t, foundClosed, "should include the closed position")
}
```

**Test: default behavior unchanged for existing API calls**
Follow the pattern at `handlers_portfolio_test.go:148-181` — same mock setup, verify no regression.

### 5. Integration test: `tests/data/portfolio_closed_positions_test.go`

Create integration test that:
1. Creates a manual portfolio via trade service
2. Adds BUY + SELL trades to fully close a position
3. Calls `GetPortfolio` — verifies closed position is in holdings array with status="closed"
4. This validates the service-layer change (assembleManualPortfolio now includes closed positions)

Follow patterns from `tests/data/trade_test.go` — uses `testManager(t)`, `testContext()`, `common.WithUserContext()`.

## Integration Points

- Handler filtering: insert between line 134 (trades strip) and line 137 (capital perf) in handlers.go
- Service change: replace lines 724-727 in service.go with open/closed branching
- Catalog: add param at line 389 in catalog.go (after force_refresh param)
- No new interfaces or dependencies needed
