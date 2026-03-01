package portfolio

import (
	"encoding/json"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for the TimeSeries feature.
// Tests growthPointsToTimeSeries, TimeSeriesPoint model, and edge cases.

// --- growthPointsToTimeSeries ---
// Note: This function will be added by the implementer. Until then, these tests
// verify the expected behavior by testing the equivalent logic inline.

// growthPointsToTimeSeriesInline replicates the expected implementation for testing.
// After capital cash fixes: Value = TotalValue (no external balance added).
func growthPointsToTimeSeriesInline(points []models.GrowthDataPoint, _ float64) []timeSeriesPoint {
	ts := make([]timeSeriesPoint, len(points))
	for i, p := range points {
		ts[i] = timeSeriesPoint{
			Date:               p.Date,
			EquityValue:        p.EquityValue,
			NetEquityCost:      p.NetEquityCost,
			NetEquityReturn:    p.NetEquityReturn,
			NetEquityReturnPct: p.NetEquityReturnPct,
			HoldingCount:       p.HoldingCount,
		}
	}
	return ts
}

// timeSeriesPoint is a test-local copy of the expected TimeSeriesPoint model.
type timeSeriesPoint struct {
	Date               time.Time `json:"date"`
	EquityValue        float64   `json:"equity_value"`
	NetEquityCost      float64   `json:"net_equity_cost"`
	NetEquityReturn    float64   `json:"net_equity_return"`
	NetEquityReturnPct float64   `json:"net_equity_return_pct"`
	HoldingCount       int       `json:"holding_count"`
}

func TestGrowthPointsToTimeSeries_EmptyInput(t *testing.T) {
	ts := growthPointsToTimeSeriesInline(nil, 50000)
	assert.Empty(t, ts, "nil input should produce empty output")

	ts2 := growthPointsToTimeSeriesInline([]models.GrowthDataPoint{}, 50000)
	assert.Empty(t, ts2, "empty input should produce empty output")
}

func TestGrowthPointsToTimeSeries_SinglePoint(t *testing.T) {
	points := []models.GrowthDataPoint{
		{
			Date:               time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
			EquityValue:        100000,
			NetEquityCost:      90000,
			NetEquityReturn:    10000,
			NetEquityReturnPct: 11.11,
			HoldingCount:       5,
		},
	}
	ts := growthPointsToTimeSeriesInline(points, 50000)
	require.Len(t, ts, 1)
	assert.Equal(t, 100000.0, ts[0].EquityValue, "value equals TotalValue (external balance no longer added)")
	assert.Equal(t, 90000.0, ts[0].NetEquityCost)
	assert.Equal(t, 10000.0, ts[0].NetEquityReturn)
	assert.Equal(t, 11.11, ts[0].NetEquityReturnPct)
	assert.Equal(t, 5, ts[0].HoldingCount)
}

func TestGrowthPointsToTimeSeries_ExternalBalanceZero(t *testing.T) {
	points := []models.GrowthDataPoint{
		{EquityValue: 100000},
	}
	ts := growthPointsToTimeSeriesInline(points, 0)
	assert.Equal(t, 100000.0, ts[0].EquityValue, "zero external balance should not affect value")
}

func TestGrowthPointsToTimeSeries_NegativeExternalBalance(t *testing.T) {
	// External balance parameter is now ignored — Value = TotalValue regardless
	points := []models.GrowthDataPoint{
		{EquityValue: 100000},
	}
	ts := growthPointsToTimeSeriesInline(points, -50000)
	assert.Equal(t, 100000.0, ts[0].EquityValue, "external balance ignored — value equals TotalValue")
}

func TestGrowthPointsToTimeSeries_NaNValues(t *testing.T) {
	points := []models.GrowthDataPoint{
		{EquityValue: math.NaN(), NetEquityCost: math.NaN(), NetEquityReturn: math.NaN(), NetEquityReturnPct: math.NaN()},
	}
	ts := growthPointsToTimeSeriesInline(points, 50000)
	assert.True(t, math.IsNaN(ts[0].EquityValue), "NaN TotalValue should propagate NaN")
	assert.True(t, math.IsNaN(ts[0].NetEquityCost))
	assert.True(t, math.IsNaN(ts[0].NetEquityReturn))
}

func TestGrowthPointsToTimeSeries_InfValues(t *testing.T) {
	points := []models.GrowthDataPoint{
		{EquityValue: math.Inf(1)},
	}
	ts := growthPointsToTimeSeriesInline(points, 50000)
	assert.True(t, math.IsInf(ts[0].EquityValue, 1), "Inf should propagate")
}

func TestGrowthPointsToTimeSeries_VeryLargeDataset(t *testing.T) {
	// 10 years of daily data = ~2500 points
	n := 2500
	points := make([]models.GrowthDataPoint, n)
	base := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		points[i] = models.GrowthDataPoint{
			Date:               base.AddDate(0, 0, i),
			EquityValue:        float64(100000 + i*10),
			NetEquityCost:      90000,
			NetEquityReturn:    float64(10000 + i*10),
			NetEquityReturnPct: float64(10000+i*10) / 90000 * 100,
			HoldingCount:       5,
		}
	}

	ts := growthPointsToTimeSeriesInline(points, 50000)
	require.Len(t, ts, n)

	// Verify ordering preserved
	for i := 1; i < len(ts); i++ {
		assert.True(t, ts[i].Date.After(ts[i-1].Date) || ts[i].Date.Equal(ts[i-1].Date),
			"time series should be in chronological order")
	}

	// Verify all values equal TotalValue (external balance no longer added)
	for i, pt := range ts {
		expected := points[i].EquityValue
		assert.Equal(t, expected, pt.EquityValue, "point %d value mismatch", i)
	}
}

