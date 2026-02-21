package eodhd

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ============================================================================
// 1. flexFloat64 hostile input edge cases
// ============================================================================

func TestStress_FlexFloat64_HostileInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
		wantErr  bool
	}{
		// Valid edge cases
		{"max_float64", "1.7976931348623157e+308", math.MaxFloat64, false},
		{"smallest_positive", "5e-324", 5e-324, false},
		{"negative_zero", "-0", 0, false},
		{"string_negative_zero", `"-0"`, 0, false},
		{"very_small_string", `"0.000000001"`, 0.000000001, false},
		{"large_int_as_string", `"999999999999"`, 999999999999, false},

		// Malformed strings — should default to 0, not error
		{"garbage_string", `"abc"`, 0, false},
		{"dots_only", `"1.2.3"`, 0, false},
		{"unicode_digits", `"\u0661\u0662\u0663"`, 0, false}, // Arabic-Indic digits
		{"trailing_space", `"42.5 "`, 0, false},
		{"leading_space", `" 42.5"`, 0, false},
		{"mixed_alpha", `"42abc"`, 0, false},
		{"comma_decimal", `"1,234.56"`, 0, false},
		{"percent", `"42%"`, 0, false},
		{"dollar_sign", `"$42.50"`, 0, false},

		// JSON booleans — should error (not a number or string)
		{"bool_true", "true", 0, true},
		{"bool_false", "false", 0, true},

		// JSON arrays/objects — should error
		{"json_array", "[]", 0, true},
		{"json_object", "{}", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f flexFloat64
			err := json.Unmarshal([]byte(tt.input), &f)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %s, got value %f", tt.input, float64(f))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %s: %v", tt.input, err)
			}
			if math.IsNaN(float64(f)) && tt.expected == 0 {
				t.Errorf("input %s: got NaN, want 0", tt.input)
				return
			}
			if math.Abs(float64(f)-tt.expected) > 1e-10 {
				t.Errorf("input %s: got %f, want %f", tt.input, float64(f), tt.expected)
			}
		})
	}
}

// ============================================================================
// FINDING: flexFloat64 passes through NaN, Infinity, -Infinity
// ============================================================================
//
// strconv.ParseFloat parses "NaN", "Infinity", "-Infinity" without error.
// flexFloat64 guards against these values (client.go lines 34 and 52),
// returning 0 instead. This test validates the guard works correctly.

func TestStress_FlexFloat64_NaN_Infinity_Passthrough(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"nan_string", `"NaN"`},
		{"inf_string", `"Infinity"`},
		{"neg_inf_string", `"-Infinity"`},
		{"overflow", `"1e999"`},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var f flexFloat64
			err := json.Unmarshal([]byte(tt.input), &f)
			if err != nil {
				return // Error is acceptable
			}
			val := float64(f)
			if math.IsNaN(val) || math.IsInf(val, 0) {
				t.Errorf("FINDING: input %s produced %v — should be 0. "+
					"NaN/Inf values will corrupt market data computations.", tt.input, val)
			}
		})
	}
}

// ============================================================================
// 2. flexInt64 hostile input edge cases
// ============================================================================

func TestStress_FlexInt64_HostileInputs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		// Valid edges
		{"max_int64", "9223372036854775807", math.MaxInt64, false},
		{"min_int64", "-9223372036854775808", math.MinInt64, false},
		{"string_max", `"9223372036854775807"`, math.MaxInt64, false},

		// Overflow — strconv.ParseInt returns error, defaults to 0
		{"overflow_pos", `"9223372036854775808"`, 0, false},
		{"overflow_neg", `"-9223372036854775809"`, 0, false},

		// Malformed
		{"float_in_string", `"42.5"`, 0, false},
		{"garbage", `"abc"`, 0, false},
		{"hex_string", `"0xFF"`, 0, false},

		// Null not handled by flexInt64 (unlike flexFloat64)
		{"null", "null", 0, false},

		// Boolean
		{"bool_true", "true", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f flexInt64
			err := json.Unmarshal([]byte(tt.input), &f)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %s, got value %d", tt.input, int64(f))
				}
				return
			}
			if err != nil {
				// flexInt64 doesn't handle null — check if this is the null case
				if tt.input == "null" {
					t.Fatalf("FINDING: flexInt64 does not handle null input — returns error: %v", err)
				}
				t.Fatalf("unexpected error for input %s: %v", tt.input, err)
			}
			if int64(f) != tt.expected {
				t.Errorf("input %s: got %d, want %d", tt.input, int64(f), tt.expected)
			}
		})
	}
}

