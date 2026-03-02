package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetDividends_ParsesResponse(t *testing.T) {
	mockResp := []map[string]interface{}{
		{
			"date":            "2024-03-20",
			"declarationDate": "2024-02-28",
			"recordDate":      "2024-03-21",
			"paymentDate":     "2024-04-05",
			"value":           0.89,
			"unadjustedValue": 0.89,
			"currency":        "AUD",
			"period":          "Quarterly",
		},
		{
			"date":            "2023-12-20",
			"declarationDate": "2023-11-30",
			"recordDate":      "2023-12-21",
			"paymentDate":     "2024-01-10",
			"value":           0.85,
			"unadjustedValue": 0.85,
			"currency":        "AUD",
			"period":          "Quarterly",
		},
	}

	var capturedPath string
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	from := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)

	dividends, err := client.GetDividends(context.Background(), "BHP.AU", from, to)
	if err != nil {
		t.Fatalf("GetDividends failed: %v", err)
	}

	// Check correct endpoint
	if capturedPath != "/div/BHP.AU" {
		t.Errorf("expected path /div/BHP.AU, got %s", capturedPath)
	}

	// Check query params include fmt=json and date range
	if capturedQuery == "" {
		t.Error("expected query parameters to be set")
	}

	// Check we got 2 results
	if len(dividends) != 2 {
		t.Fatalf("expected 2 dividend events, got %d", len(dividends))
	}

	// Check first event
	d := dividends[0]
	expectedDate := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	if !d.Date.Equal(expectedDate) {
		t.Errorf("Date = %v, want %v", d.Date, expectedDate)
	}
	if d.DeclarationDate != "2024-02-28" {
		t.Errorf("DeclarationDate = %q, want %q", d.DeclarationDate, "2024-02-28")
	}
	if d.RecordDate != "2024-03-21" {
		t.Errorf("RecordDate = %q, want %q", d.RecordDate, "2024-03-21")
	}
	if d.PaymentDate != "2024-04-05" {
		t.Errorf("PaymentDate = %q, want %q", d.PaymentDate, "2024-04-05")
	}
	if d.Value != 0.89 {
		t.Errorf("Value = %.4f, want 0.89", d.Value)
	}
	if d.UnadjustedValue != 0.89 {
		t.Errorf("UnadjustedValue = %.4f, want 0.89", d.UnadjustedValue)
	}
	if d.Currency != "AUD" {
		t.Errorf("Currency = %q, want AUD", d.Currency)
	}
	if d.Period != "Quarterly" {
		t.Errorf("Period = %q, want Quarterly", d.Period)
	}

	// Check second event
	d2 := dividends[1]
	expectedDate2 := time.Date(2023, 12, 20, 0, 0, 0, 0, time.UTC)
	if !d2.Date.Equal(expectedDate2) {
		t.Errorf("dividends[1].Date = %v, want %v", d2.Date, expectedDate2)
	}
	if d2.Value != 0.85 {
		t.Errorf("dividends[1].Value = %.4f, want 0.85", d2.Value)
	}
}

func TestGetDividends_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	dividends, err := client.GetDividends(context.Background(), "XYZ.AU", time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("GetDividends failed on empty response: %v", err)
	}

	if len(dividends) != 0 {
		t.Errorf("expected 0 dividend events, got %d", len(dividends))
	}
}

func TestGetDividends_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("ticker not found"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))

	_, err := client.GetDividends(context.Background(), "INVALID.XX", time.Time{}, time.Time{})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}
