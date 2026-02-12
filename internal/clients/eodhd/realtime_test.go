package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetRealTimeQuote_ParsesResponse(t *testing.T) {
	ts := int64(1711670340) // 2024-03-28 23:59:00 UTC
	mockResp := map[string]interface{}{
		"code":      "BHP.AU",
		"timestamp": ts,
		"open":      42.10,
		"high":      43.50,
		"low":       41.80,
		"close":     43.25,
		"volume":    float64(5000000),
	}

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	if capturedPath != "/real-time/BHP.AU" {
		t.Errorf("expected path /real-time/BHP.AU, got %s", capturedPath)
	}
	if quote.Code != "BHP.AU" {
		t.Errorf("expected code BHP.AU, got %s", quote.Code)
	}
	if quote.Open != 42.10 {
		t.Errorf("expected open 42.10, got %.2f", quote.Open)
	}
	if quote.High != 43.50 {
		t.Errorf("expected high 43.50, got %.2f", quote.High)
	}
	if quote.Low != 41.80 {
		t.Errorf("expected low 41.80, got %.2f", quote.Low)
	}
	if quote.Close != 43.25 {
		t.Errorf("expected close 43.25, got %.2f", quote.Close)
	}
	if quote.Volume != 5000000 {
		t.Errorf("expected volume 5000000, got %d", quote.Volume)
	}
	expectedTime := time.Unix(ts, 0)
	if !quote.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, quote.Timestamp)
	}
}

func TestGetRealTimeQuote_ForexTicker(t *testing.T) {
	mockResp := map[string]interface{}{
		"code":      "XAGUSD.FOREX",
		"timestamp": int64(1711670000),
		"open":      24.50,
		"high":      25.10,
		"low":       24.30,
		"close":     24.95,
		"volume":    float64(0),
	}

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "XAGUSD.FOREX")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	if capturedPath != "/real-time/XAGUSD.FOREX" {
		t.Errorf("expected path /real-time/XAGUSD.FOREX, got %s", capturedPath)
	}
	if quote.Close != 24.95 {
		t.Errorf("expected close 24.95, got %.2f", quote.Close)
	}
}

func TestGetRealTimeQuote_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("ticker not found"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetRealTimeQuote(context.Background(), "INVALID.XX")
	if err == nil {
		t.Fatal("expected error for invalid ticker")
	}
}

func TestGetRealTimeQuote_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected error on server error")
	}
}

func TestGetRealTimeQuote_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL), WithTimeout(100*time.Millisecond))
	_, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestGetRealTimeQuote_EmptyResponse(t *testing.T) {
	// EODHD may return an object with zero values when market is closed
	mockResp := map[string]interface{}{
		"code":      "BHP.AU",
		"timestamp": int64(0),
		"open":      0.0,
		"high":      0.0,
		"low":       0.0,
		"close":     0.0,
		"volume":    float64(0),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	// Zero close is valid — callers should check quote.Close > 0 before using
	if quote.Close != 0 {
		t.Errorf("expected close 0, got %.2f", quote.Close)
	}
}

func TestGetRealTimeQuote_StringTimestamp(t *testing.T) {
	// EODHD sometimes returns timestamp and volume as strings
	mockResp := map[string]interface{}{
		"code":      "CBOE.AU",
		"timestamp": "1711670340",
		"open":      "42.10",
		"high":      "43.50",
		"low":       "41.80",
		"close":     "43.25",
		"volume":    "5000000",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "CBOE.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed with string fields: %v", err)
	}

	if quote.Code != "CBOE.AU" {
		t.Errorf("expected code CBOE.AU, got %s", quote.Code)
	}
	if quote.Open != 42.10 {
		t.Errorf("expected open 42.10, got %.2f", quote.Open)
	}
	if quote.Close != 43.25 {
		t.Errorf("expected close 43.25, got %.2f", quote.Close)
	}
	if quote.Volume != 5000000 {
		t.Errorf("expected volume 5000000, got %d", quote.Volume)
	}
	expectedTime := time.Unix(1711670340, 0)
	if !quote.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, quote.Timestamp)
	}
}

func TestFlexInt64_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"number", "1711670340", 1711670340},
		{"string", `"1711670340"`, 1711670340},
		{"zero", "0", 0},
		{"string_zero", `"0"`, 0},
		{"empty_string", `""`, 0},
		{"na_string", `"N/A"`, 0},
		{"negative", "-100", -100},
		{"string_negative", `"-100"`, -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f flexInt64
			if err := json.Unmarshal([]byte(tt.input), &f); err != nil {
				t.Fatalf("UnmarshalJSON(%s) error: %v", tt.input, err)
			}
			if int64(f) != tt.expected {
				t.Errorf("UnmarshalJSON(%s) = %d, want %d", tt.input, int64(f), tt.expected)
			}
		})
	}
}

func TestGetRealTimeQuote_RateLimited(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limit exceeded"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "BHP.AU", "timestamp": int64(1711670340),
			"open": 42.0, "high": 43.0, "low": 41.0, "close": 42.5, "volume": float64(1000),
		})
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	// First call should get rate limit error
	_, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected rate limit error on first call")
	}
}
