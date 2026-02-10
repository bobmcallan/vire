package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bobmccarthy/vire/internal/app"
)

// testServer creates an httptest.Server with the full vire-server mux for testing.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := newServerMux(t)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// newServerMux creates the HTTP mux the same way main() does, using a test App.
// This function mirrors the server setup in main.go.
func newServerMux(t *testing.T) http.Handler {
	t.Helper()
	configPath := writeTestConfig(t)
	a, err := app.NewApp(configPath)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return buildMux(a)
}

// TestHealthEndpoint verifies GET /api/health returns 200 with {"status":"ok"}.
func TestHealthEndpoint(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatalf("GET /api/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("Expected status=ok, got %q", body["status"])
	}
}

// TestVersionEndpoint verifies GET /api/version returns version info.
func TestVersionEndpoint(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/version")
	if err != nil {
		t.Fatalf("GET /api/version failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["version"] == "" {
		t.Error("Expected non-empty version field")
	}
}

// TestMCPEndpoint_AcceptsPOST verifies that POST /mcp is handled (not 404).
func TestMCPEndpoint_AcceptsPOST(t *testing.T) {
	ts := testServer(t)

	// Send a minimal JSON-RPC initialize request
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(initReq))
	if err != nil {
		t.Fatalf("POST /mcp failed: %v", err)
	}
	defer resp.Body.Close()

	// StreamableHTTPServer should handle it — we expect 200, not 404/405
	if resp.StatusCode == http.StatusNotFound {
		t.Error("POST /mcp returned 404 — MCP endpoint not mounted")
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		t.Error("POST /mcp returned 405 — MCP endpoint not accepting POST")
	}

	// Read body to verify it's valid JSON-RPC
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if len(body) == 0 {
		t.Error("Empty response body from POST /mcp")
	}

	// Should contain JSON-RPC response with server info
	if !strings.Contains(string(body), "serverInfo") && !strings.Contains(string(body), "jsonrpc") {
		t.Errorf("Response doesn't look like JSON-RPC: %s", string(body)[:min(len(body), 200)])
	}
}

// TestMCPEndpoint_GETReturnsSSEOrMethodNotAllowed verifies GET /mcp behavior.
// StreamableHTTPServer either opens an SSE stream or returns 405 if streaming is disabled.
func TestMCPEndpoint_GETReturnsSSEOrMethodNotAllowed(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("GET /mcp failed: %v", err)
	}
	defer resp.Body.Close()

	// Should not be 404 — the endpoint is mounted
	if resp.StatusCode == http.StatusNotFound {
		t.Error("GET /mcp returned 404 — MCP endpoint not mounted")
	}
}

// TestHealthEndpoint_MethodNotAllowed verifies POST to health returns 405.
func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Post(ts.URL+"/api/health", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("POST /api/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 for POST /api/health, got %d", resp.StatusCode)
	}
}

// TestMCPEndpoint_ToolCallE2E tests a real JSON-RPC tools/call for get_version
// through the HTTP MCP endpoint using the real StreamableHTTPServer.
func TestMCPEndpoint_ToolCallE2E(t *testing.T) {
	ts := testServer(t)

	// Step 1: Initialize the MCP session
	initReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`

	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(initReq))
	if err != nil {
		t.Fatalf("POST /mcp initialize failed: %v", err)
	}
	sessionID := resp.Header.Get("Mcp-Session-Id")
	resp.Body.Close()

	// Step 2: Send initialized notification
	notif := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	req, _ := http.NewRequest("POST", ts.URL+"/mcp", strings.NewReader(notif))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp initialized notification failed: %v", err)
	}
	resp.Body.Close()

	// Step 3: Call get_version tool
	toolCall := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_version","arguments":{}}}`
	req, _ = http.NewRequest("POST", ts.URL+"/mcp", strings.NewReader(toolCall))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}

	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp tools/call failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	// Parse JSON-RPC response
	var rpcResp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		t.Fatalf("Failed to parse JSON-RPC response: %v\nBody: %s", err, string(body))
	}

	if rpcResp.JSONRPC != "2.0" {
		t.Errorf("Expected jsonrpc=2.0, got %q", rpcResp.JSONRPC)
	}
	if rpcResp.ID != 2 {
		t.Errorf("Expected id=2, got %d", rpcResp.ID)
	}

	// The result should contain version text
	resultStr := string(rpcResp.Result)
	if !strings.Contains(resultStr, "Vire MCP Server") && !strings.Contains(resultStr, "content") {
		t.Errorf("Expected version info in result, got: %s", resultStr[:min(len(resultStr), 200)])
	}
}

// --- test helpers ---

func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "data"), 0755)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)

	config := `
[storage.file]
path = "` + filepath.Join(dir, "data") + `"
versions = 2

[logging]
level = "error"
output = ["console"]
file_path = "` + filepath.Join(dir, "logs", "vire.log") + `"
`
	configPath := filepath.Join(dir, "vire.toml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	return configPath
}
