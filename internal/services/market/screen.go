// Package market provides market data services
package market

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/signals"
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

// screenerAPIQuery calls the EODHD Screener API with provided filters and applies strategy sector exclusions.
func (s *Screener) screenerAPIQuery(ctx context.Context, exchange, sector string, strategy *models.PortfolioStrategy, extraFilters []models.ScreenerFilter) ([]*models.ScreenerResult, string, error) {
	filters := []models.ScreenerFilter{
		{Field: "exchange", Operator: "=", Value: exchange},
		{Field: "market_capitalization", Operator: ">", Value: 100000000}, // >$100M
		{Field: "earnings_share", Operator: ">", Value: 0},                // positive earnings
	}

	var filterDescs []string
	filterDescs = append(filterDescs, fmt.Sprintf("exchange=%s", exchange))
	filterDescs = append(filterDescs, "market_cap>$100M")
	filterDescs = append(filterDescs, "EPS>0")

	if sector != "" {
		filters = append(filters, models.ScreenerFilter{
			Field: "sector", Operator: "=", Value: sector,
		})
		filterDescs = append(filterDescs, fmt.Sprintf("sector=%s", sector))
	}

	// Apply strategy-based filters
	if strategy != nil {
		if strategy.CompanyFilter.MinMarketCap > 100000000 {
			// Override the default market cap filter
			filters[1] = models.ScreenerFilter{
				Field: "market_capitalization", Operator: ">", Value: strategy.CompanyFilter.MinMarketCap,
			}
			filterDescs[1] = fmt.Sprintf("market_cap>$%.0fM", strategy.CompanyFilter.MinMarketCap/1000000)
		}
	}

	// Apply any extra filters (e.g., snipe uses different criteria)
	filters = append(filters, extraFilters...)

	opts := models.ScreenerOptions{
		Filters: filters,
		Sort:    "market_capitalization.desc",
		Limit:   100,
	}

	results, err := s.eodhd.ScreenStocks(ctx, opts)
	if err != nil {
		return nil, "", err
	}

	// Post-filter by strategy excluded sectors (checks both sector and industry)
	if strategy != nil && len(strategy.SectorPreferences.Excluded) > 0 && sector == "" {
		filtered := make([]*models.ScreenerResult, 0, len(results))
		for _, r := range results {
			if !isSectorOrIndustryExcluded(r.Sector, r.Industry, strategy.SectorPreferences.Excluded) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	s.logger.Debug().Int("results", len(results)).Msg("Screener API results")

	return results, strings.Join(filterDescs, ", "), nil
}

// refineFundamentals scores and filters screener results by fundamental quality.
// Returns the top candidates (up to cap).
func (s *Screener) refineFundamentals(ctx context.Context, results []*models.ScreenerResult, maxPE float64, strategy *models.PortfolioStrategy, cap int) []*models.ScreenerResult {
	type scored struct {
		result *models.ScreenerResult
		score  float64
	}

	scoredResults := make([]scored, 0, len(results))

	for _, r := range results {
		// Hard filters
		if r.EarningsShare <= 0 {
			continue
		}

		// Compute P/E from price and EPS
		pe := 0.0
		if r.EarningsShare > 0 && r.AdjustedClose > 0 {
			pe = r.AdjustedClose / r.EarningsShare
		}
		if pe <= 0 || pe > maxPE {
			continue
		}

		// Strategy company filter checks
		if strategy != nil {
			cf := strategy.CompanyFilter
			if cf.MinDividendYield > 0 && r.DividendYield < cf.MinDividendYield/100 {
				continue
			}
			if len(cf.AllowedSectors) > 0 && r.Sector != "" {
				found := false
				for _, sec := range cf.AllowedSectors {
					if strings.EqualFold(r.Sector, sec) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if len(cf.ExcludedSectors) > 0 && isSectorOrIndustryExcluded(r.Sector, r.Industry, cf.ExcludedSectors) {
				continue
			}
		}

		// Score fundamentals
		score := 0.0

		// P/E quality
		if pe >= 5 && pe <= 12 {
			score += 0.30
		} else if pe > 12 && pe <= 18 {
			score += 0.20
		} else {
			score += 0.10
		}

		// Market cap tier
		if r.MarketCap >= 10_000_000_000 {
			score += 0.25 // mega cap
		} else if r.MarketCap >= 1_000_000_000 {
			score += 0.20 // large cap
		} else if r.MarketCap >= 500_000_000 {
			score += 0.15 // mid cap
		} else {
			score += 0.05
		}

		// Dividend yield bonus
		if r.DividendYield > 0.04 {
			score += 0.15
		} else if r.DividendYield > 0.02 {
			score += 0.10
		} else if r.DividendYield > 0 {
			score += 0.05
		}

		// EPS strength
		if r.EarningsShare > 1.0 {
			score += 0.15
		} else if r.EarningsShare > 0.5 {
			score += 0.10
		} else {
			score += 0.05
		}

		// Volume (trading liquidity proxy)
		if r.AvgVol200d > 1_000_000 {
			score += 0.10
		} else if r.AvgVol200d > 100_000 {
			score += 0.05
		}

		scoredResults = append(scoredResults, scored{result: r, score: score})
	}

	// Sort by score descending
	sort.Slice(scoredResults, func(i, j int) bool {
		return scoredResults[i].score > scoredResults[j].score
	})

	// Take top N
	if len(scoredResults) < cap {
		cap = len(scoredResults)
	}

	out := make([]*models.ScreenerResult, cap)
	for i := 0; i < cap; i++ {
		out[i] = scoredResults[i].result
	}

	s.logger.Debug().Int("results", len(out)).Msg("Fundamental refinement results")

	return out
}

// collectMarketDataBatch collects market data for tickers using concurrent fetching.
// Fundamentals and historical EOD are fetched concurrently with rate limiting.
func (s *Screener) collectMarketDataBatch(ctx context.Context, tickers []string, includeNews bool) error {
	// Partition tickers by freshness
	needEOD := make([]string, 0, len(tickers))
	needFundamentals := make([]string, 0, len(tickers))

	for _, ticker := range tickers {
		md, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || md == nil || !common.IsFresh(md.EODUpdatedAt, common.FreshnessTodayBar) {
			needEOD = append(needEOD, ticker)
		}
		if md == nil || md.Fundamentals == nil || !common.IsFresh(md.FundamentalsUpdatedAt, common.FreshnessFundamentals) ||
			(md.Fundamentals != nil && md.Fundamentals.ISIN == "") {
			needFundamentals = append(needFundamentals, ticker)
		}
	}

	if len(needEOD) == 0 && len(needFundamentals) == 0 {
		return nil
	}

	s.logger.Debug().
		Int("need_eod", len(needEOD)).
		Int("need_fundamentals", len(needFundamentals)).
		Msg("Collecting market data for candidates (concurrent)")

	now := time.Now()

	// Concurrent fetch semaphore (max 5 concurrent API requests)
	semaphore := make(chan struct{}, 5)

	// Fetch historical EOD data concurrently
	type eodResult struct {
		ticker string
		data   []models.EODBar
		err    error
	}
	eodChan := make(chan eodResult, len(needEOD))

	for _, ticker := range needEOD {
		go func(t string) {
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			eodResp, err := s.eodhd.GetEOD(ctx, t, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
			if err != nil {
				eodChan <- eodResult{ticker: t, err: err}
			} else {
				eodChan <- eodResult{ticker: t, data: eodResp.Data}
			}
		}(ticker)
	}

	// Fetch fundamentals concurrently
	type fundamentalsResult struct {
		ticker       string
		fundamentals *models.Fundamentals
		err          error
	}
	fundamentalsChan := make(chan fundamentalsResult, len(needFundamentals))

	for _, ticker := range needFundamentals {
		go func(t string) {
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			fundamentals, err := s.eodhd.GetFundamentals(ctx, t)
			fundamentalsChan <- fundamentalsResult{ticker: t, fundamentals: fundamentals, err: err}
		}(ticker)
	}

	// Collect EOD results
	eodResults := make(map[string][]models.EODBar)
	for range needEOD {
		result := <-eodChan
		if result.err != nil {
			s.logger.Warn().Str("ticker", result.ticker).Err(result.err).Msg("Failed to fetch EOD")
		} else {
			eodResults[result.ticker] = result.data
		}
	}
	close(eodChan)

	// Collect fundamentals results
	fundamentalsResults := make(map[string]*models.Fundamentals)
	for range needFundamentals {
		result := <-fundamentalsChan
		if result.err != nil {
			s.logger.Warn().Str("ticker", result.ticker).Err(result.err).Msg("Failed to fetch fundamentals")
		} else {
			fundamentalsResults[result.ticker] = result.fundamentals
		}
	}
	close(fundamentalsChan)

	s.logger.Debug().
		Int("eod_fetched", len(eodResults)).
		Int("fundamentals_fetched", len(fundamentalsResults)).
		Msg("Concurrent data fetch complete")

	// Now update each ticker's market data
	for _, ticker := range tickers {
		// Load existing or start fresh
		existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		marketData := &models.MarketData{
			Ticker:   ticker,
			Exchange: extractExchange(ticker),
		}
		if existing != nil {
			marketData = existing
		}

		// Apply EOD data
		if data, ok := eodResults[ticker]; ok {
			marketData.EOD = data
			marketData.EODUpdatedAt = now
		}

		// Apply fundamentals results
		if fundamentals, ok := fundamentalsResults[ticker]; ok {
			marketData.Fundamentals = fundamentals
			marketData.FundamentalsUpdatedAt = now
		}

		marketData.LastUpdated = now

		// Save
		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save market data")
			continue
		}

		// Compute signals
		tickerSignals := s.signalComputer.Compute(marketData)
		if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals")
		}
	}

	return nil
}

// screenViaExchangeSymbols is a fallback screening method when EODHD Screener API is unavailable.
// It fetches exchange symbols, samples them, and filters by fundamentals.
func (s *Screener) screenViaExchangeSymbols(ctx context.Context, options interfaces.ScreenOptions, maxPE, minReturn float64) ([]*models.ScreenCandidate, error) {
	s.logger.Info().Str("exchange", options.Exchange).Msg("Screening via exchange symbols fallback")

	// Get all symbols for the exchange
	symbols, err := s.eodhd.GetExchangeSymbols(ctx, options.Exchange)
	if err != nil {
		return nil, fmt.Errorf("failed to get exchange symbols: %w", err)
	}

	s.logger.Debug().Int("total_symbols", len(symbols)).Msg("Exchange symbols retrieved")

	// Filter to common stocks only (exclude ETFs, warrants, etc.)
	// Note: Symbol from exchange-symbol-list doesn't include sector, so sector filtering
	// is done later based on fundamentals
	filtered := make([]*models.Symbol, 0, len(symbols)/2)
	for _, sym := range symbols {
		// Filter by type - only common stocks
		if sym.Type != "Common Stock" && sym.Type != "" {
			continue
		}
		filtered = append(filtered, sym)
	}

	s.logger.Debug().Int("filtered_symbols", len(filtered)).Msg("After type/sector filtering")

	// Sample symbols (can't process thousands) - take random sample up to 100
	maxSample := 100
	if len(filtered) > maxSample {
		// Simple sampling: take every Nth symbol
		step := len(filtered) / maxSample
		sampled := make([]*models.Symbol, 0, maxSample)
		for i := 0; i < len(filtered) && len(sampled) < maxSample; i += step {
			sampled = append(sampled, filtered[i])
		}
		filtered = sampled
	}

	s.logger.Debug().Int("sampled", len(filtered)).Msg("Sampled symbols for screening")

	// Build tickers and fetch market data concurrently
	tickers := make([]string, 0, len(filtered))
	for _, sym := range filtered {
		tickers = append(tickers, sym.Code+"."+options.Exchange)
	}

	// Collect market data (will fetch fundamentals)
	if err := s.collectMarketDataBatch(ctx, tickers, options.IncludeNews); err != nil {
		s.logger.Warn().Err(err).Msg("Some market data collection failed")
	}

	// Evaluate each candidate
	candidates := make([]*models.ScreenCandidate, 0)
	for _, sym := range filtered {
		ticker := sym.Code + "." + options.Exchange

		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || marketData == nil {
			continue
		}

		if marketData.Fundamentals == nil || len(marketData.EOD) < 63 {
			continue
		}

		// Sector filtering based on fundamentals (exchange symbols don't include sector)
		if options.Sector != "" && !strings.EqualFold(marketData.Fundamentals.Sector, options.Sector) {
			continue
		}
		if options.Strategy != nil && len(options.Strategy.SectorPreferences.Excluded) > 0 {
			if isSectorOrIndustryExcluded(marketData.Fundamentals.Sector, marketData.Fundamentals.Industry, options.Strategy.SectorPreferences.Excluded) {
				continue
			}
		}
		if options.Strategy != nil && !isCountryAllowed(marketData.Fundamentals.CountryISO, options.Strategy.CompanyFilter.AllowedCountries) {
			continue
		}

		candidate := s.evaluateCandidate(ctx, ticker, sym, marketData, maxPE, minReturn)
		if candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Limit results
	limit := options.Limit
	if limit <= 0 {
		limit = 5
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	s.logger.Info().Int("candidates", len(candidates)).Msg("Exchange symbols screening complete")

	// AI analysis for final candidates
	if len(candidates) > 0 && s.gemini != nil {
		for _, candidate := range candidates {
			analysis, err := s.generateScreenAnalysis(ctx, candidate, options.Strategy)
			if err != nil {
				s.logger.Warn().Str("ticker", candidate.Ticker).Err(err).Msg("Failed to generate analysis")
				continue
			}
			candidate.Analysis = analysis
		}
	}

	return candidates, nil
}

// ScreenStocks finds quality-value stocks using the EODHD Screener API:
//  1. screenerAPIQuery with quality-value filters (falls back to exchange symbols if API unavailable)
//  2. refineFundamentals: P/E filter, scoring, top 25
//  3. collectMarketDataBatch: auto-fetch EOD + fundamentals
//  4. evaluateCandidate for each (quarterly returns, signals, full scoring)
//  5. Sort by score, limit results
//  6. AI analysis via Gemini for final candidates
func (s *Screener) ScreenStocks(ctx context.Context, options interfaces.ScreenOptions) ([]*models.ScreenCandidate, error) {
	s.logger.Info().
		Str("exchange", options.Exchange).
		Int("limit", options.Limit).
		Float64("max_pe", options.MaxPE).
		Float64("min_return", options.MinQtrReturnPct).
		Msg("Running stock screen")

	// Apply defaults, adjusted by strategy when user didn't specify
	maxPE := options.MaxPE
	if maxPE <= 0 {
		// Check strategy CompanyFilter first, then fall back to risk-based defaults
		if options.Strategy != nil && options.Strategy.CompanyFilter.MaxPE > 0 {
			maxPE = options.Strategy.CompanyFilter.MaxPE
		} else {
			maxPE = 20.0 // Default: P/E under 20
			if options.Strategy != nil {
				switch options.Strategy.RiskAppetite.Level {
				case "conservative":
					maxPE = 15.0
				case "aggressive":
					maxPE = 25.0
				}
			}
		}
	}
	minReturn := options.MinQtrReturnPct
	if minReturn <= 0 {
		// Check strategy CompanyFilter first, then fall back to default
		if options.Strategy != nil && options.Strategy.CompanyFilter.MinQtrReturnPct > 0 {
			minReturn = options.Strategy.CompanyFilter.MinQtrReturnPct
		} else {
			minReturn = 10.0 // Default: 10% annualised per quarter
		}
	}

	// Step 1: EODHD Screener API — server-side filtering
	// Falls back to exchange symbols approach if screener API is unavailable (403)
	screenerResults, _, err := s.screenerAPIQuery(ctx, options.Exchange, options.Sector, options.Strategy, nil)
	if err != nil {
		// Check if this is a 403/subscription error - fall back to exchange symbols approach
		if strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "Forbidden") {
			s.logger.Info().Msg("Screener API unavailable, using exchange symbols fallback")
			return s.screenViaExchangeSymbols(ctx, options, maxPE, minReturn)
		}
		return nil, fmt.Errorf("screener API query failed: %w", err)
	}

	if len(screenerResults) == 0 {
		s.logger.Info().Msg("Stock screen: no results from screener API")
		return []*models.ScreenCandidate{}, nil
	}

	// Step 2: Fundamental refinement — top 25
	refined := s.refineFundamentals(ctx, screenerResults, maxPE, options.Strategy, 25)

	if len(refined) == 0 {
		s.logger.Info().Msg("Stock screen: no results after fundamental refinement")
		return []*models.ScreenCandidate{}, nil
	}

	// Step 3: Auto-fetch market data for refined candidates
	tickers := make([]string, 0, len(refined))
	for _, r := range refined {
		tickers = append(tickers, r.Code+"."+options.Exchange)
	}
	if err := s.collectMarketDataBatch(ctx, tickers, options.IncludeNews); err != nil {
		s.logger.Warn().Err(err).Msg("Some market data collection failed")
	}

	// Step 4: Full candidate evaluation with signals
	candidates := make([]*models.ScreenCandidate, 0)
	for _, r := range refined {
		ticker := r.Code + "." + options.Exchange

		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || marketData == nil {
			continue
		}

		if marketData.Fundamentals == nil || len(marketData.EOD) < 63 {
			continue
		}

		symbol := &models.Symbol{
			Code:     r.Code,
			Name:     r.Name,
			Exchange: r.Exchange,
		}

		candidate := s.evaluateCandidate(ctx, ticker, symbol, marketData, maxPE, minReturn)
		if candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	// Strategy-based score adjustments (applied before sorting)
	if options.Strategy != nil {
		for _, c := range candidates {
			switch options.Strategy.RiskAppetite.Level {
			case "conservative":
				// Conservative: boost dividend payers, penalise non-payers
				if c.DividendYield > 0.03 { // >3% yield
					c.Score += 0.05
				} else if c.DividendYield <= 0 {
					c.Score -= 0.03
				}
			case "aggressive":
				// Aggressive: less weight on dividends (no penalty for non-payers)
			}
			// Clamp
			if c.Score > 1 {
				c.Score = 1
			}
			if c.Score < 0 {
				c.Score = 0
			}
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

	// Step 6: Generate AI analysis for top candidates
	if s.gemini != nil && len(candidates) > 0 {
		for _, candidate := range candidates {
			analysis, err := s.generateScreenAnalysis(ctx, candidate, options.Strategy)
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

// FunnelScreen runs the same pipeline as ScreenStocks but wraps each step
// with FunnelStage timing/counts and returns a FunnelResult.
// Uses wider intermediate limits for a more thorough scan.
func (s *Screener) FunnelScreen(ctx context.Context, options interfaces.FunnelOptions) (*models.FunnelResult, error) {
	start := time.Now()

	limit := options.Limit
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}

	result := &models.FunnelResult{
		Exchange: options.Exchange,
		Sector:   options.Sector,
		Stages:   make([]models.FunnelStage, 0, 3),
	}

	s.logger.Info().
		Str("exchange", options.Exchange).
		Int("limit", limit).
		Str("sector", options.Sector).
		Msg("Starting funnel screen")

	// Stage 1: EODHD Screener API
	stage1Start := time.Now()
	screenerResults, stage1Filters, err := s.screenerAPIQuery(ctx, options.Exchange, options.Sector, options.Strategy, nil)
	if err != nil {
		return nil, fmt.Errorf("stage 1 (screener) failed: %w", err)
	}
	result.Stages = append(result.Stages, models.FunnelStage{
		Name:        "EODHD Screener",
		InputCount:  0, // unknown - API-side filtering
		OutputCount: len(screenerResults),
		Duration:    time.Since(stage1Start),
		Filters:     stage1Filters,
	})

	if len(screenerResults) == 0 {
		result.Candidates = []*models.ScreenCandidate{}
		result.Duration = time.Since(start)
		return result, nil
	}

	// Stage 2: Fundamental refinement
	stage2Start := time.Now()
	maxPE := 20.0
	if options.Strategy != nil {
		// Strategy CompanyFilter takes precedence over risk-based defaults
		if options.Strategy.CompanyFilter.MaxPE > 0 {
			maxPE = options.Strategy.CompanyFilter.MaxPE
		} else {
			switch options.Strategy.RiskAppetite.Level {
			case "conservative":
				maxPE = 15.0
			case "aggressive":
				maxPE = 25.0
			}
		}
	}
	stage2Results := s.refineFundamentals(ctx, screenerResults, maxPE, options.Strategy, 25)
	result.Stages = append(result.Stages, models.FunnelStage{
		Name:        "Fundamental Refinement",
		InputCount:  len(screenerResults),
		OutputCount: len(stage2Results),
		Duration:    time.Since(stage2Start),
		Filters:     fmt.Sprintf("P/E < %.0f, dividend yield, market cap, sector compliance", maxPE),
	})

	if len(stage2Results) == 0 {
		result.Candidates = []*models.ScreenCandidate{}
		result.Duration = time.Since(start)
		return result, nil
	}

	// Stage 3: Technical + signal scoring
	stage3Start := time.Now()

	// Collect market data for all stage 2 candidates
	tickers := make([]string, 0, len(stage2Results))
	for _, r := range stage2Results {
		tickers = append(tickers, r.Code+"."+options.Exchange)
	}
	if err := s.collectMarketDataBatch(ctx, tickers, options.IncludeNews); err != nil {
		s.logger.Warn().Err(err).Msg("Stage 3: some market data collection failed")
	}

	// Stage 3 uses same maxPE as stage 2 (already filtered)
	stage3MaxPE := maxPE

	// Get minReturn from strategy or use default
	minReturn := 10.0
	if options.Strategy != nil && options.Strategy.CompanyFilter.MinQtrReturnPct > 0 {
		minReturn = options.Strategy.CompanyFilter.MinQtrReturnPct
	}

	candidates := make([]*models.ScreenCandidate, 0)
	for _, r := range stage2Results {
		ticker := r.Code + "." + options.Exchange

		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || marketData == nil {
			continue
		}

		if marketData.Fundamentals == nil || len(marketData.EOD) < 63 {
			continue
		}

		// Country filter — reject companies domiciled outside allowed countries
		if options.Strategy != nil && !isCountryAllowed(marketData.Fundamentals.CountryISO, options.Strategy.CompanyFilter.AllowedCountries) {
			continue
		}

		symbol := &models.Symbol{
			Code:     r.Code,
			Name:     r.Name,
			Exchange: r.Exchange,
		}

		candidate := s.evaluateCandidate(ctx, ticker, symbol, marketData, stage3MaxPE, minReturn)
		if candidate != nil {
			candidates = append(candidates, candidate)
		}
	}

	// Strategy-based score adjustments
	if options.Strategy != nil {
		for _, c := range candidates {
			switch options.Strategy.RiskAppetite.Level {
			case "conservative":
				if c.DividendYield > 0.03 {
					c.Score += 0.05
				} else if c.DividendYield <= 0 {
					c.Score -= 0.03
				}
			}
			if c.Score > 1 {
				c.Score = 1
			}
			if c.Score < 0 {
				c.Score = 0
			}
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// Limit
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}

	// Generate AI analysis for final candidates
	if s.gemini != nil && len(candidates) > 0 {
		for _, candidate := range candidates {
			analysis, err := s.generateScreenAnalysis(ctx, candidate, options.Strategy)
			if err != nil {
				s.logger.Warn().Str("ticker", candidate.Ticker).Err(err).Msg("Failed to generate analysis")
				continue
			}
			candidate.Analysis = analysis
		}
	}

	result.Stages = append(result.Stages, models.FunnelStage{
		Name:        "Technical + Signal Scoring",
		InputCount:  len(stage2Results),
		OutputCount: len(candidates),
		Duration:    time.Since(stage3Start),
		Filters:     "Quarterly returns, trend alignment, RSI, MACD, news quality",
	})

	result.Candidates = candidates
	result.Duration = time.Since(start)

	s.logger.Info().
		Int("final_candidates", len(candidates)).
		Dur("duration", result.Duration).
		Msg("Funnel screen complete")

	return result, nil
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
func (s *Screener) generateScreenAnalysis(ctx context.Context, candidate *models.ScreenCandidate, strategy *models.PortfolioStrategy) (string, error) {
	prompt := buildScreenAnalysisPrompt(candidate, strategy)
	return s.gemini.GenerateContent(ctx, prompt)
}

func buildScreenAnalysisPrompt(c *models.ScreenCandidate, strategy *models.PortfolioStrategy) string {
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

	// Strategy context: only structured fields, never free-text
	if strategy != nil {
		sb.WriteString("\nInvestor Profile:\n")
		if strategy.TargetReturns.AnnualPct > 0 {
			sb.WriteString(fmt.Sprintf("- Target annual return: %.1f%%\n", strategy.TargetReturns.AnnualPct))
		}
		if strategy.IncomeRequirements.DividendYieldPct > 0 {
			sb.WriteString(fmt.Sprintf("- Target dividend yield: %.1f%%\n", strategy.IncomeRequirements.DividendYieldPct))
		}
	}

	sb.WriteString("\nProvide a brief (3-4 sentences) assessment. Focus on:\n")
	sb.WriteString("1. Why the low P/E is justified or represents genuine value\n")
	sb.WriteString("2. Whether the quarterly return trajectory is sustainable\n")
	sb.WriteString("3. The key risk that could derail the thesis\n")
	sb.WriteString("Do NOT describe this as a speculative or turnaround play.")

	return sb.String()
}
