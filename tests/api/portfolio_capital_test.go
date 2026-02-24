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

// --- CurrentPortfolioValue includes external balances ---

func TestCapitalPerformance_IncludesExternalBalances(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Add a cash transaction so capital_performance is populated
	t.Run("add_cash_transaction", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      200000,
			"description": "Deposit for external balance test",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 2: Get baseline capital performance (no external balances)
	var baselineCurrentValue float64
	t.Run("baseline_without_external_balances", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_baseline_perf", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		baselineCurrentValue = perf["current_portfolio_value"].(float64)
		assert.Greater(t, baselineCurrentValue, 0.0,
			"baseline current_portfolio_value should be positive")
	})

	// Step 3: Add external balances
	extBalanceAmount := 75000.0
	t.Run("add_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
			"type":  "cash",
			"label": "Capital Perf Test Cash",
			"value": extBalanceAmount,
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 4: Get capital performance after adding external balance
	t.Run("current_value_includes_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_perf_with_ext_balance", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		newCurrentValue := perf["current_portfolio_value"].(float64)

		// CurrentPortfolioValue should have increased by the external balance amount
		assert.InDelta(t, baselineCurrentValue+extBalanceAmount, newCurrentValue, 1.0,
			"current_portfolio_value should increase by the external balance amount")

		// It should be strictly greater than the baseline
		assert.Greater(t, newCurrentValue, baselineCurrentValue,
			"current_portfolio_value should be greater after adding external balance")
	})

	// Step 5: Verify embedded capital_performance matches standalone
	t.Run("embedded_matches_standalone_with_ext_balance", func(t *testing.T) {
		// Get portfolio response (embedded)
		resp1, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp1.Body.Close()

		body1, _ := io.ReadAll(resp1.Body)
		guard.SaveResult("03_portfolio_with_ext_balance", string(body1))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body1, &portfolio))

		require.Contains(t, portfolio, "capital_performance",
			"capital_performance should be present")
		embedded := portfolio["capital_performance"].(map[string]interface{})

		// Get standalone performance
		resp2, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp2.Body.Close()

		body2, _ := io.ReadAll(resp2.Body)
		var standalone map[string]interface{}
		require.NoError(t, json.Unmarshal(body2, &standalone))

		// current_portfolio_value should match between embedded and standalone
		assert.InDelta(t,
			standalone["current_portfolio_value"].(float64),
			embedded["current_portfolio_value"].(float64),
			0.01,
			"current_portfolio_value should match between embedded and standalone")
	})

	// Step 6: Verify current_portfolio_value matches get_portfolio total_value
	t.Run("current_value_matches_portfolio_total_value", func(t *testing.T) {
		// Get portfolio
		resp1, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp1.Body.Close()

		body1, _ := io.ReadAll(resp1.Body)
		guard.SaveResult("04_portfolio_total_value", string(body1))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body1, &portfolio))

		portfolioTotalValue := portfolio["total_value"].(float64)
		portfolioExternalTotal := portfolio["external_balance_total"].(float64)
		portfolioHoldingsValue := portfolio["total_value_holdings"].(float64)

		// Sanity: external balance should be present
		assert.Equal(t, extBalanceAmount, portfolioExternalTotal,
			"external_balance_total should match what was added")

		// total_value = holdings + external
		assert.InDelta(t, portfolioHoldingsValue+portfolioExternalTotal, portfolioTotalValue, 0.01,
			"total_value should equal total_value_holdings + external_balance_total")

		// Get capital performance
		resp2, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp2.Body.Close()

		body2, _ := io.ReadAll(resp2.Body)
		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body2, &perf))

		perfCurrentValue := perf["current_portfolio_value"].(float64)

		// capital_performance.current_portfolio_value should match portfolio.total_value
		assert.InDelta(t, portfolioTotalValue, perfCurrentValue, 1.0,
			"capital_performance.current_portfolio_value should match portfolio.total_value (holdings + external balances)")
	})

	// Step 7: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		// Remove external balances
		resp, err := env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{
				"external_balances": []map[string]interface{}{},
			}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()

		// Remove cash transactions
		resp, err = env.HTTPRequest(http.MethodGet, basePath+"/cashflows", nil, userHeaders)
		require.NoError(t, err)

		var ledger map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if json.Unmarshal(body, &ledger) == nil {
			if txns, ok := ledger["transactions"].([]interface{}); ok {
				for _, tx := range txns {
					txMap := tx.(map[string]interface{})
					txID := txMap["id"].(string)
					r, e := env.HTTPRequest(http.MethodDelete, basePath+"/cashflows/"+txID, nil, userHeaders)
					if e == nil {
						r.Body.Close()
					}
				}
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Capital performance after sync preserves external balance inclusion ---

func TestCapitalPerformance_PreservedAcrossSync(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Add cash transaction and external balance
	t.Run("setup", func(t *testing.T) {
		// Add cash transaction
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      150000,
			"description": "Deposit for sync persistence test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Add external balance
		resp, err = env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
			"type":  "accumulate",
			"label": "Sync Persist Test",
			"value": 50000,
			"rate":  0.04,
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 2: Capture capital performance before sync
	var preSyncValue float64
	t.Run("pre_sync_performance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_pre_sync_perf", string(body))

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		preSyncValue = perf["current_portfolio_value"].(float64)
		assert.Greater(t, preSyncValue, 0.0)
	})

	// Step 3: Force portfolio sync
	t.Run("force_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_sync_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))
	})

	// Step 4: Verify capital performance after sync still includes external balances
	t.Run("post_sync_performance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_post_sync_perf", string(body))

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		postSyncValue := perf["current_portfolio_value"].(float64)

		// Value should be approximately the same (market prices may shift slightly
		// between calls, but external balance component should be preserved)
		assert.InDelta(t, preSyncValue, postSyncValue, preSyncValue*0.05,
			"current_portfolio_value should be approximately the same after sync (within 5%%)")

		// Verify it still includes external balances by checking against portfolio
		resp2, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp2.Body.Close()

		body2, _ := io.ReadAll(resp2.Body)
		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body2, &portfolio))

		portfolioTotalValue := portfolio["total_value"].(float64)
		externalTotal := portfolio["external_balance_total"].(float64)

		// External balance should still be there after sync
		assert.Equal(t, 50000.0, externalTotal,
			"external_balance_total should be preserved after sync")

		// current_portfolio_value should match total_value
		assert.InDelta(t, portfolioTotalValue, postSyncValue, 1.0,
			"current_portfolio_value should match portfolio.total_value after sync")
	})

	// Step 5: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{
				"external_balances": []map[string]interface{}{},
			}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()

		resp, err = env.HTTPRequest(http.MethodGet, basePath+"/cashflows", nil, userHeaders)
		require.NoError(t, err)

		var ledger map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if json.Unmarshal(body, &ledger) == nil {
			if txns, ok := ledger["transactions"].([]interface{}); ok {
				for _, tx := range txns {
					txMap := tx.(map[string]interface{})
					txID := txMap["id"].(string)
					r, e := env.HTTPRequest(http.MethodDelete, basePath+"/cashflows/"+txID, nil, userHeaders)
					if e == nil {
						r.Body.Close()
					}
				}
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Multiple external balances contribute to CurrentPortfolioValue ---

func TestCapitalPerformance_MultipleExternalBalances(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Add cash transaction
	t.Run("add_cash_transaction", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      100000,
			"description": "Deposit for multi-balance test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 2: Get baseline
	var baselineValue float64
	t.Run("baseline", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_baseline", string(body))

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))
		baselineValue = perf["current_portfolio_value"].(float64)
	})

	// Step 3: Add multiple external balances
	totalExternal := 0.0
	balances := []struct {
		balType string
		label   string
		value   float64
	}{
		{"cash", "Multi Test Cash", 25000},
		{"accumulate", "Multi Test Accumulate", 50000},
		{"term_deposit", "Multi Test TD", 100000},
	}

	t.Run("add_multiple_external_balances", func(t *testing.T) {
		for _, b := range balances {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
				"type":  b.balType,
				"label": b.label,
				"value": b.value,
			}, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
			totalExternal += b.value
		}
	})

	// Step 4: Verify current_portfolio_value reflects all external balances
	t.Run("value_includes_all_external_balances", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cashflows/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_multi_ext_perf", string(body))

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		newValue := perf["current_portfolio_value"].(float64)

		// Should have increased by total external balance amount
		assert.InDelta(t, baselineValue+totalExternal, newValue, 1.0,
			"current_portfolio_value should increase by total external balance (%v)", totalExternal)

		// Cross-check with portfolio total_value
		resp2, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp2.Body.Close()

		body2, _ := io.ReadAll(resp2.Body)
		guard.SaveResult("03_multi_ext_portfolio", string(body2))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body2, &portfolio))

		assert.InDelta(t, totalExternal, portfolio["external_balance_total"].(float64), 0.01,
			"external_balance_total should equal sum of all added balances")

		assert.InDelta(t, portfolio["total_value"].(float64), newValue, 1.0,
			"capital_performance.current_portfolio_value should match portfolio.total_value")
	})

	// Step 5: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{
				"external_balances": []map[string]interface{}{},
			}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()

		resp, err = env.HTTPRequest(http.MethodGet, basePath+"/cashflows", nil, userHeaders)
		require.NoError(t, err)

		var ledger map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if json.Unmarshal(body, &ledger) == nil {
			if txns, ok := ledger["transactions"].([]interface{}); ok {
				for _, tx := range txns {
					txMap := tx.(map[string]interface{})
					txID := txMap["id"].(string)
					r, e := env.HTTPRequest(http.MethodDelete, basePath+"/cashflows/"+txID, nil, userHeaders)
					if e == nil {
						r.Body.Close()
					}
				}
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
