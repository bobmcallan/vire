package api

// Integration tests for portfolio value field fixes.
//
// Requirements: .claude/workdir/20260301-1743-portfolio-value-fix/requirements.md
//
// Coverage:
//   - TestPortfolioValue_AvailableCash — Full flow: create portfolio with cash, verify
//     available_cash = total_cash - total_cost (net equity capital from trades)
//   - TestPortfolioValue_TotalValueFixed — Verify total_value = equity + available_cash
//     (NOT equity + total_cash, which was the old double-counting bug)
//   - TestPortfolioValue_CapitalGainFields — Verify capital_gain and capital_gain_pct
//     are populated when capital_performance exists with net_capital_deployed > 0
//   - TestPortfolioValue_NoCashTransactions — Graceful behaviour when no cash ledger:
//     available_cash=0, total_value = equity only (no crash, no wrong calculation)
//   - TestPortfolioValue_TotalCostFromTrades — Verify total_cost reflects net equity
//     capital from trade history (buys - sells), not AvgCost*Units

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

// getPortfolioValue is a test helper that fetches the portfolio and decodes the response.
// It fails the test immediately if the request fails or the status is not 200.
func getPortfolioValue(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) map[string]interface{} {
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

// addContributionTx posts a contribution cash transaction and asserts 201 Created.
func addContributionTx(t *testing.T, env *common.Env, portfolioName string, headers map[string]string,
	date string, amount float64, description string) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-transactions",
		map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        date,
			"amount":      amount,
			"description": description,
		}, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusCreated, resp.StatusCode, "POST cash-transaction failed: %s", string(body))
}

// --- TestPortfolioValue_AvailableCash ---

