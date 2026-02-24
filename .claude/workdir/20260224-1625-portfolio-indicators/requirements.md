# Requirements: Split total_value (fb_6cac832d) & Portfolio-level indicators (fb_79536bca)

**Date:** 2026-02-24
**Requested:** Rename total_value → total_value_holdings, add combined total_value field, and implement portfolio-level technical indicators (RSI, EMA 20/50/200)

## Feedback Items

| ID | Severity | Issue |
|----|----------|-------|
| fb_6cac832d | medium | Split total_value into total_value_holdings (equity only) and total_value (equity + external balances) |
| fb_79536bca | medium | Portfolio-level RSI, EMA 20/50/200 on daily portfolio value time series |

## Scope

### In scope
1. Rename `TotalValue` → `TotalValueHoldings` in `Portfolio` model (json: `total_value_holdings`)
2. Add new `TotalValue` field = holdings + external balances (json: `total_value`)
3. Update all references: sync, review, cashflow, growth, snapshot, chart, report, handlers
4. Implement `GetPortfolioIndicators` — compute RSI(14), EMA 20/50/200 on daily portfolio value time series
5. New endpoint: `GET /api/portfolios/{name}/indicators`
6. New MCP tool: `get_portfolio_indicators`
7. Include portfolio indicators in `portfolio_compliance` response
8. Bump SchemaVersion to force re-sync (TotalValue semantics changed)

### Out of scope
- Persistent daily value snapshots (the existing `GetDailyGrowth` computes values on-demand from trade history + EOD bars — this is sufficient)
- Portal dashboard charts (portal work is separate)

## Approach

### Part 1: Split total_value (fb_6cac832d)

**Current state:** `Portfolio.TotalValue` = sum of holding market values (equity only). External balance total is a separate field. Many places manually add them: `portfolio.TotalValue + portfolio.ExternalBalanceTotal`.

**Change:** Rename `TotalValue` → `TotalValueHoldings` (equity-only). Add new `TotalValue` = `TotalValueHoldings + ExternalBalanceTotal` (true total). This eliminates the need for callers to manually add external balances.

**Model changes in `internal/models/portfolio.go`:**

```go
// Portfolio struct:
TotalValueHoldings float64 `json:"total_value_holdings"` // equity holdings only (was total_value)
TotalValue         float64 `json:"total_value"`          // holdings + external balances

// PortfolioReview struct — already includes external balances in its TotalValue, no rename needed

// PortfolioSnapshot — keep TotalValue as-is (historical snapshots don't include external balances)

// GrowthDataPoint — keep TotalValue as-is (growth doesn't include external balances currently)
```

**Service changes in `internal/services/portfolio/service.go`:**

In `SyncPortfolio` (line ~387):
- Set `p.TotalValueHoldings = totalValue` (sum of holdings)
- Set `p.TotalValue = totalValue + p.ExternalBalanceTotal`

In `ReviewPortfolio` (line ~483, ~651):
- Already computes `review.TotalValue = portfolio.TotalValue + portfolio.ExternalBalanceTotal`
- After the rename, `portfolio.TotalValue` already includes external balances, so simplify to `review.TotalValue = portfolio.TotalValue`
- Actually: ReviewPortfolio recomputes from live prices. It accumulates `liveTotal` from live holding values. So:
  - `review.TotalValue = liveTotal + portfolio.ExternalBalanceTotal` (stays the same conceptually)

**Cashflow service** (`internal/services/cashflow/service.go` line 251):
- Currently: `currentValue := portfolio.TotalValue + portfolio.ExternalBalanceTotal`
- After: `currentValue := portfolio.TotalValue` (already includes external balances)

**External balances** (`internal/services/portfolio/external_balances.go`):
- `recomputeHoldingWeights` uses `totalMarketValue + p.ExternalBalanceTotal` — no change needed (it computes from scratch)
- After changing balances, need to also update `p.TotalValue = p.TotalValueHoldings + p.ExternalBalanceTotal`

**Slim review handler** (`internal/server/handlers.go`):
- `slimPortfolioReview.TotalValue` already maps to `PortfolioReview.TotalValue` — no change needed

