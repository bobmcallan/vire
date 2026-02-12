package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_DisplayCurrencyDefault(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg.DisplayCurrency != "AUD" {
		t.Errorf("DisplayCurrency default = %q, want %q", cfg.DisplayCurrency, "AUD")
	}
}

func TestConfig_LoadDisplayCurrencyFromTOML(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "vire.toml")
	if err := os.WriteFile(tomlPath, []byte(`display_currency = "USD"`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tomlPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.DisplayCurrency != "USD" {
		t.Errorf("DisplayCurrency = %q, want %q", cfg.DisplayCurrency, "USD")
	}
}

func TestConfig_DisplayCurrencyEnvOverride(t *testing.T) {
	t.Setenv("VIRE_DISPLAY_CURRENCY", "USD")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.DisplayCurrency != "USD" {
		t.Errorf("DisplayCurrency = %q after env override, want %q", cfg.DisplayCurrency, "USD")
	}
}

func TestConfig_DisplayCurrencyOnlyAUDOrUSD(t *testing.T) {
	dir := t.TempDir()
	tomlPath := filepath.Join(dir, "vire.toml")
	// Invalid currency should fall back to AUD
	if err := os.WriteFile(tomlPath, []byte(`display_currency = "EUR"`), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(tomlPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.DisplayCurrency != "AUD" {
		t.Errorf("DisplayCurrency = %q for invalid currency, want %q (should default to AUD)", cfg.DisplayCurrency, "AUD")
	}
}
