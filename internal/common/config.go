// Package common provides shared utilities for Vire
package common

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/interfaces"
	toml "github.com/pelletier/go-toml/v2"
)

// Config holds all configuration for Vire
type Config struct {
	Environment string        `toml:"environment"`
	Portfolios  []string      `toml:"portfolios"`
	Server      ServerConfig  `toml:"server"`
	Storage     StorageConfig `toml:"storage"`
	Clients     ClientsConfig `toml:"clients"`
	Logging     LoggingConfig `toml:"logging"`
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

// DefaultPortfolio returns the first portfolio in the list (the default), or empty string.
func (c *Config) DefaultPortfolio() string {
	if len(c.Portfolios) > 0 {
		return c.Portfolios[0]
	}
	return ""
}

// StorageConfig holds storage configuration.
// Backend can be "file" (default), "gcs", or "s3".
type StorageConfig struct {
	Backend string     `toml:"backend"` // "file", "gcs", "s3" (default: "file")
	File    FileConfig `toml:"file"`
	GCS     GCSConfig  `toml:"gcs"`
	S3      S3Config   `toml:"s3"`
}

// FileConfig holds file-based storage configuration
type FileConfig struct {
	Path     string `toml:"path"`
	Versions int    `toml:"versions"`
}

// GCSConfig holds Google Cloud Storage configuration (future Phase 2)
type GCSConfig struct {
	Bucket          string `toml:"bucket"`
	Prefix          string `toml:"prefix"`           // Optional key prefix within bucket
	CredentialsFile string `toml:"credentials_file"` // Path to service account JSON (optional if using ADC)
}

// S3Config holds AWS S3 configuration (future Phase 2)
type S3Config struct {
	Bucket    string `toml:"bucket"`
	Prefix    string `toml:"prefix"`   // Optional key prefix within bucket
	Region    string `toml:"region"`   // AWS region (e.g., "us-east-1")
	Endpoint  string `toml:"endpoint"` // Custom endpoint for S3-compatible stores (MinIO, R2)
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
}

// ClientsConfig holds API client configurations
type ClientsConfig struct {
	EODHD  EODHDConfig  `toml:"eodhd"`
	Navexa NavexaConfig `toml:"navexa"`
	Gemini GeminiConfig `toml:"gemini"`
}

// EODHDConfig holds EODHD API configuration
type EODHDConfig struct {
	BaseURL   string `toml:"base_url"`
	APIKey    string `toml:"api_key"`
	RateLimit int    `toml:"rate_limit"`
	Timeout   string `toml:"timeout"`
}

// GetTimeout parses and returns the timeout duration
func (c *EODHDConfig) GetTimeout() time.Duration {
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// NavexaConfig holds Navexa API configuration
type NavexaConfig struct {
	BaseURL   string `toml:"base_url"`
	APIKey    string `toml:"api_key"`
	RateLimit int    `toml:"rate_limit"`
	Timeout   string `toml:"timeout"`
}

// GetTimeout parses and returns the timeout duration
func (c *NavexaConfig) GetTimeout() time.Duration {
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GeminiConfig holds Gemini API configuration
type GeminiConfig struct {
	APIKey         string `toml:"api_key"`
	Model          string `toml:"model"`
	MaxURLs        int    `toml:"max_urls"`
	MaxContentSize string `toml:"max_content_size"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string   `toml:"level"`
	Format     string   `toml:"format"`
	Outputs    []string `toml:"outputs"`
	FilePath   string   `toml:"file_path"`
	MaxSizeMB  int      `toml:"max_size_mb"`
	MaxBackups int      `toml:"max_backups"`
}

// NewDefaultConfig returns a Config with sensible defaults
func NewDefaultConfig() *Config {
	return &Config{
		Environment: "development",
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 4242,
		},
		Storage: StorageConfig{
			Backend: "file", // Default to file-based storage
			File: FileConfig{
				Path:     "data",
				Versions: 5,
			},
		},
		Clients: ClientsConfig{
			EODHD: EODHDConfig{
				BaseURL:   "https://eodhd.com/api",
				RateLimit: 10,
				Timeout:   "30s",
			},
			Navexa: NavexaConfig{
				BaseURL:   "https://api.navexa.com.au",
				RateLimit: 5,
				Timeout:   "30s",
			},
			Gemini: GeminiConfig{
				Model:          "gemini-2.0-flash",
				MaxURLs:        20,
				MaxContentSize: "34MB",
			},
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Outputs:    []string{"console", "file"},
			FilePath:   "./logs/vire.log",
			MaxSizeMB:  100,
			MaxBackups: 3,
		},
	}
}

// LoadConfig loads configuration from files with environment overrides
func LoadConfig(paths ...string) (*Config, error) {
	config := NewDefaultConfig()

	// Load and merge each config file in order (later files override earlier)
	for _, path := range paths {
		if path == "" {
			continue
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue // Skip missing files
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
		}

		if err := toml.Unmarshal(data, config); err != nil {
			return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
		}
	}

	// Apply environment overrides
	applyEnvOverrides(config)

	return config, nil
}

// applyEnvOverrides applies environment variable overrides to config
func applyEnvOverrides(config *Config) {
	if env := os.Getenv("VIRE_ENV"); env != "" {
		config.Environment = env
	}

	if host := os.Getenv("VIRE_HOST"); host != "" {
		config.Server.Host = host
	}

	if port := os.Getenv("VIRE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			config.Server.Port = p
		}
	}

	if level := os.Getenv("VIRE_LOG_LEVEL"); level != "" {
		config.Logging.Level = level
	}

	if path := os.Getenv("VIRE_DATA_PATH"); path != "" {
		config.Storage.File.Path = path
	}

	if dp := os.Getenv("VIRE_DEFAULT_PORTFOLIO"); dp != "" {
		// Set as first portfolio (default), preserving any others
		if len(config.Portfolios) == 0 {
			config.Portfolios = []string{dp}
		} else if config.Portfolios[0] != dp {
			// Remove dp if it exists elsewhere, then prepend
			filtered := []string{dp}
			for _, p := range config.Portfolios {
				if p != dp {
					filtered = append(filtered, p)
				}
			}
			config.Portfolios = filtered
		}
	}
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	return env == "production" || env == "prod"
}

// ResolveDefaultPortfolio resolves the default portfolio name.
// Priority: KV store (runtime) > VIRE_DEFAULT_PORTFOLIO env > first entry in config portfolios list > empty string.
func ResolveDefaultPortfolio(ctx context.Context, kvStorage interfaces.KeyValueStorage, configDefault string) string {
	// KV store (highest priority — set at runtime via set_default_portfolio tool)
	if kvStorage != nil {
		if val, err := kvStorage.Get(ctx, "default_portfolio"); err == nil && val != "" {
			return val
		}
	}

	// Environment variable
	if val := os.Getenv("VIRE_DEFAULT_PORTFOLIO"); val != "" {
		return val
	}

	// Config file fallback (first entry in portfolios list)
	return configDefault
}

// ResolveAPIKey resolves an API key from environment, KV store, or fallback
func ResolveAPIKey(ctx context.Context, kvStorage interfaces.KeyValueStorage, name string, fallback string) (string, error) {
	// Environment variable mapping
	keyToEnvMapping := map[string][]string{
		"eodhd_api_key":  {"EODHD_API_KEY", "VIRE_EODHD_API_KEY"},
		"navexa_api_key": {"NAVEXA_API_KEY", "VIRE_NAVEXA_API_KEY"},
		"gemini_api_key": {"GEMINI_API_KEY", "VIRE_GEMINI_API_KEY", "GOOGLE_API_KEY"},
	}

	// Check environment variables first (highest priority)
	if envVarNames, ok := keyToEnvMapping[name]; ok {
		for _, envVarName := range envVarNames {
			if envValue := os.Getenv(envVarName); envValue != "" {
				return envValue, nil
			}
		}
	}

	// Try KV store (medium priority)
	if kvStorage != nil {
		apiKey, err := kvStorage.Get(ctx, name)
		if err == nil && apiKey != "" {
			return apiKey, nil
		}
	}

	// Fallback (lowest priority)
	if fallback != "" {
		return fallback, nil
	}

	return "", fmt.Errorf("API key '%s' not found in environment or KV store", name)
}
