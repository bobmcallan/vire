package api

// Integration tests for capital performance category filtering.
// Requirements: .claude/workdir/20260228-2300-capital-perf-category/requirements.md
//
// Verifies that TotalDeposited and TotalWithdrawn only count category=contribution
// transactions — not transfers, dividends, fees, or other categories.
//
// Scenarios:
//  1. Only contributions → TotalDeposited = sum of positive contributions
//  2. Contributions + internal transfer → transfer not counted in deposited/withdrawn
//  3. Contributions + dividend → dividend not counted as deposited
//  4. Negative contribution (withdrawal) → TotalWithdrawn = |amount|
//  5. Mixed categories → only contributions count for deposited/withdrawn

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

// addCashTx posts a cash transaction with the given category and amount.
// Returns (ledger, statusCode). On HTTP error (4xx/5xx), ledger is nil.
func addCashTx(t *testing.T, env *common.Env, portfolioName string, headers map[string]string,
	account, category string, amount float64, date, description string,
) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-transactions",
		map[string]interface{}{
			"account":     account,
			"category":    category,
			"date":        date,
			"amount":      amount,
			"description": description,
		}, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result, resp.StatusCode
}

// getCapitalPerf fetches capital performance metrics for a portfolio.
// Returns (result map, statusCode).
func getCapitalPerf(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result, resp.StatusCode
}

// assertCapitalPerf asserts TotalDeposited and TotalWithdrawn from a performance result.
func assertCapitalPerf(t *testing.T, result map[string]interface{}, wantDeposited, wantWithdrawn float64, msg string) {
	t.Helper()
	require.NotNil(t, result, "performance result must not be nil: %s", msg)

	totalDeposited, ok := result["total_deposited"].(float64)
	require.True(t, ok, "total_deposited should be a float64: %s", msg)

	totalWithdrawn, ok := result["total_withdrawn"].(float64)
	require.True(t, ok, "total_withdrawn should be a float64: %s", msg)

	assert.InDelta(t, wantDeposited, totalDeposited, 0.01,
		"total_deposited mismatch: %s (got %.2f, want %.2f)", msg, totalDeposited, wantDeposited)
	assert.InDelta(t, wantWithdrawn, totalWithdrawn, 0.01,
		"total_withdrawn mismatch: %s (got %.2f, want %.2f)", msg, totalWithdrawn, wantWithdrawn)
}

// dateStr returns a date string N days ago in RFC3339 format.
func dateStr(daysAgo int) string {
	return time.Now().Add(time.Duration(-daysAgo) * 24 * time.Hour).Format("2006-01-02T00:00:00Z")
}

// --- Test 1: Only Contributions ---

