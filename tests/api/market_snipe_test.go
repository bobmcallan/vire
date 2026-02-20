package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

func TestMarketSnipe(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
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

	// Call strategy_scanner tool
	snipeResult, err := env.MCPRequest("tools/call", map[string]interface{}{
		"name": "strategy_scanner",
		"arguments": map[string]interface{}{
			"exchange": "AU",
		},
	})
	if err != nil {
		t.Logf("strategy_scanner call failed: %v", err)
		guard.SaveResult("02_strategy_scanner_error", err.Error())
	} else {
		guard.SaveResult("02_strategy_scanner_response", common.FormatMCPContent(snipeResult))
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
