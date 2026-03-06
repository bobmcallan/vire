package eodhd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ============================================================================
// 1. GetBulkRealTimeQuotes -- empty ticker list returns empty map
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_EmptyTickers(t *testing.T) {
	apiCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), nil)
	if err != nil {
		t.Fatalf("empty tickers should not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
	if apiCalled {
		t.Error("API should not be called for empty ticker list")
	}
}

// ============================================================================
// 2. GetBulkRealTimeQuotes -- single ticker returns object (not array)
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_SingleTicker(t *testing.T) {
	mockResp := map[string]interface{}{
		"code":      "BHP.AU",
		"timestamp": int64(1711670340),
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
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU"})
	if err != nil {
		t.Fatalf("single ticker should work: %v", err)
	}

	if capturedPath != "/real-time/BHP.AU" {
		t.Errorf("expected path /real-time/BHP.AU, got %s", capturedPath)
	}
	q, ok := result["BHP.AU"]
	if !ok {
		t.Fatal("expected BHP.AU in result map")
	}
	if q.Close != 43.25 {
		t.Errorf("expected close 43.25, got %.2f", q.Close)
	}
}

// ============================================================================
// 3. GetBulkRealTimeQuotes -- multiple tickers returns array
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_MultipleTickers(t *testing.T) {
	mockResp := []map[string]interface{}{
		{"code": "BHP.AU", "timestamp": int64(1711670340), "open": 42.0, "high": 43.0, "low": 41.0, "close": 42.5, "volume": float64(5000000)},
		{"code": "RIO.AU", "timestamp": int64(1711670340), "open": 80.0, "high": 82.0, "low": 79.0, "close": 81.0, "volume": float64(3000000)},
		{"code": "CBA.AU", "timestamp": int64(1711670340), "open": 110.0, "high": 112.0, "low": 109.0, "close": 111.0, "volume": float64(2000000)},
	}

	var capturedS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedS = r.URL.Query().Get("s")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "RIO.AU", "CBA.AU"})
	if err != nil {
		t.Fatalf("multiple tickers should work: %v", err)
	}

	if capturedS != "RIO.AU,CBA.AU" {
		t.Errorf("expected s=RIO.AU,CBA.AU, got %s", capturedS)
	}
	if len(result) != 3 {
		t.Errorf("expected 3 results, got %d", len(result))
	}
	if result["RIO.AU"].Close != 81.0 {
		t.Errorf("RIO close = %.2f, want 81.0", result["RIO.AU"].Close)
	}
}

// ============================================================================
// 4. GetBulkRealTimeQuotes -- partial response (some tickers missing)
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_PartialResponse(t *testing.T) {
	// EODHD returns data for only 2 of 3 requested tickers
	mockResp := []map[string]interface{}{
		{"code": "BHP.AU", "timestamp": int64(1711670340), "open": 42.0, "high": 43.0, "low": 41.0, "close": 42.5, "volume": float64(5000000)},
		{"code": "RIO.AU", "timestamp": int64(1711670340), "open": 80.0, "high": 82.0, "low": 79.0, "close": 81.0, "volume": float64(3000000)},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "RIO.AU", "MISSING.AU"})
	if err != nil {
		t.Fatalf("partial response should not error: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 results (missing ticker silently skipped), got %d", len(result))
	}
	if _, ok := result["MISSING.AU"]; ok {
		t.Error("MISSING.AU should not be in results")
	}
}

// ============================================================================
// 5. GetBulkRealTimeQuotes -- response contains empty code (should be skipped)
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_EmptyCode(t *testing.T) {
	mockResp := []map[string]interface{}{
		{"code": "BHP.AU", "timestamp": int64(1711670340), "close": 42.5},
		{"code": "", "timestamp": int64(0), "close": 0}, // empty code
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "GHOST.AU"})
	if err != nil {
		t.Fatalf("empty code should be silently skipped: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 result (empty code skipped), got %d", len(result))
	}
}

// ============================================================================
// 6. GetBulkRealTimeQuotes -- API error (500)
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "RIO.AU"})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

// ============================================================================
// 7. GetBulkRealTimeQuotes -- timeout
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL), WithTimeout(100*time.Millisecond))
	_, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ============================================================================
// 8. GetBulkRealTimeQuotes -- string values in response (flex types)
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_StringValues(t *testing.T) {
	// EODHD sometimes returns numbers as strings
	mockResp := []map[string]interface{}{
		{
			"code":      "BHP.AU",
			"timestamp": "1711670340",
			"open":      "42.10",
			"high":      "43.50",
			"low":       "41.80",
			"close":     "43.25",
			"volume":    "5000000",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "EXTRA.AU"})
	if err != nil {
		t.Fatalf("string values should be handled by flex types: %v", err)
	}

	q, ok := result["BHP.AU"]
	if !ok {
		t.Fatal("BHP.AU should be in results")
	}
	if q.Close != 43.25 {
		t.Errorf("expected close 43.25, got %.2f", q.Close)
	}
	if q.Volume != 5000000 {
		t.Errorf("expected volume 5000000, got %d", q.Volume)
	}
}

// ============================================================================
// 9. GetBulkRealTimeQuotes -- context cancellation
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkRealTimeQuotes(ctx, []string{"BHP.AU"})
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// ============================================================================
// 10. GetBulkRealTimeQuotes -- malformed JSON response
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"code":"BHP.AU", "close": INVALID}]`))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "RIO.AU"})
	if err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

// ============================================================================
// 11. GetBulkRealTimeQuotes -- rate limiter only counts as 1 HTTP request
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_SingleRateLimiterHit(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		mockResp := []map[string]interface{}{
			{"code": "BHP.AU", "close": 42.5},
			{"code": "RIO.AU", "close": 81.0},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "RIO.AU"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("expected 1 HTTP request (rate limiter counts as 1), got %d", requestCount)
	}
}

// ============================================================================
// 12. GetBulkRealTimeQuotes -- N/A values in response (flex types handle gracefully)
// ============================================================================

func TestStress_GetBulkRealTimeQuotes_NAValues(t *testing.T) {
	mockResp := []map[string]interface{}{
		{
			"code":      "BHP.AU",
			"timestamp": "N/A",
			"open":      "N/A",
			"high":      "N/A",
			"low":       "N/A",
			"close":     "N/A",
			"volume":    "N/A",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkRealTimeQuotes(context.Background(), []string{"BHP.AU", "RIO.AU"})
	if err != nil {
		t.Fatalf("N/A values should be handled gracefully: %v", err)
	}

	q := result["BHP.AU"]
	if q == nil {
		t.Fatal("BHP.AU should still be in results with N/A values")
	}
	// N/A should parse to 0
	if q.Close != 0 {
		t.Errorf("N/A close should be 0, got %.2f", q.Close)
	}
}
