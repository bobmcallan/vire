package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestNewApp_InitializesAllServices verifies that NewApp creates an App with
// all services, clients, and the MCP server initialized and non-nil.
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
	if a.MCPServer == nil {
		t.Error("MCPServer is nil")
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

// TestNewApp_RegistersAllTools verifies that NewApp registers all expected MCP tools.
func TestNewApp_RegistersAllTools(t *testing.T) {
	configPath := writeTestConfig(t)

	a, err := NewApp(configPath)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	defer a.Close()

	// Use in-process client to list tools
	client, err := newInProcessClient(t, a.MCPServer)
	if err != nil {
		t.Fatalf("Failed to create in-process client: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	toolsResult, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	// All 39 tools that should be registered
	expectedTools := []string{
		"get_version",
		"portfolio_review",
		"get_portfolio",
		"market_snipe",
		"stock_screen",
		"get_stock_data",
		"detect_signals",
		"list_portfolios",
		"sync_portfolio",
		"rebuild_data",
		"collect_market_data",
		"generate_report",
		"generate_ticker_report",
		"list_reports",
		"get_summary",
		"get_ticker_report",
		"list_tickers",
		"get_portfolio_snapshot",
		"get_portfolio_history",
		"set_default_portfolio",
		"get_config",
		"get_strategy_template",
		"set_portfolio_strategy",
		"get_portfolio_strategy",
		"delete_portfolio_strategy",
		"get_portfolio_plan",
		"set_portfolio_plan",
		"add_plan_item",
		"update_plan_item",
		"remove_plan_item",
		"check_plan_status",
		"funnel_screen",
		"list_searches",
		"get_search",
		"get_watchlist",
		"add_watchlist_item",
		"update_watchlist_item",
		"remove_watchlist_item",
		"set_watchlist",
		"get_diagnostics",
	}

	toolNames := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}

	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("Expected tool %q not registered", name)
		}
	}

	if len(toolsResult.Tools) < 30 {
		t.Errorf("Expected at least 30 tools, got %d", len(toolsResult.Tools))
	}
}

// TestNewApp_GetVersionToolWorks verifies that the get_version tool works
// through a full App initialization.
func TestNewApp_GetVersionToolWorks(t *testing.T) {
	configPath := writeTestConfig(t)

	a, err := NewApp(configPath)
	if err != nil {
		t.Fatalf("NewApp failed: %v", err)
	}
	defer a.Close()

	client, err := newInProcessClient(t, a.MCPServer)
	if err != nil {
		t.Fatalf("Failed to create in-process client: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	req := mcp.CallToolRequest{}
	req.Params.Name = "get_version"
	result, err := client.CallTool(ctx, req)
	if err != nil {
		t.Fatalf("get_version failed: %v", err)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "Vire MCP Server") {
		t.Errorf("Expected 'Vire MCP Server' in output, got: %s", text)
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
// No API keys are configured — clients will be nil, which is acceptable.
func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create data and logs subdirectories
	os.MkdirAll(filepath.Join(dir, "data"), 0755)
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)

	config := `
[storage.file]
path = "` + filepath.Join(dir, "data") + `"
versions = 2

[logging]
level = "error"
output = ["console"]
file_path = "` + filepath.Join(dir, "logs", "vire.log") + `"
`
	configPath := filepath.Join(dir, "vire.toml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	return configPath
}

// newInProcessClient creates an mcp-go in-process client connected to the given
// MCP server. Handles initialization handshake.
func newInProcessClient(t *testing.T, mcpServer *server.MCPServer) (*client.Client, error) {
	t.Helper()

	c, err := client.NewInProcessClient(mcpServer)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		return nil, err
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}
