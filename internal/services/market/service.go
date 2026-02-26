// Package market provides market data services
package market

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/signals"
)

// Service implements MarketService
type Service struct {
	storage             interfaces.StorageManager
	eodhd               interfaces.EODHDClient
	gemini              interfaces.GeminiClient
	signalComputer      *signals.Computer
	logger              *common.Logger
	filingSizeThreshold int64 // PDFs above this size (bytes) are processed one-at-a-time (0 = use default 5MB)
}

// NewService creates a new market service
func NewService(
	storage interfaces.StorageManager,
	eodhd interfaces.EODHDClient,
	gemini interfaces.GeminiClient,
	logger *common.Logger,
) *Service {
	return &Service{
		storage:        storage,
		eodhd:          eodhd,
		gemini:         gemini,
		signalComputer: signals.NewComputer(),
		logger:         logger,
	}
}

// SetFilingSizeThreshold sets the size threshold for large filing handling.
func (s *Service) SetFilingSizeThreshold(threshold int64) {
	s.filingSizeThreshold = threshold
}

// getFilingSizeThreshold returns the configured threshold or the default (5MB).
func (s *Service) getFilingSizeThreshold() int64 {
	if s.filingSizeThreshold > 0 {
		return s.filingSizeThreshold
	}
	return 5 * 1024 * 1024
}

