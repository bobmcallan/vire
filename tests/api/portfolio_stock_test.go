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

// TestPortfolioStock_GainPercentage verifies that after syncing a portfolio,
// the get_portfolio_stock endpoint returns simple gain percentages (GainLoss / TotalCost * 100)
// rather than Navexa's IRR p.a. values. This validates the end-to-end path through
// SyncPortfolio -> calculateGainLossFromTrades -> simple % computation -> JSON response.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioStock_GainPercentage(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to populate trade data and compute gain/loss
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

	// Find an active holding with trades for testing
	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 && len(h.Trades) > 0 && h.TotalCost > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding with trades and positive TotalCost")

	// Get the specific holding via get_portfolio_stock
	resp2, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolio+"/stock/"+testTicker, nil, headers)
	require.NoError(t, err, "get portfolio stock request failed")
	defer resp2.Body.Close()

	stockBody, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode, "get portfolio stock failed: %s", string(stockBody))

	guard.SaveResult("02_get_stock_response", string(stockBody))

	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding), "unmarshal holding response")

	// Verify the gain percentages are simple percentages (GainLoss / TotalInvested * 100),
	// using total capital invested as denominator
	t.Run("gain_loss_pct_is_simple", func(t *testing.T) {
		if holding.TotalInvested > 0 {
			expectedPct := (holding.GainLoss / holding.TotalInvested) * 100
			assert.InDelta(t, expectedPct, holding.GainLossPct, 0.01,
				"GainLossPct should be simple percentage (GainLoss/TotalInvested*100)")
		}
	})

	t.Run("capital_gain_pct_equals_gain_loss_pct", func(t *testing.T) {
		assert.Equal(t, holding.GainLossPct, holding.CapitalGainPct,
			"CapitalGainPct should equal GainLossPct")
	})

	t.Run("total_return_pct_includes_dividends", func(t *testing.T) {
		if holding.TotalInvested > 0 {
			expectedTotalReturnPct := (holding.TotalReturnValue / holding.TotalInvested) * 100
			assert.InDelta(t, expectedTotalReturnPct, holding.TotalReturnPct, 0.01,
				"TotalReturnPct should be simple percentage (TotalReturnValue/TotalInvested*100)")
		}
	})

	t.Run("total_return_value_consistency", func(t *testing.T) {
		assert.InDelta(t, holding.GainLoss+holding.DividendReturn, holding.TotalReturnValue, 1.0,
			"TotalReturnValue should equal GainLoss + DividendReturn (within $1)")
	})

	t.Logf("%s: units=%.0f cost=%.2f gl=%.2f glPct=%.2f capPct=%.2f trPct=%.2f",
		holding.Ticker, holding.Units, holding.TotalCost,
		holding.GainLoss, holding.GainLossPct, holding.CapitalGainPct, holding.TotalReturnPct)
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
	if holding.TotalInvested > 0 {
		expectedPct := (holding.GainLoss / holding.TotalInvested) * 100
		assert.InDelta(t, expectedPct, holding.GainLossPct, 0.01,
			"GainLossPct should remain simple percentage after force refresh")
	}

	t.Logf("%s: price=%.2f mv=%.2f glPct=%.2f (force_refresh=true)",
		holding.Ticker, holding.CurrentPrice, holding.MarketValue, holding.GainLossPct)
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