// TestCapitalPerfCategory_OnlyContributions verifies that when a portfolio has only
// contribution transactions, TotalDeposited equals the sum of positive contributions.
func TestCapitalPerfCategory_OnlyContributions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	contributions := []float64{100000.0, 25000.0, 10000.0}
	// TotalDeposited = 100000 + 25000 + 10000 = 135000
	expectedDeposited := 135000.0

	t.Run("add_contributions", func(t *testing.T) {
		for i, amount := range contributions {
			_, status := addCashTx(t, env, portfolioName, userHeaders,
				"Trading", "contribution", amount,
				dateStr(365-i*30),
				"Contribution only test deposit")
			require.Equal(t, http.StatusCreated, status, "contribution %d should be accepted", i+1)
		}
	})

	t.Run("performance_only_counts_contributions", func(t *testing.T) {
		result, status := getCapitalPerf(t, env, portfolioName, userHeaders)
		body, _ := json.Marshal(result)
		guard.SaveResult("01_only_contributions_performance", string(body))

		require.Equal(t, http.StatusOK, status)
		assertCapitalPerf(t, result, expectedDeposited, 0.0,
			"only contribution deposits — all should count as deposited, nothing withdrawn")

		// Net capital deployed = deposited - withdrawn = 135000 - 0
		netCapital, _ := result["net_capital_deployed"].(float64)
		assert.InDelta(t, expectedDeposited, netCapital, 0.01,
			"net_capital_deployed should equal total_deposited when no withdrawals")

		txCount, _ := result["transaction_count"].(float64)
		assert.Equal(t, float64(len(contributions)), txCount,
			"transaction_count should match number of contributions added")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 2: Contributions + Internal Transfer ---

// TestCapitalPerfCategory_TransferNotCounted verifies that internal transfer entries
// (category=transfer) are excluded from TotalDeposited and TotalWithdrawn.
// An internal transfer creates a +amount credit on to_account and -amount debit on
// from_account, but neither should count toward capital deposited/withdrawn.
func TestCapitalPerfCategory_TransferNotCounted(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	contributionAmount := 100000.0
	transferAmount := 20000.0

	// Step 1: Add contribution (should count)
	t.Run("add_contribution", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution", contributionAmount,
			dateStr(90),
			"Contribution for transfer test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 2: Add internal transfer via the transfer endpoint
	// This creates: -20000 on Trading (transfer debit) and +20000 on Accumulate (transfer credit)
	// Neither should count toward TotalDeposited or TotalWithdrawn.
	t.Run("add_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost,
			"/api/portfolios/"+portfolioName+"/cash-transactions/transfer",
			map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       transferAmount,
				"date":         dateStr(30),
				"description":  "Internal transfer for category test",
			}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_transfer_response", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "transfer should succeed: %s", string(body))
	})

	// Step 3: Verify performance — transfer entries are NOT counted
	t.Run("performance_excludes_transfer", func(t *testing.T) {
		result, status := getCapitalPerf(t, env, portfolioName, userHeaders)
		body, _ := json.Marshal(result)
		guard.SaveResult("02_performance_with_transfer", string(body))

		require.Equal(t, http.StatusOK, status)

		// Only the contribution counts: deposited = 100000, withdrawn = 0
		// Transfer credit (+20000) is NOT a deposit
		// Transfer debit (-20000) is NOT a withdrawal
		assertCapitalPerf(t, result, contributionAmount, 0.0,
			"transfer entries must not count as deposited or withdrawn")

		// Net capital deployed = 100000 - 0 = 100000
		netCapital, _ := result["net_capital_deployed"].(float64)
		assert.InDelta(t, contributionAmount, netCapital, 0.01,
			"net_capital_deployed should equal contribution only (transfers excluded)")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 3: Contributions + Dividend ---

// TestCapitalPerfCategory_DividendNotCounted verifies that dividend transactions
// (category=dividend) are excluded from TotalDeposited.
// Dividends are returns on investment — not capital deposited into the fund.
func TestCapitalPerfCategory_DividendNotCounted(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	contributionAmount := 80000.0
	dividendAmount := 5000.0

	// Step 1: Add contribution
	t.Run("add_contribution", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution", contributionAmount,
			dateStr(180),
			"Contribution for dividend test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 2: Add dividend (positive amount, but should NOT count as a deposit)
	t.Run("add_dividend", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "dividend", dividendAmount,
			dateStr(30),
			"BHP dividend — should not count as deposited")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 3: Verify performance — dividend is NOT counted in TotalDeposited
	t.Run("performance_excludes_dividend", func(t *testing.T) {
		result, status := getCapitalPerf(t, env, portfolioName, userHeaders)
		body, _ := json.Marshal(result)
		guard.SaveResult("01_performance_with_dividend", string(body))

		require.Equal(t, http.StatusOK, status)

		// Only the contribution counts: deposited = 80000 (not 85000)
		// Dividend (+5000) is NOT a deposit
		assertCapitalPerf(t, result, contributionAmount, 0.0,
			"dividend must not be counted as deposited capital")

		txCount, _ := result["transaction_count"].(float64)
		assert.Equal(t, float64(2), txCount, "should have 2 transactions (contribution + dividend)")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 4: Negative Contribution is Withdrawal ---

// TestCapitalPerfCategory_NegativeContributionIsWithdrawal verifies that a negative
// contribution (category=contribution, amount < 0) counts as capital withdrawn.
// TotalWithdrawn = |amount| of negative contributions.
func TestCapitalPerfCategory_NegativeContributionIsWithdrawal(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 100000.0
	withdrawalAmount := -25000.0 // negative contribution = capital withdrawal

	// Step 1: Add positive contribution (deposit)
	t.Run("add_deposit_contribution", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution", depositAmount,
			dateStr(90),
			"Initial deposit contribution")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 2: Add negative contribution (withdrawal)
	t.Run("add_withdrawal_contribution", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution", withdrawalAmount,
			dateStr(30),
			"Partial withdrawal contribution")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 3: Verify performance
	t.Run("performance_counts_negative_contribution_as_withdrawn", func(t *testing.T) {
		result, status := getCapitalPerf(t, env, portfolioName, userHeaders)
		body, _ := json.Marshal(result)
		guard.SaveResult("01_performance_with_withdrawal", string(body))

		require.Equal(t, http.StatusOK, status)

		// deposited = 100000 (positive contribution)
		// withdrawn = 25000 (|negative contribution|)
		// net = 100000 - 25000 = 75000
		expectedWithdrawn := 25000.0 // |-25000|
		assertCapitalPerf(t, result, depositAmount, expectedWithdrawn,
			"negative contribution should count as capital withdrawn")

		netCapital, _ := result["net_capital_deployed"].(float64)
		assert.InDelta(t, depositAmount+withdrawalAmount, netCapital, 0.01,
			"net_capital_deployed = deposited - withdrawn = 100000 - 25000 = 75000")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 5: Mixed Categories ---

// TestCapitalPerfCategory_MixedCategories verifies the full scenario:
// contributions + dividends + internal transfers + fees — only contributions count.
// This is the definitive test of the category filtering requirement.
func TestCapitalPerfCategory_MixedCategories(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	type txSpec struct {
		account     string
		category    string
		amount      float64
		daysAgo     int
		description string
		countsAs    string // "deposited", "withdrawn", or "excluded"
	}

	transactions := []txSpec{
		// Contributions — these ARE counted
		{"Trading", "contribution", 100000.0, 365, "Initial fund contribution", "deposited"},
		{"Trading", "contribution", 25000.0, 270, "Q2 employer contribution", "deposited"},
		{"Trading", "contribution", -10000.0, 200, "Partial withdrawal contribution", "withdrawn"},

		// Dividends — NOT counted as deposited
		{"Trading", "dividend", 3000.0, 180, "BHP interim dividend", "excluded"},
		{"Trading", "dividend", 1500.0, 90, "WBC final dividend", "excluded"},

		// Fees — NOT counted as withdrawn
		{"Trading", "fee", -250.0, 150, "Brokerage fee Q2", "excluded"},
		{"Trading", "fee", -180.0, 120, "Brokerage fee Q3", "excluded"},

		// Other — NOT counted
		{"Trading", "other", -500.0, 60, "Admin charge", "excluded"},
	}

	// Expected: deposited = 100000 + 25000 = 125000, withdrawn = 10000
	expectedDeposited := 125000.0
	expectedWithdrawn := 10000.0

	t.Run("add_all_transactions", func(t *testing.T) {
		for i, tx := range transactions {
			_, status := addCashTx(t, env, portfolioName, userHeaders,
				tx.account, tx.category, tx.amount,
				dateStr(tx.daysAgo),
				tx.description)
			require.Equal(t, http.StatusCreated, status,
				"transaction %d (%s/%s) should be accepted", i+1, tx.category, tx.description)
		}
	})

	// Also add an internal transfer via the transfer endpoint
	// This creates 2 entries with category=transfer — neither should count
	transferAmount := 15000.0
	t.Run("add_internal_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost,
			"/api/portfolios/"+portfolioName+"/cash-transactions/transfer",
			map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       transferAmount,
				"date":         dateStr(45),
				"description":  "Internal transfer — must not count",
			}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_transfer_response", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode, "transfer should succeed")
	})

	t.Run("performance_only_contributions_counted", func(t *testing.T) {
		result, status := getCapitalPerf(t, env, portfolioName, userHeaders)
		body, _ := json.Marshal(result)
		guard.SaveResult("02_mixed_performance", string(body))

		require.Equal(t, http.StatusOK, status)

		// Only category=contribution amounts count:
		// deposited: +100000 + +25000 = 125000
		// withdrawn: |-10000| = 10000
		// All other categories (dividend, fee, other, transfer) are excluded.
		assertCapitalPerf(t, result, expectedDeposited, expectedWithdrawn,
			"only contribution category counts for deposited/withdrawn")

		// Net capital = 125000 - 10000 = 115000
		netCapital, _ := result["net_capital_deployed"].(float64)
		assert.InDelta(t, expectedDeposited-expectedWithdrawn, netCapital, 0.01,
			"net_capital_deployed = deposited - withdrawn = 125000 - 10000 = 115000")

		// Transaction count includes ALL transactions (not just contributions)
		txCount, _ := result["transaction_count"].(float64)
		// 8 direct transactions + 2 transfer entries = 10 total
		assert.Equal(t, float64(len(transactions)+2), txCount,
			"transaction_count should include all transactions regardless of category")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 6: Fee Debits Not Counted as Withdrawn ---

// TestCapitalPerfCategory_FeeNotCounted verifies that fee debit transactions
// (category=fee, negative amount) are NOT counted as capital withdrawn.
// Fees reduce cash balance but are not capital returned to the fund owner.
func TestCapitalPerfCategory_FeeNotCounted(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	contributionAmount := 50000.0
	feeAmount := -500.0

	t.Run("add_contribution", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution", contributionAmount,
			dateStr(90),
			"Contribution for fee test")
		require.Equal(t, http.StatusCreated, status)
	})

	t.Run("add_fee", func(t *testing.T) {
		_, status := addCashTx(t, env, portfolioName, userHeaders,
			"Trading", "fee", feeAmount,
			dateStr(30),
			"Brokerage fee — should not count as withdrawn")
		require.Equal(t, http.StatusCreated, status)
	})

	t.Run("performance_excludes_fee_from_withdrawn", func(t *testing.T) {
		result, status := getCapitalPerf(t, env, portfolioName, userHeaders)
		body, _ := json.Marshal(result)
		guard.SaveResult("01_performance_with_fee", string(body))

		require.Equal(t, http.StatusOK, status)

		// deposited = 50000 (contribution only)
		// withdrawn = 0 (fee is NOT counted as a withdrawal of capital)
		assertCapitalPerf(t, result, contributionAmount, 0.0,
			"fee debit must not count as capital withdrawn")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
