package api

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/tests/common"
)

// TestPortfolioStock_NewFieldNames verifies that after syncing a portfolio,
// the get_portfolio_stock endpoint returns the new JSON field names
// (net_return, net_return_pct, realized_return, unrealized_return,
// annualized_total_return_pct, time_weighted_return_pct) and does NOT return old/removed fields.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_NewFieldNames(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate trade data and compute returns
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio to find a holding with trades
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Find an active holding for testing
	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 && h.CostBasis > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding with positive CostBasis")

	// Get the specific holding via get_portfolio_stock
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker, nil, headers)
	require.NoError(t, err, "get portfolio stock request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "get portfolio stock failed: %s", string(stockBody))

	guard.SaveResult("02_get_stock_response", string(stockBody))

	// Verify field names in raw JSON
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(stockBody, &raw), "unmarshal raw response")

	t.Run("new_fields_present", func(t *testing.T) {
		assert.Contains(t, raw, "net_return", "should have net_return field")
		assert.Contains(t, raw, "net_return_pct", "should have net_return_pct field")
		assert.Contains(t, raw, "realized_return", "should have realized_return field")
		assert.Contains(t, raw, "unrealized_return", "should have unrealized_return field")
		assert.Contains(t, raw, "annualized_total_return_pct", "should have annualized_total_return_pct field")
		assert.Contains(t, raw, "time_weighted_return_pct", "should have time_weighted_return_pct field")
	})

	t.Run("removed_fields_absent", func(t *testing.T) {
		// These fields were removed in the portfolio refactoring
		assert.NotContains(t, raw, "total_return_value", "total_return_value should be removed")
		assert.NotContains(t, raw, "total_return_pct", "total_return_pct should be removed")
		assert.NotContains(t, raw, "net_pnl_if_sold_today", "net_pnl_if_sold_today should be removed")
		assert.NotContains(t, raw, "price_target_15pct", "price_target_15pct should be removed")
		assert.NotContains(t, raw, "stop_loss_5pct", "stop_loss_5pct should be removed")
		assert.NotContains(t, raw, "stop_loss_10pct", "stop_loss_10pct should be removed")
		assert.NotContains(t, raw, "stop_loss_15pct", "stop_loss_15pct should be removed")

		// Old field names should not be present
		assert.NotContains(t, raw, "gain_loss", "gain_loss should be renamed to net_return")
		assert.NotContains(t, raw, "gain_loss_pct", "gain_loss_pct should be renamed to net_return_pct")
		assert.NotContains(t, raw, "realized_gain_loss", "realized_gain_loss should be renamed to realized_return")
		assert.NotContains(t, raw, "unrealized_gain_loss", "unrealized_gain_loss should be renamed to unrealized_return")
		assert.NotContains(t, raw, "total_return_pct_irr", "total_return_pct_irr should be renamed to annualized_total_return_pct")
		assert.NotContains(t, raw, "total_return_pct_twrr", "total_return_pct_twrr should be renamed to time_weighted_return_pct")
	})

	// Verify via typed struct
	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding), "unmarshal holding response")

	t.Run("net_return_pct_is_simple", func(t *testing.T) {
		if holding.GrossInvested > 0 {
			expectedPct := (holding.NetReturn / holding.GrossInvested) * 100
			assert.InDelta(t, expectedPct, holding.NetReturnPct, 0.01,
				"NetReturnPct should be simple percentage (NetReturn/GrossInvested*100)")
		}
	})

	t.Run("stock_includes_trades", func(t *testing.T) {
		assert.NotEmpty(t, holding.Trades,
			"individual stock GET should include trades")
	})

	t.Logf("%s: units=%.0f cost=%.2f nr=%.2f nrPct=%.2f realNr=%.2f unrealNr=%.2f",
		holding.Ticker, holding.Units, holding.CostBasis,
		holding.NetReturn, holding.NetReturnPct, holding.RealizedReturn, holding.UnrealizedReturn)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_TradesStripping verifies that portfolio GET does NOT include
