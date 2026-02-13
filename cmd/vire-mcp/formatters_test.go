package main

import (
	"strings"
	"testing"
	"time"

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
				ActionRequired: "COMPLIANT",
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
				ActionRequired: "COMPLIANT",
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
				ActionRequired: "COMPLIANT",
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

func TestFormatQuote_FullData(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:          "XAGUSD.FOREX",
		Open:          31.10,
		High:          31.50,
		Low:           30.90,
		Close:         31.25,
		PreviousClose: 30.80,
		Change:        0.45,
		ChangePct:     1.46,
		Volume:        12345,
		Timestamp:     time.Date(2026, 2, 13, 9, 30, 0, 0, time.UTC),
	}

	output := formatQuote(quote)

	// Should contain the ticker in a heading
	if !strings.Contains(output, "XAGUSD.FOREX") {
		t.Error("formatQuote output missing ticker")
	}

	// Should contain all price fields
	if !strings.Contains(output, "31.25") {
		t.Error("formatQuote output missing close/price value")
	}
	if !strings.Contains(output, "31.10") {
		t.Error("formatQuote output missing open value")
	}
	if !strings.Contains(output, "31.50") {
		t.Error("formatQuote output missing high value")
	}
	if !strings.Contains(output, "30.90") {
		t.Error("formatQuote output missing low value")
	}
	if !strings.Contains(output, "12345") {
		t.Error("formatQuote output missing volume")
	}

	// Should contain change% and previous close
	if !strings.Contains(output, "1.46%") {
		t.Error("formatQuote output missing change percentage")
	}
	if !strings.Contains(output, "30.80") {
		t.Error("formatQuote output missing previous close")
	}
	if !strings.Contains(output, "+") {
		t.Error("formatQuote output should show + sign for positive change")
	}

	// Should contain timestamp
	if !strings.Contains(output, "2026") {
		t.Error("formatQuote output missing timestamp year")
	}

	// Should be a markdown table
	if !strings.Contains(output, "|") {
		t.Error("formatQuote output should contain markdown table")
	}

	// Should NOT contain $ symbol (forex pairs aren't priced in dollars)
	if strings.Contains(output, "$") {
		t.Error("formatQuote should not hardcode $ symbol")
	}
}

func TestFormatQuote_NegativeChange(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:          "BHP.AU",
		Open:          46.00,
		High:          46.50,
		Low:           44.80,
		Close:         45.00,
		PreviousClose: 46.00,
		Change:        -1.00,
		ChangePct:     -2.17,
		Volume:        500000,
		Timestamp:     time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC),
	}

	output := formatQuote(quote)

	// Negative change should show without + sign
	if !strings.Contains(output, "-1.0000") {
		t.Errorf("formatQuote should show negative change value, got:\n%s", output)
	}
	if !strings.Contains(output, "-2.17%") {
		t.Errorf("formatQuote should show negative change percentage, got:\n%s", output)
	}
}

func TestFormatQuote_ZeroVolume(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:      "AUDUSD.FOREX",
		Open:      0.6500,
		High:      0.6550,
		Low:       0.6480,
		Close:     0.6520,
		Volume:    0,
		Timestamp: time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC),
	}

	output := formatQuote(quote)

	// Should still produce output without errors
	if !strings.Contains(output, "AUDUSD.FOREX") {
		t.Error("formatQuote output missing ticker for zero-volume quote")
	}
	if !strings.Contains(output, "0.6520") {
		t.Error("formatQuote output missing close price for zero-volume quote")
	}
	// When change is zero, change row should be omitted
	if strings.Contains(output, "Change") {
		t.Error("formatQuote should omit Change row when change is zero")
	}
}

func TestFormatQuote_ZeroTimestamp(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:      "BHP.AU",
		Open:      45.00,
		High:      46.00,
		Low:       44.50,
		Close:     45.50,
		Volume:    1000000,
		Timestamp: time.Time{}, // zero value
	}

	output := formatQuote(quote)

	// Should still render without panicking
	if !strings.Contains(output, "BHP.AU") {
		t.Error("formatQuote output missing ticker for zero-timestamp quote")
	}
	if !strings.Contains(output, "45.50") {
		t.Error("formatQuote output missing close price for zero-timestamp quote")
	}
}

