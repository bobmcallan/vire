package market

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/ledongthuc/pdf"
)

// --- ASX Announcement Scraping ---

// fetchASXAnnouncements scrapes announcements from ASX HTML pages.
func (s *Service) fetchASXAnnouncements(ctx context.Context, tickerCode, period string) ([]models.CompanyFiling, error) {
	currentYear := time.Now().Year()

	// Determine years to fetch based on period
	yearsToFetch := 2
	switch period {
	case "Y2":
		yearsToFetch = 3
	case "Y3":
		yearsToFetch = 4
	case "Y5":
		yearsToFetch = 6
	}

	var allFilings []models.CompanyFiling

	client := &http.Client{Timeout: 30 * time.Second}

	for yearOffset := 0; yearOffset < yearsToFetch; yearOffset++ {
		year := currentYear - yearOffset

		url := fmt.Sprintf("https://www.asx.com.au/asx/v2/statistics/announcements.do?by=asxCode&asxCode=%s&timeframe=Y&year=%d",
			strings.ToUpper(tickerCode), year)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml")

		resp, err := client.Do(req)
		if err != nil {
			s.logger.Warn().Err(err).Int("year", year).Msg("Failed to fetch ASX HTML page")
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			s.logger.Warn().Int("status", resp.StatusCode).Int("year", year).Msg("Non-OK status from ASX HTML")
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		filings := parseAnnouncementsHTML(string(body))
		allFilings = append(allFilings, filings...)

		s.logger.Debug().
			Str("code", tickerCode).
			Int("year", year).
			Int("count", len(filings)).
			Msg("Parsed announcements from ASX HTML")
	}

	return allFilings, nil
}

// Regex patterns for HTML parsing — compiled once.
var (
	tbodyPattern    = regexp.MustCompile(`(?s)<tbody>(.*?)</tbody>`)
	trPattern       = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	datePattern     = regexp.MustCompile(`(\d{2}/\d{2}/\d{4})`)
	timePattern     = regexp.MustCompile(`class="dates-time"[^>]*>([^<]+)`)
	idsIDPattern    = regexp.MustCompile(`idsId=(\d+)`)
	headlineLinkPat = regexp.MustCompile(`(?s)displayAnnouncement\.do[^"]*idsId=(\d+)[^"]*"[^>]*>\s*([^<]+)`)
)

// parseAnnouncementsHTML parses HTML table rows to extract announcement data.
// Handles ASX HTML structure with varying whitespace and attributes.
func parseAnnouncementsHTML(html string) []models.CompanyFiling {
	var filings []models.CompanyFiling

	// Find all tbody sections (ASX pages may have multiple for code reuse history)
	tbodyMatches := tbodyPattern.FindAllStringSubmatch(html, -1)
	if len(tbodyMatches) == 0 {
		return filings
	}

	for _, tbodyMatch := range tbodyMatches {
		tbody := tbodyMatch[1]

		// Extract individual rows
		rows := trPattern.FindAllStringSubmatch(tbody, -1)

		for _, row := range rows {
			rowHTML := row[1]

			// Extract date (DD/MM/YYYY)
			dateMatch := datePattern.FindStringSubmatch(rowHTML)
			if len(dateMatch) < 2 {
				continue
			}
			dateStr := dateMatch[1]

			// Extract time
			timeStr := ""
			timeMatch := timePattern.FindStringSubmatch(rowHTML)
			if len(timeMatch) >= 2 {
				timeStr = strings.TrimSpace(timeMatch[1])
			}

			// Parse date+time
			var dateTime time.Time
			var err error
			if timeStr != "" {
				dateTime, err = time.Parse("02/01/2006 3:04 pm", dateStr+" "+timeStr)
				if err != nil {
					dateTime, err = time.Parse("02/01/2006", dateStr)
					if err != nil {
						continue
					}
				}
			} else {
				dateTime, err = time.Parse("02/01/2006", dateStr)
				if err != nil {
					continue
				}
			}

			// Check price sensitive
			priceSensitive := strings.Contains(rowHTML, "icon-price-sensitive")

			// Extract headline and idsId from the displayAnnouncement link
			linkMatch := headlineLinkPat.FindStringSubmatch(rowHTML)
			if len(linkMatch) < 3 {
				continue
			}

			idsID := linkMatch[1]
			headline := strings.TrimSpace(linkMatch[2])

			// Clean headline of any remaining whitespace/newlines
			headline = strings.Join(strings.Fields(headline), " ")

			if headline == "" {
				continue
			}

			// Build PDF URL
			pdfURL := fmt.Sprintf("https://www.asx.com.au/asx/v2/statistics/displayAnnouncement.do?display=pdf&idsId=%s", idsID)

			// Infer type from headline
			annType := inferAnnouncementType(headline)

			filings = append(filings, models.CompanyFiling{
				Date:           dateTime,
				Headline:       headline,
				Type:           annType,
				PDFURL:         pdfURL,
				DocumentKey:    idsID,
				PriceSensitive: priceSensitive,
			})
		}
	}

	return filings
}

// fetchMarkitAnnouncements fetches from Markit Digital API as fallback.
func (s *Service) fetchMarkitAnnouncements(ctx context.Context, tickerCode, period string) ([]models.CompanyFiling, error) {
	url := fmt.Sprintf("https://asx.api.markitdigital.com/asx-research/1.0/companies/%s/announcements",
		strings.ToLower(tickerCode))

	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Markit API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp markitAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	cutoffDate := calculateCutoffDate(period)

	var filings []models.CompanyFiling
	for _, item := range apiResp.Data.Items {
		date, err := time.Parse(time.RFC3339, item.Date)
		if err != nil {
			date, err = time.Parse("2006-01-02T15:04:05", item.Date)
			if err != nil {
				date = time.Now()
			}
		}

		if !cutoffDate.IsZero() && date.Before(cutoffDate) {
			continue
		}

		pdfURL := item.URL
		if pdfURL == "" && item.DocumentKey != "" {
			pdfURL = fmt.Sprintf("https://www.asx.com.au/asxpdf/%s", item.DocumentKey)
		}

		annType := item.AnnouncementType
		if annType == "" {
			annType = inferAnnouncementType(item.Headline)
		}

		filings = append(filings, models.CompanyFiling{
			Date:           date,
			Headline:       item.Headline,
			Type:           annType,
			PDFURL:         pdfURL,
			DocumentKey:    item.DocumentKey,
			PriceSensitive: item.IsPriceSensitive,
		})
	}

	return filings, nil
}

type markitAPIResponse struct {
	Data struct {
		Items []struct {
			Date             string `json:"date"`
			Headline         string `json:"headline"`
			AnnouncementType string `json:"type"`
			URL              string `json:"url"`
			DocumentKey      string `json:"documentKey"`
			IsPriceSensitive bool   `json:"priceSensitive"`
		} `json:"items"`
	} `json:"data"`
}

func calculateCutoffDate(period string) time.Time {
	now := time.Now()
	switch period {
	case "Y1":
		return now.AddDate(-1, 0, 0)
	case "Y2":
		return now.AddDate(-2, 0, 0)
	case "Y3":
		return now.AddDate(-3, 0, 0)
	case "Y5":
		return now.AddDate(-5, 0, 0)
	default:
		return now.AddDate(-3, 0, 0)
	}
}

// --- Classification ---

// inferAnnouncementType guesses announcement type from headline.
func inferAnnouncementType(headline string) string {
	headlineUpper := strings.ToUpper(headline)

	typePatterns := []struct {
		keywords []string
		annType  string
	}{
		{[]string{"TRADING HALT", "SUSPENSION"}, "Trading Halt"},
		{[]string{"QUARTERLY", "ACTIVITIES REPORT", "4C"}, "Quarterly Report"},
		{[]string{"HALF YEAR", "HALF-YEAR", "1H", "H1"}, "Half Year Report"},
		{[]string{"ANNUAL REPORT", "FULL YEAR", "FY", "PRELIMINARY FINAL", "APPENDIX 4E"}, "Annual Report"},
		{[]string{"APPENDIX 4D"}, "Half Year Report"},
		{[]string{"DIVIDEND", "DISTRIBUTION"}, "Dividend"},
		{[]string{"AGM", "GENERAL MEETING"}, "Meeting"},
		{[]string{"APPENDIX"}, "Appendix"},
		{[]string{"DIRECTOR"}, "Director Related"},
		{[]string{"SUBSTANTIAL", "HOLDER"}, "Substantial Holder"},
	}

	for _, tp := range typePatterns {
		for _, kw := range tp.keywords {
			if strings.Contains(headlineUpper, kw) {
				return tp.annType
			}
		}
	}

	return "Announcement"
}

// classifyFiling determines the relevance category of a filing.
func classifyFiling(headline, annType string, priceSensitive bool) string {
	if priceSensitive {
		return "HIGH"
	}

	typeUpper := strings.ToUpper(annType)
	headlineUpper := strings.ToUpper(headline)

	highKeywords := []string{
		"TAKEOVER", "ACQUISITION", "MERGER", "DISPOSAL",
		"DIVIDEND", "CAPITAL RAISING", "PLACEMENT", "SPP", "RIGHTS ISSUE",
		"FINANCIAL REPORT", "HALF YEAR", "FULL YEAR", "ANNUAL REPORT",
		"QUARTERLY", "PRELIMINARY FINAL", "EARNINGS",
		"GUIDANCE", "FORECAST", "OUTLOOK",
		"ASSET SALE", "DIVESTMENT", "DISTRIBUTION",
	}

	for _, kw := range highKeywords {
		if strings.Contains(typeUpper, kw) || strings.Contains(headlineUpper, kw) {
			return "HIGH"
		}
	}

	mediumKeywords := []string{
		"DIRECTOR", "CHAIRMAN", "CEO", "CFO", "MANAGING DIRECTOR",
		"APPOINTMENT", "RESIGNATION", "RETIREMENT",
		"AGM", "EGM", "GENERAL MEETING",
		"CONTRACT", "AGREEMENT", "PARTNERSHIP", "JOINT VENTURE",
		"EXPLORATION", "DRILLING", "RESOURCE", "RESERVE",
		"REGULATORY", "APPROVAL", "LICENSE", "PERMIT",
	}

	for _, kw := range mediumKeywords {
		if strings.Contains(typeUpper, kw) || strings.Contains(headlineUpper, kw) {
			return "MEDIUM"
		}
	}

	lowKeywords := []string{
		"PROGRESS REPORT", "UPDATE", "INVESTOR PRESENTATION",
		"DISCLOSURE", "CLEANSING", "STATEMENT",
		"APPENDIX", "SUBSTANTIAL HOLDER",
		"CHANGE OF ADDRESS", "COMPANY SECRETARY",
	}

	for _, kw := range lowKeywords {
		if strings.Contains(typeUpper, kw) || strings.Contains(headlineUpper, kw) {
			return "LOW"
		}
	}

	return "NOISE"
}

// filterFinancialFilings returns only financial report filings.
func filterFinancialFilings(filings []models.CompanyFiling) []models.CompanyFiling {
	financialKeywords := []string{
		"ANNUAL REPORT", "FULL YEAR", "FY", "PRELIMINARY FINAL",
		"APPENDIX 4E", "APPENDIX 4C", "APPENDIX 4D",
		"HALF YEAR", "HALF-YEAR", "QUARTERLY",
	}

	var result []models.CompanyFiling
	for _, f := range filings {
		headlineUpper := strings.ToUpper(f.Headline)
		typeUpper := strings.ToUpper(f.Type)
		for _, kw := range financialKeywords {
			if strings.Contains(headlineUpper, kw) || strings.Contains(typeUpper, kw) {
				result = append(result, f)
				break
			}
		}
	}
	return result
}

// deduplicateFilings consolidates same-day announcements with similar headlines.
func deduplicateFilings(filings []models.CompanyFiling) []models.CompanyFiling {
	if len(filings) == 0 {
		return filings
	}

	// Group by date
	byDate := make(map[string][]models.CompanyFiling)
	for _, f := range filings {
		dateKey := f.Date.Format("2006-01-02")
		byDate[dateKey] = append(byDate[dateKey], f)
	}

	var result []models.CompanyFiling
	for _, dayFilings := range byDate {
		used := make(map[int]bool)
		for i := 0; i < len(dayFilings); i++ {
			if used[i] {
				continue
			}
			used[i] = true

			// Find similar headlines on same day
			for j := i + 1; j < len(dayFilings); j++ {
				if used[j] {
					continue
				}
				if areSimilarHeadlines(dayFilings[i].Headline, dayFilings[j].Headline) {
					used[j] = true
				}
			}

			result = append(result, dayFilings[i])
		}
	}

	return result
}

// areSimilarHeadlines checks if two headlines should be considered duplicates.
func areSimilarHeadlines(h1, h2 string) bool {
	if h1 == h2 {
		return true
	}
	norm1 := normalizeHeadline(h1)
	norm2 := normalizeHeadline(h2)
	return norm1 == norm2
}

// normalizeHeadline removes trailing ticker codes and whitespace for comparison.
func normalizeHeadline(headline string) string {
	h := strings.TrimSpace(headline)
	if idx := strings.LastIndex(h, " - "); idx > 0 {
		suffix := strings.TrimSpace(h[idx+3:])
		if len(suffix) >= 2 && len(suffix) <= 4 {
			allUpper := true
			for _, r := range suffix {
				if r < 'A' || r > 'Z' {
					allUpper = false
					break
				}
			}
			if allUpper {
				h = strings.TrimSpace(h[:idx])
			}
		}
	}
	return strings.ToUpper(h)
}

// collectFilings orchestrates fetching, classifying, and deduplicating filings.
func (s *Service) collectFilings(ctx context.Context, ticker string) ([]models.CompanyFiling, error) {
	tickerCode := extractCode(ticker)
	if tickerCode == "" {
		return nil, fmt.Errorf("invalid ticker: %s", ticker)
	}

	s.logger.Debug().Str("ticker", ticker).Str("code", tickerCode).Msg("Collecting filings")

	// Fetch Y1 general announcements + Y3 for financial filings
	var allFilings []models.CompanyFiling

	generalFilings, err := s.fetchASXAnnouncements(ctx, tickerCode, "Y1")
	if err != nil {
		s.logger.Warn().Err(err).Str("ticker", ticker).Msg("ASX HTML scraping failed, trying Markit API")
		generalFilings, err = s.fetchMarkitAnnouncements(ctx, tickerCode, "Y1")
		if err != nil {
			return nil, fmt.Errorf("failed to fetch announcements: %w", err)
		}
	}
	allFilings = append(allFilings, generalFilings...)

	// Also fetch Y3 for financial filings specifically
	financialFilings, err := s.fetchASXAnnouncements(ctx, tickerCode, "Y3")
	if err != nil {
		s.logger.Debug().Err(err).Msg("Y3 HTML fetch failed, trying Markit fallback")
		financialFilings, err = s.fetchMarkitAnnouncements(ctx, tickerCode, "Y3")
		if err != nil {
			s.logger.Warn().Err(err).Msg("Y3 Markit fetch also failed")
		}
	}
	if len(financialFilings) > 0 {
		// Only add the financial report filings from the Y3 batch
		filtered := filterFinancialFilings(financialFilings)
		allFilings = append(allFilings, filtered...)
	}

	// Classify all filings
	for i := range allFilings {
		allFilings[i].Relevance = classifyFiling(allFilings[i].Headline, allFilings[i].Type, allFilings[i].PriceSensitive)
	}

	// Deduplicate same-day similar headlines
	allFilings = deduplicateFilings(allFilings)

	// Sort descending by date
	sort.Slice(allFilings, func(i, j int) bool {
		return allFilings[i].Date.After(allFilings[j].Date)
	})

	s.logger.Info().
		Str("ticker", ticker).
		Int("total", len(allFilings)).
		Msg("Collected filings")

	return allFilings, nil
}

// extractCode extracts ticker code from full ticker (e.g., "BHP.AU" -> "BHP").
func extractCode(ticker string) string {
	if idx := strings.LastIndex(ticker, "."); idx > 0 {
		return ticker[:idx]
	}
	return ticker
}

// --- PDF Download & Storage ---

const (
	filingsDir     = "./data/filings"
	maxPDFsPerTick = 15
)

// downloadFilingPDFs downloads PDFs for financial filings and stores them on disk.
func (s *Service) downloadFilingPDFs(ctx context.Context, tickerCode string, filings []models.CompanyFiling) []models.CompanyFiling {
	financialFilings := filterFinancialFilings(filings)
	if len(financialFilings) == 0 {
		return filings
	}

	// Build set of financial filing indices in original slice
	financialSet := make(map[string]bool)
	for _, f := range financialFilings {
		key := f.Date.Format("2006-01-02") + "|" + f.Headline
		financialSet[key] = true
	}

	tickerDir := filepath.Join(filingsDir, strings.ToUpper(tickerCode))
	if err := os.MkdirAll(tickerDir, 0o755); err != nil {
		s.logger.Warn().Err(err).Str("dir", tickerDir).Msg("Failed to create filings directory")
		return filings
	}

	downloadCount := 0
	for i := range filings {
		if downloadCount >= maxPDFsPerTick {
			break
		}

		f := &filings[i]
		key := f.Date.Format("2006-01-02") + "|" + f.Headline
		if !financialSet[key] {
			continue
		}

		if f.PDFURL == "" {
			continue
		}

		// Determine filename
		docID := f.DocumentKey
		if docID == "" {
			docID = fmt.Sprintf("%d", i)
		}
		filename := fmt.Sprintf("%s-%s.pdf", f.Date.Format("20060102"), docID)
		pdfPath := filepath.Join(tickerDir, filename)

		// Skip if already downloaded
		if _, err := os.Stat(pdfPath); err == nil {
			f.PDFPath = pdfPath
			continue
		}

		// Download with rate limiting
		if downloadCount > 0 {
			time.Sleep(1 * time.Second)
		}

		content, err := s.downloadASXPDF(ctx, f.PDFURL, f.DocumentKey)
		if err != nil {
			s.logger.Warn().Err(err).Str("headline", f.Headline).Msg("Failed to download PDF")
			continue
		}

		if err := os.WriteFile(pdfPath, content, 0o644); err != nil {
			s.logger.Warn().Err(err).Str("path", pdfPath).Msg("Failed to write PDF")
			continue
		}

		f.PDFPath = pdfPath
		downloadCount++

		s.logger.Debug().
			Str("headline", f.Headline).
			Str("path", pdfPath).
			Int("size", len(content)).
			Msg("Downloaded filing PDF")
	}

	s.logger.Info().
		Str("code", tickerCode).
		Int("downloaded", downloadCount).
		Msg("Filing PDF download complete")

	return filings
}

// downloadASXPDF downloads a PDF from ASX, handling their WAF cookie dance.
func (s *Service) downloadASXPDF(ctx context.Context, pdfURL, documentKey string) ([]byte, error) {
	if pdfURL == "" {
		return nil, fmt.Errorf("empty PDF URL")
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cookie jar: %w", err)
	}

	client := &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
	}

	// Step 1: Visit the HTML page to establish session cookies
	initReq, err := http.NewRequestWithContext(ctx, "GET", pdfURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}

	initReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	initReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	initReq.Header.Set("Accept-Language", "en-US,en;q=0.5")
	initReq.Header.Set("Referer", "https://www.asx.com.au/")

	initResp, err := client.Do(initReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get session cookies: %w", err)
	}
	initResp.Body.Close()

	// Step 2: Try the direct PDF URL with session cookies
	directPDFURL := fmt.Sprintf("https://announcements.asx.com.au/asxpdf/%s.pdf", documentKey)

	pdfReq, err := http.NewRequestWithContext(ctx, "GET", directPDFURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create PDF request: %w", err)
	}

	pdfReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	pdfReq.Header.Set("Accept", "application/pdf,*/*")
	pdfReq.Header.Set("Referer", "https://www.asx.com.au/")

	pdfResp, err := client.Do(pdfReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PDF: %w", err)
	}
	defer pdfResp.Body.Close()

	// If direct URL fails, try the original URL as fallback
	if pdfResp.StatusCode != http.StatusOK {
		pdfResp.Body.Close()

		fallbackReq, err := http.NewRequestWithContext(ctx, "GET", pdfURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create fallback request: %w", err)
		}
		fallbackReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		fallbackReq.Header.Set("Accept", "application/pdf,*/*")

		pdfResp, err = client.Do(fallbackReq)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch PDF from fallback: %w", err)
		}
		defer pdfResp.Body.Close()
	}

	if pdfResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PDF download failed with status: %d", pdfResp.StatusCode)
	}

	content, err := io.ReadAll(pdfResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF content: %w", err)
	}

	return content, nil
}