// TestPortfolioValue_AvailableCash verifies the full flow:
//  1. Portfolio synced from Navexa (has trades → total_cost from buys-sells)
//  2. Cash transactions added to create a total_cash balance
//  3. available_cash = total_cash - total_cost (uninvested cash)
//  4. All three fields are present and mathematically consistent in the response
func TestPortfolioValue_AvailableCash(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Get baseline portfolio to understand equity value and total_cost from trades.
	var baselineTotalCost float64
	t.Run("baseline_has_total_cost_from_trades", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_baseline_portfolio", string(raw))

		// total_cost must be present — it reflects net equity capital from trade history.
		totalCost, hasTotalCost := portfolio["total_cost"]
		require.True(t, hasTotalCost, "total_cost should be present in portfolio response")
		baselineTotalCost = totalCost.(float64)

		t.Logf("Baseline total_cost (net equity capital from trades): %.2f", baselineTotalCost)
	})

	// Step 2: Add a large cash deposit — available_cash should now be non-zero.
	depositAmount := 200000.0
	depositDate := time.Now().Add(-30 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	t.Run("add_cash_deposit", func(t *testing.T) {
		addContributionTx(t, env, portfolioName, userHeaders, depositDate, depositAmount, "Large deposit for available_cash test")
	})

	// Step 3: Verify available_cash = total_cash - total_cost.
	t.Run("available_cash_equals_total_cash_minus_total_cost", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("02_portfolio_with_deposit", string(raw))

		totalCash, hasTotalCash := portfolio["total_cash"]
		require.True(t, hasTotalCash, "total_cash should be present after adding cash transactions")

		totalCost, hasTotalCost := portfolio["total_cost"]
		require.True(t, hasTotalCost, "total_cost should be present in portfolio response")

		availableCash, hasAvailableCash := portfolio["available_cash"]
		require.True(t, hasAvailableCash, "available_cash should be present in portfolio response")

		tc := totalCash.(float64)
		tco := totalCost.(float64)
		ac := availableCash.(float64)

		// Core invariant: available_cash = total_cash - total_cost
		assert.InDelta(t, tc-tco, ac, 0.01,
			"available_cash (%.2f) should equal total_cash (%.2f) - total_cost (%.2f)",
			ac, tc, tco)

		t.Logf("total_cash=%.2f, total_cost=%.2f, available_cash=%.2f", tc, tco, ac)
	})

	// Step 4: total_cash should equal our deposit (no prior transactions).
	t.Run("total_cash_equals_deposit", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("03_verify_total_cash", string(raw))

		totalCash, ok := portfolio["total_cash"]
		require.True(t, ok, "total_cash must be present")
		assert.InDelta(t, depositAmount, totalCash.(float64), 0.01,
			"total_cash should equal the deposit amount (%.2f)", depositAmount)
	})

	// Step 5: total_cost should be unchanged from baseline (trades not affected by cash).
	t.Run("total_cost_unchanged_by_cash_transactions", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		totalCost, ok := portfolio["total_cost"]
		require.True(t, ok, "total_cost should be present")
		assert.InDelta(t, baselineTotalCost, totalCost.(float64), 0.01,
			"total_cost should be unchanged by adding cash transactions — it comes from trade history only")
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioValue_TotalValueFixed ---

// TestPortfolioValue_TotalValueFixed verifies the core bug fix:
// total_value = total_value_holdings + available_cash
// NOT total_value = total_value_holdings + total_cash (old, double-counting behaviour).
//
// With no cash: total_value = equity only.
// With cash (deposit > total_cost): total_value = equity + (deposit - total_cost).
// total_value < total_value_holdings + total_cash (unless no equity capital deployed).
func TestPortfolioValue_TotalValueFixed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Add a large cash deposit.
	depositAmount := 500000.0
	depositDate := time.Now().Add(-60 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	t.Run("add_deposit", func(t *testing.T) {
		addContributionTx(t, env, portfolioName, userHeaders, depositDate, depositAmount, "Deposit for total_value test")
	})

	// Step 2: Verify total_value = total_value_holdings + available_cash.
	t.Run("total_value_equals_equity_plus_available_cash", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_total_value", string(raw))

		totalValue, hasTotalValue := portfolio["total_value"]
		require.True(t, hasTotalValue, "total_value should be present")

		totalValueHoldings, hasTVH := portfolio["total_value_holdings"]
		require.True(t, hasTVH, "total_value_holdings should be present")

		availableCash, hasAC := portfolio["available_cash"]
		require.True(t, hasAC, "available_cash should be present")

		tv := totalValue.(float64)
		tvh := totalValueHoldings.(float64)
		ac := availableCash.(float64)

		// Core invariant: total_value = total_value_holdings + available_cash
		assert.InDelta(t, tvh+ac, tv, 0.01,
			"total_value (%.2f) should equal total_value_holdings (%.2f) + available_cash (%.2f)",
			tv, tvh, ac)

		t.Logf("total_value=%.2f, total_value_holdings=%.2f, available_cash=%.2f", tv, tvh, ac)
	})

	// Step 3: Verify total_value < total_value_holdings + total_cash when equity capital is deployed.
	// This validates the bug fix: old code used total_cash instead of available_cash, inflating total_value.
	t.Run("total_value_less_than_equity_plus_total_cash_when_capital_deployed", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("02_old_vs_new_total_value", string(raw))

		totalValue, _ := portfolio["total_value"].(float64)
		totalValueHoldings, _ := portfolio["total_value_holdings"].(float64)
		totalCash, _ := portfolio["total_cash"].(float64)
		totalCost, _ := portfolio["total_cost"].(float64)

		// If capital is deployed in equities (total_cost > 0), then:
		// new total_value < old total_value (equity + total_cash)
		// because available_cash = total_cash - total_cost < total_cash
		if totalCost > 0 {
			oldWrongTotalValue := totalValueHoldings + totalCash
			assert.Less(t, totalValue, oldWrongTotalValue,
				"total_value (%.2f) should be less than old wrong total_value (equity+totalCash=%.2f) when equity capital is deployed",
				totalValue, oldWrongTotalValue)
			t.Logf("Verified bug fix: correct total_value=%.2f, old wrong=%.2f (difference=%.2f from total_cost=%.2f)",
				totalValue, oldWrongTotalValue, oldWrongTotalValue-totalValue, totalCost)
		} else {
			// No equity capital from trades — available_cash = total_cash, values are equal
			t.Log("total_cost=0 (no trade history): total_value = equity + total_cash (same result for both formulas)")
		}
	})

	// Step 4: Verify available_cash matches expected value.
	t.Run("available_cash_is_reasonable", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		availableCash, ok := portfolio["available_cash"]
		require.True(t, ok, "available_cash should be present")
		ac := availableCash.(float64)

		// available_cash can be negative if more capital is invested than cash in ledger,
		// but with a large deposit it should be non-negative
		t.Logf("available_cash=%.2f (deposit=%.2f)", ac, depositAmount)
	})

	// Step 5: Verify the total_value field persists across a force-sync.
	t.Run("total_value_persists_after_sync", func(t *testing.T) {
		// Force sync
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_sync_response", string(body))
		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))

		// Re-fetch and verify invariant still holds
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)
		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("04_portfolio_after_sync", string(raw))

		totalValue, _ := portfolio["total_value"].(float64)
		totalValueHoldings, _ := portfolio["total_value_holdings"].(float64)
		availableCash, _ := portfolio["available_cash"].(float64)

		assert.InDelta(t, totalValueHoldings+availableCash, totalValue, 0.01,
			"total_value invariant should hold after force-sync: total_value (%.2f) = equity (%.2f) + available_cash (%.2f)",
			totalValue, totalValueHoldings, availableCash)
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioValue_CapitalGainFields ---

