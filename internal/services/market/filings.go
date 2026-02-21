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

// filingsPath returns the filings directory as a peer of other storage areas.
// Resolves to {binary-dir}/data/filings/ alongside data/market/, data/user/, etc.
// Falls back to a temp directory when DataPath is empty (e.g. in tests with mock storage).
func (s *Service) filingsPath() string {
	dp := s.storage.DataPath()
	if dp == "" {
		return filepath.Join(os.TempDir(), "vire-filings")
	}
	return filepath.Join(filepath.Dir(dp), "filings")
}

// downloadFilingPDFs downloads PDFs for all filings that have a URL and stores them on disk.
// Every document is downloaded so that Gemini has full text for extraction and the
// summary can reference a local copy of the source document.
func (s *Service) downloadFilingPDFs(ctx context.Context, tickerCode string, filings []models.CompanyFiling) []models.CompanyFiling {
	tickerDir := filepath.Join(s.filingsPath(), strings.ToUpper(tickerCode))
	if err := os.MkdirAll(tickerDir, 0o755); err != nil {
		s.logger.Warn().Err(err).Str("dir", tickerDir).Msg("Failed to create filings directory")
		return filings
	}

	downloadCount := 0
	for i := range filings {
		f := &filings[i]

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

// downloadASXPDF downloads a PDF from ASX, handling their terms acceptance page.
// ASX serves an HTML terms page with the real PDF URL embedded in a hidden form field.
// We parse the real URL, POST to accept terms, then download the actual PDF.
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

	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

	// Step 1: GET the announcement page — ASX returns an HTML terms page
	initReq, err := http.NewRequestWithContext(ctx, "GET", pdfURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create init request: %w", err)
	}
	initReq.Header.Set("User-Agent", ua)
	initReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	initResp, err := client.Do(initReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get terms page: %w", err)
	}
	body, _ := io.ReadAll(initResp.Body)
	initResp.Body.Close()

	// Step 2: Extract the real PDF URL from the terms form
	realPDFURL := extractFormPDFURL(string(body))
	if realPDFURL == "" {
		// Response might already be a PDF (no terms page)
		if len(body) > 4 && string(body[:4]) == "%PDF" {
			return body, nil
		}
		return nil, fmt.Errorf("could not extract PDF URL from terms page")
	}

	// Step 3: POST to accept terms (establishes session consent)
	formData := "pdfURL=" + realPDFURL + "&showAnnouncementPDFForm=Agree+and+proceed"
	termsURL := "https://www.asx.com.au/asx/v2/statistics/announcementTerms.do"
	termsReq, err := http.NewRequestWithContext(ctx, "POST", termsURL, strings.NewReader(formData))
	if err != nil {
		return nil, fmt.Errorf("failed to create terms request: %w", err)
	}
	termsReq.Header.Set("User-Agent", ua)
	termsReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	termsReq.Header.Set("Referer", pdfURL)

	termsResp, err := client.Do(termsReq)
	if err != nil {
		return nil, fmt.Errorf("failed to accept terms: %w", err)
	}
	termsBody, _ := io.ReadAll(termsResp.Body)
	termsResp.Body.Close()

	// The terms POST may redirect directly to the PDF
	if len(termsBody) > 4 && string(termsBody[:4]) == "%PDF" {
		return termsBody, nil
	}

	// Step 4: Download the actual PDF using the real URL
	pdfReq, err := http.NewRequestWithContext(ctx, "GET", realPDFURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create PDF request: %w", err)
	}
	pdfReq.Header.Set("User-Agent", ua)
	pdfReq.Header.Set("Accept", "application/pdf,*/*")
	pdfReq.Header.Set("Referer", termsURL)

	pdfResp, err := client.Do(pdfReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PDF: %w", err)
	}
	defer pdfResp.Body.Close()

	if pdfResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("PDF download failed with status: %d", pdfResp.StatusCode)
	}

	content, err := io.ReadAll(pdfResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read PDF content: %w", err)
	}

	// Validate we actually got a PDF
	if len(content) < 4 || string(content[:4]) != "%PDF" {
		return nil, fmt.Errorf("downloaded content is not a PDF (%d bytes)", len(content))
	}

	return content, nil
}

