package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetFundamentals_ParsesAnalystRatings(t *testing.T) {
	mockResp := map[string]interface{}{
		"General": map[string]interface{}{
			"Code": "BHP", "Name": "BHP Group", "Type": "Common Stock",
			"CountryISO": "AU", "Sector": "Basic Materials", "Industry": "Mining",
		},
		"Highlights": map[string]interface{}{
			"MarketCapitalization": 150000000000.0,
			"PERatio":              12.5,
			"EarningsShare":        3.50,
			"DividendYield":        0.045,
		},
		"Valuation":   map[string]interface{}{"PriceBookMRQ": 2.1},
		"SharesStats": map[string]interface{}{"SharesOutstanding": 5000000000.0, "SharesFloat": 4900000000.0},
		"Technicals":  map[string]interface{}{"Beta": 1.1},
		"AnalystRatings": map[string]interface{}{
			"Rating": "Buy", "TargetPrice": 48.50,
			"StrongBuy": 8, "Buy": 12, "Hold": 5, "Sell": 1, "StrongSell": 0,
		},
		"ETF_Data": map[string]interface{}{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	fundamentals, err := client.GetFundamentals(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetFundamentals failed: %v", err)
	}

	// Verify analyst ratings
	if fundamentals.AnalystRatings == nil {
		t.Fatal("AnalystRatings is nil")
	}
	if fundamentals.AnalystRatings.Rating != "Buy" {
		t.Errorf("AnalystRatings.Rating = %s, want Buy", fundamentals.AnalystRatings.Rating)
	}
	if fundamentals.AnalystRatings.TargetPrice != 48.50 {
		t.Errorf("AnalystRatings.TargetPrice = %.2f, want 48.50", fundamentals.AnalystRatings.TargetPrice)
	}
	if fundamentals.AnalystRatings.StrongBuy != 8 {
		t.Errorf("AnalystRatings.StrongBuy = %d, want 8", fundamentals.AnalystRatings.StrongBuy)
	}
	if fundamentals.AnalystRatings.Buy != 12 {
		t.Errorf("AnalystRatings.Buy = %d, want 12", fundamentals.AnalystRatings.Buy)
	}
}

func TestGetFundamentals_ParsesExpandedHighlights(t *testing.T) {
	mockResp := map[string]interface{}{
		"General": map[string]interface{}{
			"Code": "SKS", "Name": "SKS Technologies Group", "Type": "Common Stock",
			"CountryISO": "AU", "Sector": "Industrials", "Industry": "Electrical Equipment",
		},
		"Highlights": map[string]interface{}{
			"MarketCapitalization":       470510000.0,
			"PERatio":                    31.38,
			"EarningsShare":              0.13,
			"DividendYield":              0.0142,
			"ForwardPE":                  20.13,
			"PEGRatio":                   0.81,
			"ProfitMargin":               0.0536,
			"OperatingMarginTTM":         0.0759,
			"ReturnOnEquityTTM":          0.7647,
			"ReturnOnAssetsTTM":          0.1459,
			"RevenueTTM":                 261660000.0,
			"RevenuePerShareTTM":         2.27,
			"GrossProfitTTM":             138340000.0,
			"EBITDA":                     24500000.0,
			"EPSEstimateCurrentYear":     0.20,
			"EPSEstimateNextYear":        0.25,
			"QuarterlyRevenueGrowthYOY":  0.92,
			"QuarterlyEarningsGrowthYOY": 1.12,
			"WallStreetTargetPrice":      4.50,
			"MostRecentQuarter":          "2025-06-30",
		},
		"Valuation":   map[string]interface{}{"PriceBookMRQ": 20.09},
		"SharesStats": map[string]interface{}{"SharesOutstanding": 115300000.0, "SharesFloat": 110000000.0},
		"Technicals":  map[string]interface{}{"Beta": 0.34},
		"AnalystRatings": map[string]interface{}{
			"Rating": "Buy", "TargetPrice": 4.50,
			"StrongBuy": 2, "Buy": 3, "Hold": 1, "Sell": 0, "StrongSell": 0,
		},
		"ETF_Data": map[string]interface{}{},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	f, err := client.GetFundamentals(context.Background(), "SKS.AU")
	if err != nil {
		t.Fatalf("GetFundamentals failed: %v", err)
	}

	// Verify extended highlights
	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"ForwardPE", f.ForwardPE, 20.13},
		{"PEGRatio", f.PEGRatio, 0.81},
		{"ProfitMargin", f.ProfitMargin, 0.0536},
		{"OperatingMarginTTM", f.OperatingMarginTTM, 0.0759},
		{"ReturnOnEquityTTM", f.ReturnOnEquityTTM, 0.7647},
		{"ReturnOnAssetsTTM", f.ReturnOnAssetsTTM, 0.1459},
		{"RevenueTTM", f.RevenueTTM, 261660000.0},
		{"RevenuePerShareTTM", f.RevenuePerShareTTM, 2.27},
		{"GrossProfitTTM", f.GrossProfitTTM, 138340000.0},
		{"EBITDA", f.EBITDA, 24500000.0},
		{"EPSEstimateCurrent", f.EPSEstimateCurrent, 0.20},
		{"EPSEstimateNext", f.EPSEstimateNext, 0.25},
		{"RevGrowthYOY", f.RevGrowthYOY, 0.92},
		{"EarningsGrowthYOY", f.EarningsGrowthYOY, 1.12},
	}

	for _, tc := range tests {
		if tc.got != tc.want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	if f.MostRecentQuarter != "2025-06-30" {
		t.Errorf("MostRecentQuarter = %s, want 2025-06-30", f.MostRecentQuarter)
	}
}

func TestGetFundamentals_ParsesHistoricalFinancials(t *testing.T) {
	// EODHD returns Income_Statement values as strings, not numbers.
	// This test verifies flexFloat64 handles the real API format.
	mockResp := map[string]interface{}{
		"General": map[string]interface{}{
			"Code": "SKS", "Name": "SKS Technologies Group", "Type": "Common Stock",
			"CountryISO": "AU", "Sector": "Industrials", "Industry": "Electrical Equipment",
		},
		"Highlights":     map[string]interface{}{"MarketCapitalization": 470510000.0},
		"Valuation":      map[string]interface{}{},
		"SharesStats":    map[string]interface{}{},
		"Technicals":     map[string]interface{}{},
		"AnalystRatings": map[string]interface{}{},
		"ETF_Data":       map[string]interface{}{},
		"Financials": map[string]interface{}{
			"Income_Statement": map[string]interface{}{
				"yearly": map[string]interface{}{
					"2024-06-30": map[string]interface{}{
						"totalRevenue": "261660000.00",
						"netIncome":    "14030000.00",
						"grossProfit":  "138340000.00",
						"ebitda":       "24500000.00",
					},
					"2023-06-30": map[string]interface{}{
						"totalRevenue": "136080000.00",
						"netIncome":    "6590000.00",
						"grossProfit":  "61930000.00",
						"ebitda":       "11140000.00",
					},
				},
			},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	f, err := client.GetFundamentals(context.Background(), "SKS.AU")
	if err != nil {
		t.Fatalf("GetFundamentals failed: %v", err)
	}

	if len(f.HistoricalFinancials) != 2 {
		t.Fatalf("HistoricalFinancials length = %d, want 2", len(f.HistoricalFinancials))
	}

	// Should be sorted descending by date
	if f.HistoricalFinancials[0].Date != "2024-06-30" {
		t.Errorf("first period date = %s, want 2024-06-30", f.HistoricalFinancials[0].Date)
	}
	if f.HistoricalFinancials[0].Revenue != 261660000.0 {
		t.Errorf("2024 Revenue = %v, want 261660000", f.HistoricalFinancials[0].Revenue)
	}
	if f.HistoricalFinancials[0].NetIncome != 14030000.0 {
		t.Errorf("2024 NetIncome = %v, want 14030000", f.HistoricalFinancials[0].NetIncome)
	}
	if f.HistoricalFinancials[1].Date != "2023-06-30" {
		t.Errorf("second period date = %s, want 2023-06-30", f.HistoricalFinancials[1].Date)
	}
	if f.HistoricalFinancials[1].Revenue != 136080000.0 {
		t.Errorf("2023 Revenue = %v, want 136080000", f.HistoricalFinancials[1].Revenue)
	}
}

func TestGetFundamentals_NoAnalystRatings(t *testing.T) {
	// Response with no analyst ratings (e.g. an ETF)
	mockResp := map[string]interface{}{
		"General": map[string]interface{}{
			"Code": "VAS", "Name": "Vanguard Australian Shares ETF", "Type": "ETF",
		},
		"Highlights": map[string]interface{}{
			"MarketCapitalization": 10000000000.0,
		},
		"Valuation":   map[string]interface{}{},
		"SharesStats": map[string]interface{}{},
		"Technicals":  map[string]interface{}{},
		"ETF_Data": map[string]interface{}{
			"Net_Expense_Ratio": 0.07,
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	fundamentals, err := client.GetFundamentals(context.Background(), "VAS.AU")
	if err != nil {
		t.Fatalf("GetFundamentals failed: %v", err)
	}

	if fundamentals.AnalystRatings != nil {
		t.Error("expected nil analyst ratings for ETF")
	}
}
