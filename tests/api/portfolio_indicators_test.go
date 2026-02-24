package api

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Helpers ---

// setupPortfolioForIndicators imports a test user, sets the Navexa key,
// and triggers a portfolio sync so that indicator endpoints have a
// portfolio record to operate on. Returns the portfolio name and user headers.
// Skips the test if NAVEXA_API_KEY or DEFAULT_PORTFOLIO are not set.
func setupPortfolioForIndicators(t *testing.T, env *common.Env) (string, map[string]string) {
	t.Helper()

	common.LoadTestSecrets()

	navexaKey := os.Getenv("NAVEXA_API_KEY")
	if navexaKey == "" {
		t.Skip("NAVEXA_API_KEY not set (set in env or tests/docker/.env)")
	}
	portfolioName := os.Getenv("DEFAULT_PORTFOLIO")
	if portfolioName == "" {
		t.Skip("DEFAULT_PORTFOLIO not set (set in env or tests/docker/.env)")
	}

	userHeaders := map[string]string{"X-Vire-User-ID": "dev_user"}

	// Import users from fixtures
	usersPath := filepath.Join(common.FindProjectRoot(), "tests", "fixtures", "users.json")
	data, err := os.ReadFile(usersPath)
	require.NoError(t, err)

	var usersFile struct {
		Users []json.RawMessage `json:"users"`
	}
	require.NoError(t, json.Unmarshal(data, &usersFile))
	require.NotEmpty(t, usersFile.Users, "users.json should contain at least one user")

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
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Set default portfolio
	resp, err = env.HTTPRequest(http.MethodPut, "/api/portfolios/default",
		map[string]string{"name": portfolioName}, userHeaders)
	require.NoError(t, err)
	resp.Body.Close()

	// Trigger portfolio sync by fetching it
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "portfolio sync failed: %s", string(body))

	return portfolioName, userHeaders
}

// --- GET /api/portfolios/{name}/indicators ---

