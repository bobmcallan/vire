// Package common provides shared test infrastructure
package common

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/storage"
)

// DockerTestEnvironment provides an isolated test environment
type DockerTestEnvironment struct {
	t              *testing.T
	StorageManager *storage.Manager
	Config         *common.Config
	Logger         *common.Logger
	DataDir        string
	cleanup        []func()
}

// SetupDockerTestEnvironment creates a new test environment
// Returns nil if Docker is not available or tests are skipped
func SetupDockerTestEnvironment(t *testing.T) *DockerTestEnvironment {
	t.Helper()

	// Check if Docker tests are enabled
	if os.Getenv("VIRE_TEST_DOCKER") != "true" {
		t.Skip("Docker tests disabled (set VIRE_TEST_DOCKER=true to enable)")
		return nil
	}

	// Create temporary data directory
	dataDir, err := os.MkdirTemp("", "vire-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create test config
	config := common.NewDefaultConfig()
	config.Storage.Badger.Path = dataDir
	config.Environment = "test"

	// Create silent logger for tests
	logger := common.NewSilentLogger()

	// Initialize storage
	storageManager, err := storage.NewStorageManager(logger, config)
	if err != nil {
		os.RemoveAll(dataDir)
		t.Fatalf("Failed to initialize storage: %v", err)
	}

	env := &DockerTestEnvironment{
		t:              t,
		StorageManager: storageManager.(*storage.Manager),
		Config:         config,
		Logger:         logger,
		DataDir:        dataDir,
		cleanup:        make([]func(), 0),
	}

	// Register cleanup
	env.cleanup = append(env.cleanup, func() {
		storageManager.Close()
		os.RemoveAll(dataDir)
	})

	return env
}

// Cleanup releases all test resources
func (e *DockerTestEnvironment) Cleanup() {
	for i := len(e.cleanup) - 1; i >= 0; i-- {
		e.cleanup[i]()
	}
}

// AddCleanup registers a cleanup function
func (e *DockerTestEnvironment) AddCleanup(fn func()) {
	e.cleanup = append(e.cleanup, fn)
}

// Context returns a test context with timeout
func (e *DockerTestEnvironment) Context() context.Context {
	timeout := 30 * time.Second
	if envTimeout := os.Getenv("VIRE_TEST_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil {
			timeout = d
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	e.AddCleanup(cancel)
	return ctx
}

// TestOutputGuard validates test outputs
type TestOutputGuard struct {
	t       *testing.T
	outputs map[string]string
}

// NewTestOutputGuard creates a new output guard
func NewTestOutputGuard(t *testing.T) *TestOutputGuard {
	return &TestOutputGuard{
		t:       t,
		outputs: make(map[string]string),
	}
}

// AssertContains checks if output contains expected text
func (g *TestOutputGuard) AssertContains(output, expected string) {
	g.t.Helper()
	if !contains(output, expected) {
		g.t.Errorf("Expected output to contain %q, but it didn't.\nOutput: %s", expected, truncate(output, 500))
	}
}

// AssertNotContains checks if output does not contain text
func (g *TestOutputGuard) AssertNotContains(output, unexpected string) {
	g.t.Helper()
	if contains(output, unexpected) {
		g.t.Errorf("Expected output NOT to contain %q, but it did.\nOutput: %s", unexpected, truncate(output, 500))
	}
}

// SaveResult saves output to the results directory
func (g *TestOutputGuard) SaveResult(name, output string) error {
	g.outputs[name] = output

	// Create results directory
	resultsDir := filepath.Join("test", "results", "api", time.Now().Format("20060102-150405")+"-"+g.t.Name())
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return err
	}

	// Write output file
	outputPath := filepath.Join(resultsDir, name+".md")
	return os.WriteFile(outputPath, []byte(output), 0644)
}

// Helper functions

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr) >= 0))
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
