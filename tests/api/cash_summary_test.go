package api

// Integration tests for the get_cash_summary tool and currency field on CashAccount.
//
// Requirements: .claude/workdir/20260301-0425-summary-currency/requirements.md
//
// Coverage:
//   - GET /api/portfolios/{name}/cash-summary returns accounts with balances, summary totals
//   - TotalCashByCurrency breaks down balances per ISO 4217 currency
//   - update_account accepts a currency parameter; reflected in cash-summary response
//   - Empty ledger returns only the default Trading account with AUD currency
//   - cash-summary is independent of list_cash_transactions (no duplicate transaction details)
//   - Currency field on accounts persists across reads

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// getCashSummary is a test helper that GETs the cash summary endpoint
// and decodes the response body.
func getCashSummary(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) (map[string]interface{}, int) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/cash-summary", nil, headers)
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

// patchCashAccount issues a POST to /api/portfolios/{name}/cash-accounts/{account}
// with the provided update fields and returns the decoded response.
// Note: updateAccount already exists in signed_amounts_test.go (same package);
// this wrapper exists so tests in this file do not redeclare the function.
func patchCashAccount(t *testing.T, env *common.Env, portfolioName, accountName string, headers map[string]string, body map[string]interface{}) (map[string]interface{}, int) {
	t.Helper()
	return updateAccount(t, env, portfolioName, accountName, headers, body)
}

// accountCurrencyFromSummary extracts the currency field for a named account
// from a cash-summary response.
func accountCurrencyFromSummary(result map[string]interface{}, accountName string) string {
	accounts, ok := result["accounts"].([]interface{})
	if !ok {
		return ""
	}
	for _, a := range accounts {
		acc, ok := a.(map[string]interface{})
		if !ok {
			continue
		}
		if acc["name"] == accountName {
			cur, _ := acc["currency"].(string)
			return cur
		}
	}
	return ""
}

// totalCashByCurrencyFromSummary extracts the total_cash_by_currency map from a summary response.
func totalCashByCurrencyFromSummary(result map[string]interface{}) map[string]float64 {
	summary, ok := result["summary"].(map[string]interface{})
	if !ok {
		return nil
	}
	raw, ok := summary["total_cash_by_currency"].(map[string]interface{})
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(raw))
	for k, v := range raw {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out
}

// --- TestCashSummary ---

