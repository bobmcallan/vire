// Package signal provides signal detection services
package signal

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/signals"
)

// Service implements SignalService
type Service struct {
	storage  interfaces.StorageManager
	eodhd    interfaces.EODHDClient
	computer *signals.Computer
	logger   *common.Logger
}

// NewService creates a new signal service.
// eodhd may be nil; when non-nil, live quotes are overlaid onto cached EOD bars
// before computing signals so that indicators reflect current market prices.
func NewService(storage interfaces.StorageManager, eodhd interfaces.EODHDClient, logger *common.Logger) *Service {
	return &Service{
		storage:  storage,
		eodhd:    eodhd,
		computer: signals.NewComputer(),
		logger:   logger,
	}
}

// DetectSignals computes signals for tickers.
// When force is true, signals are recomputed regardless of freshness.
func (s *Service) DetectSignals(ctx context.Context, tickers []string, signalTypes []string, force bool) ([]*models.TickerSignals, error) {
	results := make([]*models.TickerSignals, 0, len(tickers))

	for _, ticker := range tickers {
		// Get market data
		marketData, err := s.storage.MarketDataStorage().GetMarketData(ctx, ticker)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to get market data for signal detection")
			results = append(results, &models.TickerSignals{
				Ticker:           ticker,
				ComputeTimestamp: time.Now(),
				Error:            fmt.Sprintf("market data unavailable: %v", err),
			})
			continue
		}

		// Check if existing signals are still fresh (computed after EOD data was updated)
		if !force {
			existing, err := s.storage.SignalStorage().GetSignals(ctx, ticker)
			if err == nil && existing != nil &&
				!existing.ComputeTimestamp.IsZero() &&
				existing.ComputeTimestamp.After(marketData.EODUpdatedAt) &&
				common.IsFresh(existing.ComputeTimestamp, common.FreshnessSignals) {
				s.logger.Debug().Str("ticker", ticker).Msg("Signals still fresh, skipping recompute")
				if len(signalTypes) > 0 {
					existing = filterSignals(existing, signalTypes)
				}
				results = append(results, existing)
				continue
			}
		}

		// Overlay live quote onto cached bars before computing signals
		s.overlayLiveQuote(ctx, ticker, marketData)

		// Compute signals
		tickerSignals, err := s.ComputeSignals(ctx, ticker, marketData)
		if err != nil {
			s.logger.Warn().Str("ticker", ticker).Err(err).Msg("Failed to compute signals")
			results = append(results, &models.TickerSignals{
				Ticker:           ticker,
				ComputeTimestamp: time.Now(),
				Error:            fmt.Sprintf("signal computation failed: %v", err),
			})
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

// overlayLiveQuote fetches a real-time quote and updates the cached EOD bars
// so that indicators (SMA, RSI, MACD) use current market prices instead of
// potentially stale end-of-day data. This is non-fatal: if the quote fetch
// fails or the EODHD client is nil, we proceed with cached data.
func (s *Service) overlayLiveQuote(ctx context.Context, ticker string, md *models.MarketData) {
	if s.eodhd == nil || md == nil || len(md.EOD) == 0 {
		return
	}

	quote, err := s.eodhd.GetRealTimeQuote(ctx, ticker)
	if err != nil {
		s.logger.Debug().Str("ticker", ticker).Err(err).Msg("Live quote fetch failed, using cached EOD data")
		return
	}
	if quote == nil || quote.Close <= 0 || math.IsNaN(quote.Close) || math.IsInf(quote.Close, 0) {
		return
	}

	today := time.Now().Truncate(24 * time.Hour)
	latestBarDate := md.EOD[0].Date.Truncate(24 * time.Hour)

	if latestBarDate.Equal(today) {
		// Same day: update the latest bar with the live quote
		md.EOD[0].Close = quote.Close
		if quote.High > md.EOD[0].High {
			md.EOD[0].High = quote.High
		}
		if quote.Low > 0 && quote.Low < md.EOD[0].Low {
			md.EOD[0].Low = quote.Low
		}
		s.logger.Debug().Str("ticker", ticker).Float64("price", quote.Close).Msg("Overlaid live quote on today's bar")
	} else if latestBarDate.Before(today) {
		// Previous day: prepend a synthetic bar for today
		syntheticBar := models.EODBar{
			Date:     today,
			Open:     quote.Open,
			High:     quote.High,
			Low:      quote.Low,
			Close:    quote.Close,
			AdjClose: quote.Close,
			Volume:   quote.Volume,
		}
		// Use previous close as open if quote open is zero
		if syntheticBar.Open <= 0 {
			syntheticBar.Open = quote.PreviousClose
		}
		md.EOD = append([]models.EODBar{syntheticBar}, md.EOD...)
		s.logger.Debug().Str("ticker", ticker).Float64("price", quote.Close).Msg("Prepended synthetic bar with live quote")
	}
}

// filterSignals filters signals based on requested types
func filterSignals(signals *models.TickerSignals, types []string) *models.TickerSignals {
	// For now, return all signals - filtering would remove specific signal components
	// This could be enhanced to zero out unrequested signal types
	return signals
}

// Ensure Service implements SignalService
var _ interfaces.SignalService = (*Service)(nil)
