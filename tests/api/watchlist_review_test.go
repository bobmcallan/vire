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

// createUser creates a test user and returns the user ID.
func createUserForWatchlist(t *testing.T, env *common.Env) string {
	t.Helper()
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "wlreview_user",
		"email":    "wlreview@test.com",
		"password": "password123",
		"role":     "user",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)
	return "wlreview_user"
}

// userHeaders returns headers with a user ID.
func userHeadersForWatchlist(userID string) map[string]string {
	return map[string]string{"X-Vire-User-ID": userID}
}

// setWatchlist sets the watchlist for a portfolio via PUT /api/portfolios/{name}/watchlist.
func setWatchlist(t *testing.T, env *common.Env, portfolio string, items []map[string]interface{}, headers map[string]string) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPut, "/api/portfolios/"+portfolio+"/watchlist",
		map[string]interface{}{
			"items": items,
		}, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "set watchlist failed: %s", string(body))
}

// reviewWatchlist calls POST /api/portfolios/{name}/watchlist/review and returns status, body, and parsed JSON.
func reviewWatchlist(t *testing.T, env *common.Env, portfolio string, reqBody map[string]interface{}, headers map[string]string) (int, []byte, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolio+"/watchlist/review",
		reqBody, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	if json.Valid(body) {
		json.Unmarshal(body, &result)
	}

	return resp.StatusCode, body, result
}

// --- Test: Review watchlist with valid items ---