// TestCashSummary verifies GET /api/portfolios/{name}/cash-summary returns
// account balances with currency and a summary with total_cash and by_category.
func TestCashSummary(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	const deposit1 = 50000.0
	const deposit2 = 10000.0
	const fee1 = 500.0 // posted as -500

	// Step 1: Add transactions to the ledger.

	t.Run("setup_transactions", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			deposit1, "2025-01-15T00:00:00Z",
			"Summary endpoint deposit 1")
		require.Equal(t, http.StatusCreated, status)

		_, status = postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			deposit2, "2025-02-15T00:00:00Z",
			"Summary endpoint deposit 2")
		require.Equal(t, http.StatusCreated, status)

		_, status = postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "fee",
			-fee1, "2025-03-01T00:00:00Z",
			"Summary endpoint fee")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 2: GET cash-summary and verify structure.

	t.Run("summary_endpoint_returns_200", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_cash_summary", string(body))

		assert.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)
	})

	t.Run("summary_has_required_fields", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_summary_fields", string(body))

		// Top-level fields.
		assert.Contains(t, result, "portfolio_name", "response must include portfolio_name")
		assert.Contains(t, result, "accounts", "response must include accounts array")
		assert.Contains(t, result, "summary", "response must include summary object")
		assert.Equal(t, portfolioName, result["portfolio_name"])
	})

	t.Run("accounts_have_balance_and_currency", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("03_accounts_with_currency", string(body))

		accounts, ok := result["accounts"].([]interface{})
		require.True(t, ok, "accounts must be an array")
		require.NotEmpty(t, accounts, "accounts must not be empty")

		for _, rawAcct := range accounts {
			acct, ok := rawAcct.(map[string]interface{})
			require.True(t, ok, "each account must be an object")
			_, hasBalance := acct["balance"]
			assert.True(t, hasBalance, "account must have balance field: %v", acct)
			_, hasCurrency := acct["currency"]
			assert.True(t, hasCurrency, "account must have currency field: %v", acct)
		}
	})

	t.Run("trading_account_currency_defaults_to_AUD", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		currency := accountCurrencyFromSummary(result, "Trading")
		assert.Equal(t, "AUD", currency, "default Trading account must have currency AUD")
	})

	t.Run("summary_total_cash_correct", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "summary must be an object")

		expectedTotalCash := deposit1 + deposit2 - fee1 // 59500
		totalCash, _ := summary["total_cash"].(float64)
		assert.InDelta(t, expectedTotalCash, totalCash, 0.01,
			"total_cash should equal net of all signed amounts")

		txCount, _ := summary["transaction_count"].(float64)
		assert.Equal(t, float64(3), txCount, "transaction_count should reflect 3 transactions")
	})

	t.Run("summary_does_not_include_transactions_array", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		_, hasTransactions := result["transactions"]
		assert.False(t, hasTransactions, "cash-summary should NOT include transactions array (use list_cash_transactions for full ledger)")
	})

	t.Run("summary_by_category_present", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "summary must be an object")

		byCategory, ok := summary["by_category"].(map[string]interface{})
		require.True(t, ok, "summary must contain by_category")

		// All 5 categories must be present.
		for _, cat := range []string{"contribution", "dividend", "transfer", "fee", "other"} {
			_, exists := byCategory[cat]
			assert.True(t, exists, "by_category must contain key %q", cat)
		}

		contributionTotal, _ := byCategory["contribution"].(float64)
		assert.InDelta(t, deposit1+deposit2, contributionTotal, 0.01,
			"by_category.contribution should sum all contributions")

		feeTotal, _ := byCategory["fee"].(float64)
		assert.InDelta(t, -fee1, feeTotal, 0.01,
			"by_category.fee should be negative net amount")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestCashSummary_EmptyLedger ---

