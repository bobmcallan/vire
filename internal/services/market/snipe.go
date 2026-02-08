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

// FindSnipeBuys identifies turnaround stocks using the EODHD Screener API:
//  1. screenerAPIQuery with broad turnaround filters (lower market cap, no earnings requirement)
//  2. collectMarketDataBatch: auto-fetch EOD + fundamentals for candidates
//  3. Compute signals and score with turnaround-specific criteria
//  4. Strategy adjustments, sort, limit
//  5. AI analysis for final candidates
func (s *Sniper) FindSnipeBuys(ctx context.Context, options interfaces.SnipeOptions) ([]*models.SnipeBuy, error) {
	s.logger.Info().
		Str("exchange", options.Exchange).
		Int("limit", options.Limit).
		Str("sector", options.Sector).
		Msg("Scanning for snipe buys")

	// Step 1: EODHD Screener API with broad turnaround filters
	// Use a Screener instance for the shared API call logic
	screener := NewScreener(s.storage, s.eodhd, s.gemini, s.signalComputer, s.logger)

	// Broad filters for turnarounds: lower market cap, NO earnings requirement
	broadFilters := []models.ScreenerFilter{
		{Field: "exchange", Operator: "=", Value: options.Exchange},
		{Field: "market_capitalization", Operator: ">", Value: 50000000}, // >$50M (lower for turnarounds)
	}

	var filterDescs []string
	filterDescs = append(filterDescs, fmt.Sprintf("exchange=%s", options.Exchange))
	filterDescs = append(filterDescs, "market_cap>$50M")

	if options.Sector != "" {
		broadFilters = append(broadFilters, models.ScreenerFilter{
			Field: "sector", Operator: "=", Value: options.Sector,
		})
		filterDescs = append(filterDescs, fmt.Sprintf("sector=%s", options.Sector))
	}

	// Apply strategy-based market cap override
	if options.Strategy != nil && options.Strategy.CompanyFilter.MinMarketCap > 50000000 {
		broadFilters[1] = models.ScreenerFilter{
			Field: "market_capitalization", Operator: ">", Value: options.Strategy.CompanyFilter.MinMarketCap,
		}
	}

	opts := models.ScreenerOptions{
		Filters: broadFilters,
		Sort:    "market_capitalization.desc",
		Limit:   100,
	}

	screenerResults, err := s.eodhd.ScreenStocks(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("screener API query failed: %w", err)
	}

	// Post-filter by strategy excluded sectors
	if options.Strategy != nil && len(options.Strategy.SectorPreferences.Excluded) > 0 && options.Sector == "" {
		filtered := make([]*models.ScreenerResult, 0, len(screenerResults))
		for _, r := range screenerResults {
			if !isSectorExcluded(r.Sector, options.Strategy.SectorPreferences.Excluded) {
				filtered = append(filtered, r)
			}
		}
		screenerResults = filtered
	}

	s.logger.Debug().
		Int("results", len(screenerResults)).
		Strs("filters", filterDescs).
		Msg("Snipe screener API results")

	if len(screenerResults) == 0 {
		s.logger.Info().Msg("Snipe scan: no results from screener API")
		return []*models.SnipeBuy{}, nil
	}

	// Step 2: Auto-fetch market data for candidates
	tickers := make([]string, 0, len(screenerResults))
	for _, r := range screenerResults {
		tickers = append(tickers, r.Code+"."+options.Exchange)
	}
	if err := screener.collectMarketDataBatch(ctx, tickers, options.IncludeNews); err != nil {
		s.logger.Warn().Err(err).Msg("Some market data collection failed for snipe candidates")
	}

	// Step 3: Compute signals and score each candidate
	candidates := make([]*models.SnipeBuy, 0)

	for _, r := range screenerResults {
		ticker := r.Code + "." + options.Exchange

		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || marketData == nil {
			continue
		}

		// Get or compute signals
		tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
		if err != nil {
			tickerSignals = s.signalComputer.Compute(marketData)
		}

		// Strategy company filter checks
		if options.Strategy != nil && marketData.Fundamentals != nil && !passesCompanyFilter(marketData.Fundamentals, options.Strategy.CompanyFilter) {
			continue
		}

		// Build symbol from screener result
		symbol := &models.Symbol{
			Code:     r.Code,
			Name:     r.Name,
			Exchange: r.Exchange,
		}

		// Score the candidate
		snipeBuy := s.scoreCandidate(ticker, symbol, marketData, tickerSignals)
		if snipeBuy != nil && snipeBuy.Score >= 0.6 {
			// Conservative strategies penalise high-volatility candidates
			if options.Strategy != nil && options.Strategy.RiskAppetite.Level == "conservative" {
				for _, flag := range tickerSignals.RiskFlags {
					if flag == "high_volatility" {
						snipeBuy.Score -= 0.10
						break
					}
				}
				if snipeBuy.Score < 0.6 {
					continue
				}
			}
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

	// Step 5: Generate AI analysis for top candidates
	if s.gemini != nil && len(candidates) > 0 {
		for _, candidate := range candidates {
			analysis, err := s.generateAnalysis(ctx, candidate, options.Strategy)
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

	sector := ""
	if marketData.Fundamentals != nil {
		sector = marketData.Fundamentals.Sector
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
		Sector:      sector,
	}
}

// generateAnalysis creates AI analysis for a snipe candidate
func (s *Sniper) generateAnalysis(ctx context.Context, candidate *models.SnipeBuy, strategy *models.PortfolioStrategy) (string, error) {
	prompt := buildSnipeAnalysisPrompt(candidate, strategy)
	return s.gemini.GenerateContent(ctx, prompt)
}

func buildSnipeAnalysisPrompt(candidate *models.SnipeBuy, strategy *models.PortfolioStrategy) string {
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

	// Strategy context: only structured fields, never free-text
	if strategy != nil {
		prompt += "\nInvestor Profile:\n"
		if strategy.RiskAppetite.Level != "" {
			prompt += "- Risk appetite: " + strategy.RiskAppetite.Level + "\n"
		}
		if len(strategy.SectorPreferences.Preferred) > 0 {
			prompt += "- Preferred sectors: " + strings.Join(strategy.SectorPreferences.Preferred, ", ") + "\n"
		}
	}

	prompt += "\nProvide a brief (2-3 sentences) assessment of this opportunity, "
	prompt += "highlighting the key catalyst for potential upside and the main risk to monitor."

	return prompt
}

// passesCompanyFilter checks if fundamentals pass the strategy's company filter criteria.
// Returns true if no filter is set or all criteria pass.
func passesCompanyFilter(f *models.Fundamentals, filter models.CompanyFilter) bool {
	if f == nil {
		return true // No fundamentals to filter against
	}

	if filter.MinMarketCap > 0 && f.MarketCap < filter.MinMarketCap {
		return false
	}
	if filter.MaxPE > 0 && f.PE > 0 && f.PE > filter.MaxPE {
		return false
	}
	if filter.MinDividendYield > 0 && f.DividendYield < filter.MinDividendYield/100 {
		return false
	}
	if len(filter.ExcludedSectors) > 0 && isSectorExcluded(f.Sector, filter.ExcludedSectors) {
		return false
	}
	if len(filter.AllowedSectors) > 0 && f.Sector != "" {
		found := false
		for _, s := range filter.AllowedSectors {
			if strings.EqualFold(f.Sector, s) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// isSectorExcluded checks if a sector is in the excluded list (case-insensitive).
func isSectorExcluded(sector string, excluded []string) bool {
	for _, ex := range excluded {
		if strings.EqualFold(sector, ex) {
			return true
		}
	}
	return false
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.2f", f)
}
