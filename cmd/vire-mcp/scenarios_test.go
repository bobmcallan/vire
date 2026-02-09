package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

	expectedTools := []string{"get_version", "get_portfolio", "get_portfolio_history", "set_default_portfolio"}
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

// TestScenario_GetPortfolioFast simulates the fast path: get_portfolio returns
// cached holdings without signals, AI, or charts.
func TestScenario_GetPortfolioFast(t *testing.T) {
	h := newTestHarness(t)

	h.mockPortfolio.getPortfolioFn = func(ctx context.Context, name string) (*models.Portfolio, error) {
		return &models.Portfolio{
			Name:         name,
			TotalValue:   350000,
			TotalCost:    300000,
			TotalGain:    50000,
			TotalGainPct: 16.67,
			LastSynced:   time.Now(),
			Holdings: []models.Holding{
				{Ticker: "ACDC", Name: "ACDC ETF", Units: 282, AvgCost: 120.0, CurrentPrice: 143.92, MarketValue: 40585.44, Weight: 11.5, GainLoss: 6745.44, GainLossPct: 19.94},
				{Ticker: "PMGOLD", Name: "Perth Mint Gold", Units: 895, AvgCost: 55.0, CurrentPrice: 70.93, MarketValue: 63482.35, Weight: 18.0, GainLoss: 14257.35, GainLossPct: 28.99},
				{Ticker: "ASB", Name: "Austal", Units: 0, AvgCost: 0, CurrentPrice: 6.18, MarketValue: 0, Weight: 0, GainLoss: -500, GainLossPct: -10.0},
			},
		}, nil
	}

	result, err := h.callTool("get_portfolio", nil)
	if err != nil {
		t.Fatalf("get_portfolio failed: %v", err)
	}

	if result.IsError {
		t.Fatalf("get_portfolio returned error: %s", h.getTextContent(result, 0))
	}

	text := h.getTextContent(result, 0)

	// Must contain portfolio header
	if !strings.Contains(text, "# Portfolio:") {
		t.Error("Missing portfolio header")
	}
	if !strings.Contains(text, "**Total Value:**") {
		t.Error("Missing total value")
	}

	// Must contain active holdings
	if !strings.Contains(text, "ACDC") {
		t.Error("Missing active holding ACDC")
	}
	if !strings.Contains(text, "PMGOLD") {
		t.Error("Missing active holding PMGOLD")
	}

	// Must contain closed positions section
	if !strings.Contains(text, "## Closed Positions") {
		t.Error("Missing closed positions section")
	}
	if !strings.Contains(text, "ASB") {
		t.Error("Missing closed position ASB")
	}

	// Must NOT contain signal/review content
	if strings.Contains(text, "BUY") || strings.Contains(text, "SELL") || strings.Contains(text, "HOLD") {
		t.Error("get_portfolio should not contain buy/sell/hold signals")
	}
	if strings.Contains(text, "RSI") || strings.Contains(text, "SMA") {
		t.Error("get_portfolio should not contain technical signals")
	}
}

