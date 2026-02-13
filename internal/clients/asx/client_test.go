package asx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetRealTimeQuote_ParsesResponse(t *testing.T) {
	mockResp := headerResponse{
		Data: struct {
			PriceLast          float64 `json:"priceLast"`
			PriceChange        float64 `json:"priceChange"`
			PriceChangePercent float64 `json:"priceChangePercent"`
			Volume             int64   `json:"volume"`
		}{
			PriceLast:          152.50,
			PriceChange:        2.30,
			PriceChangePercent: 1.53,
			Volume:             3500000,
		},
	}

	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL))
	before := time.Now()
	quote, err := client.GetRealTimeQuote(context.Background(), "ETPMAG.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}

	if capturedPath != "/companies/etpmag/header" {
		t.Errorf("expected path /companies/etpmag/header, got %s", capturedPath)
	}
	if quote.Code != "ETPMAG.AU" {
		t.Errorf("expected code ETPMAG.AU, got %s", quote.Code)
	}
	if quote.Close != 152.50 {
		t.Errorf("expected close 152.50, got %.2f", quote.Close)
	}
	if quote.Change != 2.30 {
		t.Errorf("expected change 2.30, got %.2f", quote.Change)
	}
	if quote.ChangePct != 1.53 {
		t.Errorf("expected change_p 1.53, got %.2f", quote.ChangePct)
	}
	if quote.Volume != 3500000 {
		t.Errorf("expected volume 3500000, got %d", quote.Volume)
	}
	if quote.Source != "asx" {
		t.Errorf("expected source asx, got %s", quote.Source)
	}
	if quote.Timestamp.Before(before) {
		t.Errorf("expected timestamp >= test start, got %v", quote.Timestamp)
	}
	// ASX Markit does not return open/high/low/previousClose — verify zero values
	if quote.Open != 0 {
		t.Errorf("expected open 0 (not provided by ASX API), got %.2f", quote.Open)
	}
	if quote.High != 0 {
		t.Errorf("expected high 0 (not provided by ASX API), got %.2f", quote.High)
	}
	if quote.Low != 0 {
		t.Errorf("expected low 0 (not provided by ASX API), got %.2f", quote.Low)
	}
	if quote.PreviousClose != 0 {
		t.Errorf("expected previous close 0 (not provided by ASX API), got %.2f", quote.PreviousClose)
	}
}

func TestGetRealTimeQuote_StripsExchangeSuffix(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(headerResponse{})
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL))

	tests := []struct {
		ticker       string
		expectedPath string
	}{
		{"BHP.AU", "/companies/bhp/header"},
		{"PMGOLD.AU", "/companies/pmgold/header"},
		{"CBA.AU", "/companies/cba/header"},
	}

	for _, tt := range tests {
		quote, err := client.GetRealTimeQuote(context.Background(), tt.ticker)
		if err != nil {
			t.Fatalf("GetRealTimeQuote(%s) failed: %v", tt.ticker, err)
		}
		if capturedPath != tt.expectedPath {
			t.Errorf("ticker %s: expected path %s, got %s", tt.ticker, tt.expectedPath, capturedPath)
		}
		if quote.Code != tt.ticker {
			t.Errorf("ticker %s: expected code preserved as %s, got %s", tt.ticker, tt.ticker, quote.Code)
		}
	}
}

func TestGetRealTimeQuote_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL))
	_, err := client.GetRealTimeQuote(context.Background(), "INVALID.AU")
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

	client := NewClient(WithBaseURL(srv.URL))
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

	client := NewClient(WithBaseURL(srv.URL), WithTimeout(100*time.Millisecond))
	_, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestGetRealTimeQuote_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL))
	_, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestGetRealTimeQuote_ZeroPriceIsValid(t *testing.T) {
	// Market may be closed, returning zero prices
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(headerResponse{})
	}))
	defer srv.Close()

	client := NewClient(WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("GetRealTimeQuote failed: %v", err)
	}
	if quote.Close != 0 {
		t.Errorf("expected close 0, got %.2f", quote.Close)
	}
}
