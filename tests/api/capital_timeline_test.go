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

// --- Helpers ---

// cleanupCashFlows deletes all cash transactions for a portfolio.
func cleanupCashFlows(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, headers)
	if err != nil {
		return
	}
	var ledger map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if json.Unmarshal(body, &ledger) != nil {
		return
	}
	txns, ok := ledger["transactions"].([]interface{})
	if !ok {
		return
	}
	for _, tx := range txns {
		txMap, ok := tx.(map[string]interface{})
		if !ok {
			continue
		}
		txID, ok := txMap["id"].(string)
		if !ok {
			continue
		}
		r, e := env.HTTPRequest(http.MethodDelete, "/api/portfolios/"+portfolioName+"/cash-transactions/"+txID, nil, headers)
		if e == nil {
			r.Body.Close()
		}
	}
}

// --- Capital Allocation Timeline in time_series ---

// TestCapitalTimeline_FieldsPresentAfterCashTransactions verifies that
// after adding cash transactions, the indicators time_series includes
// cash_balance, external_balance, total_capital, and net_deployed fields.
func TestCapitalTimeline_FieldsPresentAfterCashTransactions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Get indicators before adding cash transactions
	t.Run("no_cash_timeline_fields_absent", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_no_cash", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		// time_series should be present (may be empty for a fresh portfolio)
		if ts, ok := indicators["time_series"]; ok && ts != nil {
			tsSlice, ok := ts.([]interface{})
			if ok && len(tsSlice) > 0 {
				// If time_series has points, they should not have capital flow fields
				// (omitempty means they are absent when zero)
				firstPoint := tsSlice[0].(map[string]interface{})
				_, hasCashBalance := firstPoint["cash_balance"]
				_, hasTotalCapital := firstPoint["total_capital"]
				_, hasNetDeployed := firstPoint["net_deployed"]
				assert.False(t, hasCashBalance, "cash_balance should be absent without cash transactions")
				assert.False(t, hasTotalCapital, "total_capital should be absent without cash transactions")
				assert.False(t, hasNetDeployed, "net_deployed should be absent without cash transactions")
			}
		}
	})

	// Step 2: Add cash transactions (deposit + contribution + withdrawal)
	t.Run("add_cash_transactions", func(t *testing.T) {
		transactions := []map[string]interface{}{
			{
				"type":        "deposit",
				"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      100000,
				"description": "Initial deposit for timeline test",
			},
			{
				"type":        "contribution",
				"date":        time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      25000,
				"description": "Q2 contribution for timeline test",
			},
			{
				"type":        "withdrawal",
				"date":        time.Now().Add(-90 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      10000,
				"description": "Partial withdrawal for timeline test",
			},
		}

		for _, tx := range transactions {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", tx, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
		}
	})

	// Step 3: Get indicators after adding cash transactions
	t.Run("capital_timeline_fields_present", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_indicators_with_cash", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		// time_series must be present and non-empty for a synced portfolio
		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		require.NotEmpty(t, tsSlice, "time_series should have at least one point")

		// Check at least the last point for capital flow fields
		// Later points will have the highest cumulative values
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})

		// net_deployed should be present and equal to deposits+contributions - withdrawals
		// = 100000 + 25000 - 10000 = 115000
		netDeployed, hasNetDeployed := lastPoint["net_deployed"]
		assert.True(t, hasNetDeployed, "net_deployed should be present in last time_series point after adding cash transactions")
		if hasNetDeployed {
			assert.InDelta(t, 115000.0, netDeployed.(float64), 1.0,
				"net_deployed should equal deposits+contributions-withdrawals")
		}
	})

	// Step 4: Verify total_capital invariant: total_capital = value + cash_balance + external_balance
	t.Run("total_capital_invariant", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_total_capital_check", string(body))

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — skipping total_capital invariant check")
		}

		tsSlice := ts.([]interface{})
		if len(tsSlice) == 0 {
			t.Skip("time_series empty — skipping total_capital invariant check")
		}

		// Check invariant for each point that has the new fields
		for i, pt := range tsSlice {
			point := pt.(map[string]interface{})

			value, hasValue := point["value"].(float64)
			if !hasValue {
				continue
			}

			cashBalance, hasCashBalance := point["cash_balance"].(float64)
			if !hasCashBalance {
				cashBalance = 0.0
			}

			externalBalance, hasExternalBalance := point["external_balance"].(float64)
			if !hasExternalBalance {
				externalBalance = 0.0
			}

			totalCapital, hasTotalCapital := point["total_capital"].(float64)
			if !hasTotalCapital {
				continue // if total_capital is absent, skip this point
			}

			expected := value + cashBalance + externalBalance
			assert.InDelta(t, expected, totalCapital, 0.01,
				"point[%d]: total_capital should equal value + cash_balance + external_balance", i)
		}
	})

	// Step 5: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Capital Timeline with External Balances ---

