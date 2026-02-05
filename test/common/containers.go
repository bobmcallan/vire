// Package common provides shared test infrastructure
package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	buildOnce  sync.Once
	buildError error
)

// Env represents an isolated Docker test environment
type Env struct {
	t          *testing.T
	container  testcontainers.Container
	ctx        context.Context
	cancel     context.CancelFunc
	BaseURL    string
	Port       int
	ResultsDir string
}

// buildTestImage builds the Docker image once per test run
func buildTestImage() error {
	buildOnce.Do(func() {
		ctx := context.Background()

		// Find project root (walk up from test/common/)
		projectRoot := findProjectRoot()

		req := testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				FromDockerfile: testcontainers.FromDockerfile{
					Context:    projectRoot,
					Dockerfile: "tests/docker/Dockerfile",
					Tag:        "vire-mcp:test",
					KeepImage:  true,
				},
			},
		}

		// Build via a throwaway container request to cache the image
		_, buildError = testcontainers.GenericContainer(ctx, req)
		if buildError != nil {
			// If container creation failed but image built, that's ok
			if strings.Contains(buildError.Error(), "vire-mcp:test") {
				buildError = nil
			}
		}
	})
	return buildError
}

// NewEnv creates a new isolated Docker test environment
func NewEnv(t *testing.T) *Env {
	t.Helper()

	// Skip if Docker tests disabled
	if os.Getenv("VIRE_TEST_DOCKER") != "true" {
		t.Skip("Docker tests disabled (set VIRE_TEST_DOCKER=true to enable)")
		return nil
	}

	// Build image once
	if err := buildTestImage(); err != nil {
		t.Fatalf("Failed to build test image: %v", err)
	}

	// Create results directory
	resultsDir := filepath.Join(findProjectRoot(), "tests", "results", t.Name())
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatalf("Failed to create results dir: %v", err)
	}

	// Create context with timeout
	timeout := 60 * time.Second
	if envTimeout := os.Getenv("VIRE_TEST_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Create container
	req := testcontainers.ContainerRequest{
		Image:        "vire-mcp:test",
		ExposedPorts: []string{"4242/tcp"},
		WaitingFor:   wait.ForHTTP("/sse").WithPort("4242/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		cancel()
		t.Fatalf("Failed to start container: %v", err)
	}

	// Get mapped port
	mappedPort, err := container.MappedPort(ctx, "4242/tcp")
	if err != nil {
		container.Terminate(ctx)
		cancel()
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		cancel()
		t.Fatalf("Failed to get host: %v", err)
	}

	port := mappedPort.Int()
	baseURL := fmt.Sprintf("http://%s:%d", host, port)

	env := &Env{
		t:          t,
		container:  container,
		ctx:        ctx,
		cancel:     cancel,
		BaseURL:    baseURL,
		Port:       port,
		ResultsDir: resultsDir,
	}

	t.Logf("Container started: %s (port %d)", baseURL, port)

	return env
}

// Cleanup tears down the container and collects logs
func (e *Env) Cleanup() {
	if e == nil {
		return
	}

	// Collect container logs before teardown
	e.collectLogs()

	if e.container != nil {
		if err := e.container.Terminate(e.ctx); err != nil {
			e.t.Logf("Warning: failed to terminate container: %v", err)
		}
	}

	if e.cancel != nil {
		e.cancel()
	}
}

// Context returns the test context
func (e *Env) Context() context.Context {
	return e.ctx
}

// MCPRequest sends a JSON-RPC request to the MCP server
func (e *Env) MCPRequest(method string, params interface{}) (json.RawMessage, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Connect to SSE endpoint first to get message URL
	sseResp, err := http.Get(e.BaseURL + "/sse")
	if err != nil {
		return nil, fmt.Errorf("connect to SSE: %w", err)
	}

	// Read the endpoint event
	buf := make([]byte, 4096)
	n, err := sseResp.Body.Read(buf)
	sseResp.Body.Close()
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read SSE endpoint: %w", err)
	}

	// Parse message URL from SSE event
	lines := strings.Split(string(buf[:n]), "\n")
	var messageURL string
	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			messageURL = strings.TrimPrefix(line, "data: ")
			messageURL = strings.TrimSpace(messageURL)
			break
		}
	}

	if messageURL == "" {
		return nil, fmt.Errorf("no message URL in SSE response")
	}

	// Send the request
	resp, err := http.Post(messageURL, "application/json", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return json.RawMessage(respBody), nil
}

// Get performs an HTTP GET request to the container
func (e *Env) Get(path string) (*http.Response, []byte, error) {
	resp, err := http.Get(e.BaseURL + path)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, err
	}

	return resp, body, nil
}

// SaveResult saves test output to the results directory
func (e *Env) SaveResult(name string, data []byte) error {
	return os.WriteFile(filepath.Join(e.ResultsDir, name), data, 0644)
}

// collectLogs saves container logs to results directory
func (e *Env) collectLogs() {
	if e.container == nil {
		return
	}

	reader, err := e.container.Logs(e.ctx)
	if err != nil {
		e.t.Logf("Warning: failed to get container logs: %v", err)
		return
	}
	defer reader.Close()

	logs, err := io.ReadAll(reader)
	if err != nil {
		e.t.Logf("Warning: failed to read container logs: %v", err)
		return
	}

	logPath := filepath.Join(e.ResultsDir, "container.log")
	if err := os.WriteFile(logPath, logs, 0644); err != nil {
		e.t.Logf("Warning: failed to save logs: %v", err)
	}
}

// findProjectRoot walks up directories to find go.mod
func findProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
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
	if !strings.Contains(output, expected) {
		g.t.Errorf("Expected output to contain %q, but it didn't.\nOutput: %s", expected, truncate(output, 500))
	}
}

// AssertNotContains checks if output does not contain text
func (g *TestOutputGuard) AssertNotContains(output, unexpected string) {
	g.t.Helper()
	if strings.Contains(output, unexpected) {
		g.t.Errorf("Expected output NOT to contain %q, but it did.\nOutput: %s", unexpected, truncate(output, 500))
	}
}

// SaveResult saves output to the results directory
func (g *TestOutputGuard) SaveResult(name, output string) error {
	g.outputs[name] = output

	resultsDir := filepath.Join(findProjectRoot(), "tests", "results", g.t.Name())
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return err
	}

	outputPath := filepath.Join(resultsDir, name+".md")
	return os.WriteFile(outputPath, []byte(output), 0644)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
