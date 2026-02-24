package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Helpers ---

// filingContentResponse represents the GET /api/market/stocks/{ticker}/filings/{document_key} response.
type filingContentResponse struct {
	Ticker         string `json:"ticker"`
	DocumentKey    string `json:"document_key"`
	Date           string `json:"date"`
	Headline       string `json:"headline"`
	Type           string `json:"type"`
	PriceSensitive bool   `json:"price_sensitive"`
	Relevance      string `json:"relevance"`
	PDFURL         string `json:"pdf_url"`
	PDFPath        string `json:"pdf_path"`
	Text           string `json:"text"`
	TextLength     int    `json:"text_length"`
	PageCount      int    `json:"page_count"`
}

// findFilingDocumentKey fetches stock data for a ticker and returns the first filing document_key
// that has a non-empty pdf_path. Returns empty string if none found.
// Uses a 15s timeout to avoid hanging when the server triggers external data collection.
func findFilingDocumentKey(t *testing.T, env *common.Env, ticker string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.ServerURL()+"/api/market/stocks/"+ticker, nil)
	if err != nil {
		t.Logf("Failed to create request for %s: %v", ticker, err)
		return ""
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("Stock data request for %s failed (likely timeout — no pre-seeded data): %v", ticker, err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Stock data request for %s returned status %d", ticker, resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return ""
	}

	// Navigate to filings array in the market data
	marketData, ok := data["market_data"].(map[string]interface{})
	if !ok {
		t.Logf("No market_data in response for %s", ticker)
		return ""
	}

	filings, ok := marketData["filings"].([]interface{})
	if !ok || len(filings) == 0 {
		t.Logf("No filings in market data for %s", ticker)
		return ""
	}

	for _, f := range filings {
		filing, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		docKey, _ := filing["document_key"].(string)
		pdfPath, _ := filing["pdf_path"].(string)
		if docKey != "" && pdfPath != "" {
			return docKey
		}
	}

	// Fall back to any filing with a document key (even without pdf_path)
	for _, f := range filings {
		filing, ok := f.(map[string]interface{})
		if !ok {
			continue
		}
		docKey, _ := filing["document_key"].(string)
		if docKey != "" {
			return docKey
		}
	}

	return ""
}

// --- Error Path Tests (no external data dependency) ---

func TestReadFiling_InvalidTicker(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{
			name:       "empty_ticker",
			path:       "/api/market/stocks//filings/12345",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ticker_without_exchange",
			path:       "/api/market/stocks/BHP/filings/12345",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "ticker_with_special_chars",
			path:       "/api/market/stocks/BH$P.AU/filings/12345",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPGet(tt.path)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("invalid_ticker_"+tt.name, string(body))

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
		})
	}
}

func TestReadFiling_MissingDocumentKey(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Path with trailing slash but no document key resolves to empty docKey
	resp, err := env.HTTPGet("/api/market/stocks/BHP.AU/filings/")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("missing_document_key", string(body))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestReadFiling_NonexistentDocumentKey(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Use a ticker that likely has no market data in the test environment
	resp, err := env.HTTPGet("/api/market/stocks/BHP.AU/filings/99999999")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("nonexistent_document_key", string(body))

	// Should be 404 — either no market data or document key not found
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestReadFiling_MethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name   string
		method string
	}{
		{"post", http.MethodPost},
		{"put", http.MethodPut},
		{"delete", http.MethodDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPRequest(tt.method, "/api/market/stocks/BHP.AU/filings/12345", nil, nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("method_not_allowed_"+tt.name, string(body))

			assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
		})
	}
}

// --- Happy Path Test (data-dependent, skips gracefully) ---

func TestReadFiling_WithFilingData(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Try to find a filing with a document key in the test environment.
	// Filing data may not be available (depends on market data collection).
	ticker := "BHP.AU"
	docKey := findFilingDocumentKey(t, env, ticker)
	if docKey == "" {
		t.Skip("No filings with document keys available in test environment for " + ticker)
		return
	}

	t.Logf("Found filing document_key=%s for %s", docKey, ticker)

	resp, err := env.HTTPGet("/api/market/stocks/" + ticker + "/filings/" + docKey)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("read_filing_success", string(body))

	if resp.StatusCode == http.StatusNotFound {
		// Filing exists but PDF not downloaded — acceptable in test environment
		t.Logf("Filing found but PDF not available (404): %s", string(body))
		return
	}

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var result filingContentResponse
	require.NoError(t, json.Unmarshal(body, &result))

	t.Run("has_ticker", func(t *testing.T) {
		assert.Equal(t, ticker, result.Ticker)
	})

	t.Run("has_document_key", func(t *testing.T) {
		assert.Equal(t, docKey, result.DocumentKey)
	})

	t.Run("has_metadata", func(t *testing.T) {
		assert.NotEmpty(t, result.Date)
		assert.NotEmpty(t, result.Headline)
		assert.NotEmpty(t, result.PDFURL)
	})

	t.Run("has_text_content", func(t *testing.T) {
		assert.NotEmpty(t, result.Text)
		assert.Greater(t, result.TextLength, 0)
		assert.Equal(t, len(result.Text), result.TextLength)
	})

	t.Run("has_page_count", func(t *testing.T) {
		assert.Greater(t, result.PageCount, 0)
	})

	t.Run("text_within_limits", func(t *testing.T) {
		assert.LessOrEqual(t, result.TextLength, 50000, "text should be truncated to 50000 chars")
	})
}

// --- MCP Tool Catalog Test ---

func TestReadFiling_InToolCatalog(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPGet("/api/mcp/tools")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("tool_catalog", string(body))

	require.Equal(t, http.StatusOK, resp.StatusCode)

	var tools []map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &tools))

	found := false
	for _, tool := range tools {
		if tool["name"] == "read_filing" {
			found = true
			assert.Equal(t, "GET", tool["method"])
			assert.Equal(t, "/api/market/stocks/{ticker}/filings/{document_key}", tool["path"])

			params, ok := tool["params"].([]interface{})
			assert.True(t, ok, "params should be an array")
			assert.Len(t, params, 2)
			break
		}
	}

	assert.True(t, found, "read_filing tool should be in the MCP tool catalog")
}
