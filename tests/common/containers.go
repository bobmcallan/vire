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

	toml "github.com/pelletier/go-toml/v2"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	buildOnce  sync.Once
	buildError error
)

// EnvOptions configures the Docker test environment
type EnvOptions struct {
	// ConfigFile is the config file name to use (e.g., "vire-service.toml" or "vire-service-blank.toml")
	// Defaults to "vire-service.toml" if empty
	ConfigFile string
}

// Env represents an isolated Docker test environment
type Env struct {
	t           *testing.T
	container   testcontainers.Container
	surrealDB   testcontainers.Container
	testNetwork *testcontainers.DockerNetwork
	ctx         context.Context
	cancel      context.CancelFunc
	ResultsDir  string
	configFile  string
	serverURL   string
}

// buildTestImage builds the Docker image once per test run
func buildTestImage() error {
	buildOnce.Do(func() {
		ctx := context.Background()

		// Find project root (walk up from test/common/)
		projectRoot := FindProjectRoot()

		req := testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				FromDockerfile: testcontainers.FromDockerfile{
					Context:    projectRoot,
					Dockerfile: "tests/docker/Dockerfile.server",
					Repo:       "vire-server-itest",
					Tag:        "latest",
					KeepImage:  true,
				},
			},
		}

		// Build via a throwaway container request to cache the image
		_, buildError = testcontainers.GenericContainer(ctx, req)
		if buildError != nil {
			// If container creation failed but image built, that's ok
			if strings.Contains(buildError.Error(), "vire-server-itest") {
				buildError = nil
			}
		}
	})
	return buildError
}

// NewEnv creates a new isolated Docker test environment with default options.
func NewEnv(t *testing.T) *Env {
	return NewEnvWithOptions(t, EnvOptions{})
}