func TestFormatQuote_AllZeros(t *testing.T) {
	// Closed market or no data scenario — all fields zero except code
	quote := &models.RealTimeQuote{
		Code:      "AAPL.US",
		Open:      0,
		High:      0,
		Low:       0,
		Close:     0,
		Volume:    0,
		Timestamp: time.Time{},
	}

	output := formatQuote(quote)

	// Should still render a valid table without panicking
	if !strings.Contains(output, "AAPL.US") {
		t.Error("formatQuote should show ticker even with all-zero data")
	}
	if !strings.Contains(output, "| Price |") {
		t.Error("formatQuote should show Price row even when zero")
	}
	// Change and Prev Close rows should be omitted when zero
	if strings.Contains(output, "Change") {
		t.Error("formatQuote should omit Change row when change is zero")
	}
	if strings.Contains(output, "Prev Close") {
		t.Error("formatQuote should omit Prev Close row when previous close is zero")
	}
	// Timestamp should be omitted when zero
	if strings.Contains(output, "Timestamp") {
		t.Error("formatQuote should omit Timestamp row when timestamp is zero")
	}
}

func TestFormatQuote_SmallDecimals(t *testing.T) {
	// CRITICAL: Forex prices must not truncate small decimals
	quote := &models.RealTimeQuote{
		Code:          "AUDUSD.FOREX",
		Open:          0.6523,
		High:          0.6548,
		Low:           0.6501,
		Close:         0.6537,
		PreviousClose: 0.6519,
		Change:        0.0018,
		ChangePct:     0.28,
		Volume:        0,
		Timestamp:     time.Date(2026, 2, 13, 14, 30, 0, 0, time.UTC),
	}

	output := formatQuote(quote)

	// All 4 decimal places must be preserved
	if !strings.Contains(output, "0.6537") {
		t.Errorf("formatQuote must preserve 4 decimal places for forex close price, got:\n%s", output)
	}
	if !strings.Contains(output, "0.6523") {
		t.Errorf("formatQuote must preserve 4 decimal places for forex open price, got:\n%s", output)
	}
	if !strings.Contains(output, "0.6548") {
		t.Errorf("formatQuote must preserve 4 decimal places for forex high price, got:\n%s", output)
	}
	if !strings.Contains(output, "0.6501") {
		t.Errorf("formatQuote must preserve 4 decimal places for forex low price, got:\n%s", output)
	}
	if !strings.Contains(output, "0.6519") {
		t.Errorf("formatQuote must preserve 4 decimal places for forex prev close, got:\n%s", output)
	}
	if !strings.Contains(output, "0.0018") {
		t.Errorf("formatQuote must preserve 4 decimal places for forex change value, got:\n%s", output)
	}
}

func TestFormatQuote_StaleData(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:          "BHP.AU",
		Open:          45.00,
		High:          46.00,
		Low:           44.50,
		Close:         45.50,
		PreviousClose: 45.00,
		Change:        0.50,
		ChangePct:     1.11,
		Volume:        500000,
		Timestamp:     time.Now().Add(-2 * time.Hour),
	}

	output := formatQuote(quote)

	// Should contain stale warning (2h > 15m threshold)
	if !strings.Contains(output, "STALE DATA") {
		t.Error("formatQuote should show STALE DATA warning when quote is stale")
	}
	if !strings.Contains(output, "Verify with a live source") {
		t.Error("formatQuote should advise verification with live source")
	}
	// Should contain the Data Age row in the table
	if !strings.Contains(output, "Data Age") {
		t.Error("formatQuote should show Data Age row in table")
	}
	if !strings.Contains(output, "2h") {
		t.Error("formatQuote Data Age should show 2h for 2-hour-old data")
	}
}

func TestFormatQuote_FreshData(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:          "AAPL.US",
		Open:          180.00,
		High:          182.00,
		Low:           179.50,
		Close:         181.50,
		PreviousClose: 180.00,
		Change:        1.50,
		ChangePct:     0.83,
		Volume:        1000000,
		Timestamp:     time.Now().Add(-2 * time.Minute),
	}

	output := formatQuote(quote)

	// Should NOT contain stale warning (2m < 15m threshold)
	if strings.Contains(output, "STALE DATA") {
		t.Error("formatQuote should not show STALE DATA warning for fresh data")
	}
	// Should still show Data Age row
	if !strings.Contains(output, "Data Age") {
		t.Error("formatQuote should show Data Age row even for fresh data")
	}
	if !strings.Contains(output, "2m") {
		t.Error("formatQuote Data Age should show 2m for 2-minute-old data")
	}
}