// CollectMarketData fetches and stores market data for tickers.
// When force is true, all data is re-fetched regardless of freshness.
func (s *Service) CollectMarketData(ctx context.Context, tickers []string, includeNews bool, force bool) error {
	now := time.Now()

	for _, ticker := range tickers {
		s.logger.Debug().Str("ticker", ticker).Bool("force", force).Msg("Collecting market data")

		// Load existing data from storage
		existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)

		// Start with existing or blank
		marketData := &models.MarketData{
			Ticker:   ticker,
			Exchange: extractExchange(ticker),
		}
		if existing != nil {
			marketData = existing
		}

		// Schema-aware invalidation: clear stale derived data from older schema versions
		if existing != nil && existing.DataVersion != common.SchemaVersion {
			s.logger.Info().Str("ticker", ticker).
				Str("cached_version", existing.DataVersion).
				Str("current_version", common.SchemaVersion).
				Msg("Schema mismatch — clearing stale derived data and forcing fundamentals re-fetch")
			marketData.FilingSummaries = nil
			marketData.FilingSummariesUpdatedAt = time.Time{}
			marketData.CompanyTimeline = nil
			marketData.CompanyTimelineUpdatedAt = time.Time{}
			marketData.FundamentalsUpdatedAt = time.Time{} // force re-fetch to pick up new parsed fields
		}

		eodChanged := false

		// --- EOD bars ---
		if force || existing == nil || !common.IsFresh(existing.EODUpdatedAt, common.FreshnessTodayBar) {
			var eodResp *models.EODResponse
			var err error

			// Incremental fetch: only bars after the latest stored date
			if !force && existing != nil && len(existing.EOD) > 0 {
				// EOD is sorted descending (most recent first)
				latestDate := existing.EOD[0].Date
				fromDate := latestDate.AddDate(0, 0, 1) // day after last bar
				if fromDate.Before(now) {
					s.logger.Debug().Str("ticker", ticker).Str("from", fromDate.Format(time.RFC3339)).Msg("Incremental EOD fetch")
					eodResp, err = s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(fromDate, now))
					if err != nil {
						s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch incremental EOD data")
					} else if len(eodResp.Data) > 0 {
						// Merge: new bars (descending) go in front of existing
						marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
						eodChanged = true
					}
				}
				// Even if no new bars, mark as checked
				marketData.EODUpdatedAt = now
			} else {
				// Full fetch
				eodResp, err = s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
				if err != nil {
					s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data")
					continue
				}
				marketData.EOD = eodResp.Data
				marketData.EODUpdatedAt = now
				eodChanged = true
			}
		}

		// --- Fundamentals ---
		// Also re-fetch if ISIN is missing (migration: old cache has listing-country, not ISIN-based domicile)
		needFundamentals := force || existing == nil || !common.IsFresh(existing.FundamentalsUpdatedAt, common.FreshnessFundamentals) ||
			(existing != nil && existing.Fundamentals != nil && existing.Fundamentals.ISIN == "")
		if needFundamentals {
			fundamentals, err := s.eodhd.GetFundamentals(ctx, ticker)
			if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch fundamentals")
			} else {
				if fundamentals != nil {
					s.enrichFundamentals(ctx, fundamentals)
				}
				marketData.Fundamentals = fundamentals
				marketData.FundamentalsUpdatedAt = now
			}
		}

		// --- News ---
		if includeNews && (force || existing == nil || !common.IsFresh(existing.NewsUpdatedAt, common.FreshnessNews)) {
			news, err := s.eodhd.GetNews(ctx, ticker, 10)
			if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch news")
			} else {
				marketData.News = news
				marketData.NewsUpdatedAt = now
			}
		}

		// --- News Intelligence ---
		if includeNews && s.gemini != nil && len(marketData.News) > 0 {
			if force || !common.IsFresh(marketData.NewsIntelUpdatedAt, common.FreshnessNewsIntel) {
				intel := s.generateNewsIntelligence(ctx, ticker, marketData.Name, marketData.News)
				if intel != nil {
					marketData.NewsIntelligence = intel
					marketData.NewsIntelUpdatedAt = now
				}
			}
		}

		// --- Filings ---
		if force || existing == nil || !common.IsFresh(existing.FilingsIndexUpdatedAt, common.FreshnessFilings) {
			filings, err := s.collectFilings(ctx, ticker)
			if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to collect filings")
			} else {
				filings = s.downloadFilingPDFs(ctx, extractCode(ticker), filings)
				marketData.Filings = filings
				marketData.FilingsIndexUpdatedAt = now
			}
		}

		// --- Filing Summaries (per-filing extraction) ---
		if s.gemini != nil && len(marketData.Filings) > 0 {
			if force {
				// Clear existing summaries so all filings are re-analyzed from scratch
				marketData.FilingSummaries = nil
				marketData.FilingSummariesUpdatedAt = time.Time{}
				marketData.CompanyTimeline = nil
				marketData.CompanyTimelineUpdatedAt = time.Time{}
			}
			newSummaries, changed := s.summarizeNewFilings(ctx, ticker, marketData.Filings, marketData.FilingSummaries, nil)
			if changed {
				marketData.FilingSummaries = newSummaries
				marketData.FilingSummariesUpdatedAt = now

				// Rebuild timeline when summaries change
				timeline := s.generateCompanyTimeline(ctx, ticker, marketData.FilingSummaries, marketData.Fundamentals)
				if timeline != nil {
					marketData.CompanyTimeline = timeline
					marketData.CompanyTimelineUpdatedAt = now
				}
			} else if !common.IsFresh(marketData.CompanyTimelineUpdatedAt, common.FreshnessTimeline) {
				// Periodically rebuild timeline even without new summaries
				timeline := s.generateCompanyTimeline(ctx, ticker, marketData.FilingSummaries, marketData.Fundamentals)
				if timeline != nil {
					marketData.CompanyTimeline = timeline
					marketData.CompanyTimelineUpdatedAt = now
				}
			}
		}

		// Tag with current schema version for future mismatch detection
		marketData.DataVersion = common.SchemaVersion

		// Update LastUpdated to max of component timestamps
		marketData.LastUpdated = now

		// Save market data
		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			s.logger.Error().Str("ticker", ticker).Err(err).Msg("Failed to save market data")
			continue
		}

		// Compute and save signals only when EOD data changed
		if eodChanged {
			tickerSignals := s.signalComputer.Compute(marketData)
			if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals")
			}
		}
	}

	return nil
}

