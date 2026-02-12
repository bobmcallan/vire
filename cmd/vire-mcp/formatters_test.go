package main

import (
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
)

func TestFormatPortfolioHoldings_NoCcyColumn(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 7700,
		TotalCost:  5500,
		TotalGain:  2200,
		FXRate:     0.75,
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

	// Should NOT contain Ccy column header (removed)
	if strings.Contains(output, "| Ccy |") {
		t.Error("formatPortfolioHoldings should not have Ccy column")
	}

	// Should still contain country column header
	if !strings.Contains(output, "Country") {
		t.Error("formatPortfolioHoldings output missing 'Country' column header")
	}

	// Should show country codes
	if !strings.Contains(output, "| AU |") || !strings.Contains(output, "| US |") {
		t.Error("formatPortfolioHoldings output missing country codes in table cells")
	}
}

func TestFormatPortfolioHoldings_FXConversion(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 7700,
		FXRate:     0.75, // 1 AUD = 0.75 USD, so USD 100 = AUD 133.33
		Holdings: []models.Holding{
			{
				Ticker:       "CBOE",
				Exchange:     "US",
				Name:         "Cboe Global Markets",
				Units:        10,
				AvgCost:      100.00,
				CurrentPrice: 100.00,
				MarketValue:  1000.00,
				GainLoss:     0,
				GainLossPct:  0,
				Weight:       50.0,
				Currency:     "USD",
				Country:      "US",
			},
		},
	}

	output := formatPortfolioHoldings(portfolio)

	// USD $100 at rate 0.75 should display as $133.33 (100 / 0.75)
	if !strings.Contains(output, "$133.33") {
		t.Errorf("expected USD $100 to convert to $133.33 AUD at rate 0.75, got:\n%s", output)
	}

	// USD $1000 market value at rate 0.75 should display as $1,333.33
	if !strings.Contains(output, "$1,333.33") {
		t.Errorf("expected USD $1000 to convert to $1,333.33 AUD at rate 0.75, got:\n%s", output)
	}
}

func TestFormatPortfolioHoldings_FXRateNote(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 5000,
		FXRate:     0.7133,
		Holdings: []models.Holding{
			{Ticker: "BHP", Units: 100, CurrentPrice: 50, MarketValue: 5000, Currency: "AUD", Country: "AU"},
		},
	}

	output := formatPortfolioHoldings(portfolio)

	if !strings.Contains(output, "FX Rate (AUDUSD)") {
		t.Error("formatPortfolioHoldings should show FX Rate note when FXRate > 0")
	}
	if !strings.Contains(output, "0.7133") {
		t.Error("formatPortfolioHoldings should display the FX rate value")
	}
	if !strings.Contains(output, "USD holdings converted to AUD") {
		t.Error("formatPortfolioHoldings should include conversion explanation")
	}
}

func TestFormatPortfolioHoldings_NoFXRateNoteWhenZero(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 5000,
		FXRate:     0,
		Holdings: []models.Holding{
			{Ticker: "BHP", Units: 100, CurrentPrice: 50, MarketValue: 5000, Currency: "AUD", Country: "AU"},
		},
	}

	output := formatPortfolioHoldings(portfolio)

	if strings.Contains(output, "FX Rate") {
		t.Error("formatPortfolioHoldings should not show FX Rate note when FXRate is 0")
	}
}

