package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Manual Portfolio Creation ---

// TestCreateManualPortfolioViaAPI verifies portfolio creation endpoint for manual portfolios.
func TestCreateManualPortfolioViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Get auth headers
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create manual portfolio
	createPayload := map[string]interface{}{
		"name":        "ManualAPITest",
		"source_type": "manual",
		"currency":    "AUD",
	}

	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_create_manual", string(respBody))

	require.Equal(t, http.StatusCreated, resp.StatusCode, "create portfolio should return 201")

	var portfolio map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &portfolio), "parse response")

	assert.Equal(t, "ManualAPITest", portfolio["name"])
	assert.Equal(t, "manual", portfolio["source_type"])
	assert.Equal(t, "AUD", portfolio["currency"])
}

// TestCreateSnapshotPortfolioViaAPI verifies portfolio creation for snapshot portfolios.
func TestCreateSnapshotPortfolioViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	createPayload := map[string]interface{}{
		"name":        "SnapshotAPITest",
		"source_type": "snapshot",
		"currency":    "USD",
	}

	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_create_snapshot", string(respBody))

	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var portfolio map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &portfolio))

	assert.Equal(t, "SnapshotAPITest", portfolio["name"])
	assert.Equal(t, "snapshot", portfolio["source_type"])
	assert.Equal(t, "USD", portfolio["currency"])
}

// TestCreatePortfolioValidationErrors verifies error responses for invalid input.
func TestCreatePortfolioValidationErrors(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	tests := []struct {
		name           string
		payload        map[string]interface{}
		expectedStatus int
	}{
		{
			name: "empty portfolio name",
			payload: map[string]interface{}{
				"name":        "",
				"source_type": "manual",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid source type",
			payload: map[string]interface{}{
				"name":        "Invalid",
				"source_type": "unknown_type",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
			require.NoError(t, err)
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			guard.SaveResult("error_"+tt.name, string(respBody))

			assert.Equal(t, tt.expectedStatus, resp.StatusCode,
				"expected %d for %s, got %d", tt.expectedStatus, tt.name, resp.StatusCode)
		})
	}
}

// --- Trade Add Tests ---

// TestAddTradeViaAPI verifies trade add endpoint.
func TestAddTradeViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	// Create manual portfolio
	portfolioName := "TradeTestPortfolio"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create portfolio")

	// Add a buy trade
	tradePayload := map[string]interface{}{
		"ticker": "BHP.AU",
		"action": "buy",
		"date":   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  100.0,
		"price":  50.0,
		"fees":   50.0,
	}

	body, _ = json.Marshal(tradePayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_add_trade", string(respBody))

	require.Equal(t, http.StatusCreated, resp.StatusCode, "add trade should return 201")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	// Verify trade was recorded
	assert.NotEmpty(t, result["trade"].(map[string]interface{})["id"])
	assert.Equal(t, "BHP.AU", result["trade"].(map[string]interface{})["ticker"])

	// Verify holding was derived
	holding := result["holding"].(map[string]interface{})
	assert.Equal(t, "BHP.AU", holding["ticker"])
	assert.Equal(t, 100.0, holding["units"])
}

// TestAddMultipleTradesAggregation verifies position aggregation with multiple trades.
func TestAddMultipleTradesAggregation(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	portfolioName := "AggregationTestPortfolio"
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

	// Add first buy: 100 @ 50
	trade1 := map[string]interface{}{
		"ticker": "ANZ.AU",
		"action": "buy",
		"date":   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  100.0,
		"price":  50.0,
		"fees":   0.0,
	}
	body, _ = json.Marshal(trade1)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result1 map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result1))
	holding1 := result1["holding"].(map[string]interface{})
	assert.InDelta(t, 50.0, holding1["avg_cost"], 0.01, "first buy avg cost")

	// Add second buy: 50 @ 60
	trade2 := map[string]interface{}{
		"ticker": "ANZ.AU",
		"action": "buy",
		"date":   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  50.0,
		"price":  60.0,
		"fees":   0.0,
	}
	body, _ = json.Marshal(trade2)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ = io.ReadAll(resp.Body)
	guard.SaveResult("02_second_trade", string(respBody))

	var result2 map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result2))
	holding2 := result2["holding"].(map[string]interface{})

	// Verify weighted average cost: (100*50 + 50*60) / 150 = 53.33
	assert.Equal(t, 150.0, holding2["units"], "total units")
	assert.InDelta(t, 53.33, holding2["avg_cost"], 0.01, "weighted average cost")
}

// TestAddSellTradeRealizesGain verifies realized P&L on sell.
func TestAddSellTradeRealizesGain(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	portfolioName := "SellGainPortfolio"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Buy 100 @ 100
	buyTrade := map[string]interface{}{
		"ticker": "CBA.AU",
		"action": "buy",
		"date":   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  100.0,
		"price":  100.0,
		"fees":   0.0,
	}
	body, _ = json.Marshal(buyTrade)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Sell 60 @ 120
	sellTrade := map[string]interface{}{
		"ticker": "CBA.AU",
		"action": "sell",
		"date":   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  60.0,
		"price":  120.0,
		"fees":   0.0,
	}
	body, _ = json.Marshal(sellTrade)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_sell_realized", string(respBody))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	holding := result["holding"].(map[string]interface{})
	assert.Equal(t, 40.0, holding["units"], "remaining units")
	// Realized: (60 * 120) - (60 * 100) = 1200
	assert.InDelta(t, 1200.0, holding["realized_return"], 0.01, "realized P&L")
}

