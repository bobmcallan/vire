package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/tests/common"
)

// TestPortfolioGainLossFields verifies that after syncing a portfolio via the API,
// each holding's NetReturn, TotalCost, and return breakdown fields are populated.
// This validates the end-to-end path: Navexa fetch -> calculateGainLossFromTrades
// -> EODHD price cross-check (delta approach) -> JSON response.
//
// Also verifies that portfolio GET strips trades (trades should only appear in
// the individual stock endpoint).
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioGainLossFields(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio (populates trade data and calculates NetReturn)
	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	// Get synced portfolio
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

	// Verify portfolio-level new fields
	t.Run("portfolio_totals_present", func(t *testing.T) {
		// Use raw JSON to check field names
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &raw))

		assert.Contains(t, raw, "net_equity_return", "portfolio should have net_equity_return field")
		assert.Contains(t, raw, "net_equity_return_pct", "portfolio should have net_equity_return_pct field")
		assert.Contains(t, raw, "realized_equity_return", "portfolio should have realized_equity_return field")
		assert.Contains(t, raw, "unrealized_equity_return", "portfolio should have unrealized_equity_return field")

		// Verify old fields are NOT present
		assert.NotContains(t, raw, "total_gain", "portfolio should not have old total_gain field")
		assert.NotContains(t, raw, "total_gain_pct", "portfolio should not have old total_gain_pct field")
	})

	// Validate fields on each holding
	for _, h := range got.Holdings {
		t.Run(h.Ticker, func(t *testing.T) {
			// Portfolio GET should NOT include trades (trades stripped at handler level)
			assert.Empty(t, h.Trades,
				"portfolio GET should not include trades (stripped at handler level)")

			// Active positions should have market data
			if h.Units > 0 {
				assert.Greater(t, h.CurrentPrice, 0.0,
					"active holding should have a positive CurrentPrice")
				assert.Greater(t, h.MarketValue, 0.0,
					"active holding should have a positive MarketValue")
			}

			// Get the individual stock to verify trades ARE present there
			resp2, err := env.HTTPRequest(http.MethodGet,
				"/api/portfolios/"+portfolio+"/stock/"+h.Ticker, nil, headers)
			require.NoError(t, err, "get stock request failed for %s", h.Ticker)
			defer resp2.Body.Close()

			stockBody, err := io.ReadAll(resp2.Body)
			require.NoError(t, err)
			if resp2.StatusCode == http.StatusOK {
				var stock models.Holding
				require.NoError(t, json.Unmarshal(stockBody, &stock))

				if stock.Units > 0 {
					assert.NotEmpty(t, stock.Trades,
						"individual stock GET should include trades for active holding")

					// Verify NetReturn relationship for buy-only
					hasSells := false
					for _, tr := range stock.Trades {
						if strings.EqualFold(tr.Type, "sell") {
							hasSells = true
							break
						}
					}
					if !hasSells {
						expected := stock.MarketValue - stock.CostBasis
						assert.InDelta(t, expected, stock.NetReturn, 1.0,
							"buy-only holding NetReturn should equal MarketValue - TotalCost (within $1)")
					}
				}
			}

			t.Logf("%s: units=%.0f price=%.2f mv=%.2f nr=%.2f cost=%.2f",
				h.Ticker, h.Units, h.CurrentPrice, h.MarketValue, h.NetReturn, h.CostBasis)
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
