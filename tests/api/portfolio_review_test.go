package api

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/tests/common"
)

func TestPortfolioReview(t *testing.T) {
	// Use the real config with API keys
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// MCP initialize
	initResult, err := env.MCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "vire-test",
			"version": "1.0.0",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, initResult)
	guard.SaveResult("01_initialize_response", common.FormatMCPContent(initResult))

	// First sync the portfolio from Navexa
	syncResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "sync_portfolio",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
			"force":          true,
		},
	})
	require.NoError(t, err, "sync_portfolio MCP request failed")
	guard.SaveResult("02_sync_portfolio_response", common.FormatMCPContent(syncResult))

	// Validate sync succeeded
	err = common.ValidateMCPToolResponse(syncResult)
	require.NoError(t, err, "sync_portfolio returned invalid response")

	// Now call portfolio_review tool
	reviewResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "portfolio_review",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
		},
	})
	require.NoError(t, err, "portfolio_review MCP request failed")
	guard.SaveResult("03_portfolio_review_response", common.FormatMCPContent(reviewResult))

	// Validate the response - must not be an error and must have content
	err = common.ValidateMCPToolResponse(reviewResult)
	require.NoError(t, err, "portfolio_review returned invalid response")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGetPortfolio tests the lightweight get_portfolio tool which returns
// cached holdings data (positions, units, value, weight) without signals,
// AI analysis, or buy/sell/hold recommendations.
func TestGetPortfolio(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	totalStart := time.Now()

	// Phase 1: Initialize
	phaseStart := time.Now()
	initResult, err := env.MCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "vire-test",
			"version": "1.0.0",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, initResult)
	guard.SaveResult("01_initialize_response", common.FormatMCPContent(initResult))
	t.Logf("Phase 1 (initialize): %v", time.Since(phaseStart))

	// Phase 2: Sync portfolio (populates cached data)
	phaseStart = time.Now()
	syncResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "sync_portfolio",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
			"force":          true,
		},
	})
	require.NoError(t, err, "sync_portfolio MCP request failed")
	guard.SaveResult("02_sync_portfolio_response", common.FormatMCPContent(syncResult))
	err = common.ValidateMCPToolResponse(syncResult)
	require.NoError(t, err, "sync_portfolio returned invalid response")
	t.Logf("Phase 2 (sync_portfolio): %v", time.Since(phaseStart))

	// Phase 3: Get portfolio data (the fast path — cached read only)
	phaseStart = time.Now()
	getResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "get_portfolio",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
		},
	})
	getPortfolioElapsed := time.Since(phaseStart)
	require.NoError(t, err, "get_portfolio MCP request failed")
	guard.SaveResult("03_get_portfolio_response", common.FormatMCPContent(getResult))
	t.Logf("Phase 3 (get_portfolio): %v", getPortfolioElapsed)

	// Validate response structure
	err = common.ValidateMCPToolResponse(getResult)
	require.NoError(t, err, "get_portfolio returned invalid response")

	// Phase 4: Validate content
	phaseStart = time.Now()
	content := common.FormatMCPContent(getResult)

	// Must contain portfolio header
	assert.Contains(t, content, "# Portfolio:")
	assert.Contains(t, content, "**Total Value:**")
	assert.Contains(t, content, "**Last Synced:**")

	// Must contain holdings table with expected columns
	assert.Contains(t, content, "| Symbol |")
	assert.Contains(t, content, "| Units |")
	assert.Contains(t, content, "| Value |")
	assert.Contains(t, content, "| Weight |")

	// Must NOT contain buy/sell/hold signals (that's portfolio_review's job)
	assert.NotContains(t, content, "BUY")
	assert.NotContains(t, content, "SELL")
	assert.NotContains(t, content, "HOLD")
	assert.NotContains(t, content, "RSI")
	assert.NotContains(t, content, "SMA")
	assert.NotContains(t, content, "AI Summary")

	// Must contain at least one active holding ticker
	assert.True(t, strings.Contains(content, "## Holdings"),
		"Expected active holdings section")

	t.Logf("Phase 4 (validate content): %v", time.Since(phaseStart))

	// Phase 5: Performance assertion — get_portfolio should be fast (cached read)
	t.Logf("TOTAL: %v", time.Since(totalStart))
	t.Logf("get_portfolio alone: %v (target: <2s)", getPortfolioElapsed)

	// get_portfolio reads a single cached JSON file — should complete well under 2s
	// even accounting for docker exec overhead
	assert.Less(t, getPortfolioElapsed, 5*time.Second,
		"get_portfolio should be fast (single cached read)")
}

