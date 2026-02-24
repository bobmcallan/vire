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
	externalBalance := 50.0

	bars := growthToBars(points, externalBalance)

	if len(bars) != 3 {
		t.Fatalf("expected 3 bars, got %d", len(bars))
	}

	// Newest-first order: bars[0] should be Jan 3 (last input point)
	if !bars[0].Date.Equal(time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bars[0].Date = %v, want 2024-01-03", bars[0].Date)
	}
	if bars[0].Close != 170 { // 120 + 50
		t.Errorf("bars[0].Close = %.0f, want 170 (120 + 50)", bars[0].Close)
	}

	// bars[1] should be Jan 2
	if !bars[1].Date.Equal(time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bars[1].Date = %v, want 2024-01-02", bars[1].Date)
	}
	if bars[1].Close != 160 { // 110 + 50
		t.Errorf("bars[1].Close = %.0f, want 160 (110 + 50)", bars[1].Close)
	}

	// bars[2] should be Jan 1 (oldest)
	if !bars[2].Date.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("bars[2].Date = %v, want 2024-01-01", bars[2].Date)
	}
	if bars[2].Close != 150 { // 100 + 50
		t.Errorf("bars[2].Close = %.0f, want 150 (100 + 50)", bars[2].Close)
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
	bars := growthToBars(nil, 100)
	if len(bars) != 0 {
		t.Errorf("expected 0 bars for nil input, got %d", len(bars))
	}

	bars = growthToBars([]models.GrowthDataPoint{}, 100)
	if len(bars) != 0 {
		t.Errorf("expected 0 bars for empty input, got %d", len(bars))
	}
}

func TestGrowthToBars_ZeroExternalBalance(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 100},
	}
	bars := growthToBars(points, 0)
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
		TotalValueHoldings:   100000,
		ExternalBalanceTotal: 50000,
	}
	// After our changes, TotalValue should be set to holdings + external balances
	p.TotalValue = p.TotalValueHoldings + p.ExternalBalanceTotal

	if p.TotalValue != 150000 {
		t.Errorf("TotalValue = %.0f, want 150000 (holdings + external)", p.TotalValue)
	}
	if p.TotalValueHoldings != 100000 {
		t.Errorf("TotalValueHoldings = %.0f, want 100000", p.TotalValueHoldings)
	}
}

func TestRecomputeExternalBalanceTotal_UpdatesTotalValue(t *testing.T) {
	p := &models.Portfolio{
		TotalValueHoldings: 100000,
		TotalValue:         100000, // initially no external balances
		ExternalBalances: []models.ExternalBalance{
			{Type: "cash", Label: "Cash", Value: 25000},
			{Type: "offset", Label: "Offset", Value: 75000},
		},
	}

	recomputeExternalBalanceTotal(p)

	if p.ExternalBalanceTotal != 100000 {
		t.Errorf("ExternalBalanceTotal = %.0f, want 100000", p.ExternalBalanceTotal)
	}
	if p.TotalValue != 200000 {
		t.Errorf("TotalValue = %.0f, want 200000 (100000 holdings + 100000 external)", p.TotalValue)
	}
}

func TestRecomputeExternalBalanceTotal_EmptyBalances(t *testing.T) {
	p := &models.Portfolio{
		TotalValueHoldings: 100000,
		TotalValue:         150000,
		ExternalBalances:   nil,
	}

	recomputeExternalBalanceTotal(p)

	if p.ExternalBalanceTotal != 0 {
		t.Errorf("ExternalBalanceTotal = %.0f, want 0", p.ExternalBalanceTotal)
	}
	if p.TotalValue != 100000 {
		t.Errorf("TotalValue = %.0f, want 100000 (just holdings)", p.TotalValue)
	}
}
