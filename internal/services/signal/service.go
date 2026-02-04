// Package signal provides signal detection services
package signal

import (
	"context"
	"fmt"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
	"github.com/bobmccarthy/vire/internal/signals"
)

// Service implements SignalService
type Service struct {
	storage  interfaces.StorageManager
	computer *signals.Computer
	logger   *common.Logger
}

// NewService creates a new signal service
func NewService(storage interfaces.StorageManager, logger *common.Logger) *Service {
	return &Service{
		storage:  storage,
		computer: signals.NewComputer(),
		logger:   logger,
	}
}

// DetectSignals computes signals for tickers
func (s *Service) DetectSignals(ctx context.Context, tickers []string, signalTypes []string) ([]*models.TickerSignals, error) {
	results := make([]*models.TickerSignals, 0, len(tickers))

	for _, ticker := range tickers {
		// Get market data
		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to get market data for signal detection")
			continue
		}

		// Compute signals
		tickerSignals, err := s.ComputeSignals(ctx, ticker, marketData)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to compute signals")
			continue
		}

		// Filter by signal types if specified
		if len(signalTypes) > 0 {
			tickerSignals = filterSignals(tickerSignals, signalTypes)
		}

		// Save signals
		if err := s.storage.SignalStorage().SaveSignals(ctx, tickerSignals); err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to save signals")
		}

		results = append(results, tickerSignals)
	}

	return results, nil
}

// ComputeSignals calculates all signals for a ticker
func (s *Service) ComputeSignals(ctx context.Context, ticker string, marketData *models.MarketData) (*models.TickerSignals, error) {
	if marketData == nil {
		return nil, fmt.Errorf("market data is nil for ticker %s", ticker)
	}

	tickerSignals := s.computer.Compute(marketData)
	return tickerSignals, nil
}

// filterSignals filters signals based on requested types
func filterSignals(signals *models.TickerSignals, types []string) *models.TickerSignals {
	// For now, return all signals - filtering would remove specific signal components
	// This could be enhanced to zero out unrequested signal types
	return signals
}

// Ensure Service implements SignalService
var _ interfaces.SignalService = (*Service)(nil)