// CollectCoreMarketData fetches only EOD bars and fundamentals (fast path).
// Uses bulk EOD API where possible. Skips filings, news, AI summaries.
// When force is true, all data is re-fetched regardless of freshness.
func (s *Service) CollectCoreMarketData(ctx context.Context, tickers []string, force bool) error {
	if len(tickers) == 0 {
		return nil
	}

	now := time.Now()

	// Group tickers by exchange for bulk EOD
	byExchange := make(map[string][]string)
	for _, ticker := range tickers {
		exchange := extractExchange(ticker)
		if exchange == "" {
			exchange = "AU" // default
		}
		byExchange[exchange] = append(byExchange[exchange], ticker)
	}

	// Fetch bulk EOD (last day) per exchange
	bulkBars := make(map[string]models.EODBar)
	if s.eodhd != nil {
		for exchange, exchangeTickers := range byExchange {
			bars, err := s.eodhd.GetBulkEOD(ctx, exchange, exchangeTickers)
			if err != nil {
				s.logger.Warn().Str("exchange", exchange).Err(err).Msg("Bulk EOD fetch failed")
			} else {
				for k, v := range bars {
					bulkBars[k] = v
				}
			}
		}
	}

	// Process each ticker: EOD + fundamentals only
	const maxConcurrent = 5
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error

	for _, ticker := range tickers {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			break
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(ticker string) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.collectCoreTicker(ctx, ticker, bulkBars, force, now); err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", ticker, err))
				mu.Unlock()
			}
		}(ticker)
	}

	wg.Wait()

	if len(errs) > 0 {
		s.logger.Warn().Int("errors", len(errs)).Msg("CollectCoreMarketData completed with errors")
		return errors.Join(errs...)
	}

	return nil
}

// collectCoreTicker handles EOD + fundamentals for a single ticker in the fast path.
func (s *Service) collectCoreTicker(ctx context.Context, ticker string, bulkBars map[string]models.EODBar, force bool, now time.Time) error {
	existing, _ := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)

	marketData := &models.MarketData{
		Ticker:   ticker,
		Exchange: extractExchange(ticker),
	}
	if existing != nil {
		marketData = existing
	}

	// Schema-aware invalidation
	if existing != nil && existing.DataVersion != common.SchemaVersion {
		s.logger.Info().Str("ticker", ticker).Msg("Schema mismatch — clearing stale derived data (core path)")
		marketData.FilingSummaries = nil
		marketData.FilingSummariesUpdatedAt = time.Time{}
		marketData.CompanyTimeline = nil
		marketData.CompanyTimelineUpdatedAt = time.Time{}
		marketData.FundamentalsUpdatedAt = time.Time{}
	}

	eodChanged := false

	// --- EOD bars ---
	if force || existing == nil || !common.IsFresh(existing.EODUpdatedAt, common.FreshnessTodayBar) {
		// Try bulk bar first
		if bar, ok := bulkBars[ticker]; ok && !force && existing != nil && len(existing.EOD) > 0 {
			barDate := bar.Date.Format("2006-01-02")
			latestDate := existing.EOD[0].Date.Format("2006-01-02")
			if barDate != latestDate {
				marketData.EOD = mergeEODBars([]models.EODBar{bar}, existing.EOD)
				eodChanged = true
			}
			marketData.EODUpdatedAt = now
		} else if s.eodhd != nil {
			// Full or incremental fetch
			if !force && existing != nil && len(existing.EOD) > 0 {
				latestDate := existing.EOD[0].Date
				fromDate := latestDate.AddDate(0, 0, 1)
				if fromDate.Before(now) {
					eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(fromDate, now))
					if err != nil {
						s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch incremental EOD data (core)")
					} else if len(eodResp.Data) > 0 {
						marketData.EOD = mergeEODBars(eodResp.Data, existing.EOD)
						eodChanged = true
					}
				}
				marketData.EODUpdatedAt = now
			} else {
				eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithDateRange(now.AddDate(-3, 0, 0), now))
				if err != nil {
					s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data (core)")
					return err
				}
				marketData.EOD = eodResp.Data
				marketData.EODUpdatedAt = now
				eodChanged = true
			}
		}
	}

	// --- Fundamentals ---
	needFundamentals := force || existing == nil || !common.IsFresh(existing.FundamentalsUpdatedAt, common.FreshnessFundamentals) ||
		(existing != nil && existing.Fundamentals != nil && existing.Fundamentals.ISIN == "")
	if needFundamentals && s.eodhd != nil {
		fundamentals, err := s.eodhd.GetFundamentals(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch fundamentals (core)")
		} else {
			if fundamentals != nil {
				s.enrichFundamentals(ctx, fundamentals)
			}
			marketData.Fundamentals = fundamentals
			marketData.FundamentalsUpdatedAt = now
		}
	}

	// --- Filings Index (fast: HTML index only, PDFs downloaded in background) ---
	needFilingsIndex := force || existing == nil || !common.IsFresh(existing.FilingsIndexUpdatedAt, common.FreshnessFilings)
	if needFilingsIndex {
		filings, err := s.collectFilings(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to collect filings index (core)")
		} else {
			// Merge with existing PDF paths to preserve previously downloaded files
			if existing != nil && len(existing.Filings) > 0 {
				filings = s.mergeFilingPDFPaths(existing.Filings, filings)
			}
			marketData.Filings = filings
			marketData.FilingsIndexUpdatedAt = now
		}
	}

	marketData.DataVersion = common.SchemaVersion
	marketData.LastUpdated = now

	if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
		return fmt.Errorf("failed to save market data: %w", err)
	}

	// Compute and save signals only when EOD data changed
	if eodChanged {
		tickerSignals := s.signalComputer.Compute(marketData)
		if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals (core)")
		}
	}

	return nil
}

