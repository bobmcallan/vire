# Requirements: Portfolio Changes Section

**Feature**: Add a `changes` section to `get_portfolio` response showing metric changes over time.

**Scope**:
- Add new `PortfolioChanges` struct with per-metric, per-period change tracking
- Populate from timeline snapshots (fast cache path) or ledger (fallback)
- Include: equity_value, portfolio_value, gross_cash, dividend_return
- Time periods: yesterday, 7 days, 30 days
- For each: raw change and % change

**Out of Scope**:
- Portal UI changes
- New API endpoints
- Changes to existing field names/values

---

## Files to Change

### 1. `internal/models/portfolio.go`

Add new structs after line 79 (after Portfolio struct):

```go
// MetricChange tracks raw and percentage change for a single metric.
type MetricChange struct {
	Current     float64 `json:"current"`               // Current value
	Previous    float64 `json:"previous"`              // Value at reference date
	RawChange   float64 `json:"raw_change"`            // Current - Previous
	PctChange   float64 `json:"pct_change,omitempty"`  // % change ((current - previous) / previous * 100)
	HasPrevious bool    `json:"has_previous"`          // True if historical data available
}

// PeriodChanges groups metric changes for a single time period.
type PeriodChanges struct {
	EquityValue   MetricChange `json:"equity_value"`   // Holdings market value
	PortfolioValue MetricChange `json:"portfolio_value"` // Total portfolio (equity + cash)
	GrossCash     MetricChange `json:"gross_cash"`     // Cash balance
	Dividend      MetricChange `json:"dividend"`       // Cumulative dividends received
}

// PortfolioChanges contains change tracking across multiple time periods.
// Computed on response from timeline snapshots — not persisted.
type PortfolioChanges struct {
	Yesterday PeriodChanges `json:"yesterday"` // Changes since yesterday close
	Week      PeriodChanges `json:"week"`      // Changes since 7 days ago
	Month     PeriodChanges `json:"month"`     // Changes since 30 days ago
}
```

Add to Portfolio struct after line 79:

```go
	// Change tracking — computed on response, not persisted
	Changes *PortfolioChanges `json:"changes,omitempty"`
```

### 2. `internal/models/timeline.go` (or add to portfolio.go if timeline.go doesn't exist)

Add to TimelineSnapshot struct (after line 297, before Metadata section):

```go
	// Cumulative dividend tracking
	CumulativeDividendReturn float64 `json:"cumulative_dividend_return,omitempty"` // Total dividends received up to this date
```

### 3. `internal/services/portfolio/service.go`

Add new function after `populateNetFlows()` (around line 725):

```go
// populateChanges computes the Changes section from timeline snapshots and ledger.
// Uses timeline cache (fast) when available; falls back to ledger computation.
func (s *Service) populateChanges(ctx context.Context, portfolio *models.Portfolio) {
	tl := s.storage.TimelineStore()
	userID := common.ResolveUserID(ctx)

	now := time.Now().Truncate(24 * time.Hour)
	dates := struct {
		yesterday time.Time
		week      time.Time
		month     time.Time
	}{
		yesterday: now.AddDate(0, 0, -1),
		week:      now.AddDate(0, 0, -7),
		month:     now.AddDate(0, 0, -30),
	}

	changes := &models.PortfolioChanges{}

	// Populate each period
	changes.Yesterday = s.computePeriodChanges(ctx, userID, portfolio, tl, dates.yesterday)
	changes.Week = s.computePeriodChanges(ctx, userID, portfolio, tl, dates.week)
	changes.Month = s.computePeriodChanges(ctx, userID, portfolio, tl, dates.month)

	portfolio.Changes = changes
}

// computePeriodChanges calculates metric changes for a single reference date.
func (s *Service) computePeriodChanges(ctx context.Context, userID string, portfolio *models.Portfolio, tl storage.TimelineStore, refDate time.Time) models.PeriodChanges {
	current := models.PeriodChanges{
		EquityValue: models.MetricChange{
			Current:     portfolio.EquityValue,
			HasPrevious: false,
		},
		PortfolioValue: models.MetricChange{
			Current:     portfolio.PortfolioValue,
			HasPrevious: false,
		},
		GrossCash: models.MetricChange{
			Current:     portfolio.GrossCashBalance,
			HasPrevious: false,
		},
		Dividend: models.MetricChange{
			Current:     portfolio.LedgerDividendReturn,
			HasPrevious: false,
		},
	}

	// Try timeline store first
	if tl != nil {
		snaps, err := tl.GetRange(ctx, userID, portfolio.Name, refDate, refDate)
		if err == nil && len(snaps) > 0 {
			snap := snaps[0]
			current.EquityValue = buildMetricChange(portfolio.EquityValue, snap.EquityValue)
			current.PortfolioValue = buildMetricChange(portfolio.PortfolioValue, snap.PortfolioValue)
			current.GrossCash = buildMetricChange(portfolio.GrossCashBalance, snap.GrossCashBalance)
			// Dividend: use snapshot if available, else compute from ledger below
			if snap.CumulativeDividendReturn > 0 || portfolio.LedgerDividendReturn > 0 {
				current.Dividend = buildMetricChange(portfolio.LedgerDividendReturn, snap.CumulativeDividendReturn)
			}
			return current
		}
	}

	// Fallback: compute dividend from ledger for the period
	if s.cashflowSvc != nil {
		ledger, err := s.cashflowSvc.GetLedger(ctx, portfolio.Name)
		if err == nil && ledger != nil {
			// Compute cumulative dividends up to refDate
			divToDate := cumulativeDividendsByDate(ledger, refDate)
			current.Dividend = buildMetricChange(portfolio.LedgerDividendReturn, divToDate)
		}
	}

	return current
}

// buildMetricChange creates a MetricChange from current and previous values.
func buildMetricChange(current, previous float64) models.MetricChange {
	mc := models.MetricChange{
		Current:     current,
		Previous:    previous,
		HasPrevious: previous > 0,
		RawChange:   current - previous,
	}
	if previous > 0 {
		mc.PctChange = ((current - previous) / previous) * 100
	}
	return mc
}

// cumulativeDividendsByDate returns total dividends received up to (and including) refDate.
func cumulativeDividendsByDate(ledger *models.CashFlowLedger, refDate time.Time) float64 {
	var total float64
	for _, tx := range ledger.Transactions {
		if tx.Category != models.CashCatDividend {
			continue
		}
		txDate := tx.Date.Truncate(24 * time.Hour)
		if txDate.Before(refDate) || txDate.Equal(refDate) {
			total += tx.Amount // dividends are positive credits
		}
	}
	return total
}
```

