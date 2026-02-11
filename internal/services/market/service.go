// Package market provides market data services
package market

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/signals"
)

// Service implements MarketService
type Service struct {
	storage        interfaces.StorageManager
	eodhd          interfaces.EODHDClient
	gemini         interfaces.GeminiClient
	signalComputer *signals.Computer
	logger         *common.Logger
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
		if force || existing == nil || !common.IsFresh(existing.FilingsUpdatedAt, common.FreshnessFilings) {
			filings, err := s.collectFilings(ctx, ticker)
			if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to collect filings")
			} else {
				filings = s.downloadFilingPDFs(ctx, extractCode(ticker), filings)
				marketData.Filings = filings
				marketData.FilingsUpdatedAt = now
			}
		}

		// --- Filings Intelligence ---
		if s.gemini != nil && len(marketData.Filings) > 0 {
			if force || !common.IsFresh(marketData.FilingsIntelUpdatedAt, common.FreshnessFilingsIntel) {
				intel := s.generateFilingsIntelligence(ctx, ticker, marketData.Name, marketData.Filings)
				if intel != nil {
					marketData.FilingsIntelligence = intel
					marketData.FilingsIntelUpdatedAt = now
				}
			}
		}

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
		// Try to fetch fresh data
		if err := s.CollectMarketData(ctx, []string{ticker}, include.News, false); err != nil {
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
	if len(marketData.Filings) == 0 || !common.IsFresh(marketData.FilingsUpdatedAt, common.FreshnessFilings) {
		filings, err := s.collectFilings(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to auto-collect filings")
		} else if len(filings) > 0 {
			filings = s.downloadFilingPDFs(ctx, extractCode(ticker), filings)
			marketData.Filings = filings
			marketData.FilingsUpdatedAt = time.Now()
			_ = s.storage.MarketDataStorage().SaveMarketData(ctx, marketData)
		}
	}

	stockData.Filings = marketData.Filings
	stockData.FilingsIntelligence = marketData.FilingsIntelligence

	// Auto-generate filings intelligence if filings exist but no cached intel
	if marketData.FilingsIntelligence == nil && s.gemini != nil && len(marketData.Filings) > 0 {
		intel := s.generateFilingsIntelligence(ctx, ticker, marketData.Name, marketData.Filings)
		if intel != nil {
			marketData.FilingsIntelligence = intel
			marketData.FilingsIntelUpdatedAt = time.Now()
			stockData.FilingsIntelligence = intel
			_ = s.storage.MarketDataStorage().SaveMarketData(ctx, marketData)
		}
	}

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