// --- PDF Text Extraction ---

// extractPDFText extracts text content from a PDF file.
func extractPDFText(pdfPath string) (string, error) {
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return "", fmt.Errorf("failed to open PDF: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	totalPages := r.NumPage()

	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		sb.WriteString(text)
		sb.WriteString("\n")

		// Truncate to ~50,000 chars for Gemini context limits
		if sb.Len() > 50000 {
			break
		}
	}

	result := sb.String()
	if len(result) > 50000 {
		result = result[:50000]
	}

	return result, nil
}

// --- AI Summarization ---

// generateFilingsIntelligence uses Gemini to produce a filings analysis.
func (s *Service) generateFilingsIntelligence(ctx context.Context, ticker, name string, filings []models.CompanyFiling) *models.FilingsIntelligence {
	if len(filings) == 0 {
		return nil
	}

	// Build context from fundamentals if available
	var fundamentalsCtx string
	md, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if md != nil && md.Fundamentals != nil {
		f := md.Fundamentals
		fundamentalsCtx = fmt.Sprintf(
			"\n## Current Fundamentals\n- P/E: %.2f\n- P/B: %.2f\n- EPS: $%.2f\n- Dividend Yield: %.2f%%\n- Market Cap: $%.0fM\n",
			f.PE, f.PB, f.EPS, f.DividendYield*100, f.MarketCap/1_000_000)
	}

	prompt := buildFilingsIntelPrompt(ticker, name, filings, fundamentalsCtx)

	// Try URL context tool first with PDF URLs for unextracted PDFs
	response, err := s.gemini.GenerateWithURLContextTool(ctx, prompt)
	if err != nil {
		s.logger.Debug().Str("ticker", ticker).Err(err).Msg("URL context tool failed, falling back to GenerateContent")
		response, err = s.gemini.GenerateContent(ctx, prompt)
	}
	if err != nil {
		s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to generate filings intelligence")
		return nil
	}

	intel := parseFilingsIntelResponse(response)
	if intel == nil {
		s.logger.Warn().Str("ticker", ticker).Msg("Failed to parse filings intelligence response")
		return nil
	}
	intel.GeneratedAt = time.Now()
	intel.FilingsAnalyzed = len(filings)
	return intel
}

