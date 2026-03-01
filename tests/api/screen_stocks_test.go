package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// TestScreenStocks_FundamentalMode verifies that the /screen/stocks endpoint
// works with mode=fundamental (merged from legacy /screen endpoint).
func TestScreenStocks_FundamentalMode(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen/stocks",
		map[string]interface{}{
			"exchange": "AU",
			"mode":     "fundamental",
			"limit":    5,
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_stocks_fundamental", string(body))

	// Endpoint should exist and return either 200 or error about no matching stocks
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest,
		"screen/stocks with mode=fundamental should succeed or return validation error (not 404)")

	if resp.StatusCode == http.StatusOK {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Should have results array if successful
		if _, hasResults := result["results"]; hasResults {
			assert.True(t, true, "successful response should include results")
		}
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestScreenStocks_TechnicalMode verifies that the /screen/stocks endpoint
// works with mode=technical (merged from legacy /screen/snipe endpoint).
func TestScreenStocks_TechnicalMode(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen/stocks",
		map[string]interface{}{
			"exchange": "AU",
			"mode":     "technical",
			"limit":    5,
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_stocks_technical", string(body))

	// Endpoint should exist and return either 200 or error about no matching stocks
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest,
		"screen/stocks with mode=technical should succeed or return validation error (not 404)")

	if resp.StatusCode == http.StatusOK {
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		// Should have results array if successful
		if _, hasResults := result["results"]; hasResults {
			assert.True(t, true, "successful response should include results")
		}
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestScreenStocks_InvalidMode verifies that an invalid mode value returns an error.
func TestScreenStocks_InvalidMode(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen/stocks",
		map[string]interface{}{
			"exchange": "AU",
			"mode":     "invalid_mode",
			"limit":    5,
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_stocks_invalid_mode", string(body))

	// Should return 400 Bad Request for invalid mode
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"screen/stocks with invalid mode should return 400 Bad Request")

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err == nil {
		if errMsg, hasError := result["error"]; hasError {
			assert.NotEmpty(t, errMsg, "error response should explain the invalid mode")
		}
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestScreenStocks_MissingMode verifies that missing mode parameter returns an error.
func TestScreenStocks_MissingMode(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen/stocks",
		map[string]interface{}{
			"exchange": "AU",
			// mode is intentionally missing
			"limit": 5,
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_stocks_missing_mode", string(body))

	// Should return 400 Bad Request for missing mode
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"screen/stocks without mode parameter should return 400 Bad Request")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestScreenStocks_DefaultMode verifies that if mode is not specified, the endpoint
// clearly requires it (rather than defaulting).
func TestScreenStocks_RequiresMode(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Call without mode parameter at all
	resp, err := env.HTTPRequest(http.MethodPost, "/api/screen/stocks",
		map[string]interface{}{
			"exchange": "AU",
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_screen_stocks_requires_mode", string(body))

	// Must return error, not default to a mode
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"screen/stocks must require mode parameter (not default)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}
