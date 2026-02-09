package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/tests/common"
)

func TestPortfolioReview(t *testing.T) {
	env := common.NewEnv(t)
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
	guard.SaveResult("01_initialize_response", common.FormatJSON(initResult))

	// Call portfolio_review tool
	reviewResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name":      "portfolio_review",
		"arguments": map[string]interface{}{},
	})
	if err != nil {
		t.Logf("portfolio_review call failed (expected if no portfolio data): %v", err)
		guard.SaveResult("02_portfolio_review_error", err.Error())
	} else {
		guard.SaveResult("02_portfolio_review_response", common.FormatJSON(reviewResult))
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

func TestListPortfolios(t *testing.T) {
	env := common.NewEnv(t)
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
	guard.SaveResult("01_initialize_response", common.FormatJSON(initResult))

	// Call list_portfolios tool
	listResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name":      "list_portfolios",
		"arguments": map[string]interface{}{},
	})
	if err != nil {
		t.Logf("list_portfolios call failed: %v", err)
		guard.SaveResult("02_list_portfolios_error", err.Error())
	} else {
		guard.SaveResult("02_list_portfolios_response", common.FormatJSON(listResult))
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
