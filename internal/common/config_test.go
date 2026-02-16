package common

import (
	"testing"
)

func TestConfig_DefaultPort(t *testing.T) {
	cfg := NewDefaultConfig()
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port default = %d, want %d", cfg.Server.Port, 8080)
	}
}

func TestConfig_PortEnvOverride(t *testing.T) {
	t.Setenv("VIRE_PORT", "9090")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d after env override, want %d", cfg.Server.Port, 9090)
	}
}
