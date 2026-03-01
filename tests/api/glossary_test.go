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

// --- Helpers ---

// setupPortfolioForGlossary reuses the indicators setup helper since it needs
// the same prerequisites: a synced portfolio with Navexa data.
func setupPortfolioForGlossary(t *testing.T, env *common.Env) (string, map[string]string) {
	t.Helper()
	return setupPortfolioForIndicators(t, env)
}

// --- GET /api/portfolios/{name}/glossary ---

// TestGlossary_GET verifies that the glossary endpoint returns a valid
// GlossaryResponse with the correct top-level structure.
func TestGlossary_GET(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	t.Run("returns_200_with_valid_structure", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_glossary_response", string(body))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result), "response should be valid JSON")

		// Top-level fields
		assert.Equal(t, portfolioName, result["portfolio_name"], "portfolio_name should match")
		assert.Contains(t, result, "generated_at", "response should have generated_at timestamp")
		assert.Contains(t, result, "categories", "response should have categories array")

		categories, ok := result["categories"].([]interface{})
		require.True(t, ok, "categories should be an array")
		assert.NotEmpty(t, categories, "categories should not be empty")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_Categories verifies that the expected categories are present
// in the glossary response.
func TestGlossary_Categories(t *testing.T) {
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
	guard.SaveResult("02_categories", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	// Collect category names
	categoryNames := make([]string, 0, len(categories))
	for _, cat := range categories {
		catMap, ok := cat.(map[string]interface{})
		require.True(t, ok, "each category should be a JSON object")
		name, ok := catMap["name"].(string)
		require.True(t, ok, "each category should have a name string")
		categoryNames = append(categoryNames, name)

		// Each category must have a terms array
		terms, ok := catMap["terms"].([]interface{})
		require.True(t, ok, "category %q should have a terms array", name)
		assert.NotEmpty(t, terms, "category %q should have at least one term", name)
	}

	// Minimum required categories per task spec
	assert.Contains(t, categoryNames, "Portfolio Valuation", "must include Portfolio Valuation category")
	assert.Contains(t, categoryNames, "Holding Metrics", "must include Holding Metrics category")

	t.Logf("Categories found: %v", categoryNames)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_TermStructure verifies that each term has the required fields.
func TestGlossary_TermStructure(t *testing.T) {
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
	guard.SaveResult("03_term_structure", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories, ok := result["categories"].([]interface{})
	require.True(t, ok)

	for _, cat := range categories {
		catMap := cat.(map[string]interface{})
		catName := catMap["name"].(string)
		terms := catMap["terms"].([]interface{})

		for _, term := range terms {
			termMap, ok := term.(map[string]interface{})
			require.True(t, ok, "term in category %q should be a JSON object", catName)

			// Required fields per spec
			assert.Contains(t, termMap, "term", "term in %q must have 'term' field", catName)
			assert.Contains(t, termMap, "label", "term in %q must have 'label' field", catName)
			assert.Contains(t, termMap, "definition", "term in %q must have 'definition' field", catName)
			assert.Contains(t, termMap, "value", "term in %q must have 'value' field", catName)

			// term must be snake_case (non-empty string)
			termKey, ok := termMap["term"].(string)
			assert.True(t, ok && len(termKey) > 0, "term.term should be a non-empty string in %q", catName)

			// label must be non-empty
			label, ok := termMap["label"].(string)
			assert.True(t, ok && len(label) > 0, "term.label should be a non-empty string in %q", catName)

			// definition must be non-empty
			def, ok := termMap["definition"].(string)
			assert.True(t, ok && len(def) > 0, "term.definition should be a non-empty string in %q", catName)
		}
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_PortfolioValuationTerms verifies the Portfolio Valuation category
// contains the expected terms.
func TestGlossary_PortfolioValuationTerms(t *testing.T) {
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
	guard.SaveResult("04_valuation_terms", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	// Find the Portfolio Valuation category
	var valuationCat map[string]interface{}
	categories := result["categories"].([]interface{})
	for _, cat := range categories {
		catMap := cat.(map[string]interface{})
		if catMap["name"] == "Portfolio Valuation" {
			valuationCat = catMap
			break
		}
	}
	require.NotNil(t, valuationCat, "Portfolio Valuation category must exist")

	terms := valuationCat["terms"].([]interface{})
	termKeys := make([]string, 0, len(terms))
	for _, term := range terms {
		termMap := term.(map[string]interface{})
		termKeys = append(termKeys, termMap["term"].(string))
	}

	// Per spec: Portfolio Valuation terms
	assert.Contains(t, termKeys, "total_value")
	assert.Contains(t, termKeys, "total_cost")
	assert.Contains(t, termKeys, "net_return")
	assert.Contains(t, termKeys, "net_return_pct")
	assert.Contains(t, termKeys, "total_capital")
	assert.Contains(t, termKeys, "total_cash")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_HoldingMetricsTerms verifies the Holding Metrics category
// contains the expected terms and that examples reference actual holdings.
func TestGlossary_HoldingMetricsTerms(t *testing.T) {
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
	guard.SaveResult("05_holding_metrics", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	// Find Holding Metrics category
	var holdingCat map[string]interface{}
	categories := result["categories"].([]interface{})
	for _, cat := range categories {
		catMap := cat.(map[string]interface{})
		if catMap["name"] == "Holding Metrics" {
			holdingCat = catMap
			break
		}
	}
	require.NotNil(t, holdingCat, "Holding Metrics category must exist")

	terms := holdingCat["terms"].([]interface{})
	termKeys := make([]string, 0, len(terms))
	for _, term := range terms {
		termMap := term.(map[string]interface{})
		termKeys = append(termKeys, termMap["term"].(string))
	}

	// Per spec: Holding Metrics terms
	assert.Contains(t, termKeys, "market_value")
	assert.Contains(t, termKeys, "avg_cost")
	assert.Contains(t, termKeys, "weight")
	assert.Contains(t, termKeys, "net_return")
	assert.Contains(t, termKeys, "net_return_pct")

	// Examples should reference actual holdings (non-empty example strings)
	for _, term := range terms {
		termMap := term.(map[string]interface{})
		if example, ok := termMap["example"].(string); ok {
			assert.NotEmpty(t, example, "holding metric %q should have a non-empty example", termMap["term"])
		}
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_NotFound verifies that requesting a glossary for a non-existent
// portfolio returns 404.
func TestGlossary_NotFound(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	t.Run("nonexistent_portfolio_returns_404", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/portfolios/nonexistent_portfolio_xyz/glossary")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("06_not_found", string(body))

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_MethodNotAllowed verifies that non-GET methods return 405.
func TestGlossary_MethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			resp, err := env.HTTPRequest(method, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("method_"+method, string(body))

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode,
				"%s should return 405 Method Not Allowed", method)
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestGlossary_CapitalPerformanceSection verifies that when cash transactions
// exist, the Capital Performance category is included in the glossary.
func TestGlossary_CapitalPerformanceSection(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForGlossary(t, env)

	// Add a cash transaction so capital performance data exists
	resp, err := env.HTTPRequest(http.MethodPost,
		"/api/portfolios/"+portfolioName+"/cash-transactions",
		map[string]interface{}{
			"type":        "deposit",
			"date":        "2025-01-15T00:00:00Z",
			"amount":      50000,
			"description": "Glossary test deposit",
		},
		userHeaders,
	)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "deposit should succeed")

	// Fetch glossary
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/glossary", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("07_with_capital_perf", string(body))
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	categories := result["categories"].([]interface{})
	categoryNames := make([]string, 0, len(categories))
	for _, cat := range categories {
		catMap := cat.(map[string]interface{})
		categoryNames = append(categoryNames, catMap["name"].(string))
	}

	assert.Contains(t, categoryNames, "Capital Performance",
		"Capital Performance category should be present when transactions exist")

	// Verify Capital Performance terms
	var capitalCat map[string]interface{}
	for _, cat := range categories {
		catMap := cat.(map[string]interface{})
		if catMap["name"] == "Capital Performance" {
			capitalCat = catMap
			break
		}
	}

	if capitalCat != nil {
		terms := capitalCat["terms"].([]interface{})
		termKeys := make([]string, 0, len(terms))
		for _, term := range terms {
			termMap := term.(map[string]interface{})
			termKeys = append(termKeys, termMap["term"].(string))
		}

		assert.Contains(t, termKeys, "total_deposited")
		assert.Contains(t, termKeys, "total_withdrawn")
		assert.Contains(t, termKeys, "net_capital_deployed")
		assert.Contains(t, termKeys, "simple_return_pct")
		assert.Contains(t, termKeys, "annualized_return_pct")
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
