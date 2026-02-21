package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetBulkEOD_StringFields(t *testing.T) {
	// AU exchange returns price/volume fields as strings
	mockResp := `[
		{
			"code": "BHP",
			"exchange_short": "AU",
			"date": "2025-03-28",
			"open": "42.10",
			"high": "43.50",
			"low": "41.80",
			"close": "43.25",
			"adjusted_close": "43.25",
			"volume": "5000000"
		},
		{
			"code": "RIO",
			"exchange_short": "AU",
			"date": "2025-03-28",
			"open": "110.50",
			"high": "112.00",
			"low": "109.80",
			"close": "111.75",
			"adjusted_close": "111.75",
			"volume": "3000000"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkEOD(context.Background(), "AU", []string{"BHP.AU", "RIO.AU"})
	if err != nil {
		t.Fatalf("GetBulkEOD failed: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}

	bhp := result["BHP.AU"]
	if bhp.Open != 42.10 {
		t.Errorf("BHP open = %.2f, want 42.10", bhp.Open)
	}
	if bhp.High != 43.50 {
		t.Errorf("BHP high = %.2f, want 43.50", bhp.High)
	}
	if bhp.Low != 41.80 {
		t.Errorf("BHP low = %.2f, want 41.80", bhp.Low)
	}
	if bhp.Close != 43.25 {
		t.Errorf("BHP close = %.2f, want 43.25", bhp.Close)
	}
	if bhp.AdjClose != 43.25 {
		t.Errorf("BHP adjclose = %.2f, want 43.25", bhp.AdjClose)
	}
	if bhp.Volume != 5000000 {
		t.Errorf("BHP volume = %d, want 5000000", bhp.Volume)
	}

	rio := result["RIO.AU"]
	if rio.Close != 111.75 {
		t.Errorf("RIO close = %.2f, want 111.75", rio.Close)
	}
}

func TestGetBulkEOD_NumericFields(t *testing.T) {
	// Some exchanges return price/volume fields as numbers
	mockResp := `[
		{
			"code": "AAPL",
			"exchange_short": "US",
			"date": "2025-03-28",
			"open": 175.50,
			"high": 178.00,
			"low": 174.25,
			"close": 177.80,
			"adjusted_close": 177.80,
			"volume": 45000000
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkEOD(context.Background(), "US", []string{"AAPL.US"})
	if err != nil {
		t.Fatalf("GetBulkEOD failed: %v", err)
	}

	aapl := result["AAPL.US"]
	if aapl.Open != 175.50 {
		t.Errorf("AAPL open = %.2f, want 175.50", aapl.Open)
	}
	if aapl.Close != 177.80 {
		t.Errorf("AAPL close = %.2f, want 177.80", aapl.Close)
	}
	if aapl.Volume != 45000000 {
		t.Errorf("AAPL volume = %d, want 45000000", aapl.Volume)
	}
}

func TestGetBulkEOD_MixedFields(t *testing.T) {
	// Edge case: some fields as strings, some as numbers (defensive)
	mockResp := `[
		{
			"code": "WOW",
			"exchange_short": "AU",
			"date": "2025-03-28",
			"open": "30.50",
			"high": 31.00,
			"low": "29.80",
			"close": 30.75,
			"adjusted_close": "30.75",
			"volume": "2000000"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkEOD(context.Background(), "AU", []string{"WOW.AU"})
	if err != nil {
		t.Fatalf("GetBulkEOD failed: %v", err)
	}

	wow := result["WOW.AU"]
	if wow.Open != 30.50 {
		t.Errorf("WOW open = %.2f, want 30.50", wow.Open)
	}
	if wow.High != 31.00 {
		t.Errorf("WOW high = %.2f, want 31.00", wow.High)
	}
	if wow.Close != 30.75 {
		t.Errorf("WOW close = %.2f, want 30.75", wow.Close)
	}
	if wow.Volume != 2000000 {
		t.Errorf("WOW volume = %d, want 2000000", wow.Volume)
	}
}

func TestGetBulkEOD_EmptyTickers(t *testing.T) {
	client := NewClient("test-key")
	result, err := client.GetBulkEOD(context.Background(), "AU", []string{})
	if err != nil {
		t.Fatalf("GetBulkEOD with empty tickers should not fail: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestBulkEODResponse_NullAndEmptyValues(t *testing.T) {
	// Test that null/empty string values unmarshal to zero
	tests := []struct {
		name   string
		json   string
		expect float64
	}{
		{"null_open", `{"code":"X","date":"2025-01-01","open":null,"high":0,"low":0,"close":0,"adjusted_close":0,"volume":0}`, 0},
		{"empty_open", `{"code":"X","date":"2025-01-01","open":"","high":0,"low":0,"close":0,"adjusted_close":0,"volume":0}`, 0},
		{"na_open", `{"code":"X","date":"2025-01-01","open":"N/A","high":0,"low":0,"close":0,"adjusted_close":0,"volume":0}`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var row bulkEODResponse
			if err := json.Unmarshal([]byte(tt.json), &row); err != nil {
				t.Fatalf("Unmarshal failed: %v", err)
			}
			if float64(row.Open) != tt.expect {
				t.Errorf("open = %f, want %f", float64(row.Open), tt.expect)
			}
		})
	}

	// Test null volume separately since it checks a different field
	t.Run("null_volume", func(t *testing.T) {
		var row bulkEODResponse
		data := `{"code":"X","date":"2025-01-01","open":0,"high":0,"low":0,"close":0,"adjusted_close":0,"volume":null}`
		if err := json.Unmarshal([]byte(data), &row); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if int64(row.Volume) != 0 {
			t.Errorf("volume = %d, want 0", int64(row.Volume))
		}
	})
}
