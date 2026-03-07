package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestPortfolio_HoldingSignalsPopulated verifies that portfolio holdings
// include trend_label and trend_score from batch-fetched signals (Fix C).
func TestPortfolio_HoldingSignalsPopulated(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	// Fetch portfolio
	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolioName, nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "response: %s", string(body))

	var portfolio struct {
		Holdings []struct {
			Ticker     string  `json:"ticker"`
			TrendLabel string  `json:"trend_label"`
			TrendScore float64 `json:"trend_score"`
		} `json:"holdings"`
		Breadth *struct {
			TrendLabel string  `json:"trend_label"`
			TrendScore float64 `json:"trend_score"`
		} `json:"breadth_summary"`
	}
	require.NoError(t, json.Unmarshal(body, &portfolio))

	// At least some holdings should have signals populated
	holdingsWithSignals := 0
	for _, h := range portfolio.Holdings {
		if h.TrendLabel != "" {
			holdingsWithSignals++
		}
	}

	// With batch loading, all holdings with computed signals should have labels
	assert.Greater(t, holdingsWithSignals, 0,
		"at least one holding should have trend_label populated from batch signals")

	// Breadth summary should aggregate trend data
	if portfolio.Breadth != nil {
		assert.NotEmpty(t, portfolio.Breadth.TrendLabel,
			"breadth summary should have aggregated trend label")
	}

	guard.SaveResult("01_holding_signals", fmt.Sprintf(
		"Holdings with signals: %d/%d. Breadth: %+v",
		holdingsWithSignals, len(portfolio.Holdings), portfolio.Breadth))
}

// TestPortfolio_ChangePercentages verifies that the portfolio endpoint returns
// correct change percentages (yesterday, week, month) in holdings.
func TestPortfolio_ChangePercentages(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	portfolioName, userHeaders := setupPortfolioForIndicators(t, env)

	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/portfolios/"+portfolioName, nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusOK, resp.StatusCode, "response: %s", string(body))

	var portfolio struct {
		Holdings []struct {
			Ticker                  string  `json:"ticker"`
			CurrentPrice            float64 `json:"current_price"`
			YesterdayClosePrice     float64 `json:"yesterday_close_price"`
			YesterdayPriceChangePct float64 `json:"yesterday_price_change_pct"`
			LastWeekClosePrice      float64 `json:"last_week_close_price"`
			LastWeekPriceChangePct  float64 `json:"last_week_price_change_pct"`
		} `json:"holdings"`
	}
	require.NoError(t, json.Unmarshal(body, &portfolio))

	holdingsWithPriceChanges := 0
	for _, h := range portfolio.Holdings {
		if h.YesterdayClosePrice > 0 {
			holdingsWithPriceChanges++

			// Verify percentage is computed correctly from prices
			expectedPct := ((h.CurrentPrice - h.YesterdayClosePrice) / h.YesterdayClosePrice) * 100
			assert.InDelta(t, expectedPct, h.YesterdayPriceChangePct, 0.1,
				"%s: yesterday pct should match (current-yesterday)/yesterday",
				h.Ticker)
		}

		if h.LastWeekClosePrice > 0 {
			expectedPct := ((h.CurrentPrice - h.LastWeekClosePrice) / h.LastWeekClosePrice) * 100
			assert.InDelta(t, expectedPct, h.LastWeekPriceChangePct, 0.1,
				"%s: last_week pct should match (current-lastweek)/lastweek",
				h.Ticker)
		}
	}

	assert.Greater(t, holdingsWithPriceChanges, 0,
		"at least some holdings should have yesterday close prices populated")

	guard.SaveResult("02_change_percentages", fmt.Sprintf(
		"Holdings with price changes: %d/%d",
		holdingsWithPriceChanges, len(portfolio.Holdings)))
}

// TestStockData_PctValuesConsistent verifies that the stock data endpoint
// returns consistent price change percentages that match the formula
// (current - historical_close) / historical_close * 100.
func TestStockData_PctValuesConsistent(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ConfigFile: "vire.toml",
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	setupPortfolioForIndicators(t, env)

	// Use a well-known AU ticker
	ticker := "BHP.AU"
	userHeaders := map[string]string{"X-Vire-User-ID": "dev_user"}

	resp, err := env.HTTPRequest(http.MethodGet,
		"/api/market/stock/"+ticker+"?include=price", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Skipf("stock data unavailable for %s: %s", ticker, string(body))
	}

	var stockData struct {
		Price *struct {
			Current        float64 `json:"current"`
			YesterdayClose float64 `json:"yesterday_close"`
			YesterdayPct   float64 `json:"yesterday_pct"`
			LastWeekClose  float64 `json:"last_week_close"`
			LastWeekPct    float64 `json:"last_week_pct"`
		} `json:"price"`
	}
	require.NoError(t, json.Unmarshal(body, &stockData))
	require.NotNil(t, stockData.Price, "price data should be included")

	if stockData.Price.YesterdayClose > 0 {
		expectedPct := ((stockData.Price.Current - stockData.Price.YesterdayClose) / stockData.Price.YesterdayClose) * 100
		assert.InDelta(t, expectedPct, stockData.Price.YesterdayPct, 0.01,
			"yesterday_pct should be (current - yesterday_close) / yesterday_close * 100")
	}

	if stockData.Price.LastWeekClose > 0 {
		expectedPct := ((stockData.Price.Current - stockData.Price.LastWeekClose) / stockData.Price.LastWeekClose) * 100
		assert.InDelta(t, expectedPct, stockData.Price.LastWeekPct, 0.01,
			"last_week_pct should be (current - last_week_close) / last_week_close * 100")
	}

	guard.SaveResult("03_stock_pct_consistent", fmt.Sprintf(
		"Ticker=%s Current=%.2f YesterdayClose=%.2f YesterdayPct=%.2f LastWeekClose=%.2f LastWeekPct=%.2f",
		ticker,
		stockData.Price.Current,
		stockData.Price.YesterdayClose, stockData.Price.YesterdayPct,
		stockData.Price.LastWeekClose, stockData.Price.LastWeekPct))
}
