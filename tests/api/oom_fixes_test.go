package api

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestServerBootsWithOOMFixes verifies that the server starts successfully
// with the new config fields (watcher_startup_delay, heavy_job_limit).
// The OOM fixes add startup staggering and a heavy job semaphore, both of
// which initialize during server boot. This test confirms the server reaches
// healthy status with these changes.
func TestServerBootsWithOOMFixes(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Health check confirms server booted with new config fields
	resp, err := env.HTTPGet("/api/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_health_response.json", string(body))

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]string
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "ok", result["status"])
}

// TestPortfolioComplianceBlankConfig verifies that the portfolio compliance
// endpoint (which triggers signal computation and may load MarketData) returns
// a well-formed error when no portfolio data exists. This exercises the code
// path through memory-optimized MarketData loading introduced by the OOM fixes.
func TestPortfolioComplianceBlankConfig(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Request portfolio compliance without any configured portfolio.
	// This exercises the compliance code path including MarketData loading,
	// signal computation, and the memory optimizations.
	resp, err := env.HTTPRequest("POST", "/api/portfolios/TestPortfolio/review",
		map[string]interface{}{}, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "compliance request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_compliance_blank_response.json", string(body))

	// With blank config and no user, we expect a non-200 response
	assert.NotEqual(t, 200, resp.StatusCode,
		"expected error with no portfolio configured")

	// Response should be valid JSON regardless of error
	assert.True(t, json.Valid(body),
		"response should be valid JSON: %s", string(body))
}
