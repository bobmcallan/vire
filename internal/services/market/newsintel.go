package market

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/models"
)

// generateNewsIntelligence uses Gemini to produce a critical news analysis for a ticker.
func (s *Service) generateNewsIntelligence(ctx context.Context, ticker string, companyName string, news []*models.NewsItem) *models.NewsIntelligence {
	if len(news) == 0 {
		return nil
	}

	// Build context from fundamentals if available
	sector, industry := "", ""
	md, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if md != nil && md.Fundamentals != nil {
		sector = md.Fundamentals.Sector
		industry = md.Fundamentals.Industry
	}

	prompt := buildNewsIntelPrompt(ticker, companyName, sector, industry, news)

	// Try URL context tool first — lets Gemini fetch article URLs and search for more
	response, err := s.gemini.GenerateWithURLContextTool(ctx, prompt)
	if err != nil {
		s.logger.Debug().Str("ticker", ticker).Err(err).Msg("URL context tool failed, falling back to GenerateContent")
		response, err = s.gemini.GenerateContent(ctx, prompt)
	}
	if err != nil {
		s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to generate news intelligence")
		return nil
	}

	intel := parseNewsIntelResponse(response)
	if intel == nil {
		s.logger.Warn().Str("ticker", ticker).Msg("Failed to parse news intelligence response")
		return nil
	}
	intel.GeneratedAt = time.Now()
	return intel
}

// buildNewsIntelPrompt creates the prompt for news intelligence analysis.
func buildNewsIntelPrompt(ticker, companyName, sector, industry string, news []*models.NewsItem) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are a financial news analyst. Analyze recent news for %s (%s)", companyName, ticker))
	if sector != "" || industry != "" {
		sb.WriteString(fmt.Sprintf(", a %s company in %s", sector, industry))
	}
	sb.WriteString(".\n\nRecent articles:\n")

	for i, n := range news {
		date := n.PublishedAt.Format("2006-01-02")
		sb.WriteString(fmt.Sprintf("%d. \"%s\" - %s (%s) - %s\n", i+1, n.Title, n.Source, date, n.URL))
	}

	sb.WriteString(`
Provide a critical analysis. Return ONLY valid JSON:
{
  "summary": "2-3 paragraph critical analysis of the news landscape. Be actionable — what does this mean for investors?",
  "overall_sentiment": "bullish|bearish|neutral|mixed",
  "key_themes": ["theme1", "theme2"],
  "impact_week": "Short-term impact assessment (1 sentence)",
  "impact_month": "Medium-term impact assessment (1 sentence)",
  "impact_year": "Long-term outlook (1 sentence)",
  "articles": [
    {
      "title": "article title",
      "url": "article url",
      "source": "source name",
      "credibility": "credible|fluff|promotional|speculative",
      "relevance": "high|medium|low",
      "summary": "One-line critical take on this article"
    }
  ]
}

Rules:
- Be skeptical. Flag Motley Fool, Simply Wall St, and similar sites as "fluff"
- Focus on material information that could move the stock price
- Distinguish between news with genuine substance vs clickbait/promotional content
- For each article, assess credibility and actual relevance to stock valuation
- The summary should be actionable — what does this news mean for investors?
- Include articles from the input list plus any additional relevant articles you find
- Return ONLY the JSON object, no markdown code fences, no explanation`)

	return sb.String()
}

// newsIntelResponse is the expected JSON shape from Gemini.
type newsIntelResponse struct {
	Summary          string `json:"summary"`
	OverallSentiment string `json:"overall_sentiment"`
	KeyThemes        []string `json:"key_themes"`
	ImpactWeek       string `json:"impact_week"`
	ImpactMonth      string `json:"impact_month"`
	ImpactYear       string `json:"impact_year"`
	Articles         []struct {
		Title       string `json:"title"`
		URL         string `json:"url"`
		Source      string `json:"source"`
		Credibility string `json:"credibility"`
		Relevance   string `json:"relevance"`
		Summary     string `json:"summary"`
	} `json:"articles"`
}

// parseNewsIntelResponse parses Gemini's JSON response into a NewsIntelligence struct.
func parseNewsIntelResponse(response string) *models.NewsIntelligence {
	// Strip markdown code fences if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var data newsIntelResponse
	if err := json.Unmarshal([]byte(response), &data); err != nil {
		return nil
	}

	if data.Summary == "" {
		return nil
	}

	articles := make([]models.AnalyzedArticle, 0, len(data.Articles))
	for _, a := range data.Articles {
		articles = append(articles, models.AnalyzedArticle{
			Title:       a.Title,
			URL:         a.URL,
			Source:      a.Source,
			Credibility: a.Credibility,
			Relevance:   a.Relevance,
			Summary:     a.Summary,
		})
	}

	return &models.NewsIntelligence{
		Summary:          data.Summary,
		OverallSentiment: data.OverallSentiment,
		KeyThemes:        data.KeyThemes,
		ImpactWeek:       data.ImpactWeek,
		ImpactMonth:      data.ImpactMonth,
		ImpactYear:       data.ImpactYear,
		Articles:         articles,
	}
}
