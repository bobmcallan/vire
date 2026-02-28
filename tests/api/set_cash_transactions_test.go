package api

// Integration tests for the set_cash_transactions endpoint (PUT /api/portfolios/{name}/cash-transactions).
// Requirements: .claude/workdir/20260228-1100-signed-amounts/requirements.md
//
// Scenarios:
//  1. PUT with full transaction set replaces all existing transactions
//  2. PUT with empty items clears all transactions, accounts preserved
//  3. PUT with invalid transaction (missing account) returns 400
//  4. PUT with zero amount returns 400
//  5. PUT preserves existing accounts, auto-creates new account names
//  6. PUT with notes sets ledger notes
//  7. PUT assigns new IDs (overwrites any user-provided ID)
//  8. PUT sorts transactions by date ascending

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

// putSetTransactions sends a PUT to /api/portfolios/{name}/cash-transactions.
// items is the transaction slice (nil = send empty array). Returns decoded ledger and status.
func putSetTransactions(t *testing.T, env *common.Env, portfolioName string, headers map[string]string, items []map[string]interface{}, notes string) (map[string]interface{}, int) {
	t.Helper()

	body := map[string]interface{}{
		"items": items,
		"notes": notes,
	}
	if items == nil {
		body["items"] = []interface{}{}
	}

	resp, err := env.HTTPRequest(http.MethodPut, "/api/portfolios/"+portfolioName+"/cash-transactions", body, headers)
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

// txItem builds a transaction map for use in putSetTransactions.
func txItem(account, category, date, description string, amount float64) map[string]interface{} {
	return map[string]interface{}{
		"account":     account,
		"category":    category,
		"date":        date,
		"amount":      amount,
		"description": description,
	}
}

// ledgerTransactions extracts the transactions slice from a ledger response.
func ledgerTransactions(result map[string]interface{}) []map[string]interface{} {
	raw, ok := result["transactions"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, tx := range raw {
		if m, ok := tx.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// ledgerAccounts extracts the accounts slice from a ledger response.
func ledgerAccounts(result map[string]interface{}) []map[string]interface{} {
	raw, ok := result["accounts"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, a := range raw {
		if m, ok := a.(map[string]interface{}); ok {
			out = append(out, m)
		}
	}
	return out
}

// hasAccount returns true if the named account appears in the ledger accounts list.
func hasAccount(result map[string]interface{}, name string) bool {
	for _, acc := range ledgerAccounts(result) {
		if acc["name"] == name {
			return true
		}
	}
	return false
}

// --- Test 1: Full Replace ---

// TestSetCashTransactions_ReplacesAllExisting verifies that a PUT with a new
// transaction set removes all previous transactions and stores only the new ones.
func TestSetCashTransactions_ReplacesAllExisting(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Seed two transactions via POST
	t.Run("seed_initial_transactions", func(t *testing.T) {
		for _, item := range []map[string]interface{}{
			txItem("Trading", "contribution", "2025-01-01T00:00:00Z", "Seed deposit one", 50000),
			txItem("Trading", "contribution", "2025-02-01T00:00:00Z", "Seed deposit two", 25000),
		} {
			_, status := postSignedTransaction(t, env, portfolioName, userHeaders, item)
			require.Equal(t, http.StatusCreated, status)
		}
	})

	// PUT with a completely different single transaction
	newItems := []map[string]interface{}{
		txItem("Trading", "dividend", "2025-06-01", "Dividend income", 1200.50),
	}

	t.Run("put_replaces_all", func(t *testing.T) {
		result, status := putSetTransactions(t, env, portfolioName, userHeaders, newItems, "")

		body, _ := json.Marshal(result)
		guard.SaveResult("01_after_put_replace", string(body))

		assert.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)

		txns := ledgerTransactions(result)
		assert.Len(t, txns, 1, "PUT should replace all: only 1 transaction expected")

		tx := txns[0]
		assert.Equal(t, "Dividend income", tx["description"])
		assert.InDelta(t, 1200.50, tx["amount"].(float64), 0.01)
		assert.Equal(t, "Trading", tx["account"])
		assert.Equal(t, "dividend", tx["category"])
	})

	// Verify via GET
	t.Run("get_confirms_replacement", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_get_after_put", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		txns := ledgerTransactions(result)
		assert.Len(t, txns, 1)
		if len(txns) == 1 {
			assert.Equal(t, "Dividend income", txns[0]["description"])
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 2: Empty Items Clears Transactions ---

// TestSetCashTransactions_EmptyItemsClearsTransactions verifies that PUT with an
// empty items array removes all transactions but preserves existing accounts.
func TestSetCashTransactions_EmptyItemsClearsTransactions(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Seed a transaction to create the Trading account
	t.Run("seed_transaction", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, txItem(
			"Trading", "contribution", "2025-01-01T00:00:00Z", "Deposit to clear", 30000,
		))
		require.Equal(t, http.StatusCreated, status)
	})

	// PUT with empty items
	t.Run("put_empty_clears_transactions", func(t *testing.T) {
		result, status := putSetTransactions(t, env, portfolioName, userHeaders, nil, "")

		body, _ := json.Marshal(result)
		guard.SaveResult("01_after_empty_put", string(body))

		assert.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)

		txns := ledgerTransactions(result)
		assert.Len(t, txns, 0, "empty PUT should result in zero transactions")

		// Trading account should be preserved
		assert.True(t, hasAccount(result, "Trading"),
			"Trading account should be preserved after empty PUT")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 3: Validation — Missing Account ---

// TestSetCashTransactions_ValidationMissingAccount verifies that PUT returns HTTP 400
// when a transaction is missing the required account field.
func TestSetCashTransactions_ValidationMissingAccount(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	invalidItems := []map[string]interface{}{
		{
			"account":     "", // empty — invalid
			"category":    "contribution",
			"date":        "2025-01-01",
			"amount":      1000.0,
			"description": "Should be rejected",
		},
	}

	resp, err := env.HTTPRequest(http.MethodPut, "/api/portfolios/"+portfolioName+"/cash-transactions", map[string]interface{}{
		"items": invalidItems,
		"notes": "",
	}, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_missing_account_400", string(body))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"empty account should return 400, got: %s", string(body))

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 4: Validation — Zero Amount ---

// TestSetCashTransactions_ValidationZeroAmount verifies that PUT returns HTTP 400
// when a transaction has amount = 0.
func TestSetCashTransactions_ValidationZeroAmount(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	invalidItems := []map[string]interface{}{
		{
			"account":     "Trading",
			"category":    "contribution",
			"date":        "2025-01-01",
			"amount":      0.0, // zero — invalid
			"description": "Zero amount should be rejected",
		},
	}

	resp, err := env.HTTPRequest(http.MethodPut, "/api/portfolios/"+portfolioName+"/cash-transactions", map[string]interface{}{
		"items": invalidItems,
		"notes": "",
	}, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_zero_amount_400", string(body))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"zero amount should return 400, got: %s", string(body))

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 5: Preserves Existing Accounts, Auto-Creates New ---

// TestSetCashTransactions_AccountPreservationAndAutoCreate verifies:
// - Accounts from prior transactions remain in the ledger after PUT
// - New account names referenced by PUT items are auto-created
func TestSetCashTransactions_AccountPreservationAndAutoCreate(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Seed a transaction on "OldAccount"
	t.Run("seed_old_account", func(t *testing.T) {
		_, status := postSignedTransaction(t, env, portfolioName, userHeaders, txItem(
			"OldAccount", "contribution", "2025-01-01T00:00:00Z", "Old account seed", 10000,
		))
		require.Equal(t, http.StatusCreated, status)
	})

	// PUT referencing a brand new account
	t.Run("put_with_new_account", func(t *testing.T) {
		result, status := putSetTransactions(t, env, portfolioName, userHeaders, []map[string]interface{}{
			txItem("BrandNewAccount", "contribution", "2025-03-01", "New account deposit", 20000),
		}, "")

		body, _ := json.Marshal(result)
		guard.SaveResult("01_after_put_accounts", string(body))

		assert.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)

		// OldAccount preserved (even though no transactions reference it)
		assert.True(t, hasAccount(result, "OldAccount"),
			"OldAccount should be preserved after PUT")

		// BrandNewAccount auto-created
		assert.True(t, hasAccount(result, "BrandNewAccount"),
			"BrandNewAccount should be auto-created by PUT")

		// Only one transaction (the new one)
		txns := ledgerTransactions(result)
		assert.Len(t, txns, 1)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 6: Notes Field ---

// TestSetCashTransactions_NotesUpdated verifies that the notes field in the PUT body
// is stored in the ledger and returned in the response.
func TestSetCashTransactions_NotesUpdated(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	notesText := "Imported from CSV export 2025-06-15"

	result, status := putSetTransactions(t, env, portfolioName, userHeaders, []map[string]interface{}{
		txItem("Trading", "contribution", "2025-01-01", "Deposit with notes", 50000),
	}, notesText)

	body, _ := json.Marshal(result)
	guard.SaveResult("01_put_with_notes", string(body))

	assert.Equal(t, http.StatusOK, status)
	require.NotNil(t, result)

	notes, _ := result["notes"].(string)
	assert.Equal(t, notesText, notes,
		"ledger notes should match what was provided in PUT body")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 7: IDs Are Assigned (User-Provided IDs Overwritten) ---

// TestSetCashTransactions_AssignsNewIDs verifies that SetTransactions always
// assigns fresh ct_* prefixed IDs, ignoring any id provided in the request.
func TestSetCashTransactions_AssignsNewIDs(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	items := []map[string]interface{}{
		{
			"id":          "user-provided-id", // should be ignored
			"account":     "Trading",
			"category":    "contribution",
			"date":        "2025-01-01",
			"amount":      10000.0,
			"description": "Deposit with user ID",
		},
		txItem("Trading", "fee", "2025-01-02", "Management fee", -250),
	}

	result, status := putSetTransactions(t, env, portfolioName, userHeaders, items, "")

	body, _ := json.Marshal(result)
	guard.SaveResult("01_put_id_assignment", string(body))

	assert.Equal(t, http.StatusOK, status)
	require.NotNil(t, result)

	txns := ledgerTransactions(result)
	require.Len(t, txns, 2)

	ids := make(map[string]bool)
	for _, tx := range txns {
		id, _ := tx["id"].(string)
		assert.NotEmpty(t, id, "every transaction should have an ID")
		assert.NotEqual(t, "user-provided-id", id,
			"user-provided IDs should be overwritten by server-assigned IDs")
		// IDs use ct_ prefix per generateCashTransactionID convention
		assert.True(t, len(id) > 3, "ID should be non-trivial")
		ids[id] = true
	}
	assert.Len(t, ids, 2, "all assigned IDs should be unique")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 8: Sorted By Date ---

// TestSetCashTransactions_SortsByDate verifies that PUT returns transactions
// sorted by date ascending regardless of the order they were submitted.
func TestSetCashTransactions_SortsByDate(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Submit in reverse-chronological order
	items := []map[string]interface{}{
		txItem("Trading", "contribution", "2025-12-01", "December deposit", 3000),
		txItem("Trading", "contribution", "2025-06-01", "June deposit", 2000),
		txItem("Trading", "contribution", "2025-01-01", "January deposit", 1000),
	}

	result, status := putSetTransactions(t, env, portfolioName, userHeaders, items, "")

	body, _ := json.Marshal(result)
	guard.SaveResult("01_put_sorted_by_date", string(body))

	assert.Equal(t, http.StatusOK, status)
	require.NotNil(t, result)

	txns := ledgerTransactions(result)
	require.Len(t, txns, 3)

	// Verify ascending date order
	for i := 1; i < len(txns); i++ {
		dateA, _ := time.Parse(time.RFC3339, txns[i-1]["date"].(string))
		dateB, _ := time.Parse(time.RFC3339, txns[i]["date"].(string))
		assert.False(t, dateB.Before(dateA),
			"transaction[%d] date %v should not be before transaction[%d] date %v",
			i, dateB, i-1, dateA)
	}

	// First should be January
	assert.Equal(t, "January deposit", txns[0]["description"],
		"first transaction should be the earliest date (January)")

	// Last should be December
	assert.Equal(t, "December deposit", txns[2]["description"],
		"last transaction should be the latest date (December)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 9: Mixed Signs in Bulk Set ---

// TestSetCashTransactions_MixedSigns verifies that PUT handles a mix of positive
// (credit) and negative (debit) amounts and that performance reflects signed totals.
func TestSetCashTransactions_MixedSigns(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	credits := []float64{100000, 25000, 5000} // total = 130000
	debits := []float64{-10000, -3000}        // total withdrawn = 13000

	items := []map[string]interface{}{
		txItem("Trading", "contribution", "2024-01-01", "Initial deposit", credits[0]),
		txItem("Trading", "contribution", "2024-06-01", "Top-up", credits[1]),
		txItem("Trading", "dividend", "2024-09-01", "Dividend", credits[2]),
		txItem("Trading", "other", "2024-03-01", "Partial withdrawal", debits[0]),
		txItem("Trading", "fee", "2024-07-01", "Admin fee", debits[1]),
	}

	t.Run("put_mixed_signs", func(t *testing.T) {
		result, status := putSetTransactions(t, env, portfolioName, userHeaders, items, "")

		body, _ := json.Marshal(result)
		guard.SaveResult("01_after_mixed_put", string(body))

		assert.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)

		txns := ledgerTransactions(result)
		assert.Len(t, txns, len(items))

		// Count positives and negatives
		var positiveCount, negativeCount int
		for _, tx := range txns {
			amount := tx["amount"].(float64)
			if amount > 0 {
				positiveCount++
			} else if amount < 0 {
				negativeCount++
			}
		}
		assert.Equal(t, 3, positiveCount, "should have 3 credit entries")
		assert.Equal(t, 2, negativeCount, "should have 2 debit entries")
	})

	t.Run("performance_reflects_signed_totals", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_performance_after_bulk_set", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		totalDeposited := perf["total_deposited"].(float64)
		totalWithdrawn := perf["total_withdrawn"].(float64)

		expectedDeposited := 0.0
		for _, c := range credits {
			expectedDeposited += c
		}
		expectedWithdrawn := 0.0
		for _, d := range debits {
			expectedWithdrawn += -d
		}

		assert.InDelta(t, expectedDeposited, totalDeposited, 0.01,
			"total_deposited should sum all positive amounts")
		assert.InDelta(t, expectedWithdrawn, totalWithdrawn, 0.01,
			"total_withdrawn should be abs sum of negative amounts")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test 10: Idempotent — Repeated PUT with Same Items ---

// TestSetCashTransactions_IdempotentRepeatedPut verifies that calling PUT twice
// with the same items results in the same ledger state (no duplicates).
func TestSetCashTransactions_IdempotentRepeatedPut(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	items := []map[string]interface{}{
		txItem("Trading", "contribution", "2025-01-01", "Annual contribution", 75000),
		txItem("Trading", "fee", "2025-04-01", "Platform fee", -500),
	}

	// First PUT
	result1, status1 := putSetTransactions(t, env, portfolioName, userHeaders, items, "First import")

	body1, _ := json.Marshal(result1)
	guard.SaveResult("01_first_put", string(body1))

	assert.Equal(t, http.StatusOK, status1)
	require.NotNil(t, result1)
	assert.Len(t, ledgerTransactions(result1), 2)

	// Second PUT with same items
	result2, status2 := putSetTransactions(t, env, portfolioName, userHeaders, items, "Second import")

	body2, _ := json.Marshal(result2)
	guard.SaveResult("02_second_put", string(body2))

	assert.Equal(t, http.StatusOK, status2)
	require.NotNil(t, result2)

	// Still exactly 2 transactions — no accumulation
	txns2 := ledgerTransactions(result2)
	assert.Len(t, txns2, 2,
		"repeated PUT with same items should not accumulate — still 2 transactions")

	// Notes should reflect second PUT
	notes, _ := result2["notes"].(string)
	assert.Equal(t, "Second import", notes)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
