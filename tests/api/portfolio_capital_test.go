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

// --- GET /api/portfolios/{name} includes capital_performance ---

func TestPortfolio_CapitalPerformanceIncluded(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Portfolio without cash transactions should NOT have capital_performance
	t.Run("no_capital_performance_without_transactions", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_no_capital_perf", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		_, hasCapitalPerf := portfolio["capital_performance"]
		assert.False(t, hasCapitalPerf,
			"capital_performance should be absent when no cash transactions exist")
	})

	// Step 2: Add a cash transaction
	t.Run("add_cash_transaction", func(t *testing.T) {
		tx := map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      100000,
			"description": "Initial deposit for capital perf test",
		}
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", tx, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_add_transaction", string(body))

		require.Equal(t, http.StatusCreated, resp.StatusCode, "add transaction failed: %s", string(body))
	})

	// Step 3: Portfolio should now include capital_performance
	t.Run("capital_performance_included", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_with_capital_perf", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// capital_performance should be present
		require.Contains(t, portfolio, "capital_performance",
			"capital_performance should be present when cash transactions exist")

		perf := portfolio["capital_performance"].(map[string]interface{})

		// Required fields
		assert.Contains(t, perf, "total_deposited")
		assert.Contains(t, perf, "total_withdrawn")
		assert.Contains(t, perf, "net_capital_deployed")
		assert.Contains(t, perf, "current_portfolio_value")
		assert.Contains(t, perf, "simple_return_pct")
		assert.Contains(t, perf, "annualized_return_pct")
		assert.Contains(t, perf, "transaction_count")

		// Values should be reasonable
		totalDeposited := perf["total_deposited"].(float64)
		assert.Equal(t, 100000.0, totalDeposited,
			"total_deposited should match the deposit amount")

		txCount := perf["transaction_count"].(float64)
		assert.GreaterOrEqual(t, txCount, 1.0,
			"transaction_count should be at least 1")

		currentValue := perf["current_portfolio_value"].(float64)
		assert.Greater(t, currentValue, 0.0,
			"current_portfolio_value should be positive")
	})

	// Step 4: Verify capital_performance matches standalone endpoint
	t.Run("matches_standalone_endpoint", func(t *testing.T) {
		// Get from portfolio response
		resp1, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp1.Body.Close()

		var portfolio map[string]interface{}
		body1, _ := io.ReadAll(resp1.Body)
		require.NoError(t, json.Unmarshal(body1, &portfolio))
		embedded := portfolio["capital_performance"].(map[string]interface{})

		// Get from standalone endpoint
		resp2, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp2.Body.Close()

		var standalone map[string]interface{}
		body2, _ := io.ReadAll(resp2.Body)
		guard.SaveResult("04_standalone_perf", string(body2))
		require.NoError(t, json.Unmarshal(body2, &standalone))

		// Key metrics should match
		assert.Equal(t, standalone["total_deposited"], embedded["total_deposited"],
			"total_deposited should match between embedded and standalone")
		assert.Equal(t, standalone["total_withdrawn"], embedded["total_withdrawn"],
			"total_withdrawn should match between embedded and standalone")
		assert.Equal(t, standalone["net_capital_deployed"], embedded["net_capital_deployed"],
			"net_capital_deployed should match between embedded and standalone")
		assert.Equal(t, standalone["transaction_count"], embedded["transaction_count"],
			"transaction_count should match between embedded and standalone")
		assert.InDelta(t, standalone["simple_return_pct"].(float64), embedded["simple_return_pct"].(float64), 0.01,
			"simple_return_pct should match between embedded and standalone")
		assert.InDelta(t, standalone["annualized_return_pct"].(float64), embedded["annualized_return_pct"].(float64), 0.01,
			"annualized_return_pct should match between embedded and standalone")
	})

	// Step 5: Clean up cash transactions
	t.Run("cleanup", func(t *testing.T) {
		// Get ledger to find transaction IDs
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		var ledger map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		require.NoError(t, json.Unmarshal(body, &ledger))

		transactions, ok := ledger["transactions"].([]interface{})
		if !ok || len(transactions) == 0 {
			return
		}

		for _, tx := range transactions {
			txMap := tx.(map[string]interface{})
			txID := txMap["id"].(string)
			resp, err := env.HTTPRequest(http.MethodDelete, basePath+"/cashflows/"+txID, nil, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