func TestFormatPortfolioHoldings_AUDNotConverted(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 5000,
		FXRate:     0.75,
		Holdings: []models.Holding{
			{
				Ticker:       "BHP",
				Units:        100,
				AvgCost:      40.00,
				CurrentPrice: 50.00,
				MarketValue:  5000.00,
				GainLoss:     1000.00,
				Weight:       100,
				Currency:     "AUD",
				Country:      "AU",
			},
		},
	}

	output := formatPortfolioHoldings(portfolio)

	// AUD values should remain unchanged: $50.00 price, $5,000.00 value
	if !strings.Contains(output, "$50.00") {
		t.Errorf("AUD holding price should remain $50.00, got:\n%s", output)
	}
	if !strings.Contains(output, "$5,000.00") {
		t.Errorf("AUD holding value should remain $5,000.00, got:\n%s", output)
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

func TestFormatPortfolioReview_NoCcyColumn(t *testing.T) {
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

	if strings.Contains(output, "| Ccy |") {
		t.Error("formatPortfolioReview should not have Ccy column")
	}
}

func TestFormatPortfolioReview_FXConversion(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		TotalValue:    7700,
		TotalCost:     5500,
		TotalGain:     2200,
		FXRate:        0.75,
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{
					Ticker:           "CBOE",
					Exchange:         "US",
					Name:             "Cboe Global Markets",
					Units:            10,
					AvgCost:          100.00,
					CurrentPrice:     100.00,
					MarketValue:      1000.00,
					TotalReturnValue: 0,
					Weight:           50.0,
					Currency:         "USD",
					Country:          "US",
				},
				Fundamentals:   &models.Fundamentals{Sector: "Financials"},
				ActionRequired: "HOLD",
			},
		},
	}

	output := formatPortfolioReview(review)

	// USD $100 at rate 0.75 -> AUD $133.33
	if !strings.Contains(output, "$133.33") {
		t.Errorf("expected USD $100 to convert to $133.33 AUD at rate 0.75, got:\n%s", output)
	}
}

func TestFormatPortfolioReview_FXRateNote(t *testing.T) {
	review := &models.PortfolioReview{
		PortfolioName: "SMSF",
		TotalValue:    5000,
		FXRate:        0.7133,
		HoldingReviews: []models.HoldingReview{
			{
				Holding: models.Holding{
					Ticker:       "BHP",
					Units:        100,
					CurrentPrice: 50.00,
					MarketValue:  5000.00,
					Currency:     "AUD",
				},
				Fundamentals:   &models.Fundamentals{Sector: "Materials"},
				ActionRequired: "HOLD",
			},
		},
	}

	output := formatPortfolioReview(review)

	if !strings.Contains(output, "FX Rate (AUDUSD)") {
		t.Error("formatPortfolioReview should show FX Rate note when FXRate > 0")
	}
	if !strings.Contains(output, "USD holdings converted to AUD") {
		t.Error("formatPortfolioReview should include conversion explanation")
	}
}

func TestFormatSyncResult_FXConversion(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 7700,
		Currency:   "AUD",
		FXRate:     0.75,
		Holdings: []models.Holding{
			{Ticker: "BHP", Units: 100, CurrentPrice: 45, MarketValue: 4500, Currency: "AUD"},
			{Ticker: "AAPL", Units: 10, CurrentPrice: 150, MarketValue: 1500, Currency: "USD"},
		},
	}

	output := formatSyncResult(portfolio)

	// USD $150 at rate 0.75 -> AUD $200.00
	if !strings.Contains(output, "$200.00") {
		t.Errorf("expected USD $150 to convert to $200.00 AUD at rate 0.75, got:\n%s", output)
	}

	// USD $1500 at rate 0.75 -> AUD $2,000.00
	if !strings.Contains(output, "$2,000.00") {
		t.Errorf("expected USD $1500 to convert to $2,000.00 AUD at rate 0.75, got:\n%s", output)
	}
}

func TestFormatSyncResult_NoCcyColumn(t *testing.T) {
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

	if strings.Contains(output, "| Ccy |") {
		t.Error("formatSyncResult should not have Ccy column")
	}
}

func TestFormatSyncResult_FXRateNote(t *testing.T) {
	portfolio := &models.Portfolio{
		Name:       "SMSF",
		TotalValue: 7700,
		Currency:   "AUD",
		FXRate:     0.6250,
		Holdings: []models.Holding{
			{Ticker: "BHP", Units: 100, CurrentPrice: 45, MarketValue: 4500, Currency: "AUD"},
		},
	}

	output := formatSyncResult(portfolio)

	if !strings.Contains(output, "0.6250") {
		t.Error("formatSyncResult should show FX rate value")
	}
	if !strings.Contains(output, "FX Rate (AUDUSD)") {
		t.Error("formatSyncResult should show FX Rate label")
	}
	if !strings.Contains(output, "USD holdings converted to AUD") {
		t.Error("formatSyncResult should include conversion explanation")
	}
}
