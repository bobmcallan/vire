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

// --- Helpers ---

// setupPortfolioForCashFlows imports a test user, sets the Navexa key,
// and triggers a portfolio sync so that cash flow endpoints have a
// portfolio record to operate on. Returns the portfolio name and user headers.
// Skips the test if NAVEXA_API_KEY or DEFAULT_PORTFOLIO are not set.
func setupPortfolioForCashFlows(t *testing.T, env *common.Env) (string, map[string]string) {
	t.Helper()
	return setupPortfolioForExternalBalances(t, env)
}

// postCashTransaction is a test helper that POSTs a cash transaction and returns the decoded response.
func postCashTransaction(t *testing.T, env *common.Env, portfolioName string, headers map[string]string, body map[string]interface{}) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-transactions", body, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))
	return result, resp.StatusCode
}

// getCashFlows is a test helper that GETs cash flow ledger and decodes the response.
func getCashFlows(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))
	return result, resp.StatusCode
}

// getCashFlowPerformance is a test helper that GETs capital performance metrics.
func getCashFlowPerformance(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))
	return result, resp.StatusCode
}

// --- CRUD Lifecycle ---

func TestCashFlowCRUDLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	// Step 1: GET -- initially empty ledger
	t.Run("get_empty", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)
		txns := result["transactions"].([]interface{})
		assert.Empty(t, txns)
	})

	// Step 2: POST -- add deposit (contribution)
	var depositID string
	t.Run("add_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        "2025-01-15T00:00:00Z",
			"amount":      50000,
			"description": "Initial SMSF deposit",
			"notes":       "Opening deposit from rollover",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_add_deposit", string(body))

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Verify the ledger is returned with the new transaction
		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 1)

		tx := txns[0].(map[string]interface{})
		assert.Contains(t, tx["id"], "ct_", "ID should have ct_ prefix")
		assert.Equal(t, "contribution", tx["category"])
		assert.Equal(t, 50000.0, tx["amount"])
		assert.Equal(t, "Initial SMSF deposit", tx["description"])
		assert.Equal(t, "Opening deposit from rollover", tx["notes"])

		depositID = tx["id"].(string)
		assert.True(t, len(depositID) > 0, "ID should not be empty")
	})

	// Step 3: POST -- add contribution
	var contributionID string
	t.Run("add_contribution", func(t *testing.T) {
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        "2025-02-15T00:00:00Z",
			"amount":      10000,
			"description": "Employer contribution Q1",
		})
		assert.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 2)

		// Find the contribution (transactions sorted by date ascending)
		tx := txns[1].(map[string]interface{})
		assert.Equal(t, "contribution", tx["category"])
		contributionID = tx["id"].(string)
	})

	// Step 4: POST -- add withdrawal (negative contribution)
	t.Run("add_withdrawal", func(t *testing.T) {
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"category":    "other",
			"account":     "Trading",
			"date":        "2025-03-01T00:00:00Z",
			"amount":      -5000,
			"description": "Admin expense withdrawal",
		})
		assert.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 3)
	})

	// Step 5: POST -- add dividend
	t.Run("add_dividend", func(t *testing.T) {
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"category":    "dividend",
			"account":     "Trading",
			"date":        "2025-03-15T00:00:00Z",
			"amount":      1200,
			"description": "BHP interim dividend",
		})
		assert.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 4)
	})

	// Step 6: GET -- verify all transactions present and sorted by date ascending
	t.Run("get_all_transactions", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_get_all", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 4)

		// Verify date ascending ordering by category
		categories := make([]string, len(txns))
		for i, tx := range txns {
			categories[i] = tx.(map[string]interface{})["category"].(string)
		}
		assert.Equal(t, "contribution", categories[0])
		assert.Equal(t, "contribution", categories[1])
		assert.Equal(t, "other", categories[2])
		assert.Equal(t, "dividend", categories[3])
	})

	// Step 7: PUT -- update the contribution amount and description
	t.Run("update_contribution", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath+"/"+contributionID, map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        "2025-02-15T00:00:00Z",
			"amount":      12000,
			"description": "Employer contribution Q1 (corrected)",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_update_contribution", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		txns := result["transactions"].([]interface{})
		// Find the updated transaction
		var found bool
		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			if txMap["id"] == contributionID {
				assert.Equal(t, 12000.0, txMap["amount"])
				assert.Equal(t, "Employer contribution Q1 (corrected)", txMap["description"])
				found = true
				break
			}
		}
		assert.True(t, found, "updated transaction should be in ledger")
	})

	// Step 8: DELETE -- remove the deposit
	t.Run("delete_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodDelete, basePath+"/"+depositID, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Step 9: GET -- verify deposit removed
	t.Run("verify_delete", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_after_delete", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		txns := result["transactions"].([]interface{})
		assert.Len(t, txns, 3)

		// Verify the deposit ID is gone
		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			assert.NotEqual(t, depositID, txMap["id"], "deleted deposit should not appear")
		}
	})

	// Step 10: GET performance
	t.Run("get_performance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("05_performance", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Verify structure of performance response
		assert.Contains(t, result, "total_deposited")
		assert.Contains(t, result, "total_withdrawn")
		assert.Contains(t, result, "net_capital_deployed")
		assert.Contains(t, result, "current_portfolio_value")
		assert.Contains(t, result, "simple_return_pct")
		assert.Contains(t, result, "annualized_return_pct")
		assert.Contains(t, result, "transaction_count")

		// With 3 remaining transactions: contribution(12000) + dividend(1200) + other(-5000)
		// total_deposited = sum of positive contribution amounts = 12000
		// total_withdrawn = sum of negative contribution amounts = 0 (the -5000 is "other", not "contribution")
		assert.Equal(t, float64(3), result["transaction_count"])
		assert.InDelta(t, 12000.0, result["total_deposited"].(float64), 0.01)
		assert.InDelta(t, 0.0, result["total_withdrawn"].(float64), 0.01)
		assert.InDelta(t, 12000.0, result["net_capital_deployed"].(float64), 0.01)

		// current_portfolio_value should be > 0 (equity + external balances)
		assert.Greater(t, result["current_portfolio_value"].(float64), 0.0)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- All Transaction Types ---

func TestCashFlowAllTransactionTypes(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	categories := []struct {
		category    string
		description string
		amount      float64
	}{
		{"contribution", "Cash deposit", 50000},
		{"contribution", "Employer contribution", 10000},
		{"dividend", "BHP dividend", 1200},
		{"fee", "Brokerage fee", -29.50},
		{"other", "Misc credit", 500},
	}

	for _, tt := range categories {
		t.Run(tt.category+"_"+tt.description, func(t *testing.T) {
			resp, err := env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
				"category":    tt.category,
				"account":     "Trading",
				"date":        "2025-06-15T00:00:00Z",
				"amount":      tt.amount,
				"description": tt.description,
			}, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("category_"+tt.category, string(body))

			assert.Equal(t, http.StatusCreated, resp.StatusCode)
		})
	}

	// Verify all five are present
	t.Run("verify_all_present", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)

		txns := result["transactions"].([]interface{})
		assert.Len(t, txns, 5)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Validation Tests ---

func TestCashFlowValidation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name: "invalid_category",
			body: map[string]interface{}{
				"category":    "savings",
				"account":     "Trading",
				"date":        "2025-01-15T00:00:00Z",
				"amount":      1000,
				"description": "Bad category",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "empty_category",
			body: map[string]interface{}{
				"category":    "",
				"account":     "Trading",
				"date":        "2025-01-15T00:00:00Z",
				"amount":      1000,
				"description": "Empty category",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "zero_amount",
			body: map[string]interface{}{
				"category":    "contribution",
				"account":     "Trading",
				"date":        "2025-01-15T00:00:00Z",
				"amount":      0,
				"description": "Zero amount",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "empty_description",
			body: map[string]interface{}{
				"category":    "contribution",
				"account":     "Trading",
				"date":        "2025-01-15T00:00:00Z",
				"amount":      1000,
				"description": "",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "whitespace_description",
			body: map[string]interface{}{
				"category":    "contribution",
				"account":     "Trading",
				"date":        "2025-01-15T00:00:00Z",
				"amount":      1000,
				"description": "   ",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_date",
			body: map[string]interface{}{
				"category":    "contribution",
				"account":     "Trading",
				"amount":      1000,
				"description": "No date",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "future_date",
			body: map[string]interface{}{
				"category":    "contribution",
				"account":     "Trading",
				"date":        "2099-01-15T00:00:00Z",
				"amount":      1000,
				"description": "Future date",
			},
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing_account",
			body: map[string]interface{}{
				"category":    "contribution",
				"date":        "2025-01-15T00:00:00Z",
				"amount":      1000,
				"description": "No account",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPRequest(http.MethodPost, basePath, tt.body, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("validation_"+tt.name, string(body))

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Not Found Tests ---

func TestCashFlowNotFound(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// GET cashflows for non-existent portfolio
	t.Run("get_nonexistent_portfolio", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/portfolios/nonexistent/cash-transactions")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("notfound_get", string(body))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// GET performance for non-existent portfolio
	t.Run("performance_nonexistent_portfolio", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/portfolios/nonexistent/cash-transactions/performance")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("notfound_performance", string(body))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// DELETE non-existent transaction ID from non-existent portfolio
	t.Run("delete_nonexistent_portfolio", func(t *testing.T) {
		resp, err := env.HTTPDelete("/api/portfolios/nonexistent/cash-transactions/ct_00000000")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Delete Non-Existent Transaction ID ---

func TestCashFlowDeleteNonExistentID(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	// Delete a transaction ID that doesn't exist in a real portfolio
	resp, err := env.HTTPRequest(http.MethodDelete, "/api/portfolios/"+portfolioName+"/cash-transactions/ct_nonexistent", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("delete_nonexistent_id", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Update Non-Existent Transaction ID ---

func TestCashFlowUpdateNonExistentID(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	resp, err := env.HTTPRequest(http.MethodPut, "/api/portfolios/"+portfolioName+"/cash-transactions/ct_nonexistent", map[string]interface{}{
		"category":    "contribution",
		"account":     "Trading",
		"date":        "2025-01-15T00:00:00Z",
		"amount":      1000,
		"description": "Update to non-existent",
	}, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("update_nonexistent_id", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Performance With Empty Ledger ---

func TestCashFlowPerformanceEmpty(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	// Performance with no manual cash transactions auto-derives from trade history.
	// The test portfolio has real Navexa trade data, so performance should be non-empty.
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("performance_empty", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	// Verify structure is present — values come from auto-derived trade history
	assert.Contains(t, result, "total_deposited")
	assert.Contains(t, result, "total_withdrawn")
	assert.Contains(t, result, "net_capital_deployed")
	assert.Contains(t, result, "transaction_count")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Performance Calculation Correctness ---

func TestCashFlowPerformanceCalculation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	// Add known transactions for predictable totals
	inflows := []struct {
		txType      string
		amount      float64
		description string
		date        string
	}{
		{"deposit", 100000, "Initial deposit", "2024-01-15T00:00:00Z"},
		{"contribution", 25000, "Q1 contribution", "2024-04-01T00:00:00Z"},
		{"dividend", 3000, "BHP dividend", "2024-06-15T00:00:00Z"},
		{"transfer_in", 10000, "From accumulate", "2024-09-01T00:00:00Z"},
	}

	outflows := []struct {
		txType      string
		amount      float64
		description string
		date        string
	}{
		{"withdrawal", 15000, "Tax payment", "2024-07-01T00:00:00Z"},
		{"transfer_out", 5000, "To accumulate", "2024-10-01T00:00:00Z"},
	}

	for _, in := range inflows {
		_, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"type":        in.txType,
			"date":        in.date,
			"amount":      in.amount,
			"description": in.description,
		})
		require.Equal(t, http.StatusCreated, status)
	}

	for _, out := range outflows {
		_, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"type":        out.txType,
			"date":        out.date,
			"amount":      out.amount,
			"description": out.description,
		})
		require.Equal(t, http.StatusCreated, status)
	}

	// Get performance
	resp, err := env.HTTPRequest(http.MethodGet, basePath+"/performance", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("performance_calculated", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	// Verify totals: deposits(100000) + contributions(25000) + dividends(3000) + transfer_in(10000) = 138000
	assert.InDelta(t, 138000.0, result["total_deposited"].(float64), 0.01)

	// Withdrawals(15000) + transfer_out(5000) = 20000
	assert.InDelta(t, 20000.0, result["total_withdrawn"].(float64), 0.01)

	// Net capital deployed = 138000 - 20000 = 118000
	assert.InDelta(t, 118000.0, result["net_capital_deployed"].(float64), 0.01)

	assert.Equal(t, float64(6), result["transaction_count"])

	// current_portfolio_value should be set (equity + external balances)
	assert.Greater(t, result["current_portfolio_value"].(float64), 0.0)

	// simple_return_pct should be numeric (sign depends on portfolio value vs net capital)
	_, ok := result["simple_return_pct"].(float64)
	assert.True(t, ok, "simple_return_pct should be a number")

	// annualized_return_pct should be numeric
	_, ok = result["annualized_return_pct"].(float64)
	assert.True(t, ok, "annualized_return_pct should be a number")

	// first_transaction_date should be set
	assert.NotNil(t, result["first_transaction_date"])

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Persistence Across Portfolio Sync ---

func TestCashFlowPersistenceAcrossSync(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	// Add transactions
	t.Run("add_transactions", func(t *testing.T) {
		_, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        "2025-01-01T00:00:00Z",
			"amount":      50000,
			"description": "Persist test deposit",
		})
		require.Equal(t, http.StatusCreated, status)

		_, status = postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"category":    "contribution",
			"account":     "Trading",
			"date":        "2025-02-01T00:00:00Z",
			"amount":      10000,
			"description": "Persist test contribution",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// Verify before sync
	t.Run("verify_before_sync", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)

		txns := result["transactions"].([]interface{})
		assert.Len(t, txns, 2)
	})

	// Force portfolio sync
	t.Run("force_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("sync_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))
	})

	// Verify cash flows preserved after sync
	t.Run("verify_after_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("after_sync", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		txns := result["transactions"].([]interface{})
		assert.Len(t, txns, 2, "cash flow transactions should be preserved across sync")

		// Verify transaction details intact
		descriptions := make([]string, len(txns))
		for i, tx := range txns {
			descriptions[i] = tx.(map[string]interface{})["description"].(string)
		}
		assert.Contains(t, descriptions, "Persist test deposit")
		assert.Contains(t, descriptions, "Persist test contribution")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Response Summary ---

// TestCashFlowResponseSummary verifies that the cash flow response includes the
// redesigned summary object and per-account balances.
//
// Per requirements in .claude/workdir/20260301-cash-summary-redesign/requirements.md:
//   - GET /api/portfolios/{name}/cash-transactions returns summary.total_cash and summary.by_category
//   - Each account in the accounts array has a balance field
//   - Transfers net to zero in by_category.transfer
//   - POST response also includes the redesigned summary
func TestCashFlowResponseSummary(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	// Known amounts for predictable assertions.
	const contribution1 = 50000.0
	const contribution2 = 10000.0
	const fee1 = 5000.0 // will be posted as -5000
	// Transfer pair: from Trading to Accumulate
	const transferAmount = 8000.0
	const expectedTotalCash = contribution1 + contribution2 - fee1 // 55000 (transfers net to zero)
	const expectedCount = 4                                        // 2 contributions + 1 fee + 2 transfer legs = but we count 4 here before transfer

	// Step 1: Add contribution and POST response has redesigned summary.

	t.Run("add_contribution_summary", func(t *testing.T) {
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        "2025-01-15T00:00:00Z",
			"amount":      contribution1,
			"description": "Summary test contribution 1",
		})
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		// POST response must include redesigned summary.
		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "POST response should contain summary object")
		assert.InDelta(t, contribution1, summary["total_cash"], 0.01,
			"total_cash should equal the contribution amount")
		assert.Equal(t, float64(1), summary["transaction_count"])

		byCategory, ok := summary["by_category"].(map[string]interface{})
		require.True(t, ok, "summary must contain by_category object")
		assert.InDelta(t, contribution1, byCategory["contribution"], 0.01,
			"by_category.contribution should equal the contribution")
		assert.Equal(t, 0.0, byCategory["fee"], "by_category.fee should be 0")
	})

	t.Run("add_contribution_2", func(t *testing.T) {
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        "2025-02-15T00:00:00Z",
			"amount":      contribution2,
			"description": "Summary test contribution 2",
		})
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "POST response should contain summary after second contribution")
		assert.InDelta(t, contribution1+contribution2, summary["total_cash"], 0.01)
		assert.Equal(t, float64(2), summary["transaction_count"])

		byCategory, ok := summary["by_category"].(map[string]interface{})
		require.True(t, ok, "summary must contain by_category object")
		assert.InDelta(t, contribution1+contribution2, byCategory["contribution"], 0.01)
	})

	t.Run("add_fee", func(t *testing.T) {
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "fee",
			"date":        "2025-03-01T00:00:00Z",
			"amount":      -fee1, // negative = money out
			"description": "Summary test fee",
		})
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "POST response should contain summary after fee")
		assert.InDelta(t, contribution1+contribution2-fee1, summary["total_cash"], 0.01,
			"total_cash should reflect the fee deduction")
		assert.Equal(t, float64(3), summary["transaction_count"])

		byCategory, ok := summary["by_category"].(map[string]interface{})
		require.True(t, ok, "summary must contain by_category object")
		assert.InDelta(t, -fee1, byCategory["fee"], 0.01,
			"by_category.fee should be negative (money out)")
	})

	// Step 2: GET cash-transactions and verify full redesigned summary.

	t.Run("get_summary_fields", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_get_with_summary", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Summary object must be present and correctly typed.
		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "response must contain a 'summary' object")

		assert.InDelta(t, contribution1+contribution2-fee1, summary["total_cash"], 0.01,
			"total_cash should equal net of all amounts")
		assert.Equal(t, float64(3), summary["transaction_count"],
			"transaction_count should equal total number of transactions")

		byCategory, ok := summary["by_category"].(map[string]interface{})
		require.True(t, ok, "summary must contain by_category")
		assert.InDelta(t, contribution1+contribution2, byCategory["contribution"], 0.01,
			"by_category.contribution should sum all contributions")
		assert.InDelta(t, -fee1, byCategory["fee"], 0.01,
			"by_category.fee should be negative net")
		// All 5 categories must be present.
		for _, cat := range []string{"contribution", "dividend", "transfer", "fee", "other"} {
			_, exists := byCategory[cat]
			assert.True(t, exists, "by_category must contain key %q", cat)
		}
	})

	// Step 3: Verify per-account balance fields in accounts array.

	t.Run("accounts_have_balance_field", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_accounts_with_balance", string(body))

		accounts, ok := result["accounts"].([]interface{})
		require.True(t, ok, "accounts must be an array")
		require.NotEmpty(t, accounts, "accounts array must not be empty")

		for _, rawAccount := range accounts {
			acct, ok := rawAccount.(map[string]interface{})
			require.True(t, ok, "each account must be an object")
			_, hasBalance := acct["balance"]
			assert.True(t, hasBalance, "each account must have a balance field, got: %v", acct)
		}

		// Trading account balance should equal contributions minus fee.
		for _, rawAccount := range accounts {
			acct := rawAccount.(map[string]interface{})
			if acct["name"] == "Trading" {
				expectedBalance := contribution1 + contribution2 - fee1
				assert.InDelta(t, expectedBalance, acct["balance"], 0.01,
					"Trading account balance should be net of contributions minus fee")
			}
		}
	})

	// Step 4: Verify existing top-level fields are still present alongside summary.

	t.Run("existing_fields_preserved", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("03_existing_fields", string(body))

		assert.Contains(t, result, "portfolio_name", "portfolio_name must be present")
		assert.Contains(t, result, "accounts", "accounts must be present")
		assert.Contains(t, result, "transactions", "transactions must be present")
		assert.Contains(t, result, "summary", "summary must be present")

		assert.Equal(t, portfolioName, result["portfolio_name"])

		txns, ok := result["transactions"].([]interface{})
		require.True(t, ok, "transactions must be an array")
		assert.Len(t, txns, 3, "should have 3 transactions")
	})

	// Step 5: Add a transfer pair and verify transfers net to zero in by_category.

	t.Run("transfer_pair_nets_to_zero", func(t *testing.T) {
		// Transfer debit from Trading.
		_, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "transfer",
			"date":        "2025-04-01T00:00:00Z",
			"amount":      -transferAmount,
			"description": "Transfer out of Trading",
		})
		require.Equal(t, http.StatusCreated, status)

		// Transfer credit to Accumulate.
		result, status := postCashTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Accumulate",
			"category":    "transfer",
			"date":        "2025-04-01T00:00:00Z",
			"amount":      transferAmount,
			"description": "Transfer into Accumulate",
		})
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("04_after_transfer", string(body))

		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "POST response should contain summary after transfer")

		byCategory, ok := summary["by_category"].(map[string]interface{})
		require.True(t, ok, "summary must contain by_category")

		// Transfer pair nets to zero.
		assert.InDelta(t, 0.0, byCategory["transfer"], 0.01,
			"by_category.transfer should net to zero for a paired transfer")

		// total_cash unchanged by internal transfers.
		assert.InDelta(t, contribution1+contribution2-fee1, summary["total_cash"], 0.01,
			"total_cash should be unchanged by internal transfer")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