// extractFormPDFURL parses the real PDF URL from the ASX terms acceptance HTML page.
var pdfURLPattern = regexp.MustCompile(`name="pdfURL"\s+value="([^"]+)"`)

func extractFormPDFURL(html string) string {
	m := pdfURLPattern.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// --- PDF Text Extraction ---

// extractPDFText extracts text content from a PDF file.
// Recovers from panics (e.g. zlib: invalid header) caused by corrupt PDFs.
func extractPDFText(pdfPath string) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text = ""
			err = fmt.Errorf("panic during PDF extraction: %v", r)
		}
	}()

	f, r, openErr := pdf.Open(pdfPath)
	if openErr != nil {
		return "", fmt.Errorf("failed to open PDF: %w", openErr)
	}
	defer f.Close()

	var sb strings.Builder
	totalPages := r.NumPage()

	for i := 1; i <= totalPages; i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}

		pageText, pageErr := page.GetPlainText(nil)
		if pageErr != nil {
			continue
		}
		sb.WriteString(pageText)
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

// --- Per-Filing Summarization ---

const filingSummaryBatchSize = 5

// summarizeNewFilings extracts structured data from filings not yet summarized.
// Returns the full list of summaries (existing + new). Incremental: only unsummarized filings are sent to Gemini.
func (s *Service) summarizeNewFilings(ctx context.Context, ticker string, filings []models.CompanyFiling, existing []models.FilingSummary) ([]models.FilingSummary, bool) {
	if len(filings) == 0 {
		return existing, false
	}

	// Build lookup of existing summaries by dedup key.
	// Track which ones are "empty" (headline-only with no extracted data) so they
	// can be re-analyzed now that a PDF may be available.
	existingByKey := make(map[string]*models.FilingSummary, len(existing))
	for i := range existing {
		key := filingSummaryKey(existing[i].DocumentKey, existing[i].Date, existing[i].Headline)
		existingByKey[key] = &existing[i]
	}

	// Filter to filings that need (re-)summarization:
	//  - Not yet summarized at all
	//  - Previously headline-only (empty key_facts) and a PDF is now available
	var unsummarized []models.CompanyFiling
	var staleKeys []string // keys of empty summaries being replaced
	for _, f := range filings {
		if f.Relevance != "HIGH" && f.Relevance != "MEDIUM" {
			continue
		}
		key := filingSummaryKey(f.DocumentKey, f.Date, f.Headline)
		prev := existingByKey[key]
		if prev == nil {
			// Never summarized
			unsummarized = append(unsummarized, f)
		} else if f.PDFPath != "" && prev.PDFPath == "" && len(prev.KeyFacts) == 0 {
			// Was headline-only with no data — PDF now available, re-analyze
			unsummarized = append(unsummarized, f)
			staleKeys = append(staleKeys, key)
		}
	}

	if len(unsummarized) == 0 {
		return existing, false
	}

	// Remove stale summaries that will be replaced
	if len(staleKeys) > 0 {
		staleSet := make(map[string]bool, len(staleKeys))
		for _, k := range staleKeys {
			staleSet[k] = true
		}
		filtered := make([]models.FilingSummary, 0, len(existing))
		for _, fs := range existing {
			key := filingSummaryKey(fs.DocumentKey, fs.Date, fs.Headline)
			if !staleSet[key] {
				filtered = append(filtered, fs)
			}
		}
		existing = filtered
		s.logger.Info().Str("ticker", ticker).Int("stale_replaced", len(staleKeys)).Msg("Re-analyzing headline-only summaries with PDF content")
	}

	s.logger.Info().Str("ticker", ticker).Int("new", len(unsummarized)).Int("existing", len(existing)).Msg("Summarizing new filings")

	// Process in batches
	var newSummaries []models.FilingSummary
	for i := 0; i < len(unsummarized); i += filingSummaryBatchSize {
		end := i + filingSummaryBatchSize
		if end > len(unsummarized) {
			end = len(unsummarized)
		}
		batch := unsummarized[i:end]

		if i > 0 {
			time.Sleep(1 * time.Second) // rate limit between batches
		}

		summaries := s.summarizeFilingBatch(ctx, ticker, batch)
		newSummaries = append(newSummaries, summaries...)
	}

	// Combine existing + new
	result := make([]models.FilingSummary, 0, len(existing)+len(newSummaries))
	result = append(result, existing...)
	result = append(result, newSummaries...)

	// Sort by date descending
	sort.Slice(result, func(i, j int) bool {
		return result[i].Date.After(result[j].Date)
	})

	return result, true
}

