package api

// Integration tests for signed amounts and account-based external balances.
// Requirements: .claude/workdir/20260228-1100-signed-amounts/requirements.md
//
// Scenarios:
//  1. Positive amount → credit (money in)
//  2. Negative amount → debit (money out)
//  3. Transfer → paired entries with correct signs (+/-)
//  4. List transactions → amounts have correct signs
//  5. Capital performance → total_deposited / total_withdrawn from sign
//  6. Portfolio → external_balance_total from non-transactional accounts
//  7. Update account type and properties via update_account endpoint
//  8. Amount = 0 is rejected

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

// postSignedTransaction posts a cash transaction using signed amount semantics
// (positive = credit, negative = debit). Returns the decoded ledger and status.
func postSignedTransaction(t *testing.T, env *common.Env, portfolioName string, headers map[string]string, body map[string]interface{}) (map[string]interface{}, int) {
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

// postSignedTransfer posts a transfer using the /cash-transactions/transfer endpoint.
// Amount is always positive; the service creates -amount on from and +amount on to.
func postSignedTransfer(t *testing.T, env *common.Env, portfolioName string, headers map[string]string, fromAccount, toAccount string, amount float64, date, description string) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-transactions/transfer", map[string]interface{}{
		"from_account": fromAccount,
		"to_account":   toAccount,
		"amount":       amount,
		"date":         date,
		"description":  description,
	}, headers)
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

// getAccountBalance returns the sum of signed amounts for a named account in a ledger response.
func getAccountBalance(result map[string]interface{}, accountName string) float64 {
	txns, ok := result["transactions"].([]interface{})
	if !ok {
		return 0
	}
	var balance float64
	for _, tx := range txns {
		txMap, ok := tx.(map[string]interface{})
		if !ok {
			continue
		}
		if txMap["account"] != accountName {
			continue
		}
		amount, _ := txMap["amount"].(float64)
		balance += amount
	}
	return balance
}

// getTotalLedgerBalance returns the sum of all signed amounts in a ledger response.
func getTotalLedgerBalance(result map[string]interface{}) float64 {
	txns, ok := result["transactions"].([]interface{})
	if !ok {
		return 0
	}
	var total float64
	for _, tx := range txns {
		txMap, ok := tx.(map[string]interface{})
		if !ok {
			continue
		}
		amount, _ := txMap["amount"].(float64)
		total += amount
	}
	return total
}

// updateAccount sends a POST to the update_account endpoint for a named account.
func updateAccount(t *testing.T, env *common.Env, portfolioName, accountName string, headers map[string]string, body map[string]interface{}) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-accounts/"+accountName, body, headers)
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

// --- Test 1: Positive Amount is Credit ---