// trades in holdings, but individual stock GET DOES include trades.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_TradesStripping(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio (should have trades stripped)
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	guard.SaveResult("02_get_portfolio_response", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Verify NO holding in portfolio GET has trades
	t.Run("portfolio_get_no_trades", func(t *testing.T) {
		for _, h := range got.Holdings {
			assert.Empty(t, h.Trades,
				"holding %s in portfolio GET should not have trades", h.Ticker)
		}
	})

	// Also verify via raw JSON that trades key is absent or null/empty
	t.Run("portfolio_get_no_trades_in_json", func(t *testing.T) {
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &raw))

		holdings, ok := raw["holdings"].([]interface{})
		require.True(t, ok, "holdings should be an array")

		for i, hRaw := range holdings {
			hMap, ok := hRaw.(map[string]interface{})
			require.True(t, ok, "holding[%d] should be a map", i)

			trades, exists := hMap["trades"]
			if exists {
				assert.Nil(t, trades,
					"holding[%d] trades should be null/absent in portfolio GET", i)
			}
		}
	})

	// Find an active holding and verify trades ARE present in stock GET
	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding")

	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker, nil, headers)
	require.NoError(t, err, "get stock request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "get stock failed: %s", string(stockBody))

	guard.SaveResult("03_get_stock_response", string(stockBody))

	var stock models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &stock))

	t.Run("stock_get_has_trades", func(t *testing.T) {
		assert.NotEmpty(t, stock.Trades,
			"individual stock GET for active holding should include trades")

		for j, tr := range stock.Trades {
			assert.NotEmpty(t, tr.Type, "trade[%d] should have a Type", j)
			assert.Greater(t, tr.Units, 0.0, "trade[%d] should have positive Units", j)
			assert.Greater(t, tr.Price, 0.0, "trade[%d] should have positive Price", j)
		}
	})

	t.Logf("%s: trades=%d in stock GET, 0 in portfolio GET", testTicker, len(stock.Trades))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_BreakevenFieldsOpenPosition verifies that the true_breakeven_price
// field is populated for active (open) positions returned by get_portfolio_stock.
// The removed fields (net_pnl_if_sold_today, price_target_15pct, stop losses) must NOT appear.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_BreakevenFieldsOpenPosition(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate trade data
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio to find an active holding
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Find an active holding with positive cost
	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 && h.CostBasis > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding with positive CostBasis")

	// Get the specific holding via get_portfolio_stock
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker, nil, headers)
	require.NoError(t, err, "get portfolio stock request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "get portfolio stock failed: %s", string(stockBody))

	guard.SaveResult("02_breakeven_open_response", string(stockBody))

	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding), "unmarshal holding response")

	t.Run("breakeven_populated", func(t *testing.T) {
		assert.NotNil(t, holding.TrueBreakevenPrice,
			"true_breakeven_price should be non-nil for open position")
	})

	t.Run("true_breakeven_price_formula", func(t *testing.T) {
		if holding.TrueBreakevenPrice == nil {
			t.Skip("true_breakeven_price is nil")
		}
		require.Greater(t, holding.Units, 0.0, "units must be positive for breakeven calc")
		expected := (holding.CostBasis - holding.RealizedReturn) / holding.Units
		assert.InDelta(t, expected, *holding.TrueBreakevenPrice, 0.01,
			"true_breakeven_price should equal (total_cost - realized_return) / units")
	})

	t.Run("breakeven_is_positive", func(t *testing.T) {
		if holding.TrueBreakevenPrice == nil {
			t.Skip("true_breakeven_price is nil")
		}
		assert.Greater(t, *holding.TrueBreakevenPrice, 0.0,
			"true_breakeven_price should be positive for an active holding")
		assert.False(t, math.IsNaN(*holding.TrueBreakevenPrice),
			"true_breakeven_price should not be NaN")
		assert.False(t, math.IsInf(*holding.TrueBreakevenPrice, 0),
			"true_breakeven_price should not be Inf")
	})

	t.Run("removed_breakeven_fields_absent", func(t *testing.T) {
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(stockBody, &raw))

		assert.NotContains(t, raw, "net_pnl_if_sold_today", "removed field should not appear")
		assert.NotContains(t, raw, "price_target_15pct", "removed field should not appear")
		assert.NotContains(t, raw, "stop_loss_5pct", "removed field should not appear")
		assert.NotContains(t, raw, "stop_loss_10pct", "removed field should not appear")
		assert.NotContains(t, raw, "stop_loss_15pct", "removed field should not appear")
	})

	if holding.TrueBreakevenPrice != nil {
		t.Logf("%s: units=%.0f breakeven=%.4f avgCost=%.4f realizedNR=%.2f unrealizedNR=%.2f",
			holding.Ticker, holding.Units, *holding.TrueBreakevenPrice, holding.AvgCost,
			holding.RealizedReturn, holding.UnrealizedReturn)
	}
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_BreakevenFieldsClosedPosition verifies that the true_breakeven_price
// is null for closed positions (units=0).
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_BreakevenFieldsClosedPosition(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio to find a closed holding (units=0)
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Find a closed position
	var closedTicker string
	for _, h := range got.Holdings {
		if h.Units == 0 {
			closedTicker = h.Ticker
			break
		}
	}
	if closedTicker == "" {
		t.Skip("no closed positions (units=0) in portfolio — cannot test null breakeven fields")
	}

	// Get the closed holding via get_portfolio_stock
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+closedTicker, nil, headers)
	require.NoError(t, err, "get portfolio stock request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "get portfolio stock failed: %s", string(stockBody))

	guard.SaveResult("02_breakeven_closed_response", string(stockBody))

	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding))

	t.Run("closed_breakeven_nil", func(t *testing.T) {
		assert.Nil(t, holding.TrueBreakevenPrice,
			"true_breakeven_price should be nil for closed position")
	})

	t.Logf("%s: units=%.0f (closed) — breakeven should be nil",
		closedTicker, holding.Units)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_BreakevenAllHoldings iterates over all holdings in the portfolio
