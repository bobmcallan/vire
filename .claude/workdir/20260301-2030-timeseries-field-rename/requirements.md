# Requirements: TimeSeriesPoint Field Rename for Portfolio Consistency

## Problem

TimeSeriesPoint JSON field names are inconsistent with the portfolio response (`get_portfolio`):

| Concept | Portfolio field | TimeSeriesPoint field (current) | TimeSeriesPoint field (new) |
|---------|----------------|--------------------------------|----------------------------|
| Holdings value | `total_value` | `value` | `total_value` |
| Cost basis | `total_cost` | `cost` | `total_cost` |
| Total cash | `total_cash` | `cash_balance` | `total_cash` |
| Available cash | `available_cash` | (missing) | `available_cash` (NEW) |
| Capital deployed | `net_capital_deployed` | `net_deployed` | `net_capital_deployed` |
| Deprecated | — | `external_balance` | REMOVE |
| Total capital | `total_capital` | `total_capital` | (no change) |
| Net return | `net_return` | `net_return` | (no change) |
| Net return % | `net_return_pct` | `net_return_pct` | (no change) |
| Holding count | `holding_count` | `holding_count` | (no change) |

### Key definitions
- **total_cash** = running sum of all manual cash transactions (deposits + dividends - fees ± transfers). This is the "gross cash" position.
- **available_cash** = total_cash - total_cost = uninvested cash remaining after capital deployed in equities.
- **total_capital** = total_value + total_cash (unchanged formula, just renamed variables).
- **net_capital_deployed** = cumulative deposits - withdrawals (excludes dividends/fees).

## Scope

**In scope:**
1. Rename TimeSeriesPoint struct fields + JSON tags
2. Add `available_cash` field (computed: total_cash - total_cost)
3. Remove deprecated `external_balance` (always 0)
4. Update GrowthDataPoint: remove ExternalBalance field
5. Update GrowthPointsToTimeSeries conversion
6. Update all unit tests
7. Update all integration tests
8. Update MCP catalog descriptions

**Out of scope:**
- Portfolio response fields (already correct)
- GrowthDataPoint field names other than ExternalBalance (internal only, not serialized)

## Files to Change

### 1. `internal/models/portfolio.go` — TimeSeriesPoint struct (lines 221-233)

**Replace the entire struct:**
```go
// TimeSeriesPoint represents a single point in the daily portfolio value time series.
type TimeSeriesPoint struct {
	Date               time.Time `json:"date"`
	TotalValue         float64   `json:"total_value"`
	TotalCost          float64   `json:"total_cost"`
	NetReturn          float64   `json:"net_return"`
	NetReturnPct       float64   `json:"net_return_pct"`
	HoldingCount       int       `json:"holding_count"`
	TotalCash          float64   `json:"total_cash,omitempty"`
	AvailableCash      float64   `json:"available_cash,omitempty"`
	TotalCapital       float64   `json:"total_capital,omitempty"`
	NetCapitalDeployed float64   `json:"net_capital_deployed,omitempty"`
}
```

### 2. `internal/models/portfolio.go` — GrowthDataPoint struct (lines 208-219)

**Remove ExternalBalance field:**
```go
type GrowthDataPoint struct {
	Date         time.Time
	TotalValue   float64
	TotalCost    float64
	NetReturn    float64
	NetReturnPct float64
	HoldingCount int
	CashBalance  float64 // Running cash balance as of this date
	TotalCapital float64 // Value + CashBalance
	NetDeployed  float64 // Cumulative deposits - withdrawals to date
}
```

### 3. `internal/services/portfolio/indicators.go` — GrowthPointsToTimeSeries (lines 13-31)

**Replace the conversion function body:**
```go
func GrowthPointsToTimeSeries(points []models.GrowthDataPoint) []models.TimeSeriesPoint {
	ts := make([]models.TimeSeriesPoint, len(points))
	for i, p := range points {
		totalValue := p.TotalValue
		totalCash := p.CashBalance
		ts[i] = models.TimeSeriesPoint{
			Date:               p.Date,
			TotalValue:         totalValue,
			TotalCost:          p.TotalCost,
			NetReturn:          p.NetReturn,
			NetReturnPct:       p.NetReturnPct,
			HoldingCount:       p.HoldingCount,
			TotalCash:          totalCash,
			AvailableCash:      totalCash - p.TotalCost,
			TotalCapital:       totalValue + totalCash,
			NetCapitalDeployed: p.NetDeployed,
		}
	}
	return ts
}
```

### 4. `internal/services/portfolio/growth.go` — Remove ExternalBalance (line 238)

**Remove line 238** (`ExternalBalance: 0,`) from the GrowthDataPoint construction at line 230.

### 5. Unit test updates — field name substitutions

