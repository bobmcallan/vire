package api

import (
	"testing"

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
