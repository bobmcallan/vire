package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

// newReviewHarness creates a test harness with the portfolio_review tool registered
// and mock services configured for chart file saving tests.
func newReviewHarness(t *testing.T) *testHarness {
	t.Helper()
	h := newTestHarness(t)

	// Set up a temp directory for chart file output
	h.mockStorage.dataPath = t.TempDir()

	// Create charts subdirectory (simulates what NewFileStore does)
	if err := os.MkdirAll(filepath.Join(h.mockStorage.dataPath, "charts"), 0755); err != nil {
		t.Fatalf("Failed to create charts dir: %v", err)
	}

	// Register portfolio_review tool with the harness's mock services
	defaultPortfolio := "TestSMSF"
	h.mcpServer.AddTool(
		createPortfolioReviewTool(),
		handlePortfolioReview(h.mockPortfolio, h.mockStorage, defaultPortfolio, h.logger),
	)

	return h
}

// TestPortfolioReview_ChartFileSaved verifies that when growth data exists,
// the handler saves a PNG file to the charts directory and returns its path.
func TestPortfolioReview_ChartFileSaved(t *testing.T) {
	h := newReviewHarness(t)

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(30), nil
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("portfolio_review returned error: %s", tryGetTextContent(result, 0))
	}

	// Verify chart file was saved to the data path
	chartPath := filepath.Join(h.mockStorage.dataPath, "charts", "testsmsf-growth.png")
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		t.Fatalf("Expected chart file at %s, but it does not exist", chartPath)
	}

	// Verify the file is a valid PNG (check magic bytes)
	data, err := os.ReadFile(chartPath)
	if err != nil {
		t.Fatalf("Failed to read chart file: %v", err)
	}
	assertValidPNG(t, data)

	// Verify a CHART_FILE marker is returned in a TextContent block
	chartMarker := findChartFileMarker(result)
	if chartMarker == "" {
		t.Fatal("Expected a TextContent block with <!-- CHART_FILE: --> marker, not found")
	}

	// Verify the path in the marker matches the actual file
	if !strings.Contains(chartMarker, chartPath) {
		t.Errorf("CHART_FILE marker does not contain expected path %s, got: %s", chartPath, chartMarker)
	}
}

// TestPortfolioReview_ChartFilePathFormat verifies the CHART_FILE marker
// is in the expected parseable format: <!-- CHART_FILE:/path/to/file.png -->
func TestPortfolioReview_ChartFilePathFormat(t *testing.T) {
	h := newReviewHarness(t)

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(10), nil
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	chartMarker := findChartFileMarker(result)
	if chartMarker == "" {
		t.Fatal("No CHART_FILE marker found in response")
	}

	// Verify format: <!-- CHART_FILE:/path/to/file.png -->
	if !strings.HasPrefix(chartMarker, "<!-- CHART_FILE:") {
		t.Errorf("Marker should start with '<!-- CHART_FILE:', got: %s", chartMarker)
	}
	if !strings.HasSuffix(strings.TrimSpace(chartMarker), "-->") {
		t.Errorf("Marker should end with '-->', got: %s", chartMarker)
	}

	// Extract the path and verify it ends with .png
	path := extractChartFilePath(chartMarker)
	if path == "" {
		t.Fatal("Failed to extract path from CHART_FILE marker")
	}
	if !strings.HasSuffix(path, ".png") {
		t.Errorf("Chart file path should end with .png, got: %s", path)
	}
	if !strings.Contains(path, "charts") {
		t.Errorf("Chart file path should contain 'charts' directory, got: %s", path)
	}
}

// TestPortfolioReview_NoGrowthData_NoChartFile verifies that when no growth data
// exists, no chart file is written and no chart path is returned.
func TestPortfolioReview_NoGrowthData_NoChartFile(t *testing.T) {
	h := newReviewHarness(t)

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return nil, nil // No growth data
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// Verify no chart file exists
	chartPath := filepath.Join(h.mockStorage.dataPath, "charts", "testsmsf-growth.png")
	if _, err := os.Stat(chartPath); !os.IsNotExist(err) {
		t.Errorf("Expected no chart file when there's no growth data, but file exists at %s", chartPath)
	}

	// Verify no CHART_FILE marker in response
	chartMarker := findChartFileMarker(result)
	if chartMarker != "" {
		t.Errorf("Expected no CHART_FILE marker with no growth data, got: %s", chartMarker)
	}
}

