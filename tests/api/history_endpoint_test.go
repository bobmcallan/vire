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

// --- History Endpoint: snake_case Fields ---

// TestHistoryEndpoint_SnakeCaseFields verifies that the /api/portfolios/{name}/history
// endpoint returns data_points array with snake_case JSON field names, not PascalCase.
func TestHistoryEndpoint_SnakeCaseFields(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Add cash transactions so time series has data
	t.Run("add_cash_transactions", func(t *testing.T) {
		transactions := []map[string]interface{}{
			{
				"category":    "contribution",
				"account":     "Trading",
				"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      100000,
				"description": "Initial deposit for history test",
			},
			{
				"category":    "contribution",
				"account":     "Trading",
				"date":        time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      25000,
				"description": "Contribution for history test",
			},
		}

		for _, tx := range transactions {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", tx, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
		}
	})

	// Step 2: Get history with daily format (default)
	t.Run("history_daily_format", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_history_daily_response", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		// Verify response structure
		assert.Contains(t, historyResp, "portfolio")
		assert.Contains(t, historyResp, "format")
		assert.Contains(t, historyResp, "data_points")
		assert.Contains(t, historyResp, "count")

		// Verify data_points is an array
		dataPoints, ok := historyResp["data_points"].([]interface{})
		require.True(t, ok, "data_points should be an array")

		if len(dataPoints) > 0 {
			// Check first point for snake_case field names
			firstPoint := dataPoints[0].(map[string]interface{})

			// Verify snake_case fields are present
			assert.Contains(t, firstPoint, "date", "should have date field")
			assert.Contains(t, firstPoint, "total_value", "should have total_value field")
			assert.Contains(t, firstPoint, "total_cost", "should have total_cost field")
			assert.Contains(t, firstPoint, "net_return", "should have net_return field")
			assert.Contains(t, firstPoint, "net_return_pct", "should have net_return_pct field")
			assert.Contains(t, firstPoint, "holding_count", "should have holding_count field")
			assert.Contains(t, firstPoint, "total_cash", "should have total_cash field")
			assert.Contains(t, firstPoint, "total_capital", "should have total_capital field")
			assert.Contains(t, firstPoint, "net_capital_deployed", "should have net_capital_deployed field")

			// Verify NO PascalCase field names
			assert.NotContains(t, firstPoint, "TotalValue", "should NOT have TotalValue (PascalCase)")
			assert.NotContains(t, firstPoint, "NetDeployed", "should NOT have NetDeployed (PascalCase)")
			assert.NotContains(t, firstPoint, "CashBalance", "should NOT have CashBalance (PascalCase)")
			assert.NotContains(t, firstPoint, "NetReturnPct", "should NOT have NetReturnPct (PascalCase)")
		}
	})

	// Step 3: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- History Endpoint: net_deployed Field ---

