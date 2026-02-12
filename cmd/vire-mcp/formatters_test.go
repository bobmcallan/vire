package main

import (
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

func TestFormatPortfolioHoldings_IncludesCurrencyAndCountryColumns(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 7700,
		TotalCost:  5500,
		TotalGain:  2200,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP",
				Exchange:     "AU",
				Name:         "BHP Group",
				Units:        100,
				AvgCost:      40.00,
				CurrentPrice: 45.00,
				MarketValue:  4500.00,
				GainLoss:     500.00,
				GainLossPct:  12.5,
				Weight:       58.4,
				Currency:     "AUD",
				Country:      "AU",
			},
			{
				Ticker:       "AAPL",
				Exchange:     "US",
				Name:         "Apple Inc",
				Units:        10,
				AvgCost:      150.00,
				CurrentPrice: 200.00,
				MarketValue:  2000.00,
				GainLoss:     500.00,
				GainLossPct:  33.3,
				Weight:       41.6,
				Currency:     "USD",
				Country:      "US",
			},
		},
	}

	output := formatPortfolioHoldings(portfolio)

	// Should contain currency column header
	if !strings.Contains(output, "Ccy") {
		t.Error("formatPortfolioHoldings output missing 'Ccy' column header")
	}

	// Should contain country column header
	if !strings.Contains(output, "Country") {
		t.Error("formatPortfolioHoldings output missing 'Country' column header")
	}

	// Should show AUD and USD currency markers
	if !strings.Contains(output, "AUD") {
		t.Error("formatPortfolioHoldings output missing AUD currency")
	}
	if !strings.Contains(output, "USD") {
		t.Error("formatPortfolioHoldings output missing USD currency")
	}

	// Should show country codes
	if !strings.Contains(output, "| AU |") || !strings.Contains(output, "| US |") {
		t.Error("formatPortfolioHoldings output missing country codes in table cells")
	}
}

func TestFormatPortfolioHoldings_SortedBySymbol(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 5000,
		Holdings: []models.Holding{
			{Ticker: "CBA", Exchange: "AU", Name: "CBA", Units: 10, CurrentPrice: 100, MarketValue: 1000, Currency: "AUD", Country: "AU"},
			{Ticker: "AAPL", Exchange: "US", Name: "Apple", Units: 10, CurrentPrice: 200, MarketValue: 2000, Currency: "USD", Country: "US"},
			{Ticker: "BHP", Exchange: "AU", Name: "BHP", Units: 10, CurrentPrice: 200, MarketValue: 2000, Currency: "AUD", Country: "AU"},
		},
	}

	output := formatPortfolioHoldings(portfolio)

	// AAPL should appear before BHP, BHP before CBA
	aaplIdx := strings.Index(output, "AAPL")
	bhpIdx := strings.Index(output, "BHP")
	cbaIdx := strings.Index(output, "CBA")

	if aaplIdx < 0 || bhpIdx < 0 || cbaIdx < 0 {
		t.Fatal("missing tickers in output")
	}

	if !(aaplIdx < bhpIdx && bhpIdx < cbaIdx) {
		t.Errorf("holdings not sorted by symbol: AAPL@%d, BHP@%d, CBA@%d", aaplIdx, bhpIdx, cbaIdx)
	}
}

func TestFormatPortfolioReview_IncludesCurrencyColumn(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		TotalValue:    7700,
		TotalCost:     5500,
		TotalGain:     2200,
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{
					Ticker:       "BHP",
					Exchange:     "AU",
					Name:         "BHP Group",
					Units:        100,
					CurrentPrice: 45.00,
					MarketValue:  4500.00,
					Weight:       58.4,
					Currency:     "AUD",
					Country:      "AU",
				},
				Fundamentals:   &models.Fundamentals{Sector: "Materials"},
				ActionRequired: "HOLD",
			},
		},
	}

	output := formatPortfolioReview(review)

	if !strings.Contains(output, "Ccy") {
		t.Error("formatPortfolioReview output missing 'Ccy' column header")
	}
}

func TestFormatSyncResult_ShowsFXRate(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 7700,
		Currency:   "AUD",
		FXRate:     0.6250,
		Holdings: []models.Holding{
			{Ticker: "BHP", Units: 100, CurrentPrice: 45, MarketValue: 4500, Currency: "AUD"},
			{Ticker: "AAPL", Units: 10, CurrentPrice: 200, MarketValue: 2000, Currency: "USD"},
		},
	}

	output := formatSyncResult(portfolio)

	// Should mention the FX rate when it's non-zero
	if !strings.Contains(output, "0.6250") && !strings.Contains(output, "FX") {
		t.Error("formatSyncResult should show FX rate when non-zero")
	}
}