// mergeEODBars merges new bars into existing bars, deduplicating by date.
// Both slices are expected to be sorted descending (most recent first).
func mergeEODBars(newBars, existingBars []models.EODBar) []models.EODBar {
	seen := make(map[string]struct{}, len(existingBars))
	for _, b := range existingBars {
		seen[b.Date.Format("2006-01-02")] = struct{}{}
	}

	merged := make([]models.EODBar, 0, len(newBars)+len(existingBars))
	for _, b := range newBars {
		key := b.Date.Format("2006-01-02")
		if _, exists := seen[key]; !exists {
			merged = append(merged, b)
			seen[key] = struct{}{}
		} else {
			// Replace existing bar with newer data (e.g. today's bar updated)
			merged = append(merged, b)
		}
	}
	// Append existing bars that weren't replaced
	for _, b := range existingBars {
		key := b.Date.Format("2006-01-02")
		replaced := false
		for _, nb := range newBars {
			if nb.Date.Format("2006-01-02") == key {
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, b)
		}
	}
	return merged
}

// GetStockData retrieves stock data with optional components
func (s *Service) GetStockData(ctx context.Context, ticker string, include interfaces.StockDataInclude) (*models.StockData, error) {
	stockData := &models.StockData{
		Ticker: ticker,
	}

	// Get market data from storage
	marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if err != nil {
		// Try to fetch fresh data with a timeout to prevent indefinite blocking
		collectCtx, collectCancel := context.WithTimeout(ctx, 60*time.Second)
		defer collectCancel()
		if err := s.CollectMarketData(collectCtx, []string{ticker}, include.News, false); err != nil {
			return nil, fmt.Errorf("failed to collect market data: %w", err)
		}
		marketData, err = s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil {
			return nil, fmt.Errorf("market data not found: %w", err)
		}
	}

	stockData.Exchange = marketData.Exchange
	stockData.Name = marketData.Name

	// Include price data
	if include.Price && len(marketData.EOD) > 0 {
		current := marketData.EOD[0]
		var prevClose float64
		if len(marketData.EOD) > 1 {
			prevClose = marketData.EOD[1].Close
		}

		stockData.Price = &models.PriceData{
			Current:       current.Close,
			Open:          current.Open,
			High:          current.High,
			Low:           current.Low,
			PreviousClose: prevClose,
			Change:        current.Close - prevClose,
			ChangePct:     ((current.Close - prevClose) / prevClose) * 100,
			Volume:        current.Volume,
			AvgVolume:     signals.AverageVolume(marketData.EOD, 20),
			High52Week:    signals.High52Week(marketData.EOD),
			Low52Week:     signals.Low52Week(marketData.EOD),
			LastUpdated:   marketData.LastUpdated,
		}

		// Populate historical price changes (consistent with portfolio implementation)
		// Yesterday: EOD[1] is previous trading day close (same as PreviousClose)
		if len(marketData.EOD) > 1 {
			stockData.Price.YesterdayClose = marketData.EOD[1].Close
			if marketData.EOD[1].Close > 0 {
				stockData.Price.YesterdayPct = ((current.Close - marketData.EOD[1].Close) / marketData.EOD[1].Close) * 100
			}
		}
		// Last week: ~5 trading days back (offset 5 from today = EOD[5])
		if len(marketData.EOD) > 5 {
			lastWeekClose := marketData.EOD[5].Close
			stockData.Price.LastWeekClose = lastWeekClose
			if lastWeekClose > 0 {
				stockData.Price.LastWeekPct = ((current.Close - lastWeekClose) / lastWeekClose) * 100
			}
		}

		// Attempt real-time price to override EOD close
		if s.eodhd != nil {
			if quote, err := s.eodhd.GetRealTimeQuote(ctx, ticker); err == nil && quote.Close > 0 {
				stockData.Price.Current = quote.Close
				stockData.Price.Open = quote.Open
				stockData.Price.High = quote.High
				stockData.Price.Low = quote.Low
				stockData.Price.Volume = quote.Volume
				stockData.Price.LastUpdated = quote.Timestamp
				if prevClose > 0 {
					stockData.Price.Change = quote.Close - prevClose
					stockData.Price.ChangePct = ((quote.Close - prevClose) / prevClose) * 100
				}
				s.logger.Info().Str("ticker", ticker).Float64("live_price", quote.Close).Msg("Using real-time price")
			} else if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Real-time price unavailable, using EOD close")
			}
		}
	}

	// Include fundamentals
	if include.Fundamentals {
		stockData.Fundamentals = marketData.Fundamentals
	}

	// Include signals
	if include.Signals {
		tickerSignals, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
		if err != nil {
			// Compute fresh signals
			tickerSignals = s.signalComputer.Compute(marketData)
		}
		stockData.Signals = tickerSignals
	}

	// Include news
	if include.News {
		stockData.News = marketData.News

		// Auto-generate news intelligence if we have articles but no cached intel
		if marketData.NewsIntelligence == nil && s.gemini != nil && len(marketData.News) > 0 {
			intel := s.generateNewsIntelligence(ctx, ticker, marketData.Name, marketData.News)
			if intel != nil {
				marketData.NewsIntelligence = intel
				marketData.NewsIntelUpdatedAt = time.Now()
				// Persist for next time
				_ = s.storage.MarketDataStorage().SaveMarketData(ctx, marketData)
			}
		}

		stockData.NewsIntelligence = marketData.NewsIntelligence
	}

	// Auto-collect filings from ASX if missing or stale
	if len(marketData.Filings) == 0 || !common.IsFresh(marketData.FilingsIndexUpdatedAt, common.FreshnessFilings) {
		filings, err := s.collectFilings(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to auto-collect filings")
		} else if len(filings) > 0 {
			filings = s.downloadFilingPDFs(ctx, extractCode(ticker), filings)
			marketData.Filings = filings
			marketData.FilingsIndexUpdatedAt = time.Now()
			_ = s.storage.MarketDataStorage().SaveMarketData(ctx, marketData)
		}
	}

	// Layer 2: Company Releases — serve stored data, generation handled by job manager
	stockData.Filings = marketData.Filings
	stockData.FilingSummaries = marketData.FilingSummaries

	// Layer 3: Company Timeline — serve stored data, generation handled by job manager
	stockData.Timeline = marketData.CompanyTimeline

	// Quality Assessment — compute if fundamentals are available but assessment is missing
	if marketData.Fundamentals != nil && marketData.QualityAssessment == nil {
		qa := computeQualityAssessment(marketData.Fundamentals)
		if qa != nil {
			marketData.QualityAssessment = qa
			_ = s.storage.MarketDataStorage().SaveMarketData(ctx, marketData)
		}
	}
	stockData.QualityAssessment = marketData.QualityAssessment

	return stockData, nil
}