func TestWatchlistReview(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userID := createUserForWatchlist(t, env)
	headers := userHeadersForWatchlist(userID)

	// Set up a watchlist with known tickers (no market data exists in fresh env,
	// so all items should return "Market data unavailable")
	watchlistItems := []map[string]interface{}{
		{"ticker": "BHP.AU", "verdict": "PASS", "name": "BHP Group", "reason": "Strong fundamentals"},
		{"ticker": "CBA.AU", "verdict": "WATCH", "name": "Commonwealth Bank", "reason": "High PE"},
		{"ticker": "WOW.AU", "verdict": "FAIL", "name": "Woolworths", "reason": "Declining margins"},
	}
	setWatchlist(t, env, "TestPortfolio", watchlistItems, headers)

	t.Run("review_returns_item_reviews", func(t *testing.T) {
		status, body, result := reviewWatchlist(t, env, "TestPortfolio",
			map[string]interface{}{}, headers)

		guard.SaveResult("01_review_response", string(body))

		assert.Equal(t, http.StatusOK, status, "unexpected status: %s", string(body))
		assert.Equal(t, "TestPortfolio", result["portfolio_name"])
		assert.NotEmpty(t, result["review_date"])

		itemReviews, ok := result["item_reviews"].([]interface{})
		require.True(t, ok, "item_reviews should be an array")
		assert.Len(t, itemReviews, 3, "should have one review per watchlist item")

		// Each item review should have the watchlist item data
		for _, ir := range itemReviews {
			review := ir.(map[string]interface{})
			item := review["item"].(map[string]interface{})
			assert.NotEmpty(t, item["ticker"], "each review should have a ticker")
			assert.NotEmpty(t, review["action_required"], "each review should have action_required")
			assert.NotEmpty(t, review["action_reason"], "each review should have action_reason")
		}
	})

	t.Run("review_preserves_watchlist_item_data", func(t *testing.T) {
		status, body, result := reviewWatchlist(t, env, "TestPortfolio",
			map[string]interface{}{}, headers)

		guard.SaveResult("02_review_item_data", string(body))

		require.Equal(t, http.StatusOK, status)

		itemReviews := result["item_reviews"].([]interface{})
		// Verify the first item retains its watchlist data
		firstReview := itemReviews[0].(map[string]interface{})
		item := firstReview["item"].(map[string]interface{})
		assert.Equal(t, "BHP.AU", item["ticker"])
		assert.Equal(t, "PASS", item["verdict"])
		assert.Equal(t, "BHP Group", item["name"])
		assert.Equal(t, "Strong fundamentals", item["reason"])
	})

	t.Run("items_without_market_data_get_watch_action", func(t *testing.T) {
		// In a fresh container there's no market data, so all items should
		// get the "Market data unavailable" action
		status, body, result := reviewWatchlist(t, env, "TestPortfolio",
			map[string]interface{}{}, headers)

		guard.SaveResult("03_no_market_data", string(body))

		require.Equal(t, http.StatusOK, status)

		itemReviews := result["item_reviews"].([]interface{})
		for _, ir := range itemReviews {
			review := ir.(map[string]interface{})
			assert.Equal(t, "WATCH", review["action_required"],
				"items without market data should have WATCH action")
			assert.Contains(t, review["action_reason"], "Market data unavailable",
				"action_reason should explain missing data")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Review with focus_signals parameter ---

func TestWatchlistReviewWithFocusSignals(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userID := createUserForWatchlist(t, env)
	headers := userHeadersForWatchlist(userID)

	watchlistItems := []map[string]interface{}{
		{"ticker": "BHP.AU", "verdict": "PASS", "name": "BHP Group"},
	}
	setWatchlist(t, env, "TestPortfolio", watchlistItems, headers)

	// Request with focus_signals — the endpoint should accept this parameter
	status, body, result := reviewWatchlist(t, env, "TestPortfolio",
		map[string]interface{}{
			"focus_signals": []string{"rsi", "sma"},
		}, headers)

	guard.SaveResult("01_focus_signals", string(body))

	assert.Equal(t, http.StatusOK, status, "focus_signals should be accepted: %s", string(body))
	assert.Equal(t, "TestPortfolio", result["portfolio_name"])

	itemReviews := result["item_reviews"].([]interface{})
	assert.Len(t, itemReviews, 1)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Review empty watchlist returns error ---

func TestWatchlistReviewEmptyWatchlist(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userID := createUserForWatchlist(t, env)
	headers := userHeadersForWatchlist(userID)

	// Set an empty watchlist
	setWatchlist(t, env, "TestPortfolio", []map[string]interface{}{}, headers)

	status, body, _ := reviewWatchlist(t, env, "TestPortfolio",
		map[string]interface{}{}, headers)

	guard.SaveResult("01_empty_watchlist_review", string(body))

	// The service returns an error for empty watchlists
	assert.Equal(t, http.StatusInternalServerError, status,
		"empty watchlist should return error: %s", string(body))
	assert.True(t, json.Valid(body), "response should be valid JSON")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Review non-existent portfolio watchlist ---

func TestWatchlistReviewNonExistentPortfolio(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// No user setup, no watchlist — call review on non-existent portfolio
	status, body, _ := reviewWatchlist(t, env, "nonexistent",
		map[string]interface{}{}, nil)

	guard.SaveResult("01_nonexistent_portfolio", string(body))

	assert.Equal(t, http.StatusInternalServerError, status,
		"non-existent portfolio should return error")
	assert.True(t, json.Valid(body), "response should be valid JSON")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Wrong HTTP method ---

func TestWatchlistReviewMethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name   string
		method string
	}{
		{"GET", http.MethodGet},
		{"PUT", http.MethodPut},
		{"DELETE", http.MethodDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPRequest(tt.method, "/api/portfolios/TestPortfolio/watchlist/review",
				nil, nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("method_"+tt.name, string(body))

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode,
				"%s should not be allowed", tt.name)
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Response structure validation ---

func TestWatchlistReviewResponseStructure(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userID := createUserForWatchlist(t, env)
	headers := userHeadersForWatchlist(userID)

	watchlistItems := []map[string]interface{}{
		{"ticker": "BHP.AU", "verdict": "PASS", "name": "BHP Group"},
	}
	setWatchlist(t, env, "TestPortfolio", watchlistItems, headers)

	status, body, result := reviewWatchlist(t, env, "TestPortfolio",
		map[string]interface{}{}, headers)

	guard.SaveResult("01_structure", string(body))

	require.Equal(t, http.StatusOK, status)

	// Top-level fields
	assert.Contains(t, result, "portfolio_name", "response must have portfolio_name")
	assert.Contains(t, result, "review_date", "response must have review_date")
	assert.Contains(t, result, "item_reviews", "response must have item_reviews")

	// item_reviews[0] structure
	itemReviews := result["item_reviews"].([]interface{})
	require.Len(t, itemReviews, 1)

	review := itemReviews[0].(map[string]interface{})
	assert.Contains(t, review, "item", "review must have item")
	assert.Contains(t, review, "action_required", "review must have action_required")
	assert.Contains(t, review, "action_reason", "review must have action_reason")
	assert.Contains(t, review, "overnight_move", "review must have overnight_move")
	assert.Contains(t, review, "overnight_pct", "review must have overnight_pct")

	// item sub-structure
	item := review["item"].(map[string]interface{})
	assert.Contains(t, item, "ticker", "item must have ticker")
	assert.Contains(t, item, "verdict", "item must have verdict")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Review with blank config ---

func TestWatchlistReviewBlankConfig(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// With blank config and no user, review should fail gracefully
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/SMSF/watchlist/review",
		map[string]interface{}{}, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "review request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_blank_config_review", string(body))

	// Should return error (no watchlist exists)
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"expected error with blank config, got: %s", string(body))
	assert.True(t, json.Valid(body), "response should be valid JSON: %s", string(body))

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Test: Multiple review calls are idempotent ---

func TestWatchlistReviewIdempotent(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userID := createUserForWatchlist(t, env)
	headers := userHeadersForWatchlist(userID)

	watchlistItems := []map[string]interface{}{
		{"ticker": "BHP.AU", "verdict": "PASS", "name": "BHP Group"},
		{"ticker": "CBA.AU", "verdict": "WATCH", "name": "Commonwealth Bank"},
	}
	setWatchlist(t, env, "TestPortfolio", watchlistItems, headers)

	// Call review twice — both should succeed with consistent structure
	status1, body1, result1 := reviewWatchlist(t, env, "TestPortfolio",
		map[string]interface{}{}, headers)
	guard.SaveResult("01_review_call_1", string(body1))

	status2, body2, result2 := reviewWatchlist(t, env, "TestPortfolio",
		map[string]interface{}{}, headers)
	guard.SaveResult("02_review_call_2", string(body2))

	assert.Equal(t, http.StatusOK, status1)
	assert.Equal(t, http.StatusOK, status2)

	// Same portfolio name
	assert.Equal(t, result1["portfolio_name"], result2["portfolio_name"])

	// Same number of item reviews
	reviews1 := result1["item_reviews"].([]interface{})
	reviews2 := result2["item_reviews"].([]interface{})
	assert.Equal(t, len(reviews1), len(reviews2),
		"repeated reviews should return same number of items")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
