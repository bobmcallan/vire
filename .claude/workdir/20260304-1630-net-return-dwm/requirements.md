# Requirements: Net Return D/W/M Period Breakdowns

## Feedback Items
- fb_83fcf232 (MEDIUM): Net Return $ and Net Return % show no D/W/M breakdown — add period sub-labels
- fb_9537e416 (MEDIUM): D/W/M on Holdings Value (equity_value) is not useful — move to Net Return, remove from equity

## What the User Wants

The dashboard currently shows D/W/M change bars under "Holdings Value" (equity_value) via `Changes.Yesterday.EquityValue`, etc. This is not actionable — equity value moves because of prices.

The user wants D/W/M on **Net Return $ and Net Return %** instead:
- "Net Return $" = `portfolio.NetEquityReturn` (unrealized P&L: equity_value - net_equity_cost)
- "Net Return %" = `portfolio.NetEquityReturnPct` ((net_equity_return / net_equity_cost) * 100)

## Scope

### In Scope
1. **Add**: `NetEquityReturn` and `NetEquityReturnPct` MetricChange fields to `PeriodChanges`
2. **Remove**: `EquityValue` MetricChange field from `PeriodChanges`
3. **Fix**: `buildMetricChange` doesn't handle negative values (P&L can be negative)
4. **Compute**: Look up previous values from `TimelineSnapshot` (already has `NetEquityReturn` and `NetEquityReturnPct`)

### Out of Scope
- Portfolio-level fields (`PortfolioYesterdayValue`, etc.) — unchanged
- `NetCapitalReturn` D/W/M — user hasn't requested this
- Per-holding D/W/M changes — not requested
- Portal changes — separate repo, will consume new JSON fields
- Glossary changes — glossary covers Portfolio fields, not Changes struct

---

## Files to Change

### 1. `internal/models/portfolio.go`

**Modify PeriodChanges struct** (lines 97-102):

Replace:
```go
type PeriodChanges struct {
	EquityValue    MetricChange `json:"equity_value"`    // Holdings market value
	PortfolioValue MetricChange `json:"portfolio_value"` // Total portfolio (equity + cash)
	GrossCash      MetricChange `json:"gross_cash"`      // Cash balance
	Dividend       MetricChange `json:"dividend"`        // Cumulative dividends received
}
```

With:
```go
type PeriodChanges struct {
	PortfolioValue     MetricChange `json:"portfolio_value"`      // Total portfolio (equity + cash)
	NetEquityReturn    MetricChange `json:"net_equity_return"`    // Unrealized P&L (equity_value - net_equity_cost)
	NetEquityReturnPct MetricChange `json:"net_equity_return_pct"` // Return % on equity cost
	GrossCash          MetricChange `json:"gross_cash"`           // Cash balance
	Dividend           MetricChange `json:"dividend"`             // Cumulative dividends received
}
```

Changes:
- Removed: `EquityValue` — D/W/M on holdings value is not actionable
- Added: `NetEquityReturn` — D/W/M on Net Return $ (unrealized P&L)
- Added: `NetEquityReturnPct` — D/W/M on Net Return % (percentage return)

### 2. `internal/services/portfolio/service.go`

**A. New helper: `buildSignedMetricChange`** (add after `buildMetricChange`, line 1169)

The existing `buildMetricChange` uses `HasPrevious: previous > 0` which fails for P&L
values that can be negative. Need a variant that takes an explicit `hasPrevious` flag and
uses `math.Abs(previous)` for the PctChange denominator.

```go
// buildSignedMetricChange creates a MetricChange for values that can be negative (e.g. P&L).
// Unlike buildMetricChange, hasPrevious is explicit (not derived from previous > 0)
// and PctChange uses math.Abs(previous) as denominator to handle sign correctly.
func buildSignedMetricChange(current, previous float64, hasPrevious bool) models.MetricChange {
	mc := models.MetricChange{
		Current:     current,
		Previous:    previous,
		HasPrevious: hasPrevious,
		RawChange:   current - previous,
	}
	if hasPrevious && previous != 0 {
		mc.PctChange = ((current - previous) / math.Abs(previous)) * 100
	}
	return mc
}
```

Note: `math` is already imported in service.go (line 9).

**B. Modify `computePeriodChanges`** (lines 1108-1155)

Replace the full function with:

```go
func (s *Service) computePeriodChanges(ctx context.Context, userID string, portfolio *models.Portfolio, tl interfaces.TimelineStore, refDate time.Time) models.PeriodChanges {
	current := models.PeriodChanges{
		PortfolioValue: models.MetricChange{
			Current:     portfolio.PortfolioValue,
			HasPrevious: false,
		},
		NetEquityReturn: models.MetricChange{
			Current:     portfolio.NetEquityReturn,
			HasPrevious: false,
		},
		NetEquityReturnPct: models.MetricChange{
			Current:     portfolio.NetEquityReturnPct,
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
			current.PortfolioValue = buildMetricChange(portfolio.PortfolioValue, snap.PortfolioValue)
			current.NetEquityReturn = buildSignedMetricChange(portfolio.NetEquityReturn, snap.NetEquityReturn, true)
			current.NetEquityReturnPct = buildSignedMetricChange(portfolio.NetEquityReturnPct, snap.NetEquityReturnPct, true)
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
```

Key changes vs current code:
- Removed: `EquityValue` initialization (was lines 1110-1113) and snapshot lookup (was line 1133)
- Added: `NetEquityReturn` and `NetEquityReturnPct` initialization with `HasPrevious: false`
- Added: snapshot lookup using `buildSignedMetricChange` (not `buildMetricChange`) because P&L can be negative
- Data source: `snap.NetEquityReturn` and `snap.NetEquityReturnPct` from `TimelineSnapshot` (already persisted at lines 323-325 of portfolio.go)

---

## Unit Tests

### File: `internal/services/portfolio/service_test.go` or new `period_changes_test.go`

| # | Test Name | Verifies |
|---|-----------|----------|
| 1 | `TestBuildSignedMetricChange_PositiveValues` | Standard case: both current and previous positive |
| 2 | `TestBuildSignedMetricChange_NegativeValues` | Both values negative (portfolio in loss) — PctChange uses Abs denominator |
| 3 | `TestBuildSignedMetricChange_CrossZero` | Previous positive, current negative (went from gain to loss) |
| 4 | `TestBuildSignedMetricChange_ZeroPrevious` | Previous is zero — PctChange should be 0, no division by zero |
| 5 | `TestBuildSignedMetricChange_NoPrevious` | hasPrevious=false — HasPrevious false, RawChange still computed |
| 6 | `TestComputePeriodChanges_HasNetEquityReturn` | Verify PeriodChanges contains NetEquityReturn and NetEquityReturnPct (not EquityValue) |
| 7 | `TestComputePeriodChanges_NoSnapshot` | When no timeline data, all HasPrevious = false |

---

## Verification Checklist

- [ ] `go build ./cmd/vire-server/` succeeds
- [ ] `go vet ./...` clean
- [ ] `go test ./internal/services/portfolio/...` passes
- [ ] PeriodChanges no longer contains `equity_value` in JSON output
- [ ] PeriodChanges contains `net_equity_return` and `net_equity_return_pct` in JSON output
- [ ] D/W/M RawChange shows dollar P&L difference (e.g. lost $1,357 more than yesterday)
- [ ] Negative P&L values handled correctly (HasPrevious = true, PctChange uses Abs denominator)
- [ ] MCP tool `portfolio_get` response includes new fields in Changes section
