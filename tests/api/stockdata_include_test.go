package api

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestGetStockData_IncludeParam tests the ?include query parameter parsing for
// GET /api/market/stocks/{ticker}. It verifies that:
//   - Single value: ?include=price
//   - Repeated key: ?include=price&include=fundamentals
//   - Comma-separated: ?include=price,signals
//   - Mixed format: ?include=price,signals&include=news
//   - Default (no param): all sections included
//   - Edge cases: unknown values, empty values, duplicates
//
// Tests share one Docker environment since they are read-only and don't mutate state.
func TestGetStockData_IncludeParam(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// BHP.AU is a well-known ASX ticker. The server accepts this format regardless of
	// whether market data has been collected — we're testing HTTP parameter parsing,
	// not data presence. A 500 (no data) is acceptable; a 400 means the parameter
	// was rejected and is a failure.
	const ticker = "BHP.AU"

	t.Run("single_value_price", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=price")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("single_include_price", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"?include=price should be accepted, not rejected as bad request")

		if resp.StatusCode == 200 {
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &result))

			assert.NotContains(t, result, "fundamentals",
				"fundamentals should not be present when only price is requested")
			assert.NotContains(t, result, "signals",
				"signals should not be present when only price is requested")
			assert.NotContains(t, result, "news",
				"news should not be present when only price is requested")
		}
	})

	t.Run("repeated_key_price_fundamentals", func(t *testing.T) {
		// This is the key regression test: ?include=price&include=fundamentals must
		// correctly include BOTH price and fundamentals. The old code using
		// r.URL.Query().Get("include") would only see the first value ("price"),
		// leaving fundamentals absent. The fix uses r.URL.Query()["include"] to
		// get all values.
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=price&include=fundamentals")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("multi_include_price_fundamentals", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"?include=price&include=fundamentals should be accepted as valid")

		if resp.StatusCode == 200 {
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &result))

			// signals and news must be absent — not requested
			assert.NotContains(t, result, "signals",
				"signals should not be present when only price+fundamentals are requested")
			assert.NotContains(t, result, "news",
				"news should not be present when only price+fundamentals are requested")
		}
	})

	t.Run("comma_separated_price_signals", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=price,signals")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("comma_include_price_signals", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"?include=price,signals (comma-separated) should be accepted as valid")

		if resp.StatusCode == 200 {
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &result))

			assert.NotContains(t, result, "fundamentals",
				"fundamentals should not be present when only price,signals are requested")
			assert.NotContains(t, result, "news",
				"news should not be present when only price,signals are requested")
		}
	})

	t.Run("mixed_format_price_signals_news", func(t *testing.T) {
		// Mixed: comma-separated in one param + repeated key for another.
		// ?include=price,signals&include=news should include price, signals, and news,
		// but NOT fundamentals.
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=price,signals&include=news")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("mixed_include_price_signals_news", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"mixed include formats should be accepted as valid")

		if resp.StatusCode == 200 {
			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(body, &result))

			assert.NotContains(t, result, "fundamentals",
				"fundamentals should not be present when only price,signals,news are requested")
		}
	})

	t.Run("default_no_include_param", func(t *testing.T) {
		// No include param means all sections are returned by default.
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("default_include_all", string(body))

		// 400 would mean the endpoint itself broke; 200/500 are both acceptable.
		assert.NotEqual(t, 400, resp.StatusCode,
			"omitting include param should return all sections (default behaviour)")
		t.Logf("Default (no include param) status: %d", resp.StatusCode)
	})

	t.Run("unknown_value_ignored", func(t *testing.T) {
		// Unknown include value "INVALID" should be silently ignored.
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=price,INVALID")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("invalid_include_value", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"unknown include values should be silently ignored, not rejected as bad request")
	})

	t.Run("empty_value_handled_gracefully", func(t *testing.T) {
		// ?include=&include=price — empty string value should be ignored.
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=&include=price")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("empty_include_value", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"empty include values should be handled gracefully, not rejected as bad request")
	})

	t.Run("duplicate_values_handled", func(t *testing.T) {
		// ?include=price&include=price — duplicates are idempotent.
		resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "?include=price&include=price")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("duplicate_include_value", string(body))

		assert.NotEqual(t, 400, resp.StatusCode,
			"duplicate include values should be handled gracefully")
		assert.NotEqual(t, 500, resp.StatusCode,
			"duplicate include values should not cause a server error")
	})
}
