package market

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
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

	svc := &Service{storage: &mockStorageManager{}}
	prompt := svc.buildFilingSummaryPrompt("SKS.AU", batch)

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

func TestExtractPDFTextFromBytes_EmptyData(t *testing.T) {
	text, err := ExtractPDFTextFromBytes([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
	if text != "" {
		t.Error("expected empty text for empty data")
	}
}

func TestExtractPDFTextFromBytes_CorruptData(t *testing.T) {
	// This is a minimal corrupt PDF that triggers zlib issues
	corruptData := []byte("%PDF-1.4\ncorrupt data that should cause an error")

	// Should not panic — should return error gracefully
	text, err := ExtractPDFTextFromBytes(corruptData)
	// Either returns an error or empty text (depending on pdf lib behavior)
	// The key assertion is that this does NOT panic
	_ = text
	_ = err
}

func TestFilingSummaryPromptHash_Stable(t *testing.T) {
	h1 := filingSummaryPromptHash()
	h2 := filingSummaryPromptHash()
	if h1 != h2 {
		t.Errorf("prompt hash not stable: %s != %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("expected 64-char SHA-256 hex, got %d chars", len(h1))
	}
}

func TestFilingSummaryPromptHash_NonEmpty(t *testing.T) {
	h := filingSummaryPromptHash()
	if h == "" {
		t.Error("expected non-empty prompt hash")
	}
}

// --- ReadFiling tests ---

func newTestService(marketData map[string]*models.MarketData, files map[string][]byte) *Service {
	storage := &mockStorageManager{
		market: &mockMarketDataStorage{data: marketData},
		files:  &mockFileStore{files: files},
	}
	logger := common.NewLogger("error")
	return NewService(storage, nil, nil, logger)
}

func TestReadFiling_Success(t *testing.T) {
	// Create a minimal valid PDF
	pdfBytes := buildMinimalPDF()

	marketData := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU",
			Filings: []models.CompanyFiling{
				{
					Date:           time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC),
					Headline:       "Full Year Results",
					Type:           "Annual Report",
					DocumentKey:    "03063826",
					PriceSensitive: true,
					Relevance:      "HIGH",
					PDFURL:         "https://www.asx.com.au/asx/v2/statistics/displayAnnouncement.do?display=pdf&idsId=03063826",
					PDFPath:        "BHP/20250820-03063826.pdf",
				},
			},
		},
	}

	files := map[string][]byte{
		"filing_pdf/BHP/20250820-03063826.pdf": pdfBytes,
	}

	svc := newTestService(marketData, files)
	result, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err != nil {
		t.Fatalf("ReadFiling failed: %v", err)
	}

	if result.Ticker != "BHP.AU" {
		t.Errorf("ticker = %s, want BHP.AU", result.Ticker)
	}
	if result.DocumentKey != "03063826" {
		t.Errorf("document_key = %s, want 03063826", result.DocumentKey)
	}
	if result.Headline != "Full Year Results" {
		t.Errorf("headline = %s, want Full Year Results", result.Headline)
	}
	if result.Type != "Annual Report" {
		t.Errorf("type = %s, want Annual Report", result.Type)
	}
	if !result.PriceSensitive {
		t.Error("expected price_sensitive = true")
	}
	if result.Relevance != "HIGH" {
		t.Errorf("relevance = %s, want HIGH", result.Relevance)
	}
	if result.TextLength != len(result.Text) {
		t.Errorf("text_length = %d, but len(text) = %d", result.TextLength, len(result.Text))
	}
	if result.PageCount < 1 {
		t.Errorf("page_count = %d, want >= 1", result.PageCount)
	}
}

func TestReadFiling_DocumentKeyNotFound(t *testing.T) {
	marketData := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU",
			Filings: []models.CompanyFiling{
				{DocumentKey: "111111"},
			},
		},
	}

	svc := newTestService(marketData, nil)
	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "999999")
	if err == nil {
		t.Fatal("expected error for missing document key")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found' substring", err)
	}
}

func TestReadFiling_NoMarketData(t *testing.T) {
	svc := newTestService(map[string]*models.MarketData{}, nil)
	_, err := svc.ReadFiling(context.Background(), "UNKNOWN.AU", "123")
	if err == nil {
		t.Fatal("expected error for missing market data")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found' substring", err)
	}
}

func TestReadFiling_PDFPathEmpty(t *testing.T) {
	marketData := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU",
			Filings: []models.CompanyFiling{
				{
					DocumentKey: "03063826",
					PDFPath:     "", // PDF not downloaded
				},
			},
		},
	}

	svc := newTestService(marketData, nil)
	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Fatal("expected error for empty PDF path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %v, want 'not found' substring", err)
	}
}

func TestReadFiling_FileStoreError(t *testing.T) {
	marketData := map[string]*models.MarketData{
		"BHP.AU": {
			Ticker: "BHP.AU",
			Filings: []models.CompanyFiling{
				{
					DocumentKey: "03063826",
					PDFPath:     "BHP/20250820-03063826.pdf",
				},
			},
		},
	}

	// Empty file store - file not present
	svc := newTestService(marketData, map[string][]byte{})
	_, err := svc.ReadFiling(context.Background(), "BHP.AU", "03063826")
	if err == nil {
		t.Fatal("expected error when file not in store")
	}
	if !strings.Contains(err.Error(), "failed to read PDF") {
		t.Errorf("error = %v, want 'failed to read PDF' substring", err)
	}
}

// buildMinimalPDF creates a minimal valid PDF with one page.
// Byte offsets are computed correctly for the cross-reference table.
func buildMinimalPDF() []byte {
	// Build each object, tracking byte offsets
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\nendobj\n",
	}

	content := "BT /F1 12 Tf 100 700 Td (Test content) Tj ET"
	stream := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	objects = append(objects, stream)
	objects = append(objects, "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	header := "%PDF-1.4\n"
	offsets := make([]int, len(objects))
	pos := len(header)
	for i, obj := range objects {
		offsets[i] = pos
		pos += len(obj)
	}

	xrefOffset := pos

	var buf strings.Builder
	buf.WriteString(header)
	for _, obj := range objects {
		buf.WriteString(obj)
	}

	buf.WriteString(fmt.Sprintf("xref\n0 %d\n", len(objects)+1))
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		buf.WriteString(fmt.Sprintf("%010d 00000 n \n", off))
	}

	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\n", len(objects)+1))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefOffset))

	return []byte(buf.String())
}

func TestParseFilingSummaryResponse_FinancialFields(t *testing.T) {
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
			"guidance_revenue": "",
			"guidance_profit": "",
			"financial_summary": "Revenue grew 92% to $261.7M with net profit doubling to $14.0M",
			"performance_commentary": "Management noted strong demand in data centres driving margin expansion",
			"key_facts": ["Revenue $261.7M"],
			"period": "FY2025"
		}
	]`

	batch := []models.CompanyFiling{
		{Date: time.Date(2025, 8, 20, 0, 0, 0, 0, time.UTC), Headline: "Full Year Results", DocumentKey: "001", PriceSensitive: true},
	}

	summaries := parseFilingSummaryResponse(response, batch)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}

	s := summaries[0]
	if s.FinancialSummary != "Revenue grew 92% to $261.7M with net profit doubling to $14.0M" {
		t.Errorf("financial_summary = %s", s.FinancialSummary)
	}
	if s.PerformanceCommentary != "Management noted strong demand in data centres driving margin expansion" {
		t.Errorf("performance_commentary = %s", s.PerformanceCommentary)
	}
}