// TestPortfolioReview_SingleGrowthPoint_NoChart verifies that with only 1 data point
// (too few for a chart), no chart file is written.
func TestPortfolioReview_SingleGrowthPoint_NoChart(t *testing.T) {
	h := newReviewHarness(t)

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(1), nil // Only 1 point — too few for chart
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// RenderGrowthChart requires >= 2 points, so no chart should be written
	chartPath := filepath.Join(h.mockStorage.dataPath, "charts", "testsmsf-growth.png")
	if _, err := os.Stat(chartPath); !os.IsNotExist(err) {
		t.Errorf("Expected no chart file with only 1 growth point, but file exists at %s", chartPath)
	}

	// No CHART_FILE marker
	chartMarker := findChartFileMarker(result)
	if chartMarker != "" {
		t.Errorf("Expected no CHART_FILE marker with 1 growth point, got: %s", chartMarker)
	}
}

// TestPortfolioReview_ChartOverwritten verifies that running the review twice
// overwrites the same chart file rather than accumulating files.
func TestPortfolioReview_ChartOverwritten(t *testing.T) {
	h := newReviewHarness(t)

	callCount := 0
	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		callCount++
		// Return different amounts of data each time so chart sizes differ
		return generateTestGrowthPoints(10 + callCount*20), nil
	}

	// First call
	_, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("First callTool failed: %v", err)
	}

	chartPath := filepath.Join(h.mockStorage.dataPath, "charts", "testsmsf-growth.png")
	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		t.Fatalf("Chart file not found after first call")
	}

	// Second call — should overwrite same file
	_, err = h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("Second callTool failed: %v", err)
	}

	if _, err := os.Stat(chartPath); os.IsNotExist(err) {
		t.Fatalf("Chart file not found after second call")
	}

	// Verify only one chart file exists in the charts directory
	chartsDir := filepath.Join(h.mockStorage.dataPath, "charts")
	entries, err := os.ReadDir(chartsDir)
	if err != nil {
		t.Fatalf("Failed to read charts directory: %v", err)
	}

	pngCount := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".png") {
			pngCount++
		}
	}
	if pngCount != 1 {
		t.Errorf("Expected exactly 1 PNG file in charts directory after 2 reviews, got %d", pngCount)
	}
}

// TestPortfolioReview_ImageContentStillReturned verifies that the base64 ImageContent
// block is still included in the response for backward compatibility with Claude Desktop.
func TestPortfolioReview_ImageContentStillReturned(t *testing.T) {
	h := newReviewHarness(t)

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(30), nil
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// Verify there's still an ImageContent block (using mcp.AsImageContent)
	foundImage := false
	for _, c := range result.Content {
		if _, ok := mcp.AsImageContent(c); ok {
			foundImage = true
			break
		}
	}

	if !foundImage {
		t.Errorf("Expected an ImageContent block in the response for backward compatibility with Claude Desktop")
	}
}

// TestPortfolioReview_PathTraversal verifies that a malicious portfolio name
// like "../evil" is sanitized and does not escape the charts directory.
func TestPortfolioReview_PathTraversal(t *testing.T) {
	h := newReviewHarness(t)

	// Override the default portfolio name to include path traversal
	h.mcpServer.AddTool(
		createPortfolioReviewTool(),
		handlePortfolioReview(h.mockPortfolio, h.mockStorage, "../evil", h.logger),
	)

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(10), nil
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// Chart file must be inside the charts directory, not escaped
	chartMarker := findChartFileMarker(result)
	if chartMarker != "" {
		path := extractChartFilePath(chartMarker)
		chartsDir := filepath.Join(h.mockStorage.dataPath, "charts")
		if !strings.HasPrefix(path, chartsDir) {
			t.Errorf("Chart path escaped charts directory: %s (expected prefix %s)", path, chartsDir)
		}
		// Verify the resolved path is still within charts dir (no traversal via symlinks etc.)
		resolved, err := filepath.Abs(path)
		if err == nil {
			absChartsDir, _ := filepath.Abs(chartsDir)
			if !strings.HasPrefix(resolved, absChartsDir) {
				t.Errorf("Resolved chart path escaped charts directory: %s", resolved)
			}
		}
	}

	// Verify no file was written outside charts dir
	parentDir := filepath.Dir(filepath.Join(h.mockStorage.dataPath, "charts"))
	entries, _ := os.ReadDir(parentDir)
	for _, e := range entries {
		if strings.Contains(e.Name(), "evil") {
			t.Errorf("Found escaped file in parent directory: %s", e.Name())
		}
	}
}

