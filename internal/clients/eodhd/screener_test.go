package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobmccarthy/vire/internal/models"
)

func TestScreenStocks_ParsesResponse(t *testing.T) {
	// Mock screener API response (EODHD returns {"data": [...]} wrapper)
	mockResponse := map[string]interface{}{
		"data": []map[string]interface{}{
			{
				"code":                  "BHP",
				"name":                  "BHP Group Limited",
				"exchange":              "AU",
				"sector":                "Basic Materials",
				"industry":              "Other Industrial Metals & Mining",
				"market_capitalization": 180000000000.0,
				"earnings_share":        3.50,
				"dividend_yield":        0.045,
				"adjusted_close":        42.50,
				"currency_symbol":       "AUD",
				"refund_1d_p":           1.2,
				"refund_5d_p":           3.5,
				"avgvol_200d":           5000000.0,
			},
			{
				"code":                  "CBA",
				"name":                  "Commonwealth Bank",
				"exchange":              "AU",
				"sector":                "Financials",
				"industry":              "Banks",
				"market_capitalization": 200000000000.0,
				"earnings_share":        5.80,
				"dividend_yield":        0.038,
				"adjusted_close":        115.00,
				"currency_symbol":       "AUD",
				"refund_1d_p":           0.5,
				"refund_5d_p":           -1.2,
				"avgvol_200d":           3000000.0,
			},
		},
	}

	var capturedPath string
	var capturedFilters string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedFilters = r.URL.Query().Get("filters")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	opts := models.ScreenerOptions{
		Filters: []models.ScreenerFilter{
			{Field: "exchange", Operator: "=", Value: "AU"},
			{Field: "market_capitalization", Operator: ">", Value: 1000000000},
			{Field: "earnings_share", Operator: ">", Value: 0},
		},
		Sort:  "market_capitalization.desc",
		Limit: 100,
	}

	results, err := client.ScreenStocks(context.Background(), opts)
	if err != nil {
		t.Fatalf("ScreenStocks failed: %v", err)
	}

	// Verify API was called with correct path
	if capturedPath != "/screener" {
		t.Errorf("Expected path /screener, got %s", capturedPath)
	}

	// Verify filters were sent as JSON
	if capturedFilters == "" {
		t.Fatal("Expected filters parameter to be set")
	}

	var parsedFilters [][]interface{}
	if err := json.Unmarshal([]byte(capturedFilters), &parsedFilters); err != nil {
		t.Fatalf("Failed to parse filters JSON: %v", err)
	}
	if len(parsedFilters) != 3 {
		t.Errorf("Expected 3 filters, got %d", len(parsedFilters))
	}

	// Verify results
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}

	bhp := results[0]
	if bhp.Code != "BHP" {
		t.Errorf("Expected code 'BHP', got '%s'", bhp.Code)
	}
	if bhp.Name != "BHP Group Limited" {
		t.Errorf("Expected name 'BHP Group Limited', got '%s'", bhp.Name)
	}
	if bhp.Exchange != "AU" {
		t.Errorf("Expected exchange 'AU', got '%s'", bhp.Exchange)
	}
	if bhp.Sector != "Basic Materials" {
		t.Errorf("Expected sector 'Basic Materials', got '%s'", bhp.Sector)
	}
	if bhp.MarketCap != 180000000000.0 {
		t.Errorf("Expected market cap 180B, got %.0f", bhp.MarketCap)
	}
	if bhp.EarningsShare != 3.50 {
		t.Errorf("Expected EPS 3.50, got %.2f", bhp.EarningsShare)
	}
	if bhp.DividendYield != 0.045 {
		t.Errorf("Expected dividend yield 0.045, got %.4f", bhp.DividendYield)
	}

	cba := results[1]
	if cba.Code != "CBA" {
		t.Errorf("Expected code 'CBA', got '%s'", cba.Code)
	}
}

func TestScreenStocks_LimitClamping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := r.URL.Query().Get("limit")
		if limit != "100" {
			t.Errorf("Expected limit '100', got '%s'", limit)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{}})
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	// Limit > 100 should be clamped to 100
	_, err := client.ScreenStocks(context.Background(), models.ScreenerOptions{
		Limit: 200,
	})
	if err != nil {
		t.Fatalf("ScreenStocks failed: %v", err)
	}
}

func TestScreenStocks_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{}})
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	results, err := client.ScreenStocks(context.Background(), models.ScreenerOptions{
		Filters: []models.ScreenerFilter{
			{Field: "exchange", Operator: "=", Value: "AU"},
		},
	})
	if err != nil {
		t.Fatalf("ScreenStocks failed: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(results))
	}
}

func TestScreenStocks_WithSignals(t *testing.T) {
	var capturedSignals string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSignals = r.URL.Query().Get("signals")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{}})
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	_, err := client.ScreenStocks(context.Background(), models.ScreenerOptions{
		Signals: []string{"50d_new_hi", "bookvalue_pos"},
	})
	if err != nil {
		t.Fatalf("ScreenStocks failed: %v", err)
	}

	if capturedSignals != "50d_new_hi,bookvalue_pos" {
		t.Errorf("Expected signals '50d_new_hi,bookvalue_pos', got '%s'", capturedSignals)
	}
}

func TestScreenStocks_WithOffset(t *testing.T) {
	var capturedOffset string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedOffset = r.URL.Query().Get("offset")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{}})
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	_, err := client.ScreenStocks(context.Background(), models.ScreenerOptions{
		Offset: 100,
	})
	if err != nil {
		t.Fatalf("ScreenStocks failed: %v", err)
	}

	if capturedOffset != "100" {
		t.Errorf("Expected offset '100', got '%s'", capturedOffset)
	}
}

func TestScreenStocks_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limit exceeded"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	_, err := client.ScreenStocks(context.Background(), models.ScreenerOptions{
		Filters: []models.ScreenerFilter{
			{Field: "exchange", Operator: "=", Value: "AU"},
		},
	})
	if err == nil {
		t.Fatal("Expected error on API error response")
	}
}
