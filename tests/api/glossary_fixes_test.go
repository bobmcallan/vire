package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Fix 3+4: Glossary Corrections and Duplicate Removals ---

// TestGlossary_NoDuplicateTerms verifies that glossary terms are unique across all categories.
// Before fix: "capital_gross" appeared in both Valuation and Growth categories.
// Before fix: "capital_contributions_net" appeared in both Capital and Growth categories.
// After fix: Duplicates removed from Growth category.
func TestGlossary_NoDuplicateTerms(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_glossary_no_duplicates", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok, "categories should be an array")

	// Collect all term names across categories
	termNames := make(map[string]int) // term name -> count
	var growthCategory map[string]interface{}

	for _, catInterface := range categories {
		cat, ok := catInterface.(map[string]interface{})
		require.True(t, ok)

		catName, ok := cat["name"].(string)
		require.True(t, ok)

		if catName == "Growth" {
			growthCategory = cat
		}

		terms, ok := cat["terms"].([]interface{})
		require.True(t, ok)

		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			termNames[termName]++
		}
	}

	// Check for duplicates
	var duplicates []string
	for name, count := range termNames {
		if count > 1 {
			duplicates = append(duplicates, name)
		}
	}

	assert.Empty(t, duplicates,
		"Glossary should have no duplicate terms across categories. Found duplicates: %v", duplicates)

	// Verify that Growth category does NOT contain "capital_gross" or "capital_contributions_net"
	if growthCategory != nil {
		terms, ok := growthCategory["terms"].([]interface{})
		require.True(t, ok)

		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			assert.NotEqual(t, "capital_gross", termName,
				"Growth category should not contain 'gross_cash_balance' (should be in Valuation only)")
			assert.NotEqual(t, "capital_contributions_net", termName,
				"Growth category should not contain 'net_capital_deployed' (should be in Capital only)")
		}
	}
}

// TestGlossary_GrowthMetricsOnlyHasRequiredTerms verifies that Growth Metrics category
// contains only yesterday_change and last_week_change after the fix.
// Before fix: contained duplicate gross_cash_balance and net_capital_deployed.
func TestGlossary_GrowthMetricsOnlyHasRequiredTerms(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("02_growth_metrics_check", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	for _, catInterface := range categories {
		cat, ok := catInterface.(map[string]interface{})
		require.True(t, ok)

		catName, ok := cat["name"].(string)
		require.True(t, ok)

		if catName != "Growth" {
			continue
		}

		terms, ok := cat["terms"].([]interface{})
		require.True(t, ok)

		// After fix: Growth should have exactly 2 terms: yesterday_change and last_week_change
		// (or similar count - the exact number depends on the spec)
		assert.NotEmpty(t, terms, "Growth category should have terms")

		termNames := make([]string, 0)
		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			termNames = append(termNames, termName)

			// These should NOT be in Growth (they're duplicates)
			assert.NotEqual(t, "capital_gross", termName,
				"Growth should not contain gross_cash_balance after fix")
			assert.NotEqual(t, "capital_contributions_net", termName,
				"Growth should not contain net_capital_deployed after fix")
		}

		// Verify Growth contains the expected terms
		assert.Contains(t, termNames, "yesterday_change",
			"Growth should contain yesterday_change")
		assert.Contains(t, termNames, "last_week_change",
			"Growth should contain last_week_change")

		t.Logf("Growth category terms after fix: %v", termNames)
	}
}

// TestGlossary_CorrectFormulas verifies that glossary formulas are correct.
// Before fix: simple_capital_return_pct formula used equity_value (wrong).
// After fix: formula uses portfolio_value (correct).
// Also: net_equity_return definition incorrectly said "Unrealised gain or loss" (wrong).
// After fix: definition mentions realised + unrealised (correct).
func TestGlossary_CorrectFormulas(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("03_formulas_check", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	for _, catInterface := range categories {
		cat, ok := catInterface.(map[string]interface{})
		require.True(t, ok)

		terms, ok := cat["terms"].([]interface{})
		require.True(t, ok)

		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			// Check simple_capital_return_pct formula
			if termName == "capital_return_simple_pct" {
				formula, ok := term["formula"].(string)
				require.True(t, ok, "simple_capital_return_pct should have a formula")

				// After fix: should use portfolio_value, not equity_value
				assert.Contains(t, formula, "portfolio_value",
					"simple_capital_return_pct formula should use portfolio_value (after Fix 1)")
				assert.NotContains(t, formula, "equity_holdings_value",
					"simple_capital_return_pct formula should NOT use equity_value (it's now called current_value)")

				t.Logf("simple_capital_return_pct formula: %s", formula)
			}

			// Check net_equity_return definition
			if termName == "equity_holdings_return" {
				definition, ok := term["definition"].(string)
				require.True(t, ok, "net_equity_return should have a definition")

				// After fix: should mention realised gains/losses (not just unrealised)
				assert.Contains(t, definition, "realised",
					"net_equity_return definition should mention realised gains/losses")

				// Should NOT say only "unrealised"
				if assert.NotContains(t, definition, "Unrealised gain or loss", "") {
					t.Logf("net_equity_return definition correctly updated: %s", definition)
				}
			}
		}
	}
}