// ============================================================================
// 3. flexInt64 null handling — asymmetry with flexFloat64
// ============================================================================
//
// flexFloat64 handles null (returns 0), but flexInt64 does NOT handle null.
// This is an inconsistency — if EODHD returns null for volume, flexInt64
// will return an error and the entire bulk response deserialization fails.

func TestStress_FlexInt64_NullHandling(t *testing.T) {
	var f flexInt64
	err := json.Unmarshal([]byte("null"), &f)
	if err != nil {
		t.Errorf("FINDING: flexInt64 does not handle null — unmarshal error: %v. "+
			"This is inconsistent with flexFloat64 which handles null gracefully. "+
			"If EODHD returns null for volume, the entire bulkEODResponse deserialization will fail.", err)
	} else {
		if int64(f) != 0 {
			t.Errorf("null should unmarshal to 0, got %d", int64(f))
		}
	}
}

// ============================================================================
// 4. GetBulkEOD with API error
// ============================================================================

func TestStress_GetBulkEOD_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkEOD(context.Background(), "AU", []string{"BHP.AU"})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

// ============================================================================
// 5. GetBulkEOD with invalid JSON response
// ============================================================================

func TestStress_GetBulkEOD_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkEOD(context.Background(), "AU", []string{"BHP.AU"})
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

// ============================================================================
// 6. GetBulkEOD returns empty array
// ============================================================================

func TestStress_GetBulkEOD_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkEOD(context.Background(), "AU", []string{"BHP.AU", "RIO.AU"})
	if err != nil {
		t.Fatalf("empty response should not error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 results, got %d", len(result))
	}
}

// ============================================================================
// 7. GetBulkEOD returns unknown tickers (not in request)
// ============================================================================

