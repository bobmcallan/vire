package api

// Integration tests for 4 bug fixes:
// - Bug 1: EODHD ticker validation (fb_9f137670 + fb_fb1b044f)
// - Bug 2: compute_indicators silent failure + EMA/RSI fixes (fb_5581bfa1)
// - Bug 3: Compliance exchange→country mapping (fb_c6fd4a3e)
//
// These tests exercise the HTTP API layer. Unit tests for EMA/RSI math and
// compliance exchange mapping logic are in:
//   - internal/signals/indicators_test.go
//   - internal/services/strategy/compliance_test.go

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

// --- Bug 1 + Bug 2: Ticker validation and signal detection via API ---
//
// POST /api/market/signals = compute_indicators MCP tool backend.

// TestComputeIndicators_TickerValidation verifies that the POST
// /api/market/signals endpoint enforces ticker format rules introduced by Bug 1
// (exchange suffix required, invalid characters rejected).
func TestComputeIndicators_TickerValidation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name       string
		tickers    []string
		wantStatus int
		wantErr    string
	}{
		{
			name:       "bare ticker without exchange suffix is rejected",
			tickers:    []string{"ACDC"},
			wantStatus: http.StatusBadRequest,
			wantErr:    "ACDC",
		},
		{
			name:       "ticker with invalid characters is rejected",
			tickers:    []string{"BHP AU"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty tickers list is rejected",
			tickers:    []string{},
			wantStatus: http.StatusBadRequest,
			wantErr:    "tickers",
		},
		{
			// After ticker validation passes, signal detection runs.
			// No market data → returns error entry (Bug 2a fix), not error status.
			name:       "valid ticker with AU suffix proceeds to signal detection",
			tickers:    []string{"ACDC.AU"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "valid ticker with US suffix proceeds to signal detection",
			tickers:    []string{"AAPL.US"},
			wantStatus: http.StatusOK,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPPost("/api/market/signals", map[string]interface{}{
				"tickers": tt.tickers,
			})
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult(fmt.Sprintf("%02d_%s", i+1, sanitizeName(tt.name)), string(body))

			assert.Equal(t, tt.wantStatus, resp.StatusCode,
				"ticker=%v expected status %d got %d: %s",
				tt.tickers, tt.wantStatus, resp.StatusCode, string(body))

			if tt.wantErr != "" && resp.StatusCode != http.StatusOK {
				assert.Contains(t, string(body), tt.wantErr,
					"error response should mention %q", tt.wantErr)
			}
		})
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestComputeIndicators_MissingMarketData verifies Bug 2a fix:
// POST /api/market/signals with a valid ticker that has no stored market data
// returns 200 with a signals array that includes an error entry for that ticker,
// rather than silently omitting it (the pre-fix behaviour).
func TestComputeIndicators_MissingMarketData(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/market/signals", map[string]interface{}{
		"tickers": []string{"NODATA.AU"},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_missing_market_data", string(body))

	// Missing market data is a partial result — server returns 200, not 5xx.
	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"compute_indicators should return 200 even when market data is missing: %s", string(body))

	var result struct {
		Signals []struct {
			Ticker string `json:"ticker"`
			Error  string `json:"error"`
		} `json:"signals"`
	}
	require.NoError(t, json.Unmarshal(body, &result), "response must be valid JSON")

	// Bug 2a fix: ticker must appear in the response with an error field.
	require.Len(t, result.Signals, 1,
		"NODATA.AU should appear in signals array with an error entry (Bug 2a fix)")

	assert.Equal(t, "NODATA.AU", result.Signals[0].Ticker,
		"ticker field in error entry should match requested ticker")
	assert.NotEmpty(t, result.Signals[0].Error,
		"error field must be non-empty when market data is missing (Bug 2a fix)")

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestComputeIndicators_AllTickersMissing verifies that when every requested ticker
// lacks market data the response is still 200 with error entries for all of them.
func TestComputeIndicators_AllTickersMissing(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tickers := []string{"MISS1.AU", "MISS2.US"}
	resp, err := env.HTTPPost("/api/market/signals", map[string]interface{}{
		"tickers": tickers,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("02_all_tickers_missing", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode,
		"should return 200 when all tickers have no market data: %s", string(body))

	var result struct {
		Signals []struct {
			Ticker string `json:"ticker"`
			Error  string `json:"error"`
		} `json:"signals"`
	}
	require.NoError(t, json.Unmarshal(body, &result))

	// All tickers must appear with error entries, not be silently dropped.
	assert.Len(t, result.Signals, len(tickers),
		"all tickers should appear in signals array even without market data")

	for _, sig := range result.Signals {
		assert.NotEmpty(t, sig.Ticker, "ticker must be set in error entry")
		assert.NotEmpty(t, sig.Error, "error field must be set for tickers with no market data")
	}

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- Signal endpoint method and input guards ---

// TestSignals_EmptyTickersList verifies that POST /api/market/signals with an
// empty tickers array returns 400 Bad Request.
func TestSignals_EmptyTickersList(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/market/signals", map[string]interface{}{
		"tickers": []string{},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("03_empty_tickers", string(body))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode,
		"empty tickers should return 400: %s", string(body))

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// TestSignals_MethodNotAllowed verifies that GET on /api/market/signals returns 405.
func TestSignals_MethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPGet("/api/market/signals")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("04_method_not_allowed", string(body))

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode,
		"GET on /api/market/signals should return 405: %s", string(body))

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// sanitizeName replaces non-alphanumeric characters with underscores for use
// as a file or result name.
func sanitizeName(s string) string {
	out := make([]byte, len(s))
	for i := range s {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			out[i] = c
		} else {
			out[i] = '_'
		}
	}
	return string(out)
}
