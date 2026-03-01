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

// postContribution posts a single cash transaction using the current API format
// (account + category + signed amount) and returns the HTTP status code.
func postContribution(t *testing.T, env *common.Env, portfolioName string, headers map[string]string,
	account, category, date, description string, amount float64) int {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost,
		"/api/portfolios/"+portfolioName+"/cash-transactions",
		map[string]interface{}{
			"account":     account,
			"category":    category,
			"date":        date,
			"amount":      amount,
			"description": description,
		}, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

// fetchPortfolio fetches the portfolio response and returns the decoded map.
func fetchPortfolio(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) map[string]interface{} {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET portfolio failed: %s", string(body))
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

// fetchIndicators fetches the portfolio indicators response and returns the decoded map.
func fetchIndicators(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) map[string]interface{} {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/indicators", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET indicators failed: %s", string(body))
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

// --- Fix 1: total_cash field on portfolio equals sum of all account balances ---

// TestCapitalCashFixes_TotalCashField verifies that the portfolio response
// includes total_cash equal to the sum of all cash account balances.
// Fixes fb_d895f8f9: ExternalBalanceTotal previously used NonTransactionalBalance()
// (excluding the Trading account), but should use TotalCashBalance() (all accounts).
func TestCapitalCashFixes_TotalCashField(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Portfolio without cash transactions — total_cash should be 0 or absent
	t.Run("no_transactions_total_cash_zero", func(t *testing.T) {
		portfolio := fetchPortfolio(t, env, portfolioName, userHeaders)
		guard.SaveResult("01_portfolio_no_cash", prettyJSON(portfolio))

		totalCash, hasTotalCash := portfolio["total_cash"]
		if hasTotalCash {
			assert.Equal(t, 0.0, totalCash.(float64),
				"total_cash should be 0 when no cash transactions exist")
		}
		// total_cash may be absent (omitempty) when 0 — both are valid
	})

	// Step 2: Add a deposit to the Trading account
	depositAmount := 50000.0
	t.Run("add_trading_deposit", func(t *testing.T) {
		date := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
		status := postContribution(t, env, portfolioName, userHeaders,
			"Trading", "contribution", date, "Deposit to Trading account", depositAmount)
		require.Equal(t, http.StatusCreated, status, "POST cash-transaction failed")
	})

	// Step 3: total_cash should now equal the deposit amount
	t.Run("total_cash_equals_deposit", func(t *testing.T) {
		portfolio := fetchPortfolio(t, env, portfolioName, userHeaders)
		guard.SaveResult("02_portfolio_after_deposit", prettyJSON(portfolio))

		totalCash, hasTotalCash := portfolio["total_cash"]
		require.True(t, hasTotalCash, "total_cash should be present after adding cash transactions")
		assert.InDelta(t, depositAmount, totalCash.(float64), 0.01,
			"total_cash should equal the deposit amount (%.2f)", depositAmount)
	})

	// Step 4: Add another deposit to verify accumulation
	secondDeposit := 25000.0
	t.Run("add_second_deposit", func(t *testing.T) {
		date := time.Now().Add(-15 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
		status := postContribution(t, env, portfolioName, userHeaders,
			"Trading", "contribution", date, "Second deposit", secondDeposit)
		require.Equal(t, http.StatusCreated, status)
	})

	t.Run("total_cash_accumulates", func(t *testing.T) {
		portfolio := fetchPortfolio(t, env, portfolioName, userHeaders)
		guard.SaveResult("03_portfolio_two_deposits", prettyJSON(portfolio))

		totalCash, hasTotalCash := portfolio["total_cash"]
		require.True(t, hasTotalCash, "total_cash should be present")
		assert.InDelta(t, depositAmount+secondDeposit, totalCash.(float64), 0.01,
			"total_cash should equal sum of all deposits (%.2f)", depositAmount+secondDeposit)
	})

	// Step 5: Add a withdrawal (negative contribution) — total_cash should decrease
	withdrawalAmount := 10000.0
	t.Run("add_withdrawal", func(t *testing.T) {
		date := time.Now().Add(-5 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
		// Withdrawals are negative contributions
		status := postContribution(t, env, portfolioName, userHeaders,
			"Trading", "contribution", date, "Withdrawal from Trading account", -withdrawalAmount)
		require.Equal(t, http.StatusCreated, status)
	})

	t.Run("total_cash_decreases_with_withdrawal", func(t *testing.T) {
		portfolio := fetchPortfolio(t, env, portfolioName, userHeaders)
		guard.SaveResult("04_portfolio_after_withdrawal", prettyJSON(portfolio))

		totalCash, hasTotalCash := portfolio["total_cash"]
		require.True(t, hasTotalCash, "total_cash should be present")
		expected := depositAmount + secondDeposit - withdrawalAmount
		assert.InDelta(t, expected, totalCash.(float64), 0.01,
			"total_cash should decrease by withdrawal amount (expected %.2f)", expected)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 2: time_series TotalCapital = Value + CashBalance (no ExternalBalance double-count) ---

// TestCapitalCashFixes_TotalCapitalNoDoubleCount verifies that in time_series,
// total_capital = value + cash_balance (ExternalBalance is NOT added separately).
// Fixes fb_60bddec8: ExternalBalance was counted in both CashBalance (via
// runningCashBalance) AND separately in TotalCapital, causing double-counting.
func TestCapitalCashFixes_TotalCapitalNoDoubleCount(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add a deposit far enough in the past to generate time_series history
	depositAmount := 100000.0
	t.Run("add_deposit_for_history", func(t *testing.T) {
		date := time.Now().Add(-400 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
		status := postContribution(t, env, portfolioName, userHeaders,
			"Trading", "contribution", date, "Initial deposit for total_capital test", depositAmount)
		require.Equal(t, http.StatusCreated, status)
	})

	t.Run("total_capital_equals_value_plus_cash_balance", func(t *testing.T) {
		indicators := fetchIndicators(t, env, portfolioName, userHeaders)
		guard.SaveResult("01_indicators_with_cash", prettyJSON(indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		if len(tsSlice) == 0 {
			t.Skip("time_series empty — insufficient historical data")
		}

		// Verify the invariant for ALL points that have total_capital
		// Post-fix: total_capital = value + cash_balance (ExternalBalance is deprecated/zero)
		violations := 0
		for i, pt := range tsSlice {
			point := pt.(map[string]interface{})

			value, hasValue := point["value"].(float64)
			if !hasValue {
				continue
			}

			totalCapital, hasTotalCapital := point["total_capital"].(float64)
			if !hasTotalCapital {
				continue
			}

			cashBalance, _ := point["cash_balance"].(float64)
			// external_balance should be 0 after the fix (not contributing to total_capital)
			externalBalance, _ := point["external_balance"].(float64)

			// The CORRECT formula post-fix: total_capital = value + cash_balance
			// ExternalBalance should NOT be separately added (it was double-counted before)
			expectedWithoutExternal := value + cashBalance
			expectedWithExternal := value + cashBalance + externalBalance

			// After fix: total_capital should equal value + cash_balance
			if !assert.InDelta(t, expectedWithoutExternal, totalCapital, 0.01,
				"point[%d]: total_capital should equal value + cash_balance (%.2f), not include extra external_balance (%.2f)",
				i, expectedWithoutExternal, externalBalance) {
				violations++
				t.Logf("point[%d]: value=%.2f, cash_balance=%.2f, external_balance=%.2f, total_capital=%.2f, expected=%.2f, expectedWithExt=%.2f",
					i, value, cashBalance, externalBalance, totalCapital, expectedWithoutExternal, expectedWithExternal)
			}
		}

		if violations > 0 {
			t.Errorf("Found %d time_series points where total_capital includes external_balance (double-count)", violations)
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 3: net_deployed steps up at each deposit date ---

// TestCapitalCashFixes_NetDeployedStepping verifies that net_deployed in
// time_series increases at each contribution date and stabilizes between dates.
func TestCapitalCashFixes_NetDeployedStepping(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add contributions at three distinct dates, well separated
	now := time.Now().UTC()
	date1 := now.Add(-365 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	date2 := now.Add(-180 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	date3 := now.Add(-90 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)

	deposit1 := 50000.0
	deposit2 := 30000.0
	deposit3 := 20000.0

	t.Run("add_staggered_contributions", func(t *testing.T) {
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date1, "First deposit", deposit1))
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date2, "Second deposit", deposit2))
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date3, "Third deposit", deposit3))
	})

	t.Run("net_deployed_steps_at_contribution_dates", func(t *testing.T) {
		indicators := fetchIndicators(t, env, portfolioName, userHeaders)
		guard.SaveResult("01_indicators_staggered_deposits", prettyJSON(indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		if len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		// Find the last point's net_deployed — should equal total of all deposits
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		netDeployed, hasNetDeployed := lastPoint["net_deployed"]
		require.True(t, hasNetDeployed, "net_deployed should be present in last time_series point")

		expectedTotal := deposit1 + deposit2 + deposit3 // 100000
		assert.InDelta(t, expectedTotal, netDeployed.(float64), 1.0,
			"net_deployed at last point should equal sum of all deposits (%.2f)", expectedTotal)

		// Verify net_deployed is monotonically non-decreasing (contributions only increase it)
		var prevNetDeployed float64
		for i, pt := range tsSlice {
			point := pt.(map[string]interface{})
			nd, hasND := point["net_deployed"].(float64)
			if !hasND {
				continue
			}
			assert.GreaterOrEqual(t, nd, prevNetDeployed,
				"point[%d]: net_deployed (%.2f) should be >= previous (%.2f) — contributions only increase it",
				i, nd, prevNetDeployed)
			prevNetDeployed = nd
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 4: net_deployed decreases with negative contribution (withdrawal) ---

// TestCapitalCashFixes_NetDeployedNegativeContribution verifies that a negative
// contribution (category=contribution, amount<0) decreases net_deployed.
// Fixes fb_7d8dafdb: NetDeployedImpact previously ignored negative contributions,
// causing net_deployed to never decrease even when capital was withdrawn.
func TestCapitalCashFixes_NetDeployedNegativeContribution(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	now := time.Now().UTC()
	// Deposit 300 days ago
	depositDate := now.Add(-300 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	// Withdraw (negative contribution) 100 days ago
	withdrawalDate := now.Add(-100 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)

	depositAmount := 100000.0
	withdrawalAmount := 30000.0 // will be sent as -30000

	t.Run("add_deposit_and_withdrawal", func(t *testing.T) {
		// Add positive contribution (deposit)
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders,
				"Trading", "contribution", depositDate, "Initial deposit", depositAmount))

		// Add negative contribution (withdrawal — category=contribution, amount<0)
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders,
				"Trading", "contribution", withdrawalDate, "Capital withdrawal", -withdrawalAmount))
	})

	t.Run("net_deployed_decreases_after_withdrawal", func(t *testing.T) {
		indicators := fetchIndicators(t, env, portfolioName, userHeaders)
		guard.SaveResult("01_indicators_with_withdrawal", prettyJSON(indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		if len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		// The last point should have net_deployed = 100000 - 30000 = 70000
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})
		netDeployed, hasNetDeployed := lastPoint["net_deployed"]
		require.True(t, hasNetDeployed, "net_deployed should be present in last time_series point")

		expectedFinal := depositAmount - withdrawalAmount // 70000
		assert.InDelta(t, expectedFinal, netDeployed.(float64), 1.0,
			"net_deployed should equal deposit - withdrawal (%.2f - %.2f = %.2f)",
			depositAmount, withdrawalAmount, expectedFinal)

		// Find any point after the withdrawal date and verify net_deployed dropped
		// (scan for a step-down in net_deployed values)
		prevNetDeployed := -1.0
		foundDecrease := false
		for _, pt := range tsSlice {
			point := pt.(map[string]interface{})
			nd, hasND := point["net_deployed"].(float64)
			if !hasND {
				continue
			}
			if prevNetDeployed > 0 && nd < prevNetDeployed {
				foundDecrease = true
				t.Logf("Found net_deployed decrease: %.2f -> %.2f", prevNetDeployed, nd)
			}
			prevNetDeployed = nd
		}
		assert.True(t, foundDecrease,
			"net_deployed should decrease at the withdrawal date (negative contribution should reduce net_deployed)")
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 5: Capital timeline cash_balance and net_deployed track correctly ---

// TestCapitalCashFixes_CapitalTimelineTracking verifies that cash_balance and
// net_deployed in the time_series correctly track contributions and withdrawals.
func TestCapitalCashFixes_CapitalTimelineTracking(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	now := time.Now().UTC()

	// Build a known sequence of transactions
	// Date1: +100000 (deposit)
	// Date2: +25000  (contribution)
	// Date3: -10000  (withdrawal = negative contribution)
	// Expected final net_deployed = 115000, final cash_balance = 115000
	date1 := now.Add(-400 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	date2 := now.Add(-200 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	date3 := now.Add(-100 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)

	t.Run("add_sequence_of_transactions", func(t *testing.T) {
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date1, "Initial deposit", 100000.0))
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date2, "Q2 contribution", 25000.0))
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date3, "Partial withdrawal", -10000.0))
	})

	t.Run("verify_final_cash_balance_and_net_deployed", func(t *testing.T) {
		indicators := fetchIndicators(t, env, portfolioName, userHeaders)
		guard.SaveResult("01_indicators_sequence", prettyJSON(indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		if len(tsSlice) == 0 {
			t.Skip("time_series empty")
		}

		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})

		// net_deployed: 100000 + 25000 - 10000 = 115000
		netDeployed, hasNetDeployed := lastPoint["net_deployed"]
		if hasNetDeployed {
			assert.InDelta(t, 115000.0, netDeployed.(float64), 1.0,
				"net_deployed should equal total contributions minus withdrawals")
		}

		// cash_balance: also 115000 (sum of all signed amounts)
		cashBalance, hasCashBalance := lastPoint["cash_balance"]
		if hasCashBalance {
			assert.InDelta(t, 115000.0, cashBalance.(float64), 1.0,
				"cash_balance should equal total cash in all accounts")
		}
	})

	t.Run("verify_portfolio_total_cash_matches_ledger", func(t *testing.T) {
		portfolio := fetchPortfolio(t, env, portfolioName, userHeaders)
		guard.SaveResult("02_portfolio_state", prettyJSON(portfolio))

		// total_cash on portfolio should equal the net of all transactions
		totalCash, hasTotalCash := portfolio["total_cash"]
		if hasTotalCash {
			assert.InDelta(t, 115000.0, totalCash.(float64), 0.01,
				"portfolio total_cash should equal sum of all cash account balances (115000)")
		}

		// Verify total_cash field is present and correct
		require.True(t, hasTotalCash, "total_cash should be present on portfolio after cash transactions")
	})

	t.Run("verify_total_capital_invariant_throughout_timeline", func(t *testing.T) {
		indicators := fetchIndicators(t, env, portfolioName, userHeaders)
		guard.SaveResult("03_indicators_invariant_check", prettyJSON(indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent")
		}

		tsSlice := ts.([]interface{})
		for i, pt := range tsSlice {
			point := pt.(map[string]interface{})

			value, hasValue := point["value"].(float64)
			totalCapital, hasTotalCapital := point["total_capital"].(float64)

			if !hasValue || !hasTotalCapital {
				continue
			}

			cashBalance, _ := point["cash_balance"].(float64)

			// Post-fix invariant: total_capital = value + cash_balance
			// (external_balance should be 0 or not contribute separately)
			expected := value + cashBalance
			assert.InDelta(t, expected, totalCapital, 0.01,
				"point[%d]: total_capital (%.2f) should equal value (%.2f) + cash_balance (%.2f)",
				i, totalCapital, value, cashBalance)
		}
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Regression: get_portfolio total_cash is computed from ALL accounts ---

// TestCapitalCashFixes_TotalCashAllAccounts verifies that total_cash on the
// portfolio sums transactions across ALL named accounts (not just non-transactional ones).
// Fixes fb_d895f8f9: old code used NonTransactionalBalance() which excluded the
// Trading account from the calculation.
func TestCapitalCashFixes_TotalCashAllAccounts(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	date := time.Now().Add(-60 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	// Add a deposit to the Trading account (transactional)
	tradingDeposit := 80000.0
	t.Run("add_trading_account_deposit", func(t *testing.T) {
		require.Equal(t, http.StatusCreated,
			postContribution(t, env, portfolioName, userHeaders, "Trading", "contribution", date, "Trading deposit", tradingDeposit))
	})

	// Add a credit to a non-transactional account (Stake Accumulate)
	accumulateAmount := 20000.0
	t.Run("add_accumulate_account_credit", func(t *testing.T) {
		// First, add the Stake Accumulate account by posting a transaction to it
		// (account is auto-created if new)
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions",
			map[string]interface{}{
				"account":     "Stake Accumulate",
				"category":    "contribution",
				"date":        date,
				"amount":      accumulateAmount,
				"description": "Accumulate account credit",
			}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_add_accumulate_credit", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// total_cash should sum BOTH accounts: 80000 + 20000 = 100000
	t.Run("total_cash_includes_all_accounts", func(t *testing.T) {
		portfolio := fetchPortfolio(t, env, portfolioName, userHeaders)
		guard.SaveResult("02_portfolio_all_accounts", prettyJSON(portfolio))

		totalCash, hasTotalCash := portfolio["total_cash"]
		require.True(t, hasTotalCash, "total_cash should be present")

		expectedTotal := tradingDeposit + accumulateAmount // 100000
		assert.InDelta(t, expectedTotal, totalCash.(float64), 0.01,
			"total_cash should include BOTH Trading (%.2f) AND Accumulate (%.2f) accounts = %.2f",
			tradingDeposit, accumulateAmount, expectedTotal)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
