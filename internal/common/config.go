// Package common provides shared utilities for Vire
package common

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	toml "github.com/pelletier/go-toml/v2"
)

// Config holds all configuration for Vire
type Config struct {
	Environment string           `toml:"environment"`
	Server      ServerConfig     `toml:"server"`
	Storage     StorageConfig    `toml:"storage"`
	Clients     ClientsConfig    `toml:"clients"`
	Logging     LoggingConfig    `toml:"logging"`
	Auth        AuthConfig       `toml:"auth"`
	JobManager  JobManagerConfig `toml:"jobmanager"`
}

// JobManagerConfig holds configuration for the background job manager
type JobManagerConfig struct {
	Enabled             bool   `toml:"enabled"`
	Interval            string `toml:"interval"`              // Deprecated: use WatcherInterval
	WatcherInterval     string `toml:"watcher_interval"`      // How often to scan stock index (default "1m")
	MaxConcurrent       int    `toml:"max_concurrent"`        // Concurrent job processors (default 5)
	MaxRetries          int    `toml:"max_retries"`           // Max retry attempts per job (default 3)
	PurgeAfter          string `toml:"purge_after"`           // Purge completed jobs older than (default "24h")
	WatcherStartupDelay string `toml:"watcher_startup_delay"` // Delay before first scan (default "10s")
	HeavyJobLimit       int    `toml:"heavy_job_limit"`       // Max concurrent PDF-heavy jobs (default 1)
	FilingSizeThreshold int64  `toml:"filing_size_threshold"` // PDFs above this size (bytes) are processed one-at-a-time (default 5MB)
}

// GetInterval parses and returns the job manager interval duration (legacy, prefer GetWatcherInterval)
func (c *JobManagerConfig) GetInterval() time.Duration {
	return c.GetWatcherInterval()
}

// GetWatcherInterval parses and returns the watcher interval duration.
func (c *JobManagerConfig) GetWatcherInterval() time.Duration {
	interval := c.WatcherInterval
	if interval == "" {
		interval = c.Interval // backward compat
	}
	if interval == "" {
		return 1 * time.Minute
	}
	d, err := time.ParseDuration(interval)
	if err != nil {
		return 1 * time.Minute
	}
	return d
}

// GetMaxRetries returns the max retry attempts per job.
func (c *JobManagerConfig) GetMaxRetries() int {
	if c.MaxRetries <= 0 {
		return 3
	}
	return c.MaxRetries
}

