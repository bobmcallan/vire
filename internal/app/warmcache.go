package app

import (
	"context"
	"os"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// warmCache pre-fetches portfolio and market data on startup so the first user query is fast.
func warmCache(ctx context.Context, portfolioService interfaces.PortfolioService, marketService interfaces.MarketService, storage interfaces.StorageManager, configDefault string, logger *common.Logger) {
	// Check env var override
	if os.Getenv("VIRE_WARM_CACHE") == "off" {
		logger.Info().Msg("Warm cache: disabled via VIRE_WARM_CACHE=off")
		return
	}

	start := time.Now()

	// Resolve default portfolio name
	portfolioName := common.ResolveDefaultPortfolio(ctx, storage.InternalStore(), configDefault)
	if portfolioName == "" {
		logger.Info().Msg("Warm cache: no default portfolio configured, skipping")
		return
	}

	// Try to get cached portfolio to check freshness
	portfolio, err := portfolioService.GetPortfolio(ctx, portfolioName)
	if err == nil && common.IsFresh(portfolio.LastSynced, common.FreshnessPortfolio) {
		logger.Info().Str("portfolio", portfolioName).Msg("Warm cache: portfolio already fresh, skipping")
		return
	}

	logger.Info().Str("portfolio", portfolioName).Msg("Warm cache: starting")

	// Sync portfolio (incremental — won't re-fetch if recently synced)
	portfolio, err = portfolioService.SyncPortfolio(ctx, portfolioName, false)
	if err != nil {
		// Sync failed (expected when running without portal/Navexa) — fall back to cached data
		logger.Info().Str("portfolio", portfolioName).Msg("Warm cache: sync unavailable, using cached portfolio")
		portfolio, err = portfolioService.GetPortfolio(ctx, portfolioName)
		if err != nil {
			logger.Info().Str("portfolio", portfolioName).Msg("Warm cache: no cached portfolio data, skipping")
			return
		}
	}

	// Extract tickers with trade history (includes closed positions for historical growth data)
	tickers := make([]string, 0, len(portfolio.Holdings))
	for _, h := range portfolio.Holdings {
		if len(h.Trades) > 0 {
			tickers = append(tickers, h.EODHDTicker())
		}
	}

	if len(tickers) == 0 {
		logger.Info().Msg("Warm cache: no active holdings, skipping market data")
		return
	}

	// Collect market data (incremental — only fetches stale/missing data)
	if err := marketService.CollectMarketData(ctx, tickers, false, false); err != nil {
		logger.Warn().Err(err).Msg("Warm cache: market data collection failed")
		return
	}

	logger.Info().
		Str("portfolio", portfolioName).
		Int("tickers", len(tickers)).
		Dur("elapsed", time.Since(start)).
		Msg("Warm cache: complete")
}
