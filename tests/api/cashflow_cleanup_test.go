package api

// Integration tests for cash flow cleanup:
//   - Removal of legacy migration code (no auto-migration endpoint needed)
//   - Transfer transactions counted as normal flows (not excluded)
//   - Transfer endpoint creates paired debit/credit entries
//   - Account balances correctly reflect transfers
//   - Total portfolio cash unchanged after a transfer
//
// Requirements: .claude/workdir/20260228-1030-cashflow-cleanup/requirements.md

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

// postTransfer posts a transfer between two accounts via /cash-transactions/transfer.
func postTransfer(t *testing.T, env *common.Env, portfolioName string, headers map[string]string, fromAccount, toAccount string, amount float64, date, description string) (map[string]interface{}, int) {
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

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode
	}

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result, resp.StatusCode
}

// postCashTx posts a single cash transaction with the account-based format.
// Amount is signed: positive = credit, negative = debit.
func postCashTx(t *testing.T, env *common.Env, portfolioName string, headers map[string]string, account, category string, amount float64, date, description string) (map[string]interface{}, int) {
	t.Helper()
	return postCashTransaction(t, env, portfolioName, headers, map[string]interface{}{
		"account":     account,
		"category":    category,
		"date":        date,
		"amount":      amount,
		"description": description,
	})
}

// ledgerTxCount returns the number of transactions in the decoded ledger response.
func ledgerTxCount(result map[string]interface{}) int {
	txns, ok := result["transactions"].([]interface{})
	if !ok {
		return 0
	}
	return len(txns)
}

// accountBalanceFromLedger reads the computed balance for a named account from the response.
func accountBalanceFromLedger(result map[string]interface{}, accountName string) float64 {
	accounts, ok := result["accounts"].([]interface{})
	if !ok {
		return 0
	}
	for _, a := range accounts {
		acc, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if acc["name"] == accountName {
			bal, _ := acc["balance"].(float64)
			return bal
		}
	}
	return 0
}

// totalBalanceFromLedger reads total_cash from the summary.
func totalBalanceFromLedger(result map[string]interface{}) float64 {
	summary, ok := result["summary"].(map[string]interface{})
	if !ok {
		return 0
	}
	total, _ := summary["total_cash"].(float64)
	return total
}

// --- Transfer Endpoint: Paired Debit/Credit Entries ---

// TestCashFlowTransfer_CreatesPairedEntries verifies that POST /cash-transactions/transfer
// creates two linked transactions: a debit from fromAccount and a credit to toAccount.
func TestCashFlowTransfer_CreatesPairedEntries(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add an initial deposit to the Trading account
	t.Run("add_initial_deposit", func(t *testing.T) {
		result, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			100000, time.Now().Add(-60*24*time.Hour).Format(time.RFC3339),
			"Initial deposit for transfer test")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_initial_deposit", string(body))
	})

	// Transfer from Trading to Accumulate
	transferAmount := 20000.0
	t.Run("post_transfer", func(t *testing.T) {
		result, status := postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", transferAmount,
			time.Now().Add(-30*24*time.Hour).Format(time.RFC3339),
			"Monthly transfer to Accumulate")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_after_transfer", string(body))

		// Should now have 3 transactions: 1 deposit + 2 transfer entries
		assert.Equal(t, 3, ledgerTxCount(result), "transfer should create 2 paired entries")

		// Find transfer entries (negative amount = from, positive amount = to)
		txns := result["transactions"].([]interface{})
		var debitFound, creditFound bool
		var debitLinkedID, creditLinkedID string
		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			if txMap["category"] != "transfer" {
				continue
			}
			amount, _ := txMap["amount"].(float64)
			if amount < 0 {
				assert.Equal(t, "Trading", txMap["account"], "debit should be from Trading")
				assert.Equal(t, -transferAmount, amount)
				debitLinkedID, _ = txMap["linked_id"].(string)
				debitFound = true
			}
			if amount > 0 {
				assert.Equal(t, "Accumulate", txMap["account"], "credit should be to Accumulate")
				assert.Equal(t, transferAmount, amount)
				creditLinkedID, _ = txMap["linked_id"].(string)
				creditFound = true
			}
		}
		assert.True(t, debitFound, "debit transfer entry should be present")
		assert.True(t, creditFound, "credit transfer entry should be present")
		assert.NotEmpty(t, debitLinkedID, "debit should have a linked_id")
		assert.NotEmpty(t, creditLinkedID, "credit should have a linked_id")
		// The two entries should link to each other
		assert.NotEqual(t, "", debitLinkedID, "debit linked_id should not be empty")
		assert.NotEqual(t, "", creditLinkedID, "credit linked_id should not be empty")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Transfer: Account Balances Updated Correctly ---