Apply these substitutions across ALL test files in `internal/services/portfolio/`:

| Old Go field | New Go field |
|---|---|
| `.Value` (on TimeSeriesPoint) | `.TotalValue` |
| `.Cost` (on TimeSeriesPoint) | `.TotalCost` |
| `.CashBalance` (on TimeSeriesPoint) | `.TotalCash` |
| `.ExternalBalance` (on TimeSeriesPoint) | REMOVE all assertions |
| `.NetDeployed` (on TimeSeriesPoint) | `.NetCapitalDeployed` |

For GrowthDataPoint fields:
| Old Go field | Change |
|---|---|
| `.ExternalBalance` | REMOVE all assertions and field assignments |
| `.CashBalance` | Keep (GrowthDataPoint field name unchanged) |
| `.NetDeployed` | Keep (GrowthDataPoint field name unchanged) |

**Files:**
- `capital_timeline_test.go` — update TimeSeriesPoint field refs
- `capital_timeline_stress_test.go` — update TimeSeriesPoint field refs
- `capital_cash_fixes_stress_test.go` — update TimeSeriesPoint and GrowthDataPoint field refs, remove ExternalBalance assertions
- `indicators_test.go` — update TimeSeriesPoint field refs
- `timeseries_stress_test.go` — update TimeSeriesPoint field refs

**CRITICAL**: In `capital_cash_fixes_stress_test.go`:
- Tests 4 and 7 specifically test ExternalBalance=0. REMOVE these test cases entirely (the field no longer exists).
- Update test comments that reference ExternalBalance.
- Update the invariant assertions from `Value + CashBalance` to `TotalValue + TotalCash`.
- Remove `ExternalBalance: 0` from GrowthDataPoint literal constructions.

### 6. Integration test updates — JSON field name substitutions

Apply these substitutions across ALL test files in `tests/api/`:

| Old JSON key | New JSON key |
|---|---|
| `"value"` (when accessing time_series points) | `"total_value"` |
| `"cost"` (when accessing time_series points) | `"total_cost"` |
| `"cash_balance"` | `"total_cash"` |
| `"external_balance"` | REMOVE all assertions |
| `"net_deployed"` | `"net_capital_deployed"` |

**Files:**
- `tests/api/history_endpoint_test.go` — lines 93, 95, 184, 405
- `tests/api/capital_timeline_test.go` — lines 88, 90, 159, 193, 198, 203, 261, 295, 302, 306, 307, 373, 375
- `tests/api/external_balance_fixes_test.go` — lines 235, 435, 487, 532, 637, 652, 705, 831
- `tests/api/capital_cash_fixes_test.go` — lines 210, 220, 222, 304, 315, 390, 404, 480, 487, 522, 529
- `tests/api/portfolio_indicators_test.go` — line 303

**IMPORTANT**: Be careful with `"value"` — only change it when it's accessing a time_series/data_points field. Do NOT change `"value"` when it's part of cash transaction payloads (e.g. `{"value": 60000}` for a transaction amount).

### 7. `internal/server/catalog.go` — MCP tool descriptions

Update the following tool descriptions:

**get_capital_timeline** (around line 626): Update field list in description to reference `total_value`, `total_cost`, `total_cash`, `available_cash`, `net_capital_deployed` instead of old names. Also update the `data_points` array field descriptions.

**get_portfolio_indicators** (search for the tool): Update `time_series` field descriptions to use new names.

### 8. Update `TestGrowthPointsToTimeSeries_JSONFieldNames` (from previous session)

Update the JSON field name assertions:
```go
assert.Contains(t, jsonStr, `"total_value"`)    // was `"value"`
assert.Contains(t, jsonStr, `"total_cost"`)      // was `"cost"`
assert.Contains(t, jsonStr, `"total_cash"`)      // was `"cash_balance"`
assert.Contains(t, jsonStr, `"available_cash"`)   // NEW
assert.Contains(t, jsonStr, `"net_capital_deployed"`) // was `"net_deployed"`

// Verify NO old field names
assert.NotContains(t, jsonStr, `"cash_balance"`)
assert.NotContains(t, jsonStr, `"external_balance"`)
assert.NotContains(t, jsonStr, `"net_deployed"`)
```

## Test Cases

| Test | What it verifies |
|------|-----------------|
| `TestGrowthPointsToTimeSeries_JSONFieldNames` (updated) | JSON uses new field names, no old names |
| All existing time series tests (updated) | Field renames don't break calculations |
| All existing integration tests (updated) | API returns new field names |

## Verification

After all changes:
```bash
go build ./cmd/vire-server/
go vet ./...
go test ./internal/services/portfolio/... -count=1
go test ./tests/api/... -run "Timeline|History|Indicator|Balance|CashFix" -v -timeout 300s
```
