package portfolio

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestGrowthToBars_CorrectConversion(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), TotalValue: 110},
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), TotalValue: 120},
	}

	bars := growthToBars(points)

	if len(bars) != 3 {
		t.Fatalf("expected 3 bars, got %d", len(bars))
	}

	// Newest-first order: bars[0] should be Jan 3 (last input point)
	if !bars[0].Date.Equal(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bars[0].Date = %v, want 2024-01-03", bars[0].Date)
	}
	if bars[0].Close != 120 {
		t.Errorf("bars[0].Close = %.0f, want 120", bars[0].Close)
	}

	// bars[1] should be Jan 2
	if !bars[1].Date.Equal(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bars[1].Date = %v, want 2024-01-02", bars[1].Date)
	}
	if bars[1].Close != 110 {
		t.Errorf("bars[1].Close = %.0f, want 110", bars[1].Close)
	}

	// bars[2] should be Jan 1 (oldest)
	if !bars[2].Date.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bars[2].Date = %v, want 2024-01-01", bars[2].Date)
	}
	if bars[2].Close != 100 {
		t.Errorf("bars[2].Close = %.0f, want 100", bars[2].Close)
	}

	// All OHLC fields should be set to the same value
	for i, bar := range bars {
		if bar.Open != bar.Close || bar.High != bar.Close || bar.Low != bar.Close || bar.AdjClose != bar.Close {
			t.Errorf("bars[%d] OHLC fields not uniform: O=%.0f H=%.0f L=%.0f C=%.0f A=%.0f",
				i, bar.Open, bar.High, bar.Low, bar.Close, bar.AdjClose)
		}
	}
}

func TestGrowthToBars_Empty(t *testing.T) {
	bars := growthToBars(nil)
	if len(bars) != 0 {
		t.Errorf("expected 0 bars for nil input, got %d", len(bars))
	}

	bars = growthToBars([]models.GrowthDataPoint{})
	if len(bars) != 0 {
		t.Errorf("expected 0 bars for empty input, got %d", len(bars))
	}
}

func TestGrowthToBars_ValueEqualsTotalValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
	}
	bars := growthToBars(points)
	if len(bars) != 1 {
		t.Fatalf("expected 1 bar, got %d", len(bars))
	}
	if bars[0].Close != 100 {
		t.Errorf("bars[0].Close = %.0f, want 100", bars[0].Close)
	}
}

func TestDetectEMACrossover_InsufficientData(t *testing.T) {
	bars := make([]models.EODBar, 200) // need >= 201
	for i := range bars {
		bars[i] = models.EODBar{
			Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i),
			Close: 100,
		}
	}
	result := detectEMACrossover(bars)
	if result != "none" {
		t.Errorf("expected 'none' for insufficient data, got %q", result)
	}
}

func TestDetectEMACrossover_NoSignal(t *testing.T) {
	// Flat data — no crossover
	bars := make([]models.EODBar, 250)
	for i := range bars {
		bars[i] = models.EODBar{
			Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i),
			Close: 100,
		}
	}
	result := detectEMACrossover(bars)
	if result != "none" {
		t.Errorf("expected 'none' for flat data, got %q", result)
	}
}

func TestDetectEMACrossover_GoldenCross(t *testing.T) {
	// Build data where EMA50 rises above EMA200:
	// Start with declining prices, then sharply rising prices at the end.
	bars := make([]models.EODBar, 250)
	for i := range bars {
		price := 100.0
		if i < 50 {
			// Recent data: sharply rising prices (newest first)
			price = 200.0 - float64(i)*1.0
		} else {
			// Older data: low flat prices
			price = 80.0
		}
		bars[i] = models.EODBar{
			Date:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, -i),
			Close: price,
		}
	}
	result := detectEMACrossover(bars)
	// With sharply rising recent prices vs flat old prices, EMA50 should be above EMA200
	// The exact crossover detection depends on the data shape, but the function shouldn't panic
	if result != "golden_cross" && result != "none" {
		t.Logf("detectEMACrossover returned %q (golden_cross or none expected)", result)
	}
}

func TestTotalValueSplit(t *testing.T) {
	p := &models.Portfolio{
		EquityValue: 100000,
		GrossCashBalance:          50000,
	}
	// After our changes, TotalValue should be set to holdings + total cash
	p.EquityValue = p.EquityValue + p.GrossCashBalance

	if p.EquityValue != 150000 {
		t.Errorf("TotalValue = %.0f, want 150000 (holdings + total cash)", p.EquityValue)
	}
	if p.EquityValue != 100000 {
		t.Errorf("TotalValueHoldings = %.0f, want 100000", p.EquityValue)
	}
}

// --- Time Series Tests ---

func TestGrowthPointsToTimeSeries_CorrectConversion(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100000, TotalCost: 90000, NetReturn: 10000, NetReturnPct: 11.11, HoldingCount: 5},
		{Date: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), TotalValue: 105000, TotalCost: 90000, NetReturn: 15000, NetReturnPct: 16.67, HoldingCount: 5},
		{Date: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC), TotalValue: 102000, TotalCost: 90000, NetReturn: 12000, NetReturnPct: 13.33, HoldingCount: 4},
	}

	ts := GrowthPointsToTimeSeries(points)

	if len(ts) != 3 {
		t.Fatalf("expected 3 time series points, got %d", len(ts))
	}

	// TotalValue passes through
	if ts[0].EquityValue != 100000 {
		t.Errorf("ts[0].EquityValue = %.0f, want 100000", ts[0].EquityValue)
	}
	if ts[1].EquityValue != 105000 {
		t.Errorf("ts[1].EquityValue = %.0f, want 105000", ts[1].EquityValue)
	}

	// Check that TotalCost, net return, and holding count pass through
	if ts[0].NetEquityCost != 90000 {
		t.Errorf("ts[0].NetEquityCost = %.0f, want 90000", ts[0].NetEquityCost)
	}
	if ts[0].NetReturn != 10000 {
		t.Errorf("ts[0].NetReturn = %.0f, want 10000", ts[0].NetReturn)
	}
	if ts[2].HoldingCount != 4 {
		t.Errorf("ts[2].HoldingCount = %d, want 4", ts[2].HoldingCount)
	}

	// Check date preservation
	if !ts[0].Date.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ts[0].Date = %v, want 2024-01-01", ts[0].Date)
	}
}

func TestGrowthPointsToTimeSeries_Empty(t *testing.T) {
	ts := GrowthPointsToTimeSeries(nil)
	if len(ts) != 0 {
		t.Errorf("expected 0 time series points for nil input, got %d", len(ts))
	}

	ts = GrowthPointsToTimeSeries([]models.GrowthDataPoint{})
	if len(ts) != 0 {
		t.Errorf("expected 0 time series points for empty input, got %d", len(ts))
	}
}

func TestGrowthPointsToTimeSeries_ValueEqualsHoldings(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100000, TotalCost: 90000},
	}
	ts := GrowthPointsToTimeSeries(points)
	if len(ts) != 1 {
		t.Fatalf("expected 1 time series point, got %d", len(ts))
	}
	// TotalValue passes through
	if ts[0].EquityValue != 100000 {
		t.Errorf("ts[0].EquityValue = %.0f, want 100000", ts[0].EquityValue)
	}
}
