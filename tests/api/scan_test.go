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

// --- Helpers ---

// scanFieldsResponse represents the GET /api/scan/fields response.
type scanFieldsResponse struct {
	Version   string           `json:"version"`
	Groups    []scanFieldGroup `json:"groups"`
	Exchanges []string         `json:"exchanges"`
	MaxLimit  int              `json:"max_limit"`
}

type scanFieldGroup struct {
	Name   string         `json:"name"`
	Fields []scanFieldDef `json:"fields"`
}

type scanFieldDef struct {
	Field       string   `json:"field"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Filterable  bool     `json:"filterable"`
	Sortable    bool     `json:"sortable"`
	Operators   []string `json:"operators"`
}

// scanResponse represents the POST /api/scan response.
type scanResponse struct {
	Results []map[string]interface{} `json:"results"`
	Meta    scanMeta                 `json:"meta"`
}

type scanMeta struct {
	TotalMatched int    `json:"total_matched"`
	Returned     int    `json:"returned"`
	Exchange     string `json:"exchange"`
	ExecutedAt   string `json:"executed_at"`
	QueryTimeMS  int    `json:"query_time_ms"`
}

// parseScanResponse reads the response body and unmarshals into a scanResponse.
func parseScanResponse(t *testing.T, body io.ReadCloser) scanResponse {
	t.Helper()
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var resp scanResponse
	require.NoError(t, json.Unmarshal(data, &resp), "response: %s", string(data))
	return resp
}

// --- Tests ---

func TestScanFields(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPGet("/api/scan/fields")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_scan_fields_response.md", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result scanFieldsResponse
	require.NoError(t, json.Unmarshal(body, &result))

	t.Run("has_version", func(t *testing.T) {
		assert.NotEmpty(t, result.Version)
	})

	t.Run("has_exchanges", func(t *testing.T) {
		assert.Contains(t, result.Exchanges, "AU")
		assert.Contains(t, result.Exchanges, "US")
		assert.Contains(t, result.Exchanges, "ALL")
	})

	t.Run("has_max_limit", func(t *testing.T) {
		assert.Greater(t, result.MaxLimit, 0)
	})

	t.Run("has_seven_groups", func(t *testing.T) {
		groupNames := make([]string, len(result.Groups))
		for i, g := range result.Groups {
			groupNames[i] = g.Name
		}
		expectedGroups := []string{
			"Identity", "Price & Momentum", "Moving Averages",
			"Oscillators & Indicators", "Volume & Liquidity", "Fundamentals", "Analyst Sentiment",
		}
		for _, eg := range expectedGroups {
			assert.Contains(t, groupNames, eg, "missing group: %s", eg)
		}
	})

	t.Run("fields_have_required_properties", func(t *testing.T) {
		totalFields := 0
		for _, group := range result.Groups {
			for _, f := range group.Fields {
				assert.NotEmpty(t, f.Field, "field name empty in group %s", group.Name)
				assert.NotEmpty(t, f.Type, "type empty for field %s", f.Field)
				assert.NotEmpty(t, f.Description, "description empty for field %s", f.Field)
				totalFields++
			}
		}
		assert.Greater(t, totalFields, 30, "expected at least 30 fields")
	})
}

func TestScanBasic(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
		"exchange": "AU",
		"fields":   []string{"ticker", "name", "sector"},
		"limit":    5,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_scan_basic_response.md", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result scanResponse
	require.NoError(t, json.Unmarshal(body, &result))

	t.Run("has_meta", func(t *testing.T) {
		assert.Equal(t, "AU", result.Meta.Exchange)
		assert.GreaterOrEqual(t, result.Meta.TotalMatched, 0)
		assert.Equal(t, len(result.Results), result.Meta.Returned)
		assert.NotEmpty(t, result.Meta.ExecutedAt)
		assert.GreaterOrEqual(t, result.Meta.QueryTimeMS, 0)
	})

	t.Run("results_have_requested_fields", func(t *testing.T) {
		for _, r := range result.Results {
			assert.Contains(t, r, "ticker")
			assert.Contains(t, r, "name")
			assert.Contains(t, r, "sector")
		}
	})

	t.Run("results_limited", func(t *testing.T) {
		assert.LessOrEqual(t, len(result.Results), 5)
	})
}

func TestScanWithFilters(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
		"exchange": "AU",
		"filters": []map[string]interface{}{
			{"field": "pe_ratio", "op": "<=", "value": 25},
			{"field": "pe_ratio", "op": ">", "value": 0},
		},
		"fields": []string{"ticker", "pe_ratio", "sector"},
		"limit":  10,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_scan_with_filters_response.md", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result scanResponse
	require.NoError(t, json.Unmarshal(body, &result))

	for _, r := range result.Results {
		if pe, ok := r["pe_ratio"]; ok && pe != nil {
			peFloat, isFloat := pe.(float64)
			if isFloat {
				assert.LessOrEqual(t, peFloat, 25.0, "pe_ratio should be <= 25")
				assert.Greater(t, peFloat, 0.0, "pe_ratio should be > 0")
			}
		}
	}
}

func TestScanWithSort(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	t.Run("sort_pe_ratio_asc", func(t *testing.T) {
		resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
			"exchange": "AU",
			"filters": []map[string]interface{}{
				{"field": "pe_ratio", "op": ">", "value": 0},
			},
			"fields": []string{"ticker", "pe_ratio"},
			"sort":   map[string]string{"field": "pe_ratio", "order": "asc"},
			"limit":  10,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("01_sort_pe_asc.md", string(body))

		var result scanResponse
		require.NoError(t, json.Unmarshal(body, &result))

		// Verify ascending order
		if len(result.Results) > 1 {
			for i := 1; i < len(result.Results); i++ {
				prevPE, _ := result.Results[i-1]["pe_ratio"].(float64)
				currPE, _ := result.Results[i]["pe_ratio"].(float64)
				assert.LessOrEqual(t, prevPE, currPE, "results should be sorted ascending by pe_ratio")
			}
		}
	})

	t.Run("sort_market_cap_desc", func(t *testing.T) {
		resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
			"exchange": "AU",
			"filters": []map[string]interface{}{
				{"field": "market_cap", "op": "not_null"},
			},
			"fields": []string{"ticker", "market_cap"},
			"sort":   map[string]string{"field": "market_cap", "order": "desc"},
			"limit":  10,
		})
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("02_sort_market_cap_desc.md", string(body))

		var result scanResponse
		require.NoError(t, json.Unmarshal(body, &result))

		// Verify descending order
		if len(result.Results) > 1 {
			for i := 1; i < len(result.Results); i++ {
				prevCap, _ := result.Results[i-1]["market_cap"].(float64)
				currCap, _ := result.Results[i]["market_cap"].(float64)
				assert.GreaterOrEqual(t, prevCap, currCap, "results should be sorted descending by market_cap")
			}
		}
	})
}

func TestScanMultiSort(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
		"exchange": "AU",
		"filters": []map[string]interface{}{
			{"field": "pe_ratio", "op": ">", "value": 0},
			{"field": "market_cap", "op": "not_null"},
		},
		"fields": []string{"ticker", "market_cap", "pe_ratio"},
		"sort": []map[string]string{
			{"field": "market_cap", "order": "desc"},
			{"field": "pe_ratio", "order": "asc"},
		},
		"limit": 10,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_multi_sort_response.md", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestScanOrFilters(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
		"exchange": "AU",
		"filters": []map[string]interface{}{
			{
				"or": []map[string]interface{}{
					{"field": "sector", "op": "==", "value": "Materials"},
					{"field": "sector", "op": "==", "value": "Technology"},
				},
			},
		},
		"fields": []string{"ticker", "sector"},
		"limit":  20,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_or_filters_response.md", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result scanResponse
	require.NoError(t, json.Unmarshal(body, &result))

	for _, r := range result.Results {
		sector, ok := r["sector"].(string)
		if ok && sector != "" {
			assert.Contains(t, []string{"Materials", "Technology"}, sector,
				"sector should be Materials or Technology, got %s", sector)
		}
	}
}

func TestScanLimitEnforcement(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name     string
		limit    interface{} // nil = omit, 0, 50, 100
		maxCheck int
	}{
		{"default_limit_omitted", nil, 20},
		{"limit_50", 50, 50},
		{"limit_100_capped", 100, 50},
		{"limit_0_default", 0, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]interface{}{
				"exchange": "AU",
				"fields":   []string{"ticker"},
			}
			if tt.limit != nil {
				body["limit"] = tt.limit
			}

			resp, err := env.HTTPPost("/api/scan", body)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			guard.SaveResult("01_"+tt.name+".md", string(respBody))

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var result scanResponse
			require.NoError(t, json.Unmarshal(respBody, &result))

			assert.LessOrEqual(t, result.Meta.Returned, tt.maxCheck,
				"returned should not exceed %d", tt.maxCheck)
		})
	}
}

func TestScanBadRequest(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name       string
		body       map[string]interface{}
		method     string // "POST" or "GET"
		wantStatus int
	}{
		{
			name:       "missing_exchange",
			body:       map[string]interface{}{"fields": []string{"ticker"}},
			method:     "POST",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty_fields",
			body:       map[string]interface{}{"exchange": "AU", "fields": []string{}},
			method:     "POST",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid_field",
			body:       map[string]interface{}{"exchange": "AU", "fields": []string{"nonexistent"}},
			method:     "POST",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp *http.Response
			var err error

			if tt.method == "GET" {
				resp, err = env.HTTPGet("/api/scan")
			} else {
				resp, err = env.HTTPPost("/api/scan", tt.body)
			}
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("01_bad_request_"+tt.name+".md", string(body))

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestScanMethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// GET /api/scan should return 405 (Method Not Allowed) since scan requires POST
	resp, err := env.HTTPGet("/api/scan")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestScanNullableFields(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Request fields that may be null (analyst data, some fundamental fields)
	resp, err := env.HTTPPost("/api/scan", map[string]interface{}{
		"exchange": "AU",
		"fields": []string{
			"ticker", "pe_ratio", "analyst_target_price",
			"analyst_consensus", "beta", "dividend_yield_pct",
		},
		"limit": 5,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("01_nullable_fields_response.md", string(body))

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result scanResponse
	require.NoError(t, json.Unmarshal(body, &result))

	// Verify nullable fields are present in results (value may be null)
	for _, r := range result.Results {
		assert.Contains(t, r, "ticker")
		assert.Contains(t, r, "pe_ratio")
		assert.Contains(t, r, "analyst_target_price")
	}
}