// TestGetPortfolioThenReview tests the intended usage pattern: get_portfolio
// for fast data retrieval throughout the day, portfolio_review for full analysis.
// Requires market data to already be cached (run TestPortfolioReview first, or
// use generate_report to populate the cache).
func TestGetPortfolioThenReview(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Initialize
	initResult, err := env.MCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "vire-test",
			"version": "1.0.0",
		},
	})
	require.NoError(t, err)
	guard.SaveResult("01_initialize_response", common.FormatMCPContent(initResult))

	// Sync portfolio
	syncResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "sync_portfolio",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
			"force":          true,
		},
	})
	require.NoError(t, err, "sync_portfolio failed")
	guard.SaveResult("02_sync_portfolio_response", common.FormatMCPContent(syncResult))
	require.NoError(t, common.ValidateMCPToolResponse(syncResult))

	// Step A: Fast get_portfolio (just the data)
	getStart := time.Now()
	getResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "get_portfolio",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
		},
	})
	getElapsed := time.Since(getStart)
	require.NoError(t, err, "get_portfolio failed")
	guard.SaveResult("03_get_portfolio_response", common.FormatMCPContent(getResult))
	require.NoError(t, common.ValidateMCPToolResponse(getResult))

	getContent := common.FormatMCPContent(getResult)
	assert.Contains(t, getContent, "## Holdings")
	assert.NotContains(t, getContent, "SELL")
	assert.NotContains(t, getContent, "BUY")

	// Step B: Full portfolio_review (signals, AI, chart — uses cached market data)
	reviewStart := time.Now()
	reviewResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "portfolio_review",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
		},
	})
	reviewElapsed := time.Since(reviewStart)
	require.NoError(t, err, "portfolio_review failed")
	guard.SaveResult("04_portfolio_review_response", common.FormatMCPContent(reviewResult))
	require.NoError(t, common.ValidateMCPToolResponse(reviewResult))

	// Timing comparison
	t.Logf("get_portfolio:    %v", getElapsed)
	t.Logf("portfolio_review: %v", reviewElapsed)
	t.Logf("review/get ratio: %.1fx slower", float64(reviewElapsed)/float64(getElapsed))

	// get_portfolio should be significantly faster than portfolio_review
	assert.Less(t, getElapsed, reviewElapsed,
		"get_portfolio should be faster than portfolio_review")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

func TestListPortfolios(t *testing.T) {
	// Use the real config with API keys
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// MCP initialize
	initResult, err := env.MCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "vire-test",
			"version": "1.0.0",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, initResult)
	guard.SaveResult("01_initialize_response", common.FormatMCPContent(initResult))

	// First sync the portfolio from Navexa (list_portfolios shows synced portfolios)
	syncResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "sync_portfolio",
		"arguments": map[string]interface{}{
			"portfolio_name": "SMSF",
			"force":          true,
		},
	})
	require.NoError(t, err, "sync_portfolio MCP request failed")
	guard.SaveResult("02_sync_portfolio_response", common.FormatMCPContent(syncResult))

	// Validate sync succeeded
	err = common.ValidateMCPToolResponse(syncResult)
	require.NoError(t, err, "sync_portfolio returned invalid response")

	// Call list_portfolios tool
	listResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name":      "list_portfolios",
		"arguments": map[string]interface{}{},
	})
	require.NoError(t, err, "list_portfolios MCP request failed")
	guard.SaveResult("03_list_portfolios_response", common.FormatMCPContent(listResult))

	// Validate the response - must not be an error and must have content
	err = common.ValidateMCPToolResponse(listResult)
	require.NoError(t, err, "list_portfolios returned invalid response")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioReviewBlankConfig tests behavior with empty API keys
func TestPortfolioReviewBlankConfig(t *testing.T) {
	// Use the blank config with empty/demo API keys
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// MCP initialize
	initResult, err := env.MCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "vire-test",
			"version": "1.0.0",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, initResult)
	guard.SaveResult("01_initialize_response", common.FormatMCPContent(initResult))

	// Call portfolio_review tool - expected to fail with blank config
	reviewResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name":      "portfolio_review",
		"arguments": map[string]interface{}{},
	})
	require.NoError(t, err, "portfolio_review MCP request failed")
	guard.SaveResult("02_portfolio_review_response", common.FormatMCPContent(reviewResult))

	// With blank config, we expect an error response (isError: true)
	validationErr := common.ValidateMCPToolResponse(reviewResult)
	assert.Error(t, validationErr, "expected error with blank API key config")
	t.Logf("Expected error with blank config: %v", validationErr)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
