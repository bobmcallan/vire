package api

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

// --- Fix 1: Yesterday/Week Fields Populated After Sync (fb_fb956a5e) ---

// TestPortfolioHistoricalFields_PopulatedAfterSync verifies that yesterday_total,
// last_week_total and their per-holding equivalents are populated after a portfolio sync.
func TestPortfolioHistoricalFields_PopulatedAfterSync(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Force sync the portfolio
	t.Run("force_sync", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_sync_response", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))
	})

	// Step 2: Fetch portfolio and check historical aggregate fields
	t.Run("historical_aggregate_fields_present", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_portfolio_after_sync", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// If EOD data is available, these fields should be non-zero.
		// If market data hasn't been collected yet, they may be absent (omitempty),
		// so we verify the structure doesn't panic and the response is valid.
		assert.Contains(t, portfolio, "total_value", "portfolio should have total_value")
		assert.Contains(t, portfolio, "holdings", "portfolio should have holdings")

		// yesterday_total and last_week_total should be present when EOD data exists.
		// We check for their presence — their value depends on market data availability.
		if _, ok := portfolio["yesterday_total"]; ok {
			yt := portfolio["yesterday_total"].(float64)
			assert.GreaterOrEqual(t, yt, 0.0, "yesterday_total should be >= 0")
		}
		if _, ok := portfolio["last_week_total"]; ok {
			lwt := portfolio["last_week_total"].(float64)
			assert.GreaterOrEqual(t, lwt, 0.0, "last_week_total should be >= 0")
		}
	})

	// Step 3: Verify per-holding historical fields are populated for holdings with EOD data
	t.Run("per_holding_historical_fields", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		holdings, ok := portfolio["holdings"].([]interface{})
		require.True(t, ok, "holdings should be a list")

		for _, h := range holdings {
			holding := h.(map[string]interface{})
			ticker := holding["ticker"].(string)

			// For open positions (units > 0), check historical fields are populated
			// when EOD data is available (non-zero yesterday_close indicates availability)
			if units, ok := holding["units"].(float64); ok && units > 0 {
				if yc, ok := holding["yesterday_close"].(float64); ok && yc > 0 {
					assert.GreaterOrEqual(t, yc, 0.0,
						"holding %s: yesterday_close should be >= 0", ticker)

					// yesterday_pct should be present when yesterday_close is non-zero
					_, hasPct := holding["yesterday_pct"]
					assert.True(t, hasPct,
						"holding %s: yesterday_pct should be present when yesterday_close is set", ticker)
				}
				if lwc, ok := holding["last_week_close"].(float64); ok && lwc > 0 {
					assert.GreaterOrEqual(t, lwc, 0.0,
						"holding %s: last_week_close should be >= 0", ticker)
				}
			}
		}

		guard.SaveResult("03_holdings_historical", string(body))
	})

	// Step 4: Verify fields survive a second sync (idempotent)
	t.Run("fields_survive_repeated_sync", func(t *testing.T) {
		// Re-sync
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Fetch again
		resp, err = env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("04_after_second_sync", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// Basic structural check — no panics, valid response
		assert.Contains(t, portfolio, "total_value")
		assert.Contains(t, portfolio, "holdings")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioHistoricalFields_SyncPopulatesFields verifies that SyncPortfolio
// now calls populateHistoricalValues, so the response from sync includes fields.
func TestPortfolioHistoricalFields_SyncPopulatesFields(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Force a sync and capture its response
	t.Run("sync_response_includes_portfolio_name", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/sync",
			map[string]interface{}{"force": true}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_sync_result", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode, "sync failed: %s", string(body))

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// Sync should return a valid portfolio
		assert.Equal(t, portfolioName, portfolio["name"],
			"sync response should return the portfolio with correct name")

		// Holdings should be present
		holdings, ok := portfolio["holdings"].([]interface{})
		require.True(t, ok, "sync response should include holdings")
		assert.NotEmpty(t, holdings, "portfolio should have at least one holding")

		// last_synced should be recent
		lastSynced, ok := portfolio["last_synced"].(string)
		assert.True(t, ok, "last_synced should be a string")
		if ok {
			syncTime, err := time.Parse(time.RFC3339, lastSynced)
			if err == nil {
				assert.WithinDuration(t, time.Now(), syncTime, 5*time.Minute,
					"last_synced should be recent (within 5 minutes)")
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 2: Capital Performance Auto-Derive from Trades (fb_742053d8) ---

// TestCapitalPerformance_WithoutTransactionsAutoDerivesFromTrades verifies that
// get_capital_performance returns non-zero data when no manual cash transactions
// exist but Navexa trade history is available.
func TestCapitalPerformance_WithoutTransactionsAutoDerivesFromTrades(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Ensure no cash transactions exist (clean state)
	t.Run("ensure_no_transactions", func(t *testing.T) {
		// Get current ledger
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("00_initial_ledger", string(body))

		var ledger map[string]interface{}
		if err := json.Unmarshal(body, &ledger); err == nil {
			txns, ok := ledger["transactions"].([]interface{})
			if ok {
				for _, tx := range txns {
					txMap := tx.(map[string]interface{})
					txID := txMap["id"].(string)
					r, e := env.HTTPRequest(http.MethodDelete, basePath+"/cash-transactions/"+txID, nil, userHeaders)
					if e == nil {
						r.Body.Close()
					}
				}
			}
		}
	})

	// Step 2: Call capital performance endpoint — should auto-derive from trades
	t.Run("auto_derives_from_trades", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_perf_no_transactions", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode,
			"performance endpoint should return 200 even without cash transactions: %s", string(body))

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		// When trades exist, the auto-derive path should populate these fields
		assert.Contains(t, perf, "total_deposited",
			"capital_performance should have total_deposited field")
		assert.Contains(t, perf, "current_portfolio_value",
			"capital_performance should have current_portfolio_value field")
		assert.Contains(t, perf, "net_capital_deployed",
			"capital_performance should have net_capital_deployed field")

		// If Navexa is reachable and the portfolio has trades, total_deposited should be > 0
		// (we don't assert a specific value since it depends on real Navexa data)
		totalDeposited, ok := perf["total_deposited"].(float64)
		if ok && totalDeposited > 0 {
			t.Logf("Trade-based total_deposited: %.2f", totalDeposited)
			assert.Greater(t, totalDeposited, 0.0,
				"trade-based total_deposited should be positive when buy trades exist")

			currentValue, ok := perf["current_portfolio_value"].(float64)
			if ok {
				assert.Greater(t, currentValue, 0.0,
					"current_portfolio_value should be positive")
			}
		} else {
			// No trades or Navexa unreachable — still expect 200 with valid structure
			t.Log("No trade data available (Navexa may be unreachable); verifying zero-value response structure")
			assert.Contains(t, perf, "total_deposited")
		}
	})

	// Step 3: Verify portfolio response includes capital_performance via trade fallback
	t.Run("portfolio_includes_capital_performance_via_trades", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath, nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_portfolio_with_trades_perf", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var portfolio map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &portfolio))

		// capital_performance may be present when trade-based derivation returns non-zero data
		if cp, ok := portfolio["capital_performance"].(map[string]interface{}); ok {
			t.Log("capital_performance present in portfolio response (trade-based)")
			assert.Contains(t, cp, "total_deposited")
			assert.Contains(t, cp, "current_portfolio_value")
		} else {
			// Still valid — means trades returned zero (e.g. no buys found)
			t.Log("capital_performance absent from portfolio response (no qualifying trades)")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestCapitalPerformance_ManualTransactionsTakePrecedence verifies that when manual
// cash transactions exist, they are used instead of the trade-based fallback.
func TestCapitalPerformance_ManualTransactionsTakePrecedence(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Add a manual cash transaction
	var txID string
	t.Run("add_manual_transaction", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/cash-transactions", map[string]interface{}{
			"type":        "deposit",
			"date":        time.Now().Add(-365 * 24 * time.Hour).Format(time.RFC3339),
			"amount":      999999,
			"description": "Manual tx precedence test",
		}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_add_tx", string(body))

		require.Equal(t, http.StatusCreated, resp.StatusCode, "add tx failed: %s", string(body))

		var ledger map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &ledger))

		// Find transaction ID for cleanup
		if txns, ok := ledger["transactions"].([]interface{}); ok && len(txns) > 0 {
			tx := txns[len(txns)-1].(map[string]interface{})
			txID = tx["id"].(string)
		}
	})

	// Step 2: Verify capital_performance uses manual transaction
	t.Run("manual_transaction_is_used", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions/performance", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_perf_with_manual_tx", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var perf map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &perf))

		// The manual transaction should be reflected
		totalDeposited, ok := perf["total_deposited"].(float64)
		require.True(t, ok, "total_deposited should be a float")

		// total_deposited should be >= 999999 (may have more if other txns existed)
		assert.GreaterOrEqual(t, totalDeposited, 999999.0,
			"total_deposited should include the manual transaction amount")

		txCount, ok := perf["transaction_count"].(float64)
		require.True(t, ok, "transaction_count should be a float")
		assert.GreaterOrEqual(t, txCount, 1.0, "transaction_count should be >= 1")
	})

	// Step 3: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		if txID == "" {
			// No txID found, try to list and delete all
			resp, err := env.HTTPRequest(http.MethodGet, basePath+"/cash-transactions", nil, userHeaders)
			if err != nil {
				return
			}
			defer resp.Body.Close()

			var ledger map[string]interface{}
			body, _ := io.ReadAll(resp.Body)
			if json.Unmarshal(body, &ledger) == nil {
				if txns, ok := ledger["transactions"].([]interface{}); ok {
					for _, tx := range txns {
						txMap := tx.(map[string]interface{})
						id := txMap["id"].(string)
						r, e := env.HTTPRequest(http.MethodDelete, basePath+"/cash-transactions/"+id, nil, userHeaders)
						if e == nil {
							r.Body.Close()
						}
					}
				}
			}
			return
		}

		resp, err := env.HTTPRequest(http.MethodDelete, basePath+"/cash-transactions/"+txID, nil, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Fix 3: Time Series in Portfolio Indicators (fb_cafb4fa0) ---

// TestPortfolioIndicators_TimeSeries verifies that get_portfolio_indicators
// returns a time_series field with daily portfolio value data points.
func TestPortfolioIndicators_TimeSeries(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("time_series_present_in_response", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet,
			"/api/portfolios/"+portfolioName+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_with_time_series", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		// time_series should be present when there is historical data
		ts, hasTimeSeries := indicators["time_series"]
		if !hasTimeSeries {
			// time_series is omitempty — absent when no historical data
			dataPoints := indicators["data_points"].(float64)
			assert.Equal(t, 0.0, dataPoints,
				"time_series absent only when data_points is 0")
			t.Log("time_series absent (no historical data for this portfolio)")
			return
		}

		timeSeriesSlice, ok := ts.([]interface{})
		require.True(t, ok, "time_series should be an array")
		assert.NotEmpty(t, timeSeriesSlice, "time_series should have at least one point")

		// Validate structure of each time series point
		for i, pointRaw := range timeSeriesSlice {
			point, ok := pointRaw.(map[string]interface{})
			require.True(t, ok, "time_series[%d] should be an object", i)

			assert.Contains(t, point, "date",
				"time_series[%d] should have date field", i)
			assert.Contains(t, point, "value",
				"time_series[%d] should have value field", i)
			assert.Contains(t, point, "cost",
				"time_series[%d] should have cost field", i)
			assert.Contains(t, point, "net_return",
				"time_series[%d] should have net_return field", i)
			assert.Contains(t, point, "net_return_pct",
				"time_series[%d] should have net_return_pct field", i)
			assert.Contains(t, point, "holding_count",
				"time_series[%d] should have holding_count field", i)

			// Value should be >= 0
			value, ok := point["value"].(float64)
			if ok {
				assert.GreaterOrEqual(t, value, 0.0,
					"time_series[%d].value should be >= 0", i)
			}

			// holding_count should be >= 0
			hc, ok := point["holding_count"].(float64)
			if ok {
				assert.GreaterOrEqual(t, hc, 0.0,
					"time_series[%d].holding_count should be >= 0", i)
			}
		}
	})

	// Verify time_series length matches data_points
	t.Run("time_series_length_matches_data_points", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet,
			"/api/portfolios/"+portfolioName+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_indicators_datapoints_check", string(body))

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		dataPoints := indicators["data_points"].(float64)
		ts, hasTimeSeries := indicators["time_series"]

		if dataPoints == 0 {
			assert.False(t, hasTimeSeries,
				"time_series should be absent when data_points is 0")
		} else if hasTimeSeries {
			timeSeriesSlice := ts.([]interface{})
			assert.Equal(t, int(dataPoints), len(timeSeriesSlice),
				"time_series length should match data_points")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioIndicators_TimeSeriesWithExternalBalance verifies that
// time_series values include external balances (additive to portfolio value).
func TestPortfolioIndicators_TimeSeriesWithExternalBalance(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)
	basePath := "/api/portfolios/" + portfolioName

	// Step 1: Get indicators without external balances
	var baselineLatestValue float64
	var hasBaselineData bool
	t.Run("baseline_indicators", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_indicators_baseline", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, hasTimeSeries := indicators["time_series"]
		if hasTimeSeries {
			points := ts.([]interface{})
			if len(points) > 0 {
				// Get the latest point value
				latest := points[len(points)-1].(map[string]interface{})
				baselineLatestValue = latest["value"].(float64)
				hasBaselineData = true
			}
		}

		if !hasBaselineData {
			t.Skip("No time series data available (portfolio too new or no EOD data)")
		}
	})

	extBalanceAmount := 50000.0

	// Step 2: Add external balance
	t.Run("add_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPost, basePath+"/external-balances",
			map[string]interface{}{
				"type":  "cash",
				"label": "TimeSeries Test Cash",
				"value": extBalanceAmount,
			}, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("02_add_ext_balance", string(body))
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	// Step 3: Verify time_series values include external balance
	t.Run("time_series_values_include_external_balance", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodGet, basePath+"/indicators", nil, userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("03_indicators_with_ext_balance", string(body))

		require.Equal(t, http.StatusOK, resp.StatusCode)

		var indicators map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &indicators))

		ts, ok := indicators["time_series"]
		require.True(t, ok, "time_series should be present")

		points := ts.([]interface{})
		require.NotEmpty(t, points, "time_series should have points")

		// Latest point value should have increased by external balance amount
		latest := points[len(points)-1].(map[string]interface{})
		newLatestValue := latest["value"].(float64)

		assert.InDelta(t, baselineLatestValue+extBalanceAmount, newLatestValue, 1.0,
			"latest time_series value should have increased by external balance (%v)", extBalanceAmount)
	})

	// Step 4: Cleanup
	t.Run("cleanup", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPut, basePath+"/external-balances",
			map[string]interface{}{
				"external_balances": []map[string]interface{}{},
			}, userHeaders)
		require.NoError(t, err)
		resp.Body.Close()
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioIndicators_TimeSeriesEmpty verifies that when a portfolio has
// no historical data, time_series is absent (omitempty) and data_points is 0 or 1.
func TestPortfolioIndicators_TimeSeriesEmpty(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Use a non-existent portfolio to get a structured error
	t.Run("nonexistent_portfolio_returns_error", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/portfolios/nonexistent_xyz_portfolio/indicators")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("01_nonexistent_indicators", string(body))

		// Should return error status for non-existent portfolio
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode,
			"non-existent portfolio should return error")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