// and validates breakeven field consistency for each one. Open positions must have
// true_breakeven_price populated; closed positions must have nil.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_BreakevenAllHoldings(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get full portfolio — note: trades are stripped in portfolio GET,
	// so we use individual stock GET for trade-dependent assertions
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	guard.SaveResult("02_get_portfolio_response", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	for _, h := range got.Holdings {
		t.Run(h.Ticker, func(t *testing.T) {
			// Get individual stock for full data including trades
			resp2, err := env.HTTPRequest(http.MethodGet,
				"/api/portfolios/"+portfolio+"/stock/"+h.Ticker, nil, headers)
			require.NoError(t, err, "get stock request failed for %s", h.Ticker)
			defer resp2.Body.Close()

			stockBody, err := io.ReadAll(resp2.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp2.StatusCode)

			var stock models.Holding
			require.NoError(t, json.Unmarshal(stockBody, &stock))

			if stock.Units > 0 {
				// Open position: breakeven should be populated
				assert.NotNil(t, stock.TrueBreakevenPrice,
					"open position should have true_breakeven_price")

				// Verify breakeven formula
				if stock.TrueBreakevenPrice != nil && stock.Units > 0 {
					expected := (stock.CostBasis - stock.RealizedReturn) / stock.Units
					assert.InDelta(t, expected, *stock.TrueBreakevenPrice, 0.01,
						"breakeven = (total_cost - realized_return) / units")
				}

				// Verify no NaN/Inf
				if stock.TrueBreakevenPrice != nil {
					assert.False(t, math.IsNaN(*stock.TrueBreakevenPrice), "breakeven should not be NaN")
					assert.False(t, math.IsInf(*stock.TrueBreakevenPrice, 0), "breakeven should not be Inf")
				}

				if stock.TrueBreakevenPrice != nil {
					t.Logf("units=%.0f avgCost=%.4f breakeven=%.4f realizedNR=%.2f",
						stock.Units, stock.AvgCost, *stock.TrueBreakevenPrice, stock.RealizedReturn)
				}
			} else {
				// Closed position: breakeven should be nil
				assert.Nil(t, stock.TrueBreakevenPrice,
					"closed position should have nil breakeven")

				t.Logf("units=0 (closed) — breakeven nil")
			}
		})
	}

	t.Logf("Validated breakeven fields for %d holdings", len(got.Holdings))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_ForceRefresh verifies that calling get_portfolio_stock with
// force_refresh=true triggers a fresh sync and returns updated data.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_ForceRefresh(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Initial sync to populate the portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio to find an active holding
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got))
	require.NotEmpty(t, got.Holdings)

	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding")

	// Call get_portfolio_stock with force_refresh=true
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker+"?force_refresh=true",
		nil, headers)
	require.NoError(t, err, "force_refresh request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)

	guard.SaveResult("02_force_refresh_response", string(stockBody))

	assert.Equal(t, http.StatusOK, resp2.StatusCode,
		"force_refresh should return 200: %s", string(stockBody))

	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding))

	// Verify the holding has valid data after force refresh
	assert.Equal(t, testTicker, holding.Ticker)
	assert.Greater(t, holding.Units, 0.0, "active holding should have positive units")
	assert.Greater(t, holding.CurrentPrice, 0.0, "holding should have a current price after refresh")
	assert.Greater(t, holding.MarketValue, 0.0, "holding should have market value after refresh")

	// Verify gain percentages are still simple percentages after force refresh
	if holding.GrossInvested > 0 {
		expectedPct := (holding.NetReturn / holding.GrossInvested) * 100
		assert.InDelta(t, expectedPct, holding.NetReturnPct, 0.01,
			"NetReturnPct should remain simple percentage after force refresh")
	}

	t.Logf("%s: price=%.2f mv=%.2f nrPct=%.2f (force_refresh=true)",
		holding.Ticker, holding.CurrentPrice, holding.MarketValue, holding.NetReturnPct)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_ForceRefreshNoNavexa verifies that calling get_portfolio_stock
