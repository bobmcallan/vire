# Requirements: Portfolio Breadth Summary (Server-Side)

## Context

Feedback `fb_411cb26f` requests two features:
1. **Image attachments on feedback** — Already shipped in v0.3.171. Mark resolved.
2. **Portfolio breadth bar with per-holding trend signals** — Split into server + portal. This task covers server only.

## Scope

### In scope (server)
- Add `PortfolioBreadth` struct to the portfolio model
- Add `Breadth *PortfolioBreadth` field to the `Portfolio` struct
- Compute breadth from holdings after signal enrichment in `populateHistoricalValues`
- Return breadth data in the `portfolio_get` API response

### Out of scope (portal — will create feedback)
- Breadth bar UI component (red→grey→green dollar-weighted gradient)
- Per-holding trend signal display (arrow + label + $ change)
- Portfolio-level aggregated signal label rendering

---

## Design

### PortfolioBreadth Struct

Add to `internal/models/portfolio.go` after `PortfolioChanges`:

```go
// PortfolioBreadth aggregates holding trend signals into a portfolio-level breadth summary.
// Computed on response from holdings that have trend data — not persisted.
type PortfolioBreadth struct {
	// Counts by trend direction
	RisingCount  int `json:"rising_count"`
	FlatCount    int `json:"flat_count"`
	FallingCount int `json:"falling_count"`

	// Dollar-weighted proportions (0.0 to 1.0, sum to 1.0)
	RisingWeight  float64 `json:"rising_weight"`
	FlatWeight    float64 `json:"flat_weight"`
	FallingWeight float64 `json:"falling_weight"`

	// Dollar amounts by direction
	RisingValue  float64 `json:"rising_value"`
	FlatValue    float64 `json:"flat_value"`
	FallingValue float64 `json:"falling_value"`

	// Portfolio-level trend
	TrendLabel string  `json:"trend_label"` // "Strong Uptrend", "Uptrend", "Mixed", "Downtrend", "Strong Downtrend"
	TrendScore float64 `json:"trend_score"` // Dollar-weighted average of holding trend scores (-1.0 to +1.0)

	// Today's aggregate change
	TodayChange    float64 `json:"today_change"`     // Sum of (yesterday_price_change_pct / 100 * market_value) across holdings
	TodayChangePct float64 `json:"today_change_pct"` // Weighted % change
}
```

### Trend Direction Classification

Map existing `TrendLabel` to direction buckets:
- **Rising**: "Strong Uptrend", "Uptrend"
- **Flat**: "Consolidating", "" (no signal)
- **Falling**: "Downtrend", "Strong Downtrend"

### Portfolio-Level Trend Label

Derived from dollar-weighted `TrendScore`:
- score >= 0.4: "Strong Uptrend"
- score >= 0.15: "Uptrend"
- score > -0.15: "Mixed"
- score > -0.4: "Downtrend"
- else: "Strong Downtrend"

### Today's Change Computation

For each open holding with `YesterdayClosePrice > 0`:
- Holding $ change = `(CurrentPrice - YesterdayClosePrice) * Units`
- Sum across all holdings → `TodayChange`
- `TodayChangePct` = `TodayChange / sum(MarketValue) * 100` (only if total > 0)

---

## Files to Change

### 1. `internal/models/portfolio.go`

Add `PortfolioBreadth` struct (as above) after `PortfolioChanges` struct (after line 111).

Add field to `Portfolio` struct after `Changes` field (line 84):
```go
Breadth            *PortfolioBreadth `json:"breadth,omitempty"`
```

### 2. `internal/services/portfolio/service.go`

Add `computeBreadth` method after `populateNetFlows` (after line ~1100).

Call it from `populateHistoricalValues` after `populateFromMarketData` (line 938). Insert after line 938:
```go
// Compute breadth summary from holdings with trend data
s.computeBreadth(portfolio)
```

Implementation — follow the pattern of `populateNetFlows` (short, focused helper):