// filingSummaryKey produces a dedup key for a filing summary.
// Always uses date+headline to avoid ASX vs Markit document key format mismatches.
func filingSummaryKey(_ string, date time.Time, headline string) string {
	return date.Format("2006-01-02") + "|" + headline
}

// summarizeFilingBatch sends a batch of filings to Gemini for structured extraction.
func (s *Service) summarizeFilingBatch(ctx context.Context, ticker string, batch []models.CompanyFiling) []models.FilingSummary {
	// Log PDF text availability for diagnostics
	withPDF := 0
	for _, f := range batch {
		if f.PDFPath != "" {
			withPDF++
		}
	}
	s.logger.Info().
		Str("ticker", ticker).
		Int("batch_size", len(batch)).
		Int("with_pdf", withPDF).
		Int("headline_only", len(batch)-withPDF).
		Msg("Filing extraction batch PDF availability")

	prompt := buildFilingSummaryPrompt(ticker, batch)

	response, err := s.gemini.GenerateWithURLContext(ctx, prompt)
	if err != nil {
		s.logger.Debug().Str("ticker", ticker).Err(err).Msg("URL context failed for filing summary, falling back")
		response, err = s.gemini.GenerateContent(ctx, prompt)
	}
	if err != nil {
		s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to summarize filing batch")
		return nil
	}

	summaries := parseFilingSummaryResponse(response, batch)
	s.logger.Debug().Str("ticker", ticker).Int("input", len(batch)).Int("output", len(summaries)).Msg("Filing batch summarized")
	return summaries
}

// buildFilingSummaryPrompt creates the Gemini prompt for per-filing data extraction.
// Handles filings with and without PDF text content.
func buildFilingSummaryPrompt(ticker string, batch []models.CompanyFiling) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Extract structured financial data from these %s filings.\n\n", ticker))
	sb.WriteString("For each filing, extract ACTUAL NUMBERS from the document. Do not invent data.\n")
	sb.WriteString("If a filing has no financial data, use type \"other\" with key_facts only.\n\n")

	withPDF := 0
	withoutPDF := 0

	for i, f := range batch {
		sb.WriteString(fmt.Sprintf("--- FILING %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("Date: %s\nHeadline: %s\nType: %s\nPrice Sensitive: %v\n",
			f.Date.Format("2006-01-02"), f.Headline, f.Type, f.PriceSensitive))

		// Include PDF text if available
		hasPDF := false
		if f.PDFPath != "" {
			text, err := extractPDFText(f.PDFPath)
			if err == nil && len(strings.TrimSpace(text)) > 100 {
				if len(text) > 15000 {
					text = text[:15000]
				}
				sb.WriteString("\nDocument content:\n")
				sb.WriteString(text)
				hasPDF = true
				withPDF++
			}
		}

		if !hasPDF {
			withoutPDF++
			sb.WriteString("\nNo document content available. Extract what you can from the headline and metadata.\n")
			sb.WriteString("Use the headline to infer: filing type, any dollar values, company names, or operational details mentioned.\n")
		}
		sb.WriteString("\n\n")
	}

	// Log PDF availability for debugging (injected into the logger by caller)
	_ = withPDF
	_ = withoutPDF

	sb.WriteString(fmt.Sprintf(`Return a JSON array with exactly %d objects, one per filing in order.
Each object:
{
  "type": "financial_results|guidance|contract|acquisition|business_change|other",
  "revenue": "$261.7M",
  "revenue_growth": "+92%%",
  "profit": "$14.0M net profit",
  "profit_growth": "+112%%",
  "margin": "5.4%%",
  "eps": "$0.12",
  "dividend": "$0.06 fully franked",
  "contract_value": "$130M",
  "customer": "NEXTDC",
  "acq_target": "Delta Elcom",
  "acq_price": "$13.75M",
  "guidance_revenue": "$340M",
  "guidance_profit": "$34M PBT",
  "key_facts": ["Revenue $261.7M, up 92%% YoY", "Net profit $14.0M", "Work-on-hand $560M"],
  "period": "FY2025"
}

Rules:
- Extract ACTUAL numbers from the document — "$261.7M" not "Revenue increased"
- For headline-only filings: infer type and extract any dollar amounts, company names, or metrics from the headline text
- key_facts: up to 5 bullet points of specific, factual statements with numbers
- Use empty strings for fields that don't apply
- If a headline mentions a contract value (e.g., "$130M data centre project"), populate contract_value
- If a headline mentions an acquisition target (e.g., "Completes Delta Elcom Acquisition"), populate acq_target
- If a headline mentions guidance/forecast/upgrade, populate guidance_revenue and/or guidance_profit
- type must be one of the listed values
- Return ONLY the JSON array, no markdown fences
`, len(batch)))

	return sb.String()
}

