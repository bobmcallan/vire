package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// === tickerMatches stress tests ===
// These tests validate the tickerMatches() helper that will be added by Bug 1 fix.
// They will compile only after the implementation lands.

func TestTickerMatches_ExactMatch(t *testing.T) {
	assert := func(expected bool, got bool, msg string) {
		t.Helper()
		if got != expected {
			t.Errorf("%s: expected %v, got %v", msg, expected, got)
		}
	}

	assert(true, tickerMatches("BHP.AU", "BHP.AU"), "exact match with suffix")
	assert(true, tickerMatches("BHP", "BHP"), "exact match no suffix")
}

func TestTickerMatches_StrippedSuffix(t *testing.T) {
	// EODHD sometimes strips the .AU suffix and returns just "ACDC"
	if !tickerMatches("ACDC.AU", "ACDC") {
		t.Error("ACDC.AU should match ACDC (suffix stripped by API)")
	}
}

func TestTickerMatches_WrongExchange(t *testing.T) {
	// ACDC.AU should NOT match ACDC.US (different exchange)
	if tickerMatches("ACDC.AU", "ACDC.US") {
		t.Error("ACDC.AU should NOT match ACDC.US")
	}
}

func TestTickerMatches_CaseInsensitive(t *testing.T) {
	if !tickerMatches("BHP.au", "BHP.AU") {
		t.Error("exchange suffix comparison should be case-insensitive")
	}
	if !tickerMatches("bhp.AU", "BHP.AU") {
		// Note: base ticker might need case-insensitive too?
		// If not, this documents the expected behavior
		t.Log("Base ticker comparison is case-sensitive (may want to change)")
	}
}

func TestTickerMatches_MultipleDots(t *testing.T) {
	// Edge case: ticker with multiple dots like "BHP.AU.extra"
	// Should the base be "BHP" and exchange "AU"? Or "BHP.AU" and exchange "extra"?
	// This depends on implementation. Test what should NOT happen.
	if tickerMatches("BHP.AU.extra", "BHP.US") {
		t.Error("multi-dot ticker should not match different exchange")
	}
}

func TestTickerMatches_EmptyTicker(t *testing.T) {
	// Edge case: empty requested ticker
	if tickerMatches("", "BHP.AU") {
		t.Error("empty requested ticker should not match")
	}
}

func TestTickerMatches_EmptyReturned(t *testing.T) {
	// When API returns empty code, the validation should be skipped
	// (handled by the caller checking resp.Code != ""), but tickerMatches itself:
	if tickerMatches("BHP.AU", "") {
		t.Error("empty returned ticker should not match")
	}
}

func TestTickerMatches_Unicode(t *testing.T) {
	// Adversarial: unicode characters in ticker
	if tickerMatches("BHP\u0000.AU", "BHP.AU") {
		t.Error("null byte in ticker should not match clean ticker")
	}
}

func TestTickerMatches_DotOnly(t *testing.T) {
	// Edge case: ticker is just a dot
	result := tickerMatches(".", ".")
	_ = result // just verify no panic
}

func TestTickerMatches_ForexTicker(t *testing.T) {
	// FOREX tickers have different structure
	if !tickerMatches("XAGUSD.FOREX", "XAGUSD.FOREX") {
		t.Error("FOREX ticker should match exactly")
	}
}

// === GetRealTimeQuote ticker mismatch stress tests ===
// These test the actual validation behavior after Bug 1 is fixed.

func TestGetRealTimeQuote_TickerStrippedSuffix_Accepted(t *testing.T) {
	// EODHD returns ACDC (suffix stripped) when we asked for ACDC.AU.
	// Per requirements: "ACDC.AU" matches "ACDC" (no conflicting exchange suffix).
	// The real mismatch case is when EODHD returns ACDC.US for ACDC.AU.
	mockResp := map[string]interface{}{
		"code":      "ACDC",
		"timestamp": int64(1711670340),
		"open":      5.0,
		"high":      5.1,
		"low":       4.9,
		"close":     5.02,
		"volume":    float64(100000),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "ACDC.AU")

	// Suffix-stripped response is accepted (base ticker matches)
	if err != nil {
		t.Fatalf("expected no error for suffix-stripped match, got: %v", err)
	}
	if quote.Close != 5.02 {
		t.Errorf("expected close 5.02, got %.2f", quote.Close)
	}
}

func TestGetRealTimeQuote_TickerMatch_Succeeds(t *testing.T) {
	mockResp := map[string]interface{}{
		"code":      "BHP.AU",
		"timestamp": int64(1711670340),
		"open":      42.0,
		"high":      43.0,
		"low":       41.0,
		"close":     42.5,
		"volume":    float64(5000000),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("expected no error for matching ticker, got: %v", err)
	}
	if quote.Code != "BHP.AU" {
		t.Errorf("expected code BHP.AU, got %s", quote.Code)
	}
}

func TestGetRealTimeQuote_EmptyCodeField_NoValidation(t *testing.T) {
	// When EODHD returns empty Code, we skip validation (as per requirements)
	mockResp := map[string]interface{}{
		"code":      "",
		"timestamp": int64(1711670340),
		"open":      42.0,
		"high":      43.0,
		"low":       41.0,
		"close":     42.5,
		"volume":    float64(5000000),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	quote, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err != nil {
		t.Fatalf("empty code should skip validation, got error: %v", err)
	}
	if quote == nil {
		t.Fatal("expected non-nil quote")
	}
}

func TestGetRealTimeQuote_WrongExchange_ReturnsError(t *testing.T) {
	// API returns ACDC.US when we asked for ACDC.AU
	mockResp := map[string]interface{}{
		"code":      "ACDC.US",
		"timestamp": int64(1711670340),
		"open":      5.0,
		"high":      5.1,
		"low":       4.9,
		"close":     5.02,
		"volume":    float64(100000),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetRealTimeQuote(context.Background(), "ACDC.AU")
	if err == nil {
		t.Fatal("expected error when API returns ACDC.US but we requested ACDC.AU")
	}
}

func TestGetRealTimeQuote_SameBaseDifferentExchange(t *testing.T) {
	// API returns BHP.US (hypothetical) when we asked for BHP.AU
	mockResp := map[string]interface{}{
		"code":      "BHP.US",
		"timestamp": int64(1711670340),
		"open":      50.0,
		"high":      51.0,
		"low":       49.0,
		"close":     50.5,
		"volume":    float64(1000000),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetRealTimeQuote(context.Background(), "BHP.AU")
	if err == nil {
		t.Fatal("expected error for BHP.AU requested, BHP.US returned")
	}
}
