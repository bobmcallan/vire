// Package market provides market data services
package market

import (
	"context"
	"sort"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/signals"
)

// Sniper identifies turnaround stock opportunities
type Sniper struct {
	storage        interfaces.StorageManager
	eodhd          interfaces.EODHDClient
	gemini         interfaces.GeminiClient
	signalComputer *signals.Computer
	logger         *common.Logger
}

// NewSniper creates a new sniper
func NewSniper(
	storage interfaces.StorageManager,
	eodhd interfaces.EODHDClient,
	gemini interfaces.GeminiClient,
	signalComputer *signals.Computer,
	logger *common.Logger,
) *Sniper {
	return &Sniper{
		storage:        storage,
		eodhd:          eodhd,
		gemini:         gemini,
		signalComputer: signalComputer,
		logger:         logger,
	}
}

// FindSnipeBuys identifies turnaround stocks based on criteria
func (s *Sniper) FindSnipeBuys(ctx context.Context, options interfaces.SnipeOptions) ([]*models.SnipeBuy, error) {
	s.logger.Info().
		Str("exchange", options.Exchange).
		Int("limit", options.Limit).
		Str("sector", options.Sector).
		Msg("Scanning for snipe buys")

	// Get all symbols for the exchange
	symbols, err := s.eodhd.GetExchangeSymbols(ctx, options.Exchange)
	if err != nil {
		return nil, err
	}

	s.logger.Debug().Int("symbols", len(symbols)).Msg("Fetched exchange symbols")

	// Filter and score candidates
	candidates := make([]*models.SnipeBuy, 0)

	for _, symbol := range symbols {
		// Skip non-equity types
		if symbol.Type != "Common Stock" && symbol.Type != "" {
			continue
		}

		// Filter by sector if specified
		if options.Sector != "" && symbol.Type != options.Sector {
			continue
		}

		ticker := symbol.Code + "." + options.Exchange

		// Try to get existing market data
		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil {
			// Skip if no data available
			continue
		}

		// Get or compute signals
		tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
		if err != nil {
			tickerSignals = s.signalComputer.Compute(marketData)
		}

		// Score the candidate
		snipeBuy := s.scoreCandidate(ticker, symbol, marketData, tickerSignals)
		if snipeBuy != nil && snipeBuy.Score >= 0.6 {
			candidates = append(candidates, snipeBuy)
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
			analysis, err := s.generateAnalysis(ctx, candidate)
			if err != nil {
				s.logger.Warn().Str("ticker", candidate.Ticker).Err(err).Msg("Failed to generate AI analysis")
				continue
			}
			candidate.Analysis = analysis
		}
	}

	s.logger.Info().Int("candidates", len(candidates)).Msg("Snipe scan complete")

	return candidates, nil
}