// TestPortfolioReview_WriteFail_GracefulDegradation verifies that when chart
// file saving fails, the handler still returns the review without error.
func TestPortfolioReview_WriteFail_GracefulDegradation(t *testing.T) {
	h := newReviewHarness(t)

	// Remove the charts directory to force a write failure
	chartsDir := filepath.Join(h.mockStorage.dataPath, "charts")
	os.RemoveAll(chartsDir)
	// Create a file at "charts" path so MkdirAll/WriteFile fails
	os.WriteFile(chartsDir, []byte("blocker"), 0444)
	t.Cleanup(func() { os.Remove(chartsDir) })

	h.mockPortfolio.reviewFn = func(ctx context.Context, name string, opts interfaces.ReviewOptions) (*models.PortfolioReview, error) {
		return &models.PortfolioReview{PortfolioName: name}, nil
	}
	h.mockPortfolio.dailyGrowthFn = func(ctx context.Context, name string, opts interfaces.GrowthOptions) ([]models.GrowthDataPoint, error) {
		return generateTestGrowthPoints(10), nil
	}

	result, err := h.callTool("portfolio_review", nil)
	if err != nil {
		t.Fatalf("callTool failed: %v", err)
	}

	// Handler should NOT return an error — chart failure is non-fatal
	if result.IsError {
		t.Fatalf("Expected non-error result even when chart save fails, got error: %s", tryGetTextContent(result, 0))
	}

	// Should still have the review markdown (TextContent[0])
	text := tryGetTextContent(result, 0)
	if text == "" {
		t.Error("Expected review markdown in TextContent[0] even when chart save fails")
	}

	// ImageContent should still be present (chart rendering succeeded, only file save failed)
	foundImage := false
	for _, c := range result.Content {
		if _, ok := mcp.AsImageContent(c); ok {
			foundImage = true
			break
		}
	}
	if !foundImage {
		t.Error("Expected ImageContent even when chart file save fails")
	}

	// No CHART_FILE marker since the save failed
	chartMarker := findChartFileMarker(result)
	if chartMarker != "" {
		t.Errorf("Expected no CHART_FILE marker when save fails, got: %s", chartMarker)
	}
}

// --- Helper functions ---

// tryGetTextContent attempts to extract text from a content block, returning empty string if not text.
func tryGetTextContent(result *mcp.CallToolResult, index int) string {
	if index >= len(result.Content) {
		return ""
	}
	tc, ok := result.Content[index].(mcp.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

// findChartFileMarker searches all TextContent blocks for the CHART_FILE marker.
func findChartFileMarker(result *mcp.CallToolResult) string {
	for i := 0; i < len(result.Content); i++ {
		text := tryGetTextContent(result, i)
		if text != "" && strings.Contains(text, "<!-- CHART_FILE:") {
			return text
		}
	}
	return ""
}

// extractChartFilePath extracts the file path from a <!-- CHART_FILE:/path/to/file.png --> marker.
func extractChartFilePath(marker string) string {
	start := strings.Index(marker, "<!-- CHART_FILE:")
	if start == -1 {
		return ""
	}
	start += len("<!-- CHART_FILE:")
	end := strings.Index(marker[start:], " -->")
	if end == -1 {
		return ""
	}
	return marker[start : start+end]
}

// assertValidPNG checks that the given data starts with the PNG magic bytes.
func assertValidPNG(t *testing.T, data []byte) {
	t.Helper()
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < 8 {
		t.Fatalf("PNG data too short: %d bytes", len(data))
	}
	for i, b := range pngHeader {
		if data[i] != b {
			t.Fatalf("byte %d: got 0x%02X, want 0x%02X (not a valid PNG)", i, data[i], b)
		}
	}
}