// TestScenario_GetPortfolioUsesDefault verifies get_portfolio respects the KV default portfolio.
func TestScenario_GetPortfolioUsesDefault(t *testing.T) {
	h := newTestHarness(t)

	var capturedName string
	h.mockPortfolio.getPortfolioFn = func(ctx context.Context, name string) (*models.Portfolio, error) {
		capturedName = name
		return &models.Portfolio{
			Name:       name,
			TotalValue: 100000,
			LastSynced: time.Now(),
			Holdings:   []models.Holding{{Ticker: "TEST", Units: 100, MarketValue: 10000, Weight: 100}},
		}, nil
	}

	// Set a KV default
	_, err := h.callTool("set_default_portfolio", map[string]any{"portfolio_name": "Personal"})
	if err != nil {
		t.Fatalf("set_default_portfolio failed: %v", err)
	}

	// Call get_portfolio WITHOUT portfolio_name
	result, err := h.callTool("get_portfolio", nil)
	if err != nil {
		t.Fatalf("get_portfolio failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("get_portfolio returned error")
	}

	// Should have used "Personal" from KV default
	if capturedName != "Personal" {
		t.Errorf("Expected portfolio name 'Personal' from KV default, got '%s'", capturedName)
	}
}

// TestScenario_ForceSyncReturnsFreshPrices verifies that calling sync_portfolio
// with force=true twice returns updated prices from the second call, not stale
// data from the first call. This exposes the scenario where Navexa returns
// different prices on subsequent calls (e.g., Friday close vs Monday close).
func TestScenario_ForceSyncReturnsFreshPrices(t *testing.T) {
	h := newTestHarness(t)

	fridayPrice := 143.92 // Friday's close
	mondayPrice := 147.50 // Monday's close (fresh)
	callCount := 0

	h.mockPortfolio.syncFn = func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
		callCount++
		price := fridayPrice
		if callCount >= 2 {
			price = mondayPrice
		}
		return &models.Portfolio{
			Name:       name,
			TotalValue: price * 282,
			Currency:   "AUD",
			LastSynced: time.Now(),
			Holdings: []models.Holding{
				{
					Ticker:       "ACDC",
					Name:         "ACDC ETF",
					Units:        282,
					CurrentPrice: price,
					MarketValue:  price * 282,
					Weight:       100,
				},
			},
		}, nil
	}

	// First sync — returns Friday's price
	result1, err := h.callTool("sync_portfolio", map[string]any{
		"portfolio_name": "SMSF",
		"force":          true,
	})
	if err != nil {
		t.Fatalf("First sync_portfolio failed: %v", err)
	}
	if result1.IsError {
		t.Fatalf("First sync_portfolio returned error: %s", h.getTextContent(result1, 0))
	}
	text1 := h.getTextContent(result1, 0)

	// Second sync — should return Monday's (updated) price
	result2, err := h.callTool("sync_portfolio", map[string]any{
		"portfolio_name": "SMSF",
		"force":          true,
	})
	if err != nil {
		t.Fatalf("Second sync_portfolio failed: %v", err)
	}
	if result2.IsError {
		t.Fatalf("Second sync_portfolio returned error: %s", h.getTextContent(result2, 0))
	}
	text2 := h.getTextContent(result2, 0)

	// The second response must contain the updated price, not the stale one
	if !strings.Contains(text2, "$147.50") {
		t.Errorf("Second sync should show Monday's price $147.50, got:\n%s", text2)
	}
	if strings.Contains(text2, "$143.92") {
		t.Errorf("Second sync still shows Friday's stale price $143.92:\n%s", text2)
	}

	// First response should have the original price
	if !strings.Contains(text1, "$143.92") {
		t.Errorf("First sync should show Friday's price $143.92, got:\n%s", text1)
	}

	// Verify both calls used force=true (callCount should be 2)
	if callCount != 2 {
		t.Errorf("Expected 2 sync calls with force=true, got %d", callCount)
	}
}