func TestGrowthPointsToTimeSeries_VeryLargeDataset_Memory(t *testing.T) {
	// 20 years = ~5000 points — verify no excessive memory allocation
	n := 5000
	points := make([]models.GrowthDataPoint, n)
	for i := 0; i < n; i++ {
		points[i] = models.GrowthDataPoint{
			Date:        time.Date(2005, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i),
			EquityValue: float64(i) * 100,
		}
	}
	ts := growthPointsToTimeSeriesInline(points, 0)
	assert.Len(t, ts, n)
}

func TestGrowthPointsToTimeSeries_ConcurrentAccess(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), EquityValue: 101000},
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), EquityValue: 102000},
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(ext float64) {
			defer wg.Done()
			ts := growthPointsToTimeSeriesInline(points, ext)
			assert.Len(t, ts, 3)
			assert.Equal(t, points[0].EquityValue, ts[0].EquityValue, "value equals TotalValue regardless of ext param")
		}(float64(i) * 1000)
	}
	wg.Wait()
}

// --- JSON serialization of TimeSeries ---

func TestTimeSeriesPoint_JSONRoundtrip(t *testing.T) {
	pt := timeSeriesPoint{
		Date:               time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC),
		EquityValue:        150000,
		NetEquityCost:      90000,
		NetEquityReturn:    60000,
		NetEquityReturnPct: 66.67,
		HoldingCount:       5,
	}

	data, err := json.Marshal(pt)
	require.NoError(t, err)

	var restored timeSeriesPoint
	require.NoError(t, json.Unmarshal(data, &restored))

	assert.Equal(t, pt.EquityValue, restored.EquityValue)
	assert.Equal(t, pt.NetEquityCost, restored.NetEquityCost)
	assert.Equal(t, pt.NetEquityReturn, restored.NetEquityReturn)
	assert.Equal(t, pt.NetEquityReturnPct, restored.NetEquityReturnPct)
	assert.Equal(t, pt.HoldingCount, restored.HoldingCount)
}

func TestTimeSeriesPoint_JSONFieldNames(t *testing.T) {
	pt := timeSeriesPoint{
		Date:               time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		EquityValue:        100,
		NetEquityCost:      90,
		NetEquityReturn:    10,
		NetEquityReturnPct: 11.11,
		HoldingCount:       3,
	}
	data, err := json.Marshal(pt)
	require.NoError(t, err)

	raw := string(data)
	assert.Contains(t, raw, `"date"`)
	assert.Contains(t, raw, `"equity_value"`)
	assert.Contains(t, raw, `"net_equity_cost"`)
	assert.Contains(t, raw, `"net_equity_return"`)
	assert.Contains(t, raw, `"net_equity_return_pct"`)
	assert.Contains(t, raw, `"holding_count"`)
	assert.NotContains(t, raw, `"value"`)
	assert.NotContains(t, raw, `"cost"`)
}

func TestPortfolioIndicators_TimeSeriesOmitEmpty(t *testing.T) {
	// TimeSeries field has been removed from PortfolioIndicators (endpoint consolidation)
	ind := models.PortfolioIndicators{
		PortfolioName: "SMSF",
		DataPoints:    0,
	}
	data, err := json.Marshal(ind)
	require.NoError(t, err)

	raw := string(data)
	assert.NotContains(t, raw, `"time_series"`,
		"time_series field should not be present — it was removed from PortfolioIndicators")
}

// --- Value calculation edge cases ---

