package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Holding Notes GET/PUT Tests ---

// TestGetHoldingNotesEmpty returns empty collection for portfolio without notes.
func TestGetHoldingNotesEmpty(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create a portfolio
	portfolioName := "HoldingNotesEmptyTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Get holding notes (should be empty)
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/notes", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_get_empty", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	assert.Equal(t, portfolioName, result["portfolio_name"])
	items, ok := result["items"].([]interface{})
	assert.True(t, ok, "items should be a list")
	assert.Len(t, items, 0, "should have no items initially")
}

// TestAddHoldingNoteViaAPI verifies adding a note via POST /notes/items.
func TestAddHoldingNoteViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create a portfolio
	portfolioName := "HoldingNotesAddTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Add a holding note
	notePayload := map[string]interface{}{
		"ticker":            "BHP.AU",
		"name":              "BHP Group",
		"asset_type":        "ASX_stock",
		"liquidity_profile": "high",
		"thesis":            "Diversified mining, strong dividends",
		"known_behaviours":  "Cyclical, commodity-sensitive",
		"signal_overrides":  "Focus on 200-day trend, less on RSI",
		"notes":             "Strong hold for income",
		"stale_days":        90,
	}

	body, _ = json.Marshal(notePayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_add_note", string(respBody))

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	// Verify response
	assert.Equal(t, portfolioName, result["portfolio_name"])
	items, ok := result["items"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, items, 1)

	item := items[0].(map[string]interface{})
	assert.Equal(t, "BHP.AU", item["ticker"])
	assert.Equal(t, "BHP Group", item["name"])
	assert.Equal(t, "ASX_stock", item["asset_type"])
	assert.False(t, item["created_at"].(string) == "", "created_at should be set")
}

// TestUpdateHoldingNoteViaAPI verifies PATCH /notes/items/{ticker}.
func TestUpdateHoldingNoteViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create portfolio and add initial note
	portfolioName := "HoldingNotesUpdateTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Add initial note
	notePayload := map[string]interface{}{
		"ticker": "CBA.AU",
		"name":   "Commonwealth Bank",
		"thesis": "Original thesis",
	}
	body, _ = json.Marshal(notePayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Update the note
	updatePayload := map[string]interface{}{
		"thesis": "Updated thesis with new analysis",
		"notes":  "Modified holding assessment",
	}
	body, _ = json.Marshal(updatePayload)
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/portfolios/"+portfolioName+"/notes/items/CBA.AU", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_update_note", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	items := result["items"].([]interface{})
	item := items[0].(map[string]interface{})
	assert.Equal(t, "Updated thesis with new analysis", item["thesis"])
	assert.Equal(t, "Commonwealth Bank", item["name"]) // Preserved from original
}

// TestRemoveHoldingNoteViaAPI verifies DELETE /notes/items/{ticker}.
func TestRemoveHoldingNoteViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create portfolio and add two notes
	portfolioName := "HoldingNotesRemoveTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Add first note
	note1 := map[string]interface{}{"ticker": "NAB.AU"}
	body, _ = json.Marshal(note1)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Add second note
	note2 := map[string]interface{}{"ticker": "WES.AU"}
	body, _ = json.Marshal(note2)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Delete first note
	resp, err = env.HTTPRequest(http.MethodDelete, "/api/portfolios/"+portfolioName+"/notes/items/NAB.AU", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_remove_note", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	items := result["items"].([]interface{})
	assert.Len(t, items, 1, "should have 1 item after deletion")
	item := items[0].(map[string]interface{})
	assert.Equal(t, "WES.AU", item["ticker"])
}

// TestReplaceHoldingNotesViaAPI verifies PUT /notes (replace all).
func TestReplaceHoldingNotesViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create portfolio
	portfolioName := "HoldingNotesReplaceTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Replace all notes at once
	replacePayload := map[string]interface{}{
		"portfolio_name": portfolioName,
		"notes":          "Portfolio-level context",
		"items": []map[string]interface{}{
			{
				"ticker":            "BHP.AU",
				"name":              "BHP",
				"asset_type":        "ASX_stock",
				"liquidity_profile": "high",
				"thesis":            "Mining play",
			},
			{
				"ticker":            "CBA.AU",
				"name":              "CBA",
				"asset_type":        "ASX_stock",
				"liquidity_profile": "high",
				"thesis":            "Banking play",
			},
		},
	}

	body, _ = json.Marshal(replacePayload)
	resp, err = env.HTTPRequest(http.MethodPut, "/api/portfolios/"+portfolioName+"/notes", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_replace_notes", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	assert.Equal(t, "Portfolio-level context", result["notes"])
	items := result["items"].([]interface{})
	assert.Len(t, items, 2)
}

// TestAddHoldingNoteValidationErrors verifies error handling on invalid input.
func TestAddHoldingNoteValidationErrors(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create portfolio
	portfolioName := "HoldingNotesValidationTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
	}{
		{
			name: "missing ticker",
			payload: map[string]interface{}{
				"name":       "BHP",
				"asset_type": "ASX_stock",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid asset type",
			payload: map[string]interface{}{
				"ticker":     "BHP.AU",
				"asset_type": "invalid_type",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			guard.SaveResult("error_"+tt.name, string(respBody))

			assert.Equal(t, tt.expectedStatus, resp.StatusCode,
				"expected %d for %s, got %d", tt.expectedStatus, tt.name, resp.StatusCode)
		})
	}
}

// TestHoldingNotesCaseInsensitiveTicker verifies ticker matching is case-insensitive.
func TestHoldingNotesCaseInsensitiveTicker(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create portfolio
	portfolioName := "HoldingNotesCaseTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Add note with uppercase ticker
	notePayload := map[string]interface{}{
		"ticker": "BHP.AU",
		"thesis": "Original",
	}
	body, _ = json.Marshal(notePayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Update with lowercase ticker
	updatePayload := map[string]interface{}{
		"thesis": "Updated with lowercase",
	}
	body, _ = json.Marshal(updatePayload)
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/portfolios/"+portfolioName+"/notes/items/bhp.au", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_case_insensitive", string(respBody))

	// Should succeed (case-insensitive)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))
	items := result["items"].([]interface{})
	item := items[0].(map[string]interface{})
	assert.Equal(t, "Updated with lowercase", item["thesis"])
}

// TestHoldingNotesAllAssetTypes verifies all asset types are accepted.
func TestHoldingNotesAllAssetTypes(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create portfolio
	portfolioName := "HoldingNotesAssetTypesTest"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	assetTypes := []struct {
		ticker    string
		assetType string
	}{
		{"VAS.AU", "ETF"},
		{"BHP.AU", "ASX_stock"},
		{"AAPL.US", "US_equity"},
	}

	for _, at := range assetTypes {
		notePayload := map[string]interface{}{
			"ticker":     at.ticker,
			"asset_type": at.assetType,
		}
		body, _ := json.Marshal(notePayload)
		resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/notes/items", bytes.NewReader(body), userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusCreated, resp.StatusCode, "should accept asset type %s", at.assetType)
	}

	guard.SaveResult("01_asset_types", "All asset types accepted")

	// Verify all were added
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/notes", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	items := result["items"].([]interface{})
	assert.Len(t, items, 3, "should have 3 notes for 3 asset types")
}