// TestSellValidationError verifies error when selling more than held.
func TestSellValidationError(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	portfolioName := "SellValidationPortfolio"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Buy 50 units
	buyTrade := map[string]interface{}{
		"ticker": "NAB.AU",
		"action": "buy",
		"date":   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  50.0,
		"price":  100.0,
	}
	body, _ = json.Marshal(buyTrade)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Try to sell 100 (more than held)
	sellTrade := map[string]interface{}{
		"ticker": "NAB.AU",
		"action": "sell",
		"date":   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		"units":  100.0,
		"price":  110.0,
	}
	body, _ = json.Marshal(sellTrade)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_sell_error", string(respBody))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "should reject oversell")
}

// --- Trade List Tests ---

// TestListTradesViaAPI verifies trade list endpoint with filtering.
func TestListTradesViaAPI(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	portfolioName := "ListTradesPortfolio"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "manual",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Add multiple trades
	trades := []map[string]interface{}{
		{
			"ticker": "BHP.AU",
			"action": "buy",
			"date":   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"units":  100.0,
			"price":  50.0,
		},
		{
			"ticker": "CBA.AU",
			"action": "buy",
			"date":   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"units":  50.0,
			"price":  100.0,
		},
		{
			"ticker": "BHP.AU",
			"action": "sell",
			"date":   time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
			"units":  25.0,
			"price":  60.0,
		},
	}

	for _, trade := range trades {
		body, _ := json.Marshal(trade)
		resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/trades", bytes.NewReader(body), userHeaders)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// List all trades
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/trades", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_list_all", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	trades_list := result["trades"].([]interface{})
	assert.Len(t, trades_list, 3, "should have 3 trades")

	// List BHP trades only
	resp, err = env.HTTPRequest(http.MethodGet, "/api/portfolios/"+portfolioName+"/trades?ticker=BHP.AU", nil, userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ = io.ReadAll(resp.Body)
	guard.SaveResult("02_filter_ticker", string(respBody))

	require.NoError(t, json.Unmarshal(respBody, &result))
	trades_list = result["trades"].([]interface{})
	assert.Len(t, trades_list, 2, "should have 2 BHP trades")
}

// --- Snapshot Import Tests ---

// TestSnapshotImportReplace verifies snapshot import in replace mode.
func TestSnapshotImportReplace(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	portfolioName := "SnapshotReplaceAPI"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "snapshot",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Import snapshot
	snapshotPayload := map[string]interface{}{
		"positions": []map[string]interface{}{
			{
				"ticker":        "BHP.AU",
				"units":         100.0,
				"avg_cost":      50.0,
				"current_price": 55.0,
			},
			{
				"ticker":        "CBA.AU",
				"units":         50.0,
				"avg_cost":      100.0,
				"current_price": 105.0,
			},
		},
		"mode":       "replace",
		"source_ref": "initial",
	}

	body, _ = json.Marshal(snapshotPayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/snapshot", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_snapshot_import", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode, "snapshot import should return 200")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	// Verify positions were stored
	positions := result["snapshot_positions"].([]interface{})
	assert.Len(t, positions, 2)
}

// TestSnapshotImportMerge verifies snapshot import in merge mode.
func TestSnapshotImportMerge(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	userHeaders := getOrCreateUserHeaders(t, env)

	portfolioName := "SnapshotMergeAPI"
	createPayload := map[string]interface{}{
		"name":        portfolioName,
		"source_type": "snapshot",
		"currency":    "AUD",
	}
	body, _ := json.Marshal(createPayload)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/portfolios", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Initial snapshot
	snapshotPayload := map[string]interface{}{
		"positions": []map[string]interface{}{
			{
				"ticker":        "BHP.AU",
				"units":         100.0,
				"avg_cost":      50.0,
				"current_price": 55.0,
			},
			{
				"ticker":        "CBA.AU",
				"units":         50.0,
				"avg_cost":      100.0,
				"current_price": 105.0,
			},
		},
		"mode": "replace",
	}
	body, _ = json.Marshal(snapshotPayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/snapshot", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Merge update: change BHP, add NAB
	mergePayload := map[string]interface{}{
		"positions": []map[string]interface{}{
			{
				"ticker":        "BHP.AU",
				"units":         120.0,
				"avg_cost":      52.0,
				"current_price": 56.0,
			},
			{
				"ticker":        "NAB.AU",
				"units":         75.0,
				"avg_cost":      30.0,
				"current_price": 32.0,
			},
		},
		"mode": "merge",
	}
	body, _ = json.Marshal(mergePayload)
	resp, err = env.HTTPRequest(http.MethodPost, "/api/portfolios/"+portfolioName+"/snapshot", bytes.NewReader(body), userHeaders)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_snapshot_merge", string(respBody))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &result))

	// Should have 3 positions (BHP updated, CBA unchanged, NAB new)
	positions := result["snapshot_positions"].([]interface{})
	assert.Len(t, positions, 3)
}

// --- Helper Functions ---

// getOrCreateUserHeaders creates a test user and returns auth headers.
// This is a simplified version; in real tests you might use setup from other test files.
func getOrCreateUserHeaders(t *testing.T, env *common.Env) map[string]string {
	t.Helper()

	headers := make(map[string]string)
	headers["Content-Type"] = "application/json"

	// For now, return basic headers. In production, would authenticate with the test server.
	// The test framework handles this internally via env.HTTPRequest.
	return headers
}