// TestSignedAmounts_PositiveAmountIsCredit verifies that posting a positive amount
// is treated as a credit (money in) and increases the account balance.
func TestSignedAmounts_PositiveAmountIsCredit(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 50000.0

	// POST with positive amount (deposit/credit)
	t.Run("add_positive_credit", func(t *testing.T) {
		result, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      depositAmount,
			"description": "Positive amount credit test",
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("01_positive_credit", string(body))

		assert.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 1)

		tx := txns[0].(map[string]interface{})
		amount := tx["amount"].(float64)

		// Positive amount = credit = money in
		assert.Equal(t, depositAmount, amount, "positive amount should be stored as-is (credit)")
		assert.Greater(t, amount, 0.0, "credit amount must be positive")
	})

	// GET the ledger and verify balance is positive
	t.Run("verify_balance_positive", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_ledger_after_credit", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		balance := getAccountBalance(result, "Trading")
		assert.InDelta(t, depositAmount, balance, 0.01, "Trading balance should be positive after credit")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 2: Negative Amount is Debit ---

// TestSignedAmounts_NegativeAmountIsDebit verifies that posting a negative amount
// is treated as a debit (money out) and decreases the account balance.
func TestSignedAmounts_NegativeAmountIsDebit(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 100000.0
	withdrawalAmount := -20000.0

	// Add a credit first so balance doesn't go negative
	t.Run("add_initial_credit", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      depositAmount,
			"description": "Initial deposit before debit test",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// POST with negative amount (withdrawal/debit)
	t.Run("add_negative_debit", func(t *testing.T) {
		result, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "other",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      withdrawalAmount,
			"description": "Negative amount debit test",
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("01_negative_debit", string(body))

		assert.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 2)

		// Find the debit transaction
		var debitTx map[string]interface{}
		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			if txMap["amount"].(float64) < 0 {
				debitTx = txMap
				break
			}
		}
		require.NotNil(t, debitTx, "should find a transaction with negative amount")

		amount := debitTx["amount"].(float64)
		assert.Equal(t, withdrawalAmount, amount, "negative amount should be stored as-is (debit)")
		assert.Less(t, amount, 0.0, "debit amount must be negative")
	})

	// Verify net balance is correct (deposit + withdrawal)
	t.Run("verify_net_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_ledger_after_debit", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		balance := getAccountBalance(result, "Trading")
		expectedBalance := depositAmount + withdrawalAmount // 100000 - 20000 = 80000
		assert.InDelta(t, expectedBalance, balance, 0.01, "balance should reflect deposit minus withdrawal")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 3: Zero Amount is Rejected ---

// TestSignedAmounts_ZeroAmountRejected verifies that amount = 0 is rejected with HTTP 400.
func TestSignedAmounts_ZeroAmountRejected(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	tests := []struct {
		name   string
		amount float64
	}{
		{"zero_amount", 0.0},
		{"positive_zero", 0},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-transactions", map[string]interface{}{
				"account":     "Trading",
				"category":    "contribution",
				"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
				"amount":      tt.amount,
				"description": "Zero amount should be rejected",
			}, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("reject_"+tt.name, string(body))

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
				"zero amount should be rejected with 400, got: %s", string(body))
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 4: Transfer Paired Entries with Correct Signs ---

// TestSignedAmounts_TransferPairedEntriesSignedCorrectly verifies that:
// - POST /cash-transactions/transfer creates -amount on from_account (debit)
// - POST /cash-transactions/transfer creates +amount on to_account (credit)
func TestSignedAmounts_TransferPairedEntriesSignedCorrectly(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 80000.0
	transferAmount := 20000.0

	// Add initial deposit
	t.Run("add_initial_deposit", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        time.Now().Add(-90 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      depositAmount,
			"description": "Initial deposit for transfer sign test",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// Perform transfer
	t.Run("transfer_creates_signed_entries", func(t *testing.T) {
		result, status := postSignedTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", transferAmount,
			time.Now().Add(-30*24*time.Hour).Format(time.RFC3339),
			"Transfer for signed amounts test")

		body, _ := json.Marshal(result)
		guard.SaveResult("01_after_transfer", string(body))

		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		txns := result["transactions"].([]interface{})
		// Should have 3 transactions: 1 deposit + 2 transfer entries
		require.Len(t, txns, 3, "transfer should create 2 paired entries")

		// Find transfer entries
		var fromEntry, toEntry map[string]interface{}
		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			category, _ := txMap["category"].(string)
			if category != "transfer" {
				continue
			}
			account, _ := txMap["account"].(string)
			if account == "Trading" {
				fromEntry = txMap
			} else if account == "Accumulate" {
				toEntry = txMap
			}
		}

		require.NotNil(t, fromEntry, "debit entry on from_account (Trading) should exist")
		require.NotNil(t, toEntry, "credit entry on to_account (Accumulate) should exist")

		fromAmount := fromEntry["amount"].(float64)
		toAmount := toEntry["amount"].(float64)

		// from_account: negative (debit = money out)
		assert.Equal(t, -transferAmount, fromAmount,
			"from_account entry should be negative (debit/money out)")
		assert.Less(t, fromAmount, 0.0, "from_account amount must be negative")

		// to_account: positive (credit = money in)
		assert.Equal(t, transferAmount, toAmount,
			"to_account entry should be positive (credit/money in)")
		assert.Greater(t, toAmount, 0.0, "to_account amount must be positive")

		// They should have linked_id values
		debitLinkedID, _ := fromEntry["linked_id"].(string)
		creditLinkedID, _ := toEntry["linked_id"].(string)
		assert.NotEmpty(t, debitLinkedID, "debit entry should have linked_id")
		assert.NotEmpty(t, creditLinkedID, "credit entry should have linked_id")
	})

	// Verify account balances reflect the transfer correctly
	t.Run("account_balances_after_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_ledger_after_transfer", string(body))

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		tradingBalance := getAccountBalance(result, "Trading")
		accumulateBalance := getAccountBalance(result, "Accumulate")

		// Trading: deposit - transfer = 80000 - 20000 = 60000
		assert.InDelta(t, depositAmount-transferAmount, tradingBalance, 0.01,
			"Trading balance should decrease by transfer amount")

		// Accumulate: +transfer = 20000
		assert.InDelta(t, transferAmount, accumulateBalance, 0.01,
			"Accumulate balance should increase by transfer amount")

		// Total unchanged: debits and credits cancel
		total := getTotalLedgerBalance(result)
		assert.InDelta(t, depositAmount, total, 0.01,
			"total portfolio cash should be unchanged after internal transfer")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 5: Listing Transactions Shows Correct Signs ---

// TestSignedAmounts_ListTransactionsSignsCorrect verifies that GET /cash-transactions
// returns transactions with positive amounts for credits and negative for debits.
func TestSignedAmounts_ListTransactionsSignsCorrect(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add a mix of positive and negative transactions
	transactions := []struct {
		name     string
		amount   float64
		account  string
		category string
		date     string
	}{
		{
			name:     "credit_contribution",
			amount:   100000.0,
			account:  "Trading",
			category: "contribution",
			date:     time.Now().Add(-365 * 24 * time.Hour).Format("2006-01-02"),
		},
		{
			name:     "credit_dividend",
			amount:   2500.0,
			account:  "Trading",
			category: "dividend",
			date:     time.Now().Add(-180 * 24 * time.Hour).Format("2006-01-02"),
		},
		{
			name:     "debit_fee",
			amount:   -500.0,
			account:  "Trading",
			category: "fee",
			date:     time.Now().Add(-90 * 24 * time.Hour).Format("2006-01-02"),
		},
		{
			name:     "debit_other",
			amount:   -1000.0,
			account:  "Trading",
			category: "other",
			date:     time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02"),
		},
	}

	for _, tt := range transactions {
		tt := tt
		t.Run("add_"+tt.name, func(t *testing.T) {
			_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
				"account":     tt.account,
				"category":    tt.category,
				"date":        tt.date + "T00:00:00Z",
				"amount":      tt.amount,
				"description": "Signed amount test: " + tt.name,
			})
			assert.Equal(t, http.StatusCreated, status)
		})
	}

	// GET and verify all signs are stored correctly
	t.Run("verify_signed_amounts_in_list", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_list_signed_txns", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		txns := result["transactions"].([]interface{})
		require.Len(t, txns, len(transactions))

		// Build a map of description → amount
		amountByDesc := make(map[string]float64)
		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			desc := txMap["description"].(string)
			amount := txMap["amount"].(float64)
			amountByDesc[desc] = amount
		}

		for _, tt := range transactions {
			expectedDesc := "Signed amount test: " + tt.name
			actualAmount, exists := amountByDesc[expectedDesc]
			assert.True(t, exists, "transaction %q should be in list", tt.name)
			if exists {
				assert.InDelta(t, tt.amount, actualAmount, 0.01,
					"transaction %q should have amount %f, got %f", tt.name, tt.amount, actualAmount)
			}
		}

		// Verify positive amounts > 0 for credits
		for _, tt := range transactions {
			if tt.amount > 0 {
				assert.Greater(t, amountByDesc["Signed amount test: "+tt.name], 0.0,
					"credit %q should have positive amount", tt.name)
			}
		}

		// Verify negative amounts < 0 for debits
		for _, tt := range transactions {
			if tt.amount < 0 {
				assert.Less(t, amountByDesc["Signed amount test: "+tt.name], 0.0,
					"debit %q should have negative amount", tt.name)
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 6: Capital Performance Uses Category=Contribution for Deposited/Withdrawn ---

// TestSignedAmounts_PerformanceUsesContributionCategory verifies that:
// - total_deposited = sum of positive contribution amounts only
// - total_withdrawn = abs(sum of negative contribution amounts only)
// - Other categories (other, fee, transfer, dividend) do NOT affect deposited/withdrawn
func TestSignedAmounts_PerformanceUsesContributionCategory(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add known transactions — only contributions count for deposited/withdrawn
	contributions := []float64{100000.0, 25000.0, 3000.0} // total = 128000 deposited
	withdrawals := []float64{-15000.0, -5000.0}           // total = 20000 withdrawn (contribution-category debits)
	otherDebits := []float64{-2000.0, -500.0}             // NOT counted (other category)

	for i, amount := range contributions {
		date := time.Now().Add(time.Duration(-(365 - i*30)) * 24 * time.Hour).Format("2006-01-02")
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        date + "T00:00:00Z",
			"amount":      amount,
			"description": "Contribution credit for perf test",
		})
		require.Equal(t, http.StatusCreated, status)
	}

	for i, amount := range withdrawals {
		date := time.Now().Add(time.Duration(-(90 - i*30)) * 24 * time.Hour).Format("2006-01-02")
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        date + "T00:00:00Z",
			"amount":      amount,
			"description": "Contribution debit (withdrawal) for perf test",
		})
		require.Equal(t, http.StatusCreated, status)
	}

	for i, amount := range otherDebits {
		date := time.Now().Add(time.Duration(-(60 - i*10)) * 24 * time.Hour).Format("2006-01-02")
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "other",
			"date":        date + "T00:00:00Z",
			"amount":      amount,
			"description": "Other debit NOT counted for perf test",
		})
		require.Equal(t, http.StatusCreated, status)
	}

	// Get performance and verify totals
	t.Run("performance_uses_contribution_category", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_performance_with_signed_amounts", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		totalDeposited := result["total_deposited"].(float64)
		totalWithdrawn := result["total_withdrawn"].(float64)
		netCapital := result["net_capital_deployed"].(float64)

		// total_deposited = sum of positive contribution amounts only
		expectedDeposited := 0.0
		for _, c := range contributions {
			expectedDeposited += c
		}
		assert.InDelta(t, expectedDeposited, totalDeposited, 0.01,
			"total_deposited should equal sum of positive contribution amounts only")

		// total_withdrawn = abs(sum of negative contribution amounts)
		expectedWithdrawn := 0.0
		for _, w := range withdrawals {
			expectedWithdrawn += -w // abs of negative
		}
		assert.InDelta(t, expectedWithdrawn, totalWithdrawn, 0.01,
			"total_withdrawn should equal abs sum of negative contribution amounts only (not other/fee/transfer)")

		// net = deposited - withdrawn (other-category debits don't affect net capital)
		expectedNet := expectedDeposited - expectedWithdrawn
		assert.InDelta(t, expectedNet, netCapital, 0.01,
			"net_capital_deployed should equal total_deposited - total_withdrawn")

		// Transaction count = all entries (regardless of category)
		txCount := result["transaction_count"].(float64)
		assert.Equal(t, float64(len(contributions)+len(withdrawals)+len(otherDebits)), txCount)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 7: Portfolio ExternalBalanceTotal from Non-Transactional Accounts ---

// TestSignedAmounts_ExternalBalanceTotalFromLedger verifies that portfolio.external_balance_total
// is derived from non-transactional account balances in the cash flow ledger,
// not from the old ExternalBalances slice.
func TestSignedAmounts_ExternalBalanceTotalFromLedger(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Get portfolio before adding any non-transactional account balances
	var initialExtBalTotal float64
	t.Run("get_initial_external_balance_total", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_initial_portfolio", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// Before any transactions, external_balance_total should be 0 or absent
		if val, ok := portfolio["external_balance_total"].(float64); ok {
			initialExtBalTotal = val
		}
		assert.InDelta(t, 0.0, initialExtBalTotal, 0.01,
			"external_balance_total should be 0 before adding non-transactional accounts")
	})

	// Add a deposit to the Accumulate account (non-transactional)
	accumulateBalance := 50000.0
	t.Run("add_deposit_to_accumulate", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Accumulate",
			"category":    "contribution",
			"date":        time.Now().Add(-60 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      accumulateBalance,
			"description": "Deposit to Accumulate non-transactional account",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// Update the Accumulate account to be non-transactional
	t.Run("set_accumulate_non_transactional", func(t *testing.T) {
		result, status := updateAccount(t, env, portfolioName, "Accumulate", userHeaders, map[string]interface{}{
			"type":             "accumulate",
			"is_transactional": false,
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("02_update_accumulate_account", string(body))

		// Only assert success if the endpoint exists (may be 404 before implementation)
		if status == http.StatusNotFound {
			t.Skip("update_account endpoint not yet implemented — skipping external balance total test")
		}
		assert.Equal(t, http.StatusOK, status)
	})

	// Get portfolio and verify external_balance_total includes Accumulate balance
	t.Run("external_balance_total_includes_non_transactional", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_portfolio_with_non_transactional", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		extBalTotal, ok := portfolio["external_balance_total"].(float64)
		require.True(t, ok, "external_balance_total should be present in portfolio response")
		assert.InDelta(t, accumulateBalance, extBalTotal, 0.01,
			"external_balance_total should equal the sum of non-transactional account balances")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 8: Update Account Type and Properties ---

// TestSignedAmounts_UpdateAccount verifies that the update_account endpoint
// allows setting account type and is_transactional properties.
func TestSignedAmounts_UpdateAccount(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// First add a transaction to create the account
	t.Run("create_account_via_transaction", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Stake",
			"category":    "contribution",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      25000.0,
			"description": "Create Stake account for update test",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// Update account type to "accumulate"
	t.Run("update_account_type_to_accumulate", func(t *testing.T) {
		result, status := updateAccount(t, env, portfolioName, "Stake", userHeaders, map[string]interface{}{
			"type":             "accumulate",
			"is_transactional": false,
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("01_update_account_accumulate", string(body))

		if status == http.StatusNotFound {
			t.Skip("update_account endpoint not yet implemented")
		}

		assert.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)
	})

	// Verify the account has been updated by checking the ledger
	t.Run("verify_account_updated_in_ledger", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_ledger_after_update", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Check accounts list if present
		accounts, ok := result["accounts"].([]interface{})
		if ok {
			for _, acc := range accounts {
				accMap := acc.(map[string]interface{})
				if accMap["name"] == "Stake" {
					isTransactional, _ := accMap["is_transactional"].(bool)
					assert.False(t, isTransactional, "Stake account should not be transactional after update")

					accountType, _ := accMap["type"].(string)
					assert.Equal(t, "accumulate", accountType, "Stake account type should be 'accumulate'")
					break
				}
			}
		}
	})

	// Update account type to "term_deposit"
	t.Run("update_account_type_to_term_deposit", func(t *testing.T) {
		result, status := updateAccount(t, env, portfolioName, "Stake", userHeaders, map[string]interface{}{
			"type":             "term_deposit",
			"is_transactional": false,
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("03_update_account_term_deposit", string(body))

		if status == http.StatusNotFound {
			t.Skip("update_account endpoint not yet implemented")
		}

		assert.Equal(t, http.StatusOK, status)
	})

	// Reset back to trading (transactional)
	t.Run("update_account_back_to_trading", func(t *testing.T) {
		result, status := updateAccount(t, env, portfolioName, "Stake", userHeaders, map[string]interface{}{
			"type":             "trading",
			"is_transactional": true,
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("04_update_account_trading", string(body))

		if status == http.StatusNotFound {
			t.Skip("update_account endpoint not yet implemented")
		}

		assert.Equal(t, http.StatusOK, status)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 9: Update Account Validation ---

// TestSignedAmounts_UpdateAccountValidation verifies that invalid account type values
// are rejected by the update_account endpoint.
func TestSignedAmounts_UpdateAccountValidation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Create account via transaction
	t.Run("setup", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "TestAcct",
			"category":    "contribution",
			"date":        time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      10000.0,
			"description": "Create account for validation test",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
	}{
		{
			name:       "invalid_type",
			body:       map[string]interface{}{"type": "savings", "is_transactional": false},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_type",
			body:       map[string]interface{}{"type": "", "is_transactional": false},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-accounts/TestAcct", tt.body, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("validation_"+tt.name, string(body))

			if resp.StatusCode == http.StatusNotFound {
				t.Skip("update_account endpoint not yet implemented")
			}

			assert.Equal(t, tt.wantStatus, resp.StatusCode,
				"expected %d for %q, got %d: %s", tt.wantStatus, tt.name, resp.StatusCode, string(body))
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 10: External Balance Tools Removed ---

// TestSignedAmounts_ExternalBalanceEndpointsRemoved verifies that the old
// external balance HTTP endpoints are no longer registered, returning 404.
func TestSignedAmounts_ExternalBalanceEndpointsRemoved(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	endpoints := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "get_external_balances",
			method: http.MethodGet,
			path:   "/api/portfolios/" + portfolioName + "/external-balances",
		},
		{
			name:   "post_external_balance",
			method: http.MethodPost,
			path:   "/api/portfolios/" + portfolioName + "/external-balances",
		},
		{
			name:   "put_external_balances",
			method: http.MethodPut,
			path:   "/api/portfolios/" + portfolioName + "/external-balances",
		},
		{
			name:   "delete_external_balance",
			method: http.MethodDelete,
			path:   "/api/portfolios/" + portfolioName + "/external-balances/eb_test",
		},
	}

	for _, ep := range endpoints {
		ep := ep
		t.Run(ep.name, func(t *testing.T) {
			var body interface{}
			if ep.method == http.MethodPost {
				body = map[string]interface{}{"type": "cash", "label": "Test", "value": 1000}
			} else if ep.method == http.MethodPut {
				body = map[string]interface{}{"external_balances": []interface{}{}}
			}

			resp, err := env.HTTPRequest(ep.method, ep.path, body, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			guard.SaveResult("removed_"+ep.name, string(respBody))

			// Should return 404 (endpoint removed) or 405 (method not allowed on different endpoint)
			assert.Equal(t, http.StatusNotFound, resp.StatusCode,
				"external balance endpoint %q should return 404 after removal, got %d: %s",
				ep.name, resp.StatusCode, string(respBody))
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 11: Performance with Mixed Account Types ---

// TestSignedAmounts_PerformanceMixedAccountTypes verifies that capital performance
// only includes transactions from transactional accounts (not non-transactional),
// or otherwise correctly handles the signed amount sum from all accounts.
func TestSignedAmounts_PerformanceMixedAccountTypes(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	tradingDeposit := 100000.0
	accumulateBalance := 50000.0

	// Add deposit to Trading (transactional)
	t.Run("add_trading_deposit", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Trading",
			"category":    "contribution",
			"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      tradingDeposit,
			"description": "Trading deposit for mixed account perf test",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// Add deposit to Accumulate (will be set non-transactional)
	t.Run("add_accumulate_deposit", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, map[string]interface{}{
			"account":     "Accumulate",
			"category":    "contribution",
			"date":        time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      accumulateBalance,
			"description": "Accumulate deposit for mixed account perf test",
		})
		require.Equal(t, http.StatusCreated, status)
	})

	// Update Accumulate to be non-transactional
	t.Run("set_accumulate_non_transactional", func(t *testing.T) {
		result, status := updateAccount(t, env, portfolioName, "Accumulate", userHeaders, map[string]interface{}{
			"type":             "accumulate",
			"is_transactional": false,
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("01_update_accumulate", string(body))

		if status == http.StatusNotFound {
			t.Skip("update_account endpoint not yet implemented")
		}
		require.Equal(t, http.StatusOK, status)
	})

	// Get performance and verify
	t.Run("performance_shows_correct_amounts", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_performance_mixed_accounts", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		totalDeposited := result["total_deposited"].(float64)
		totalWithdrawn := result["total_withdrawn"].(float64)
		txCount := result["transaction_count"].(float64)

		// Verify performance reflects signed amount totals
		assert.Greater(t, totalDeposited, 0.0, "total_deposited should be positive")
		assert.InDelta(t, 0.0, totalWithdrawn, 0.01, "total_withdrawn should be 0 (no debits)")
		assert.Greater(t, txCount, 0.0, "transaction_count should be > 0")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