func TestFormatQuote_StaleDataDaysOld(t *testing.T) {
	// Simulate weekend scenario: data from Friday, now Sunday
	quote := &models.RealTimeQuote{
		Code:          "CBA.AU",
		Open:          110.00,
		High:          111.00,
		Low:           109.50,
		Close:         110.50,
		PreviousClose: 110.00,
		Change:        0.50,
		ChangePct:     0.45,
		Volume:        200000,
		Timestamp:     time.Now().Add(-48 * time.Hour),
	}

	output := formatQuote(quote)

	if !strings.Contains(output, "STALE DATA") {
		t.Error("formatQuote should show STALE DATA warning for days-old data")
	}
	// Data Age row should show 48h
	if !strings.Contains(output, "48h") {
		t.Errorf("formatQuote Data Age should show 48h, got:\n%s", output)
	}
}

func TestFormatQuote_ZeroTimestampNoStaleness(t *testing.T) {
	quote := &models.RealTimeQuote{
		Code:      "AAPL.US",
		Open:      180.00,
		High:      182.00,
		Low:       179.50,
		Close:     181.50,
		Volume:    1000000,
		Timestamp: time.Time{},
	}

	output := formatQuote(quote)

	// Should NOT contain Data Age row when Timestamp is zero
	if strings.Contains(output, "Data Age") {
		t.Error("formatQuote should omit Data Age row when Timestamp is zero")
	}
	// Should NOT contain stale warning
	if strings.Contains(output, "STALE DATA") {
		t.Error("formatQuote should not show STALE DATA for zero timestamp")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d        time.Duration
		expected string
	}{
		{30 * time.Second, "30s"},
		{2 * time.Minute, "2m"},
		{2*time.Minute + 15*time.Second, "2m 15s"},
		{1 * time.Hour, "1h"},
		{1*time.Hour + 30*time.Minute, "1h 30m"},
		{48 * time.Hour, "48h"},
		{48*time.Hour + 15*time.Minute, "48h 15m"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.d)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, result, tt.expected)
		}
	}
}

func TestFormatPortfolioHistory_DailyPctColumn(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), TotalValue: 10000, TotalCost: 9000, GainLoss: 1000, GainLossPct: 11.11, HoldingCount: 5},
		{Date: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), TotalValue: 10200, TotalCost: 9000, GainLoss: 1200, GainLossPct: 13.33, HoldingCount: 5},
		{Date: time.Date(2026, 2, 12, 0, 0, 0, 0, time.UTC), TotalValue: 10050, TotalCost: 9000, GainLoss: 1050, GainLossPct: 11.67, HoldingCount: 5},
	}

	output := formatPortfolioHistory(points, "daily")

	// Should have "Day %" column header
	if !strings.Contains(output, "Day %") {
		t.Error("daily table should have 'Day %' column header")
	}

	// Day 2: (10200 - 10000) / 10000 * 100 = +2.00%
	if !strings.Contains(output, "+2.00%") {
		t.Errorf("expected +2.00%% for day 2, got:\n%s", output)
	}

	// Day 3: (10050 - 10200) / 10200 * 100 = -1.47%
	if !strings.Contains(output, "-1.47%") {
		t.Errorf("expected -1.47%% for day 3, got:\n%s", output)
	}
}

func TestFormatPortfolioHistory_WeeklyPctColumn(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC), TotalValue: 10000, HoldingCount: 3},
		{Date: time.Date(2026, 2, 14, 0, 0, 0, 0, time.UTC), TotalValue: 10500, HoldingCount: 3},
	}

	output := formatPortfolioHistory(points, "weekly")

	if !strings.Contains(output, "Week %") {
		t.Error("weekly table should have 'Week %' column header")
	}

	// (10500 - 10000) / 10000 * 100 = +5.00%
	if !strings.Contains(output, "+5.00%") {
		t.Errorf("expected +5.00%% for week change, got:\n%s", output)
	}
}

func TestFormatPortfolioHistory_MonthlyPctColumn(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), TotalValue: 10000, HoldingCount: 3},
		{Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), TotalValue: 9800, HoldingCount: 3},
	}

	output := formatPortfolioHistory(points, "monthly")

	if !strings.Contains(output, "Month %") {
		t.Error("monthly table should have 'Month %' column header")
	}

	// (9800 - 10000) / 10000 * 100 = -2.00%
	if !strings.Contains(output, "-2.00%") {
		t.Errorf("expected -2.00%% for month change, got:\n%s", output)
	}
}

