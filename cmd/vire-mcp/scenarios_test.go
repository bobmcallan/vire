package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// TestScenario_ToolDiscoveryAndVersion simulates Claude Desktop's initial connection:
// it discovers available tools via listTools, then calls get_version to verify connectivity.
func TestScenario_ToolDiscoveryAndVersion(t *testing.T) {
	h := newTestHarness(t)

	// Step 1: Claude Desktop calls listTools to discover available tools
	toolsResult, err := h.client.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	// Verify expected tools are registered
	toolNames := make(map[string]bool)
	for _, tool := range toolsResult.Tools {
		toolNames[tool.Name] = true
	}

	expectedTools := []string{"get_version", "get_portfolio_history", "set_default_portfolio"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("Expected tool '%s' not found in listTools response", name)
		}
	}

	// Step 2: Claude Desktop calls get_version to verify server health
	result, err := h.callTool("get_version", nil)
	if err != nil {
		t.Fatalf("get_version failed: %v", err)
	}

	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "Vire MCP Server") {
		t.Errorf("Expected version output to contain 'Vire MCP Server', got: %s", text)
	}
	if !strings.Contains(text, "Status: OK") {
		t.Errorf("Expected 'Status: OK' in version output")
	}
}

// TestScenario_SetDefaultThenHistory simulates a Claude Desktop session where the user
// first sets a default portfolio, then asks for history without specifying the name.
func TestScenario_SetDefaultThenHistory(t *testing.T) {
	h := newTestHarness(t)

	h.mockPortfolio.listPortfoliosFn = func(ctx context.Context) ([]string, error) {
		return []string{"SMSF", "Personal", "Trading"}, nil
	}

	var historyPortfolioName string
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		historyPortfolioName = name
		return generateTestGrowthPoints(10), nil
	}

	// Step 1: User says "Use my Personal portfolio"
	// Claude Desktop calls set_default_portfolio
	result, err := h.callTool("set_default_portfolio", map[string]any{
		"portfolio_name": "Personal",
	})
	if err != nil {
		t.Fatalf("Step 1 (set_default_portfolio) failed: %v", err)
	}
	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "Personal") {
		t.Errorf("Expected confirmation of 'Personal' default, got: %s", text)
	}

	// Step 2: User asks "How's my portfolio doing?"
	// Claude Desktop calls get_portfolio_history WITHOUT portfolio_name
	// The KV store now has "Personal" as default
	result, err = h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("Step 2 (get_portfolio_history) failed: %v", err)
	}

	// The handler should have used "Personal" from KV storage
	if historyPortfolioName != "Personal" {
		t.Errorf("Expected portfolio name 'Personal' from KV default, got '%s'", historyPortfolioName)
	}

	// Verify we still get structured data
	if len(result.Content) != 2 {
		t.Fatalf("Expected 2 content blocks, got %d", len(result.Content))
	}
}

// TestScenario_DailyHistoryForCharting simulates Claude Desktop requesting daily data
// explicitly for building a chart artifact — verifying every data point is present
// and the JSON structure is complete.
func TestScenario_DailyHistoryForCharting(t *testing.T) {
	h := newTestHarness(t)

	numDays := 60
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(numDays), nil
	}

	// Claude Desktop explicitly requests daily format for maximum granularity
	result, err := h.callTool("get_portfolio_history", map[string]any{
		"format": "daily",
	})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// Extract and parse JSON
	jsonStr := h.getTextContent(result, 1)
	if !strings.HasPrefix(jsonStr, "<!-- CHART_DATA -->") {
		t.Fatalf("Missing CHART_DATA marker")
	}

	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var points []struct {
		Date     string  `json:"date"`
		Value    float64 `json:"value"`
		Cost     float64 `json:"cost"`
		Gain     float64 `json:"gain"`
		GainPct  float64 `json:"gain_pct"`
		Holdings int     `json:"holding_count"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &points); err != nil {
		t.Fatalf("JSON parse failed: %v", err)
	}

	// Verify every day is present (no gaps from downsampling)
	if len(points) != numDays {
		t.Fatalf("Expected %d daily points, got %d — data was unexpectedly downsampled", numDays, len(points))
	}

	// Verify dates are sequential (no gaps)
	for i := 1; i < len(points); i++ {
		prev := points[i-1].Date
		curr := points[i].Date
		if prev >= curr {
			t.Errorf("Dates not sequential: points[%d]=%s >= points[%d]=%s", i-1, prev, i, curr)
		}
	}

	// Verify values are reasonable (monotonically increasing in our test data)
	for i := 1; i < len(points); i++ {
		if points[i].Value <= points[i-1].Value {
			t.Errorf("Values not increasing: points[%d]=%.2f <= points[%d]=%.2f",
				i, points[i].Value, i-1, points[i-1].Value)
		}
	}

	// Verify gain = value - cost
	for i, p := range points {
		expectedGain := p.Value - p.Cost
		diff := p.Gain - expectedGain
		if diff > 0.01 || diff < -0.01 {
			t.Errorf("Point %d: gain %.2f != value %.2f - cost %.2f (diff: %.4f)",
				i, p.Gain, p.Value, p.Cost, diff)
		}
	}
}

// TestScenario_WeeklyOverviewThenDrillDown simulates a user starting with a high-level
// weekly view, then drilling down to daily for a specific period.
func TestScenario_WeeklyOverviewThenDrillDown(t *testing.T) {
	h := newTestHarness(t)

	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(180), nil
	}

	// Step 1: Get weekly overview (180 days → auto=weekly)
	result, err := h.callTool("get_portfolio_history", map[string]any{
		"format": "weekly",
	})
	if err != nil {
		t.Fatalf("Step 1 failed: %v", err)
	}

	jsonStr := h.getTextContent(result, 1)
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var weeklyPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &weeklyPoints); err != nil {
		t.Fatalf("Failed to parse weekly JSON: %v", err)
	}

	// Should be ~26 weekly points (180/7)
	if len(weeklyPoints) < 20 || len(weeklyPoints) > 30 {
		t.Errorf("Expected ~26 weekly points, got %d", len(weeklyPoints))
	}

	// Step 2: Drill down to daily for same period
	result, err = h.callTool("get_portfolio_history", map[string]any{
		"format": "daily",
	})
	if err != nil {
		t.Fatalf("Step 2 failed: %v", err)
	}

	jsonStr = h.getTextContent(result, 1)
	rawJSON = strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var dailyPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &dailyPoints); err != nil {
		t.Fatalf("Failed to parse daily JSON: %v", err)
	}

	// Daily should have all 180 points
	if len(dailyPoints) != 180 {
		t.Errorf("Expected 180 daily points, got %d", len(dailyPoints))
	}

	// Daily should have more data points than weekly
	if len(dailyPoints) <= len(weeklyPoints) {
		t.Errorf("Daily points (%d) should exceed weekly (%d)", len(dailyPoints), len(weeklyPoints))
	}
}