// filingSummaryRaw is the JSON shape returned by Gemini for a single filing.
type filingSummaryRaw struct {
	Type            string   `json:"type"`
	Revenue         string   `json:"revenue"`
	RevenueGrowth   string   `json:"revenue_growth"`
	Profit          string   `json:"profit"`
	ProfitGrowth    string   `json:"profit_growth"`
	Margin          string   `json:"margin"`
	EPS             string   `json:"eps"`
	Dividend        string   `json:"dividend"`
	ContractValue   string   `json:"contract_value"`
	Customer        string   `json:"customer"`
	AcqTarget       string   `json:"acq_target"`
	AcqPrice        string   `json:"acq_price"`
	GuidanceRevenue string   `json:"guidance_revenue"`
	GuidanceProfit  string   `json:"guidance_profit"`
	KeyFacts        []string `json:"key_facts"`
	Period          string   `json:"period"`
}

// parseFilingSummaryResponse parses the Gemini JSON array response into FilingSummary structs.
func parseFilingSummaryResponse(response string, batch []models.CompanyFiling) []models.FilingSummary {
	response = stripMarkdownFences(response)

	var raw []filingSummaryRaw
	if err := json.Unmarshal([]byte(response), &raw); err != nil {
		return nil
	}

	now := time.Now()
	summaries := make([]models.FilingSummary, 0, len(raw))
	for i, r := range raw {
		if i >= len(batch) {
			break
		}
		f := batch[i]
		summaries = append(summaries, models.FilingSummary{
			Date:            f.Date,
			Headline:        f.Headline,
			Type:            r.Type,
			PriceSensitive:  f.PriceSensitive,
			Revenue:         r.Revenue,
			RevenueGrowth:   r.RevenueGrowth,
			Profit:          r.Profit,
			ProfitGrowth:    r.ProfitGrowth,
			Margin:          r.Margin,
			EPS:             r.EPS,
			Dividend:        r.Dividend,
			ContractValue:   r.ContractValue,
			Customer:        r.Customer,
			AcqTarget:       r.AcqTarget,
			AcqPrice:        r.AcqPrice,
			GuidanceRevenue: r.GuidanceRevenue,
			GuidanceProfit:  r.GuidanceProfit,
			KeyFacts:        r.KeyFacts,
			Period:          r.Period,
			DocumentKey:     f.DocumentKey,
			PDFPath:         f.PDFPath,
			AnalyzedAt:      now,
		})
	}
	return summaries
}

// --- Company Timeline Generation ---

