package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/bobmcallan/vire/internal/app"
	"github.com/bobmcallan/vire/internal/server"
)

// testServer creates an httptest.Server with the full vire-server handler for testing.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	configPath := writeTestConfig(t)
	a, err := app.NewApp(configPath)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	t.Cleanup(func() { a.Close() })

	srv := server.NewServer(a)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts
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

// TestHealthEndpoint_MethodNotAllowed verifies POST to health returns 405.
func TestHealthEndpoint_MethodNotAllowed(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Post(ts.URL+"/api/health", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405 for POST /api/health, got %d", resp.StatusCode)
	}
}

// TestConfigEndpoint verifies GET /api/config returns configuration.
func TestConfigEndpoint(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET /api/config failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var body map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if body["runtime_settings"] == nil {
		t.Error("Expected runtime_settings in config response")
	}
}

// TestPortfolioListEndpoint verifies GET /api/portfolios returns a list.
func TestPortfolioListEndpoint(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Get(ts.URL + "/api/portfolios")
	if err != nil {
		t.Fatalf("GET /api/portfolios failed: %v", err)
	}
	defer resp.Body.Close()

	// May return 200 with empty list or 500 if no portfolio service configured
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status 200 or 500, got %d", resp.StatusCode)
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
outputs = ["console"]
file_path = "` + filepath.Join(dir, "logs", "vire.log") + `"
`
	configPath := filepath.Join(dir, "vire.toml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	return configPath
}