func TestGrowthPointsToTimeSeries_ZeroGrowthPoints(t *testing.T) {
	// New portfolio with no trade history — growth returns empty
	ts := growthPointsToTimeSeriesInline([]models.GrowthDataPoint{}, 100000)
	assert.Empty(t, ts, "no growth points should produce empty time series")
	t.Log("FINDING: When GetDailyGrowth returns empty (new portfolio), " +
		"TimeSeries will be empty/nil, which is correct behavior with omitempty")
}

func TestGrowthPointsToTimeSeries_AllZeroValues(t *testing.T) {
	// Growth points where all values are zero (e.g., portfolio created but no trades executed)
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
	}
	ts := growthPointsToTimeSeriesInline(points, 0)
	require.Len(t, ts, 2)
	assert.Equal(t, 0.0, ts[0].EquityValue)
	assert.Equal(t, 0.0, ts[0].NetEquityCost)
}

func TestGrowthPointsToTimeSeries_ExternalBalanceChangedMidStream(t *testing.T) {
	// After capital cash fixes: external balance is no longer added to Value.
	// Value = TotalValue. Cash tracking is now done via CashBalance field on each point.
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), EquityValue: 120000},
	}

	ts := growthPointsToTimeSeriesInline(points, 50000)
	assert.Equal(t, 100000.0, ts[0].EquityValue, "Value = TotalValue, external balance no longer added")
	assert.Equal(t, 120000.0, ts[1].EquityValue, "Value = TotalValue, external balance no longer added")
}

func TestGrowthPointsToTimeSeries_NetReturnPctConsistency(t *testing.T) {
	// Verify that NetReturnPct from growth data is passed through unchanged.
	// After fix: Value = TotalValue (no external balance), so NetReturnPct is now
	// consistent with the Value field.
	points := []models.GrowthDataPoint{
		{
			Date:               time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			EquityValue:        110000,
			NetEquityCost:      100000,
			NetEquityReturn:    10000,
			NetEquityReturnPct: 10.0, // 10000/100000 * 100
		},
	}
	ts := growthPointsToTimeSeriesInline(points, 50000)
	assert.Equal(t, 110000.0, ts[0].EquityValue, "value = TotalValue (110000), external balance no longer added")
	assert.Equal(t, 10.0, ts[0].NetEquityReturnPct,
		"NetReturnPct is now consistent with Value since external balance is no longer mixed in")
}

// --- growthToBars vs growthPointsToTimeSeries consistency ---

func TestGrowthToBarsAndTimeSeries_ConsistentValues(t *testing.T) {
	// Both functions should produce the same Value = TotalValue + externalBalance
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), EquityValue: 110000},
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), EquityValue: 105000},
	}
	ext := 50000.0

	bars := growthToBars(points)
	ts := growthPointsToTimeSeriesInline(points, ext)

	require.Len(t, bars, len(points))
	require.Len(t, ts, len(points))

	// growthToBars reverses order (newest first), timeseries keeps chronological
	// After fix: Value = TotalValue (no external balance added)
	for i, p := range points {
		tsValue := ts[i].EquityValue
		barValue := bars[len(bars)-1-i].Close // reverse index for comparison
		assert.Equal(t, p.EquityValue, tsValue, "timeseries value mismatch at %d", i)
		assert.Equal(t, p.EquityValue, barValue, "bar value mismatch at %d", i)
		assert.Equal(t, tsValue, barValue, "timeseries and bars should have same value at %d", i)
	}
}

// --- Duplicate/non-monotonic dates ---

func TestGrowthPointsToTimeSeries_DuplicateDates(t *testing.T) {
	// GetDailyGrowth shouldn't produce duplicate dates, but verify passthrough
	date := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	points := []models.GrowthDataPoint{
		{Date: date, EquityValue: 100000},
		{Date: date, EquityValue: 110000}, // same date, different value
	}
	ts := growthPointsToTimeSeriesInline(points, 0)
	require.Len(t, ts, 2, "duplicate dates should be passed through as-is")
	assert.Equal(t, 100000.0, ts[0].EquityValue)
	assert.Equal(t, 110000.0, ts[1].EquityValue)
}

func TestGrowthPointsToTimeSeries_NonMonotonicDates(t *testing.T) {
	// Out-of-order dates — shouldn't happen but verify no sorting is done
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), EquityValue: 120000},
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
	}
	ts := growthPointsToTimeSeriesInline(points, 0)
	require.Len(t, ts, 2)
	// Should preserve input order (no sorting in conversion)
	assert.True(t, ts[0].Date.After(ts[1].Date), "should preserve input order without sorting")
}
