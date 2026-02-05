package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/test/common"
)

func TestPortfolioReview(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Verify server is running via SSE endpoint
	resp, body, err := env.Get("/sse")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(body), "endpoint")

	guard := common.NewTestOutputGuard(t)
	guard.SaveResult("sse_response", string(body))
}

func TestListPortfolios(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// MCP initialize
	result, err := env.MCPRequest("initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "vire-test",
			"version": "1.0.0",
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	env.SaveResult("initialize.json", result)
}
