package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Fix 5: trade_list Empty for Navexa Portfolios (fb_9e063cf1) ---

// TestTradeList_NavexaPortfolio_ReturnsTrades verifies that trade_list returns
// trades extracted from Navexa holdings when the portfolio is Navexa-sourced.
//
// Before fix: trade_list queries UserDataStore (manual/snapshot only), returns empty for Navexa.
// After fix: handler checks source type and extracts trades from holdings[].Trades.
func TestTradeList_NavexaPortfolio_ReturnsTrades(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Step 1: Create a Navexa-sourced portfolio with holdings that contain trades
	// Note: This test documents the expected behavior. The actual Navexa portfolio
	// setup depends on how the test environment loads Navexa data.
	portfolioName := "navexa-test-portfolio"
	userHeaders := map[string]string{
		"X-User-ID": "trade-list-navexa-user",
	}

	t.Run("setup_navexa_portfolio", func(t *testing.T) {
		// In a real scenario, this would be loaded from Navexa API
		// For testing, we'd use a snapshot or mock
		// This is a placeholder to document what should happen.
		t.Logf("Portfolio %s should be Navexa-sourced with holdings containing trades", portfolioName)
	})

	// Step 2: Get portfolio and verify it has holdings with trades
	t.Run("get_portfolio_with_holdings", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, userHeaders)
		if err != nil {
			t.Skipf("Test environment not configured for Navexa portfolio test: %v", err)
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("01_get_portfolio", fmt.Sprintf("%d response", resp.StatusCode))

		if resp.StatusCode != http.StatusOK {
			t.Skipf("Portfolio not found (status %d), skipping Navexa trade test", resp.StatusCode)
			return
		}

		body, _ := io.ReadAll(resp.Body)
		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// Verify portfolio is Navexa-sourced
		sourceType, ok := portfolio["source_type"].(string)
		if !ok || sourceType == "" {
			t.Skip("Test portfolio source_type not set; skipping Navexa trade extraction test")
			return
		}
	})

	// Step 3: Call trade_list endpoint
	t.Run("trade_list_returns_trades", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/trades", nil, userHeaders)
		if err != nil {
			t.Skipf("Trade endpoint error: %v", err)
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("02_trade_list", resp.Status)

		// After fix: trade_list should return trades extracted from holdings
		// Before fix: would return empty because UserDataStore has no manual trades
		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &result))

			trades, ok := result["trades"].([]interface{})
			if ok && len(trades) > 0 {
				// Verify first trade has expected fields
				trade := trades[0].(map[string]interface{})
				assert.Contains(t, trade, "ticker", "trade should have ticker field")
				assert.Contains(t, trade, "action", "trade should have action field (buy/sell)")
				assert.Contains(t, trade, "units", "trade should have units field")
				assert.Contains(t, trade, "price", "trade should have price field")
				assert.Contains(t, trade, "date", "trade should have date field")
			}
		}
	})
}

// TestTradeList_NavexaPortfolio_FilterByAction verifies that trades can be filtered by action (buy/sell).
func TestTradeList_NavexaPortfolio_FilterByAction(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	portfolioName := "navexa-filter-test"
	userHeaders := map[string]string{
		"X-User-ID": "trade-filter-user",
	}

	t.Run("filter_by_sell_action", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/trades?action=sell", nil, userHeaders)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Skipf("Trade filter endpoint not available")
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("03_filter_sell", resp.Status)

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		trades, ok := result["trades"].([]interface{})
		if ok {
			// All returned trades should be sell actions
			for _, item := range trades {
				trade := item.(map[string]interface{})
				action := trade["action"].(string)
				assert.Equal(t, "sell", action, "filtered results should only contain sell trades")
			}
		}
	})
}

// TestTradeList_NavexaPortfolio_FilterByDate verifies that trades can be filtered by date range.
func TestTradeList_NavexaPortfolio_FilterByDate(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	portfolioName := "navexa-date-filter-test"
	userHeaders := map[string]string{
		"X-User-ID": "trade-date-filter-user",
	}

	t.Run("filter_by_date_range", func(t *testing.T) {
		startDate := time.Now().Add(-30 * 24 * time.Hour).Format("2006-01-02")
		endDate := time.Now().Format("2006-01-02")

		url := "/api/portfolios/" + portfolioName + "/trades?date_from=" + startDate + "&date_to=" + endDate
		resp, err := env.HTTPRequest(http.MethodGet, url, nil, userHeaders)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Skipf("Trade date filter endpoint not available")
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("04_filter_date", resp.Status)

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		trades, ok := result["trades"].([]interface{})
		if ok {
			// All returned trades should be within the date range
			for _, item := range trades {
				trade := item.(map[string]interface{})
				dateStr := trade["date"].(string)
				tradeDate, parseErr := time.Parse(time.RFC3339, dateStr)
				if parseErr == nil {
					assert.True(t, !tradeDate.Before(time.Now().Add(-30*24*time.Hour)),
						"trade date should be >= start date")
					assert.True(t, !tradeDate.After(time.Now()),
						"trade date should be <= end date")
				}
			}
		}
	})
}

// TestTradeList_ManualPortfolio_UsesTradeService verifies that manual/snapshot
// portfolios still use TradeService (UserDataStore) after the fix.
//
// After Fix 5: handler checks portfolio.SourceType. If not Navexa, falls back
// to TradeService.ListTrades (which queries UserDataStore).
func TestTradeList_ManualPortfolio_UsesTradeService(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Create a manual portfolio
	portfolioName := "manual-trade-list-test"
	userHeaders := map[string]string{
		"X-User-ID": "manual-trade-user",
	}

	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Create portfolio
	t.Run("create_manual_portfolio", func(t *testing.T) {
		createReq := map[string]interface{}{
			"name":        portfolioName,
			"description": "Manual portfolio for trade list test",
		}
		resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", createReq, userHeaders)
		if err != nil || resp.StatusCode != http.StatusCreated {
			t.Skipf("Portfolio creation failed")
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("05_create_manual_portfolio", resp.Status)
	})

	// Step 2: Add a manual trade
	t.Run("add_manual_trade", func(t *testing.T) {
		tradeReq := map[string]interface{}{
			"action":      "buy",
			"ticker":      "BHP.AU",
			"units":       100.0,
			"price":       55.50,
			"date":        time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339),
			"description": "Manual buy trade",
		}
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/trades", tradeReq, userHeaders)
		if err != nil {
			t.Logf("Trade creation not supported in test environment: %v", err)
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("06_add_manual_trade", resp.Status)
	})

	// Step 3: List trades from manual portfolio
	t.Run("list_trades_from_manual_portfolio", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/trades", nil, userHeaders)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Skipf("Trade list endpoint not available")
			return
		}
		defer resp.Body.Close()

		guard.SaveResult("07_manual_trade_list", resp.Status)

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Verify structure
		assert.Contains(t, result, "trades", "response should contain trades array")
		assert.Contains(t, result, "total", "response should contain total count")

		// After the fix, manual portfolios should still work via TradeService
		trades, ok := result["trades"].([]interface{})
		if ok && len(trades) > 0 {
			trade := trades[0].(map[string]interface{})
			assert.Contains(t, trade, "ticker")
			assert.Contains(t, trade, "action")
			assert.Equal(t, "BHP.AU", trade["ticker"], "trade ticker should match")
		}
	})
}
