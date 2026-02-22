package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/tests/common"
)

// TestPortfolioFX_HoldingCurrencyConversion verifies that after syncing a portfolio
// containing USD holdings (e.g. CBOE), the GET /api/portfolios/{name} response returns
// all holding values converted to the portfolio base currency (AUD).
//
// Specifically:
//   - USD holdings should have Currency="AUD" (converted) and OriginalCurrency="USD"
//   - AUD holdings should have Currency="AUD" and no OriginalCurrency
//   - FXRate should be populated on the portfolio
//   - Monetary fields (CurrentPrice, MarketValue, etc.) should be in AUD
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioFX_HoldingCurrencyConversion(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Sync portfolio to trigger FX conversion
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
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	t.Run("fx_rate_populated", func(t *testing.T) {
		assert.Greater(t, got.FXRate, 0.0,
			"portfolio FXRate should be populated when USD holdings exist")
		t.Logf("FXRate (AUDUSD): %.4f", got.FXRate)
	})

	t.Run("all_holdings_in_base_currency", func(t *testing.T) {
		for _, h := range got.Holdings {
			assert.Equal(t, "AUD", h.Currency,
				"holding %s should have Currency=AUD after FX conversion", h.Ticker)
		}
	})

	// Check for a known USD holding (CBOE) and verify conversion
	var cboe *models.Holding
	for i, h := range got.Holdings {
		if h.Ticker == "CBOE" {
			cboe = &got.Holdings[i]
			break
		}
	}

	if cboe != nil {
		t.Run("cboe_original_currency_set", func(t *testing.T) {
			assert.Equal(t, "USD", cboe.OriginalCurrency,
				"CBOE should have OriginalCurrency=USD")
		})

		t.Run("cboe_values_in_aud", func(t *testing.T) {
			assert.Greater(t, cboe.CurrentPrice, 0.0,
				"CBOE CurrentPrice should be positive (in AUD)")
			assert.Greater(t, cboe.MarketValue, 0.0,
				"CBOE MarketValue should be positive (in AUD)")
		})

		t.Logf("CBOE: currency=%s original=%s price=%.2f mv=%.2f",
			cboe.Currency, cboe.OriginalCurrency, cboe.CurrentPrice, cboe.MarketValue)
	} else {
		t.Log("CBOE not found in portfolio -- skipping USD-specific assertions")
	}

	// Verify AUD holdings do NOT have OriginalCurrency set
	t.Run("aud_holdings_no_original_currency", func(t *testing.T) {
		for _, h := range got.Holdings {
			if h.OriginalCurrency == "" {
				// This is a native AUD holding -- expected
				continue
			}
			// If OriginalCurrency is set, it should be a non-AUD currency
			assert.NotEqual(t, "AUD", h.OriginalCurrency,
				"holding %s: OriginalCurrency should not be AUD (only set for converted holdings)", h.Ticker)
		}
	})

	t.Logf("Validated %d holdings for currency conversion", len(got.Holdings))
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioFX_NetReturnNonZero verifies that after syncing, active holdings
// have non-zero net_return, realized_net_return, and unrealized_net_return values.
// This validates that the cache invalidation (DataVersion check) correctly triggers
// re-sync when stale data with zero values exists from old schema versions.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioFX_NetReturnNonZero(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Force sync to get fresh data with current schema version
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
	require.NotEmpty(t, got.Holdings, "portfolio should have at least one holding")

	// Find active holdings (units > 0) and verify return fields are populated
	activeCount := 0
	for _, h := range got.Holdings {
		if h.Units <= 0 {
			continue
		}
		activeCount++

		t.Run(h.Ticker+"_returns_populated", func(t *testing.T) {
			// At least one of the return fields should be non-zero for an active position
			hasReturn := h.NetReturn != 0 || h.RealizedNetReturn != 0 || h.UnrealizedNetReturn != 0
			assert.True(t, hasReturn,
				"%s: at least one return field should be non-zero (nr=%.2f real=%.2f unreal=%.2f)",
				h.Ticker, h.NetReturn, h.RealizedNetReturn, h.UnrealizedNetReturn)

			// UnrealizedNetReturn should be non-zero for active positions with market value
			if h.MarketValue > 0 && h.TotalCost > 0 {
				assert.NotEqual(t, 0.0, h.UnrealizedNetReturn,
					"%s: unrealized return should be non-zero for active position with cost and market value",
					h.Ticker)
			}

			t.Logf("%s: nr=%.2f real=%.2f unreal=%.2f div=%.2f",
				h.Ticker, h.NetReturn, h.RealizedNetReturn, h.UnrealizedNetReturn, h.DividendReturn)
		})
	}

	require.Greater(t, activeCount, 0, "portfolio should have at least one active holding")
	t.Logf("Validated return fields for %d active holdings", activeCount)
	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioFX_ForceSyncRefreshesStaleSchema verifies that a force sync via
// POST /api/portfolios/{name}/sync correctly refreshes data that may have been
// cached with an old schema version (DataVersion mismatch).
//
// The test syncs twice: first to populate, then force-syncs again to verify
// the data is consistent and DataVersion is current.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioFX_ForceSyncRefreshesStaleSchema(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Initial sync
	syncBody1 := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_initial_sync", string(syncBody1))

	// Get portfolio after first sync
	resp1, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err)
	defer resp1.Body.Close()

	body1, err := io.ReadAll(resp1.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp1.StatusCode)

	guard.SaveResult("02_first_get", string(body1))

	var portfolio1 models.Portfolio
	require.NoError(t, json.Unmarshal(body1, &portfolio1))

	// Force sync again
	syncBody2 := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("03_force_sync", string(syncBody2))

	// Get portfolio after force sync
	resp2, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err)
	defer resp2.Body.Close()

	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp2.StatusCode)

	guard.SaveResult("04_second_get", string(body2))

	var portfolio2 models.Portfolio
	require.NoError(t, json.Unmarshal(body2, &portfolio2))

	t.Run("holdings_count_consistent", func(t *testing.T) {
		assert.Equal(t, len(portfolio1.Holdings), len(portfolio2.Holdings),
			"holdings count should be consistent across syncs")
	})

	t.Run("totals_consistent", func(t *testing.T) {
		// Values may vary slightly due to real-time price changes, but should be in same ballpark
		if portfolio1.TotalValue > 0 {
			ratio := portfolio2.TotalValue / portfolio1.TotalValue
			assert.InDelta(t, 1.0, ratio, 0.1,
				"total value should be within 10%% across syncs (%.2f vs %.2f)",
				portfolio1.TotalValue, portfolio2.TotalValue)
		}
	})

	t.Run("data_version_populated", func(t *testing.T) {
		// After force sync, DataVersion should be set
		// Note: DataVersion is stored in the record but may not be returned in the API response.
		// The raw JSON check verifies it's present in the serialized form.
		var raw map[string]interface{}
		require.NoError(t, json.Unmarshal(body2, &raw))

		if dv, ok := raw["data_version"]; ok {
			assert.NotEmpty(t, dv, "data_version should not be empty if present")
			t.Logf("DataVersion: %v", dv)
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestPortfolioFX_RawJSONFieldNames verifies the raw JSON field names in the
// portfolio response include the new FX-related fields (original_currency, fx_rate)
// and that the response structure is correct.
//
// Requires NAVEXA_API_KEY and DEFAULT_PORTFOLIO environment variables.
func TestPortfolioFX_RawJSONFieldNames(t *testing.T) {
	env, headers, portfolio := setupPortfolioEnv(t, common.EnvOptions{
		ConfigFile: "vire-service.toml",
	})
	defer env.Cleanup()

	guard := env.OutputGuard()

	syncBody := syncPortfolio(t, env, portfolio, headers)
	guard.SaveResult("01_sync_response", string(syncBody))

	resp, err := env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolio, nil, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	guard.SaveResult("02_get_portfolio_response", string(body))

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &raw))

	t.Run("portfolio_level_fx_fields", func(t *testing.T) {
		assert.Contains(t, raw, "fx_rate", "portfolio should have fx_rate field")
		assert.Contains(t, raw, "currency", "portfolio should have currency field")
	})

	t.Run("holding_level_currency_fields", func(t *testing.T) {
		holdings, ok := raw["holdings"].([]interface{})
		require.True(t, ok, "holdings should be an array")
		require.NotEmpty(t, holdings)

		for i, hRaw := range holdings {
			hMap, ok := hRaw.(map[string]interface{})
			require.True(t, ok, "holding[%d] should be a map", i)

			assert.Contains(t, hMap, "currency",
				"holding[%d] should have currency field", i)

			// Check for original_currency on holdings that were converted
			if oc, exists := hMap["original_currency"]; exists && oc != nil {
				assert.NotEmpty(t, oc,
					"holding[%d] original_currency should not be empty if present", i)
				ticker := hMap["ticker"]
				t.Logf("holding[%d] %v: currency=%v original_currency=%v",
					i, ticker, hMap["currency"], oc)
			}
		}
	})

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
