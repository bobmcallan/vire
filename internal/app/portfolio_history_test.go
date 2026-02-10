package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

func TestPortfolioHistory_AutoDaily(t *testing.T) {
	h := newTestHarness(t)
	points := generateTestGrowthPoints(30) // <=90 → daily
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return points, nil
	}

	result, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if len(result.Content) != 2 {
		t.Fatalf("Expected 2 content blocks, got %d", len(result.Content))
	}

	markdown := h.getTextContent(result, 0)
	if !strings.Contains(markdown, "daily") {
		t.Errorf("Expected markdown to mention 'daily' granularity, got:\n%s", markdown[:200])
	}
	if !strings.Contains(markdown, "30 data points") {
		t.Errorf("Expected markdown to mention '30 data points'")
	}
	// Daily table has "Day Change" column
	if !strings.Contains(markdown, "Day Change") {
		t.Errorf("Expected daily table with 'Day Change' column")
	}
}

func TestPortfolioHistory_AutoWeekly(t *testing.T) {
	h := newTestHarness(t)
	points := generateTestGrowthPoints(120) // >90 → weekly
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return points, nil
	}

	result, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if len(result.Content) != 2 {
		t.Fatalf("Expected 2 content blocks, got %d", len(result.Content))
	}

	markdown := h.getTextContent(result, 0)
	if !strings.Contains(markdown, "weekly") {
		t.Errorf("Expected markdown to mention 'weekly' granularity")
	}
	// Weekly table has "Week Change" column
	if !strings.Contains(markdown, "Week Change") {
		t.Errorf("Expected weekly table with 'Week Change' column")
	}

	// JSON should have fewer points than original 120
	jsonStr := h.getTextContent(result, 1)
	if !strings.HasPrefix(jsonStr, "<!-- CHART_DATA -->") {
		t.Fatalf("Missing CHART_DATA prefix in JSON block")
	}
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var jsonPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &jsonPoints); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(jsonPoints) >= 120 {
		t.Errorf("Expected weekly downsampling to reduce points, got %d", len(jsonPoints))
	}
}

func TestPortfolioHistory_FormatDaily(t *testing.T) {
	h := newTestHarness(t)
	points := generateTestGrowthPoints(120) // Would be weekly in auto, but we force daily
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return points, nil
	}

	result, err := h.callTool("get_portfolio_history", map[string]any{"format": "daily"})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	markdown := h.getTextContent(result, 0)
	if !strings.Contains(markdown, "daily") {
		t.Errorf("Expected markdown to mention 'daily'")
	}

	// JSON should have all 120 points (no downsampling)
	jsonStr := h.getTextContent(result, 1)
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var jsonPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &jsonPoints); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(jsonPoints) != 120 {
		t.Errorf("Expected 120 daily points, got %d", len(jsonPoints))
	}
}

func TestPortfolioHistory_FormatWeekly(t *testing.T) {
	h := newTestHarness(t)
	points := generateTestGrowthPoints(30) // Would be daily in auto, but we force weekly
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return points, nil
	}

	result, err := h.callTool("get_portfolio_history", map[string]any{"format": "weekly"})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	markdown := h.getTextContent(result, 0)
	if !strings.Contains(markdown, "weekly") {
		t.Errorf("Expected markdown to mention 'weekly'")
	}
	if !strings.Contains(markdown, "Week Change") {
		t.Errorf("Expected weekly table with 'Week Change' column")
	}

	// JSON should have fewer points than original 30
	jsonStr := h.getTextContent(result, 1)
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var jsonPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &jsonPoints); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(jsonPoints) >= 30 {
		t.Errorf("Expected weekly downsampling to reduce from 30 points, got %d", len(jsonPoints))
	}
}