// TestCashSummary_EmptyLedger verifies that cash-summary for a portfolio
// with no manual transactions returns an empty accounts list with the default
// Trading account (AUD currency) and zeroed summary totals.
func TestCashSummary_EmptyLedger(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	// Ensure ledger is clean (clear any pre-existing transactions).
	cleanupCashFlows(t, env, portfolioName, userHeaders)

	result, status := getCashSummary(t, env, portfolioName, userHeaders)

	body, _ := json.Marshal(result)
	guard.SaveResult("01_empty_ledger_summary", string(body))

	require.Equal(t, http.StatusOK, status)
	require.NotNil(t, result)

	t.Run("has_default_trading_account", func(t *testing.T) {
		accounts, ok := result["accounts"].([]interface{})
		require.True(t, ok, "accounts must be an array")
		require.NotEmpty(t, accounts, "accounts must contain at least the default Trading account")

		// Find Trading account.
		var foundTrading bool
		for _, rawAcct := range accounts {
			acct, ok := rawAcct.(map[string]interface{})
			require.True(t, ok)
			if acct["name"] == "Trading" {
				foundTrading = true
				isTransactional, _ := acct["is_transactional"].(bool)
				assert.True(t, isTransactional, "default Trading account must be transactional")
				currency, _ := acct["currency"].(string)
				assert.Equal(t, "AUD", currency, "default Trading account must have currency AUD")
				balance, _ := acct["balance"].(float64)
				assert.InDelta(t, 0.0, balance, 0.01, "empty Trading account balance must be 0")
			}
		}
		assert.True(t, foundTrading, "default Trading account must be present")
	})

	t.Run("summary_totals_are_zero", func(t *testing.T) {
		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "response must contain a summary object")

		totalCash, _ := summary["total_cash"].(float64)
		assert.InDelta(t, 0.0, totalCash, 0.01, "total_cash must be 0 for empty ledger")

		txCount, _ := summary["transaction_count"].(float64)
		assert.Equal(t, float64(0), txCount, "transaction_count must be 0 for empty ledger")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestCashSummary_MultipleCurrencies ---

// TestCashSummary_MultipleCurrencies verifies that the summary.total_cash_by_currency
// map correctly accumulates balances per ISO 4217 currency code when accounts
// have different currencies set.
func TestCashSummary_MultipleCurrencies(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	const audDeposit = 100000.0
	const usdDeposit = 48000.0

	// Step 1: Add AUD deposit to Trading account (default AUD).
	t.Run("add_aud_deposit", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Trading", "contribution",
			audDeposit, "2025-01-10T00:00:00Z",
			"AUD deposit to Trading")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 2: Add USD deposit to a "Wall St" account.
	t.Run("add_usd_deposit", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Wall St", "contribution",
			usdDeposit, "2025-01-15T00:00:00Z",
			"USD deposit to Wall St")
		require.Equal(t, http.StatusCreated, status)
	})

	// Step 3: Update "Wall St" account currency to USD.
	t.Run("set_wall_st_currency_to_usd", func(t *testing.T) {
		result, status := patchCashAccount(t, env, portfolioName, "Wall St", userHeaders, map[string]interface{}{
			"currency": "USD",
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("01_update_currency_response", string(body))

		require.Equal(t, http.StatusOK, status, "update_account should succeed with currency=USD")
		require.NotNil(t, result)
	})

	// Step 4: GET cash-summary and verify per-currency breakdown.
	t.Run("total_cash_by_currency_breakdown", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_multi_currency_summary", string(body))

		require.Equal(t, http.StatusOK, status)
		require.NotNil(t, result)

		summary, ok := result["summary"].(map[string]interface{})
		require.True(t, ok, "response must contain summary object")
		require.Contains(t, summary, "total_cash_by_currency",
			"summary must include total_cash_by_currency field")

		byCurrency := totalCashByCurrencyFromSummary(result)
		require.NotNil(t, byCurrency, "total_cash_by_currency must be a non-nil map")

		assert.InDelta(t, audDeposit, byCurrency["AUD"], 0.01,
			"AUD total should equal the Trading account deposit")
		assert.InDelta(t, usdDeposit, byCurrency["USD"], 0.01,
			"USD total should equal the Wall St account deposit")
	})

	t.Run("wall_st_account_shows_usd_currency", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		currency := accountCurrencyFromSummary(result, "Wall St")
		assert.Equal(t, "USD", currency, "Wall St account currency should be USD after update")
	})

	t.Run("trading_account_still_aud", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		currency := accountCurrencyFromSummary(result, "Trading")
		assert.Equal(t, "AUD", currency, "Trading account currency should remain AUD")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestCashAccountCurrency ---

// TestCashAccountCurrency verifies the full lifecycle of the currency field on
// a cash account: created with AUD default, updated to USD, persists across reads,
// and is reflected in the cash-summary response.
func TestCashAccountCurrency(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	const deposit = 25000.0
	const accountName = "Stake US"

	// Step 1: Create a transaction on a new account (auto-created with AUD default).
	t.Run("create_account_with_transaction", func(t *testing.T) {
		result, status := postCashTx(t, env, portfolioName, userHeaders,
			accountName, "contribution",
			deposit, "2025-01-20T00:00:00Z",
			"Initial USD account deposit")
		require.Equal(t, http.StatusCreated, status)
		require.NotNil(t, result)

		body, _ := json.Marshal(result)
		guard.SaveResult("01_create_transaction", string(body))
	})

	// Step 2: Verify account auto-created with AUD default currency.
	t.Run("new_account_defaults_to_AUD", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_before_currency_update", string(body))

		currency := accountCurrencyFromSummary(result, accountName)
		assert.Equal(t, "AUD", currency,
			"newly auto-created account must have default currency AUD")
	})

	// Step 3: Update account currency to USD.
	t.Run("update_currency_to_usd", func(t *testing.T) {
		result, status := patchCashAccount(t, env, portfolioName, accountName, userHeaders, map[string]interface{}{
			"currency": "USD",
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("03_update_currency", string(body))

		require.Equal(t, http.StatusOK, status, "update_account currency should succeed")
		require.NotNil(t, result)
	})

	// Step 4: Verify currency updated in cash-summary response.
	t.Run("currency_reflected_in_summary", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("04_after_currency_update", string(body))

		currency := accountCurrencyFromSummary(result, accountName)
		assert.Equal(t, "USD", currency,
			"account currency must be USD after update")
	})

	// Step 5: Verify currency persists across a fresh GET.
	t.Run("currency_persists_across_reads", func(t *testing.T) {
		// Second read to confirm persistence.
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("05_currency_persistence", string(body))

		currency := accountCurrencyFromSummary(result, accountName)
		assert.Equal(t, "USD", currency,
			"account currency must persist across reads")
	})

	// Step 6: Verify account balance is correct.
	t.Run("account_balance_correct", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		accounts, ok := result["accounts"].([]interface{})
		require.True(t, ok, "accounts must be an array")

		for _, rawAcct := range accounts {
			acct, ok := rawAcct.(map[string]interface{})
			if !ok {
				continue
			}
			if acct["name"] == accountName {
				balance, _ := acct["balance"].(float64)
				assert.InDelta(t, deposit, balance, 0.01,
					"account balance must equal the deposit amount")
			}
		}
	})

	// Step 7: Verify total_cash_by_currency reflects the USD account.
	t.Run("usd_reflected_in_by_currency", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		byCurrency := totalCashByCurrencyFromSummary(result)
		require.NotNil(t, byCurrency, "total_cash_by_currency must be present")

		assert.InDelta(t, deposit, byCurrency["USD"], 0.01,
			"total_cash_by_currency[USD] should equal the Stake US account deposit")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestCashSummary_NotFound ---

// TestCashSummary_NotFound verifies that cash-summary returns 404 for a
// portfolio that does not exist.
func TestCashSummary_NotFound(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPGet("/api/portfolios/nonexistent-portfolio-xyz/cash-summary")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_not_found", string(body))

	assert.Equal(t, http.StatusNotFound, resp.StatusCode,
		"cash-summary for non-existent portfolio should return 404")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestCashAccountCurrency_UpdatesOtherFieldsToo ---

// TestCashAccountCurrency_UpdatesOtherFieldsToo verifies that the update_account
// endpoint can update currency alongside other fields (type, is_transactional)
// without losing existing field values.
func TestCashAccountCurrency_UpdatesOtherFieldsToo(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForCashFlows(t, env)

	cleanupCashFlows(t, env, portfolioName, userHeaders)

	// Create "Accumulate" account by adding a transaction.
	t.Run("create_accumulate_account", func(t *testing.T) {
		_, status := postCashTx(t, env, portfolioName, userHeaders,
			"Accumulate", "contribution",
			5000, "2025-01-05T00:00:00Z",
			"Accumulate account seed transaction")
		require.Equal(t, http.StatusCreated, status)
	})

	// Update: set type=accumulate + currency=USD simultaneously.
	t.Run("update_type_and_currency_together", func(t *testing.T) {
		result, status := patchCashAccount(t, env, portfolioName, "Accumulate", userHeaders, map[string]interface{}{
			"type":     "accumulate",
			"currency": "USD",
		})

		body, _ := json.Marshal(result)
		guard.SaveResult("01_update_type_and_currency", string(body))

		require.Equal(t, http.StatusOK, status, "update_account with type+currency should succeed")
		require.NotNil(t, result)
	})

	// Verify both type and currency are reflected.
	t.Run("both_fields_reflected_in_summary", func(t *testing.T) {
		result, status := getCashSummary(t, env, portfolioName, userHeaders)
		require.Equal(t, http.StatusOK, status)

		body, _ := json.Marshal(result)
		guard.SaveResult("02_verify_type_and_currency", string(body))

		accounts, ok := result["accounts"].([]interface{})
		require.True(t, ok)

		var foundAccumulate bool
		for _, rawAcct := range accounts {
			acct, ok := rawAcct.(map[string]interface{})
			if !ok {
				continue
			}
			if acct["name"] == "Accumulate" {
				foundAccumulate = true
				accountType, _ := acct["type"].(string)
				currency, _ := acct["currency"].(string)
				assert.Equal(t, "accumulate", accountType,
					"account type should be updated to accumulate")
				assert.Equal(t, "USD", currency,
					"account currency should be updated to USD")
			}
		}
		assert.True(t, foundAccumulate, "Accumulate account must be present in summary")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
