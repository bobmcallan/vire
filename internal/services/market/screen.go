// Package market provides market data services
package market

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/signals"
)

// Screener identifies quality-value stocks with proven returns
type Screener struct {
	storage        interfaces.StorageManager
	eodhd          interfaces.EODHDClient
	gemini         interfaces.GeminiClient
	signalComputer *signals.Computer
	logger         *common.Logger
}

// NewScreener creates a new screener
func NewScreener(
	storage interfaces.StorageManager,
	eodhd interfaces.EODHDClient,
	gemini interfaces.GeminiClient,
	signalComputer *signals.Computer,
	logger *common.Logger,
) *Screener {
	return &Screener{
		storage:        storage,
		eodhd:          eodhd,
		gemini:         gemini,
		signalComputer: signalComputer,
		logger:         logger,
	}
}

// ScreenStocks finds quality-value stocks matching criteria:
//   - Low P/E ratio (positive earnings)
//   - Positive financial outlook (price trajectory aligns with fundamentals)
//   - Consistent quarterly returns >= annualised 10% over past 3 quarters
//   - Good news support (credible, not story-stock hype)
//   - Not story stocks (real earnings, real revenue, established sector)
func (s *Screener) ScreenStocks(ctx context.Context, options interfaces.ScreenOptions) ([]*models.ScreenCandidate, error) {
	s.logger.Info().
		Str("exchange", options.Exchange).
		Int("limit", options.Limit).
		Float64("max_pe", options.MaxPE).
		Float64("min_return", options.MinQtrReturnPct).
		Msg("Running stock screen")

	symbols, err := s.eodhd.GetExchangeSymbols(ctx, options.Exchange)
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange symbols: %w", err)
	}

	s.logger.Debug().Int("symbols", len(symbols)).Msg("Fetched exchange symbols")

	// Apply defaults
	maxPE := options.MaxPE
	if maxPE <= 0 {
		maxPE = 20.0 // Default: P/E under 20
	}
	minReturn := options.MinQtrReturnPct
	if minReturn <= 0 {
		minReturn = 10.0 // Default: 10% annualised per quarter
	}

	candidates := make([]*models.ScreenCandidate, 0)

	for _, symbol := range symbols {
		if symbol.Type != "Common Stock" && symbol.Type != "" {
			continue
		}

		if options.Sector != "" && !strings.EqualFold(symbol.Type, options.Sector) {
			// Sector filtering happens on fundamentals below
		}

		ticker := symbol.Code + "." + options.Exchange

		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil {
			continue
		}

		// Must have fundamentals and price history
		if marketData.Fundamentals == nil || len(marketData.EOD) < 63 {
			continue
		}

		// Sector filter on fundamentals
		if options.Sector != "" && !strings.EqualFold(marketData.Fundamentals.Sector, options.Sector) {
			continue
		}

		candidate := s.evaluateCandidate(ctx, ticker, symbol, marketData, maxPE, minReturn)
		if candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Limit results
	if options.Limit > 0 && len(candidates) > options.Limit {
		candidates = candidates[:options.Limit]
	}

	// Generate AI analysis for top candidates
	if s.gemini != nil && len(candidates) > 0 {
		for _, candidate := range candidates {
			analysis, err := s.generateScreenAnalysis(ctx, candidate)
			if err != nil {
				s.logger.Warn().Str("ticker", candidate.Ticker).Err(err).Msg("Failed to generate screen analysis")
				continue
			}
			candidate.Analysis = analysis
		}
	}

	s.logger.Info().Int("candidates", len(candidates)).Msg("Stock screen complete")
	return candidates, nil
}

