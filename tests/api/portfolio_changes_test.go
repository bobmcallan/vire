package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/bobmcallan/vire/tests/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetPortfolio_ChangesSection tests the changes section in portfolio response
func TestGetPortfolio_ChangesSection(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	_, userHeaders, portfolioName := setupPortfolioEnv(t, common.EnvOptions{})

	basePath := "/api/portfolios/" + portfolioName

	// Test Case 1: Portfolio without timeline data (initial state)
	t.Run("InitialState", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		guard.SaveResult("01_initial_state", fmt.Sprintf("Status: %d", resp.StatusCode))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var response map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		// Verify changes section exists
		changes, exists := response["changes"]
		assert.True(t, exists, "Changes section should exist")

		// Verify structure
		changesMap := changes.(map[string]interface{})
		assert.Contains(t, changesMap, "yesterday")
		assert.Contains(t, changesMap, "week")
		assert.Contains(t, changesMap, "month")

		// Check yesterday's changes
		yesterday := changesMap["yesterday"].(map[string]interface{})
		assert.NotNil(t, yesterday["net_equity_return"])
		assert.NotNil(t, yesterday["net_equity_return_pct"])
		assert.NotNil(t, yesterday["portfolio_value"])
		assert.NotNil(t, yesterday["gross_cash"])
		assert.NotNil(t, yesterday["dividend"])
	})

	// Test Case 2: Portfolio with timeline data
	t.Run("WithTimelineData", func(t *testing.T) {
		// First sync to populate timeline
		syncPortfolio(t, env, portfolioName, userHeaders)

		// Wait a moment to ensure different timestamps
		time.Sleep(100 * time.Millisecond)

		// Sync again to create a change
		syncPortfolio(t, env, portfolioName, userHeaders)

		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		var response map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		// Verify changes are populated
		changes := response["changes"].(map[string]interface{})
		yesterday := changes["yesterday"].(map[string]interface{})

		// Check that HasPrevious might be true for some metrics
		netReturnChange := yesterday["net_equity_return"].(map[string]interface{})
		hasPrevious := netReturnChange["has_previous"].(bool)
		if hasPrevious {
			assert.NotZero(t, netReturnChange["previous"])
			assert.NotZero(t, netReturnChange["raw_change"])
		}
	})

	// Test Case 3: Portfolio with cash flow transactions
	t.Run("WithCashFlowTransactions", func(t *testing.T) {
		// Add cash flow transaction
		tx := map[string]interface{}{
			"account":     "Trading",
			"category":    "dividend",
			"date":        time.Now().Format("2006-01-02"),
			"amount":      100.0,
			"description": "Test dividend",
		}

		txBody, _ := json.Marshal(tx)
		resp, err := env.HTTPRequest(http.MethodPost, "/cash/transaction", bytes.NewBuffer(txBody), userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		guard.SaveResult("03_dividend_transaction", "Added dividend transaction")

		// Get portfolio again
		resp, err = env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		var response map[string]interface{}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &response)
		require.NoError(t, err)

		// Verify dividend changes are reflected
		changes := response["changes"].(map[string]interface{})
		week := changes["week"].(map[string]interface{})
		dividend := week["dividend"].(map[string]interface{})

		// Dividend should have previous value
		assert.True(t, dividend["has_previous"].(bool))
		assert.GreaterOrEqual(t, dividend["previous"].(float64), 0.0)
	})
}

// TestGetPortfolio_ChangesAfterSync tests changes after sync operations
func TestGetPortfolio_ChangesAfterSync(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	_, userHeaders, portfolioName := setupPortfolioEnv(t, common.EnvOptions{})

	// Sync the portfolio
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/sync",
		map[string]interface{}{"force": true}, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Get portfolio after sync
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var afterSyncResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&afterSyncResponse)
	require.NoError(t, err)

	// Values may have changed
	newNetReturn := afterSyncResponse["net_equity_return"].(float64)
	newPortfolio := afterSyncResponse["portfolio_value"].(float64)

	// Check changes section
	changes := afterSyncResponse["changes"].(map[string]interface{})
	yesterday := changes["yesterday"].(map[string]interface{})

	// Verify raw changes are correct for net_equity_return
	netReturnChange := yesterday["net_equity_return"].(map[string]interface{})
	if netReturnChange["has_previous"].(bool) {
		expectedRawChange := newNetReturn - netReturnChange["previous"].(float64)
		assert.Equal(t, expectedRawChange, netReturnChange["raw_change"])
	}

	portfolioChange := yesterday["portfolio_value"].(map[string]interface{})
	if portfolioChange["has_previous"].(bool) {
		expectedRawChange := newPortfolio - portfolioChange["previous"].(float64)
		assert.Equal(t, expectedRawChange, portfolioChange["raw_change"])
	}

	// Percentage changes use math.Abs(previous) as denominator (handles negative P&L)
	if netReturnChange["has_previous"].(bool) && netReturnChange["previous"].(float64) != 0 {
		prev := netReturnChange["previous"].(float64)
		absPrev := prev
		if absPrev < 0 {
			absPrev = -absPrev
		}
		expectedPctChange := ((newNetReturn - prev) / absPrev) * 100
		assert.InDelta(t, expectedPctChange, netReturnChange["pct_change"], 0.0001)
	}
}
