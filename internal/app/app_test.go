package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNewApp_InitializesAllServices verifies that NewApp creates an App with
// all services initialized and non-nil.
func TestNewApp_InitializesAllServices(t *testing.T) {
	configPath := writeTestConfig(t)

	a, err := NewApp(configPath)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	defer a.Close()

	if a.Config == nil {
		t.Error("Config is nil")
	}
	if a.Logger == nil {
		t.Error("Logger is nil")
	}
	if a.Storage == nil {
		t.Error("Storage is nil")
	}
	if a.PortfolioService == nil {
		t.Error("PortfolioService is nil")
	}
	if a.MarketService == nil {
		t.Error("MarketService is nil")
	}
	if a.SignalService == nil {
		t.Error("SignalService is nil")
	}
	if a.ReportService == nil {
		t.Error("ReportService is nil")
	}
	if a.StrategyService == nil {
		t.Error("StrategyService is nil")
	}
	if a.PlanService == nil {
		t.Error("PlanService is nil")
	}
	if a.WatchlistService == nil {
		t.Error("WatchlistService is nil")
	}
	if a.StartupTime.IsZero() {
		t.Error("StartupTime is zero")
	}
}

// TestNewApp_CloseIsIdempotent verifies that calling Close multiple times
// does not panic.
func TestNewApp_CloseIsIdempotent(t *testing.T) {
	configPath := writeTestConfig(t)

	a, err := NewApp(configPath)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}

	// Close twice — should not panic
	a.Close()
	a.Close()
}

// TestNewApp_InvalidConfigReturnsError verifies that an invalid config file
// returns a meaningful error.
func TestNewApp_InvalidConfigReturnsError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "bad.toml")
	os.WriteFile(configPath, []byte("{{{{invalid toml"), 0644)

	_, err := NewApp(configPath)
	if err == nil {
		t.Fatal("Expected error for invalid config content, got nil")
	}
}

// --- test helpers ---

// writeTestConfig creates a minimal vire.toml in a temp directory for testing.
func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "data"), 0755)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)

	config := `
[storage.user_data]
path = "` + filepath.Join(dir, "data", "user") + `"
versions = 2

[storage.data]
path = "` + filepath.Join(dir, "data", "data") + `"
versions = 0

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
