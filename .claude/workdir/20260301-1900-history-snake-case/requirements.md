# Requirements: History Endpoint — snake_case JSON + Format Downsampling

**Feedback**: fb_2d9bad2f
**Status**: in progress

## Problem

The `/api/portfolios/{name}/history` endpoint (MCP tool: `get_capital_timeline`) returns `GrowthDataPoint` structs directly in JSON. `GrowthDataPoint` has **no JSON tags**, so all fields serialize as PascalCase (`NetDeployed`, `TotalValue`, etc.). The portal expects snake_case (`net_deployed`, `total_value`), so it can't find the capital deployment data and draws a flat line.

Additionally, the `format` query parameter (daily/weekly/monthly/auto) is parsed but **never applied** — downsampling functions exist but are never called.

The same PascalCase issue affects `handlePortfolioReview` which also returns growth points as `"growth"`.

## Root Cause

- `GrowthDataPoint` (models/portfolio.go:208-219) has no JSON tags
- `TimeSeriesPoint` (models/portfolio.go:222-233) has correct JSON tags with snake_case
- `handlePortfolioHistory` (handlers.go:501-504) returns GrowthDataPoint directly
- `handlePortfolioReview` (handlers.go:294) returns GrowthDataPoint directly as `"growth"`
- `growthPointsToTimeSeries` (indicators.go:13-32) already converts correctly — but is unexported and only used by the indicators endpoint

## Solution

Convert GrowthDataPoint → TimeSeriesPoint in both handlers before returning JSON. Apply downsampling based on `format` parameter.

## Scope

**In scope:**
1. Export the conversion function `GrowthPointsToTimeSeries`
2. Apply `format` downsampling in the history handler
3. Convert to TimeSeriesPoint in both history and review handlers
4. Unit tests for the conversion + downsampling flow
5. Integration test for history endpoint field names

**Out of scope:**
- Portal chart changes (separate task, tracked in fb_f9083a7f)
- Changing GrowthDataPoint struct itself (internal type, no JSON tags needed)

## Files to Change

### 1. `internal/services/portfolio/indicators.go` — Export conversion function

**Change**: Rename `growthPointsToTimeSeries` → `GrowthPointsToTimeSeries` (line 13)

```go
// Before (line 13):
func growthPointsToTimeSeries(points []models.GrowthDataPoint) []models.TimeSeriesPoint {

// After:
func GrowthPointsToTimeSeries(points []models.GrowthDataPoint) []models.TimeSeriesPoint {
```

Update the internal call site at line 111 in the same file:
```go
// Before (line 111, in GetPortfolioIndicators):
timeSeries := growthPointsToTimeSeries(dailyPoints)

// After:
timeSeries := GrowthPointsToTimeSeries(dailyPoints)
```

### 2. `internal/server/handlers.go` — History handler: apply format + convert

**Replace lines 495-506** of `handlePortfolioHistory`:

```go
	points, err := s.app.PortfolioService.GetDailyGrowth(ctx, name, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("History error: %v", err))
		return
	}

	// Apply downsampling based on format parameter
	switch format {
	case "weekly":
		points = portfolio.DownsampleToWeekly(points)
	case "monthly":
		points = portfolio.DownsampleToMonthly(points)
	case "auto":
		if len(points) > 365 {
			points = portfolio.DownsampleToWeekly(points)
		}
		if len(points) > 200 {
			points = portfolio.DownsampleToMonthly(points)
		}
	// "daily" or default: no downsampling
	}

	// Convert to TimeSeriesPoint for consistent snake_case JSON
	timeSeries := portfolio.GrowthPointsToTimeSeries(points)

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"portfolio":   name,
		"format":      format,
		"data_points": timeSeries,
		"count":       len(timeSeries),
	})
```

**Add import** for the portfolio package at the top of handlers.go:
```go
"github.com/bobmcallan/vire/internal/services/portfolio"
```

**Note on auto logic**: Check len(points) AFTER weekly downsampling — if weekly still produces >200 points, downsample further to monthly. This handles multi-year portfolios gracefully.

### 3. `internal/server/handlers.go` — Review handler: convert growth points

**Replace line 294** in `handlePortfolioReview`:

```go
// Before:
"growth": dailyPoints,

// After:
"growth": portfolio.GrowthPointsToTimeSeries(dailyPoints),
```

### 4. Unit tests — `internal/services/portfolio/capital_timeline_test.go`

Add a test verifying `GrowthPointsToTimeSeries` produces correct JSON field names:

```go
func TestGrowthPointsToTimeSeries_JSONFieldNames(t *testing.T) {
	points := []models.GrowthDataPoint{
		{
			Date:         time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			TotalValue:   100000,
			TotalCost:    95000,
			NetReturn:    5000,
			NetReturnPct: 5.26,
			HoldingCount: 5,
			CashBalance:  50000,
			TotalCapital: 150000,
			NetDeployed:  120000,
		},
	}

	ts := GrowthPointsToTimeSeries(points)
	require.Len(t, ts, 1)

	// Marshal to JSON and verify snake_case field names
	data, err := json.Marshal(ts[0])
	require.NoError(t, err)

	jsonStr := string(data)
	assert.Contains(t, jsonStr, `"date"`)
	assert.Contains(t, jsonStr, `"value"`)
	assert.Contains(t, jsonStr, `"cost"`)
	assert.Contains(t, jsonStr, `"net_return"`)
	assert.Contains(t, jsonStr, `"net_return_pct"`)
	assert.Contains(t, jsonStr, `"holding_count"`)
	assert.Contains(t, jsonStr, `"cash_balance"`)
	assert.Contains(t, jsonStr, `"total_capital"`)
	assert.Contains(t, jsonStr, `"net_deployed"`)

	// Verify NO PascalCase field names
	assert.NotContains(t, jsonStr, `"TotalValue"`)
	assert.NotContains(t, jsonStr, `"NetDeployed"`)
	assert.NotContains(t, jsonStr, `"CashBalance"`)
}
```

### 5. Integration test — `tests/api/` (test-creator will write this)

Test that the `/history` endpoint returns snake_case fields including `net_deployed`:
- Sync portfolio, add cash transactions, GET `/history`
- Verify response has `data_points` array with snake_case keys
- Verify `net_deployed` field is present and has correct cumulative value
- Test `format=weekly` and `format=monthly` return fewer points than daily

## Test Cases

| Test | What it verifies |
|------|-----------------|
| `TestGrowthPointsToTimeSeries_JSONFieldNames` | JSON output uses snake_case, no PascalCase |
| `TestHistoryEndpoint_SnakeCaseFields` (integration) | API returns snake_case in data_points |
| `TestHistoryEndpoint_NetDeployedPresent` (integration) | net_deployed accumulates correctly |
| `TestHistoryEndpoint_FormatDownsampling` (integration) | weekly/monthly return fewer points |

## Integration Points

- `indicators.go:13` — rename function (only internal call at line 111)
- `handlers.go:495-506` — replace history response construction
- `handlers.go:294` — replace growth field in review response
- `handlers.go` imports — add portfolio package import