**SchemaVersion bump** (`internal/common/version.go`):
- Bump to "9" — the JSON field rename from `total_value` to `total_value_holdings` means cached portfolio records have the wrong field. A version bump forces re-sync.

### Part 2: Portfolio Indicators (fb_79536bca)

**Approach:** Use the existing `GetDailyGrowth` to compute the portfolio value time series on-demand, then run the existing `signals.RSI()`, `signals.EMA()`, `signals.SMA()` indicator functions on the resulting bars.

**Key insight:** `GetDailyGrowth` already computes daily portfolio value from trade history + EOD bars. We convert the resulting `[]GrowthDataPoint` to `[]models.EODBar` (setting Close = TotalValue) and call the existing indicator functions. No new storage needed — indicators are computed on-demand.

**For external balances in the growth series:** The current `GetDailyGrowth` only computes equity values. For portfolio indicators, we need to add external balance total to each data point. We can do this by simply adding `portfolio.ExternalBalanceTotal` to each point's value (assumes external balances are relatively stable — they're manually set and don't have daily history).

**New model in `internal/models/portfolio.go`:**

```go
// PortfolioIndicators contains technical indicators computed on portfolio value time series.
type PortfolioIndicators struct {
    PortfolioName    string    `json:"portfolio_name"`
    ComputeDate      time.Time `json:"compute_date"`
    CurrentValue     float64   `json:"current_value"`      // latest total portfolio value
    DataPoints       int       `json:"data_points"`         // number of daily values used

    // Moving Averages
    EMA20            float64   `json:"ema_20"`
    EMA50            float64   `json:"ema_50"`
    EMA200           float64   `json:"ema_200"`
    AboveEMA20       bool      `json:"above_ema_20"`
    AboveEMA50       bool      `json:"above_ema_50"`
    AboveEMA200      bool      `json:"above_ema_200"`

    // RSI
    RSI              float64   `json:"rsi"`
    RSISignal        string    `json:"rsi_signal"`          // oversold, neutral, overbought

    // Crossovers
    EMA50CrossEMA200 string    `json:"ema_50_cross_200"`    // golden_cross, death_cross, none

    // Trend
    Trend            TrendType `json:"trend"`
    TrendDescription string    `json:"trend_description"`
}
```

**New method in `internal/services/portfolio/service.go`:**

```go
func (s *Service) GetPortfolioIndicators(ctx context.Context, name string) (*models.PortfolioIndicators, error) {
    // 1. Get current portfolio for TotalValue and ExternalBalanceTotal
    portfolio, err := s.getPortfolioRecord(ctx, name)

    // 2. Compute daily growth (equity only)
    growth, err := s.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})

    // 3. Convert to EOD bars (add external balance total to each point)
    bars := growthToBars(growth, portfolio.ExternalBalanceTotal)

    // 4. Compute indicators using existing signals package
    indicators := &models.PortfolioIndicators{
        PortfolioName: name,
        ComputeDate:   time.Now(),
        CurrentValue:  portfolio.TotalValue,
        DataPoints:    len(bars),
    }

    if len(bars) >= 20 {
        indicators.EMA20 = signals.EMA(bars, 20)
        indicators.AboveEMA20 = portfolio.TotalValue > indicators.EMA20
    }
    if len(bars) >= 50 {
        indicators.EMA50 = signals.EMA(bars, 50)
        indicators.AboveEMA50 = portfolio.TotalValue > indicators.EMA50
    }
    if len(bars) >= 200 {
        indicators.EMA200 = signals.EMA(bars, 200)
        indicators.AboveEMA200 = portfolio.TotalValue > indicators.EMA200
    }
    if len(bars) >= 14 {
        indicators.RSI = signals.RSI(bars, 14)
        indicators.RSISignal = signals.ClassifyRSI(indicators.RSI)
    }

    // Crossover detection (EMA50 vs EMA200)
    if len(bars) >= 200 {
        indicators.EMA50CrossEMA200 = signals.DetectCrossover(bars, 50, 200)
    }

    // Trend
    sma20 := signals.SMA(bars, 20)
    sma50 := signals.SMA(bars, 50)
    sma200 := signals.SMA(bars, 200)
    indicators.Trend = signals.DetermineTrend(portfolio.TotalValue, sma20, sma50, sma200)
    // ... trend description

    return indicators, nil
}
```