// evaluateCandidate scores a stock against all screen criteria
func (s *Screener) evaluateCandidate(
	ctx context.Context,
	ticker string,
	symbol *models.Symbol,
	marketData *models.MarketData,
	maxPE float64,
	minReturnPct float64,
) *models.ScreenCandidate {
	f := marketData.Fundamentals

	// Hard filters — reject immediately
	// Must have positive earnings (not a story stock)
	if f.PE <= 0 {
		return nil
	}
	// Must be under max P/E threshold
	if f.PE > maxPE {
		return nil
	}
	// Must have positive EPS
	if f.EPS <= 0 {
		return nil
	}
	// Must have meaningful market cap (no micro-caps / penny stocks)
	if f.MarketCap < 100_000_000 {
		return nil
	}

	// Calculate quarterly returns from EOD data
	qtrReturns := s.calculateQuarterlyReturns(marketData.EOD)
	if len(qtrReturns) < 3 {
		return nil
	}

	// All 3 quarters must meet the minimum annualised return
	for _, qr := range qtrReturns[:3] {
		if qr < minReturnPct {
			return nil
		}
	}
	avgReturn := (qtrReturns[0] + qtrReturns[1] + qtrReturns[2]) / 3.0

	// Get or compute signals
	tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
	if err != nil {
		tickerSignals = s.signalComputer.Compute(marketData)
	}

	// Score the candidate (weighted multi-factor)
	score, strengths, concerns := s.scoreCandidate(f, tickerSignals, marketData, qtrReturns, avgReturn)

	// Minimum score threshold
	if score < 0.50 {
		return nil
	}

	// News assessment
	newsSentiment, newsCredibility := s.assessNews(marketData)

	return &models.ScreenCandidate{
		Ticker:           ticker,
		Exchange:         symbol.Exchange,
		Name:             symbol.Name,
		Score:            score,
		Price:            tickerSignals.Price.Current,
		PE:               f.PE,
		EPS:              f.EPS,
		DividendYield:    f.DividendYield,
		MarketCap:        f.MarketCap,
		Sector:           f.Sector,
		Industry:         f.Industry,
		QuarterlyReturns: qtrReturns[:3],
		AvgQtrReturn:     avgReturn,
		Signals:          tickerSignals,
		NewsSentiment:    newsSentiment,
		NewsCredibility:  newsCredibility,
		Strengths:        strengths,
		Concerns:         concerns,
	}
}

// calculateQuarterlyReturns calculates annualised return for each of the last 3 quarters.
// Returns up to 3 values (most recent first) as annualised percentages.
// Each quarter is approximately 63 trading days.
func (s *Screener) calculateQuarterlyReturns(bars []models.EODBar) []float64 {
	const qtrDays = 63
	returns := make([]float64, 0, 3)

	for q := 0; q < 3; q++ {
		startIdx := q * qtrDays
		endIdx := startIdx + qtrDays

		if endIdx >= len(bars) {
			break
		}

		// bars[startIdx] is more recent, bars[endIdx] is older
		endPrice := bars[endIdx].Close
		startPrice := bars[startIdx].Close

		if endPrice <= 0 {
			break
		}

		// Quarterly return as a percentage
		qtrReturn := ((startPrice - endPrice) / endPrice) * 100
		// Annualise: multiply by 4 (4 quarters in a year)
		annualised := qtrReturn * 4

		returns = append(returns, annualised)
	}

	return returns
}

