package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestRemovedEndpoints_History verifies that the old /history endpoint returns 404.
// The route has been renamed to /timeline.
func TestRemovedEndpoints_History(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_history_endpoint_404", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"old /history endpoint should return 404 (renamed to /timeline)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestRemovedEndpoints_CashSummary verifies that the old /cash-summary endpoint returns 404.
// Use list_cash_transactions with summary_only=true instead.
func TestRemovedEndpoints_CashSummary(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-summary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_cash_summary_endpoint_404", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"old /cash-summary endpoint should return 404 (use list_cash_transactions with summary_only=true)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestRemovedEndpoints_CapitalPerformance verifies that the old /cash-transactions/performance
// endpoint returns 404. Capital performance data is now embedded in the portfolio response.
func TestRemovedEndpoints_CapitalPerformance(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_capital_performance_endpoint_404", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"old /cash-transactions/performance endpoint should return 404 (use capital_performance field in portfolio)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestRemovedEndpoints_ScreenLegacy verifies that the old /screen endpoint returns 404.
// Use /screen/stocks with mode parameter instead.
func TestRemovedEndpoints_ScreenLegacy(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen",
		map[string]interface{}{
			"exchange": "AU",
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_endpoint_404", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"old /api/screen endpoint should return 404 (use /api/screen/stocks with mode=fundamental)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestRemovedEndpoints_ScreenSnipe verifies that the old /screen/snipe endpoint returns 404.
// Use /screen/stocks with mode=technical instead.
func TestRemovedEndpoints_ScreenSnipe(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen/snipe",
		map[string]interface{}{
			"exchange": "AU",
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_snipe_endpoint_404", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"old /api/screen/snipe endpoint should return 404 (use /api/screen/stocks with mode=technical)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestListCashTransactions_SummaryOnlyParam verifies that list_cash_transactions
// with summary_only=true returns only accounts and summary, omitting transactions array.
func TestListCashTransactions_SummaryOnlyParam(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add some transactions
	_, status := postCashTx(t, env, portfolioName, userHeaders,
		"Trading", "contribution",
		50000, "2025-01-15T00:00:00Z",
		"Test deposit")
	require.Equal(t, http.StatusCreated, status)

	t.Run("summary_only_true_omits_transactions", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet,
			"/api/portfolios/"+portfolioName+"/cash-transactions?summary_only=true",
			nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_summary_only_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Should have accounts and summary
		assert.Contains(t, result, "accounts", "response should have accounts")
		assert.Contains(t, result, "summary", "response should have summary")

		// Should NOT have transactions array
		_, hasTransactions := result["transactions"]
		assert.False(t, hasTransactions,
			"summary_only=true should omit transactions array")
	})

	t.Run("default_without_summary_only_includes_transactions", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet,
			"/api/portfolios/"+portfolioName+"/cash-transactions",
			nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_full_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Should have all three
		assert.Contains(t, result, "accounts", "response should have accounts")
		assert.Contains(t, result, "summary", "response should have summary")
		_, hasTransactions := result["transactions"]
		assert.True(t, hasTransactions,
			"default response should include transactions array")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioResponse_NewFieldNames verifies that the portfolio response
// contains the new field names from the refactor.
func TestPortfolioResponse_NewFieldNames(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_portfolio_response", string(body))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &raw))

	// Portfolio-level field names should use new naming
	t.Run("portfolio_level_fields", func(t *testing.T) {
		expectedFields := []string{
			"equity_value",              // Was: total_value_holdings
			"portfolio_value",           // Was: total_value
			"net_equity_cost",           // Was: total_cost
			"gross_cash_balance",        // Was: total_cash
			"net_cash_balance",          // Was: available_cash
			"net_equity_return",         // Was: total_net_return
			"net_equity_return_pct",     // Was: total_net_return_pct
			"realized_equity_return",    // Was: total_realized_net_return
			"unrealized_equity_return",  // Was: total_unrealized_net_return
		}

		for _, field := range expectedFields {
			assert.Contains(t, raw, field,
				"portfolio response should contain new field name: %s", field)
		}
	})

	// Verify old field names are gone
	t.Run("old_field_names_removed", func(t *testing.T) {
		oldFields := []string{
			"total_value_holdings",
			"total_value",
			"total_cost",
			"total_cash",
			"available_cash",
			"total_net_return",
			"total_net_return_pct",
			"total_realized_net_return",
			"total_unrealized_net_return",
		}

		for _, field := range oldFields {
			_, exists := raw[field]
			assert.False(t, exists,
				"old field name should be removed: %s", field)
		}
	})

	// Holding-level field names
	t.Run("holding_level_fields", func(t *testing.T) {
		holdings, ok := raw["holdings"].([]interface{})
		require.True(t, ok, "holdings should be an array")

		if len(holdings) > 0 {
			holding := holdings[0].(map[string]interface{})

			expectedFields := []string{
				"market_value",           // Keep
				"cost_basis",             // Was: total_cost
				"gross_invested",         // Was: total_invested
				"gross_proceeds",         // Was: total_proceeds
				"realized_return",        // Was: realized_net_return
				"unrealized_return",      // Was: unrealized_net_return
				"portfolio_weight_pct",   // Was: weight
			}

			for _, field := range expectedFields {
				assert.Contains(t, holding, field,
					"holding should contain new field name: %s", field)
			}

			// Verify old holding field names are gone
			oldFields := []string{
				"total_cost",
				"total_invested",
				"total_proceeds",
				"realized_net_return",
				"unrealized_net_return",
				"weight",
			}

			for _, field := range oldFields {
				_, exists := holding[field]
				assert.False(t, exists,
					"old holding field name should be removed: %s", field)
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioIndicators_NoTimeSeries verifies that get_portfolio_indicators
// does NOT include the time_series field (it's been moved to get_portfolio_timeline).
func TestPortfolioIndicators_NoTimeSeries(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolioName+"/indicators",
		nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_indicators_response", string(body))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	// Should NOT have time_series (moved to /timeline endpoint)
	_, hasTimeSeries := result["time_series"]
	assert.False(t, hasTimeSeries,
		"indicators response should NOT include time_series (use /timeline endpoint instead)")

	// Should still have RSI, EMA, trend indicators
	assert.Contains(t, result, "rsi", "indicators should have RSI data")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
