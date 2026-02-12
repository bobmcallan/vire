package portfolio

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestReplayTradesAsOf(t *testing.T) {
	tests := []struct {
		name      string
		trades    []*models.NavexaTrade
		cutoff    time.Time
		wantUnits float64
		wantAvg   float64
		wantCost  float64
	}{
		{
			name: "single buy",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-01-10", Units: 100, Price: 10.00, Fees: 10},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 100,
			wantAvg:   10.10, // (100*10 + 10) / 100
			wantCost:  1010,
		},
		{
			name: "date with T00:00:00 suffix on exact cutoff",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-01-10T00:00:00", Units: 100, Price: 10.00, Fees: 0},
			},
			cutoff:    time.Date(2024, 1, 10, 0, 0, 0, 0, time.UTC),
			wantUnits: 100,
			wantAvg:   10.0,
			wantCost:  1000,
		},
		{
			name: "buy then sell",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-01-10", Units: 100, Price: 10.00, Fees: 0},
				{Type: "Sell", Date: "2024-03-01", Units: 50, Price: 12.00, Fees: 0},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 50,
			wantAvg:   10.0,
			wantCost:  500,
		},
		{
			name: "date cutoff excludes later buy",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-01-10", Units: 100, Price: 10.00, Fees: 0},
				{Type: "Buy", Date: "2024-07-15", Units: 50, Price: 15.00, Fees: 0},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 100,
			wantAvg:   10.0,
			wantCost:  1000,
		},
		{
			name: "opening balance",
			trades: []*models.NavexaTrade{
				{Type: "Opening Balance", Date: "2023-06-01", Units: 200, Price: 5.00, Fees: 0},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 200,
			wantAvg:   5.0,
			wantCost:  1000,
		},
		{
			name: "cost base adjustment",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-01-10", Units: 100, Price: 10.00, Fees: 0},
				{Type: "Cost Base Increase", Date: "2024-02-01", Value: 100},
				{Type: "Cost Base Decrease", Date: "2024-03-01", Value: 50},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 100,
			wantAvg:   10.50, // (1000 + 100 - 50) / 100
			wantCost:  1050,
		},
		{
			name: "all sold",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-01-10", Units: 100, Price: 10.00, Fees: 0},
				{Type: "Sell", Date: "2024-02-01", Units: 100, Price: 12.00, Fees: 0},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 0,
			wantAvg:   0,
			wantCost:  0,
		},
		{
			name: "cutoff on exact trade date is inclusive",
			trades: []*models.NavexaTrade{
				{Type: "Buy", Date: "2024-06-01", Units: 100, Price: 10.00, Fees: 0},
			},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 100,
			wantAvg:   10.0,
			wantCost:  1000,
		},
		{
			name:      "no trades",
			trades:    []*models.NavexaTrade{},
			cutoff:    time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
			wantUnits: 0,
			wantAvg:   0,
			wantCost:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			units, avgCost, totalCost := replayTradesAsOf(tt.trades, tt.cutoff)

			if diff := units - tt.wantUnits; diff > 0.01 || diff < -0.01 {
				t.Errorf("units = %.2f, want %.2f", units, tt.wantUnits)
			}
			if diff := avgCost - tt.wantAvg; diff > 0.01 || diff < -0.01 {
				t.Errorf("avgCost = %.2f, want %.2f", avgCost, tt.wantAvg)
			}
			if diff := totalCost - tt.wantCost; diff > 0.01 || diff < -0.01 {
				t.Errorf("totalCost = %.2f, want %.2f", totalCost, tt.wantCost)
			}
		})
	}
}

func TestFindClosingPriceAsOf(t *testing.T) {
	bars := []models.EODBar{
		{Date: time.Date(2024, 6, 7, 0, 0, 0, 0, time.UTC), Close: 15.00},  // Friday
		{Date: time.Date(2024, 6, 6, 0, 0, 0, 0, time.UTC), Close: 14.50},  // Thursday
		{Date: time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC), Close: 14.00},  // Wednesday
		{Date: time.Date(2024, 6, 4, 0, 0, 0, 0, time.UTC), Close: 13.50},  // Tuesday
		{Date: time.Date(2024, 6, 3, 0, 0, 0, 0, time.UTC), Close: 13.00},  // Monday
		{Date: time.Date(2024, 5, 31, 0, 0, 0, 0, time.UTC), Close: 12.50}, // Prev Friday
	}

	tests := []struct {
		name      string
		bars      []models.EODBar
		asOf      time.Time
		wantPrice float64
		wantDate  time.Time
		wantFound bool
	}{
		{
			name:      "exact match",
			bars:      bars,
			asOf:      time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC),
			wantPrice: 14.00,
			wantDate:  time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC),
			wantFound: true,
		},
		{
			name:      "weekend falls back to Friday",
			bars:      bars,
			asOf:      time.Date(2024, 6, 8, 0, 0, 0, 0, time.UTC), // Saturday
			wantPrice: 15.00,
			wantDate:  time.Date(2024, 6, 7, 0, 0, 0, 0, time.UTC),
			wantFound: true,
		},
		{
			name:      "Sunday also falls back to Friday",
			bars:      bars,
			asOf:      time.Date(2024, 6, 9, 0, 0, 0, 0, time.UTC), // Sunday
			wantPrice: 15.00,
			wantDate:  time.Date(2024, 6, 7, 0, 0, 0, 0, time.UTC),
			wantFound: true,
		},
		{
			name:      "empty bars",
			bars:      []models.EODBar{},
			asOf:      time.Date(2024, 6, 5, 0, 0, 0, 0, time.UTC),
			wantPrice: 0,
			wantFound: false,
		},
		{
			name:      "date before all bars",
			bars:      bars,
			asOf:      time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
			wantPrice: 0,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			price, barDate, found := findClosingPriceAsOf(tt.bars, tt.asOf)

			if found != tt.wantFound {
				t.Errorf("found = %v, want %v", found, tt.wantFound)
			}
			if diff := price - tt.wantPrice; diff > 0.01 || diff < -0.01 {
				t.Errorf("price = %.2f, want %.2f", price, tt.wantPrice)
			}
			if tt.wantFound && !barDate.Equal(tt.wantDate) {
				t.Errorf("barDate = %v, want %v", barDate, tt.wantDate)
			}
		})
	}
}