// GetPurgeAfter returns the duration after which completed jobs are purged.
func (c *JobManagerConfig) GetPurgeAfter() time.Duration {
	if c.PurgeAfter == "" {
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(c.PurgeAfter)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

// GetWatcherStartupDelay returns the delay before the first watcher scan.
func (c *JobManagerConfig) GetWatcherStartupDelay() time.Duration {
	s := c.WatcherStartupDelay
	if s == "" {
		if v := os.Getenv("VIRE_WATCHER_STARTUP_DELAY"); v != "" {
			s = v
		}
	}
	if s == "" {
		return 10 * time.Second
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 10 * time.Second
	}
	return d
}

// GetHeavyJobLimit returns the max concurrent PDF-heavy jobs.
func (c *JobManagerConfig) GetHeavyJobLimit() int {
	if c.HeavyJobLimit <= 0 {
		return 1
	}
	return c.HeavyJobLimit
}

// GetFilingSizeThreshold returns the filing size threshold in bytes.
// PDFs larger than this are processed one at a time. Default: 5MB.
func (c *JobManagerConfig) GetFilingSizeThreshold() int64 {
	if c.FilingSizeThreshold <= 0 {
		return 5 * 1024 * 1024 // 5MB
	}
	return c.FilingSizeThreshold
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
}

// StorageConfig holds storage configuration for SurrealDB.
type StorageConfig struct {
	Address   string `toml:"address"`
	Namespace string `toml:"namespace"`
	Database  string `toml:"database"`
	Username  string `toml:"username"`
	Password  string `toml:"password"`
	DataPath  string `toml:"data_path"` // for generated files (charts)
}

// FileConfig is kept for backward compatibility during migration detection.
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

// AuthConfig holds authentication configuration for OAuth and JWT.
type AuthConfig struct {
	JWTSecret       string        `toml:"jwt_secret"`
	TokenExpiry     string        `toml:"token_expiry"`      // duration string, default "24h"
	CallbackBaseURL string        `toml:"callback_base_url"` // e.g. "https://vire-pprod-portal.fly.dev" — overrides request-derived scheme+host
	Google          OAuthProvider `toml:"google"`
	GitHub          OAuthProvider `toml:"github"`
}

// OAuthProvider holds OAuth client credentials for an external provider.
type OAuthProvider struct {
	ClientID     string `toml:"client_id"`
	ClientSecret string `toml:"client_secret"`
}

// GetTokenExpiry parses and returns the token expiry duration.
func (c *AuthConfig) GetTokenExpiry() time.Duration {
	d, err := time.ParseDuration(c.TokenExpiry)
	if err != nil {
		return 24 * time.Hour
	}
	return d
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level      string   `toml:"level" mapstructure:"level"`
	Format     string   `toml:"format" mapstructure:"format"`
	Outputs    []string `toml:"outputs" mapstructure:"outputs"`
	FilePath   string   `toml:"file_path" mapstructure:"file_path"`
	MaxSizeMB  int      `toml:"max_size_mb" mapstructure:"max_size_mb"`
	MaxBackups int      `toml:"max_backups" mapstructure:"max_backups"`
}

// NewDefaultConfig returns a Config with sensible defaults
func NewDefaultConfig() *Config {
	return &Config{
		Environment: "production",
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		Storage: StorageConfig{
			Address:   "ws://localhost:8000/rpc",
			Namespace: "vire",
			Database:  "vire",
			Username:  "root",
			Password:  "root",
			DataPath:  "data/market", // fallback for raw charts
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
				Model:          "gemini-3-flash-preview",
				MaxURLs:        20,
				MaxContentSize: "34MB",
			},
		},
		Auth: AuthConfig{
			JWTSecret:   "change-me-in-production",
			TokenExpiry: "24h",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			Outputs:    []string{"console", "file"},
			FilePath:   "logs/vire.log",
			MaxSizeMB:  100,
			MaxBackups: 3,
		},
		JobManager: JobManagerConfig{
			Enabled:             true,
			WatcherInterval:     "1m",
			MaxConcurrent:       5,
			MaxRetries:          3,
			PurgeAfter:          "24h",
			WatcherStartupDelay: "10s",
			HeavyJobLimit:       1,
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
		config.Storage.DataPath = path
	}

	if addr := os.Getenv("VIRE_STORAGE_ADDRESS"); addr != "" {
		config.Storage.Address = addr
	}
	if v := os.Getenv("VIRE_STORAGE_PASSWORD"); v != "" {
		config.Storage.Password = v
	}
	if v := os.Getenv("VIRE_STORAGE_NAMESPACE"); v != "" {
		config.Storage.Namespace = v
	}
	if v := os.Getenv("VIRE_STORAGE_DATABASE"); v != "" {
		config.Storage.Database = v
	}
	if v := os.Getenv("VIRE_STORAGE_USERNAME"); v != "" {
		config.Storage.Username = v
	}

	// Client API key overrides
	for _, envVar := range []string{"EODHD_API_KEY", "VIRE_EODHD_API_KEY"} {
		if v := os.Getenv(envVar); v != "" {
			config.Clients.EODHD.APIKey = v
			break
		}
	}
	for _, envVar := range []string{"GEMINI_API_KEY", "VIRE_GEMINI_API_KEY", "GOOGLE_API_KEY"} {
		if v := os.Getenv(envVar); v != "" {
			config.Clients.Gemini.APIKey = v
			break
		}
	}

	// Auth overrides
	if v := os.Getenv("VIRE_AUTH_JWT_SECRET"); v != "" {
		config.Auth.JWTSecret = v
	}
	if v := os.Getenv("VIRE_AUTH_TOKEN_EXPIRY"); v != "" {
		config.Auth.TokenExpiry = v
	}
	if v := os.Getenv("VIRE_AUTH_GOOGLE_CLIENT_ID"); v != "" {
		config.Auth.Google.ClientID = v
	}
	if v := os.Getenv("VIRE_AUTH_GOOGLE_CLIENT_SECRET"); v != "" {
		config.Auth.Google.ClientSecret = v
	}
	if v := os.Getenv("VIRE_AUTH_GITHUB_CLIENT_ID"); v != "" {
		config.Auth.GitHub.ClientID = v
	}
	if v := os.Getenv("VIRE_AUTH_GITHUB_CLIENT_SECRET"); v != "" {
		config.Auth.GitHub.ClientSecret = v
	}
	if v := os.Getenv("VIRE_AUTH_CALLBACK_BASE_URL"); v != "" {
		config.Auth.CallbackBaseURL = v
	}

	// Job manager overrides
	if v := os.Getenv("VIRE_JOBS_MAX_CONCURRENT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.JobManager.MaxConcurrent = n
		}
	}
	if v := os.Getenv("VIRE_WATCHER_STARTUP_DELAY"); v != "" {
		config.JobManager.WatcherStartupDelay = v
	}
	if v := os.Getenv("VIRE_JOBS_HEAVY_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			config.JobManager.HeavyJobLimit = n
		}
	}
	if v := os.Getenv("VIRE_FILING_SIZE_THRESHOLD"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			config.JobManager.FilingSizeThreshold = n
		}
	}
}