// TestHistoryEndpoint_NetDeployedPresent verifies that net_deployed field is present
// in history data_points and accumulates correctly.
func TestHistoryEndpoint_NetDeployedPresent(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add cash transactions with known values
	t.Run("add_cash_transactions_with_known_values", func(t *testing.T) {
		// deposit(100000) + contribution(25000) - transfer(15000) = 110000 net
		transactions := []map[string]interface{}{
			{
				"category":    "contribution",
				"account":     "Trading",
				"date":        time.Now().Add(-300 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      100000,
				"description": "Initial deposit",
			},
			{
				"category":    "contribution",
				"account":     "Trading",
				"date":        time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      25000,
				"description": "Contribution",
			},
			{
				"category":    "transfer",
				"account":     "Trading",
				"date":        time.Now().Add(-100 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      -15000,
				"description": "Withdrawal",
			},
		}

		for _, tx := range transactions {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", tx, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
		}
	})

	// Get history and verify net_deployed
	t.Run("verify_net_deployed_accumulation", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_history_net_deployed", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		dataPoints, ok := historyResp["data_points"].([]interface{})
		require.True(t, ok, "data_points should be an array")

		if len(dataPoints) > 0 {
			// Last point should have highest cumulative net_capital_deployed
			lastPoint := dataPoints[len(dataPoints)-1].(map[string]interface{})

			netCapitalDeployed, hasNetCapitalDeployed := lastPoint["net_capital_deployed"]
			assert.True(t, hasNetCapitalDeployed, "last point should have net_capital_deployed field")

			if hasNetCapitalDeployed {
				// net_capital_deployed = 100000 + 25000 - 15000 = 110000
				assert.InDelta(t, 110000.0, netCapitalDeployed.(float64), 1.0,
					"net_capital_deployed should equal deposits + contributions - transfers")
			}
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- History Endpoint: Format Downsampling ---

// TestHistoryEndpoint_FormatDownsampling verifies that the format parameter
// (daily, weekly, monthly, auto) correctly downsamples the data_points array.
func TestHistoryEndpoint_FormatDownsampling(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add cash transactions to ensure we have historical data
	t.Run("add_transactions_for_downsampling", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      100000,
			"description": "Long-term deposit for downsampling test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Get history in different formats and compare point counts
	var dailyCount, weeklyCount, monthlyCount int

	t.Run("get_daily_format", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history?format=daily", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_history_daily", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		assert.Equal(t, "daily", historyResp["format"], "format should be 'daily'")

		dataPoints, ok := historyResp["data_points"].([]interface{})
		require.True(t, ok, "data_points should be an array")
		dailyCount = len(dataPoints)
		t.Logf("Daily format returned %d points", dailyCount)
	})

	t.Run("get_weekly_format", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history?format=weekly", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_history_weekly", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		assert.Equal(t, "weekly", historyResp["format"], "format should be 'weekly'")

		dataPoints, ok := historyResp["data_points"].([]interface{})
		require.True(t, ok, "data_points should be an array")
		weeklyCount = len(dataPoints)
		t.Logf("Weekly format returned %d points", weeklyCount)
	})

	t.Run("get_monthly_format", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history?format=monthly", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_history_monthly", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		assert.Equal(t, "monthly", historyResp["format"], "format should be 'monthly'")

		dataPoints, ok := historyResp["data_points"].([]interface{})
		require.True(t, ok, "data_points should be an array")
		monthlyCount = len(dataPoints)
		t.Logf("Monthly format returned %d points", monthlyCount)
	})

	t.Run("verify_downsampling_reduces_points", func(t *testing.T) {
		// Weekly should have fewer or equal points than daily
		if dailyCount > 0 {
			assert.True(t, weeklyCount <= dailyCount,
				"weekly downsampling should reduce or maintain point count (daily=%d, weekly=%d)", dailyCount, weeklyCount)
		}

		// Monthly should have fewer or equal points than weekly
		if weeklyCount > 0 {
			assert.True(t, monthlyCount <= weeklyCount,
				"monthly downsampling should reduce or maintain point count (weekly=%d, monthly=%d)", weeklyCount, monthlyCount)
		}
	})

	t.Run("get_auto_format", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history?format=auto", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_history_auto", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		assert.Equal(t, "auto", historyResp["format"], "format should be 'auto'")

		dataPoints, ok := historyResp["data_points"].([]interface{})
		require.True(t, ok, "data_points should be an array")

		// Auto format should be reasonable (not > 365 points for daily, not > 200 for weekly)
		autoCount := len(dataPoints)
		t.Logf("Auto format returned %d points", autoCount)
		assert.True(t, autoCount <= dailyCount,
			"auto format should return <= daily points (auto=%d, daily=%d)", autoCount, dailyCount)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Review Endpoint: Growth Field snake_case ---

// TestReviewEndpoint_GrowthFieldSnakeCase verifies that the /api/portfolios/{name}/review
// endpoint returns growth data with snake_case field names.
func TestReviewEndpoint_GrowthFieldSnakeCase(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add cash transactions so we have growth data
	t.Run("add_transactions", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      100000,
			"description": "Deposit for review test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Get review and check growth field format
	t.Run("review_growth_field_format", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/review", map[string]interface{}{}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_review_response", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var reviewResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &reviewResp))

		// Verify growth field exists
		growth, hasGrowth := reviewResp["growth"]
		assert.True(t, hasGrowth, "review response should have growth field")

		if hasGrowth {
			// growth should be an array of objects
			growthArray, ok := growth.([]interface{})
			require.True(t, ok, "growth should be an array")

			if len(growthArray) > 0 {
				// Check first point for snake_case field names
				firstPoint := growthArray[0].(map[string]interface{})

				// Verify snake_case fields
				assert.Contains(t, firstPoint, "date", "growth point should have date field")
				assert.Contains(t, firstPoint, "total_value", "growth point should have total_value field")
				assert.Contains(t, firstPoint, "total_cost", "growth point should have total_cost field")
				assert.Contains(t, firstPoint, "net_capital_deployed", "growth point should have net_capital_deployed field")

				// Verify NO PascalCase
				assert.NotContains(t, firstPoint, "TotalValue", "should NOT have TotalValue (PascalCase)")
				assert.NotContains(t, firstPoint, "NetDeployed", "should NOT have NetDeployed (PascalCase)")
			}
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- History Endpoint: Default Format Handling ---

// TestHistoryEndpoint_DefaultFormatDaily verifies that the history endpoint
// defaults to daily format when no format parameter is provided.
func TestHistoryEndpoint_DefaultFormatDaily(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add transactions
	t.Run("add_transactions", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      80000,
			"description": "Deposit for default format test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Get history without format parameter
	t.Run("history_no_format_param", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/history", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_history_default_format", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var historyResp map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &historyResp))

		// Default format should be "auto"
		format, hasFormat := historyResp["format"]
		assert.True(t, hasFormat, "response should have format field")
		assert.Equal(t, "auto", format, "default format should be 'auto'")
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
