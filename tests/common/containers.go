// Package common provides shared test infrastructure
package common

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// EnvOptions configures the Docker test environment
type EnvOptions struct {
	// ConfigFile is the config file name to use (e.g., "vire.toml" or "vire-blank.toml")
	// Defaults to "vire.toml" if empty
	ConfigFile string
}

// Env represents an isolated Docker test environment
type Env struct {
	t          *testing.T
	container  testcontainers.Container
	ctx        context.Context
	cancel     context.CancelFunc
	ResultsDir string
	configFile string
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
					Repo:       "vire-mcp",
					Tag:        "test",
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

// NewEnv creates a new isolated Docker test environment with default options.
// The container runs a stdio MCP server; use ExecMCP to send JSON-RPC requests.
func NewEnv(t *testing.T) *Env {
	return NewEnvWithOptions(t, EnvOptions{})
}

// NewEnvWithOptions creates a new isolated Docker test environment with custom options.
func NewEnvWithOptions(t *testing.T, opts EnvOptions) *Env {
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

	// Default config file
	configFile := opts.ConfigFile
	if configFile == "" {
		configFile = "vire.toml"
	}

	// Create results directory with datetime prefix: {datetime}-{test-name}
	datetime := time.Now().Format("20060102-150405")
	resultsDir := filepath.Join(findProjectRoot(), "tests", "results", datetime+"-"+t.Name())
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

	// Create container — runs tail keepalive; MCP server starts per-request via docker exec
	req := testcontainers.ContainerRequest{
		Image:      "vire-mcp:test",
		WaitingFor: wait.ForExec([]string{"ls", "./vire-mcp"}).WithStartupTimeout(10 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		cancel()
		t.Fatalf("Failed to start container: %v", err)
	}

	env := &Env{
		t:          t,
		container:  container,
		ctx:        ctx,
		cancel:     cancel,
		ResultsDir: resultsDir,
		configFile: configFile,
	}

	t.Logf("Container started (stdio mode, config: %s)", configFile)

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

// MCPRequest sends a JSON-RPC request to the MCP server via docker exec.
// It spawns a new process inside the container that communicates over stdio.
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

	// Use docker exec to send the JSON-RPC request via stdin to a new vire-mcp process
	// Use the configured config file
	cmd := fmt.Sprintf("echo '%s' | ./vire-mcp --config ./%s", string(bodyBytes), e.configFile)
	code, reader, err := e.container.Exec(e.ctx, []string{"sh", "-c", cmd})
	if err != nil {
		return nil, fmt.Errorf("exec failed: %w", err)
	}

	output, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read exec output: %w", err)
	}

	if code != 0 {
		return nil, fmt.Errorf("exec exited with code %d: %s", code, string(output))
	}

	return json.RawMessage(output), nil
}

// SaveResult saves test output to the results directory
func (e *Env) SaveResult(name string, data []byte) error {
	return os.WriteFile(filepath.Join(e.ResultsDir, name), data, 0644)
}

// OutputGuard returns a TestOutputGuard that uses the same results directory as this Env
func (e *Env) OutputGuard() *TestOutputGuard {
	return NewTestOutputGuardWithDir(e.t, e.ResultsDir)
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
	t          *testing.T
	outputs    map[string]string
	resultsDir string
}

// NewTestOutputGuard creates a new output guard with datetime-prefixed results directory
func NewTestOutputGuard(t *testing.T) *TestOutputGuard {
	datetime := time.Now().Format("20060102-150405")
	resultsDir := filepath.Join(findProjectRoot(), "tests", "results", datetime+"-"+t.Name())
	return &TestOutputGuard{
		t:          t,
		outputs:    make(map[string]string),
		resultsDir: resultsDir,
	}
}

// NewTestOutputGuardWithDir creates a new output guard with a specific results directory
func NewTestOutputGuardWithDir(t *testing.T, resultsDir string) *TestOutputGuard {
	return &TestOutputGuard{
		t:          t,
		outputs:    make(map[string]string),
		resultsDir: resultsDir,
	}
}

// ResultsDir returns the results directory path
func (g *TestOutputGuard) ResultsDir() string {
	return g.resultsDir
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

	if err := os.MkdirAll(g.resultsDir, 0755); err != nil {
		return err
	}

	outputPath := filepath.Join(g.resultsDir, name+".md")
	return os.WriteFile(outputPath, []byte(output), 0644)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// FormatJSON pretty-prints JSON for readable output.
// It extracts the JSON from container output (which may include log lines).
func FormatJSON(data json.RawMessage) string {
	// Extract just the JSON part (skip log lines)
	cleaned := extractJSON(string(data))
	if cleaned == "" {
		return string(data)
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(cleaned), &parsed); err != nil {
		return string(data)
	}
	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return string(data)
	}
	return string(formatted)
}

// FormatMCPContent extracts the markdown content from an MCP tool response.
// MCP responses contain JSON-RPC wrapper with result.content[].text fields.
// This function extracts and concatenates all text content for readable output.
func FormatMCPContent(data json.RawMessage) string {
	resp, err := ParseMCPToolResponse(data)
	if err != nil {
		// Fall back to formatted JSON if parsing fails
		return FormatJSON(data)
	}

	// Check for JSON-RPC error
	if resp.Error != nil {
		return fmt.Sprintf("# MCP Error\n\nCode: %d\nMessage: %s\n", resp.Error.Code, resp.Error.Message)
	}

	// Check for tool error
	if resp.Result.IsError {
		var texts []string
		for _, c := range resp.Result.Content {
			if c.Type == "text" && c.Text != "" {
				texts = append(texts, c.Text)
			}
		}
		if len(texts) > 0 {
			return "# Tool Error\n\n" + strings.Join(texts, "\n")
		}
		return "# Tool Error\n\nUnknown error (no content)"
	}

	// Extract text content
	var texts []string
	for _, c := range resp.Result.Content {
		if c.Type == "text" && c.Text != "" {
			texts = append(texts, c.Text)
		}
	}

	if len(texts) == 0 {
		return "# Empty Response\n\nNo content returned from tool."
	}

	return strings.Join(texts, "\n")
}

// MCPToolResponse represents the structure of an MCP tools/call response
type MCPToolResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Result  struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ParseMCPToolResponse parses an MCP tool response from raw JSON
func ParseMCPToolResponse(data json.RawMessage) (*MCPToolResponse, error) {
	// Strip any leading non-JSON content (e.g., log lines from container)
	cleaned := extractJSON(string(data))
	if cleaned == "" {
		return nil, fmt.Errorf("no valid JSON found in response")
	}

	var resp MCPToolResponse
	if err := json.Unmarshal([]byte(cleaned), &resp); err != nil {
		return nil, fmt.Errorf("parse MCP response: %w", err)
	}
	return &resp, nil
}

// extractJSON finds and returns the first valid JSON object in the string
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	// Find the matching closing brace
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// ValidateMCPToolResponse validates that an MCP tool response is successful
// Returns an error if the response indicates an error or has empty content
func ValidateMCPToolResponse(data json.RawMessage) error {
	resp, err := ParseMCPToolResponse(data)
	if err != nil {
		return err
	}

	// Check for JSON-RPC error
	if resp.Error != nil {
		return fmt.Errorf("MCP error [%d]: %s", resp.Error.Code, resp.Error.Message)
	}

	// Check for tool error flag
	if resp.Result.IsError {
		if len(resp.Result.Content) > 0 {
			return fmt.Errorf("MCP tool error: %s", resp.Result.Content[0].Text)
		}
		return fmt.Errorf("MCP tool returned error with no content")
	}

	// Check for empty content
	if len(resp.Result.Content) == 0 {
		return fmt.Errorf("MCP tool returned empty content")
	}

	// Check that content has actual text
	hasContent := false
	for _, c := range resp.Result.Content {
		if c.Text != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return fmt.Errorf("MCP tool returned content with no text")
	}

	return nil
}