Update `populateAggregateValues()` (around line 560) to call the new function:

```go
	// Add after line 589 (after populateNetFlows call):
	s.populateChanges(ctx, portfolio)
```

### 4. `internal/services/portfolio/growth.go`

Update `persistTimelineSnapshots()` to include dividend (around line 714):

```go
	// In the loop that builds snapshots, add:
	// Note: We don't have dividend per-day in growth.go; that comes from ledger.
	// For now, leave CumulativeDividendReturn as 0 (computed on demand from ledger).
```

**Important**: The `TimelineSnapshot` struct needs the new field, but `persistTimelineSnapshots` in growth.go doesn't have access to dividend data. This is acceptable - the field will be 0 for historical snapshots, and the fallback path (`cumulativeDividendsByDate`) will compute from ledger.

### 5. `internal/services/portfolio/service.go` - Update writeTodaySnapshot

Update `writeTodaySnapshot()` (around line 759) to include dividend:

```go
	snap := models.TimelineSnapshot{
		// ... existing fields ...
		CumulativeDividendReturn: portfolio.LedgerDividendReturn, // NEW LINE
	}
```

---

## Test Cases

### Unit Tests (`internal/services/portfolio/service_test.go`)

1. **TestPopulateChanges_TimelineHit** - Timeline has snapshot, verify all metrics populated
2. **TestPopulateChanges_TimelineMiss_LedgerFallback** - No timeline, ledger provides dividend
3. **TestPopulateChanges_NoTimelineNoLedger** - All HasPrevious = false
4. **TestBuildMetricChange_ZeroPrevious** - PctChange = 0 when previous is 0
5. **TestBuildMetricChange_NegativeChange** - Raw/Pct correctly negative
6. **TestCumulativeDividendsByDate** - Correct sum up to reference date
7. **TestPopulateChanges_AllPeriods** - Yesterday, Week, Month all computed

### Integration Tests (`tests/api/portfolio_changes_test.go`)

1. **TestGetPortfolio_ChangesSection** - End-to-end: GET /portfolio returns changes section
2. **TestGetPortfolio_ChangesAfterSync** - Sync then verify changes populated

---

## Integration Points

1. **Call chain**: `GetPortfolio()` → `SyncPortfolio()` → `populateAggregateValues()` → `populateChanges()`
2. **Dependencies**: TimelineStore (optional), CashFlowService (optional)
3. **Response**: New `changes` field in Portfolio JSON

---

## Notes

- **"Yesterday close (localhost time)"**: Use `time.Now().Truncate(24 * time.Hour).AddDate(0, 0, -1)` which is based on server local time (AUD)
- **7 days / 30 days**: Use `AddDate(0, 0, -7)` and `AddDate(0, 0, -30)`
- **Schema version**: NOT bumped (no persisted data changes, only response-time computation)
- **Backward compatible**: `changes` field is `omitempty` - old clients ignore it
