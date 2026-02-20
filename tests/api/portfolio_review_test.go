package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// setupPortfolioEnv creates a test environment with a user and Navexa key configured.
// Returns the env, user headers map, and the default portfolio name.
// Skips the test if NAVEXA_API_KEY or DEFAULT_PORTFOLIO are not set.
func setupPortfolioEnv(t *testing.T, opts common.EnvOptions) (*common.Env, map[string]string, string) {
	t.Helper()

	common.LoadTestSecrets()

	navexaKey := os.Getenv("NAVEXA_API_KEY")
	if navexaKey == "" {
		t.Skip("NAVEXA_API_KEY not set")
	}
	defaultPortfolio := os.Getenv("DEFAULT_PORTFOLIO")
	if defaultPortfolio == "" {
		t.Skip("DEFAULT_PORTFOLIO not set")
	}

	env := common.NewEnvWithOptions(t, opts)
	if env == nil {
		t.SkipNow()
	}

	userHeaders := map[string]string{"X-Vire-User-ID": "dev_user"}

	// Import users
	usersPath := filepath.Join(common.FindProjectRoot(), "tests", "fixtures", "users.json")
	data, err := os.ReadFile(usersPath)
	require.NoError(t, err)

	var usersFile struct {
		Users []json.RawMessage `json:"users"`
	}
	require.NoError(t, json.Unmarshal(data, &usersFile))

	for _, userRaw := range usersFile.Users {
		resp, err := env.HTTPPost("/api/users/upsert", json.RawMessage(userRaw))
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Set Navexa key
	resp, err := env.HTTPPut("/api/users/dev_user", map[string]string{
		"navexa_key": navexaKey,
	})
	require.NoError(t, err)
	resp.Body.Close()

	return env, userHeaders, defaultPortfolio
}

// syncPortfolio syncs a portfolio via POST /api/portfolios/{name}/sync and returns the response body.
func syncPortfolio(t *testing.T, env *common.Env, name string, headers map[string]string) []byte {
	t.Helper()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+name+"/sync",
		map[string]interface{}{"force": true}, headers)
	require.NoError(t, err, "sync request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))

	return body
}

func TestPortfolioReview(t *testing.T) {
	t.Skip("Portfolio review loads full financial data (filings, signals, charts) — deferred to later development")

	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	// Get portfolio review/compliance
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolio+"/review",
		map[string]interface{}{}, headers)
	require.NoError(t, err, "review request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "review failed: %s", string(body))

	// Validate response has expected fields
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Contains(t, result, "review", "response should contain review field")

	guard.SaveResult("02_review_response.json", string(body))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

func TestGetPortfolio(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()
	totalStart := time.Now()

	// Phase 1: Sync portfolio (populates cached data)
	syncStart := time.Now()
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))
	t.Logf("Phase 1 (sync): %v", time.Since(syncStart))

	// Phase 2: Get portfolio data (the fast path — cached read only)
	getStart := time.Now()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	getElapsed := time.Since(getStart)
	require.NoError(t, err, "get_portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get_portfolio failed: %s", string(body))

	guard.SaveResult("02_get_portfolio_response.json", string(body))
	t.Logf("Phase 2 (get_portfolio): %v", getElapsed)

	// Phase 3: Validate content
	content := string(body)

	// Must contain portfolio data (JSON response with holdings)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Contains(t, result, "holdings", "response should contain holdings")
	assert.Contains(t, result, "name", "response should contain portfolio name")

	// Phase 4: Performance assertion — get_portfolio should be fast after sync
	t.Logf("TOTAL: %v", time.Since(totalStart))
	t.Logf("get_portfolio alone: %v (target: <5s)", getElapsed)
	t.Logf("response size: %d bytes", len(content))

	assert.Less(t, getElapsed, 5*time.Second,
		"get_portfolio should be fast (cached read)")
}

func TestGetPortfolioThenReview(t *testing.T) {
	t.Skip("Portfolio review loads full financial data (filings, signals, charts) — deferred to later development")

	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio first
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	// Step A: Fast get_portfolio (just the data)
	getStart := time.Now()
	getResp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	getElapsed := time.Since(getStart)
	require.NoError(t, err, "get_portfolio failed")
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, getResp.StatusCode, "get_portfolio: %s", string(getBody))
	guard.SaveResult("02_get_portfolio_response.json", string(getBody))

	var getResult map[string]interface{}
	require.NoError(t, json.Unmarshal(getBody, &getResult))
	assert.Contains(t, getResult, "holdings")

	// Step B: Full portfolio review (signals, analysis — uses cached market data)
	reviewStart := time.Now()
	reviewResp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolio+"/review",
		map[string]interface{}{}, headers)
	reviewElapsed := time.Since(reviewStart)
	require.NoError(t, err, "portfolio review failed")
	defer reviewResp.Body.Close()

	reviewBody, err := io.ReadAll(reviewResp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, reviewResp.StatusCode, "review: %s", string(reviewBody))
	guard.SaveResult("03_review_response.json", string(reviewBody))

	var reviewResult map[string]interface{}
	require.NoError(t, json.Unmarshal(reviewBody, &reviewResult))
	assert.Contains(t, reviewResult, "review")

	// Timing comparison
	t.Logf("get_portfolio:       %v", getElapsed)
	t.Logf("portfolio review:    %v", reviewElapsed)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

func TestListPortfolios(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync a portfolio first so there's data to list
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	// List portfolios
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios", nil, headers)
	require.NoError(t, err, "list_portfolios request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "list_portfolios failed: %s", string(body))

	var result struct {
		Portfolios []interface{} `json:"portfolios"`
	}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Greater(t, len(result.Portfolios), 0, "expected at least one portfolio")

	guard.SaveResult("02_list_portfolios_response.json", string(body))
	t.Logf("Listed %d portfolios", len(result.Portfolios))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioReviewBlankConfig tests behavior with empty API keys
func TestPortfolioReviewBlankConfig(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Try to get a portfolio review without any user or Navexa setup
	// This should fail gracefully (no Navexa key, no portfolio data)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/SMSF/review",
		map[string]interface{}{}, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "review request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// With blank config and no user, we expect a non-200 response
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"expected error with blank config, got: %s", string(body))

	// Response should still be valid JSON
	assert.True(t, json.Valid(body), "response should be valid JSON: %s", string(body))

	guard.SaveResult("01_review_blank_config.json", string(body))
	t.Logf("Expected error with blank config: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}