func TestPortfolioHistory_FormatMonthly(t *testing.T) {
	h := newTestHarness(t)
	points := generateTestGrowthPoints(120) // ~4 months of data
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return points, nil
	}

	result, err := h.callTool("get_portfolio_history", map[string]any{"format": "monthly"})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	markdown := h.getTextContent(result, 0)
	if !strings.Contains(markdown, "monthly") {
		t.Errorf("Expected markdown to mention 'monthly'")
	}
	if !strings.Contains(markdown, "Month Change") {
		t.Errorf("Expected monthly table with 'Month Change' column")
	}

	// JSON should have ~4-5 points (one per month)
	jsonStr := h.getTextContent(result, 1)
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var jsonPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &jsonPoints); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(jsonPoints) > 6 || len(jsonPoints) < 3 {
		t.Errorf("Expected 3-6 monthly points from 120 days, got %d", len(jsonPoints))
	}
}

func TestPortfolioHistory_InvalidFormat(t *testing.T) {
	h := newTestHarness(t)
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(10), nil
	}

	result, err := h.callTool("get_portfolio_history", map[string]any{"format": "biweekly"})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if !result.IsError {
		t.Errorf("Expected error result for invalid format")
	}
	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "invalid format") {
		t.Errorf("Expected error message about invalid format, got: %s", text)
	}
}

