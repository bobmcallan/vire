// Package market provides market data services
package market

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/signals"
)

// Funnel implements a 3-stage screening pipeline
type Funnel struct {
	storage        interfaces.StorageManager
	eodhd          interfaces.EODHDClient
	gemini         interfaces.GeminiClient
	signalComputer *signals.Computer
	logger         *common.Logger
}

// NewFunnel creates a new funnel screener
func NewFunnel(
	storage interfaces.StorageManager,
	eodhd interfaces.EODHDClient,
	gemini interfaces.GeminiClient,
	signalComputer *signals.Computer,
	logger *common.Logger,
) *Funnel {
	return &Funnel{
		storage:        storage,
		eodhd:          eodhd,
		gemini:         gemini,
		signalComputer: signalComputer,
		logger:         logger,
	}
}

// FunnelScreen runs a 3-stage screening pipeline:
// Stage 1: EODHD Screener API (up to 100 candidates)
// Stage 2: Fundamental refinement (top 25)
// Stage 3: Technical + signal scoring (top N, default 5)
func (f *Funnel) FunnelScreen(ctx context.Context, options interfaces.FunnelOptions) (*models.FunnelResult, error) {
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

	f.logger.Info().
		Str("exchange", options.Exchange).
		Int("limit", limit).
		Str("sector", options.Sector).
		Msg("Starting funnel screen")

	// Stage 1: EODHD Screener API
	stage1Start := time.Now()
	screenerResults, stage1Filters, err := f.stage1Screener(ctx, options)
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
	stage2Results := f.stage2Fundamentals(ctx, screenerResults, options)
	result.Stages = append(result.Stages, models.FunnelStage{
		Name:        "Fundamental Refinement",
		InputCount:  len(screenerResults),
		OutputCount: len(stage2Results),
		Duration:    time.Since(stage2Start),
		Filters:     "P/E quality, dividend yield, market cap, sector compliance",
	})

	if len(stage2Results) == 0 {
		result.Candidates = []*models.ScreenCandidate{}
		result.Duration = time.Since(start)
		return result, nil
	}

	// Stage 3: Technical + signal scoring
	stage3Start := time.Now()
	candidates := f.stage3Technical(ctx, stage2Results, options, limit)
	result.Stages = append(result.Stages, models.FunnelStage{
		Name:        "Technical + Signal Scoring",
		InputCount:  len(stage2Results),
		OutputCount: len(candidates),
		Duration:    time.Since(stage3Start),
		Filters:     "Quarterly returns, trend alignment, RSI, MACD, news quality",
	})

	result.Candidates = candidates
	result.Duration = time.Since(start)

	f.logger.Info().
		Int("final_candidates", len(candidates)).
		Dur("duration", result.Duration).
		Msg("Funnel screen complete")

	return result, nil
}

