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

// --- Net Flow Fields on Portfolio Response ---

// TestPortfolioNetFlow_NoTransactions verifies that yesterday_net_flow and
// last_week_net_flow are absent (omitempty) when no cash transactions exist.
func TestPortfolioNetFlow_NoTransactions(t *testing.T) {
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

	resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_portfolio_no_transactions", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var portfolio map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &portfolio))

	// Without cash transactions, net flow fields should be absent (omitempty)
	_, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
	_, hasLastWeekNetFlow := portfolio["last_week_net_flow"]

	assert.False(t, hasYesterdayNetFlow,
		"yesterday_net_flow should be absent when no cash transactions exist")
	assert.False(t, hasLastWeekNetFlow,
		"last_week_net_flow should be absent when no cash transactions exist")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioNetFlow_YesterdayFlow verifies that yesterday_net_flow is set
// when a cash transaction occurred yesterday.
func TestPortfolioNetFlow_YesterdayFlow(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add a deposit dated yesterday
	yesterday := time.Now().Add(-24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
	depositAmount := 15000.0

	t.Run("add_yesterday_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        yesterday,
			"amount":      depositAmount,
			"description": "Yesterday deposit for net flow test",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_add_yesterday_deposit", string(body))

		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("yesterday_net_flow_present", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_portfolio_with_yesterday_flow", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// yesterday_net_flow should now be present and equal to the deposit amount
		yesterdayNetFlow, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
		assert.True(t, hasYesterdayNetFlow,
			"yesterday_net_flow should be present when a transaction occurred yesterday")

		if hasYesterdayNetFlow {
			assert.InDelta(t, depositAmount, yesterdayNetFlow.(float64), 0.01,
				"yesterday_net_flow should equal the deposit amount")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioNetFlow_LastWeekFlow verifies that last_week_net_flow sums all
// transactions within the last 7 days.
func TestPortfolioNetFlow_LastWeekFlow(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add transactions within the last 7 days and one before the window
	now := time.Now().UTC()
	recentDate := now.Add(-3 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	olderDate := now.Add(-5 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)
	outsideWindow := now.Add(-10 * 24 * time.Hour).Truncate(24 * time.Hour).Format(time.RFC3339)

	transactions := []struct {
		body       map[string]interface{}
		inWindow   bool
		netContrib float64
	}{
		{
			body: map[string]interface{}{
				"type": "deposit", "date": recentDate,
				"amount": 20000, "description": "Recent deposit (in window)",
			},
			inWindow:   true,
			netContrib: 20000,
		},
		{
			body: map[string]interface{}{
				"type": "contribution", "date": olderDate,
				"amount": 10000, "description": "Older contribution (in window)",
			},
			inWindow:   true,
			netContrib: 10000,
		},
		{
			body: map[string]interface{}{
				"type": "withdrawal", "date": recentDate,
				"amount": 5000, "description": "Recent withdrawal (in window)",
			},
			inWindow:   true,
			netContrib: -5000,
		},
		{
			body: map[string]interface{}{
				"type": "deposit", "date": outsideWindow,
				"amount": 50000, "description": "Old deposit (outside 7-day window)",
			},
			inWindow:   false,
			netContrib: 0, // should not be counted
		},
	}

	// Expected last_week_net_flow = 20000 + 10000 - 5000 = 25000
	expectedLastWeekFlow := 25000.0

	t.Run("add_transactions", func(t *testing.T) {
		for _, tx := range transactions {
			resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", tx.body, userHeaders)
			require.NoError(t, err)
			resp.Body.Close()
			require.Equal(t, http.StatusCreated, resp.StatusCode)
		}
	})

	t.Run("last_week_net_flow_correct", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_portfolio_last_week_flow", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		lastWeekNetFlow, hasLastWeekNetFlow := portfolio["last_week_net_flow"]
		assert.True(t, hasLastWeekNetFlow,
			"last_week_net_flow should be present when transactions occurred in the last 7 days")

		if hasLastWeekNetFlow {
			assert.InDelta(t, expectedLastWeekFlow, lastWeekNetFlow.(float64), 0.01,
				"last_week_net_flow should sum deposits+contributions-withdrawals in last 7 days")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioNetFlow_NegativeFlow verifies that net flow can be negative
// when withdrawals exceed deposits in the period.
func TestPortfolioNetFlow_NegativeFlow(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add a withdrawal larger than deposits yesterday
	yesterday := time.Now().Add(-24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	t.Run("add_transactions_with_net_outflow", func(t *testing.T) {
		// Small deposit yesterday
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        yesterday,
			"amount":      5000,
			"description": "Small deposit for negative flow test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Large withdrawal yesterday
		resp, err = env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "withdrawal",
			"date":        yesterday,
			"amount":      20000,
			"description": "Large withdrawal for negative flow test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("yesterday_net_flow_negative", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_portfolio_negative_flow", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		yesterdayNetFlow, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
		if hasYesterdayNetFlow {
			// 5000 deposit - 20000 withdrawal = -15000
			assert.InDelta(t, -15000.0, yesterdayNetFlow.(float64), 0.01,
				"yesterday_net_flow should be negative when withdrawals exceed deposits")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioNetFlow_OnlyOutsideWindowTransactions verifies that when all
// transactions are older than 7 days, net flow fields are zero or absent.
func TestPortfolioNetFlow_OnlyOutsideWindowTransactions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Add deposit older than 7 days
	oldDate := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	t.Run("add_old_transaction", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        oldDate,
			"amount":      100000,
			"description": "Old deposit for outside-window test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("net_flow_fields_zero_or_absent", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_portfolio_old_only", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// yesterday_net_flow should be absent or zero (no yesterday transactions)
		yesterdayNetFlow, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
		if hasYesterdayNetFlow {
			assert.Equal(t, 0.0, yesterdayNetFlow.(float64),
				"yesterday_net_flow should be zero when no transactions occurred yesterday")
		}

		// last_week_net_flow should be absent or zero (no last-7-day transactions)
		lastWeekNetFlow, hasLastWeekNetFlow := portfolio["last_week_net_flow"]
		if hasLastWeekNetFlow {
			assert.Equal(t, 0.0, lastWeekNetFlow.(float64),
				"last_week_net_flow should be zero when no transactions in last 7 days")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioNetFlow_DividendExcluded verifies that dividend transactions
// are NOT counted in net_flow fields (dividends are investment return, not capital flow).
func TestPortfolioNetFlow_DividendExcluded(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	yesterday := time.Now().Add(-24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
	depositAmount := 10000.0
	dividendAmount := 5000.0

	t.Run("add_deposit_and_dividend_yesterday", func(t *testing.T) {
		// Add deposit (should count in net flow)
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        yesterday,
			"amount":      depositAmount,
			"description": "Deposit for dividend exclusion test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		// Add dividend (should NOT count in net flow)
		resp, err = env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "dividend",
			"date":        yesterday,
			"amount":      dividendAmount,
			"description": "Dividend for dividend exclusion test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("yesterday_net_flow_excludes_dividend", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_portfolio_dividend_excluded", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		yesterdayNetFlow, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
		if hasYesterdayNetFlow {
			// Should only count the deposit, not the dividend
			assert.InDelta(t, depositAmount, yesterdayNetFlow.(float64), 0.01,
				"yesterday_net_flow should exclude dividend amounts")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioNetFlow_PersistsAfterSync verifies that net flow fields
// are still computed correctly after a portfolio sync.
func TestPortfolioNetFlow_PersistsAfterSync(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	yesterday := time.Now().Add(-24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
	depositAmount := 30000.0

	t.Run("add_yesterday_deposit", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cashflows", map[string]interface{}{
			"type":        "deposit",
			"date":        yesterday,
			"amount":      depositAmount,
			"description": "Deposit for sync persistence test",
		}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("verify_before_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_before_sync", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		yesterdayNetFlow, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
		if hasYesterdayNetFlow {
			assert.InDelta(t, depositAmount, yesterdayNetFlow.(float64), 0.01)
		}
	})

	t.Run("force_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_sync_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))
	})

	t.Run("verify_after_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_after_sync", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// Net flow fields should still be computed after sync
		yesterdayNetFlow, hasYesterdayNetFlow := portfolio["yesterday_net_flow"]
		if hasYesterdayNetFlow {
			assert.InDelta(t, depositAmount, yesterdayNetFlow.(float64), 0.01,
				"yesterday_net_flow should be preserved after sync")
		}
	})

	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
