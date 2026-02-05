package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmccarthy/vire/test/common"
)

func TestMarketSnipe(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Verify container is healthy via SSE endpoint
	resp, body, err := env.Get("/sse")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(body), "endpoint")

	guard := common.NewTestOutputGuard(t)
	guard.SaveResult("sse_response", string(body))
}