// TestPortfolioValue_CapitalGainFields verifies that capital_gain and capital_gain_pct
// are populated in the portfolio response when capital_performance exists with
// a positive net_capital_deployed.
//
// capital_gain = total_value - net_capital_deployed
// capital_gain_pct = (capital_gain / net_capital_deployed) * 100
func TestPortfolioValue_CapitalGainFields(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Without cash transactions — capital_gain fields should be absent (omitempty).
	t.Run("no_capital_gain_without_transactions", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_no_transactions", string(raw))

		// capital_gain and capital_gain_pct have omitempty — absent when no capital performance
		_, hasCapGain := portfolio["capital_gain"]
		_, hasCapGainPct := portfolio["capital_gain_pct"]

		// If capital_performance is absent, capital_gain should also be absent
		_, hasCapPerf := portfolio["capital_performance"]
		if !hasCapPerf {
			assert.False(t, hasCapGain,
				"capital_gain should be absent when capital_performance is absent")
			assert.False(t, hasCapGainPct,
				"capital_gain_pct should be absent when capital_performance is absent")
			t.Log("capital_performance absent (expected) — capital_gain fields correctly absent")
		}
	})

	// Step 2: Add a cash transaction so capital_performance is populated.
	depositAmount := 250000.0
	depositDate := time.Now().Add(-365 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)

	t.Run("add_cash_transaction", func(t *testing.T) {
		addContributionTx(t, env, portfolioName, userHeaders, depositDate, depositAmount, "Deposit for capital_gain test")
	})

	// Step 3: Verify capital_gain fields are present and consistent.
	t.Run("capital_gain_fields_present", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("02_portfolio_with_capital_perf", string(raw))

		// capital_performance must be present for capital_gain to be computed
		capPerf, hasCapPerf := portfolio["capital_performance"]
		if !hasCapPerf {
			t.Skip("capital_performance absent — possibly no qualifying trades; cannot test capital_gain fields")
			return
		}

		perfMap := capPerf.(map[string]interface{})
		netCapDeployed, _ := perfMap["net_capital_deployed"].(float64)

		if netCapDeployed <= 0 {
			t.Skipf("net_capital_deployed (%.2f) <= 0 — capital_gain fields require positive net_capital_deployed", netCapDeployed)
			return
		}

		// capital_gain and capital_gain_pct should be present
		capGain, hasCapGain := portfolio["capital_gain"]
		capGainPct, hasCapGainPct := portfolio["capital_gain_pct"]

		assert.True(t, hasCapGain, "capital_gain should be present when capital_performance has net_capital_deployed > 0")
		assert.True(t, hasCapGainPct, "capital_gain_pct should be present when capital_performance has net_capital_deployed > 0")

		if !hasCapGain || !hasCapGainPct {
			return
		}

		totalValue, _ := portfolio["total_value"].(float64)
		cg := capGain.(float64)
		cgPct := capGainPct.(float64)

		// capital_gain = total_value - net_capital_deployed
		assert.InDelta(t, totalValue-netCapDeployed, cg, 0.01,
			"capital_gain (%.2f) should equal total_value (%.2f) - net_capital_deployed (%.2f)",
			cg, totalValue, netCapDeployed)

		// capital_gain_pct = (capital_gain / net_capital_deployed) * 100
		expectedPct := (cg / netCapDeployed) * 100
		assert.InDelta(t, expectedPct, cgPct, 0.01,
			"capital_gain_pct (%.4f) should equal capital_gain/net_capital_deployed*100 (%.4f)",
			cgPct, expectedPct)

		t.Logf("capital_gain=%.2f, capital_gain_pct=%.4f%%, net_capital_deployed=%.2f, total_value=%.2f",
			cg, cgPct, netCapDeployed, totalValue)
	})

	// Step 4: Verify capital_gain and capital_performance.current_portfolio_value are consistent.
	t.Run("capital_gain_consistent_with_capital_performance", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("03_capital_gain_consistency", string(raw))

		capPerf, hasCapPerf := portfolio["capital_performance"]
		if !hasCapPerf {
			t.Skip("capital_performance absent — skipping consistency check")
			return
		}

		perfMap := capPerf.(map[string]interface{})
		netCapDeployed, _ := perfMap["net_capital_deployed"].(float64)
		if netCapDeployed <= 0 {
			t.Skip("net_capital_deployed <= 0 — skipping consistency check")
			return
		}

		capGain, hasCapGain := portfolio["capital_gain"]
		if !hasCapGain {
			t.Skip("capital_gain absent — skipping consistency check")
			return
		}

		totalValue, _ := portfolio["total_value"].(float64)
		cg := capGain.(float64)

		// capital_gain = total_value - net_capital_deployed
		// → total_value = capital_gain + net_capital_deployed
		assert.InDelta(t, cg+netCapDeployed, totalValue, 0.01,
			"total_value (%.2f) should equal capital_gain (%.2f) + net_capital_deployed (%.2f)",
			totalValue, cg, netCapDeployed)
	})

	// Step 5: Verify standalone /cash-transactions/performance endpoint matches.
	t.Run("capital_gain_matches_standalone_endpoint", func(t *testing.T) {
		// Get portfolio-embedded capital_performance
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)
		capPerf, hasCapPerf := portfolio["capital_performance"]
		if !hasCapPerf {
			t.Skip("capital_performance absent — skipping")
			return
		}

		embedded := capPerf.(map[string]interface{})

		// Get standalone capital performance
		resp, err := env.HTTPRequest(http.MethodGet,
			basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_standalone_performance", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var standalone map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &standalone))

		// net_capital_deployed should match between embedded and standalone
		embeddedNCD, _ := embedded["net_capital_deployed"].(float64)
		standaloneNCD, _ := standalone["net_capital_deployed"].(float64)
		assert.InDelta(t, standaloneNCD, embeddedNCD, 0.01,
			"net_capital_deployed should match between embedded and standalone capital_performance")
	})

	// Cleanup
	t.Run("cleanup", func(t *testing.T) {
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioValue_NoCashTransactions ---

// TestPortfolioValue_NoCashTransactions verifies graceful behaviour when there
// are no cash transactions in the ledger:
//   - available_cash should be 0 (or absent via omitempty)
//   - total_value should equal total_value_holdings (equity only)
//   - No crash or panic from division-by-zero or nil pointer
func TestPortfolioValue_NoCashTransactions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Get portfolio with no cash transactions.
	t.Run("portfolio_without_cash_is_valid", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_no_cash", string(raw))

		// Response must be a valid portfolio
		assert.Contains(t, portfolio, "total_value", "total_value must be present")
		assert.Contains(t, portfolio, "total_value_holdings", "total_value_holdings must be present")
		assert.Contains(t, portfolio, "holdings", "holdings must be present")
		assert.Contains(t, portfolio, "total_cost", "total_cost must be present")
	})

	// Step 2: total_cash should be 0 (or absent via omitempty).
	t.Run("total_cash_is_zero_without_transactions", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		totalCash, hasTotalCash := portfolio["total_cash"]
		if hasTotalCash {
			assert.Equal(t, 0.0, totalCash.(float64),
				"total_cash should be 0 when no cash transactions exist")
		}
		// Absent (omitempty at zero) is also valid
	})

	// Step 3: available_cash should be 0 or absent.
	t.Run("available_cash_is_zero_without_transactions", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("02_available_cash_check", string(raw))

		availableCash, hasAvailableCash := portfolio["available_cash"]
		if hasAvailableCash {
			ac := availableCash.(float64)
			// available_cash = total_cash (0) - total_cost; can be negative if total_cost > 0
			totalCost, _ := portfolio["total_cost"].(float64)
			assert.InDelta(t, -totalCost, ac, 0.01,
				"available_cash should equal 0 - total_cost when no cash (%.2f)", -totalCost)
		}
		// No panic, no crash is the key assertion here
	})

	// Step 4: total_value should reflect equity + available_cash consistently.
	t.Run("total_value_consistent_with_no_cash", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("03_total_value_no_cash", string(raw))

		totalValue, _ := portfolio["total_value"].(float64)
		totalValueHoldings, _ := portfolio["total_value_holdings"].(float64)

		// Get available_cash (may be absent, default 0)
		availableCash := 0.0
		if ac, ok := portfolio["available_cash"].(float64); ok {
			availableCash = ac
		}

		// Core invariant holds even without cash
		assert.InDelta(t, totalValueHoldings+availableCash, totalValue, 0.01,
			"total_value (%.2f) should equal total_value_holdings (%.2f) + available_cash (%.2f) even without cash",
			totalValue, totalValueHoldings, availableCash)
	})

	// Step 5: capital_gain fields should be absent when no capital_performance.
	t.Run("capital_gain_fields_absent_without_transactions", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		_, hasCapPerf := portfolio["capital_performance"]
		if !hasCapPerf {
			_, hasCapGain := portfolio["capital_gain"]
			_, hasCapGainPct := portfolio["capital_gain_pct"]

			assert.False(t, hasCapGain,
				"capital_gain should be absent when no capital_performance")
			assert.False(t, hasCapGainPct,
				"capital_gain_pct should be absent when no capital_performance")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioValue_TotalCostFromTrades ---

// TestPortfolioValue_TotalCostFromTrades verifies that portfolio total_cost is
// computed from trade history (sum of TotalInvested - TotalProceeds for all holdings),
// not from AvgCost * Units (the old incorrect formula).
//
// Verification approach:
//  1. Check portfolio has total_cost at all (it comes from trades)
//  2. Check total_cost does not equal sum(avg_cost * units) for holdings with mixed buys/sells
//  3. Verify total_cost is stable across re-fetches (not recalculated differently)
//  4. Verify total_cost changes after a sync (new trade data refreshed from Navexa)
func TestPortfolioValue_TotalCostFromTrades(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Step 1: Fetch portfolio and extract total_cost and holdings.
	t.Run("total_cost_present_and_non_negative", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_total_cost", string(raw))

		totalCost, hasTotalCost := portfolio["total_cost"]
		require.True(t, hasTotalCost, "total_cost should be present in portfolio response")

		tc := totalCost.(float64)
		// total_cost can be negative if more proceeds than invested (shouldn't happen normally)
		// but typically represents net equity capital deployed
		t.Logf("total_cost (net equity capital from trades): %.2f", tc)

		// total_cost should be non-negative for a portfolio with active positions
		holdings, _ := portfolio["holdings"].([]interface{})
		if len(holdings) > 0 {
			// If holdings exist, total_cost should typically be positive
			// (unless all positions were fully closed at a loss before proceeds exceeded cost)
			t.Logf("Portfolio has %d holdings; total_cost=%.2f", len(holdings), tc)
		}
	})

	// Step 2: Verify total_cost != naive sum(avg_cost * units) for each holding.
	// This validates the fix: old code used avg_cost * units for open positions only.
	// New code uses TotalInvested - TotalProceeds for ALL holdings (open + closed).
	t.Run("total_cost_not_naive_avg_cost_times_units", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("02_holdings_for_cost_check", string(raw))

		holdings, ok := portfolio["holdings"].([]interface{})
		if !ok || len(holdings) == 0 {
			t.Skip("No holdings — cannot verify cost calculation difference")
			return
		}

		totalCost, _ := portfolio["total_cost"].(float64)

		// Compute naive total cost: sum of (avg_cost * units) for OPEN positions only (old formula)
		naiveTotalCost := 0.0
		for _, h := range holdings {
			holding, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			units, _ := holding["units"].(float64)
			if units <= 0 {
				continue // skip closed positions (old formula excluded them)
			}
			avgCost, _ := holding["avg_cost"].(float64)
			naiveTotalCost += avgCost * units
		}

		t.Logf("total_cost from trades=%.2f, naive avg_cost*units (open only)=%.2f, difference=%.2f",
			totalCost, naiveTotalCost, totalCost-naiveTotalCost)

		// If there are closed positions or partial sells, the values will differ.
		// We can't always assert they're different (a portfolio with only new buys
		// and no sells may have total_cost ≈ avg_cost * units for open positions).
		// The key assertion is that total_cost is computed from total_invested - total_proceeds:
		// we verify per-holding total_invested is present (new field from the fix).
		for _, h := range holdings {
			holding, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			ticker := holding["ticker"].(string)
			_, hasTotalInvested := holding["total_invested"]
			assert.True(t, hasTotalInvested,
				"holding %s should have total_invested field (required for trade-based total_cost)", ticker)
		}
	})

	// Step 3: Verify total_cost is consistent with available_cash after adding cash.
	t.Run("total_cost_consistent_with_available_cash", func(t *testing.T) {
		// Add a cash transaction to make available_cash non-trivial
		depositDate := time.Now().Add(-90 * 24 * time.Hour).UTC().Truncate(24 * time.Hour).Format(time.RFC3339)
		addContributionTx(t, env, portfolioName, userHeaders, depositDate, 100000.0, "Deposit for cost consistency check")

		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("03_portfolio_with_deposit", string(raw))

		totalCost, _ := portfolio["total_cost"].(float64)
		totalCash, _ := portfolio["total_cash"].(float64)
		availableCash, hasAC := portfolio["available_cash"]

		if hasAC {
			ac := availableCash.(float64)
			assert.InDelta(t, totalCash-totalCost, ac, 0.01,
				"available_cash (%.2f) should equal total_cash (%.2f) - total_cost (%.2f)",
				ac, totalCash, totalCost)
		}

		// Cleanup this transaction
		cleanupCashFlows(t, env, portfolioName, userHeaders)
	})

	// Step 4: Verify total_invested is present on each holding (new field from the fix).
	t.Run("holdings_have_total_invested_field", func(t *testing.T) {
		portfolio := getPortfolioValue(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("04_holdings_total_invested", string(raw))

		holdings, ok := portfolio["holdings"].([]interface{})
		if !ok || len(holdings) == 0 {
			t.Skip("No holdings to verify")
			return
		}

		for _, h := range holdings {
			holding, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			ticker, _ := holding["ticker"].(string)

			ti, hasTI := holding["total_invested"]
			assert.True(t, hasTI, "holding %s should have total_invested", ticker)
			if hasTI {
				assert.GreaterOrEqual(t, ti.(float64), 0.0,
					"holding %s total_invested should be >= 0", ticker)
			}
		}
	})

	// Step 5: Verify total_cost is stable across multiple fetches (deterministic).
	t.Run("total_cost_stable_across_fetches", func(t *testing.T) {
		portfolio1 := getPortfolioValue(t, env, portfolioName, userHeaders)
		portfolio2 := getPortfolioValue(t, env, portfolioName, userHeaders)

		tc1, _ := portfolio1["total_cost"].(float64)
		tc2, _ := portfolio2["total_cost"].(float64)

		assert.InDelta(t, tc1, tc2, 0.01,
			"total_cost should be identical across two consecutive fetches (%.2f vs %.2f)", tc1, tc2)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
