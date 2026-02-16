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

func refreshPrices(ctx context.Context, portfolioService interfaces.PortfolioService, marketService interfaces.MarketService, storage interfaces.StorageManager, logger *common.Logger) {
	start := time.Now()

	portfolioName := common.ResolveDefaultPortfolio(ctx, storage.InternalStore())
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
