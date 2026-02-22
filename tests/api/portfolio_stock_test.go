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

// TestPortfolioStock_GainPercentage verifies that after syncing a portfolio,
// the get_portfolio_stock endpoint returns simple gain percentages (GainLoss / TotalCost * 100)
// rather than Navexa's IRR p.a. values. This validates the end-to-end path through
// SyncPortfolio -> calculateGainLossFromTrades -> simple % computation -> JSON response.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_GainPercentage(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate trade data and compute gain/loss
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio to find a holding with trades
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Find an active holding with trades for testing
	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 && len(h.Trades) > 0 && h.TotalCost > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding with trades and positive TotalCost")

	// Get the specific holding via get_portfolio_stock
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker, nil, headers)
	require.NoError(t, err, "get portfolio stock request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "get portfolio stock failed: %s", string(stockBody))

	guard.SaveResult("02_get_stock_response", string(stockBody))

	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding), "unmarshal holding response")

	// Verify the gain percentages are simple percentages (GainLoss / TotalInvested * 100),
	// using total capital invested as denominator
	t.Run("gain_loss_pct_is_simple", func(t *testing.T) {
		if holding.TotalInvested > 0 {
			expectedPct := (holding.GainLoss / holding.TotalInvested) * 100
			assert.InDelta(t, expectedPct, holding.GainLossPct, 0.01,
				"GainLossPct should be simple percentage (GainLoss/TotalInvested*100)")
		}
	})

	t.Run("capital_gain_pct_equals_gain_loss_pct", func(t *testing.T) {
		assert.Equal(t, holding.GainLossPct, holding.CapitalGainPct,
			"CapitalGainPct should equal GainLossPct")
	})

	t.Run("total_return_pct_includes_dividends", func(t *testing.T) {
		if holding.TotalInvested > 0 {
			expectedTotalReturnPct := (holding.TotalReturnValue / holding.TotalInvested) * 100
			assert.InDelta(t, expectedTotalReturnPct, holding.TotalReturnPct, 0.01,
				"TotalReturnPct should be simple percentage (TotalReturnValue/TotalInvested*100)")
		}
	})

	t.Run("total_return_value_consistency", func(t *testing.T) {
		assert.InDelta(t, holding.GainLoss+holding.DividendReturn, holding.TotalReturnValue, 1.0,
			"TotalReturnValue should equal GainLoss + DividendReturn (within $1)")
	})

	t.Logf("%s: units=%.0f cost=%.2f gl=%.2f glPct=%.2f capPct=%.2f trPct=%.2f",
		holding.Ticker, holding.Units, holding.TotalCost,
		holding.GainLoss, holding.GainLossPct, holding.CapitalGainPct, holding.TotalReturnPct)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_ForceRefresh verifies that calling get_portfolio_stock with
// force_refresh=true triggers a fresh sync and returns updated data.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_ForceRefresh(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Initial sync to populate the portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio to find an active holding
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got))
	require.NotEmpty(t, got.Holdings)

	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding")

	// Call get_portfolio_stock with force_refresh=true
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker+"?force_refresh=true",
		nil, headers)
	require.NoError(t, err, "force_refresh request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	guard.SaveResult("02_force_refresh_response", string(stockBody))

	assert.Equal(t, http.StatusOK, resp2.StatusCode,
		"force_refresh should return 200: %s", string(stockBody))

	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding))

	// Verify the holding has valid data after force refresh
	assert.Equal(t, testTicker, holding.Ticker)
	assert.Greater(t, holding.Units, 0.0, "active holding should have positive units")
	assert.Greater(t, holding.CurrentPrice, 0.0, "holding should have a current price after refresh")
	assert.Greater(t, holding.MarketValue, 0.0, "holding should have market value after refresh")

	// Verify gain percentages are still simple percentages after force refresh
	if holding.TotalInvested > 0 {
		expectedPct := (holding.GainLoss / holding.TotalInvested) * 100
		assert.InDelta(t, expectedPct, holding.GainLossPct, 0.01,
			"GainLossPct should remain simple percentage after force refresh")
	}

	t.Logf("%s: price=%.2f mv=%.2f glPct=%.2f (force_refresh=true)",
		holding.Ticker, holding.CurrentPrice, holding.MarketValue, holding.GainLossPct)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_ForceRefreshNoNavexa verifies that calling get_portfolio_stock
// with force_refresh=true but without Navexa credentials returns a graceful error
// rather than crashing.
func TestPortfolioStock_ForceRefreshNoNavexa(t *testing.T) {
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
		"/api/portfolios/SMSF/stock/BHP?force_refresh=true",
		nil, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_force_refresh_no_navexa", string(body))

	// Should return an error status (not 200) since there's no portfolio data
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"force_refresh without Navexa should not succeed")

	// Response should be valid JSON (graceful error, not a crash)
	assert.True(t, json.Valid(body),
		"error response should be valid JSON: %s", string(body))

	t.Logf("Expected error without Navexa: status=%d body=%s",
		resp.StatusCode, string(body))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}