// TestCapitalTimeline_ExternalBalanceInTotal verifies that external_balance
// is reflected in total_capital on time_series points.
func TestCapitalTimeline_ExternalBalanceInTotal(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Add a cash transaction so capital flow fields appear
	t.Run("add_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      80000,
			"description": "Deposit for external balance timeline test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 2: Add external balance (e.g. accumulate)
	extBalanceAmount := 50000.0
	t.Run("add_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
			"type":  "accumulate",
			"label": "Timeline Test Accumulate",
			"value": extBalanceAmount,
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 3: Get indicators and verify external_balance appears in time_series
	t.Run("external_balance_in_time_series", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_with_ext_balance", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice := ts.([]interface{})
		if len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		// Check a point that should reflect external balance
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})

		externalBalance, hasExternalBalance := lastPoint["external_balance"]
		if hasExternalBalance {
			assert.InDelta(t, extBalanceAmount, externalBalance.(float64), 0.01,
				"external_balance in time_series should match the added external balance")
		}

		// total_capital invariant: total_capital = value + cash_balance + external_balance
		value, hasValue := lastPoint["value"].(float64)
		totalCapital, hasTotalCapital := lastPoint["total_capital"].(float64)

		if hasValue && hasTotalCapital {
			cashBalance, _ := lastPoint["cash_balance"].(float64)
			extBal, _ := lastPoint["external_balance"].(float64)
			expected := value + cashBalance + extBal
			assert.InDelta(t, expected, totalCapital, 0.01,
				"total_capital = value + cash_balance + external_balance")
		}
	})

	// Step 4: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
		env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Capital Timeline with Empty Ledger ---

// TestCapitalTimeline_EmptyLedger verifies that when no cash transactions exist,
// time_series points do not have capital flow fields (omitempty).
func TestCapitalTimeline_EmptyLedger(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Ensure no cash transactions exist (cleanup from previous tests in case of shared state)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_indicators_empty_ledger", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var indicators map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &indicators))

	ts, hasTS := indicators["time_series"]
	if !hasTS || ts == nil {
		// Acceptable — no historical data
		t.Logf("time_series absent for portfolio with no historical data")
		t.Logf("Results saved to: %s", guard.ResultsDir())
		return
	}

	tsSlice, ok := ts.([]interface{})
	if !ok || len(tsSlice) == 0 {
		t.Logf("time_series empty — no data points to check")
		t.Logf("Results saved to: %s", guard.ResultsDir())
		return
	}

	// All time_series points should NOT have cash_balance, total_capital, or net_deployed
	// when there are no cash transactions (omitempty ensures these fields are absent)
	for i, pt := range tsSlice {
		point := pt.(map[string]interface{})
		_, hasCashBalance := point["cash_balance"]
		_, hasTotalCapital := point["total_capital"]
		_, hasNetDeployed := point["net_deployed"]
		assert.False(t, hasCashBalance, "point[%d]: cash_balance should be absent with empty ledger", i)
		assert.False(t, hasTotalCapital, "point[%d]: total_capital should be absent with empty ledger", i)
		assert.False(t, hasNetDeployed, "point[%d]: net_deployed should be absent with empty ledger", i)
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Net Deployed Accumulates Correctly ---

// TestCapitalTimeline_NetDeployedAccumulation verifies that net_deployed
// accumulates correctly as deposits, contributions, and withdrawals are added.
func TestCapitalTimeline_NetDeployedAccumulation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add a sequence of transactions at known dates (all in the past)
	// deposit(50000) + contribution(30000) - withdrawal(20000) = 60000 net
	// dividend does NOT count toward net_deployed (it's investment return)
	transactions := []map[string]interface{}{
		{
			"type":        "deposit",
			"date":        time.Now().Add(-300 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      50000,
			"description": "Deposit 1 for net_deployed test",
		},
		{
			"type":        "contribution",
			"date":        time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      30000,
			"description": "Contribution for net_deployed test",
		},
		{
			"type":        "withdrawal",
			"date":        time.Now().Add(-100 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      20000,
			"description": "Withdrawal for net_deployed test",
		},
		{
			"type":        "dividend",
			"date":        time.Now().Add(-50 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      5000,
			"description": "Dividend (should not count in net_deployed)",
		},
	}

	t.Run("add_transactions", func(t *testing.T) {
		for _, tx := range transactions {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", tx, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
		}
	})

	t.Run("verify_net_deployed", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_net_deployed", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — insufficient historical data")
		}

		tsSlice := ts.([]interface{})
		if len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		// The last point should have net_deployed = 50000 + 30000 - 20000 = 60000
		// (dividend is NOT included in net_deployed — it's investment return)
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		netDeployed, hasNetDeployed := lastPoint["net_deployed"]
		if hasNetDeployed {
			assert.InDelta(t, 60000.0, netDeployed.(float64), 1.0,
				"net_deployed = deposits + contributions - withdrawals (dividend excluded)")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Same-Day Multiple Transactions ---

// TestCapitalTimeline_SameDayTransactions verifies that multiple transactions
// on the same date are all included in the cumulative totals.
func TestCapitalTimeline_SameDayTransactions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add 3 transactions on the same date
	sameDate := time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339)
	transactions := []map[string]interface{}{
		{"type": "deposit", "date": sameDate, "amount": 40000, "description": "Same-day deposit A"},
		{"type": "deposit", "date": sameDate, "amount": 60000, "description": "Same-day deposit B"},
		{"type": "withdrawal", "date": sameDate, "amount": 10000, "description": "Same-day withdrawal"},
	}

	t.Run("add_same_day_transactions", func(t *testing.T) {
		for _, tx := range transactions {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", tx, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
		}
	})

	t.Run("verify_same_day_aggregated", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_same_day_indicators", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — insufficient historical data")
		}

		tsSlice := ts.([]interface{})
		if len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		// Last point should have net_deployed = 40000 + 60000 - 10000 = 90000
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		netDeployed, hasNetDeployed := lastPoint["net_deployed"]
		if hasNetDeployed {
			assert.InDelta(t, 90000.0, netDeployed.(float64), 1.0,
				"all same-day transactions should be aggregated in net_deployed")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