// with force_refresh=true but without Navexa credentials returns a graceful error
// rather than crashing.
func TestPortfolioStock_ForceRefreshNoNavexa(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire-service-blank.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Call force_refresh without any user or Navexa configuration
	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/SMSF/stock/BHP?force_refresh=true",
		nil, map[string]string{"X-Vire-User-ID": "nonexistent"})
	require.NoError(t, err, "request should not fail at HTTP level")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	guard.SaveResult("01_force_refresh_no_navexa", string(body))

	// Should return an error status (not 200) since there's no portfolio data
	assert.NotEqual(t, http.StatusOK, resp.StatusCode,
		"force_refresh without Navexa should not succeed")

	// Response should be valid JSON (graceful error, not a crash)
	assert.True(t, json.Valid(body),
		"error response should be valid JSON: %s", string(body))

	t.Logf("Expected error without Navexa: status=%d body=%s",
		resp.StatusCode, string(body))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_PortfolioTotals verifies the portfolio-level totals
// include total_realized_return and total_unrealized_return.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_PortfolioTotals(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get portfolio
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	guard.SaveResult("02_get_portfolio_response", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")

	t.Run("total_net_return_populated", func(t *testing.T) {
		// TotalNetReturn should be a real number (not zero unless portfolio is empty)
		if len(got.Holdings) > 0 {
			t.Logf("TotalNetReturn=%.2f TotalNetReturnPct=%.2f", got.NetEquityReturn, got.NetEquityReturnPct)
		}
	})

	t.Run("realized_unrealized_breakdown", func(t *testing.T) {
		t.Logf("TotalRealizedNetReturn=%.2f TotalUnrealizedNetReturn=%.2f",
			got.RealizedEquityReturn, got.UnrealizedEquityReturn)

		// Verify no NaN/Inf in totals
		assert.False(t, math.IsNaN(got.RealizedEquityReturn), "TotalRealizedNetReturn should not be NaN")
		assert.False(t, math.IsNaN(got.UnrealizedEquityReturn), "TotalUnrealizedNetReturn should not be NaN")
		assert.False(t, math.IsInf(got.RealizedEquityReturn, 0), "TotalRealizedNetReturn should not be Inf")
		assert.False(t, math.IsInf(got.UnrealizedEquityReturn, 0), "TotalUnrealizedNetReturn should not be Inf")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
