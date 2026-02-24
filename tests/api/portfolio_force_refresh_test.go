package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/tests/common"
)

// TestPortfolioForceRefresh_TriggersSync verifies that calling GET /api/portfolios/{name}
// with force_refresh=true triggers a Navexa sync, advancing last_synced beyond the
// initial sync timestamp.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioForceRefresh_TriggersSync(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Phase 1: Initial sync to populate cached data
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_initial_sync", string(syncBody))

	// Phase 2: Get portfolio without force_refresh to capture baseline last_synced
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "baseline get_portfolio failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "baseline get_portfolio: %s", string(body))

	guard.SaveResult("02_baseline_get_portfolio", string(body))

	var baseline models.Portfolio
	require.NoError(t, json.Unmarshal(body, &baseline))
	require.False(t, baseline.LastSynced.IsZero(), "baseline last_synced should not be zero")

	baselineLastSynced := baseline.LastSynced
	t.Logf("Baseline last_synced: %s", baselineLastSynced)

	// Brief pause to ensure timestamps differ
	time.Sleep(2 * time.Second)

	// Phase 3: Get portfolio WITH force_refresh=true
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"?force_refresh=true", nil, headers)
	require.NoError(t, err, "force_refresh get_portfolio failed")
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	guard.SaveResult("03_force_refresh_response", string(body2))

	require.Equal(t, http.StatusOK, resp2.StatusCode,
		"force_refresh should return 200: %s", string(body2))

	var refreshed models.Portfolio
	require.NoError(t, json.Unmarshal(body2, &refreshed))

	t.Run("last_synced_advances", func(t *testing.T) {
		assert.True(t, refreshed.LastSynced.After(baselineLastSynced),
			"last_synced should advance after force_refresh (baseline=%s, refreshed=%s)",
			baselineLastSynced, refreshed.LastSynced)
	})

	t.Run("response_has_holdings", func(t *testing.T) {
		assert.NotEmpty(t, refreshed.Holdings,
			"force_refresh response should contain holdings")
	})

	t.Run("response_has_name", func(t *testing.T) {
		assert.Equal(t, portfolio, refreshed.Name,
			"portfolio name should match request")
	})

	t.Run("trades_stripped", func(t *testing.T) {
		for _, h := range refreshed.Holdings {
			assert.Empty(t, h.Trades,
				"holding %s trades should be stripped in portfolio GET", h.Ticker)
		}
	})

	t.Logf("Baseline last_synced=%s, Refreshed last_synced=%s",
		baselineLastSynced, refreshed.LastSynced)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioForceRefresh_CachedWithoutFlag verifies that calling
// GET /api/portfolios/{name} without force_refresh returns cached data
// quickly (the fast path), and that last_synced does NOT advance.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioForceRefresh_CachedWithoutFlag(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate cached data
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// First GET (cached read)
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "first get_portfolio failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "first get_portfolio: %s", string(body))

	guard.SaveResult("02_first_get", string(body))

	var first models.Portfolio
	require.NoError(t, json.Unmarshal(body, &first))
	firstLastSynced := first.LastSynced

	// Brief pause
	time.Sleep(2 * time.Second)

	// Second GET without force_refresh (should return cached data)
	start := time.Now()
	resp2, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	elapsed := time.Since(start)
	require.NoError(t, err, "second get_portfolio failed")
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "second get_portfolio: %s", string(body2))

	guard.SaveResult("03_second_get_cached", string(body2))

	var second models.Portfolio
	require.NoError(t, json.Unmarshal(body2, &second))

	t.Run("last_synced_unchanged", func(t *testing.T) {
		assert.Equal(t, firstLastSynced, second.LastSynced,
			"last_synced should not change without force_refresh")
	})

	t.Run("cached_read_is_fast", func(t *testing.T) {
		assert.Less(t, elapsed, 5*time.Second,
			"cached read should complete quickly (got %v)", elapsed)
	})

	t.Run("data_matches_first_read", func(t *testing.T) {
		assert.Equal(t, first.Name, second.Name)
		assert.Equal(t, len(first.Holdings), len(second.Holdings),
			"holding count should match between reads")
	})

	t.Logf("First last_synced=%s, Second last_synced=%s, elapsed=%v",
		firstLastSynced, second.LastSynced, elapsed)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioForceRefresh_NoNavexa verifies that calling get_portfolio with
// force_refresh=true but without Navexa credentials returns an error
// rather than crashing.
func TestPortfolioForceRefresh_NoNavexa(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Call force_refresh without any user or Navexa configuration
	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/SMSF?force_refresh=true",
		nil, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_force_refresh_no_navexa", string(body))

	t.Run("returns_error_status", func(t *testing.T) {
		assert.NotEqual(t, http.StatusOK, resp.StatusCode,
			"force_refresh without Navexa should not succeed")
	})

	t.Run("response_is_valid_json", func(t *testing.T) {
		assert.True(t, json.Valid(body),
			"error response should be valid JSON: %s", string(body))
	})

	t.Logf("Expected error without Navexa: status=%d body=%s",
		resp.StatusCode, string(body))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioForceRefresh_FalseValue verifies that force_refresh=false
// behaves identically to omitting the parameter (cached read).
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioForceRefresh_FalseValue(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync to populate cache
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get with force_refresh=false (should be cached read)
	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"?force_refresh=false", nil, headers)
	require.NoError(t, err, "get_portfolio with force_refresh=false failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("02_force_refresh_false", string(body))

	require.Equal(t, http.StatusOK, resp.StatusCode,
		"force_refresh=false should return 200: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got))

	t.Run("response_has_holdings", func(t *testing.T) {
		assert.NotEmpty(t, got.Holdings,
			"force_refresh=false should return cached holdings")
	})

	t.Run("response_has_name", func(t *testing.T) {
		assert.Equal(t, portfolio, got.Name)
	})

	t.Logf("force_refresh=false: holdings=%d last_synced=%s",
		len(got.Holdings), got.LastSynced)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}