**Helper function:**
```go
func growthToBars(points []models.GrowthDataPoint, externalBalanceTotal float64) []models.EODBar {
    // EOD bars are newest-first, growth points are oldest-first — reverse
    bars := make([]models.EODBar, len(points))
    for i, p := range points {
        value := p.TotalValue + externalBalanceTotal
        bars[len(points)-1-i] = models.EODBar{
            Date:  p.Date,
            Close: value,
            Open:  value, High: value, Low: value, // Only Close matters for EMA/RSI/SMA
        }
    }
    return bars
}
```

**New interface method in `internal/interfaces/services.go`:**
```go
GetPortfolioIndicators(ctx context.Context, name string) (*models.PortfolioIndicators, error)
```

**New endpoint: `GET /api/portfolios/{name}/indicators`**
Route in `routes.go`, handler in `handlers.go`.

**New MCP tool in `catalog.go`:**
```go
{
    Name:        "get_portfolio_indicators",
    Description: "Get portfolio-level technical indicators (RSI, EMA 20/50/200) computed on daily portfolio value time series.",
    Method:      "GET",
    Path:        "/api/portfolios/{portfolio_name}/indicators",
    Params:      []models.ParamDefinition{portfolioParam},
}
```

**Include in portfolio_compliance:** Add `PortfolioIndicators *PortfolioIndicators` to `PortfolioReview` model. In `ReviewPortfolio`, compute indicators and attach to the review. The slim review handler should pass through the indicators field.

### Important: DetectCrossover uses SMA, not EMA

The existing `signals.DetectCrossover(bars, short, long)` computes SMA crossovers. For EMA crossovers (golden/death cross), we need a simple custom check: compare current EMA50 vs EMA200 and previous-day EMA50 vs EMA200. This is straightforward — just compute EMAs on bars[0:] and bars[1:] and compare.

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Rename TotalValue → TotalValueHoldings in Portfolio, add new TotalValue; add PortfolioIndicators struct; add PortfolioIndicators field to PortfolioReview |
| `internal/services/portfolio/service.go` | Update SyncPortfolio, ReviewPortfolio to set both fields; add GetPortfolioIndicators method; add growthToBars helper |
| `internal/services/portfolio/external_balances.go` | Update TotalValue after external balance changes |
| `internal/services/portfolio/snapshot.go` | Rename TotalValue → TotalValueHoldings in snapshot building (if applicable) |
| `internal/services/portfolio/growth.go` | No change — growth points stay as equity-only |
| `internal/services/portfolio/chart.go` | Update TotalValue reference |
| `internal/services/cashflow/service.go` | Simplify to use portfolio.TotalValue directly |
| `internal/services/report/formatter.go` | Update TotalValue reference |
| `internal/interfaces/services.go` | Add GetPortfolioIndicators to PortfolioService |
| `internal/server/handlers.go` | Add handlePortfolioIndicators; update slimPortfolioReview to include indicators; update toSlimReview |
| `internal/server/routes.go` | Add indicators route |
| `internal/server/catalog.go` | Add get_portfolio_indicators MCP tool |
| `internal/common/version.go` | Bump SchemaVersion to "9" |
| `internal/services/portfolio/service_test.go` | Unit tests for GetPortfolioIndicators, growthToBars |
| `internal/services/portfolio/indicators_test.go` | Dedicated indicator tests |

## Acceptance Criteria

1. `get_portfolio` returns both `total_value_holdings` (equity) and `total_value` (equity + external)
2. `total_value` = `total_value_holdings` + `external_balance_total` everywhere
3. `get_capital_performance` uses `total_value` directly (no manual addition)
4. `get_portfolio_indicators` returns RSI, EMA 20/50/200, crossover status, trend
5. Portfolio indicators are computed from daily growth data + external balance total
6. Indicators included in `portfolio_compliance` response
7. Insufficient data (< 200 days for EMA200) returns zero values gracefully
8. All existing tests pass
9. fb_6cac832d and fb_79536bca marked resolved