// buildFilingsIntelPrompt creates the prompt for filings intelligence analysis.
func buildFilingsIntelPrompt(ticker, name string, filings []models.CompanyFiling, fundamentalsCtx string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are a financial analyst. Analyze the following company filings for %s (%s).\n\n", name, ticker))

	// Recent announcements table
	sb.WriteString("## Recent Announcements\n\n")
	sb.WriteString("| Date | Headline | Type | Relevance | Price Sensitive |\n")
	sb.WriteString("|------|----------|------|-----------|----------------|\n")

	for _, f := range filings {
		ps := "No"
		if f.PriceSensitive {
			ps = "Yes"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			f.Date.Format("2006-01-02"), f.Headline, f.Type, f.Relevance, ps))
	}
	sb.WriteString("\n")

	// Financial report extracts from PDFs
	financialFilings := filterFinancialFilings(filings)
	pdfTexts := 0
	for _, f := range financialFilings {
		if f.PDFPath == "" {
			continue
		}

		text, err := extractPDFText(f.PDFPath)
		if err != nil || text == "" {
			continue
		}

		// Limit per-PDF text to keep total prompt reasonable
		if len(text) > 15000 {
			text = text[:15000]
		}

		sb.WriteString(fmt.Sprintf("## Financial Report: %s (%s)\n\n", f.Headline, f.Date.Format("2006-01-02")))
		sb.WriteString(text)
		sb.WriteString("\n\n")

		pdfTexts++
		if pdfTexts >= 5 {
			break // Limit to 5 most recent financial reports
		}
	}

	// Fundamentals
	if fundamentalsCtx != "" {
		sb.WriteString(fundamentalsCtx)
		sb.WriteString("\n")
	}

	sb.WriteString(`Provide analysis as JSON. Return ONLY valid JSON:
{
  "summary": "2-3 paragraph executive summary of company financial position and trajectory",
  "financial_health": "strong|stable|concerning|weak",
  "growth_outlook": "positive|neutral|negative",
  "can_support_10pct_pa": true or false,
  "growth_rationale": "Why yes/no on 10%+ annual share price growth, based on filings evidence",
  "key_metrics": [{"name": "Revenue", "value": "$1.2B", "period": "FY2025", "trend": "up"}],
  "year_over_year": [{"period": "FY2025 vs FY2024", "revenue": "+12%", "profit": "+8%", "outlook": "improved", "key_changes": "..."}],
  "strategy_notes": "Company strategy and direction based on filings",
  "risk_factors": ["factor 1", "factor 2"],
  "positive_factors": ["factor 1", "factor 2"]
}

Rules:
- Base analysis ONLY on the filings data provided — do not speculate beyond what the documents show
- The 10% growth assessment should be rigorous and evidence-based
- Key metrics should reflect actual figures from financial reports where available
- Year-over-year should compare consecutive reporting periods
- Be specific about revenue, profit, margins, and cash flow trends
- Return ONLY the JSON object, no markdown code fences, no explanation`)

	return sb.String()
}

