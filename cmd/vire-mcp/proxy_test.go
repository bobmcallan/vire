package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPProxy_Get_Success(t *testing.T) {
	expected := map[string]string{"status": "ok"}
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/health" {
			t.Errorf("Expected /api/health, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	body, err := proxy.get("/api/health")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("Expected status=ok, got %s", result["status"])
	}
}

func TestMCPProxy_Get_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "portfolio not found"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	_, err := proxy.get("/api/portfolios/nonexistent")
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}
	if err.Error() != "portfolio not found" {
		t.Errorf("Expected 'portfolio not found', got %q", err.Error())
	}
}

func TestMCPProxy_Get_ServerUnavailable(t *testing.T) {
	proxy := NewMCPProxy("http://localhost:1")
	_, err := proxy.get("/api/health")
	if err == nil {
		t.Fatal("Expected error when server is unavailable")
	}
}

func TestMCPProxy_Post_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		// Read and verify request body
		body, _ := io.ReadAll(r.Body)
		var req map[string]interface{}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("Request body is not valid JSON: %v", err)
		}
		if req["ticker"] != "AAPL" {
			t.Errorf("Expected ticker=AAPL, got %v", req["ticker"])
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "created"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	body, err := proxy.post("/api/portfolios/test/plan/items", map[string]string{"ticker": "AAPL"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var result map[string]string
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	if result["result"] != "created" {
		t.Errorf("Expected result=created, got %s", result["result"])
	}
}

func TestMCPProxy_Post_NilBody(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	body, err := proxy.post("/api/portfolios/test/sync", nil)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("Expected non-empty response body")
	}
}

func TestMCPProxy_Post_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid ticker"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	_, err := proxy.post("/api/portfolios/test/plan/items", map[string]string{"ticker": ""})
	if err == nil {
		t.Fatal("Expected error for 400 response")
	}
	if err.Error() != "invalid ticker" {
		t.Errorf("Expected 'invalid ticker', got %q", err.Error())
	}
}

func TestMCPProxy_Put_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "updated"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	body, err := proxy.put("/api/portfolios/test/strategy", map[string]string{"name": "growth"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["result"] != "updated" {
		t.Errorf("Expected result=updated, got %s", result["result"])
	}
}

func TestMCPProxy_Patch_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Expected PATCH, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "patched"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	body, err := proxy.patch("/api/portfolios/test/plan/items/1", map[string]string{"status": "done"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["result"] != "patched" {
		t.Errorf("Expected result=patched, got %s", result["result"])
	}
}

func TestMCPProxy_Del_Success(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/api/portfolios/test/plan/items/abc" {
			t.Errorf("Expected path /api/portfolios/test/plan/items/abc, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"result": "deleted"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	body, err := proxy.del("/api/portfolios/test/plan/items/abc")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	var result map[string]string
	json.Unmarshal(body, &result)
	if result["result"] != "deleted" {
		t.Errorf("Expected result=deleted, got %s", result["result"])
	}
}

func TestMCPProxy_Del_ServerError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "item not found"})
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	_, err := proxy.del("/api/portfolios/test/plan/items/missing")
	if err == nil {
		t.Fatal("Expected error for 404 response")
	}
	if err.Error() != "item not found" {
		t.Errorf("Expected 'item not found', got %q", err.Error())
	}
}

func TestMCPProxy_Get_NonJSONError(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer mockServer.Close()

	proxy := NewMCPProxy(mockServer.URL)
	_, err := proxy.get("/api/health")
	if err == nil {
		t.Fatal("Expected error for 500 response")
	}
	// When the error body is not JSON, it should include the status code and raw body
	expected := "server returned 500: internal server error"
	if err.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, err.Error())
	}
}

func TestMCPProxy_NewMCPProxy(t *testing.T) {
	proxy := NewMCPProxy("http://example.com:4242")
	if proxy.serverURL != "http://example.com:4242" {
		t.Errorf("Expected serverURL=http://example.com:4242, got %s", proxy.serverURL)
	}
	if proxy.httpClient == nil {
		t.Error("Expected non-nil httpClient")
	}
	if proxy.httpClient.Timeout.Seconds() != 300 {
		t.Errorf("Expected 300s timeout, got %v", proxy.httpClient.Timeout)
	}
}
