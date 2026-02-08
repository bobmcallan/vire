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

	// Verify container is healthy
	resp, body, err := env.Get("/health")
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, string(body), "ok")

	guard := common.NewTestOutputGuard(t)
	guard.SaveResult("health_response", string(body))
}
