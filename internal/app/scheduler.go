package app

import (
	"context"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// startPriceScheduler refreshes EOD prices on a fixed interval.
// It reads the portfolio from storage (no Navexa re-sync) and updates market data for active tickers.
func startPriceScheduler(ctx context.Context, portfolioService interfaces.PortfolioService, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("Price scheduler: stopped")
			return
		case <-ticker.C:
			refreshPrices(ctx, portfolioService, marketService, storage, logger)
		}
	}
}

// startLivePriceScheduler refreshes live OHLCV prices on a 15-minute interval.
// Collects for all exchanges that have tickers in the stock index.
func startLivePriceScheduler(ctx context.Context, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger) {
	ticker := time.NewTicker(common.FreshnessLivePrice)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("Live price scheduler: stopped")
			return
		case <-ticker.C:
			refreshLivePrices(ctx, marketService, storage, logger)
		}
	}
}

func refreshLivePrices(ctx context.Context, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger) {
	start := time.Now()

	entries, err := storage.StockIndexStore().List(ctx)
	if err != nil || len(entries) == 0 {
		return
	}

	exchanges := make(map[string]bool)
	for _, e := range entries {
		if e.Exchange != "" {
			exchanges[e.Exchange] = true
		}
	}

	for exchange := range exchanges {
		if err := marketService.CollectLivePrices(ctx, exchange); err != nil {
			logger.Warn().Str("exchange", exchange).Err(err).Msg("Live price refresh failed")
		}
	}

	logger.Info().
		Int("exchanges", len(exchanges)).
		Dur("elapsed", time.Since(start)).
		Msg("Live price refresh: complete")
}

func refreshPrices(ctx context.Context, portfolioService interfaces.PortfolioService, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger) {
	start := time.Now()

	portfolioName := resolvePortfolioWithFallback(ctx, portfolioService, storage, logger)
	if portfolioName == "" {
		return
	}

	// Read from storage — don't re-sync from Navexa on every tick
	portfolio, err := portfolioService.GetPortfolio(ctx, portfolioName)
	if err != nil {
		logger.Warn().Err(err).Str("portfolio", portfolioName).Msg("Price refresh: portfolio not found in storage")
		return
	}

	tickers := make([]string, 0, len(portfolio.Holdings))
	for _, h := range portfolio.Holdings {
		if len(h.Trades) > 0 {
			tickers = append(tickers, h.EODHDTicker())
		}
	}

	if len(tickers) == 0 {
		return
	}

	if err := marketService.CollectMarketData(ctx, tickers, false, false); err != nil {
		logger.Warn().Err(err).Msg("Price refresh: market data collection failed")
		return
	}

	logger.Info().
		Str("portfolio", portfolioName).
		Int("tickers", len(tickers)).
		Dur("elapsed", time.Since(start)).
		Msg("Price refresh: complete")
}
