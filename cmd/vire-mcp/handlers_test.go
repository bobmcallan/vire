package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleGetQuote_Success(t *testing.T) {
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

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "/api/market/quote/XAGUSD.FOREX") {
			t.Errorf("Expected path containing /api/market/quote/XAGUSD.FOREX, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(quote)
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL, testLogger())
	handler := handleGetQuote(proxy)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"ticker": "XAGUSD.FOREX",
	}

	result, err := handler(nil, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("Expected success, got error: %v", result.Content)
	}

	// Check result contains the ticker, price, and change%
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "XAGUSD.FOREX") {
		t.Error("Result should contain ticker")
	}
	if !strings.Contains(text, "31.25") {
		t.Error("Result should contain close price")
	}
	if !strings.Contains(text, "1.46%") {
		t.Error("Result should contain change percentage")
	}
}

func TestHandleGetQuote_MissingTicker(t *testing.T) {
	proxy := NewMCPProxy("http://localhost:1", testLogger())
	handler := handleGetQuote(proxy)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{}

	result, err := handler(nil, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error result for missing ticker")
	}
}

func TestHandleGetQuote_InvalidTickerChars(t *testing.T) {
	// Server should reject tickers with invalid characters (path traversal defense)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid character in ticker"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL, testLogger())
	handler := handleGetQuote(proxy)

	badTickers := []string{
		"../../../etc/passwd",
		"AAPL;DROP.US",
		"BHP/.AU",
	}

	for _, ticker := range badTickers {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = map[string]interface{}{
			"ticker": ticker,
		}

		result, err := handler(nil, request)
		if err != nil {
			t.Fatalf("Unexpected error for ticker %q: %v", ticker, err)
		}
		if !result.IsError {
			t.Errorf("Expected error result for invalid ticker %q", ticker)
		}
	}
}

func TestHandleGetQuote_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "EODHD client not configured"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL, testLogger())
	handler := handleGetQuote(proxy)

	request := mcp.CallToolRequest{}
	request.Params.Arguments = map[string]interface{}{
		"ticker": "XAGUSD.FOREX",
	}

	result, err := handler(nil, request)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("Expected error result for server error")
	}
}
