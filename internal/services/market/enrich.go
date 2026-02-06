package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/models"
)

// enrichFundamentals uses Gemini to fill in missing fundamentals data.
// Only enriches when EODHD returned empty data; skips if already enriched.
func (s *Service) enrichFundamentals(ctx context.Context, f *models.Fundamentals) {
	if f == nil || s.gemini == nil {
		return
	}

	// Skip if already enriched
	if !f.EnrichedAt.IsZero() {
		return
	}

	if f.IsETF {
		s.enrichETF(ctx, f)
	} else {
		s.enrichStock(ctx, f)
	}
}

// enrichETF fetches ETF description, top holdings, sector weights, and country weights via Gemini.
func (s *Service) enrichETF(ctx context.Context, f *models.Fundamentals) {
	// Check if enrichment is needed — skip if we already have good data
	hasDescription := f.Description != "" && f.Description != "NA"
	hasHoldings := len(f.TopHoldings) > 0
	hasSectors := len(f.SectorWeights) > 0
	if hasDescription && hasHoldings && hasSectors {
		return
	}

	ticker := extractTicker(f.Ticker) // "ACDC.AU" -> "ACDC"
	fundURL := resolveETFURL(ticker)

	prompt := buildETFEnrichmentPrompt(ticker, hasDescription, hasHoldings, hasSectors)

	var response string
	var err error

	if fundURL != "" {
		response, err = s.gemini.GenerateWithURLContext(ctx, prompt, []string{fundURL})
	}
	// Fall back to Gemini's knowledge if URL context fails or no URL available
	if err != nil || fundURL == "" {
		if err != nil {
			s.logger.Debug().Str("ticker", f.Ticker).Err(err).Msg("URL context failed, falling back to Gemini knowledge")
		}
		response, err = s.gemini.GenerateContent(ctx, prompt)
	}

	if err != nil {
		s.logger.Warn().Str("ticker", f.Ticker).Err(err).Msg("Failed to enrich ETF via Gemini")
		return
	}

	s.applyETFEnrichment(f, response, hasDescription, hasHoldings, hasSectors)
	f.EnrichedAt = time.Now()
}

// enrichStock fetches a company description via Gemini when EODHD returned empty.
func (s *Service) enrichStock(ctx context.Context, f *models.Fundamentals) {
	if f.Description != "" && f.Description != "NA" {
		return
	}

	ticker := extractTicker(f.Ticker)
	stockURL := resolveStockURL(ticker)
	prompt := buildStockEnrichmentPrompt(ticker)

	var response string
	var err error

	if stockURL != "" {
		response, err = s.gemini.GenerateWithURLContext(ctx, prompt, []string{stockURL})
	}
	if err != nil || stockURL == "" {
		if err != nil {
			s.logger.Debug().Str("ticker", f.Ticker).Err(err).Msg("URL context failed, falling back to Gemini knowledge")
		}
		response, err = s.gemini.GenerateContent(ctx, prompt)
	}

	if err != nil {
		s.logger.Warn().Str("ticker", f.Ticker).Err(err).Msg("Failed to enrich stock via Gemini")
		return
	}

	description := strings.TrimSpace(response)
	if description != "" {
		f.Description = description
		f.EnrichedAt = time.Now()
	}
}

// resolveETFURL returns the fund page URL for known ASX ETF providers.
func resolveETFURL(ticker string) string {
	t := strings.ToLower(ticker)

	// Global X ETFs
	globalXTickers := map[string]bool{
		"acdc": true, "ainf": true, "wire": true, "atom": true, "semi": true,
		"robo": true, "fang": true, "ggus": true, "ethi": true, "gxcl": true,
	}
	if globalXTickers[t] {
		return fmt.Sprintf("https://www.globalxetfs.com.au/funds/%s/", t)
	}

	// VanEck ETFs
	vanEckTickers := map[string]bool{
		"dfnd": true, "moat": true, "qual": true, "mvol": true, "ifra": true,
		"dhhf": true, "esgi": true, "gdx": true, "gldm": true,
	}
	if vanEckTickers[t] {
		return fmt.Sprintf("https://www.vaneck.com.au/etf/%s/portfolio/", t)
	}

	// Betashares ETFs
	betasharesTickers := map[string]bool{
		"diam": true, "ndq": true, "a200": true, "qhal": true, "hndq": true,
		"dzzf": true, "ioo": true, "iem": true, "stw": true,
	}
	if betasharesTickers[t] {
		return fmt.Sprintf("https://www.betashares.com.au/fund/%s/", t)
	}

	// PMGOLD - Perth Mint Gold
	if t == "pmgold" {
		return "https://www.perthmint.com/invest/asx-listed-products/pmgold/"
	}

	return ""
}

