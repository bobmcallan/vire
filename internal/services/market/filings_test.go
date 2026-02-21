package market

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestParseFilingSummaryResponse_ValidJSON(t *testing.T) {
	response := `[
		{
			"type": "financial_results",
			"revenue": "$261.7M",
			"revenue_growth": "+92%",
			"profit": "$14.0M",
			"profit_growth": "+112%",
			"margin": "5.4%",
			"eps": "$0.12",
			"dividend": "$0.06",
			"contract_value": "",
			"customer": "",
			"acq_target": "",
			"acq_price": "",
			"guidance_revenue": "$340M",
			"guidance_profit": "$34M PBT",
			"key_facts": ["Revenue $261.7M, up 92% YoY", "Net profit $14.0M"],
			"period": "FY2025"
		},
		{
			"type": "contract",
			"revenue": "",
			"revenue_growth": "",
			"profit": "",
			"profit_growth": "",
			"margin": "",
			"eps": "",
			"dividend": "",
			"contract_value": "$130M",
			"customer": "NEXTDC",
			"acq_target": "",
			"acq_price": "",
			"guidance_revenue": "",
			"guidance_profit": "",
			"key_facts": ["Major data centre contract with NEXTDC worth $130M"],
			"period": ""
		}
	]`

	batch := []models.CompanyFiling{
		{Date: time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC), Headline: "Full Year Results", DocumentKey: "001", PriceSensitive: true},
		{Date: time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC), Headline: "Major Contract Award", DocumentKey: "002", PriceSensitive: true},
	}

	summaries := parseFilingSummaryResponse(response, batch)

	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}

	// First summary: financial results
	s := summaries[0]
	if s.Type != "financial_results" {
		t.Errorf("type = %s, want financial_results", s.Type)
	}
	if s.Revenue != "$261.7M" {
		t.Errorf("revenue = %s, want $261.7M", s.Revenue)
	}
	if s.RevenueGrowth != "+92%" {
		t.Errorf("revenue_growth = %s, want +92%%", s.RevenueGrowth)
	}
	if s.GuidanceRevenue != "$340M" {
		t.Errorf("guidance_revenue = %s, want $340M", s.GuidanceRevenue)
	}
	if s.Period != "FY2025" {
		t.Errorf("period = %s, want FY2025", s.Period)
	}
	if len(s.KeyFacts) != 2 {
		t.Errorf("key_facts length = %d, want 2", len(s.KeyFacts))
	}
	if s.DocumentKey != "001" {
		t.Errorf("document_key = %s, want 001", s.DocumentKey)
	}
	if !s.PriceSensitive {
		t.Error("expected price_sensitive = true")
	}

	// Second summary: contract
	s2 := summaries[1]
	if s2.Type != "contract" {
		t.Errorf("type = %s, want contract", s2.Type)
	}
	if s2.ContractValue != "$130M" {
		t.Errorf("contract_value = %s, want $130M", s2.ContractValue)
	}
	if s2.Customer != "NEXTDC" {
		t.Errorf("customer = %s, want NEXTDC", s2.Customer)
	}
}

func TestParseFilingSummaryResponse_MarkdownFences(t *testing.T) {
	response := "```json\n[{\"type\": \"other\", \"key_facts\": [\"Trading halt\"]}]\n```"

	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "Trading Halt"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].Type != "other" {
		t.Errorf("type = %s, want other", summaries[0].Type)
	}
}

func TestParseFilingSummaryResponse_InvalidJSON(t *testing.T) {
	summaries := parseFilingSummaryResponse("not json", nil)
	if summaries != nil {
		t.Error("expected nil for invalid JSON")
	}
}

