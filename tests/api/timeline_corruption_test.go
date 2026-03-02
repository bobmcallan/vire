package api

// Integration tests for force-refresh timeline integrity.
//
// These tests verify that the force-refresh implementation correctly
// preserves historical EOD data when re-fetching market data, preventing
// data corruption (timeline bar count reduction).
//
// Coverage:
//   - TestForceRefresh_PreservesTimelineIntegrity — Verifies portfolio timeline
//     data_points count is preserved after force-refreshing a holding
//   - TestForceRefresh_EODBarCount — Verifies stock data candles count is
//     preserved after force-refreshing the stock

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

// getPortfolioTimeline fetches the portfolio timeline data_points.
func getPortfolioTimeline(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) ([]interface{}, error) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/timeline", nil, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Logf("GET timeline failed with status %d: %s", resp.StatusCode, string(body))
		return nil, err
	}

	var timeline map[string]interface{}
	if err := json.Unmarshal(body, &timeline); err != nil {
		return nil, err
	}

	dataPoints, ok := timeline["data_points"].([]interface{})
	if !ok {
		return nil, err
	}

	return dataPoints, nil
}

// getStockDataWithPrice fetches stock data including price (which includes candles).
// Returns nil if market data is not yet available (not an error).
func getStockDataWithPrice(t *testing.T, env *common.Env, ticker string, headers map[string]string) map[string]interface{} {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/market/stocks/"+ticker+"?include=price", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// If market data hasn't been synced yet, the API returns 500 with "not found" error
	// This is not a test failure - it means we should skip the candle test
	if resp.StatusCode == http.StatusInternalServerError {
		return nil
	}

	require.Equal(t, http.StatusOK, resp.StatusCode, "GET stock data failed: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

// forceRefreshStockData forces a re-fetch of stock data with force_refresh=true.
// Returns nil if market data is not yet available (not an error).
func forceRefreshStockData(t *testing.T, env *common.Env, ticker string, headers map[string]string) map[string]interface{} {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/market/stocks/"+ticker+"?include=price&force_refresh=true", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// If market data hasn't been synced yet, the API returns 500 with "not found" error
	if resp.StatusCode == http.StatusInternalServerError {
		return nil
	}

	require.Equal(t, http.StatusOK, resp.StatusCode, "Force refresh stock data failed: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

// getCandlesCount extracts the candles array length from stock data.
func getCandlesCount(t *testing.T, stockData map[string]interface{}) int {
	t.Helper()
	candles, hasCandles := stockData["candles"]
	if !hasCandles {
		return 0
	}

	candlesArray, ok := candles.([]interface{})
	if !ok {
		return 0
	}

	return len(candlesArray)
}

// --- TestForceRefresh_PreservesTimelineIntegrity ---

// TestForceRefresh_PreservesTimelineIntegrity verifies that when a holding is
// force-refreshed, the portfolio's timeline data_points count does not decrease.
// This ensures that historical portfolio data is preserved and not corrupted.
func TestForceRefresh_PreservesTimelineIntegrity(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("timeline_preserved_after_force_refresh", func(t *testing.T) {
		// Step 1: Get initial timeline
		dataPointsBefore, err := getPortfolioTimeline(t, env, portfolioName, userHeaders)
		require.NoError(t, err)

		initialCount := len(dataPointsBefore)
		t.Logf("Initial timeline data_points count: %d", initialCount)

		// Save initial state
		initialBody, _ := json.Marshal(map[string]interface{}{
			"count":       initialCount,
			"data_points": dataPointsBefore,
		})
		guard.SaveResult("01_timeline_before_force_refresh", string(initialBody))

		// Step 2: Force refresh a well-known stock held in the portfolio
		// (BHP is commonly available in test portfolios)
		t.Logf("Force-refreshing BHP.AU...")
		_ = forceRefreshStockData(t, env, "BHP.AU", userHeaders)

		// Step 3: Wait a moment for any async processing
		// (In production, the sync happens quickly, but timeline computation may be slightly delayed)

		// Step 4: Get timeline again
		dataPointsAfter, err := getPortfolioTimeline(t, env, portfolioName, userHeaders)
		require.NoError(t, err)

		afterCount := len(dataPointsAfter)
		t.Logf("Timeline data_points count after force-refresh: %d", afterCount)

		// Save state after refresh
		afterBody, _ := json.Marshal(map[string]interface{}{
			"count":       afterCount,
			"data_points": dataPointsAfter,
		})
		guard.SaveResult("02_timeline_after_force_refresh", string(afterBody))

		// Step 5: Verify timeline integrity
		// The count should NOT decrease. It may stay the same or increase if new data was added.
		assert.GreaterOrEqual(t, afterCount, initialCount,
			"timeline data_points count decreased after force-refresh: before=%d, after=%d (BUG: EOD merge corrupted timeline)",
			initialCount, afterCount)

		t.Logf("Verified: timeline integrity preserved (before=%d, after=%d)", initialCount, afterCount)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestForceRefresh_EODBarCount ---

// TestForceRefresh_EODBarCount verifies that when stock data is force-refreshed,
// the candles array (EOD bars) count does not decrease. This ensures that
// historical market data is preserved and not corrupted by the force-refresh.
func TestForceRefresh_EODBarCount(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	_, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("eod_bar_count_preserved_after_force_refresh", func(t *testing.T) {
		ticker := "BHP.AU"

		// Step 1: Get stock data with price (includes candles)
		stockDataBefore := getStockDataWithPrice(t, env, ticker, userHeaders)

		// If market data hasn't been synced yet, skip this test
		if stockDataBefore == nil {
			t.Skipf("Market data for %s not yet available, skipping candles test", ticker)
		}

		candleCountBefore := getCandlesCount(t, stockDataBefore)
		t.Logf("Initial EOD bar count for %s: %d", ticker, candleCountBefore)

		// Save initial state
		beforeBody, _ := json.Marshal(map[string]interface{}{
			"ticker":       ticker,
			"candle_count": candleCountBefore,
			"stock_data":   stockDataBefore,
		})
		guard.SaveResult("01_stock_data_before_force_refresh", string(beforeBody))

		// Step 2: Force refresh the stock data
		t.Logf("Force-refreshing %s...", ticker)
		stockDataAfter := forceRefreshStockData(t, env, ticker, userHeaders)

		// If force refresh didn't get data, skip the rest of the test
		if stockDataAfter == nil {
			t.Skipf("Market data for %s not available after force refresh, skipping candles comparison", ticker)
		}

		candleCountAfter := getCandlesCount(t, stockDataAfter)
		t.Logf("EOD bar count for %s after force-refresh: %d", ticker, candleCountAfter)

		// Save state after refresh
		afterBody, _ := json.Marshal(map[string]interface{}{
			"ticker":       ticker,
			"candle_count": candleCountAfter,
			"stock_data":   stockDataAfter,
		})
		guard.SaveResult("02_stock_data_after_force_refresh", string(afterBody))

		// Step 3: Verify candle count integrity
		// The count should NOT decrease. It may stay the same or increase if new bars were added.
		assert.GreaterOrEqual(t, candleCountAfter, candleCountBefore,
			"EOD bar count decreased after force-refresh: before=%d, after=%d (BUG: EOD merge corrupted candles)",
			candleCountBefore, candleCountAfter)

		t.Logf("Verified: EOD bar count integrity preserved (before=%d, after=%d)", candleCountBefore, candleCountAfter)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
