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

// setupPortfolioForExternalBalances imports a test user, sets the Navexa key,
// and triggers a portfolio sync so that external balance endpoints have a
// portfolio record to operate on.  Returns the portfolio name and user headers.
// Skips the test if NAVEXA_API_KEY or DEFAULT_PORTFOLIO are not set.
func setupPortfolioForExternalBalances(t *testing.T, env *common.Env) (string, map[string]string) {
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

// getExternalBalances is a test helper that GETs external balances and decodes the response.
func getExternalBalances(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) ([]interface{}, float64, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/external-balances", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, 0, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	balances := result["external_balances"].([]interface{})
	total := result["total"].(float64)
	return balances, total, resp.StatusCode
}

// --- CRUD Lifecycle ---

func TestExternalBalanceCRUDLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForExternalBalances(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/external-balances"

	// Step 1: GET — initially empty
	t.Run("get_empty", func(t *testing.T) {
		balances, total, status := getExternalBalances(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)
		assert.Empty(t, balances)
		assert.Equal(t, 0.0, total)
	})

	// Step 2: POST — add first external balance (cash)
	var firstID string
	t.Run("add_cash_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
			"type":  "cash",
			"label": "ANZ Cash",
			"value": 44000,
			"notes": "SMSF cash account",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_add_cash", string(body))

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Contains(t, result, "id")
		assert.Equal(t, "cash", result["type"])
		assert.Equal(t, "ANZ Cash", result["label"])
		assert.Equal(t, 44000.0, result["value"])
		assert.Equal(t, "SMSF cash account", result["notes"])

		firstID = result["id"].(string)
		assert.True(t, len(firstID) > 0, "ID should not be empty")
		assert.Contains(t, firstID, "eb_", "ID should have eb_ prefix")
	})

	// Step 3: POST — add second external balance (accumulate)
	var secondID string
	t.Run("add_accumulate_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
			"type":  "accumulate",
			"label": "Stake Accumulate",
			"value": 50000,
			"rate":  0.05,
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_add_accumulate", string(body))

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		secondID = result["id"].(string)
		assert.Equal(t, "accumulate", result["type"])
		assert.Equal(t, 50000.0, result["value"])
		assert.Equal(t, 0.05, result["rate"])
	})

	// Step 4: GET all — verify both present
	t.Run("get_both", func(t *testing.T) {
		balances, total, status := getExternalBalances(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)
		assert.Len(t, balances, 2)
		assert.Equal(t, 94000.0, total)
	})

	// Step 5: PUT — replace all with a new set
	t.Run("set_replace_all", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath, map[string]interface{}{
			"external_balances": []map[string]interface{}{
				{"type": "term_deposit", "label": "CBA Term Deposit", "value": 100000, "rate": 0.045},
				{"type": "offset", "label": "Home Offset", "value": 200000},
			},
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_set_replace", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		balances := result["external_balances"].([]interface{})
		assert.Len(t, balances, 2)
		assert.Equal(t, 300000.0, result["total"])

		// Verify the old balances are gone
		for _, b := range balances {
			bal := b.(map[string]interface{})
			assert.NotEqual(t, firstID, bal["id"])
			assert.NotEqual(t, secondID, bal["id"])
		}
	})

	// Step 6: GET — capture new IDs for delete test
	var deleteID string
	t.Run("get_after_replace", func(t *testing.T) {
		balances, total, status := getExternalBalances(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)
		assert.Len(t, balances, 2)
		assert.Equal(t, 300000.0, total)

		// Pick first balance for deletion
		first := balances[0].(map[string]interface{})
		deleteID = first["id"].(string)
	})

	// Step 7: DELETE one by ID
	t.Run("delete_one", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodDelete, basePath+"/"+deleteID, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Step 8: GET — verify final state (one remaining)
	t.Run("get_final", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_final_state", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		balances := result["external_balances"].([]interface{})
		assert.Len(t, balances, 1)

		remaining := balances[0].(map[string]interface{})
		assert.NotEqual(t, deleteID, remaining["id"])
	})

	// Step 9: PUT empty — clear all
	t.Run("set_empty_clears_all", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath, map[string]interface{}{
			"external_balances": []map[string]interface{}{},
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("05_clear_all", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		balances := result["external_balances"].([]interface{})
		assert.Empty(t, balances)
		assert.Equal(t, 0.0, result["total"])
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Validation Tests ---

func TestExternalBalanceValidation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForExternalBalances(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/external-balances"

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "invalid_type",
			body:       map[string]interface{}{"type": "savings", "label": "Bad", "value": 1000},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_type",
			body:       map[string]interface{}{"type": "", "label": "Bad", "value": 1000},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_label",
			body:       map[string]interface{}{"type": "cash", "label": "", "value": 1000},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "whitespace_only_label",
			body:       map[string]interface{}{"type": "cash", "label": "   ", "value": 1000},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "negative_value",
			body:       map[string]interface{}{"type": "cash", "label": "Cash", "value": -100},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "negative_rate",
			body:       map[string]interface{}{"type": "cash", "label": "Cash", "value": 1000, "rate": -0.05},
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

	// PUT with invalid balances in the array
	t.Run("put_invalid_balance_in_set", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath, map[string]interface{}{
			"external_balances": []map[string]interface{}{
				{"type": "cash", "label": "Good", "value": 1000},
				{"type": "bad_type", "label": "Bad", "value": 500},
			},
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Not Found Tests ---

func TestExternalBalanceNotFound(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// These tests operate on a non-existent portfolio, so no user headers needed
	// (the portfolio lookup will fail before user context matters).

	// GET external balances for non-existent portfolio — expect 404
	t.Run("get_nonexistent_portfolio", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/portfolios/nonexistent/external-balances")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("notfound_get", string(body))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	// POST to non-existent portfolio — returns 500 because the error is about
	// loading the portfolio, not about external balance validation
	t.Run("post_nonexistent_portfolio", func(t *testing.T) {
		resp, err := env.HTTPPost("/api/portfolios/nonexistent/external-balances", map[string]interface{}{
			"type":  "cash",
			"label": "Cash",
			"value": 1000,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	// DELETE non-existent balance ID from non-existent portfolio
	t.Run("delete_nonexistent_portfolio", func(t *testing.T) {
		resp, err := env.HTTPDelete("/api/portfolios/nonexistent/external-balances/eb_00000000")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Delete Non-Existent Balance ID ---

func TestExternalBalanceDeleteNonExistent(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForExternalBalances(t, env)

	// Delete a balance ID that doesn't exist in a real portfolio
	resp, err := env.HTTPRequest(http.MethodDelete, "/api/portfolios/"+portfolioName+"/external-balances/eb_nonexistent", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("delete_nonexistent_id", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Persistence Across Sync ---

func TestExternalBalancePersistenceAcrossSync(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForExternalBalances(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/external-balances"

	// Add external balances
	t.Run("add_balances", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
			"type":  "cash",
			"label": "Persist Test Cash",
			"value": 25000,
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		resp, err = env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
			"type":  "accumulate",
			"label": "Persist Test Accumulate",
			"value": 75000,
			"rate":  0.04,
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Verify balances present
	t.Run("verify_before_sync", func(t *testing.T) {
		balances, total, status := getExternalBalances(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)
		assert.Len(t, balances, 2)
		assert.Equal(t, 100000.0, total)
	})

	// Force a portfolio re-sync via the sync endpoint
	t.Run("force_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("sync_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))
	})

	// Verify external balances are preserved after sync
	t.Run("verify_after_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("after_sync", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		balances := result["external_balances"].([]interface{})
		assert.Len(t, balances, 2, "external balances should be preserved across sync")
		assert.Equal(t, 100000.0, result["total"])

		// Verify balance details are intact
		labels := make([]string, len(balances))
		for i, b := range balances {
			labels[i] = b.(map[string]interface{})["label"].(string)
		}
		assert.Contains(t, labels, "Persist Test Cash")
		assert.Contains(t, labels, "Persist Test Accumulate")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Weight Recalculation ---

func TestExternalBalanceWeightRecalculation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForExternalBalances(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/external-balances"

	// Get portfolio before adding external balances — capture initial weights
	var initialWeightSum float64
	t.Run("get_initial_weights", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("initial_portfolio", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		holdings := portfolio["holdings"].([]interface{})
		require.NotEmpty(t, holdings, "portfolio should have holdings")

		for _, h := range holdings {
			holding := h.(map[string]interface{})
			initialWeightSum += holding["weight"].(float64)
		}

		// Without external balances, weights should sum close to 100%
		assert.InDelta(t, 100.0, initialWeightSum, 1.0, "initial weights should sum to ~100%%")
	})

	// Add a significant external balance
	t.Run("add_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath, map[string]interface{}{
			"type":  "cash",
			"label": "Weight Test Cash",
			"value": 500000,
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Get portfolio after adding external balance — weights should be reduced
	t.Run("verify_reduced_weights", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("portfolio_with_ext_balance", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		holdings := portfolio["holdings"].([]interface{})
		var newWeightSum float64
		for _, h := range holdings {
			holding := h.(map[string]interface{})
			newWeightSum += holding["weight"].(float64)
		}

		// Weights should sum to less than 100% because external balance is in the denominator
		assert.Less(t, newWeightSum, 100.0, "weights should sum to less than 100%% with external balance")

		// The external_balance_total should be present
		assert.Equal(t, 500000.0, portfolio["external_balance_total"])
	})

	// Remove external balance — weights should return to original
	t.Run("remove_and_verify_restored_weights", func(t *testing.T) {
		// Get the balance ID
		balances, _, status := getExternalBalances(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)
		require.Len(t, balances, 1)
		balanceID := balances[0].(map[string]interface{})["id"].(string)

		// Delete it
		resp, err := env.HTTPRequest(http.MethodDelete, basePath+"/"+balanceID, nil, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusNoContent, resp.StatusCode)

		// Get portfolio again
		resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("portfolio_after_ext_removed", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		holdings := portfolio["holdings"].([]interface{})
		var restoredWeightSum float64
		for _, h := range holdings {
			holding := h.(map[string]interface{})
			restoredWeightSum += holding["weight"].(float64)
		}

		// Weights should be back close to 100%
		assert.InDelta(t, 100.0, restoredWeightSum, 1.0, "weights should be restored to ~100%%")
		assert.Equal(t, 0.0, portfolio["external_balance_total"])
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- All Valid Types ---

func TestExternalBalanceAllTypes(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForExternalBalances(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/external-balances"

	types := []struct {
		balanceType string
		label       string
		value       float64
		rate        float64
	}{
		{"cash", "Cash Account", 10000, 0},
		{"accumulate", "Accumulate Account", 20000, 0.05},
		{"term_deposit", "Term Deposit", 30000, 0.045},
		{"offset", "Offset Account", 40000, 0},
	}

	for _, tt := range types {
		t.Run(tt.balanceType, func(t *testing.T) {
			body := map[string]interface{}{
				"type":  tt.balanceType,
				"label": tt.label,
				"value": tt.value,
			}
			if tt.rate > 0 {
				body["rate"] = tt.rate
			}

			resp, err := env.HTTPRequest(http.MethodPost, basePath, body, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			guard.SaveResult("type_"+tt.balanceType, string(respBody))

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(respBody, &result))
			assert.Equal(t, tt.balanceType, result["type"])
			assert.Equal(t, tt.label, result["label"])
			assert.Equal(t, tt.value, result["value"])
		})
	}

	// Verify all four are present
	t.Run("verify_all_present", func(t *testing.T) {
		balances, total, status := getExternalBalances(t, env, portfolioName, userHeaders)
		assert.Equal(t, http.StatusOK, status)
		assert.Len(t, balances, 4)
		assert.Equal(t, 100000.0, total)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