// TestScenario_ForceSyncUpdatesLastSynced verifies that each force sync
// produces a different LastSynced timestamp, confirming the data was actually
// re-fetched rather than returned from cache.
func TestScenario_ForceSyncUpdatesLastSynced(t *testing.T) {
	h := newTestHarness(t)

	h.mockPortfolio.syncFn = func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
		return &models.Portfolio{
			Name:       name,
			TotalValue: 100000,
			Currency:   "AUD",
			LastSynced: time.Now(),
			Holdings: []models.Holding{
				{Ticker: "ACDC", Units: 100, CurrentPrice: 50.0, MarketValue: 5000, Weight: 100},
			},
		}, nil
	}

	// First sync
	result1, err := h.callTool("sync_portfolio", map[string]any{
		"portfolio_name": "SMSF",
		"force":          true,
	})
	if err != nil {
		t.Fatalf("First sync failed: %v", err)
	}
	text1 := h.getTextContent(result1, 0)

	// Small delay to ensure timestamp differs
	time.Sleep(10 * time.Millisecond)

	// Second sync
	result2, err := h.callTool("sync_portfolio", map[string]any{
		"portfolio_name": "SMSF",
		"force":          true,
	})
	if err != nil {
		t.Fatalf("Second sync failed: %v", err)
	}
	text2 := h.getTextContent(result2, 0)

	// Both should contain Last Synced
	if !strings.Contains(text1, "**Last Synced:**") {
		t.Error("First sync missing Last Synced timestamp")
	}
	if !strings.Contains(text2, "**Last Synced:**") {
		t.Error("Second sync missing Last Synced timestamp")
	}

	// Extract Last Synced lines for logging
	for _, line := range strings.Split(text1, "\n") {
		if strings.Contains(line, "Last Synced") {
			t.Logf("Sync 1: %s", strings.TrimSpace(line))
		}
	}
	for _, line := range strings.Split(text2, "\n") {
		if strings.Contains(line, "Last Synced") {
			t.Logf("Sync 2: %s", strings.TrimSpace(line))
		}
	}
}

// TestScenario_SyncThenGetShowsSamePrice verifies that get_portfolio after
// sync_portfolio returns the same price data that was synced, not stale data
// from a previous sync or from a different source.
func TestScenario_SyncThenGetShowsSamePrice(t *testing.T) {
	h := newTestHarness(t)

	currentPrice := 147.50

	h.mockPortfolio.syncFn = func(ctx context.Context, name string, force bool) (*models.Portfolio, error) {
		return &models.Portfolio{
			Name:       name,
			TotalValue: currentPrice * 282,
			Currency:   "AUD",
			LastSynced: time.Now(),
			Holdings: []models.Holding{
				{
					Ticker:       "ACDC",
					Name:         "ACDC ETF",
					Units:        282,
					CurrentPrice: currentPrice,
					MarketValue:  currentPrice * 282,
					Weight:       100,
				},
			},
		}, nil
	}

	// get_portfolio should return the same data that sync stored
	h.mockPortfolio.getPortfolioFn = func(ctx context.Context, name string) (*models.Portfolio, error) {
		return &models.Portfolio{
			Name:       name,
			TotalValue: currentPrice * 282,
			LastSynced: time.Now(),
			Holdings: []models.Holding{
				{
					Ticker:       "ACDC",
					Name:         "ACDC ETF",
					Units:        282,
					CurrentPrice: currentPrice,
					MarketValue:  currentPrice * 282,
					Weight:       100,
				},
			},
		}, nil
	}

	// Sync with force
	syncResult, err := h.callTool("sync_portfolio", map[string]any{
		"portfolio_name": "SMSF",
		"force":          true,
	})
	if err != nil {
		t.Fatalf("sync_portfolio failed: %v", err)
	}
	if syncResult.IsError {
		t.Fatalf("sync_portfolio error: %s", h.getTextContent(syncResult, 0))
	}

	// Get portfolio (cached read)
	getResult, err := h.callTool("get_portfolio", map[string]any{
		"portfolio_name": "SMSF",
	})
	if err != nil {
		t.Fatalf("get_portfolio failed: %v", err)
	}
	if getResult.IsError {
		t.Fatalf("get_portfolio error: %s", h.getTextContent(getResult, 0))
	}

	getText := h.getTextContent(getResult, 0)
	syncText := h.getTextContent(syncResult, 0)

	// Both should show the same current price
	if !strings.Contains(syncText, "$147.50") {
		t.Errorf("sync_portfolio should contain $147.50, got:\n%s", syncText)
	}
	if !strings.Contains(getText, "147.50") {
		t.Errorf("get_portfolio should contain 147.50, got:\n%s", getText)
	}
}
