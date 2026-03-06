package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetBulkRealTimeQuotes_MultiTicker(t *testing.T) {
	responses := []map[string]interface{}{
		{
			"code":          "BHP.AU",
			"timestamp":     int64(1711670340),
			"open":          42.10,
			"high":          43.50,
			"low":           41.80,
			"close":         43.25,
			"previousClose": 42.00,
			"change":        1.25,
			"change_p":      2.98,
			"volume":        float64(5000000),
		},
		{
			"code":          "CBA.AU",
			"timestamp":     int64(1711670340),
			"open":          110.00,
			"high":          112.50,
			"low":           109.50,
			"close":         111.75,
			"previousClose": 110.00,
			"change":        1.75,
			"change_p":      1.59,
			"volume":        float64(3000000),
		},
		{
			"code":          "CSL.AU",
			"timestamp":     int64(1711670340),
			"open":          290.00,
			"high":          295.00,
			"low":           288.00,
			"close":         293.50,
			"previousClose": 290.00,
			"change":        3.50,
			"change_p":      1.21,
			"volume":        float64(1000000),
		},
	}

	var capturedPath string
	var capturedS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedS = r.URL.Query().Get("s")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responses)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "CBA.AU", "CSL.AU"})
	if err != nil {
		t.Fatalf("GetBulkRealTimeQuotes failed: %v", err)
	}

	if capturedPath != "/real-time/BHP.AU" {
		t.Errorf("expected path /real-time/BHP.AU, got %s", capturedPath)
	}
	if capturedS != "CBA.AU,CSL.AU" {
		t.Errorf("expected s=CBA.AU,CSL.AU, got %s", capturedS)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	bhp := result["BHP.AU"]
	if bhp == nil {
		t.Fatal("BHP.AU not found in results")
	}
	if bhp.Open != 42.10 {
		t.Errorf("expected open 42.10, got %.2f", bhp.Open)
	}
	if bhp.Close != 43.25 {
		t.Errorf("expected close 43.25, got %.2f", bhp.Close)
	}
	if bhp.Volume != 5000000 {
		t.Errorf("expected volume 5000000, got %d", bhp.Volume)
	}
	if bhp.PreviousClose != 42.00 {
		t.Errorf("expected previousClose 42.00, got %.2f", bhp.PreviousClose)
	}
	if bhp.Source != "eodhd" {
		t.Errorf("expected source eodhd, got %s", bhp.Source)
	}

	cba := result["CBA.AU"]
	if cba == nil {
		t.Fatal("CBA.AU not found in results")
	}
	if cba.Close != 111.75 {
		t.Errorf("expected close 111.75, got %.2f", cba.Close)
	}
}

func TestGetBulkRealTimeQuotes_SingleTicker(t *testing.T) {
	// Single ticker: EODHD returns a JSON object, not an array
	mockResp := map[string]interface{}{
		"code":          "BHP.AU",
		"timestamp":     int64(1711670340),
		"open":          42.10,
		"high":          43.50,
		"low":           41.80,
		"close":         43.25,
		"previousClose": 42.00,
		"change":        1.25,
		"change_p":      2.98,
		"volume":        float64(5000000),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU"})
	if err != nil {
		t.Fatalf("GetBulkRealTimeQuotes single ticker failed: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}

	bhp := result["BHP.AU"]
	if bhp == nil {
		t.Fatal("BHP.AU not found in results")
	}
	if bhp.Close != 43.25 {
		t.Errorf("expected close 43.25, got %.2f", bhp.Close)
	}
}

func TestGetBulkRealTimeQuotes_EmptyTickers(t *testing.T) {
	client := NewClient("test-key")
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{})
	if err != nil {
		t.Fatalf("GetBulkRealTimeQuotes empty tickers failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

func TestGetBulkRealTimeQuotes_SkipsMissingCode(t *testing.T) {
	// Response includes an entry with empty code — should be skipped
	responses := []map[string]interface{}{
		{
			"code":      "BHP.AU",
			"timestamp": int64(1711670340),
			"close":     43.25,
			"volume":    float64(5000000),
		},
		{
			"code":      "",
			"timestamp": int64(1711670340),
			"close":     0.0,
			"volume":    float64(0),
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responses)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "INVALID.AU"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 result (empty code skipped), got %d", len(result))
	}
}
