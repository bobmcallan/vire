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

// CollectMarketData fetches and stores market data for tickers
func (s *Service) CollectMarketData(ctx context.Context, tickers []string, includeNews bool) error {
	for _, ticker := range tickers {
		s.logger.Debug().Str("ticker", ticker).Msg("Collecting market data")

		// Fetch EOD data
		eodResp, err := s.eodhd.GetEOD(ctx, ticker, interfaces.WithLimit(365))
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch EOD data")
			continue
		}

		// Fetch fundamentals
		fundamentals, err := s.eodhd.GetFundamentals(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch fundamentals")
			// Continue without fundamentals
		}

		// Build market data
		marketData := &models.MarketData{
			Ticker:       ticker,
			Exchange:     extractExchange(ticker),
			EOD:          eodResp.Data,
			Fundamentals: fundamentals,
			LastUpdated:  time.Now(),
		}

		// Fetch news if requested
		if includeNews {
			news, err := s.eodhd.GetNews(ctx, ticker, 10)
			if err != nil {
				s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to fetch news")
			} else {
				marketData.News = news
			}
		}

		// Save market data
		if err := s.storage.MarketDataStorage().SaveMarketData(ctx, marketData); err != nil {
			s.logger.Error().Str("ticker", ticker).Err(err).Msg("Failed to save market data")
			continue
		}

		// Compute and save signals
		tickerSignals := s.signalComputer.Compute(marketData)
		if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals")
		}
	}

	return nil
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
		if err := s.CollectMarketData(ctx, []string{ticker}, include.News); err != nil {
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
	}

	return stockData, nil
}

// FindSnipeBuys identifies turnaround stocks
func (s *Service) FindSnipeBuys(ctx context.Context, options interfaces.SnipeOptions) ([]*models.SnipeBuy, error) {
	sniper := NewSniper(s.storage, s.eodhd, s.gemini, s.signalComputer, s.logger)
	return sniper.FindSnipeBuys(ctx, options)
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

	return s.CollectMarketData(ctx, staleTickers, false)
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