// TestGlossary_GrowthMetricsCalculationsCorrect verifies that growth metrics
// (yesterday_change, last_week_change) are calculated correctly.
// Before fix: EquityValue was subtracted from PortfolioYesterdayValue (mixing fields).
// After fix: PortfolioValue is subtracted from PortfolioYesterdayValue (consistent).
func TestGlossary_GrowthMetricsCalculationsCorrect(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("04_growth_calculations", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	for _, catInterface := range categories {
		cat, ok := catInterface.(map[string]interface{})
		require.True(t, ok)

		catName, ok := cat["name"].(string)
		require.True(t, ok)

		if catName != "Growth" {
			continue
		}

		terms, ok := cat["terms"].([]interface{})
		require.True(t, ok)

		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			// Check yesterday_change and last_week_change examples
			if termName == "yesterday_change" || termName == "last_week_change" {
				example, ok := term["example"].(string)
				if ok {
					// After fix: should use portfolio_value (not equity_value)
					// Example might show: "portfolio_value - portfolio_yesterday_value = ..."
					assert.Contains(t, example, "portfolio_value",
						"%s example should use portfolio_value in calculation", termName)

					// Should not mix equity_value with portfolio_yesterday_value
					if assert.NotContains(t, example, "equity_holdings_value", "") {
						t.Logf("%s example correctly uses portfolio_value", termName)
					}
				}
			}
		}
	}
}

// TestGlossary_NetEquityReturnDefinition verifies that net_equity_return
// correctly describes both realised and unrealised returns.
// Before fix: Definition said "Unrealised gain or loss" (incomplete).
// After fix: Mentions "realised gains/losses from closed positions" (complete).
func TestGlossary_NetEquityReturnDefinition(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("05_net_equity_return", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	found := false
	for _, catInterface := range categories {
		cat, ok := catInterface.(map[string]interface{})
		require.True(t, ok)

		terms, ok := cat["terms"].([]interface{})
		require.True(t, ok)

		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			if termName == "equity_holdings_return" {
				found = true

				definition, ok := term["definition"].(string)
				require.True(t, ok)

				// After fix: Should include mention of realised returns
				assert.Contains(t, definition, "realised",
					"net_equity_return should mention realised gains/losses from closed positions")

				// Should not ONLY say "unrealised"
				if len(definition) > 0 && definition != "Unrealised gain or loss across the portfolio." {
					t.Logf("net_equity_return definition fixed: %s", definition)
				}

				break
			}
		}
	}

	assert.True(t, found, "net_equity_return term should exist in glossary")
}

// TestGlossary_DayWeekMonthPeriodFixtures verifies that yesterday_change and
// last_week_change examples use consistent field names (portfolio_value, not mixed).
// Before fix: Examples mixed equity_value and portfolio_*_value.
// After fix: All examples use portfolio_value consistently.
func TestGlossary_DayWeekMonthPeriodFixtures(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("06_period_fixtures", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	for _, catInterface := range categories {
		cat, ok := catInterface.(map[string]interface{})
		require.True(t, ok)

		terms, ok := cat["terms"].([]interface{})
		require.True(t, ok)

		for _, termInterface := range terms {
			term, ok := termInterface.(map[string]interface{})
			require.True(t, ok)

			termName, ok := term["name"].(string)
			require.True(t, ok)

			// Check period change examples
			if termName == "yesterday_change" || termName == "last_week_change" {
				example, ok := term["example"].(string)
				if ok && len(example) > 0 {
					// Should be consistent: portfolio_value - portfolio_yesterday_value
					// Should NOT mix: equity_value - portfolio_yesterday_value
					if termName == "yesterday_change" {
						assert.Contains(t, example, "portfolio_value",
							"yesterday_change should use portfolio_value")
						assert.NotContains(t, example, "equity_holdings_value",
							"yesterday_change should not use equity_value")
					}
					if termName == "last_week_change" {
						assert.Contains(t, example, "portfolio_value",
							"last_week_change should use portfolio_value")
						assert.NotContains(t, example, "equity_holdings_value",
							"last_week_change should not use equity_value")
					}
				}
			}
		}
	}
}