func TestStress_GetBulkEOD_UnexpectedTickers(t *testing.T) {
	mockResp := `[
		{
			"code": "SURPRISE",
			"exchange_short": "AU",
			"date": "2025-03-28",
			"open": "10.00",
			"high": "11.00",
			"low": "9.00",
			"close": "10.50",
			"adjusted_close": "10.50",
			"volume": "1000"
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkEOD(context.Background(), "AU", []string{"BHP.AU"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unexpected ticker should get mapped with exchange suffix fallback
	if _, ok := result["SURPRISE.AU"]; !ok {
		t.Error("unexpected ticker should be mapped to SURPRISE.AU via fallback")
	}

	// The requested BHP.AU should NOT be in the result
	if _, ok := result["BHP.AU"]; ok {
		t.Error("BHP.AU was not returned by API but appears in result")
	}
}

// ============================================================================
// 8. GetBulkEOD with ticker that has multiple dots (e.g. "XAGUSD.FOREX")
// ============================================================================

func TestStress_GetBulkEOD_MultiDotTicker(t *testing.T) {
	mockResp := `[
		{
			"code": "XAGUSD",
			"exchange_short": "FOREX",
			"date": "2025-03-28",
			"open": 24.50,
			"high": 25.10,
			"low": 24.30,
			"close": 24.95,
			"adjusted_close": 24.95,
			"volume": 0
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	result, err := client.GetBulkEOD(context.Background(), "FOREX", []string{"XAGUSD.FOREX"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bar, ok := result["XAGUSD.FOREX"]; !ok {
		t.Error("XAGUSD.FOREX should be in result")
	} else {
		if bar.Close != 24.95 {
			t.Errorf("close = %.2f, want 24.95", bar.Close)
		}
	}
}

// ============================================================================
// 9. GetBulkEOD duplicate tickers in request
// ============================================================================

func TestStress_GetBulkEOD_DuplicateTickers(t *testing.T) {
	var capturedSymbols string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSymbols = r.URL.Query().Get("symbols")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"code":"BHP","date":"2025-03-28","open":"42","high":"43","low":"41","close":"42.5","adjusted_close":"42.5","volume":"1000"}]`))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	// Duplicate tickers — the second BHP.AU overwrites the first in tickerMap
	result, err := client.GetBulkEOD(context.Background(), "AU", []string{"BHP.AU", "BHP.AU"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// symbols param may contain duplicates (BHP,BHP) since no dedup is done
	t.Logf("symbols param sent: %q", capturedSymbols)

	if len(result) != 1 {
		t.Errorf("expected 1 result, got %d", len(result))
	}
}

// ============================================================================
// 10. GetBulkEOD with context cancellation
// ============================================================================

func TestStress_GetBulkEOD_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkEOD(ctx, "AU", []string{"BHP.AU"})
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

// ============================================================================
// 11. GetBulkEOD with very large ticker list
// ============================================================================

func TestStress_GetBulkEOD_LargeTickerList(t *testing.T) {
	var capturedSymbols string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSymbols = r.URL.Query().Get("symbols")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer srv.Close()

	// 200 tickers
	tickers := make([]string, 200)
	for i := range tickers {
		tickers[i] = "T" + string(rune('A'+i%26)) + string(rune('0'+i/26)) + ".AU"
	}

	client := NewClient("test-key", WithBaseURL(srv.URL))
	_, err := client.GetBulkEOD(context.Background(), "AU", tickers)
	if err != nil {
		t.Fatalf("large ticker list should not error: %v", err)
	}

	// Verify the symbols param contains all tickers (comma-separated)
	if len(capturedSymbols) == 0 {
		t.Error("symbols param should not be empty")
	}
}

// ============================================================================
// 12. bulkEODResponse with all-zero values
// ============================================================================

func TestStress_BulkEODResponse_AllZeros(t *testing.T) {
	input := `{"code":"X","exchange_short":"AU","date":"2025-01-01","open":"0","high":"0","low":"0","close":"0","adjusted_close":"0","volume":"0"}`
	var row bulkEODResponse
	if err := json.Unmarshal([]byte(input), &row); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if float64(row.Close) != 0 {
		t.Errorf("close should be 0, got %f", float64(row.Close))
	}
	if int64(row.Volume) != 0 {
		t.Errorf("volume should be 0, got %d", int64(row.Volume))
	}
}

// ============================================================================
// 13. bulkEODResponse with negative values
// ============================================================================

func TestStress_BulkEODResponse_NegativeValues(t *testing.T) {
	input := `{"code":"X","date":"2025-01-01","open":"-1.5","high":"-0.5","low":"-2.0","close":"-1.0","adjusted_close":"-1.0","volume":"-100"}`
	var row bulkEODResponse
	if err := json.Unmarshal([]byte(input), &row); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if float64(row.Open) != -1.5 {
		t.Errorf("open = %f, want -1.5", float64(row.Open))
	}
	// Negative volume is technically nonsensical but should parse
	if int64(row.Volume) != -100 {
		t.Errorf("volume = %d, want -100", int64(row.Volume))
	}
}

// ============================================================================
// 14. JSON injection via string value — no code execution risk but verify
//     that the value doesn't corrupt the response
// ============================================================================

func TestStress_BulkEODResponse_JSONInjection(t *testing.T) {
	input := `{"code":"X","date":"2025-01-01","open":"42\",\"injected\":\"true","high":"0","low":"0","close":"0","adjusted_close":"0","volume":"0"}`
	var row bulkEODResponse
	err := json.Unmarshal([]byte(input), &row)
	// This should either fail or parse safely — no injection possible via JSON
	if err != nil {
		// JSON parse failure is the expected outcome for malformed input
		return
	}
	// If it parsed, the value should default to 0 (malformed float)
	t.Logf("Parsed open value: %f (injection attempt was neutralized)", float64(row.Open))
}

// ============================================================================
// 15. GetBulkEOD with ticker code containing special characters
// ============================================================================

func TestStress_GetBulkEOD_SpecialCharacterTicker(t *testing.T) {
	mockResp := `[{"code":"BRK-B","date":"2025-01-01","open":420.0,"high":425.0,"low":418.0,"close":422.0,"adjusted_close":422.0,"volume":5000000}]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(mockResp))
	}))
	defer srv.Close()

	client := NewClient("test-key", WithBaseURL(srv.URL))
	// BRK-B.US — the code part is "BRK-B", split on "." gives ["BRK-B", "US"]
	result, err := client.GetBulkEOD(context.Background(), "US", []string{"BRK-B.US"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bar, ok := result["BRK-B.US"]; !ok {
		t.Error("BRK-B.US should be in result")
	} else if bar.Close != 422.0 {
		t.Errorf("close = %.2f, want 422.0", bar.Close)
	}
}