// NewEnvWithOptions creates a new isolated Docker test environment with custom options.
// It starts a SurrealDB container and a vire-server container on a shared Docker network.
func NewEnvWithOptions(t *testing.T, opts EnvOptions) *Env {
	t.Helper()

	// Build image once
	if err := buildTestImage(); err != nil {
		t.Fatalf("Failed to build test image: %v", err)
	}

	// Default config file
	configFile := opts.ConfigFile
	if configFile == "" {
		configFile = "vire-service.toml"
	}

	// Create results directory with datetime prefix: {datetime}-{test-name}
	datetime := time.Now().Format("20060102-150405")
	resultsDir := filepath.Join(FindProjectRoot(), "tests", "logs", datetime+"-"+t.Name())
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		t.Fatalf("Failed to create results dir: %v", err)
	}

	// Create context with timeout (240s default — portfolio review with EODHD collection is slow)
	timeout := 240 * time.Second
	if envTimeout := os.Getenv("VIRE_TEST_TIMEOUT"); envTimeout != "" {
		if d, err := time.ParseDuration(envTimeout); err == nil {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Create a shared Docker network for SurrealDB + vire-server
	testNet, err := network.New(ctx, network.WithCheckDuplicate())
	if err != nil {
		cancel()
		t.Fatalf("Failed to create Docker network: %v", err)
	}

	// Start SurrealDB container on the shared network with alias "surrealdb"
	surrealContainer, err := testcontainers.Run(ctx, "surrealdb/surrealdb:v3.0.0",
		testcontainers.WithExposedPorts("8000/tcp"),
		testcontainers.WithCmd("start", "--user", "root", "--pass", "root"),
		network.WithNetwork([]string{"surrealdb-itest"}, testNet),
		testcontainers.WithWaitStrategy(
			wait.ForAll(
				wait.ForListeningPort("8000/tcp"),
				wait.ForLog("Started web server"),
			).WithDeadline(60*time.Second),
		),
	)
	if err != nil {
		testNet.Remove(ctx)
		cancel()
		t.Fatalf("Failed to start SurrealDB container: %v", err)
	}

	// Get SurrealDB container IP to bypass Docker DNS resolution issues (CGO_ENABLED=0)
	surrealIP, err := surrealContainer.ContainerIP(ctx)
	if err != nil {
		surrealContainer.Terminate(ctx)
		testNet.Remove(ctx)
		cancel()
		t.Fatalf("Failed to get SurrealDB container IP: %v", err)
	}

	// Start vire-server container on the same network, passing SurrealDB address directly
	container, err := testcontainers.Run(ctx, "vire-server-itest:latest",
		testcontainers.WithExposedPorts("8080/tcp"),
		network.WithNetwork([]string{"vire-server-itest"}, testNet),
		testcontainers.WithEnv(map[string]string{
			"VIRE_STORAGE_ADDRESS": fmt.Sprintf("ws://%s:8000/rpc", surrealIP),
		}),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/api/health").WithPort("8080/tcp").WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		surrealContainer.Terminate(ctx)
		testNet.Remove(ctx)
		cancel()
		t.Fatalf("Failed to start vire-server container: %v", err)
	}

	// Get the mapped host port
	mappedPort, err := container.MappedPort(ctx, "8080/tcp")
	if err != nil {
		container.Terminate(ctx)
		surrealContainer.Terminate(ctx)
		testNet.Remove(ctx)
		cancel()
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		container.Terminate(ctx)
		surrealContainer.Terminate(ctx)
		testNet.Remove(ctx)
		cancel()
		t.Fatalf("Failed to get container host: %v", err)
	}

	serverURL := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	env := &Env{
		t:           t,
		container:   container,
		surrealDB:   surrealContainer,
		testNetwork: testNet,
		ctx:         ctx,
		cancel:      cancel,
		ResultsDir:  resultsDir,
		configFile:  configFile,
		serverURL:   serverURL,
	}

	t.Logf("Containers started (SurrealDB + vire-server, config: %s, url: %s)", configFile, serverURL)

	return env
}

// Cleanup tears down all containers, the network, and collects logs.
// Uses a fresh context for teardown in case the main context expired.
func (e *Env) Cleanup() {
	if e == nil {
		return
	}

	// Use a fresh context for cleanup — the main context may have expired
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cleanupCancel()

	// Collect container logs before teardown
	e.collectLogsWithCtx(cleanupCtx)

	if e.container != nil {
		if err := e.container.Terminate(cleanupCtx); err != nil {
			e.t.Logf("Warning: failed to terminate vire-server container: %v", err)
		}
	}

	if e.surrealDB != nil {
		if err := e.surrealDB.Terminate(cleanupCtx); err != nil {
			e.t.Logf("Warning: failed to terminate SurrealDB container: %v", err)
		}
	}

	if e.testNetwork != nil {
		if err := e.testNetwork.Remove(cleanupCtx); err != nil {
			e.t.Logf("Warning: failed to remove test network: %v", err)
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

// ServerURL returns the base URL of the running vire-server
func (e *Env) ServerURL() string {
	return e.serverURL
}

// HTTPGet sends a GET request to the vire-server and returns the response.
func (e *Env) HTTPGet(path string) (*http.Response, error) {
	return http.Get(e.serverURL + path)
}

// HTTPPost sends a POST request with JSON body to the vire-server.
func (e *Env) HTTPPost(path string, body interface{}) (*http.Response, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	return http.Post(e.serverURL+path, "application/json", strings.NewReader(string(bodyBytes)))
}

// HTTPPut sends a PUT request with JSON body to the vire-server.
func (e *Env) HTTPPut(path string, body interface{}) (*http.Response, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, e.serverURL+path, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return http.DefaultClient.Do(req)
}

// HTTPDelete sends a DELETE request to the vire-server.
func (e *Env) HTTPDelete(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, e.serverURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return http.DefaultClient.Do(req)
}

// HTTPRequest sends an HTTP request with custom headers to the vire-server.
// The request is bounded by the env's context timeout.
func (e *Env) HTTPRequest(method, path string, body interface{}, headers map[string]string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}
	req, err := http.NewRequestWithContext(e.ctx, method, e.serverURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return http.DefaultClient.Do(req)
}

// MCPRequest sends a JSON-RPC request to the MCP server via the Streamable HTTP endpoint.
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

	resp, err := http.Post(e.serverURL+"/mcp", "application/json", strings.NewReader(string(bodyBytes)))
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	output, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
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

// collectLogsWithCtx saves container logs to results directory using the given context
func (e *Env) collectLogsWithCtx(ctx context.Context) {
	if e.container == nil {
		return
	}

	reader, err := e.container.Logs(ctx)
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

// LoadEnvFile reads a KEY=VALUE file and sets any variables not already in the environment.
// Lines starting with # and empty lines are skipped. Existing env vars take precedence.
func LoadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return nil
}

// LoadTestSecrets loads API keys from both tests/docker/.env and tests/docker/vire-service.toml.
// Environment variables already set take precedence. This mirrors the server's own resolution —
// keys can live in either file and tests will find them.
func LoadTestSecrets() {
	root := FindProjectRoot()
	dockerDir := filepath.Join(root, "tests", "docker")

	// Load .env first (KEY=VALUE format)
	_ = LoadEnvFile(filepath.Join(dockerDir, ".env"))

	// Load TOML config for client API keys
	tomlPath := filepath.Join(dockerDir, "vire-service.toml")
	data, err := os.ReadFile(tomlPath)
	if err != nil {
		return
	}

	var cfg struct {
		Clients struct {
			EODHD struct {
				APIKey string `toml:"api_key"`
			} `toml:"eodhd"`
			Gemini struct {
				APIKey string `toml:"api_key"`
			} `toml:"gemini"`
		} `toml:"clients"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return
	}

	// Map TOML keys to env vars (only if not already set)
	if os.Getenv("EODHD_API_KEY") == "" && cfg.Clients.EODHD.APIKey != "" {
		os.Setenv("EODHD_API_KEY", cfg.Clients.EODHD.APIKey)
	}
	if os.Getenv("GEMINI_API_KEY") == "" && cfg.Clients.Gemini.APIKey != "" {
		os.Setenv("GEMINI_API_KEY", cfg.Clients.Gemini.APIKey)
	}
}

// FindProjectRoot walks up directories to find go.mod
func FindProjectRoot() string {
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
	resultsDir := filepath.Join(FindProjectRoot(), "tests", "logs", datetime+"-"+t.Name())
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
