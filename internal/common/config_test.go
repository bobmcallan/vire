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

func TestConfig_ValidateRequired_AllMissing(t *testing.T) {
	cfg := &Config{
		Auth: AuthConfig{JWTSecret: "change-me-in-production"},
	}
	missing := cfg.ValidateRequired()
	if len(missing) != 7 {
		t.Errorf("expected 7 missing fields, got %d: %v", len(missing), missing)
	}
}

func TestConfig_ValidateRequired_AllPresent(t *testing.T) {
	cfg := &Config{
		Clients: ClientsConfig{
			EODHD:  EODHDConfig{APIKey: "eodhd-key"},
			Gemini: GeminiConfig{APIKey: "gemini-key"},
		},
		Auth: AuthConfig{
			JWTSecret: "real-secret-value",
			Google:    OAuthProvider{ClientID: "goog-id", ClientSecret: "goog-secret"},
			GitHub:    OAuthProvider{ClientID: "gh-id", ClientSecret: "gh-secret"},
		},
	}
	missing := cfg.ValidateRequired()
	if len(missing) != 0 {
		t.Errorf("expected 0 missing fields, got %d: %v", len(missing), missing)
	}
}

func TestConfig_ValidateRequired_JWTDefaultRejected(t *testing.T) {
	cfg := &Config{
		Clients: ClientsConfig{
			EODHD:  EODHDConfig{APIKey: "key"},
			Gemini: GeminiConfig{APIKey: "key"},
		},
		Auth: AuthConfig{
			JWTSecret: "change-me-in-production",
			Google:    OAuthProvider{ClientID: "id", ClientSecret: "secret"},
			GitHub:    OAuthProvider{ClientID: "id", ClientSecret: "secret"},
		},
	}
	missing := cfg.ValidateRequired()
	if len(missing) != 1 {
		t.Errorf("expected 1 missing field (jwt_secret), got %d: %v", len(missing), missing)
	}
}

func TestConfig_EODHDKeyEnvOverride(t *testing.T) {
	t.Setenv("EODHD_API_KEY", "from-env")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Clients.EODHD.APIKey != "from-env" {
		t.Errorf("EODHD.APIKey = %q, want %q", cfg.Clients.EODHD.APIKey, "from-env")
	}
}

func TestConfig_GeminiKeyEnvOverride(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "gem-from-env")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Clients.Gemini.APIKey != "gem-from-env" {
		t.Errorf("Gemini.APIKey = %q, want %q", cfg.Clients.Gemini.APIKey, "gem-from-env")
	}
}

func TestConfig_GeminiKeyGoogleEnvFallback(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "google-fallback")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Clients.Gemini.APIKey != "google-fallback" {
		t.Errorf("Gemini.APIKey = %q, want %q", cfg.Clients.Gemini.APIKey, "google-fallback")
	}
}

func TestConfig_AuthEnvOverrides(t *testing.T) {
	t.Setenv("VIRE_AUTH_JWT_SECRET", "secret-from-env")
	t.Setenv("VIRE_AUTH_GOOGLE_CLIENT_ID", "goog-id-env")
	t.Setenv("VIRE_AUTH_GOOGLE_CLIENT_SECRET", "goog-secret-env")
	t.Setenv("VIRE_AUTH_GITHUB_CLIENT_ID", "gh-id-env")
	t.Setenv("VIRE_AUTH_GITHUB_CLIENT_SECRET", "gh-secret-env")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	if cfg.Auth.JWTSecret != "secret-from-env" {
		t.Errorf("Auth.JWTSecret = %q, want %q", cfg.Auth.JWTSecret, "secret-from-env")
	}
	if cfg.Auth.Google.ClientID != "goog-id-env" {
		t.Errorf("Auth.Google.ClientID = %q, want %q", cfg.Auth.Google.ClientID, "goog-id-env")
	}
	if cfg.Auth.Google.ClientSecret != "goog-secret-env" {
		t.Errorf("Auth.Google.ClientSecret = %q, want %q", cfg.Auth.Google.ClientSecret, "goog-secret-env")
	}
	if cfg.Auth.GitHub.ClientID != "gh-id-env" {
		t.Errorf("Auth.GitHub.ClientID = %q, want %q", cfg.Auth.GitHub.ClientID, "gh-id-env")
	}
	if cfg.Auth.GitHub.ClientSecret != "gh-secret-env" {
		t.Errorf("Auth.GitHub.ClientSecret = %q, want %q", cfg.Auth.GitHub.ClientSecret, "gh-secret-env")
	}
}

func TestConfig_ValidateRequired_EnvOverridesFix(t *testing.T) {
	t.Setenv("EODHD_API_KEY", "eodhd-key")
	t.Setenv("GEMINI_API_KEY", "gemini-key")
	t.Setenv("VIRE_AUTH_JWT_SECRET", "real-secret")
	t.Setenv("VIRE_AUTH_GOOGLE_CLIENT_ID", "goog-id")
	t.Setenv("VIRE_AUTH_GOOGLE_CLIENT_SECRET", "goog-secret")
	t.Setenv("VIRE_AUTH_GITHUB_CLIENT_ID", "gh-id")
	t.Setenv("VIRE_AUTH_GITHUB_CLIENT_SECRET", "gh-secret")

	cfg := NewDefaultConfig()
	applyEnvOverrides(cfg)

	missing := cfg.ValidateRequired()
	if len(missing) != 0 {
		t.Errorf("expected 0 missing after env overrides, got %d: %v", len(missing), missing)
	}
}