```go
// computeBreadth aggregates holding trend signals into a portfolio-level breadth summary.
// Only considers open holdings with trend data.
func (s *Service) computeBreadth(portfolio *models.Portfolio) {
	var (
		risingVal, flatVal, fallingVal float64
		risingCount, flatCount, fallingCount int
		weightedScore, totalWeight float64
		todayChange float64
	)

	for i := range portfolio.Holdings {
		h := &portfolio.Holdings[i]
		if h.Status != "open" || h.MarketValue == 0 {
			continue
		}

		mv := h.MarketValue

		// Classify direction from TrendLabel
		switch h.TrendLabel {
		case "Strong Uptrend", "Uptrend":
			risingVal += mv
			risingCount++
		case "Downtrend", "Strong Downtrend":
			fallingVal += mv
			fallingCount++
		default: // "Consolidating" or no signal
			flatVal += mv
			flatCount++
		}

		// Dollar-weighted trend score
		if h.TrendScore != 0 {
			weightedScore += h.TrendScore * mv
			totalWeight += mv
		}

		// Today's dollar change
		if h.YesterdayClosePrice > 0 {
			todayChange += (h.CurrentPrice - h.YesterdayClosePrice) * h.Units
		}
	}

	total := risingVal + flatVal + fallingVal
	if total == 0 {
		return // No open holdings with market value
	}

	score := 0.0
	if totalWeight > 0 {
		score = weightedScore / totalWeight
	}

	portfolio.Breadth = &models.PortfolioBreadth{
		RisingCount:  risingCount,
		FlatCount:    flatCount,
		FallingCount: fallingCount,
		RisingWeight:  risingVal / total,
		FlatWeight:    flatVal / total,
		FallingWeight: fallingVal / total,
		RisingValue:  risingVal,
		FlatValue:    flatVal,
		FallingValue: fallingVal,
		TrendLabel:   breadthTrendLabel(score),
		TrendScore:   score,
		TodayChange:    todayChange,
		TodayChangePct: todayChange / total * 100,
	}
}

// breadthTrendLabel maps a dollar-weighted trend score to a plain-English label.
func breadthTrendLabel(score float64) string {
	switch {
	case score >= 0.4:
		return "Strong Uptrend"
	case score >= 0.15:
		return "Uptrend"
	case score > -0.15:
		return "Mixed"
	case score > -0.4:
		return "Downtrend"
	default:
		return "Strong Downtrend"
	}
}
```

### 3. `internal/server/catalog.go`

No changes needed. The `portfolio_get` tool definition already returns the full Portfolio struct. `breadth` will be included automatically via the `omitempty` field.

---

## Test Cases

### Unit tests (implementer writes)

In `internal/services/portfolio/breadth_test.go`:

1. `TestComputeBreadth_AllRising` — all holdings uptrend → rising_weight = 1.0
2. `TestComputeBreadth_MixedTrends` — mix of up/flat/down → correct counts, weights, score
3. `TestComputeBreadth_NoOpenHoldings` — closed holdings only → breadth is nil
4. `TestComputeBreadth_NoTrendData` — holdings without trend labels → all classified as flat
5. `TestComputeBreadth_DollarWeighting` — large position dominates smaller ones in weights and score
6. `TestComputeBreadth_TodayChange` — correct dollar change calculation from yesterday prices
7. `TestBreadthTrendLabel_Boundaries` — test all 5 thresholds for label mapping

### Integration tests (test-creator writes)

In `tests/data/portfolio_breadth_test.go`:
1. `TestPortfolio_BreadthIncludedInResponse` — portfolio_get includes breadth field
2. `TestPortfolio_BreadthCounts` — verify counts match number of holdings by direction
3. `TestPortfolio_BreadthWeightsSumToOne` — weights sum to ~1.0

---

## Post-Implementation: Feedback Updates

After server work is complete, the team lead will:

1. **Update `fb_411cb26f`** — mark Feature 1 (attachments) resolved, note Feature 2 server-side complete
2. **Create new feedback** for portal work with these requirements:
   - Breadth bar component using `portfolio.breadth` data
   - Per-holding trend display using existing `trend_label` + `yesterday_price_change_pct` + `holding_value_market`
   - Dollar-weighted gradient bar (red→grey→green) using `rising_weight`/`flat_weight`/`falling_weight`
   - Portfolio-level trend label from `breadth.trend_label`
   - Today's total change from `breadth.today_change`
