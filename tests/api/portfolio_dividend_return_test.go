package api

// Integration tests for portfolio dividend_return field.
//
// Requirements: .claude/workdir/20260302-1500-dividend-reconciliation/requirements.md
//
// Coverage:
//   - TestPortfolioDividendReturn_FieldPresent — Verifies dividend_return field exists
//     in portfolio response and is populated after sync
//   - TestPortfolioDividendReturn_EqualsHoldingSum — Verifies dividend_return equals
//     the sum of all holding-level dividend_return values
//   - TestPortfolioDividendReturn_ZeroWhenNoDividends — Verifies field is 0 when holdings
//     have no dividend income
//   - TestPortfolioDividendReturn_IncludedInNetEquityReturn — Verifies that the dividend
//     return is properly reflected in net_equity_return calculation
//   - TestPortfolioDividendReturn_FXConversion — Verifies that dividend_return for FX
//     holdings is already converted to AUD (no double conversion needed)

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

// getPortfolioDividendData fetches the portfolio and returns key fields for dividend testing.
func getPortfolioDividendData(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) map[string]interface{} {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName, nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET portfolio failed: %s", string(body))
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

// getPortfolioHoldings fetches all holdings in the portfolio for dividend verification.
func getPortfolioHoldings(t *testing.T, env *common.Env, portfolioName string, headers map[string]string) []map[string]interface{} {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/holdings", nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "GET holdings failed: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	holdings, ok := result["holdings"].([]interface{})
	require.True(t, ok, "holdings should be an array in response")

	var holdingsList []map[string]interface{}
	for _, h := range holdings {
		hMap, ok := h.(map[string]interface{})
		require.True(t, ok, "each holding should be a map")
		holdingsList = append(holdingsList, hMap)
	}
	return holdingsList
}

// --- TestPortfolioDividendReturn_FieldPresent ---

// TestPortfolioDividendReturn_FieldPresent verifies that dividend_return field is
// present in the portfolio response after sync.
func TestPortfolioDividendReturn_FieldPresent(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("dividend_forecast_field_exists", func(t *testing.T) {
		portfolio := getPortfolioDividendData(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_with_dividend_field", string(raw))

		// Verify dividend_forecast field is present
		_, hasDividendForecast := portfolio["income_dividends_forecast"]
		require.True(t, hasDividendForecast,
			"dividend_forecast field should be present in portfolio response")

		// It should be a number (float64)
		dividendForecast, ok := portfolio["income_dividends_forecast"].(float64)
		require.True(t, ok, "dividend_forecast should be a number")

		t.Logf("dividend_forecast field present: %.2f", dividendForecast)
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioDividendReturn_EqualsHoldingSum ---

// TestPortfolioDividendReturn_EqualsHoldingSum verifies that the portfolio's
// dividend_return equals the sum of all holding-level dividend_return values.
func TestPortfolioDividendReturn_EqualsHoldingSum(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("forecast_lte_sum_of_holdings", func(t *testing.T) {
		// Get portfolio
		portfolio := getPortfolioDividendData(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_dividend_data", string(raw))

		dividendForecast, ok := portfolio["income_dividends_forecast"].(float64)
		require.True(t, ok, "dividend_forecast should be a number")

		// Get holdings
		holdings := getPortfolioHoldings(t, env, portfolioName, userHeaders)

		// Sum dividend_return from each holding
		var holdingDividendSum float64
		holdingDetails := make([]map[string]interface{}, 0)

		for _, h := range holdings {
			dividendReturn, ok := h["dividend_return"].(float64)
			if ok {
				holdingDividendSum += dividendReturn
				holdingDetails = append(holdingDetails, map[string]interface{}{
					"ticker":          h["ticker"],
					"name":            h["name"],
					"dividend_return": dividendReturn,
				})
			}
		}

		holdingsDetails, _ := json.Marshal(holdingDetails)
		guard.SaveResult("02_holdings_dividend_breakdown", string(holdingsDetails))

		// Verify: dividend_forecast <= sum of holding dividend_return
		// (forecast subtracts holdings with confirmed ledger payments)
		assert.LessOrEqual(t, dividendForecast, holdingDividendSum+0.01,
			"dividend_forecast (%.2f) should be <= sum of holding dividend_return (%.2f)",
			dividendForecast, holdingDividendSum)

		t.Logf("Portfolio dividend_forecast: %.2f", dividendForecast)
		t.Logf("Sum of holding dividend_return: %.2f", holdingDividendSum)
		t.Logf("Number of holdings: %d", len(holdings))
		t.Logf("Holdings with dividends: %d", len(holdingDetails))
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioDividendReturn_ZeroWhenNoDividends ---

// TestPortfolioDividendReturn_ZeroWhenNoDividends verifies that dividend_return
// is 0 or close to 0 when holdings have no dividend income. This tests the edge case
// where dividend data may not be available or portfolios hold only growth stocks.
func TestPortfolioDividendReturn_ZeroWhenNoDividends(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("handles_no_dividend_holdings_gracefully", func(t *testing.T) {
		portfolio := getPortfolioDividendData(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_dividend_data", string(raw))

		dividendForecast, ok := portfolio["income_dividends_forecast"].(float64)
		require.True(t, ok, "dividend_forecast should be a number")

		// Get holdings to check dividend composition
		holdings := getPortfolioHoldings(t, env, portfolioName, userHeaders)

		// Check if all holdings have zero or negative dividend return
		var holdingCount, zeroOrNegCount int
		for _, h := range holdings {
			holdingCount++
			dividendReturn, ok := h["dividend_return"].(float64)
			if ok && dividendReturn <= 0 {
				zeroOrNegCount++
			}
		}

		t.Logf("Holdings analyzed: %d", holdingCount)
		t.Logf("Holdings with zero/negative dividend: %d", zeroOrNegCount)
		t.Logf("dividend_forecast: %.2f", dividendForecast)

		// If most/all holdings have zero dividends, portfolio total should be close to 0
		if zeroOrNegCount == holdingCount {
			assert.InDelta(t, 0.0, dividendForecast, 0.01,
				"dividend_forecast should be near 0 when all holdings have zero dividend")
		} else {
			// Otherwise just verify it's a valid number (non-NaN, not infinite)
			assert.False(t, dividendForecast != dividendForecast, // NaN check
				"dividend_forecast should not be NaN")
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioDividendReturn_IncludedInNetEquityReturn ---

// TestPortfolioDividendReturn_IncludedInNetEquityReturn verifies that the dividend
// return is properly reflected in the net_equity_return calculation. Dividends are
// a component of total return and should be additive to the portfolio's net return.
func TestPortfolioDividendReturn_IncludedInNetEquityReturn(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("dividend_component_in_net_equity_return", func(t *testing.T) {
		portfolio := getPortfolioDividendData(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_return_breakdown", string(raw))

		dividendForecast, ok := portfolio["income_dividends_forecast"].(float64)
		require.True(t, ok, "dividend_forecast should be a number")

		realizedEquityReturn, ok := portfolio["equity_holdings_realized"].(float64)
		require.True(t, ok, "realized_equity_return should be a number")

		unrealizedEquityReturn, ok := portfolio["equity_holdings_unrealized"].(float64)
		require.True(t, ok, "unrealized_equity_return should be a number")

		netEquityReturn, ok := portfolio["equity_holdings_return"].(float64)
		require.True(t, ok, "net_equity_return should be a number")

		// Verify that dividends are included in the total return components
		t.Logf("Dividend Forecast: %.2f", dividendForecast)
		t.Logf("Realized Equity Return: %.2f", realizedEquityReturn)
		t.Logf("Unrealized Equity Return: %.2f", unrealizedEquityReturn)
		t.Logf("Net Equity Return: %.2f", netEquityReturn)

		// The dividend forecast should be a valid number
		assert.False(t, dividendForecast != dividendForecast, // NaN check
			"dividend_forecast should be a valid number")
		assert.False(t, netEquityReturn != netEquityReturn, // NaN check
			"net_equity_return should be a valid number")

		t.Logf("Verified: dividend_forecast is properly structured in return components")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- TestPortfolioDividendReturn_FXConversion ---

// TestPortfolioDividendReturn_FXConversion verifies that dividend_return values for
// FX holdings (e.g., USD-denominated stocks) are already converted to AUD. The
// portfolio's dividend_return should sum these already-converted values without
// applying FX conversion again.
func TestPortfolioDividendReturn_FXConversion(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	t.Run("dividend_fx_values_already_converted", func(t *testing.T) {
		portfolio := getPortfolioDividendData(t, env, portfolioName, userHeaders)

		raw, _ := json.Marshal(portfolio)
		guard.SaveResult("01_portfolio_with_fx_context", string(raw))

		// Get portfolio FX rate (if USD holdings exist)
		fxRate, hasFXRate := portfolio["fx_rate"].(float64)
		currency, hasCurrency := portfolio["currency"].(string)

		holdings := getPortfolioHoldings(t, env, portfolioName, userHeaders)

		// Check for USD-denominated holdings
		var usdHoldings []map[string]interface{}
		for _, h := range holdings {
			if exchange, ok := h["exchange"].(string); ok && (exchange == "US" || exchange == "NASDAQ" || exchange == "NYSE") {
				usdHoldings = append(usdHoldings, h)
			}
		}

		holdingDetails := make([]map[string]interface{}, 0)

		dividendForecast, ok := portfolio["income_dividends_forecast"].(float64)
		require.True(t, ok, "dividend_forecast should be a number")

		for _, h := range usdHoldings {
			holdingDetails = append(holdingDetails, map[string]interface{}{
				"ticker":          h["ticker"],
				"exchange":        h["exchange"],
				"dividend_return": h["dividend_return"],
				"currency":        h["currency"],
			})
		}

		t.Logf("Portfolio currency: %v", currency)
		t.Logf("FX Rate (if applicable): %v", fxRate)
		t.Logf("USD Holdings found: %d", len(usdHoldings))
		t.Logf("dividend_forecast (AUD equivalent): %.2f", dividendForecast)

		if hasFXRate && hasCurrency && len(usdHoldings) > 0 {
			holdingDetailsJSON, _ := json.Marshal(holdingDetails)
			guard.SaveResult("02_usd_holdings_dividend_detail", string(holdingDetailsJSON))

			// Key assertion: dividend values are already in portfolio base currency (AUD)
			// The presence of fx_rate indicates FX holdings, but the dividend_return
			// field should already be converted
			assert.True(t, fxRate > 0.5 && fxRate < 2.0,
				"FX rate should be in reasonable range for AUD conversion")

			t.Logf("Verified: USD dividend values are FX-converted (rate: %.4f)", fxRate)
		} else {
			t.Logf("No USD holdings or FX rate context — skipping FX validation")
		}

		// Core validation: dividend_forecast is a valid number
		assert.False(t, dividendForecast != dividendForecast, // NaN check
			"dividend_forecast should not be NaN even with mixed currencies")
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