// TestPortfolioStock_BreakevenFieldsOpenPosition verifies that the 7 P&L breakeven
// fields are populated for active (open) positions returned by get_portfolio_stock.
// Fields: net_pnl_if_sold_today, net_return_pct, true_breakeven_price,
// price_target_15pct, stop_loss_5pct, stop_loss_10pct, stop_loss_15pct.
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

	// Get portfolio to find an active holding with trades
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err, "get portfolio request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "get portfolio failed: %s", string(body))

	var got models.Portfolio
	require.NoError(t, json.Unmarshal(body, &got), "unmarshal portfolio response")
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Find an active holding with trades and positive cost
	var testTicker string
	for _, h := range got.Holdings {
		if h.Units > 0 && len(h.Trades) > 0 && h.TotalCost > 0 {
			testTicker = h.Ticker
			break
		}
	}
	require.NotEmpty(t, testTicker, "need at least one active holding with trades and positive TotalCost")

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

	t.Run("all_breakeven_fields_populated", func(t *testing.T) {
		assert.NotNil(t, holding.NetPnlIfSoldToday,
			"net_pnl_if_sold_today should be non-nil for open position")
		assert.NotNil(t, holding.NetReturnPct,
			"net_return_pct should be non-nil for open position")
		assert.NotNil(t, holding.TrueBreakevenPrice,
			"true_breakeven_price should be non-nil for open position")
		assert.NotNil(t, holding.PriceTarget15Pct,
			"price_target_15pct should be non-nil for open position")
		assert.NotNil(t, holding.StopLoss5Pct,
			"stop_loss_5pct should be non-nil for open position")
		assert.NotNil(t, holding.StopLoss10Pct,
			"stop_loss_10pct should be non-nil for open position")
		assert.NotNil(t, holding.StopLoss15Pct,
			"stop_loss_15pct should be non-nil for open position")
	})

	t.Run("net_pnl_equals_realized_plus_unrealized", func(t *testing.T) {
		if holding.NetPnlIfSoldToday == nil {
			t.Skip("net_pnl_if_sold_today is nil")
		}
		expected := holding.RealizedGainLoss + holding.UnrealizedGainLoss
		assert.InDelta(t, expected, *holding.NetPnlIfSoldToday, 0.01,
			"net_pnl_if_sold_today should equal realized_gain_loss + unrealized_gain_loss")
	})

	t.Run("true_breakeven_price_formula", func(t *testing.T) {
		if holding.TrueBreakevenPrice == nil {
			t.Skip("true_breakeven_price is nil")
		}
		require.Greater(t, holding.Units, 0.0, "units must be positive for breakeven calc")
		expected := (holding.TotalCost - holding.RealizedGainLoss) / holding.Units
		assert.InDelta(t, expected, *holding.TrueBreakevenPrice, 0.01,
			"true_breakeven_price should equal (total_cost - realized_gain_loss) / units")
	})

	t.Run("net_return_pct_formula", func(t *testing.T) {
		if holding.NetReturnPct == nil || holding.NetPnlIfSoldToday == nil {
			t.Skip("net_return_pct or net_pnl_if_sold_today is nil")
		}
		if holding.TotalInvested > 0 {
			expected := *holding.NetPnlIfSoldToday / holding.TotalInvested * 100
			assert.InDelta(t, expected, *holding.NetReturnPct, 0.01,
				"net_return_pct should equal net_pnl_if_sold_today / total_invested * 100")
		}
	})

	t.Run("price_target_is_multiple_of_breakeven", func(t *testing.T) {
		if holding.TrueBreakevenPrice == nil || holding.PriceTarget15Pct == nil {
			t.Skip("true_breakeven_price or price_target_15pct is nil")
		}
		expected := *holding.TrueBreakevenPrice * 1.15
		assert.InDelta(t, expected, *holding.PriceTarget15Pct, 0.01,
			"price_target_15pct should equal true_breakeven_price * 1.15")
	})

	t.Run("stop_losses_are_multiples_of_breakeven", func(t *testing.T) {
		if holding.TrueBreakevenPrice == nil {
			t.Skip("true_breakeven_price is nil")
		}
		breakeven := *holding.TrueBreakevenPrice

		if holding.StopLoss5Pct != nil {
			assert.InDelta(t, breakeven*0.95, *holding.StopLoss5Pct, 0.01,
				"stop_loss_5pct should equal true_breakeven_price * 0.95")
		}
		if holding.StopLoss10Pct != nil {
			assert.InDelta(t, breakeven*0.90, *holding.StopLoss10Pct, 0.01,
				"stop_loss_10pct should equal true_breakeven_price * 0.90")
		}
		if holding.StopLoss15Pct != nil {
			assert.InDelta(t, breakeven*0.85, *holding.StopLoss15Pct, 0.01,
				"stop_loss_15pct should equal true_breakeven_price * 0.85")
		}
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

	if holding.TrueBreakevenPrice != nil {
		t.Logf("%s: units=%.0f breakeven=%.4f avgCost=%.4f realized=%.2f unrealized=%.2f netPnl=%.2f",
			holding.Ticker, holding.Units, *holding.TrueBreakevenPrice, holding.AvgCost,
			holding.RealizedGainLoss, holding.UnrealizedGainLoss, *holding.NetPnlIfSoldToday)
	}
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_BreakevenFieldsClosedPosition verifies that the 7 P&L breakeven
// fields are null (JSON null) for closed positions (units=0).
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

	// Use raw JSON map to verify null serialization
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(stockBody, &raw), "unmarshal raw response")

	breakevenFields := []string{
		"net_pnl_if_sold_today",
		"net_return_pct",
		"true_breakeven_price",
		"price_target_15pct",
		"stop_loss_5pct",
		"stop_loss_10pct",
		"stop_loss_15pct",
	}

	for _, field := range breakevenFields {
		t.Run("closed_"+field+"_is_null", func(t *testing.T) {
			val, exists := raw[field]
			if exists {
				assert.Nil(t, val,
					"field %s should be null for closed position (units=0), got %v", field, val)
			}
			// If field doesn't exist in JSON, that's also acceptable (omitempty would remove nil pointers)
		})
	}

	// Also verify via typed struct
	var holding models.Holding
	require.NoError(t, json.Unmarshal(stockBody, &holding))

	t.Run("typed_breakeven_fields_nil", func(t *testing.T) {
		assert.Nil(t, holding.NetPnlIfSoldToday, "net_pnl_if_sold_today should be nil for closed position")
		assert.Nil(t, holding.NetReturnPct, "net_return_pct should be nil for closed position")
		assert.Nil(t, holding.TrueBreakevenPrice, "true_breakeven_price should be nil for closed position")
		assert.Nil(t, holding.PriceTarget15Pct, "price_target_15pct should be nil for closed position")
		assert.Nil(t, holding.StopLoss5Pct, "stop_loss_5pct should be nil for closed position")
		assert.Nil(t, holding.StopLoss10Pct, "stop_loss_10pct should be nil for closed position")
		assert.Nil(t, holding.StopLoss15Pct, "stop_loss_15pct should be nil for closed position")
	})

	t.Logf("%s: units=%.0f (closed) — all breakeven fields should be null/nil",
		closedTicker, holding.Units)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioStock_BreakevenAllHoldings iterates over all holdings in the portfolio
// and validates breakeven field consistency for each one. Open positions must have
// all 7 fields populated with consistent formulas; closed positions must have nil.
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

	// Get full portfolio
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
			if h.Units > 0 {
				// Open position: all breakeven fields should be populated
				assert.NotNil(t, h.NetPnlIfSoldToday,
					"open position should have net_pnl_if_sold_today")
				assert.NotNil(t, h.TrueBreakevenPrice,
					"open position should have true_breakeven_price")
				assert.NotNil(t, h.PriceTarget15Pct,
					"open position should have price_target_15pct")
				assert.NotNil(t, h.StopLoss5Pct,
					"open position should have stop_loss_5pct")
				assert.NotNil(t, h.StopLoss10Pct,
					"open position should have stop_loss_10pct")
				assert.NotNil(t, h.StopLoss15Pct,
					"open position should have stop_loss_15pct")

				// Verify net P&L consistency
				if h.NetPnlIfSoldToday != nil {
					expected := h.RealizedGainLoss + h.UnrealizedGainLoss
					assert.InDelta(t, expected, *h.NetPnlIfSoldToday, 0.01,
						"net_pnl = realized + unrealized")
				}

				// Verify breakeven formula
				if h.TrueBreakevenPrice != nil && h.Units > 0 {
					expected := (h.TotalCost - h.RealizedGainLoss) / h.Units
					assert.InDelta(t, expected, *h.TrueBreakevenPrice, 0.01,
						"breakeven = (total_cost - realized) / units")
				}

				// Verify no NaN/Inf
				if h.TrueBreakevenPrice != nil {
					assert.False(t, math.IsNaN(*h.TrueBreakevenPrice), "breakeven should not be NaN")
					assert.False(t, math.IsInf(*h.TrueBreakevenPrice, 0), "breakeven should not be Inf")
				}

				// Log for diagnostics
				if h.TrueBreakevenPrice != nil {
					t.Logf("units=%.0f avgCost=%.4f breakeven=%.4f realized=%.2f",
						h.Units, h.AvgCost, *h.TrueBreakevenPrice, h.RealizedGainLoss)
				}
			} else {
				// Closed position: all breakeven fields should be nil
				assert.Nil(t, h.NetPnlIfSoldToday, "closed position should have nil net_pnl")
				assert.Nil(t, h.NetReturnPct, "closed position should have nil net_return_pct")
				assert.Nil(t, h.TrueBreakevenPrice, "closed position should have nil breakeven")
				assert.Nil(t, h.PriceTarget15Pct, "closed position should have nil price_target")
				assert.Nil(t, h.StopLoss5Pct, "closed position should have nil stop_loss_5")
				assert.Nil(t, h.StopLoss10Pct, "closed position should have nil stop_loss_10")
				assert.Nil(t, h.StopLoss15Pct, "closed position should have nil stop_loss_15")

				t.Logf("units=0 (closed) — breakeven fields nil")
			}
		})
	}

	t.Logf("Validated breakeven fields for %d holdings", len(got.Holdings))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}