// ValidateRequired checks that all required configuration fields are set.
// Returns a list of human-readable error messages, one per missing field.
// An empty slice means all required fields are present.
func (c *Config) ValidateRequired() []string {
	var missing []string

	if c.Clients.EODHD.APIKey == "" {
		missing = append(missing, "[clients.eodhd] api_key is required (or set EODHD_API_KEY env var)")
	}
	if c.Clients.Gemini.APIKey == "" {
		missing = append(missing, "[clients.gemini] api_key is required (or set GEMINI_API_KEY env var)")
	}
	if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == "change-me-in-production" {
		missing = append(missing, "[auth] jwt_secret must be set to a secure value (or set VIRE_AUTH_JWT_SECRET env var)")
	}
	if c.Auth.Google.ClientID == "" {
		missing = append(missing, "[auth.google] client_id is required (or set VIRE_AUTH_GOOGLE_CLIENT_ID env var)")
	}
	if c.Auth.Google.ClientSecret == "" {
		missing = append(missing, "[auth.google] client_secret is required (or set VIRE_AUTH_GOOGLE_CLIENT_SECRET env var)")
	}
	if c.Auth.GitHub.ClientID == "" {
		missing = append(missing, "[auth.github] client_id is required (or set VIRE_AUTH_GITHUB_CLIENT_ID env var)")
	}
	if c.Auth.GitHub.ClientSecret == "" {
		missing = append(missing, "[auth.github] client_secret is required (or set VIRE_AUTH_GITHUB_CLIENT_SECRET env var)")
	}

	return missing
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	return env == "production" || env == "prod"
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	env := strings.ToLower(strings.TrimSpace(c.Environment))
	return env == "development" || env == "dev"
}

// ResolveDefaultPortfolio resolves the default portfolio name.
// Priority: InternalStore (runtime) > VIRE_DEFAULT_PORTFOLIO env > empty string.
func ResolveDefaultPortfolio(ctx context.Context, store interfaces.InternalStore) string {
	// InternalStore system KV (highest priority — set at runtime via set_default_portfolio tool)
	if store != nil {
		if val, err := store.GetSystemKV(ctx, "default_portfolio"); err == nil && val != "" {
			return val
		}
	}

	// Environment variable
	if val := os.Getenv("VIRE_DEFAULT_PORTFOLIO"); val != "" {
		return val
	}

	return ""
}

// ResolveAPIKey resolves an API key from environment, InternalStore, or fallback
func ResolveAPIKey(ctx context.Context, store interfaces.InternalStore, name string, fallback string) (string, error) {
	// Environment variable mapping
	keyToEnvMapping := map[string][]string{
		"eodhd_api_key":  {"EODHD_API_KEY", "VIRE_EODHD_API_KEY"},
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

	// Try InternalStore system KV (medium priority)
	if store != nil {
		apiKey, err := store.GetSystemKV(ctx, name)
		if err == nil && apiKey != "" {
			return apiKey, nil
		}
	}

	// Fallback (lowest priority)
	if fallback != "" {
		return fallback, nil
	}

	return "", fmt.Errorf("API key '%s' not found in environment or store", name)
}