func TestPortfolioHistory_JSONParseable(t *testing.T) {
	h := newTestHarness(t)
	points := generateTestGrowthPoints(10)
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return points, nil
	}

	result, err := h.callTool("get_portfolio_history", map[string]any{"format": "daily"})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	jsonStr := h.getTextContent(result, 1)

	// Verify CHART_DATA prefix
	if !strings.HasPrefix(jsonStr, "<!-- CHART_DATA -->") {
		t.Errorf("Expected JSON block to start with <!-- CHART_DATA -->, got: %s", jsonStr[:50])
	}

	// Parse JSON
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var jsonPoints []struct {
		Date     string  `json:"date"`
		Value    float64 `json:"value"`
		Cost     float64 `json:"cost"`
		Gain     float64 `json:"gain"`
		GainPct  float64 `json:"gain_pct"`
		Holdings int     `json:"holding_count"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &jsonPoints); err != nil {
		t.Fatalf("Failed to parse JSON data: %v", err)
	}

	if len(jsonPoints) != 10 {
		t.Fatalf("Expected 10 JSON points, got %d", len(jsonPoints))
	}

	// Verify fields match source data
	first := jsonPoints[0]
	if first.Date != "2025-01-01" {
		t.Errorf("Expected first date '2025-01-01', got '%s'", first.Date)
	}
	if first.Value != 100000 {
		t.Errorf("Expected first value 100000, got %f", first.Value)
	}
	if first.Cost != 95000 {
		t.Errorf("Expected first cost 95000, got %f", first.Cost)
	}
	if first.Gain != 5000 {
		t.Errorf("Expected first gain 5000, got %f", first.Gain)
	}
	if first.Holdings != 12 {
		t.Errorf("Expected first holding_count 12, got %d", first.Holdings)
	}

	// Verify last point
	last := jsonPoints[9]
	if last.Date != "2025-01-10" {
		t.Errorf("Expected last date '2025-01-10', got '%s'", last.Date)
	}
}

func TestPortfolioHistory_TwoContentBlocks(t *testing.T) {
	h := newTestHarness(t)
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(5), nil
	}

	result, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if len(result.Content) != 2 {
		t.Fatalf("Expected exactly 2 content blocks, got %d", len(result.Content))
	}

	// Block 0: markdown (starts with "# Portfolio History")
	md := h.getTextContent(result, 0)
	if !strings.HasPrefix(md, "# Portfolio History") {
		t.Errorf("Content[0] should be markdown starting with '# Portfolio History', got: %s", md[:50])
	}

	// Block 1: JSON (starts with CHART_DATA marker)
	js := h.getTextContent(result, 1)
	if !strings.HasPrefix(js, "<!-- CHART_DATA -->") {
		t.Errorf("Content[1] should start with '<!-- CHART_DATA -->', got: %s", js[:50])
	}
}

func TestPortfolioHistory_EmptyRange(t *testing.T) {
	h := newTestHarness(t)
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return nil, nil // no data
	}

	result, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// Should return single text content with "no data" message
	if len(result.Content) != 1 {
		t.Fatalf("Expected 1 content block for empty range, got %d", len(result.Content))
	}
	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "No portfolio history data") {
		t.Errorf("Expected 'No portfolio history data' message, got: %s", text)
	}
}

func TestPortfolioHistory_DateParams(t *testing.T) {
	h := newTestHarness(t)

	var capturedOpts interfaces.GrowthOptions
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		capturedOpts = opts
		return generateTestGrowthPoints(5), nil
	}

	_, err := h.callTool("get_portfolio_history", map[string]any{
		"from": "2025-06-01",
		"to":   "2025-06-30",
	})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if capturedOpts.From.Format("2006-01-02") != "2025-06-01" {
		t.Errorf("Expected from '2025-06-01', got '%s'", capturedOpts.From.Format("2006-01-02"))
	}
	if capturedOpts.To.Format("2006-01-02") != "2025-06-30" {
		t.Errorf("Expected to '2025-06-30', got '%s'", capturedOpts.To.Format("2006-01-02"))
	}
}

func TestPortfolioHistory_InvalidFromDate(t *testing.T) {
	h := newTestHarness(t)

	result, err := h.callTool("get_portfolio_history", map[string]any{
		"from": "not-a-date",
	})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if !result.IsError {
		t.Errorf("Expected error for invalid date")
	}
	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "invalid from date") {
		t.Errorf("Expected 'invalid from date' error, got: %s", text)
	}
}

func TestPortfolioHistory_InvalidToDate(t *testing.T) {
	h := newTestHarness(t)

	result, err := h.callTool("get_portfolio_history", map[string]any{
		"to": "31/12/2025",
	})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if !result.IsError {
		t.Errorf("Expected error for invalid to date")
	}
	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "invalid to date") {
		t.Errorf("Expected 'invalid to date' error, got: %s", text)
	}
}

func TestPortfolioHistory_ServiceError(t *testing.T) {
	h := newTestHarness(t)
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return nil, fmt.Errorf("database connection lost")
	}

	result, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if !result.IsError {
		t.Errorf("Expected error result for service failure")
	}
	text := h.getTextContent(result, 0)
	if !strings.Contains(text, "History error") {
		t.Errorf("Expected 'History error' message, got: %s", text)
	}
	if !strings.Contains(text, "database connection lost") {
		t.Errorf("Expected error details in message, got: %s", text)
	}
}

func TestPortfolioHistory_SingleDataPoint(t *testing.T) {
	h := newTestHarness(t)
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(1), nil
	}

	result, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if len(result.Content) != 2 {
		t.Fatalf("Expected 2 content blocks, got %d", len(result.Content))
	}

	// JSON should have exactly 1 point
	jsonStr := h.getTextContent(result, 1)
	if !strings.HasPrefix(jsonStr, "<!-- CHART_DATA -->") {
		t.Fatalf("Missing CHART_DATA marker")
	}
	rawJSON := strings.TrimPrefix(jsonStr, "<!-- CHART_DATA -->\n")
	var jsonPoints []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &jsonPoints); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if len(jsonPoints) != 1 {
		t.Errorf("Expected 1 JSON point, got %d", len(jsonPoints))
	}
}

func TestPortfolioHistory_PortfolioNameResolution(t *testing.T) {
	h := newTestHarness(t)

	var capturedName string
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		capturedName = name
		return generateTestGrowthPoints(5), nil
	}

	// Call without portfolio_name — should use the default "TestSMSF"
	_, err := h.callTool("get_portfolio_history", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if capturedName != "TestSMSF" {
		t.Errorf("Expected portfolio name 'TestSMSF' (default), got '%s'", capturedName)
	}

	// Call with explicit portfolio_name
	_, err = h.callTool("get_portfolio_history", map[string]any{
		"portfolio_name": "Personal",
	})
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	if capturedName != "Personal" {
		t.Errorf("Expected portfolio name 'Personal', got '%s'", capturedName)
	}
}