// generateCompanyTimeline builds a structured timeline from filing summaries.
func (s *Service) generateCompanyTimeline(ctx context.Context, ticker string, summaries []models.FilingSummary, fundamentals *models.Fundamentals) *models.CompanyTimeline {
	if len(summaries) == 0 {
		return nil
	}

	prompt := buildTimelinePrompt(ticker, summaries, fundamentals)

	response, err := s.gemini.GenerateContent(ctx, prompt)
	if err != nil {
		s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to generate company timeline")
		return nil
	}

	timeline := parseTimelineResponse(response)
	if timeline == nil {
		s.logger.Warn().Str("ticker", ticker).Msg("Failed to parse company timeline response")
		return nil
	}
	timeline.GeneratedAt = time.Now()

	// Fill sector/industry from fundamentals if not set by Gemini
	if fundamentals != nil {
		if timeline.Sector == "" {
			timeline.Sector = fundamentals.Sector
		}
		if timeline.Industry == "" {
			timeline.Industry = fundamentals.Industry
		}
	}

	return timeline
}

// buildTimelinePrompt creates the Gemini prompt for company timeline generation.
func buildTimelinePrompt(ticker string, summaries []models.FilingSummary, fundamentals *models.Fundamentals) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Build a structured company timeline for %s from these filing summaries.\n\n", ticker))

	if fundamentals != nil {
		sb.WriteString(fmt.Sprintf("Sector: %s | Industry: %s | Market Cap: $%.0fM | P/E: %.2f\n\n",
			fundamentals.Sector, fundamentals.Industry, fundamentals.MarketCap/1_000_000, fundamentals.PE))
	}

	// Include historical financials from EODHD for periods not covered by filings
	if fundamentals != nil && len(fundamentals.HistoricalFinancials) > 0 {
		sb.WriteString("## Historical Financials (from EODHD, use to backfill periods not in filings)\n\n")
		sb.WriteString("| Date | Revenue | Net Income | Gross Profit | EBITDA |\n")
		sb.WriteString("|------|---------|------------|--------------|--------|\n")
		for _, h := range fundamentals.HistoricalFinancials {
			sb.WriteString(fmt.Sprintf("| %s | $%.1fM | $%.1fM | $%.1fM | $%.1fM |\n",
				h.Date, h.Revenue/1_000_000, h.NetIncome/1_000_000, h.GrossProfit/1_000_000, h.EBITDA/1_000_000))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Filing Summaries (most recent first)\n\n")
	for _, fs := range summaries {
		sb.WriteString(fmt.Sprintf("### %s — %s [%s]\n", fs.Date.Format("2006-01-02"), fs.Headline, fs.Type))
		if fs.Period != "" {
			sb.WriteString(fmt.Sprintf("Period: %s\n", fs.Period))
		}
		if fs.Revenue != "" {
			sb.WriteString(fmt.Sprintf("Revenue: %s", fs.Revenue))
			if fs.RevenueGrowth != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", fs.RevenueGrowth))
			}
			sb.WriteString("\n")
		}
		if fs.Profit != "" {
			sb.WriteString(fmt.Sprintf("Profit: %s", fs.Profit))
			if fs.ProfitGrowth != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", fs.ProfitGrowth))
			}
			sb.WriteString("\n")
		}
		if fs.GuidanceRevenue != "" || fs.GuidanceProfit != "" {
			sb.WriteString(fmt.Sprintf("Guidance: Rev %s / Profit %s\n", fs.GuidanceRevenue, fs.GuidanceProfit))
		}
		if fs.ContractValue != "" {
			sb.WriteString(fmt.Sprintf("Contract: %s", fs.ContractValue))
			if fs.Customer != "" {
				sb.WriteString(fmt.Sprintf(" (%s)", fs.Customer))
			}
			sb.WriteString("\n")
		}
		if len(fs.KeyFacts) > 0 {
			for _, kf := range fs.KeyFacts {
				sb.WriteString(fmt.Sprintf("- %s\n", kf))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`Return JSON:
{
  "business_model": "2-3 sentences: what they do, how they make money",
  "sector": "sector",
  "industry": "industry",
  "periods": [
    {
      "period": "FY2025",
      "revenue": "$261.7M",
      "revenue_growth": "+92%",
      "profit": "$14.0M net profit",
      "profit_growth": "+112%",
      "margin": "5.4%",
      "eps": "$0.12",
      "dividend": "$0.06",
      "guidance_given": "FY2026 revenue $340M, PBT $34M",
      "guidance_outcome": "FY2025 guidance was $250M rev — exceeded at $261.7M"
    }
  ],
  "key_events": [
    {"date": "2026-02-05", "event": "Major contract + guidance upgrade", "detail": "$60M new contracts. Rev guidance: $320M->$340M.", "impact": "positive"}
  ],
  "next_reporting_date": "2026-08-20",
  "work_on_hand": "$560M",
  "repeat_business_rate": "94%"
}

Rules:
- periods: Group financials by reporting period. Most recent first. Use ACTUAL numbers from summaries. For periods not covered by filings, use the Historical Financials table from EODHD.
- key_events: Significant non-routine events (contracts, acquisitions, guidance changes). Date order.
- business_model: Infer from the filings data. Be specific about revenue sources.
- Do NOT invent numbers. Use filing summaries first, then historical financials for backfill. Leave empty if no data.
- next_reporting_date, work_on_hand, repeat_business_rate: include only if inferable from data.
- Return ONLY the JSON object, no markdown fences
`)

	return sb.String()
}

// timelineRaw is the JSON shape returned by Gemini for the company timeline.
type timelineRaw struct {
	BusinessModel string `json:"business_model"`
	Sector        string `json:"sector"`
	Industry      string `json:"industry"`
	Periods       []struct {
		Period          string `json:"period"`
		Revenue         string `json:"revenue"`
		RevenueGrowth   string `json:"revenue_growth"`
		Profit          string `json:"profit"`
		ProfitGrowth    string `json:"profit_growth"`
		Margin          string `json:"margin"`
		EPS             string `json:"eps"`
		Dividend        string `json:"dividend"`
		GuidanceGiven   string `json:"guidance_given"`
		GuidanceOutcome string `json:"guidance_outcome"`
	} `json:"periods"`
	KeyEvents []struct {
		Date   string `json:"date"`
		Event  string `json:"event"`
		Detail string `json:"detail"`
		Impact string `json:"impact"`
	} `json:"key_events"`
	NextReportingDate  string `json:"next_reporting_date"`
	WorkOnHand         string `json:"work_on_hand"`
	RepeatBusinessRate string `json:"repeat_business_rate"`
}

// parseTimelineResponse parses Gemini's JSON response into a CompanyTimeline.
func parseTimelineResponse(response string) *models.CompanyTimeline {
	response = stripMarkdownFences(response)

	var raw timelineRaw
	if err := json.Unmarshal([]byte(response), &raw); err != nil {
		return nil
	}

	if raw.BusinessModel == "" && len(raw.Periods) == 0 {
		return nil
	}

	periods := make([]models.PeriodSummary, 0, len(raw.Periods))
	for _, p := range raw.Periods {
		periods = append(periods, models.PeriodSummary{
			Period:          p.Period,
			Revenue:         p.Revenue,
			RevenueGrowth:   p.RevenueGrowth,
			Profit:          p.Profit,
			ProfitGrowth:    p.ProfitGrowth,
			Margin:          p.Margin,
			EPS:             p.EPS,
			Dividend:        p.Dividend,
			GuidanceGiven:   p.GuidanceGiven,
			GuidanceOutcome: p.GuidanceOutcome,
		})
	}

	events := make([]models.TimelineEvent, 0, len(raw.KeyEvents))
	for _, e := range raw.KeyEvents {
		events = append(events, models.TimelineEvent{
			Date:   e.Date,
			Event:  e.Event,
			Detail: e.Detail,
			Impact: e.Impact,
		})
	}

	return &models.CompanyTimeline{
		BusinessModel:      raw.BusinessModel,
		Sector:             raw.Sector,
		Industry:           raw.Industry,
		Periods:            periods,
		KeyEvents:          events,
		NextReportingDate:  raw.NextReportingDate,
		WorkOnHand:         raw.WorkOnHand,
		RepeatBusinessRate: raw.RepeatBusinessRate,
	}
}

// stripMarkdownFences removes markdown code fences from a response.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
