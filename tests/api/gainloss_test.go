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
// each holding's GainLoss, TotalCost, and Trades are populated. This validates the
// end-to-end path: Navexa fetch -> calculateGainLossFromTrades -> EODHD price
// cross-check (delta approach) -> JSON response.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioGainLossFields(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio (populates trade data and calculates GainLoss)
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

	// Validate GainLoss-related fields on each holding
	for _, h := range got.Holdings {
		t.Run(h.Ticker, func(t *testing.T) {
			// Active positions should have market data
			if h.Units > 0 {
				assert.Greater(t, h.CurrentPrice, 0.0,
					"active holding should have a positive CurrentPrice")
				assert.Greater(t, h.MarketValue, 0.0,
					"active holding should have a positive MarketValue")
			}

			// TotalCost should be populated for all holdings with trades
			if len(h.Trades) > 0 {
				assert.NotZero(t, h.TotalCost,
					"holding with trades should have a non-zero TotalCost")
			}

			// GainLoss should be non-zero for holdings with trades
			// (it could be negative, but shouldn't be exactly zero unless very unlikely)
			if len(h.Trades) > 0 && h.Units > 0 {
				// Don't assert non-zero — it could legitimately be zero if price == avg cost.
				// Instead, verify the relationship:
				// For buy-only: GainLoss should approx equal MarketValue - TotalCost
				hasSells := false
				for _, tr := range h.Trades {
					if strings.EqualFold(tr.Type, "sell") {
						hasSells = true
						break
					}
				}
				if !hasSells {
					// Pure buy-and-hold: GainLoss = MarketValue - TotalCost
					expected := h.MarketValue - h.TotalCost
					assert.InDelta(t, expected, h.GainLoss, 1.0,
						"buy-only holding GainLoss should equal MarketValue - TotalCost (within $1)")
				}
				// For positions with sells, we cannot validate the exact number without
				// replaying all trades, but we can check TotalReturnValue consistency
				assert.InDelta(t, h.GainLoss+h.DividendReturn, h.TotalReturnValue, 1.0,
					"TotalReturnValue should equal GainLoss + DividendReturn (within $1)")
			}

			// Trades should be populated (the sync fetches them from Navexa)
			if h.Units > 0 {
				assert.NotEmpty(t, h.Trades,
					"active holding should have trade history from Navexa")
			}

			// Each trade should have valid fields
			for j, tr := range h.Trades {
				assert.NotEmpty(t, tr.Type, "trade[%d] should have a Type", j)
				assert.Greater(t, tr.Units, 0.0, "trade[%d] should have positive Units", j)
				assert.Greater(t, tr.Price, 0.0, "trade[%d] should have positive Price", j)
			}

			t.Logf("%s: units=%.0f price=%.2f mv=%.2f gl=%.2f cost=%.2f trades=%d",
				h.Ticker, h.Units, h.CurrentPrice, h.MarketValue, h.GainLoss, h.TotalCost, len(h.Trades))
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