// scoreCandidate applies a weighted multi-factor scoring model
func (s *Screener) scoreCandidate(
	f *models.Fundamentals,
	sig *models.TickerSignals,
	data *models.MarketData,
	qtrReturns []float64,
	avgReturn float64,
) (float64, []string, []string) {
	score := 0.0
	strengths := make([]string, 0)
	concerns := make([]string, 0)

	// Factor 1: P/E quality (weight: 0.20)
	// Lower P/E = higher score, but not too low (could indicate distress)
	if f.PE >= 5 && f.PE <= 12 {
		score += 0.20
		strengths = append(strengths, fmt.Sprintf("Attractive P/E of %.1f (value range)", f.PE))
	} else if f.PE > 12 && f.PE <= 18 {
		score += 0.15
		strengths = append(strengths, fmt.Sprintf("Reasonable P/E of %.1f", f.PE))
	} else if f.PE > 0 && f.PE < 5 {
		score += 0.08
		concerns = append(concerns, fmt.Sprintf("Very low P/E of %.1f may indicate market concern", f.PE))
	} else {
		score += 0.05
	}

	// Factor 2: Consistent quarterly returns (weight: 0.25)
	// All 3 quarters already verified >= minReturn, score based on magnitude
	if avgReturn >= 40 {
		score += 0.25
		strengths = append(strengths, fmt.Sprintf("Strong avg quarterly return: %.1f%% annualised", avgReturn))
	} else if avgReturn >= 20 {
		score += 0.20
		strengths = append(strengths, fmt.Sprintf("Solid avg quarterly return: %.1f%% annualised", avgReturn))
	} else {
		score += 0.15
		strengths = append(strengths, fmt.Sprintf("Consistent quarterly return: %.1f%% annualised", avgReturn))
	}

	// Factor 3: Price trajectory alignment (weight: 0.20)
	// Price above SMA200 and SMA50 confirms fundamental outlook matches price
	if sig != nil {
		if sig.Trend == models.TrendBullish {
			score += 0.20
			strengths = append(strengths, "Bullish trend confirms financial outlook")
		} else if sig.Trend == models.TrendNeutral {
			score += 0.10
			concerns = append(concerns, "Neutral trend — price trajectory not fully confirming outlook")
		} else {
			score += 0.02
			concerns = append(concerns, "Bearish trend contradicts fundamental outlook")
		}

		// Bonus: price above all major SMAs
		if sig.Price.Current > sig.Price.SMA20 &&
			sig.Price.Current > sig.Price.SMA50 &&
			sig.Price.Current > sig.Price.SMA200 {
			score += 0.05
			strengths = append(strengths, "Price above all major moving averages")
		}
	}

	// Factor 4: Not a story stock (weight: 0.15)
	// Real market cap, real earnings, established sector, low beta
	isStoryStock := false
	if f.MarketCap >= 1_000_000_000 {
		score += 0.05
		strengths = append(strengths, "Large cap (>$1B)")
	} else if f.MarketCap >= 500_000_000 {
		score += 0.03
	}
	if f.EPS > 0 && f.PE > 0 && f.PE < 50 {
		score += 0.05
		strengths = append(strengths, "Real earnings with reasonable valuation")
	}
	if f.DividendYield > 0 {
		score += 0.03
		strengths = append(strengths, fmt.Sprintf("Pays dividend (%.2f%% yield)", f.DividendYield*100))
	}
	if f.Beta > 2.0 {
		concerns = append(concerns, fmt.Sprintf("High beta (%.2f) — volatile", f.Beta))
		isStoryStock = true
	}

	// Penalise story stock characteristics
	if isStoryStock {
		score -= 0.05
	}

	// Factor 5: News quality (weight: 0.10)
	if data.NewsIntelligence != nil {
		switch data.NewsIntelligence.OverallSentiment {
		case "bullish":
			score += 0.10
			strengths = append(strengths, "Bullish news sentiment")
		case "neutral":
			score += 0.05
		case "mixed":
			score += 0.03
			concerns = append(concerns, "Mixed news sentiment")
		case "bearish":
			concerns = append(concerns, "Bearish news sentiment")
		}

		// Check article credibility
		credible, fluff := 0, 0
		for _, a := range data.NewsIntelligence.Articles {
			if a.Credibility == "credible" {
				credible++
			} else if a.Credibility == "fluff" || a.Credibility == "promotional" {
				fluff++
			}
		}
		if fluff > credible && len(data.NewsIntelligence.Articles) > 0 {
			score -= 0.05
			concerns = append(concerns, "News coverage dominated by low-credibility sources")
		}
	}

	// Factor 6: Technical health (weight: 0.10)
	if sig != nil {
		if sig.Technical.RSI >= 40 && sig.Technical.RSI <= 65 {
			score += 0.05
			strengths = append(strengths, "RSI in healthy range (not overbought)")
		} else if sig.Technical.RSI > 70 {
			concerns = append(concerns, "RSI overbought — may be extended")
		}
		if sig.Technical.MACDCrossover == "bullish" {
			score += 0.05
			strengths = append(strengths, "Bullish MACD crossover")
		}
		if sig.VLI.Interpretation == "accumulating" {
			score += 0.03
			strengths = append(strengths, "Volume shows institutional accumulation")
		}
	}

	// Risk deductions
	if sig != nil {
		for _, flag := range sig.RiskFlags {
			switch flag {
			case "high_volatility":
				score -= 0.03
				concerns = append(concerns, "High volatility")
			case "low_liquidity":
				score -= 0.05
				concerns = append(concerns, "Low liquidity")
			case "high_valuation":
				score -= 0.03
				concerns = append(concerns, "High valuation flag")
			}
		}
	}

	// Clamp
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score, strengths, concerns
}