// scoreCandidate evaluates a stock for snipe potential
func (s *Sniper) scoreCandidate(
	ticker string,
	symbol *models.Symbol,
	marketData *models.MarketData,
	tickerSignals *models.TickerSignals,
) *models.SnipeBuy {
	if tickerSignals == nil || len(marketData.EOD) == 0 {
		return nil
	}

	score := 0.0
	reasons := make([]string, 0)
	riskFactors := make([]string, 0)

	// Criteria 1: Oversold RSI (weight: 0.25)
	if tickerSignals.Technical.RSI < 30 {
		score += 0.25
		reasons = append(reasons, "RSI oversold (<30)")
	} else if tickerSignals.Technical.RSI < 40 {
		score += 0.15
		reasons = append(reasons, "RSI approaching oversold")
	}

	// Criteria 2: Near support level (weight: 0.20)
	if tickerSignals.Technical.NearSupport {
		score += 0.20
		reasons = append(reasons, "Testing support level")
	}

	// Criteria 3: PBAS underpricing (weight: 0.20)
	if tickerSignals.PBAS.Interpretation == "underpriced" {
		score += 0.20
		reasons = append(reasons, "PBAS indicates underpriced")
	}

	// Criteria 4: Volume accumulation (weight: 0.15)
	if tickerSignals.VLI.Interpretation == "accumulating" {
		score += 0.15
		reasons = append(reasons, "Volume suggests accumulation")
	}

	// Criteria 5: Regime shift potential (weight: 0.10)
	if tickerSignals.Regime.Current == models.RegimeAccumulation {
		score += 0.10
		reasons = append(reasons, "In accumulation phase")
	}

	// Criteria 6: Price near 52-week low (weight: 0.10)
	low52 := signals.Low52Week(marketData.EOD)
	if low52 > 0 {
		distFromLow := ((tickerSignals.Price.Current - low52) / low52) * 100
		if distFromLow < 10 {
			score += 0.10
			reasons = append(reasons, "Near 52-week low")
		}
	}

	// Risk factors (deductions)
	for _, flag := range tickerSignals.RiskFlags {
		riskFactors = append(riskFactors, flag)
		switch flag {
		case "high_volatility":
			score -= 0.05
		case "low_liquidity":
			score -= 0.10
		case "negative_earnings":
			score -= 0.05
		}
	}

	// Ensure score is in valid range
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	// Calculate target price (10% upside)
	currentPrice := tickerSignals.Price.Current
	targetPrice := currentPrice * 1.10
	upsidePct := 10.0

	// Adjust target based on resistance level
	if tickerSignals.Technical.ResistanceLevel > 0 && tickerSignals.Technical.ResistanceLevel < targetPrice {
		targetPrice = tickerSignals.Technical.ResistanceLevel
		upsidePct = ((targetPrice - currentPrice) / currentPrice) * 100
	}

	return &models.SnipeBuy{
		Ticker:      ticker,
		Exchange:    symbol.Exchange,
		Name:        symbol.Name,
		Score:       score,
		Price:       currentPrice,
		TargetPrice: targetPrice,
		UpsidePct:   upsidePct,
		Signals:     tickerSignals,
		Reasons:     reasons,
		RiskFactors: riskFactors,
		Sector:      marketData.Fundamentals.Sector,
	}
}

// generateAnalysis creates AI analysis for a snipe candidate
func (s *Sniper) generateAnalysis(ctx context.Context, candidate *models.SnipeBuy) (string, error) {
	prompt := buildSnipeAnalysisPrompt(candidate)
	return s.gemini.GenerateContent(ctx, prompt)
}

func buildSnipeAnalysisPrompt(candidate *models.SnipeBuy) string {
	prompt := "Analyze this potential turnaround stock opportunity:\n\n"
	prompt += "Ticker: " + candidate.Ticker + "\n"
	prompt += "Name: " + candidate.Name + "\n"
	prompt += "Sector: " + candidate.Sector + "\n"
	prompt += "Current Price: $" + formatFloat(candidate.Price) + "\n"
	prompt += "Target Price: $" + formatFloat(candidate.TargetPrice) + "\n"
	prompt += "Potential Upside: " + formatFloat(candidate.UpsidePct) + "%\n"
	prompt += "Score: " + formatFloat(candidate.Score*100) + "/100\n\n"

	prompt += "Bullish Signals:\n"
	for _, reason := range candidate.Reasons {
		prompt += "- " + reason + "\n"
	}

	if len(candidate.RiskFactors) > 0 {
		prompt += "\nRisk Factors:\n"
		for _, risk := range candidate.RiskFactors {
			prompt += "- " + risk + "\n"
		}
	}

	if candidate.Signals != nil {
		prompt += "\nTechnical Data:\n"
		prompt += "- RSI: " + formatFloat(candidate.Signals.Technical.RSI) + "\n"
		prompt += "- Trend: " + string(candidate.Signals.Trend) + "\n"
		prompt += "- Regime: " + string(candidate.Signals.Regime.Current) + "\n"
	}

	prompt += "\nProvide a brief (2-3 sentences) assessment of this opportunity, "
	prompt += "highlighting the key catalyst for potential upside and the main risk to monitor."

	return prompt
}

func formatFloat(f float64) string {
	return string(rune(int(f*100)/100)) + "." + string(rune(int(f*100)%100))
}