func TestParseFilingSummaryResponse_MoreResultsThanBatch(t *testing.T) {
	// Gemini returns more items than the batch — should truncate to batch size
	response := `[{"type":"a","key_facts":[]},{"type":"b","key_facts":[]},{"type":"c","key_facts":[]}]`
	batch := []models.CompanyFiling{
		{Date: time.Now(), Headline: "One"},
		{Date: time.Now(), Headline: "Two"},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 2 {
		t.Errorf("expected 2 summaries (capped to batch), got %d", len(summaries))
	}
}

func TestParseTimelineResponse_ValidJSON(t *testing.T) {
	response := `{
		"business_model": "Engineering services company providing electrical and communications infrastructure.",
		"sector": "Industrials",
		"industry": "Engineering Services",
		"periods": [
			{
				"period": "FY2025",
				"revenue": "$261.7M",
				"revenue_growth": "+92%",
				"profit": "$14.0M",
				"profit_growth": "+112%",
				"margin": "5.4%",
				"eps": "$0.12",
				"dividend": "$0.06",
				"guidance_given": "FY2026 revenue $340M",
				"guidance_outcome": ""
			}
		],
		"key_events": [
			{"date": "2026-02-05", "event": "Major contract + guidance upgrade", "detail": "$60M new contracts", "impact": "positive"}
		],
		"next_reporting_date": "2026-08-20",
		"work_on_hand": "$560M",
		"repeat_business_rate": "94%"
	}`

	tl := parseTimelineResponse(response)
	if tl == nil {
		t.Fatal("parseTimelineResponse returned nil")
	}

	if tl.BusinessModel == "" {
		t.Error("expected non-empty business_model")
	}
	if len(tl.Periods) != 1 {
		t.Fatalf("periods length = %d, want 1", len(tl.Periods))
	}
	if tl.Periods[0].Revenue != "$261.7M" {
		t.Errorf("periods[0].revenue = %s, want $261.7M", tl.Periods[0].Revenue)
	}
	if tl.Periods[0].GuidanceGiven != "FY2026 revenue $340M" {
		t.Errorf("guidance_given = %s", tl.Periods[0].GuidanceGiven)
	}
	if len(tl.KeyEvents) != 1 {
		t.Fatalf("key_events length = %d, want 1", len(tl.KeyEvents))
	}
	if tl.KeyEvents[0].Impact != "positive" {
		t.Errorf("key_events[0].impact = %s, want positive", tl.KeyEvents[0].Impact)
	}
	if tl.WorkOnHand != "$560M" {
		t.Errorf("work_on_hand = %s, want $560M", tl.WorkOnHand)
	}
	if tl.RepeatBusinessRate != "94%" {
		t.Errorf("repeat_business_rate = %s, want 94%%", tl.RepeatBusinessRate)
	}
}

func TestParseTimelineResponse_MarkdownFences(t *testing.T) {
	response := "```json\n{\"business_model\": \"Test\", \"periods\": []}\n```"
	tl := parseTimelineResponse(response)
	if tl == nil {
		t.Fatal("expected non-nil for markdown-fenced response")
	}
	if tl.BusinessModel != "Test" {
		t.Errorf("business_model = %s, want Test", tl.BusinessModel)
	}
}

func TestParseTimelineResponse_EmptyResponse(t *testing.T) {
	tl := parseTimelineResponse(`{"business_model": "", "periods": []}`)
	if tl != nil {
		t.Error("expected nil for empty business_model and no periods")
	}
}

func TestFilingSummaryKey_IgnoresDocumentKey(t *testing.T) {
	date := time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC)
	key := filingSummaryKey("abc123", date, "Some Headline")
	if key != "2025-08-20|Some Headline" {
		t.Errorf("expected date|headline key even with docKey, got %s", key)
	}
}

func TestFilingSummaryKey_WithoutDocumentKey(t *testing.T) {
	date := time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC)
	key := filingSummaryKey("", date, "Full Year Results")
	if key != "2025-08-20|Full Year Results" {
		t.Errorf("expected date|headline key, got %s", key)
	}
}

func TestBuildFilingSummaryPrompt_HeadlineOnlyFallback(t *testing.T) {
	batch := []models.CompanyFiling{
		{
			Date:           time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
			Headline:       "Major Contract Award and FY26 Profit Upgrade",
			Type:           "Announcement",
			PriceSensitive: true,
		},
		{
			Date:           time.Date(2025, 11, 18, 0, 0, 0, 0, time.UTC),
			Headline:       "SKS Acquires Delta Elcom",
			Type:           "Announcement",
			PriceSensitive: true,
		},
	}

	prompt := buildFilingSummaryPrompt("SKS.AU", batch)

	// Verify headline-only instruction is present (these filings have no PDFPath)
	if !strings.Contains(prompt, "No document content available") {
		t.Error("prompt should contain headline-only extraction instruction for filings without PDF")
	}

	// Verify both filings are in the prompt
	if !strings.Contains(prompt, "Major Contract Award") {
		t.Error("prompt should contain first filing headline")
	}
	if !strings.Contains(prompt, "Delta Elcom") {
		t.Error("prompt should contain second filing headline")
	}
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"```json\n{}\n```", "{}"},
		{"```\n[]\n```", "[]"},
		{"{}", "{}"},
		{"  ```json\n{}\n```  ", "{}"},
	}
	for _, tt := range tests {
		got := stripMarkdownFences(tt.input)
		if got != tt.want {
			t.Errorf("stripMarkdownFences(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractPDFText_NonExistentFile(t *testing.T) {
	text, err := extractPDFText("/nonexistent/file.pdf")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
	if text != "" {
		t.Error("expected empty text for non-existent file")
	}
}

func TestExtractPDFText_CorruptFile(t *testing.T) {
	// Write a corrupt "PDF" file that has %PDF header but garbage data
	tmpDir := t.TempDir()
	corruptPath := tmpDir + "/corrupt.pdf"
	// This is a minimal corrupt PDF that triggers zlib issues
	corruptData := []byte("%PDF-1.4\ncorrupt data that should cause an error")
	if err := os.WriteFile(corruptPath, corruptData, 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Should not panic — should return error gracefully
	text, err := extractPDFText(corruptPath)
	// Either returns an error or empty text (depending on pdf lib behavior)
	// The key assertion is that this does NOT panic
	_ = text
	_ = err
}