// filingsIntelResponse is the expected JSON shape from Gemini.
type filingsIntelResponse struct {
	Summary         string `json:"summary"`
	FinancialHealth string `json:"financial_health"`
	GrowthOutlook   string `json:"growth_outlook"`
	CanSupport10Pct bool   `json:"can_support_10pct_pa"`
	GrowthRationale string `json:"growth_rationale"`
	KeyMetrics      []struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Period string `json:"period"`
		Trend  string `json:"trend"`
	} `json:"key_metrics"`
	YearOverYear []struct {
		Period     string `json:"period"`
		Revenue    string `json:"revenue"`
		Profit     string `json:"profit"`
		Outlook    string `json:"outlook"`
		KeyChanges string `json:"key_changes"`
	} `json:"year_over_year"`
	StrategyNotes   string   `json:"strategy_notes"`
	RiskFactors     []string `json:"risk_factors"`
	PositiveFactors []string `json:"positive_factors"`
}

// parseFilingsIntelResponse parses Gemini's JSON response into a FilingsIntelligence struct.
func parseFilingsIntelResponse(response string) *models.FilingsIntelligence {
	// Strip markdown code fences if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var data filingsIntelResponse
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		return nil
	}

	if data.Summary == "" {
		return nil
	}

	metrics := make([]models.FilingMetric, 0, len(data.KeyMetrics))
	for _, m := range data.KeyMetrics {
		metrics = append(metrics, models.FilingMetric{
			Name:   m.Name,
			Value:  m.Value,
			Period: m.Period,
			Trend:  m.Trend,
		})
	}

	yoy := make([]models.YearOverYearEntry, 0, len(data.YearOverYear))
	for _, y := range data.YearOverYear {
		yoy = append(yoy, models.YearOverYearEntry{
			Period:     y.Period,
			Revenue:    y.Revenue,
			Profit:     y.Profit,
			Outlook:    y.Outlook,
			KeyChanges: y.KeyChanges,
		})
	}

	return &models.FilingsIntelligence{
		Summary:           data.Summary,
		FinancialHealth:   data.FinancialHealth,
		GrowthOutlook:     data.GrowthOutlook,
		CanSupport10PctPA: data.CanSupport10Pct,
		GrowthRationale:   data.GrowthRationale,
		KeyMetrics:        metrics,
		YearOverYear:      yoy,
		StrategyNotes:     data.StrategyNotes,
		RiskFactors:       data.RiskFactors,
		PositiveFactors:   data.PositiveFactors,
	}
}
