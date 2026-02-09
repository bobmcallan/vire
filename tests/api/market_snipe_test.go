package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/tests/common"
)

func TestMarketSnipe(t *testing.T) {
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

	// Call market_snipe tool
	snipeResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "market_snipe",
		"arguments": map[string]interface{}{
			"exchange": "AU",
		},
	})
	if err != nil {
		t.Logf("market_snipe call failed: %v", err)
		guard.SaveResult("02_market_snipe_error", err.Error())
	} else {
		guard.SaveResult("02_market_snipe_response", common.FormatJSON(snipeResult))
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