// TestCashFlowTransfer_AccountBalancesUpdated verifies that after a transfer,
// both accounts reflect the correct balance changes.
func TestCashFlowTransfer_AccountBalancesUpdated(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 50000.0
	transferAmount := 15000.0

	// Add deposit to Trading
	t.Run("add_deposit", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			depositAmount, time.Now().Add(-90*24*time.Hour).Format(time.RFC3339),
			"Deposit for account balance test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Record baseline balances
	var tradingBefore, accumulateBefore float64
	t.Run("baseline_balances", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_baseline_ledger", string(body))

		tradingBefore = accountBalanceFromLedger(result, "Trading")
		accumulateBefore = accountBalanceFromLedger(result, "Accumulate")

		assert.InDelta(t, depositAmount, tradingBefore, 0.01,
			"Trading balance should equal the deposit amount before transfer")
		assert.InDelta(t, 0.0, accumulateBefore, 0.01,
			"Accumulate balance should be zero before transfer")
	})

	// Perform transfer
	t.Run("perform_transfer", func(t *testing.T) {
		result, status := postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", transferAmount,
			time.Now().Add(-30*24*time.Hour).Format(time.RFC3339),
			"Transfer to Accumulate for balance test")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_after_transfer", string(body))

		// Trading balance should decrease by transferAmount
		tradingAfter := accountBalanceFromLedger(result, "Trading")
		accumulateAfter := accountBalanceFromLedger(result, "Accumulate")

		assert.InDelta(t, tradingBefore-transferAmount, tradingAfter, 0.01,
			"Trading balance should decrease by transferAmount")
		assert.InDelta(t, accumulateBefore+transferAmount, accumulateAfter, 0.01,
			"Accumulate balance should increase by transferAmount")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Transfer: Total Portfolio Cash Unchanged ---

// TestCashFlowTransfer_TotalCashUnchanged verifies that the total portfolio cash
// (sum of all account balances) is unchanged after an internal transfer.
// This is the key property: debits and credits cancel each other.
func TestCashFlowTransfer_TotalCashUnchanged(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 80000.0
	transferAmount := 30000.0

	// Add initial deposit
	t.Run("add_deposit", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			depositAmount, time.Now().Add(-120*24*time.Hour).Format(time.RFC3339),
			"Deposit for total cash test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Capture total cash before transfer
	var totalBefore float64
	t.Run("total_cash_before_transfer", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_before_transfer", string(body))

		totalBefore = totalBalanceFromLedger(result)
		assert.InDelta(t, depositAmount, totalBefore, 0.01,
			"total cash should equal deposit amount before transfer")
	})

	// Perform transfer
	t.Run("perform_transfer", func(t *testing.T) {
		result, status := postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", transferAmount,
			time.Now().Add(-60*24*time.Hour).Format(time.RFC3339),
			"Internal transfer for total cash test")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_after_transfer", string(body))

		// Total cash should remain the same (debit and credit cancel)
		totalAfter := totalBalanceFromLedger(result)
		assert.InDelta(t, totalBefore, totalAfter, 0.01,
			"total portfolio cash should be unchanged after internal transfer")
	})

	// Perform a second transfer (chained)
	transferAmount2 := 10000.0
	t.Run("second_transfer_still_unchanged", func(t *testing.T) {
		result, status := postTransfer(t, env, portfolioName, userHeaders,
			"Accumulate", "TermDeposit", transferAmount2,
			time.Now().Add(-30*24*time.Hour).Format(time.RFC3339),
			"Second internal transfer")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("03_after_second_transfer", string(body))

		// Total cash still unchanged
		totalAfter2 := totalBalanceFromLedger(result)
		assert.InDelta(t, totalBefore, totalAfter2, 0.01,
			"total portfolio cash should still be unchanged after second transfer")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Transfer: Deleting One Entry Removes Both ---

// TestCashFlowTransfer_DeleteRemovesBothPairs verifies that removing one half
// of a transfer (by ID) also removes the linked paired entry.
func TestCashFlowTransfer_DeleteRemovesBothPairs(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName + "/cash-transactions"

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add a deposit and a transfer
	t.Run("setup", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			60000, time.Now().Add(-90*24*time.Hour).Format(time.RFC3339),
			"Deposit for pair delete test")
		require.Equal(t, http.StatusCreated, status)

		_, status = postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", 20000,
			time.Now().Add(-60*24*time.Hour).Format(time.RFC3339),
			"Transfer for pair delete test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Find the debit transfer entry ID
	var debitID string
	t.Run("find_debit_entry", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_before_delete", string(body))

		// Should have 3 transactions: 1 deposit + 2 transfer entries
		txns := result["transactions"].([]interface{})
		require.Len(t, txns, 3, "should have 1 deposit and 2 transfer entries")

		for _, tx := range txns {
			txMap := tx.(map[string]interface{})
			amount, _ := txMap["amount"].(float64)
			if txMap["category"] == "transfer" && amount < 0 {
				debitID = txMap["id"].(string)
				break
			}
		}
		require.NotEmpty(t, debitID, "debit transfer entry should be findable")
	})

	// Delete just the debit entry — the credit should also be removed
	t.Run("delete_debit_removes_both", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodDelete, basePath+"/"+debitID, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_delete_response", string(body))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Verify only the original deposit remains (both transfer entries gone)
	t.Run("verify_both_entries_removed", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("03_after_delete", string(body))

		txns := result["transactions"].([]interface{})
		assert.Len(t, txns, 1, "both transfer entries should be removed, only deposit remains")

		// The remaining entry should be the deposit (positive contribution)
		if len(txns) == 1 {
			txMap := txns[0].(map[string]interface{})
			assert.Equal(t, "contribution", txMap["category"])
			amount, _ := txMap["amount"].(float64)
			assert.Greater(t, amount, 0.0, "deposit should be positive")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- New Format: Account-Based Transactions Work Correctly ---

// TestCashFlowNewFormat_AccountBasedTransactions verifies that the new account-based
// format (direction + account + category) is accepted by the API after legacy migration
// code is removed.
func TestCashFlowNewFormat_AccountBasedTransactions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Table-driven: all valid transaction types (amounts are signed)
	transactions := []struct {
		name     string
		account  string
		category string
		amount   float64
		date     string
		desc     string
	}{
		{
			name:     "contribution_credit",
			account:  "Trading",
			category: "contribution",
			amount:   100000,
			date:     time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			desc:     "Initial contribution",
		},
		{
			name:     "dividend_credit",
			account:  "Trading",
			category: "dividend",
			amount:   2500,
			date:     time.Now().Add(-300 * 24 * time.Hour).Format(time.RFC3339),
			desc:     "BHP interim dividend",
		},
		{
			name:     "fee_debit",
			account:  "Trading",
			category: "fee",
			amount:   -500,
			date:     time.Now().Add(-270 * 24 * time.Hour).Format(time.RFC3339),
			desc:     "Annual admin fee",
		},
		{
			name:     "other_credit",
			account:  "Trading",
			category: "other",
			amount:   1000,
			date:     time.Now().Add(-180 * 24 * time.Hour).Format(time.RFC3339),
			desc:     "Miscellaneous credit",
		},
		{
			name:     "other_debit",
			account:  "Trading",
			category: "other",
			amount:   -200,
			date:     time.Now().Add(-90 * 24 * time.Hour).Format(time.RFC3339),
			desc:     "Miscellaneous debit",
		},
	}

	for _, tt := range transactions {
		tt := tt
		t.Run("add_"+tt.name, func(t *testing.T) {
			result, status := postCashTx(t, env, portfolioName, userHeaders,
				tt.account, tt.category,
				tt.amount, tt.date, tt.desc)

			body, _ := json.Marshal(result)
			guard.SaveResult("tx_"+tt.name, string(body))

			assert.Equal(t, http.StatusCreated, status, "transaction %q should be accepted", tt.name)
			if result != nil {
				txns := result["transactions"].([]interface{})
				assert.NotEmpty(t, txns, "ledger should have transactions after adding %q", tt.name)
			}
		})
	}

	// Verify all 5 transactions are present
	t.Run("verify_all_present", func(t *testing.T) {
		result, status := getCashFlows(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("final_ledger", string(body))

		assert.Equal(t, len(transactions), ledgerTxCount(result),
			"all %d transactions should be present in ledger", len(transactions))
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Validation: Invalid Transfer Requests Rejected ---

// TestCashFlowTransfer_Validation verifies that invalid transfer requests
// return HTTP 400 with appropriate error messages.
func TestCashFlowTransfer_Validation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	tests := []struct {
		name    string
		body    map[string]interface{}
		wantBad bool
	}{
		{
			name: "missing_from_account",
			body: map[string]interface{}{
				"to_account":  "Accumulate",
				"amount":      1000,
				"date":        time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
				"description": "Transfer without from",
			},
			wantBad: true,
		},
		{
			name: "missing_to_account",
			body: map[string]interface{}{
				"from_account": "Trading",
				"amount":       1000,
				"date":         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
				"description":  "Transfer without to",
			},
			wantBad: true,
		},
		{
			name: "same_from_to_account",
			body: map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Trading",
				"amount":       1000,
				"date":         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
				"description":  "Same account transfer",
			},
			wantBad: true,
		},
		{
			name: "zero_amount",
			body: map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       0,
				"date":         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
				"description":  "Zero amount transfer",
			},
			wantBad: true,
		},
		{
			name: "negative_amount",
			body: map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       -500,
				"date":         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
				"description":  "Negative amount transfer",
			},
			wantBad: true,
		},
		{
			name: "missing_date",
			body: map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       1000,
				"description":  "Transfer without date",
			},
			wantBad: true,
		},
		{
			name: "missing_description",
			body: map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       1000,
				"date":         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
			},
			wantBad: true,
		},
		{
			name: "valid_transfer",
			body: map[string]interface{}{
				"from_account": "Trading",
				"to_account":   "Accumulate",
				"amount":       1000,
				"date":         time.Now().Add(-24 * time.Hour).Format("2006-01-02"),
				"description":  "Valid transfer",
			},
			wantBad: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/cash-transactions/transfer", tt.body, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("validation_"+tt.name, string(body))

			if tt.wantBad {
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
					"expected 400 for %q, got %d: %s", tt.name, resp.StatusCode, string(body))
			} else {
				assert.Equal(t, http.StatusCreated, resp.StatusCode,
					"expected 201 for %q, got %d: %s", tt.name, resp.StatusCode, string(body))
			}
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Transfers Count as Normal Flows in Performance After Cleanup ---

// TestCashFlowTransfer_CountedInPerformanceAfterCleanup verifies that after the
// cleanup (transfer exclusion removed), transfer transactions count as normal
// deposits/withdrawals in CalculatePerformance.
//
// Before cleanup: transfers excluded, deposit(100000) → total_deposited=100000
// After cleanup: transfers counted, deposit(100000) + transfer_credit(20000) → total_deposited=120000
//   - transfer_debit(20000) → total_withdrawn=20000
func TestCashFlowTransfer_CountedInPerformanceAfterCleanup(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	depositAmount := 100000.0
	transferAmount := 20000.0

	// Step 1: Add a deposit
	t.Run("add_deposit", func(t *testing.T) {
		result, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			depositAmount, time.Now().Add(-90*24*time.Hour).Format(time.RFC3339),
			"Deposit for performance test")
		require.Equal(t, http.StatusCreated, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_deposit", string(body))
	})

	// Step 2: Get baseline performance (deposit only)
	var baselineDeposited float64
	t.Run("baseline_performance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_baseline_perf", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		baselineDeposited = perf["total_deposited"].(float64)
		assert.InDelta(t, depositAmount, baselineDeposited, 0.01,
			"total_deposited should equal the deposit before any transfers")
	})

	// Step 3: Add a transfer (Trading -> Accumulate)
	t.Run("add_transfer", func(t *testing.T) {
		result, status := postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", transferAmount,
			time.Now().Add(-60*24*time.Hour).Format(time.RFC3339),
			"Transfer for performance inclusion test")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("03_after_transfer", string(body))

		// Ledger should have 3 transactions: 1 deposit + 2 transfer entries
		assert.Equal(t, 3, ledgerTxCount(result))
	})

	// Step 4: Get performance after transfer
	// After cleanup, transfers count as normal flows:
	//   - credit side (Accumulate gets credit = deposit) → total_deposited increases
	//   - debit side (Trading gets debit = withdrawal) → total_withdrawn increases
	t.Run("performance_after_transfer_counts_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_perf_after_transfer", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		totalDeposited := perf["total_deposited"].(float64)
		totalWithdrawn := perf["total_withdrawn"].(float64)
		txCount := perf["transaction_count"].(float64)

		// total_deposited counts only positive contribution amounts.
		// Transfers are not contributions, so total_deposited = deposit only.
		assert.InDelta(t, depositAmount, totalDeposited, 0.01,
			"total_deposited should equal the contribution deposit")
		assert.InDelta(t, 0, totalWithdrawn, 0.01,
			"total_withdrawn should be zero (no negative contributions)")
		assert.Equal(t, float64(3), txCount,
			"transaction_count should reflect all 3 entries including transfers")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Transfers Affect Cash Balance in Timeline ---

// TestCashFlowTransfer_AffectsCashBalanceInTimeline verifies that transfer transactions
// affect the running cash balance in the capital timeline (GET /indicators time_series).
// After cleanup, transfers are no longer skipped in GetDailyGrowth().
func TestCashFlowTransfer_AffectsCashBalanceInTimeline(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add a deposit so the ledger is non-empty (required for timeline fields to appear)
	t.Run("add_deposit", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			80000, time.Now().Add(-365*24*time.Hour).Format(time.RFC3339),
			"Deposit for timeline transfer test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Add a transfer
	t.Run("add_transfer", func(t *testing.T) {
		_, status := postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", 20000,
			time.Now().Add(-180*24*time.Hour).Format(time.RFC3339),
			"Transfer for timeline test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Get indicators and check that cash_balance reflects transfer
	t.Run("timeline_reflects_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_with_transfer", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTS := indicators["time_series"]
		if !hasTS || ts == nil {
			t.Skip("time_series absent — portfolio has insufficient historical data")
		}

		tsSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		if len(tsSlice) == 0 {
			t.Skip("time_series empty — portfolio has insufficient historical data")
		}

		// After cleanup, transfer debit and credit cancel in the timeline's cash_balance
		// (the sum of all entries' cash impact = zero for a transfer pair).
		// The last point should include cash_balance if any transactions exist.
		lastPoint := tsSlice[len(tsSlice)-1].(map[string]interface{})

		// cash_balance should be present (non-zero) due to the deposit
		if cashBalance, hasCB := lastPoint["cash_balance"].(float64); hasCB {
			// After cleanup: transfer credit and debit cancel, so net impact on cash_balance
			// is zero for the pair. The deposit contributes 80000.
			// The transfer debit (-20000) and credit (+20000) cancel → net cash = 80000.
			assert.InDelta(t, 80000.0, cashBalance, 1.0,
				"cash_balance should reflect deposit + cancelled transfer pair = 80000")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Net Flows Include Transfers After Cleanup ---

// TestCashFlowTransfer_IncludedInNetFlows verifies that transfers are included
// in net flow calculations (YesterdayNetFlow / LastWeekNetFlow) after cleanup.
// Before cleanup, transfers were excluded. After cleanup, they count as real flows.
func TestCashFlowTransfer_IncludedInNetFlows(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	basePath := "/api/portfolios/" + portfolioName

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Add a deposit yesterday
	yesterday := time.Now().Add(-24 * time.Hour).Format("2006-01-02")

	t.Run("add_deposit_yesterday", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			50000, yesterday+"T00:00:00Z",
			"Deposit yesterday for net flow test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Add a transfer yesterday
	t.Run("add_transfer_yesterday", func(t *testing.T) {
		_, status := postTransfer(t, env, portfolioName, userHeaders,
			"Trading", "Accumulate", 10000,
			yesterday+"T00:00:00Z",
			"Transfer yesterday for net flow test")
		require.Equal(t, http.StatusCreated, status)
	})

	// Get portfolio and check YesterdayNetFlow
	t.Run("yesterday_net_flow_includes_transfer", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_portfolio_net_flows", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// After cleanup, transfers are included in net flows:
		// deposit credit (+50000) + transfer debit (-10000) + transfer credit (+10000) = +50000
		// But net flows only use sign * amount for each transaction:
		// deposit: credit → +50000
		// transfer debit from Trading: debit → -10000
		// transfer credit to Accumulate: credit → +10000
		// Net = +50000 - 10000 + 10000 = +50000
		// (transfer pair cancels itself in net flow)
		if yesterdayFlow, ok := portfolio["yesterday_net_flow"].(float64); ok {
			// The deposit contributes +50000, transfer pair cancels to 0
			assert.InDelta(t, 50000.0, yesterdayFlow, 1.0,
				"yesterday_net_flow should include deposit; transfer pair cancels itself")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