// resolveStockURL returns the ASX company page URL.
func resolveStockURL(ticker string) string {
	return fmt.Sprintf("https://www2.asx.com.au/markets/company/%s", strings.ToUpper(ticker))
}

// buildETFEnrichmentPrompt creates the prompt for ETF enrichment.
func buildETFEnrichmentPrompt(ticker string, hasDescription, hasHoldings, hasSectors bool) string {
	parts := []string{}
	if !hasDescription {
		parts = append(parts, `"description": a 2-3 sentence fund summary explaining the investment strategy and what this ETF tracks`)
	}
	if !hasHoldings {
		parts = append(parts, `"top_holdings": array of {"name": "Company Name", "ticker": "TICKER", "weight": percentage_number} for the top 10 holdings`)
	}
	if !hasSectors {
		parts = append(parts, `"sector_weights": array of {"sector": "Sector Name", "weight": percentage_number} for sector breakdown`)
	}
	// Always try to get country weights if we're enriching
	parts = append(parts, `"country_weights": array of {"country": "Country Name", "weight": percentage_number} for geographic allocation`)

	fields := strings.Join(parts, ",\n  ")

	return fmt.Sprintf(`Extract the following data about the %s ETF listed on the ASX. Return ONLY valid JSON with these fields:
{
  %s
}

Rules:
- Weights should be numbers (e.g. 25.5 for 25.5%%)
- Top holdings limited to 10 entries, sorted by weight descending
- Use common sector names (Technology, Healthcare, Financials, etc.)
- If a field is not available, use an empty array [] for arrays or "N/A" for description
- Return ONLY the JSON object, no markdown code fences, no explanation`, ticker, fields)
}

// buildStockEnrichmentPrompt creates the prompt for stock description enrichment.
func buildStockEnrichmentPrompt(ticker string) string {
	return fmt.Sprintf(`Provide a concise 2-3 sentence description of %s, an ASX-listed company.
Describe what the company does, its main business activities, and the sector it operates in.
Return ONLY the description text, no JSON, no markdown, no extra formatting.`, ticker)
}

// etfEnrichmentResponse represents the JSON response from Gemini for ETF enrichment.
type etfEnrichmentResponse struct {
	Description string `json:"description"`
	TopHoldings []struct {
		Name   string  `json:"name"`
		Ticker string  `json:"ticker"`
		Weight float64 `json:"weight"`
	} `json:"top_holdings"`
	SectorWeights []struct {
		Sector string  `json:"sector"`
		Weight float64 `json:"weight"`
	} `json:"sector_weights"`
	CountryWeights []struct {
		Country string  `json:"country"`
		Weight  float64 `json:"weight"`
	} `json:"country_weights"`
}

// applyETFEnrichment parses the Gemini JSON response and fills empty fields.
func (s *Service) applyETFEnrichment(f *models.Fundamentals, response string, hasDescription, hasHoldings, hasSectors bool) {
	// Strip markdown code fences if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var data etfEnrichmentResponse
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		s.logger.Warn().Str("ticker", f.Ticker).Err(err).Msg("Failed to parse ETF enrichment response")
		return
	}

	if !hasDescription && data.Description != "" && data.Description != "N/A" {
		f.Description = data.Description
	}

	if !hasHoldings && len(data.TopHoldings) > 0 {
		holdings := make([]models.ETFHolding, 0, len(data.TopHoldings))
		for _, h := range data.TopHoldings {
			holdings = append(holdings, models.ETFHolding{
				Name:   h.Name,
				Ticker: h.Ticker,
				Weight: h.Weight,
			})
		}
		f.TopHoldings = holdings
	}

	if !hasSectors && len(data.SectorWeights) > 0 {
		sectors := make([]models.SectorWeight, 0, len(data.SectorWeights))
		for _, sw := range data.SectorWeights {
			sectors = append(sectors, models.SectorWeight{
				Sector: sw.Sector,
				Weight: sw.Weight,
			})
		}
		f.SectorWeights = sectors
	}

	if len(f.CountryWeights) == 0 && len(data.CountryWeights) > 0 {
		countries := make([]models.CountryWeight, 0, len(data.CountryWeights))
		for _, cw := range data.CountryWeights {
			countries = append(countries, models.CountryWeight{
				Country: cw.Country,
				Weight:  cw.Weight,
			})
		}
		f.CountryWeights = countries
	}
}

// extractTicker extracts the base ticker from a fully qualified ticker (e.g. "ACDC.AU" -> "ACDC").
func extractTicker(ticker string) string {
	if idx := strings.LastIndex(ticker, "."); idx >= 0 {
		return ticker[:idx]
	}
	return ticker
}