func TestPortfolioIndicators_GET(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("returns_indicators", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		// Required fields
		assert.Equal(t, portfolioName, indicators["portfolio_name"])
		assert.Contains(t, indicators, "compute_date")
		assert.Contains(t, indicators, "current_value")
		assert.Contains(t, indicators, "data_points")

		// Moving averages
		assert.Contains(t, indicators, "ema_20")
		assert.Contains(t, indicators, "ema_50")
		assert.Contains(t, indicators, "ema_200")
		assert.Contains(t, indicators, "above_ema_20")
		assert.Contains(t, indicators, "above_ema_50")
		assert.Contains(t, indicators, "above_ema_200")

		// RSI
		assert.Contains(t, indicators, "rsi")
		assert.Contains(t, indicators, "rsi_signal")

		// Crossovers
		assert.Contains(t, indicators, "ema_50_cross_200")
		crossover := indicators["ema_50_cross_200"].(string)
		assert.Contains(t, []string{"golden_cross", "death_cross", "none"}, crossover)

		// Trend
		assert.Contains(t, indicators, "trend")
		assert.Contains(t, indicators, "trend_description")
		trend := indicators["trend"].(string)
		assert.Contains(t, []string{"bullish", "bearish", "neutral"}, trend)

		// RSI signal should be a valid classification
		rsiSignal := indicators["rsi_signal"].(string)
		assert.Contains(t, []string{"overbought", "neutral", "oversold"}, rsiSignal)

		// current_value should be positive for a real portfolio
		currentValue := indicators["current_value"].(float64)
		assert.Greater(t, currentValue, 0.0, "current_value should be positive")

		// data_points should be positive for a real portfolio
		dataPoints := indicators["data_points"].(float64)
		assert.Greater(t, dataPoints, 0.0, "data_points should be positive")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- GET /api/portfolios/{name}/indicators with non-existent portfolio ---

func TestPortfolioIndicators_NonExistent(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	t.Run("nonexistent_portfolio_returns_error", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/portfolios/nonexistent_portfolio_xyz/indicators")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_nonexistent", string(body))

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- POST /api/portfolios/{name}/review includes portfolio_indicators ---

func TestPortfolioReview_IncludesIndicators(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("review_contains_portfolio_indicators", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/review",
			map[string]interface{}{}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_review_with_indicators", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var review map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &review))

		// portfolio_indicators should be present in review response
		assert.Contains(t, review, "portfolio_indicators",
			"review response should include portfolio_indicators field")

		indicators := review["portfolio_indicators"].(map[string]interface{})
		assert.Contains(t, indicators, "portfolio_name")
		assert.Contains(t, indicators, "rsi")
		assert.Contains(t, indicators, "trend")
		assert.Contains(t, indicators, "ema_50_cross_200")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- GET /api/portfolios/{name} has total_value_holdings and total_value ---

func TestPortfolio_TotalValueFields(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("portfolio_has_both_total_value_fields", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_portfolio_total_value_fields", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// Both fields should be present
		assert.Contains(t, portfolio, "total_value_holdings",
			"portfolio should have total_value_holdings field")
		assert.Contains(t, portfolio, "total_value",
			"portfolio should have total_value field")

		totalValueHoldings := portfolio["total_value_holdings"].(float64)
		totalValue := portfolio["total_value"].(float64)
		externalBalanceTotal := portfolio["external_balance_total"].(float64)

		// Invariant: total_value = total_value_holdings + external_balance_total
		assert.InDelta(t, totalValueHoldings+externalBalanceTotal, totalValue, 0.01,
			"total_value should equal total_value_holdings + external_balance_total")

		// Without external balances, total_value should equal total_value_holdings
		if externalBalanceTotal == 0 {
			assert.Equal(t, totalValueHoldings, totalValue,
				"without external balances, total_value should equal total_value_holdings")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- total_value = total_value_holdings + external_balance_total ---

func TestPortfolio_TotalValueInvariantWithExternalBalances(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Get initial total_value_holdings
	var initialHoldingsValue float64
	t.Run("get_initial_state", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("05_initial_portfolio", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		initialHoldingsValue = portfolio["total_value_holdings"].(float64)
		totalValue := portfolio["total_value"].(float64)
		externalTotal := portfolio["external_balance_total"].(float64)

		assert.InDelta(t, initialHoldingsValue+externalTotal, totalValue, 0.01)
	})

	// Step 2: Add external balance
	t.Run("add_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances",
			map[string]interface{}{
				"type":  "cash",
				"label": "Invariant Test Cash",
				"value": 100000,
			}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 3: Verify invariant holds after adding external balance
	t.Run("verify_invariant_after_add", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("06_after_ext_balance", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		totalValueHoldings := portfolio["total_value_holdings"].(float64)
		totalValue := portfolio["total_value"].(float64)
		externalTotal := portfolio["external_balance_total"].(float64)

		// Holdings value should be unchanged
		assert.InDelta(t, initialHoldingsValue, totalValueHoldings, 0.01,
			"total_value_holdings should not change when external balances are added")

		// External balance should be 100000
		assert.Equal(t, 100000.0, externalTotal)

		// Invariant: total_value = total_value_holdings + external_balance_total
		assert.InDelta(t, totalValueHoldings+externalTotal, totalValue, 0.01,
			"total_value should equal total_value_holdings + external_balance_total")

		// total_value should be greater than total_value_holdings
		assert.Greater(t, totalValue, totalValueHoldings,
			"total_value should be greater than total_value_holdings when external balances exist")
	})

	// Step 4: Clean up external balances
	t.Run("cleanup", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{
				"external_balances": []map[string]interface{}{},
			}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Method not allowed ---

func TestPortfolioIndicators_MethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			resp, err := env.HTTPRequest(method, "/api/portfolios/"+portfolioName+"/indicators", nil, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("method_"+method, string(body))

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode,
				"%s should return 405 Method Not Allowed", method)
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
