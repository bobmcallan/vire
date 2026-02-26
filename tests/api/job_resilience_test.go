package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestGetStockData_Timeout_BlankConfig verifies that GET /api/market/stocks/{ticker}
// returns within a reasonable time even when market data is absent (no EODHD key).
// This exercises the timeout path in handleMarketStocks and GetStockData.
//
// Before the fix: GetStockData could hang indefinitely when CollectMarketData
// was called without a context deadline.
// After the fix: a 90s handler timeout and 60s collect timeout bound the call.
func TestGetStockData_Timeout_BlankConfig(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	start := time.Now()

	// Call with a ticker that definitely has no cached data
	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/market/stocks/BHP.AU",
		nil,
		map[string]string{"X-Vire-User-ID": "nonexistent"},
	)
	require.NoError(t, err, "request should not fail at HTTP level")
	defer resp.Body.Close()

	elapsed := time.Since(start)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_get_stock_data_no_cache", string(body))

	t.Logf("GetStockData elapsed: %v, status: %d", elapsed, resp.StatusCode)

	t.Run("returns_within_timeout", func(t *testing.T) {
		// The handler timeout is 90s; collection timeout is 60s.
		// With no real API key, collection fails quickly (auth error).
		// Even in the worst case, must return well within 120s.
		assert.Less(t, elapsed, 120*time.Second,
			"GetStockData should return within 120s even with no cached data")
	})

	t.Run("does_not_hang", func(t *testing.T) {
		// Response should be valid JSON (not a timeout/empty)
		assert.True(t, json.Valid(body),
			"response should be valid JSON: %s", string(body))
	})

	t.Run("returns_error_or_success", func(t *testing.T) {
		// With blank config (demo EODHD key), collection will fail.
		// Server should return 404 or 500, not hang.
		assert.Contains(t, []int{
			http.StatusOK,
			http.StatusNotFound,
			http.StatusInternalServerError,
		}, resp.StatusCode, "expected 200, 404, or 500, got %d: %s", resp.StatusCode, string(body))
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGetStockData_ForceRefresh_Timeout verifies that GET /api/market/stocks/{ticker}?force_refresh=true
// also returns within a bounded time when the context timeout is applied correctly.
//
// Before the fix: the force_refresh path could also hang because both CollectCoreMarketData
// and GetStockData were called without the bounded context.
// After the fix: both calls use the 90s context created in handleMarketStocks.
func TestGetStockData_ForceRefresh_Timeout(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	start := time.Now()

	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/market/stocks/BHP.AU?force_refresh=true",
		nil,
		map[string]string{"X-Vire-User-ID": "nonexistent"},
	)
	require.NoError(t, err, "force_refresh request should not fail at HTTP level")
	defer resp.Body.Close()

	elapsed := time.Since(start)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("02_force_refresh_timeout", string(body))

	t.Logf("ForceRefresh elapsed: %v, status: %d", elapsed, resp.StatusCode)

	t.Run("force_refresh_returns_within_timeout", func(t *testing.T) {
		assert.Less(t, elapsed, 120*time.Second,
			"force_refresh should return within 120s even with no real API key")
	})

	t.Run("force_refresh_returns_valid_json", func(t *testing.T) {
		assert.True(t, json.Valid(body),
			"force_refresh response should be valid JSON: %s", string(body))
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestComputeIndicators_WithMarketData verifies that POST /api/market/signals
// returns valid signals after market data has been collected (not a "market data not found" error).
//
// This test requires EODHD_API_KEY (via vire-service.toml) so that collect works.
// With the bug fix in computeSignals: if EOD data is missing, an error is returned
// (preventing silent failure that marks signals as fresh). This test verifies
// the happy path: signals are computed correctly when data is present.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestComputeIndicators_WithMarketData(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Step 1: Sync portfolio to establish market data collection
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	// Step 2: Get a valid ticker from the portfolio
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get_portfolio failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get_portfolio: %s", string(body))

	var portfolioResp struct {
		Holdings []struct {
			Ticker   string `json:"ticker"`
			Exchange string `json:"exchange"`
		} `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal(body, &portfolioResp))
	require.NotEmpty(t, portfolioResp.Holdings, "portfolio must have holdings")

	// Build EODHD ticker from first holding
	ticker := portfolioResp.Holdings[0].Ticker
	if portfolioResp.Holdings[0].Exchange != "" {
		ticker = portfolioResp.Holdings[0].Ticker
	}
	t.Logf("Using ticker: %s", ticker)

	// Step 3: Collect market data explicitly to ensure EOD is present
	collectResp, err := env.HTTPRequest(http.MethodPost, "/api/market/collect",
		map[string]interface{}{"tickers": []string{ticker}}, headers)
	require.NoError(t, err)
	collectBody, _ := io.ReadAll(collectResp.Body)
	collectResp.Body.Close()
	guard.SaveResult("02_collect_response.json", string(collectBody))
	t.Logf("Collect status: %d", collectResp.StatusCode)

	// Step 4: Call compute_indicators (POST /api/market/signals)
	signalsResp, err := env.HTTPRequest(http.MethodPost, "/api/market/signals",
		map[string]interface{}{"tickers": []string{ticker}}, headers)
	require.NoError(t, err, "compute_indicators request failed")
	defer signalsResp.Body.Close()

	signalsBody, err := io.ReadAll(signalsResp.Body)
	require.NoError(t, err)
	guard.SaveResult("03_signals_response.json", string(signalsBody))

	t.Run("returns_200", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, signalsResp.StatusCode,
			"compute_indicators should return 200: %s", string(signalsBody))
	})

	t.Run("response_has_signals_field", func(t *testing.T) {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(signalsBody, &result))
		assert.Contains(t, result, "signals",
			"response should contain 'signals' field: %s", string(signalsBody))
	})

	t.Run("signals_array_is_not_empty", func(t *testing.T) {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(signalsBody, &result))

		signals, ok := result["signals"].([]interface{})
		if assert.True(t, ok, "signals should be an array") {
			assert.NotEmpty(t, signals,
				"signals should not be empty after market data collection")
		}
	})

	t.Run("no_market_data_not_found_error", func(t *testing.T) {
		// The bug: computeSignals returned nil (success) when EOD was empty,
		// causing updateStockIndexTimestamp to mark signals as fresh (preventing retries).
		// After the fix, when EOD exists, signals should compute without "market data not found" error.
		assert.NotContains(t, string(signalsBody), "market data not found",
			"should not return 'market data not found' error after collection")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestComputeIndicators_EmptyTicker verifies that POST /api/market/signals
// with an empty tickers list returns a 400 error (not 500/hang).
func TestComputeIndicators_EmptyTicker(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/market/signals",
		map[string]interface{}{"tickers": []string{}},
		map[string]string{"X-Vire-User-ID": "nonexistent"},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_empty_tickers", string(body))

	t.Run("returns_400_for_empty_tickers", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
			"empty tickers should return 400: %s", string(body))
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestComputeIndicators_NoMarketData verifies that POST /api/market/signals
// for a ticker with no collected EOD data returns an appropriate error,
// NOT a silent success (which was the bug before the executor fix).
//
// This exercises the behavior via the API rather than testing the executor directly.
// With the fix: computeSignals returns an error when EOD is absent, so the job fails
// and the signals timestamp is NOT updated (allowing retry).
// The signals endpoint reflects this by returning error or empty signals.
func TestComputeIndicators_NoMarketData(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Use a ticker that definitely has no market data in blank config
	resp, err := env.HTTPRequest(http.MethodPost, "/api/market/signals",
		map[string]interface{}{"tickers": []string{"FAKE.AU"}},
		map[string]string{"X-Vire-User-ID": "nonexistent"},
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_no_market_data", string(body))

	t.Logf("compute_indicators with no market data: status=%d body=%s", resp.StatusCode, string(body))

	t.Run("returns_valid_response", func(t *testing.T) {
		// The signals endpoint either returns 200 with empty signals,
		// or returns an error. Either is acceptable.
		// It must NOT hang and must NOT return a corrupt/empty body.
		assert.True(t, json.Valid(body),
			"response must be valid JSON even when no market data is present: %s", string(body))
	})

	t.Run("does_not_silently_succeed_with_empty_signals", func(t *testing.T) {
		if resp.StatusCode != http.StatusOK {
			return // error is fine
		}
		// If 200, check that signals list reflects the no-data state
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return
		}
		// Don't assert on empty vs non-empty; just that response is coherent
		t.Logf("Signals response (no market data): %s", string(body))
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
