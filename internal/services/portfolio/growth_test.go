package portfolio

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestGenerateCalendarDates(t *testing.T) {
	start := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)

	dates := generateCalendarDates(start, end)

	if len(dates) != 10 {
		t.Fatalf("got %d dates, want 10", len(dates))
	}

	if !dates[0].Equal(start) {
		t.Errorf("first date = %v, want %v", dates[0], start)
	}
	if !dates[len(dates)-1].Equal(end) {
		t.Errorf("last date = %v, want %v", dates[len(dates)-1], end)
	}
}

func TestGenerateCalendarDatesSingleDay(t *testing.T) {
	day := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	dates := generateCalendarDates(day, day)

	if len(dates) != 1 {
		t.Fatalf("got %d dates, want 1", len(dates))
	}
	if !dates[0].Equal(day) {
		t.Errorf("date = %v, want %v", dates[0], day)
	}
}

func TestGenerateCalendarDatesEndBeforeStart(t *testing.T) {
	start := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	dates := generateCalendarDates(start, end)

	if dates != nil {
		t.Fatalf("expected nil for end before start, got %d dates", len(dates))
	}
}

func TestDownsampleToMonthly(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), TotalValue: 100},
		{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), TotalValue: 110},
		{Date: time.Date(2024, 2, 10, 0, 0, 0, 0, time.UTC), TotalValue: 115},
		{Date: time.Date(2024, 2, 28, 0, 0, 0, 0, time.UTC), TotalValue: 120},
		{Date: time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC), TotalValue: 125},
	}

	monthly := DownsampleToMonthly(points)

	if len(monthly) != 3 {
		t.Fatalf("got %d monthly points, want 3", len(monthly))
	}

	// Jan should pick the 31st (last point in January)
	if monthly[0].EquityValue != 110 {
		t.Errorf("Jan value = %.0f, want 110", monthly[0].EquityValue)
	}
	if !monthly[0].Date.Equal(time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Jan date = %v, want 2024-01-31", monthly[0].Date)
	}

	// Feb should pick the 28th
	if monthly[1].EquityValue != 120 {
		t.Errorf("Feb value = %.0f, want 120", monthly[1].EquityValue)
	}

	// Mar has only one point, so it's picked
	if monthly[2].EquityValue != 125 {
		t.Errorf("Mar value = %.0f, want 125", monthly[2].EquityValue)
	}
}

func TestDownsampleToMonthlyEmpty(t *testing.T) {
	result := DownsampleToMonthly(nil)
	if result != nil {
		t.Fatalf("expected nil for nil input, got %d points", len(result))
	}

	result = DownsampleToMonthly([]models.GrowthDataPoint{})
	if result != nil {
		t.Fatalf("expected nil for empty input, got %d points", len(result))
	}
}

func TestFindEarliestTradeDate(t *testing.T) {
	holdings := []models.Holding{
		{
			Ticker: "BHP",
			Trades: []*models.NavexaTrade{
				{Date: "2024-06-15", Type: "Buy", Units: 100, Price: 10},
				{Date: "2024-01-10", Type: "Buy", Units: 50, Price: 8},
			},
		},
		{
			Ticker: "CBA",
			Trades: []*models.NavexaTrade{
				{Date: "2023-11-05", Type: "Buy", Units: 200, Price: 5},
			},
		},
	}

	earliest := findEarliestTradeDate(holdings)
	want := time.Date(2023, 11, 5, 0, 0, 0, 0, time.UTC)
	if !earliest.Equal(want) {
		t.Errorf("findEarliestTradeDate = %v, want %v", earliest, want)
	}
}

func TestFindEarliestTradeDateWithTimestamp(t *testing.T) {
	// Navexa returns dates like "2025-12-24T00:00:00"
	holdings := []models.Holding{
		{
			Ticker: "ACDC",
			Trades: []*models.NavexaTrade{
				{Date: "2025-12-24T00:00:00", Type: "Buy", Units: 100, Price: 140},
			},
		},
		{
			Ticker: "BHP",
			Trades: []*models.NavexaTrade{
				{Date: "2025-06-15T00:00:00", Type: "Buy", Units: 50, Price: 42},
			},
		},
	}

	earliest := findEarliestTradeDate(holdings)
	want := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	if !earliest.Equal(want) {
		t.Errorf("findEarliestTradeDate = %v, want %v", earliest, want)
	}
}

func TestFindEarliestTradeDateNoTrades(t *testing.T) {
	holdings := []models.Holding{
		{Ticker: "BHP", Trades: nil},
	}

	earliest := findEarliestTradeDate(holdings)
	if !earliest.IsZero() {
		t.Errorf("expected zero time for no trades, got %v", earliest)
	}
}

func TestRenderGrowthChartValidPNG(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), TotalValue: 100000, TotalCost: 90000, NetReturn: 10000, NetReturnPct: 11.1, HoldingCount: 5},
		{Date: time.Date(2024, 2, 29, 0, 0, 0, 0, time.UTC), TotalValue: 105000, TotalCost: 92000, NetReturn: 13000, NetReturnPct: 14.1, HoldingCount: 5},
		{Date: time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC), TotalValue: 110000, TotalCost: 95000, NetReturn: 15000, NetReturnPct: 15.8, HoldingCount: 6},
	}

	pngBytes, err := RenderGrowthChart(points)
	if err != nil {
		t.Fatalf("RenderGrowthChart error: %v", err)
	}

	// PNG files start with the 8-byte PNG signature
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(pngBytes) < 8 {
		t.Fatalf("PNG output too short: %d bytes", len(pngBytes))
	}
	for i, b := range pngHeader {
		if pngBytes[i] != b {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X (not a valid PNG)", i, pngBytes[i], b)
		}
	}

	// Reasonable size check
	if len(pngBytes) < 1000 {
		t.Errorf("PNG suspiciously small: %d bytes", len(pngBytes))
	}
}

func TestRenderGrowthChartTooFewPoints(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC), TotalValue: 100000, TotalCost: 90000},
	}

	_, err := RenderGrowthChart(points)
	if err == nil {
		t.Fatal("expected error for single data point, got nil")
	}
}
