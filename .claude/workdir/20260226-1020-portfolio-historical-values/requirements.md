# Requirements: Portfolio Historical Values

**Date:** 2026-02-26
**Requested:** Add 4 fields to get_portfolio for total and each open stock: yesterday close, last week close, and percentage changes vs today.

## Scope

**In scope:**
- Add historical close values to each Holding: `yesterday_close`, `last_week_close`
- Add percentage changes: `yesterday_pct`, `last_week_pct`
- Add aggregate values at Portfolio level: `yesterday_total`, `last_week_total`, `yesterday_total_pct`, `last_week_total_pct`
- Handle AU and US exchanges with different trading calendars
- Apply FX conversion for USD holdings

**Out of scope:**
- Holiday calendar handling (approximate by skipping weekends)
- Chart visualization
- Persistence of calculated values

## Approach

### 1. Model Changes

Add fields to `Holding` struct in `internal/models/portfolio.go`:
```go
// Historical values
YesterdayClose float64 `json:"yesterday_close,omitempty"` // Previous trading day close (AUD)
YesterdayPct   float64 `json:"yesterday_pct,omitempty"`   // % change from yesterday to today
LastWeekClose  float64 `json:"last_week_close,omitempty"` // Last Friday close (AUD)
LastWeekPct    float64 `json:"last_week_pct,omitempty"`   // % change from last week to today
```

Add fields to `Portfolio` struct:
```go
// Aggregate historical values
YesterdayTotal     float64 `json:"yesterday_total,omitempty"`     // Total value at yesterday's close
YesterdayTotalPct  float64 `json:"yesterday_total_pct,omitempty"` // % change from yesterday
LastWeekTotal      float64 `json:"last_week_total,omitempty"`     // Total value at last week's close
LastWeekTotalPct   float64 `json:"last_week_total_pct,omitempty"` // % change from last week
```

### 2. Service Changes

In `internal/services/portfolio/service.go`, add helper functions:
- `findEODBarByOffset(eod []models.EODBar, offset int) *models.EODBar` — find bar N trading days back
- `getYesterdayClose(eod []models.EODBar) (float64, bool)` — get previous trading day close
- `getLastWeekClose(eod []models.EODBar) (float64, bool)` — get last Friday close

Modify `GetPortfolio` to:
1. Batch fetch market data for all holdings
2. For each holding, calculate historical values with FX conversion
3. Sum holdings for portfolio-level aggregates

### 3. Trading Calendar Handling

- Use EOD bar dates rather than calendar dates
- Find "yesterday" by looking at EOD[1] (second most recent bar)
- Find "last week" by looking for a bar ~5-7 trading days back

### 4. FX Conversion

For USD holdings, apply the same FX rate used for current price:
- `yesterday_close_aud = yesterday_close_usd / fx_rate`
- This ensures historical values are in the same currency as current values

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/portfolio.go` | Add historical fields to Holding and Portfolio |
| `internal/services/portfolio/service.go` | Add helper functions, modify GetPortfolio |
| `internal/services/portfolio/service_test.go` | Add unit tests for historical calculations |
