package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/tests/common"
)

// TestGetStockData_ForceRefresh verifies that GET /api/market/stocks/{ticker}?force_refresh=true
// returns a wrapped response with "data" and "advisory" fields. The advisory indicates that
// background jobs have been enqueued for slow data collection.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables (for portfolio sync
// to get a valid ticker).
func TestGetStockData_ForceRefresh(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate data and get a valid ticker
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	// Get portfolio to extract a ticker from holdings
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get_portfolio failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get_portfolio: %s", string(body))

	var p models.Portfolio
	require.NoError(t, json.Unmarshal(body, &p))
	require.NotEmpty(t, p.Holdings, "portfolio must have holdings to test stock data")

	// Pick the first holding's EODHD ticker
	ticker := p.Holdings[0].EODHDTicker()
	t.Logf("Using ticker: %s", ticker)

	// Call GET /api/market/stocks/{ticker}?force_refresh=true
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/market/stocks/"+ticker+"?force_refresh=true", nil, headers)
	require.NoError(t, err, "get_stock_data force_refresh request failed")
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	guard.SaveResult("02_stock_data_force_refresh.json", string(body2))

	require.Equal(t, http.StatusOK, resp2.StatusCode,
		"force_refresh stock data should return 200: %s", string(body2))

	// Parse response — with force_refresh and background jobs, should be wrapped
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body2, &result))

	t.Run("response_has_data_field", func(t *testing.T) {
		assert.Contains(t, result, "data",
			"force_refresh response should contain 'data' field")
	})

	t.Run("response_has_advisory_field", func(t *testing.T) {
		assert.Contains(t, result, "advisory",
			"force_refresh response should contain 'advisory' field")
	})

	t.Run("advisory_mentions_background_jobs", func(t *testing.T) {
		advisory, ok := result["advisory"].(string)
		if assert.True(t, ok, "advisory should be a string") {
			assert.Contains(t, advisory, "background job",
				"advisory should mention background jobs")
		}
	})

	t.Run("data_contains_ticker", func(t *testing.T) {
		data, ok := result["data"].(map[string]interface{})
		if assert.True(t, ok, "data should be an object") {
			assert.Contains(t, data, "ticker",
				"data should contain ticker field")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGetStockData_WithoutForceRefresh verifies that GET /api/market/stocks/{ticker}
// (without force_refresh) returns bare StockData without the wrapper format.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestGetStockData_WithoutForceRefresh(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio and get a valid ticker
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get_portfolio failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get_portfolio: %s", string(body))

	var p models.Portfolio
	require.NoError(t, json.Unmarshal(body, &p))
	require.NotEmpty(t, p.Holdings, "portfolio must have holdings to test stock data")

	ticker := p.Holdings[0].EODHDTicker()
	t.Logf("Using ticker: %s", ticker)

	// Call GET /api/market/stocks/{ticker} without force_refresh
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/market/stocks/"+ticker, nil, headers)
	require.NoError(t, err, "get_stock_data request failed")
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	guard.SaveResult("02_stock_data_no_force.json", string(body2))

	require.Equal(t, http.StatusOK, resp2.StatusCode,
		"stock data should return 200: %s", string(body2))

	// Parse as bare StockData — NOT wrapped in {data, advisory}
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body2, &result))

	t.Run("response_is_bare_stock_data", func(t *testing.T) {
		// Bare StockData has "ticker" at the top level, not nested under "data"
		assert.Contains(t, result, "ticker",
			"response should contain 'ticker' at top level (bare StockData)")
		assert.NotContains(t, result, "advisory",
			"response should NOT contain 'advisory' (not force_refresh)")
	})

	t.Run("response_has_price", func(t *testing.T) {
		assert.Contains(t, result, "price",
			"response should contain 'price' field")
	})

	t.Run("response_has_fundamentals", func(t *testing.T) {
		assert.Contains(t, result, "fundamentals",
			"response should contain 'fundamentals' field")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGetStockData_ForceRefresh_BlankConfig verifies that force_refresh with a blank
// config (no real API keys) handles gracefully without crashing. The handler logs a
// warning when CollectCoreMarketData fails but still returns a response.
func TestGetStockData_ForceRefresh_BlankConfig(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Call force_refresh with blank config (EODHD key is "demo", no real data)
	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/market/stocks/BHP.AU?force_refresh=true",
		nil, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_force_refresh_blank_config.json", string(body))

	t.Run("does_not_crash", func(t *testing.T) {
		// The handler should not crash — it may return 200 with empty data
		// or 500 if GetStockData fails, but the server should stay up
		assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusInternalServerError,
			"expected 200 or 500, got %d: %s", resp.StatusCode, string(body))
	})

	t.Run("response_is_valid_json", func(t *testing.T) {
		assert.True(t, json.Valid(body),
			"response should be valid JSON: %s", string(body))
	})

	t.Logf("force_refresh blank config: status=%d body_len=%d", resp.StatusCode, len(body))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGetPortfolio_DemandDrivenEnqueue verifies that GET /api/portfolios/{name}
// (normal, no force_refresh) returns a valid portfolio response and does not crash
// due to the demand-driven fire-and-forget goroutine that enqueues background jobs
// for stale market data.
//
// The goroutine runs asynchronously after WriteJSON, so we cannot assert on job
// enqueue directly. Instead we verify the response is unaffected by the goroutine.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestGetPortfolio_DemandDrivenEnqueue(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate cached data
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response.json", string(syncBody))

	// GET portfolio (normal, no force_refresh) — triggers demand-driven enqueue in background
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get_portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("02_get_portfolio_demand_driven.json", string(body))

	require.Equal(t, http.StatusOK, resp.StatusCode,
		"get_portfolio should return 200: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got))

	t.Run("response_has_holdings", func(t *testing.T) {
		assert.NotEmpty(t, got.Holdings,
			"portfolio should have holdings")
	})

	t.Run("response_has_name", func(t *testing.T) {
		assert.Equal(t, portfolio, got.Name,
			"portfolio name should match request")
	})

	t.Run("trades_stripped", func(t *testing.T) {
		for _, h := range got.Holdings {
			assert.Empty(t, h.Trades,
				"holding %s trades should be stripped in portfolio GET", h.Ticker)
		}
	})

	t.Run("response_is_valid_portfolio", func(t *testing.T) {
		assert.False(t, got.LastSynced.IsZero(),
			"last_synced should not be zero after sync")
	})

	t.Logf("Portfolio: %s, Holdings: %d, LastSynced: %s",
		got.Name, len(got.Holdings), got.LastSynced)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}