func TestFormatPortfolioHistory_FirstRowNoPct(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), TotalValue: 10000, HoldingCount: 3},
	}

	output := formatPortfolioHistory(points, "daily")

	// Single point: should have Day % header but no percentage values in the data row
	if !strings.Contains(output, "Day %") {
		t.Error("daily table should have 'Day %' column header even with single point")
	}

	// Find the data row and check last two cells (Day Change, Day %) are empty
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "Date") || strings.Contains(line, "---") {
			continue
		}
		if strings.Contains(line, "2026-02-10") {
			parts := strings.Split(line, "|")
			// parts: ["", " Date ", " Value ", ..., " Day Change ", " Day % ", ""]
			// Day Change cell (second from last non-empty)
			dayChangeCell := strings.TrimSpace(parts[len(parts)-3])
			dayPctCell := strings.TrimSpace(parts[len(parts)-2])
			if dayChangeCell != "" {
				t.Errorf("first row Day Change should be empty, got: %q", dayChangeCell)
			}
			if dayPctCell != "" {
				t.Errorf("first row Day %% should be empty, got: %q", dayPctCell)
			}
		}
	}
}

func TestFormatPortfolioHistory_ZeroPrevValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), TotalValue: 0, HoldingCount: 0},
		{Date: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), TotalValue: 5000, HoldingCount: 3},
	}

	output := formatPortfolioHistory(points, "daily")

	// With zero previous value, day change dollar should show but pct should be empty
	if !strings.Contains(output, "+$5,000.00") {
		t.Errorf("expected +$5,000.00 day change when going from 0 to 5000, got:\n%s", output)
	}

	// The percentage cell for the second row should be empty (no formatted pct)
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "Date") || strings.Contains(line, "---") {
			continue
		}
		if strings.Contains(line, "2026-02-11") {
			parts := strings.Split(line, "|")
			// Day % is second-to-last element (last is empty string after trailing |)
			if len(parts) >= 2 {
				dayPctCell := strings.TrimSpace(parts[len(parts)-2])
				if dayPctCell != "" {
					t.Errorf("expected empty Day %% cell when prev value is 0, got: %q", dayPctCell)
				}
			}
		}
	}
}

func TestFormatHistoryJSON_PeriodChange(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), TotalValue: 10000, TotalCost: 9000, GainLoss: 1000, GainLossPct: 11.11, HoldingCount: 5},
		{Date: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), TotalValue: 10200, TotalCost: 9000, GainLoss: 1200, GainLossPct: 13.33, HoldingCount: 5},
		{Date: time.Date(2026, 2, 12, 0, 0, 0, 0, time.UTC), TotalValue: 10050, TotalCost: 9000, GainLoss: 1050, GainLossPct: 11.67, HoldingCount: 5},
	}

	output := formatHistoryJSON(points)

	// Should contain period_change fields
	if !strings.Contains(output, "period_change") {
		t.Error("JSON should contain period_change field")
	}
	if !strings.Contains(output, "period_change_pct") {
		t.Error("JSON should contain period_change_pct field")
	}

	// First point: period_change should be 0
	if !strings.Contains(output, `"period_change":0`) {
		t.Errorf("first point period_change should be 0, got:\n%s", output)
	}

	// Second point: period_change = 200
	if !strings.Contains(output, `"period_change":200`) {
		t.Errorf("second point period_change should be 200, got:\n%s", output)
	}

	// Second point: period_change_pct = 2.0 (200/10000*100)
	if !strings.Contains(output, `"period_change_pct":2`) {
		t.Errorf("second point period_change_pct should be 2, got:\n%s", output)
	}

	// Third point: period_change = -150
	if !strings.Contains(output, `"period_change":-150`) {
		t.Errorf("third point period_change should be -150, got:\n%s", output)
	}
}

func TestFormatHistoryJSON_ZeroPrevValue(t *testing.T) {
	points := []models.GrowthDataPoint{
		{Date: time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), TotalValue: 0, HoldingCount: 0},
		{Date: time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC), TotalValue: 5000, TotalCost: 4000, GainLoss: 1000, GainLossPct: 25.0, HoldingCount: 3},
	}

	output := formatHistoryJSON(points)

	// period_change should be 5000, but period_change_pct should be 0 (div by zero guard)
	if !strings.Contains(output, `"period_change":5000`) {
		t.Errorf("period_change should be 5000, got:\n%s", output)
	}

	// With zero prev value, pct should be 0
	// The second point should have period_change_pct:0
	// Check that we don't have Inf or NaN
	if strings.Contains(output, "Inf") || strings.Contains(output, "NaN") {
		t.Errorf("JSON should not contain Inf or NaN, got:\n%s", output)
	}
}
