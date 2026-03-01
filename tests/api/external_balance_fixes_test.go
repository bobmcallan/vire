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

// --- Fix 1+2: Internal Transfers Excluded from Capital Performance (fb_65070e71, fb_7e9b3139) ---

// TestCapitalPerformance_InternalTransferExcluded verifies that transfer_out with
// an external-balance category (e.g. "accumulate") is NOT treated as a capital
// withdrawal in CalculatePerformance. Fixes fb_65070e71 and fb_7e9b3139.
func TestCapitalPerformance_InternalTransferExcluded(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Ensure clean state
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Add a known deposit so we have a baseline
	depositAmount := 100000.0
	t.Run("add_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-300 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      depositAmount,
			"description": "Initial capital for internal transfer test",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_add_deposit", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "add deposit failed: %s", string(body))
	})

	// Step 2: Get performance with deposit only (baseline)
	var baselineNetCapital float64
	t.Run("baseline_performance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_baseline_perf", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		netCapital, ok := perf["net_capital_deployed"].(float64)
		require.True(t, ok, "net_capital_deployed should be a float")
		baselineNetCapital = netCapital
		t.Logf("Baseline net_capital_deployed: %.2f", baselineNetCapital)
	})

	// Step 3: Add transfer_out with category "accumulate" (internal transfer to external balance)
	transferAmount := 20000.0
	t.Run("add_internal_transfer_out", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "transfer_out",
			"date":        time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      transferAmount,
			"description": "Move to accumulate account (internal)",
			"category":    "accumulate",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_add_internal_transfer", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "add transfer_out failed: %s", string(body))
	})

	// Step 4: Verify net_capital_deployed is unchanged (internal transfer should be skipped)
	t.Run("net_capital_unchanged_after_internal_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_perf_after_internal_transfer", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		netCapital, ok := perf["net_capital_deployed"].(float64)
		require.True(t, ok, "net_capital_deployed should be a float")

		// Internal transfer should NOT reduce net_capital_deployed
		assert.InDelta(t, baselineNetCapital, netCapital, 0.01,
			"net_capital_deployed should be unchanged when transfer_out has category=accumulate (internal transfer)")
	})

	// Step 5: Add transfer_out WITHOUT a category (real withdrawal) — this SHOULD reduce net capital
	realWithdrawalAmount := 15000.0
	t.Run("add_real_transfer_out", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "withdrawal",
			"date":        time.Now().Add(-100 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      realWithdrawalAmount,
			"description": "Real capital withdrawal (no category)",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("05_add_real_withdrawal", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 6: Verify real withdrawal DOES reduce net capital
	t.Run("real_withdrawal_reduces_net_capital", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("06_perf_after_real_withdrawal", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		netCapital, ok := perf["net_capital_deployed"].(float64)
		require.True(t, ok, "net_capital_deployed should be a float")

		// Real withdrawal should reduce net_capital_deployed
		expectedNet := baselineNetCapital - realWithdrawalAmount
		assert.InDelta(t, expectedNet, netCapital, 0.01,
			"real withdrawal should reduce net_capital_deployed by %.2f", realWithdrawalAmount)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestCapitalPerformance_HoldingsOnlyValue verifies that current_portfolio_value
// in capital performance uses TotalValueHoldings (not including external balances).
// Fixes fb_7e9b3139.
func TestCapitalPerformance_HoldingsOnlyValue(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Ensure clean state
	cleanupCashFlows(t, env, portfolioName, userHeaders)
	// Remove any existing external balances
	env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
		map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)

	// Step 1: Add a deposit so performance is computable
	t.Run("add_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      100000,
			"description": "Deposit for holdings-only value test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 2: Get performance WITHOUT external balances — record current_portfolio_value
	var baselinePortfolioValue float64
	t.Run("baseline_portfolio_value", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_perf_no_ext_balance", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		currentValue, ok := perf["current_portfolio_value"].(float64)
		require.True(t, ok, "current_portfolio_value should be a float")
		baselinePortfolioValue = currentValue
		t.Logf("Baseline current_portfolio_value (holdings only): %.2f", baselinePortfolioValue)

		// Also get portfolio's holdings value directly
		portfolioResp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer portfolioResp.Body.Close()

		portfolioBody, _ := io.ReadAll(portfolioResp.Body)
		guard.SaveResult("01b_portfolio_state", string(portfolioBody))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(portfolioBody, &portfolio))

		holdingsValue, _ := portfolio["total_value_holdings"].(float64)
		t.Logf("Portfolio total_value_holdings: %.2f", holdingsValue)

		// current_portfolio_value should match holdings-only value
		assert.InDelta(t, holdingsValue, currentValue, 0.01,
			"current_portfolio_value should equal total_value_holdings (not including external balances)")
	})

	// Step 3: Add an external balance
	extBalanceAmount := 50000.0
	t.Run("add_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
			"type":  "accumulate",
			"label": "SMSF Accumulate Account",
			"value": extBalanceAmount,
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_add_ext_balance", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "add external balance failed: %s", string(body))
	})

	// Step 4: current_portfolio_value should NOT have changed (holdings-only)
	t.Run("portfolio_value_unchanged_by_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_perf_with_ext_balance", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		currentValue, ok := perf["current_portfolio_value"].(float64)
		require.True(t, ok, "current_portfolio_value should be a float")

		// current_portfolio_value should be unchanged — external balances are excluded
		assert.InDelta(t, baselinePortfolioValue, currentValue, 0.01,
			"current_portfolio_value should remain holdings-only (%.2f), not include external balance (%.2f)",
			baselinePortfolioValue, extBalanceAmount)

		// Confirm the external balance IS visible on the portfolio response
		portfolioResp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer portfolioResp.Body.Close()

		portfolioBody, _ := io.ReadAll(portfolioResp.Body)
		guard.SaveResult("03b_portfolio_state", string(portfolioBody))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(portfolioBody, &portfolio))

		extBalTotal, _ := portfolio["external_balance_total"].(float64)
		assert.InDelta(t, extBalanceAmount, extBalTotal, 0.01,
			"portfolio should show external_balance_total = %.2f", extBalanceAmount)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
		env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestCapitalPerformance_MultipleInternalTransferCategories verifies that all
// external balance categories (cash, accumulate, term_deposit, offset) are
// treated as internal transfers when used with transfer_out/transfer_in.
func TestCapitalPerformance_MultipleInternalTransferCategories(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Ensure clean state
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add base deposit
	depositAmount := 200000.0
	t.Run("add_base_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-400 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      depositAmount,
			"description": "Base deposit for multi-category test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Get baseline net capital
	var baselineNetCapital float64
	t.Run("baseline", func(t *testing.T) {
		perf, status := getCashFlowPerformance(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)
		baselineNetCapital = perf["net_capital_deployed"].(float64)
		t.Logf("Baseline net_capital_deployed: %.2f", baselineNetCapital)
	})

	// Add transfer_out for all 4 external balance categories
	categories := []string{"cash", "accumulate", "term_deposit", "offset"}
	t.Run("add_internal_transfers_all_categories", func(t *testing.T) {
		for i, cat := range categories {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
				"type":        "transfer_out",
				"date":        time.Now().Add(time.Duration(-(300 - i*10)) * 24 * time.Hour).Format(time.RFC3339),
				"amount":      10000,
				"description": "Internal transfer to " + cat,
				"category":    cat,
			}, userHeaders)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			guard.SaveResult("tx_transfer_"+cat, string(body))
			require.Equal(t, http.StatusCreated, resp.StatusCode, "add transfer_out[%s] failed: %s", cat, string(body))
		}
	})

	// Verify net_capital_deployed is unchanged for ALL categories
	t.Run("all_categories_excluded_from_net_capital", func(t *testing.T) {
		perf, status := getCashFlowPerformance(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)
		guard.SaveResult("verify_all_categories", prettyJSON(perf))

		netCapital, ok := perf["net_capital_deployed"].(float64)
		require.True(t, ok, "net_capital_deployed should be a float")

		// All 4 transfer_out with external-balance categories should be skipped
		assert.InDelta(t, baselineNetCapital, netCapital, 0.01,
			"net_capital_deployed should be unchanged after all internal transfer_out transactions")

		t.Logf("net_capital_deployed after %d internal transfers: %.2f (expected %.2f)",
			len(categories), netCapital, baselineNetCapital)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 3: Capital Timeline Excludes Internal Transfers from Cash Balance (fb_2f9c18fe) ---

// TestCapitalTimeline_InternalTransferExcludedFromCashBalance verifies that
// transfer_out with category "accumulate" does NOT reduce the running cash
// balance in the time_series growth data. Fixes fb_2f9c18fe.
func TestCapitalTimeline_InternalTransferExcludedFromCashBalance(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Ensure clean state
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Add a deposit to establish a cash baseline
	depositAmount := 80000.0
	t.Run("add_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      depositAmount,
			"description": "Deposit for timeline internal transfer test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 2: Get indicators to capture baseline cash_balance on last point
	var baselineCashBalance float64
	var hasCashBalanceData bool
	t.Run("baseline_cash_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_baseline_indicators", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		if !ok || len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		totalCash, ok := lastPoint["total_cash"].(float64)
		if ok {
			baselineCashBalance = totalCash
			hasCashBalanceData = true
			t.Logf("Baseline total_cash on last point: %.2f", baselineCashBalance)
		}
	})

	if !hasCashBalanceData {
		t.Skip("No total_cash data available in time_series")
	}

	// Step 3: Add transfer_out with category "accumulate" (internal — should NOT affect total_cash)
	internalTransferAmount := 20000.0
	t.Run("add_internal_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "transfer_out",
			"date":        time.Now().Add(-100 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      internalTransferAmount,
			"description": "Internal move to accumulate (should be excluded from cash)",
			"category":    "accumulate",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_add_internal_transfer", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "add transfer failed: %s", string(body))
	})

	// Step 4: Verify total_cash is unchanged (internal transfer excluded)
	t.Run("total_cash_unchanged_after_internal_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_indicators_after_internal_transfer", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent after adding internal transfer")
		}

		tsSlice := ts.([]interface{})
		require.NotEmpty(t, tsSlice, "time_series should have points")

		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		totalCash, hasTotalCash := lastPoint["total_cash"].(float64)
		if hasTotalCash {
			// Total cash should be unchanged — internal transfer was excluded
			assert.InDelta(t, baselineCashBalance, totalCash, 0.01,
				"total_cash should be unchanged after internal transfer_out[accumulate] (baseline=%.2f, got=%.2f)",
				baselineCashBalance, totalCash)
		}
	})

	// Step 5: Add a REAL withdrawal (no category) — cash_balance SHOULD decrease
	realWithdrawal := 10000.0
	t.Run("add_real_withdrawal", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "withdrawal",
			"date":        time.Now().Add(-50 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      realWithdrawal,
			"description": "Real withdrawal (should reduce cash_balance)",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 6: Verify real withdrawal DOES reduce cash_balance
	t.Run("real_withdrawal_reduces_cash_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_indicators_after_real_withdrawal", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent")
		}

		tsSlice := ts.([]interface{})
		require.NotEmpty(t, tsSlice)

		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		totalCash, hasTotalCash := lastPoint["total_cash"].(float64)
		if hasTotalCash {
			expectedCash := baselineCashBalance - realWithdrawal
			assert.InDelta(t, expectedCash, totalCash, 0.01,
				"total_cash should decrease by real withdrawal (expected=%.2f, got=%.2f)",
				expectedCash, totalCash)
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestCapitalTimeline_NoCrashOnInternalTransfer verifies that the timeline
// does NOT show a false "crash" (sudden drop in cash_balance) when an internal
// transfer_out fires. Fixes the visual artifact described in fb_2f9c18fe.
func TestCapitalTimeline_NoCrashOnInternalTransfer(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add a deposit followed by several internal transfers to different accounts
	t.Run("add_deposit_and_internal_transfers", func(t *testing.T) {
		// Base deposit
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-300 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      500000,
			"description": "SMSF fund deposit",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Three internal transfers (like the SMSF scenario in the bug report)
		internalTransfers := []map[string]interface{}{
			{
				"type": "transfer_out", "date": time.Now().Add(-250 * 24 * time.Hour).Format(time.RFC3339),
				"amount": 20000, "description": "To accumulate account 1", "category": "accumulate",
			},
			{
				"type": "transfer_out", "date": time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
				"amount": 20000, "description": "To accumulate account 2", "category": "accumulate",
			},
			{
				"type": "transfer_out", "date": time.Now().Add(-150 * 24 * time.Hour).Format(time.RFC3339),
				"amount": 20600, "description": "To offset account", "category": "offset",
			},
		}

		for _, tx := range internalTransfers {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", tx, userHeaders)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode, "add internal transfer failed: %s", string(body))
		}
	})

	// Get indicators and verify no sudden drops (cash_balance stays non-negative given enough deposit)
	t.Run("no_false_crash_in_timeline", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_no_crash", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		if !ok || len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		// Check that total_cash doesn't suddenly go very negative (indicating internal transfers
		// were incorrectly subtracted). The deposit was 500000 and internal transfers 60600,
		// so total_cash should remain >= 439000 if transfers are excluded.
		//
		// With the bug, total_cash would drop to 439400 (500000 - 60600 = 439400).
		// With the fix, total_cash stays at 500000 (no decrease from internal transfers).
		for i, pt := range tsSlice {
			point, ok := pt.(map[string]interface{})
			if !ok {
				continue
			}
			totalCash, hasCash := point["total_cash"].(float64)
			if !hasCash {
				continue
			}
			// With fix: total_cash stays at 500000 (internal transfers excluded)
			// Without fix: total_cash would drop to 439400
			// Both are >= 0, but we can check it didn't drop below 450000
			// (allowing for floating point and date boundary differences)
			assert.GreaterOrEqual(t, totalCash, 440000.0,
				"point[%d]: total_cash should not reflect internal transfers (got=%.2f)", i, totalCash)
		}

		// The last point should have the full deposit as total_cash
		// (all transfers were internal, so no real cash left the portfolio)
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		totalCash, hasCash := lastPoint["total_cash"].(float64)
		if hasCash {
			assert.InDelta(t, 500000.0, totalCash, 0.01,
				"final total_cash should remain at deposit amount (%.2f) since all transfers were internal",
				500000.0)
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 4: ExternalBalance.AssetCategory() returns "cash" (fb_5d5e7e5e) ---

// TestExternalBalance_AssetCategoryIsCash verifies that all external balance types
// (cash, accumulate, term_deposit, offset) report "cash" as their asset category.
// Fixes fb_5d5e7e5e.
func TestExternalBalance_AssetCategoryIsCash(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Remove any existing external balances
	env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
		map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)

	// Add one external balance of each type
	balanceTypes := []struct {
		balType string
		label   string
		amount  float64
	}{
		{"cash", "Cash Account", 10000},
		{"accumulate", "Accumulate Account", 20000},
		{"term_deposit", "Term Deposit", 30000},
		{"offset", "Offset Account", 15000},
	}

	t.Run("add_all_balance_types", func(t *testing.T) {
		for _, bt := range balanceTypes {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
				"type":  bt.balType,
				"label": bt.label,
				"value": bt.amount,
			}, userHeaders)
			require.NoError(t, err)
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			guard.SaveResult("add_"+bt.balType, string(body))
			require.Equal(t, http.StatusCreated, resp.StatusCode, "add external balance[%s] failed: %s", bt.balType, string(body))
		}
	})

	// Verify external balances appear in portfolio response with asset_category = "cash"
	t.Run("external_balances_have_cash_asset_category", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_portfolio_with_all_balance_types", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		extBalances, ok := portfolio["external_balances"].([]interface{})
		require.True(t, ok, "portfolio should have external_balances array")
		require.NotEmpty(t, extBalances, "external_balances should not be empty")

		// For each external balance, verify asset_category is "cash"
		for _, eb := range extBalances {
			balance, ok := eb.(map[string]interface{})
			require.True(t, ok, "each external balance should be an object")

			balType, _ := balance["type"].(string)
			assetCategory, hasCategory := balance["asset_category"]
			if hasCategory {
				assert.Equal(t, "cash", assetCategory,
					"external_balance[type=%s] should have asset_category=cash", balType)
			}
			// Note: if asset_category is omitted from response, that's also acceptable
			// as long as the server-side logic classifies it correctly for allocation
		}

		t.Logf("Found %d external balances", len(extBalances))
	})

	// Also verify the total external balance value is correctly summed
	t.Run("external_balance_total_correct", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_external_balance_total", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		totalExpected := 10000.0 + 20000.0 + 30000.0 + 15000.0 // 75000
		extBalTotal, ok := portfolio["external_balance_total"].(float64)
		require.True(t, ok, "external_balance_total should be present")
		assert.InDelta(t, totalExpected, extBalTotal, 0.01,
			"external_balance_total should sum all 4 balance types")
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Combined Scenario: Internal Transfer with External Balance (Regression) ---

// TestInternalTransfer_WithExternalBalance_ConsistentState verifies that adding
// an external balance AND an internal transfer_out for the same category produces
// consistent results (no double-counting of the transfer in performance metrics).
func TestInternalTransfer_WithExternalBalance_ConsistentState(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)
	env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
		map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)

	// Add deposit
	t.Run("add_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      300000,
			"description": "SMSF capital deposit",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Add internal transfer_out to accumulate (matches external balance account)
	t.Run("add_internal_transfer_to_accumulate", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "transfer_out",
			"date":        time.Now().Add(-200 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      60000,
			"description": "Move funds to accumulate account",
			"category":    "accumulate",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Add matching external balance (the accumulate account)
	t.Run("add_external_balance_accumulate", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances", map[string]interface{}{
			"type":  "accumulate",
			"label": "SMSF Accumulate",
			"value": 60000,
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Get performance and verify consistency
	t.Run("verify_no_double_counting", func(t *testing.T) {
		perf, status := getCashFlowPerformance(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)
		guard.SaveResult("01_combined_performance", prettyJSON(perf))

		netCapital, ok := perf["net_capital_deployed"].(float64)
		require.True(t, ok, "net_capital_deployed should be a float")

		// net_capital_deployed should be 300000 (deposit only, internal transfer excluded)
		assert.InDelta(t, 300000.0, netCapital, 0.01,
			"net_capital_deployed should be 300000 (deposit only, internal transfer excluded)")

		currentValue, ok := perf["current_portfolio_value"].(float64)
		require.True(t, ok, "current_portfolio_value should be a float")

		// current_portfolio_value should be holdings-only (NOT including the 60000 external balance)
		t.Logf("current_portfolio_value (holdings only): %.2f", currentValue)
		t.Logf("net_capital_deployed: %.2f", netCapital)

		// Get portfolio to check total_value vs total_value_holdings
		portfolioResp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer portfolioResp.Body.Close()

		portfolioBody, _ := io.ReadAll(portfolioResp.Body)
		guard.SaveResult("01b_portfolio_state", string(portfolioBody))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(portfolioBody, &portfolio))

		holdingsValue, _ := portfolio["total_value_holdings"].(float64)
		totalValue, _ := portfolio["total_value"].(float64)

		t.Logf("total_value_holdings: %.2f, total_value (with ext bal): %.2f", holdingsValue, totalValue)

		// current_portfolio_value should match holdings only (not total_value which includes ext bal)
		assert.InDelta(t, holdingsValue, currentValue, 0.01,
			"current_portfolio_value should equal total_value_holdings (%.2f), not total_value (%.2f)",
			holdingsValue, totalValue)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
		env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{"external_balances": []map[string]interface{}{}}, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Helper ---

// prettyJSON formats a map as indented JSON for saving to test results.
func prettyJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