// stage1Screener uses the EODHD Screener API for broad filtering
func (f *Funnel) stage1Screener(ctx context.Context, options interfaces.FunnelOptions) ([]*models.ScreenerResult, string, error) {
	filters := []models.ScreenerFilter{
		{Field: "exchange", Operator: "=", Value: options.Exchange},
		{Field: "market_capitalization", Operator: ">", Value: 100000000}, // >$100M
		{Field: "earnings_share", Operator: ">", Value: 0},                // positive earnings
	}

	var filterDescs []string
	filterDescs = append(filterDescs, fmt.Sprintf("exchange=%s", options.Exchange))
	filterDescs = append(filterDescs, "market_cap>$100M")
	filterDescs = append(filterDescs, "EPS>0")

	if options.Sector != "" {
		filters = append(filters, models.ScreenerFilter{
			Field: "sector", Operator: "=", Value: options.Sector,
		})
		filterDescs = append(filterDescs, fmt.Sprintf("sector=%s", options.Sector))
	}

	// Apply strategy-based filters
	if options.Strategy != nil {
		if options.Strategy.CompanyFilter.MinMarketCap > 100000000 {
			// Override the default market cap filter
			filters[1] = models.ScreenerFilter{
				Field: "market_capitalization", Operator: ">", Value: options.Strategy.CompanyFilter.MinMarketCap,
			}
			filterDescs[1] = fmt.Sprintf("market_cap>$%.0fM", options.Strategy.CompanyFilter.MinMarketCap/1000000)
		}
	}

	opts := models.ScreenerOptions{
		Filters: filters,
		Sort:    "market_capitalization.desc",
		Limit:   100,
	}

	results, err := f.eodhd.ScreenStocks(ctx, opts)
	if err != nil {
		return nil, "", err
	}

	// Post-filter by strategy excluded sectors
	if options.Strategy != nil && len(options.Strategy.SectorPreferences.Excluded) > 0 && options.Sector == "" {
		filtered := make([]*models.ScreenerResult, 0, len(results))
		for _, r := range results {
			if !isSectorExcluded(r.Sector, options.Strategy.SectorPreferences.Excluded) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	f.logger.Debug().Int("results", len(results)).Msg("Stage 1: screener results")

	return results, strings.Join(filterDescs, ", "), nil
}

// stage2Fundamentals scores and filters by fundamental quality
func (f *Funnel) stage2Fundamentals(ctx context.Context, results []*models.ScreenerResult, options interfaces.FunnelOptions) []*models.ScreenerResult {
	type scored struct {
		result *models.ScreenerResult
		score  float64
	}

	maxPE := 20.0
	if options.Strategy != nil {
		switch options.Strategy.RiskAppetite.Level {
		case "conservative":
			maxPE = 15.0
		case "aggressive":
			maxPE = 25.0
		}
		if options.Strategy.CompanyFilter.MaxPE > 0 {
			maxPE = options.Strategy.CompanyFilter.MaxPE
		}
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
		if options.Strategy != nil {
			cf := options.Strategy.CompanyFilter
			if cf.MinDividendYield > 0 && r.DividendYield < cf.MinDividendYield/100 {
				continue
			}
			if len(cf.AllowedSectors) > 0 && r.Sector != "" {
				found := false
				for _, s := range cf.AllowedSectors {
					if strings.EqualFold(r.Sector, s) {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}
			if len(cf.ExcludedSectors) > 0 && isSectorExcluded(r.Sector, cf.ExcludedSectors) {
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

	// Take top 25
	cap := 25
	if len(scoredResults) < cap {
		cap = len(scoredResults)
	}

	out := make([]*models.ScreenerResult, cap)
	for i := 0; i < cap; i++ {
		out[i] = scoredResults[i].result
	}

	f.logger.Debug().Int("results", len(out)).Msg("Stage 2: fundamental refinement results")

	return out
}

// stage3Technical does full technical analysis on the refined set
func (f *Funnel) stage3Technical(ctx context.Context, results []*models.ScreenerResult, options interfaces.FunnelOptions, limit int) []*models.ScreenCandidate {
	// Collect market data for all stage 2 candidates
	tickers := make([]string, 0, len(results))
	for _, r := range results {
		ticker := r.Code + "." + options.Exchange
		tickers = append(tickers, ticker)
	}

	// Batch collect - uses existing market data if fresh, fetches otherwise
	if err := f.collectMarketDataBatch(ctx, tickers, options.IncludeNews); err != nil {
		f.logger.Warn().Err(err).Msg("Stage 3: some market data collection failed")
	}

	// Build screen candidates with full scoring
	screener := NewScreener(f.storage, f.eodhd, f.gemini, f.signalComputer, f.logger)

	maxPE := 25.0 // Wider than stock_screen since stage 2 already filtered
	if options.Strategy != nil {
		switch options.Strategy.RiskAppetite.Level {
		case "conservative":
			maxPE = 18.0
		case "aggressive":
			maxPE = 30.0
		}
	}
	minReturn := 10.0 // Same threshold as stock_screen — consistent quality bar

	candidates := make([]*models.ScreenCandidate, 0)

	for _, r := range results {
		ticker := r.Code + "." + options.Exchange

		marketData, err := f.storage.MarketDataStorage().GetMarketData(ctx, ticker)
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

		candidate := screener.evaluateCandidate(ctx, ticker, symbol, marketData, maxPE, minReturn)
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
	if f.gemini != nil && len(candidates) > 0 {
		for _, candidate := range candidates {
			analysis, err := screener.generateScreenAnalysis(ctx, candidate, options.Strategy)
			if err != nil {
				f.logger.Warn().Str("ticker", candidate.Ticker).Err(err).Msg("Failed to generate analysis")
				continue
			}
			candidate.Analysis = analysis
		}
	}

	f.logger.Debug().Int("results", len(candidates)).Msg("Stage 3: technical scoring results")

	return candidates
}

// collectMarketDataBatch collects market data for tickers, skipping fresh data
func (f *Funnel) collectMarketDataBatch(ctx context.Context, tickers []string, includeNews bool) error {
	needed := make([]string, 0, len(tickers))
	for _, ticker := range tickers {
		md, err := f.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil || md == nil || !common.IsFresh(md.EODUpdatedAt, common.FreshnessTodayBar) {
			needed = append(needed, ticker)
		}
	}

	if len(needed) == 0 {
		return nil
	}

	f.logger.Debug().Int("tickers", len(needed)).Msg("Stage 3: collecting market data for candidates")

	now := time.Now()
	for _, ticker := range needed {
		// Load existing or start fresh
		existing, _ := f.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		marketData := &models.MarketData{
			Ticker:   ticker,
			Exchange: extractExchange(ticker),
		}
		if existing != nil {
			marketData = existing
		}

		// Fetch EOD data
		eodResp, err := f.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
		if err != nil {
			f.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD for funnel candidate")
			continue
		}
		marketData.EOD = eodResp.Data
		marketData.EODUpdatedAt = now

		// Fetch fundamentals if missing or stale
		if marketData.Fundamentals == nil || !common.IsFresh(marketData.FundamentalsUpdatedAt, common.FreshnessFundamentals) {
			fundamentals, err := f.eodhd.GetFundamentals(ctx, ticker)
			if err != nil {
				f.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch fundamentals for funnel candidate")
			} else {
				marketData.Fundamentals = fundamentals
				marketData.FundamentalsUpdatedAt = now
			}
		}

		marketData.LastUpdated = now

		// Save
		if err := f.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			f.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save market data")
			continue
		}

		// Compute signals
		tickerSignals := f.signalComputer.Compute(marketData)
		if err := f.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
			f.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals")
		}
	}

	return nil
}