// FindSnipeBuys identifies turnaround stocks
func (s *Service) FindSnipeBuys(ctx context.Context, options interfaces.SnipeOptions) ([]*models.SnipeBuy, error) {
	sniper := NewSniper(s.storage, s.eodhd, s.gemini, s.signalComputer, s.logger)
	return sniper.FindSnipeBuys(ctx, options)
}

// ScreenStocks finds quality-value stocks with low P/E and consistent returns
func (s *Service) ScreenStocks(ctx context.Context, options interfaces.ScreenOptions) ([]*models.ScreenCandidate, error) {
	screener := NewScreener(s.storage, s.eodhd, s.gemini, s.signalComputer, s.logger)
	return screener.ScreenStocks(ctx, options)
}

// FunnelScreen runs a 3-stage funnel: EODHD screener -> fundamentals -> technical scoring
func (s *Service) FunnelScreen(ctx context.Context, options interfaces.FunnelOptions) (*models.FunnelResult, error) {
	screener := NewScreener(s.storage, s.eodhd, s.gemini, s.signalComputer, s.logger)
	return screener.FunnelScreen(ctx, options)
}

// RefreshStaleData updates outdated market data
func (s *Service) RefreshStaleData(ctx context.Context, exchange string) error {
	// Get stale tickers (older than 24 hours)
	maxAge := int64(24 * 60 * 60) // 24 hours in seconds
	staleTickers, err := s.storage.MarketDataStorage().GetStaleTickers(ctx, exchange, maxAge)
	if err != nil {
		return fmt.Errorf("failed to get stale tickers: %w", err)
	}

	if len(staleTickers) == 0 {
		s.logger.Info().Str("exchange", exchange).Msg("No stale data to refresh")
		return nil
	}

	s.logger.Info().Str("exchange", exchange).Int("count", len(staleTickers)).Msg("Refreshing stale data")

	return s.CollectMarketData(ctx, staleTickers, false, false)
}

// ScanMarket executes a flexible market scan query
func (s *Service) ScanMarket(ctx context.Context, query models.ScanQuery) (*models.ScanResponse, error) {
	scanner := NewScanner(s.storage, s.logger)
	return scanner.Scan(ctx, query)
}

// ScanFields returns the available scan field definitions
func (s *Service) ScanFields() *models.ScanFieldsResponse {
	scanner := NewScanner(s.storage, s.logger)
	return scanner.Fields()
}

// extractExchange extracts exchange from ticker (e.g., "BHP.AU" -> "AU")
func extractExchange(ticker string) string {
	for i := len(ticker) - 1; i >= 0; i-- {
		if ticker[i] == '.' {
			return ticker[i+1:]
		}
	}
	return ""
}

// Ensure Service implements MarketService
var _ interfaces.MarketService = (*Service)(nil)