// assessNews summarises news quality for the candidate
func (s *Screener) assessNews(data *models.MarketData) (sentiment string, credibility string) {
	sentiment = "unknown"
	credibility = "unknown"

	if data.NewsIntelligence == nil {
		return
	}

	sentiment = data.NewsIntelligence.OverallSentiment

	credibleCount, totalCount := 0, len(data.NewsIntelligence.Articles)
	for _, a := range data.NewsIntelligence.Articles {
		if a.Credibility == "credible" {
			credibleCount++
		}
	}
	if totalCount == 0 {
		credibility = "unknown"
	} else if float64(credibleCount)/float64(totalCount) >= 0.6 {
		credibility = "high"
	} else if float64(credibleCount)/float64(totalCount) >= 0.3 {
		credibility = "mixed"
	} else {
		credibility = "low"
	}

	return
}

// generateScreenAnalysis creates AI analysis for a screen candidate
func (s *Screener) generateScreenAnalysis(ctx context.Context, candidate *models.ScreenCandidate) (string, error) {
	prompt := buildScreenAnalysisPrompt(candidate)
	return s.gemini.GenerateContent(ctx, prompt)
}

func buildScreenAnalysisPrompt(c *models.ScreenCandidate) string {
	var sb strings.Builder

	sb.WriteString("Analyze this quality-value stock candidate. This is NOT a speculative play — it has real earnings, low P/E, and consistent returns.\n\n")
	sb.WriteString(fmt.Sprintf("Ticker: %s\n", c.Ticker))
	sb.WriteString(fmt.Sprintf("Name: %s\n", c.Name))
	sb.WriteString(fmt.Sprintf("Sector: %s | Industry: %s\n", c.Sector, c.Industry))
	sb.WriteString(fmt.Sprintf("Price: $%.2f\n", c.Price))
	sb.WriteString(fmt.Sprintf("P/E Ratio: %.1f\n", c.PE))
	sb.WriteString(fmt.Sprintf("EPS: $%.2f\n", c.EPS))
	sb.WriteString(fmt.Sprintf("Market Cap: $%.0fM\n", c.MarketCap/1_000_000))
	sb.WriteString(fmt.Sprintf("Dividend Yield: %.2f%%\n", c.DividendYield*100))
	sb.WriteString(fmt.Sprintf("Score: %.0f/100\n\n", c.Score*100))

	sb.WriteString("Quarterly returns (annualised):\n")
	labels := []string{"Most recent", "Previous", "Earliest"}
	for i, r := range c.QuarterlyReturns {
		if i < len(labels) {
			sb.WriteString(fmt.Sprintf("- %s quarter: %.1f%%\n", labels[i], r))
		}
	}

	if len(c.Strengths) > 0 {
		sb.WriteString("\nStrengths:\n")
		for _, s := range c.Strengths {
			sb.WriteString(fmt.Sprintf("- %s\n", s))
		}
	}
	if len(c.Concerns) > 0 {
		sb.WriteString("\nConcerns:\n")
		for _, con := range c.Concerns {
			sb.WriteString(fmt.Sprintf("- %s\n", con))
		}
	}

	sb.WriteString(fmt.Sprintf("\nNews sentiment: %s (credibility: %s)\n", c.NewsSentiment, c.NewsCredibility))

	if c.Signals != nil {
		sb.WriteString(fmt.Sprintf("\nTechnical: Trend=%s, RSI=%.1f, Regime=%s\n",
			c.Signals.Trend, c.Signals.Technical.RSI, c.Signals.Regime.Current))
	}

	sb.WriteString("\nProvide a brief (3-4 sentences) assessment. Focus on:\n")
	sb.WriteString("1. Why the low P/E is justified or represents genuine value\n")
	sb.WriteString("2. Whether the quarterly return trajectory is sustainable\n")
	sb.WriteString("3. The key risk that could derail the thesis\n")
	sb.WriteString("Do NOT describe this as a speculative or turnaround play.")

	return sb.String()
}
