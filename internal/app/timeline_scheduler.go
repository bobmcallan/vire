package app

import (
	"context"
	"sync"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
)

// startTimelineScheduler runs two independent loops:
//   - Incremental (incrementalInterval): calls SyncPortfolio which writes today's snapshot
//   - Rebuild (rebuildInterval): calls GetDailyGrowth which recomputes full history
//
// An initial rebuild runs on startup to populate historical timeline data without
// waiting for the first tick (rebuildInterval may be hours). A sync.Mutex prevents
// concurrent timeline operations; ticks are skipped if the lock is held.
func startTimelineScheduler(ctx context.Context, portfolioSvc interfaces.PortfolioService, storage interfaces.StorageManager, logger *common.Logger, incrementalInterval, rebuildInterval time.Duration) {
	// Run an immediate rebuild on startup so historical timeline data is
	// available right away instead of waiting for the first rebuild tick.
	logger.Info().Msg("Timeline scheduler: running initial rebuild on startup")
	rebuildTimeline(ctx, portfolioSvc, storage, logger)

	incrementalTicker := time.NewTicker(incrementalInterval)
	rebuildTicker := time.NewTicker(rebuildInterval)
	defer incrementalTicker.Stop()
	defer rebuildTicker.Stop()

	var mu sync.Mutex

	for {
		select {
		case <-ctx.Done():
			logger.Info().Msg("Timeline scheduler: stopped")
			return
		case <-rebuildTicker.C:
			if mu.TryLock() {
				rebuildTimeline(ctx, portfolioSvc, storage, logger)
				mu.Unlock()
			} else {
				logger.Debug().Msg("Timeline scheduler: rebuild skipped (already running)")
			}
		case <-incrementalTicker.C:
			if mu.TryLock() {
				incrementalTimeline(ctx, portfolioSvc, storage, logger)
				mu.Unlock()
			} else {
				logger.Debug().Msg("Timeline scheduler: incremental skipped (already running)")
			}
		}
	}
}

func rebuildTimeline(ctx context.Context, portfolioSvc interfaces.PortfolioService, storage interfaces.StorageManager, logger *common.Logger) {
	portfolioName := common.ResolveDefaultPortfolio(ctx, storage.InternalStore())
	if portfolioName == "" {
		return
	}

	start := time.Now()
	if _, err := portfolioSvc.GetDailyGrowth(ctx, portfolioName, interfaces.GrowthOptions{}); err != nil {
		logger.Warn().Err(err).Str("portfolio", portfolioName).Msg("Timeline rebuild failed")
		return
	}
	logger.Info().Str("portfolio", portfolioName).Dur("elapsed", time.Since(start)).Msg("Timeline rebuild complete")
}

func incrementalTimeline(ctx context.Context, portfolioSvc interfaces.PortfolioService, storage interfaces.StorageManager, logger *common.Logger) {
	portfolioName := common.ResolveDefaultPortfolio(ctx, storage.InternalStore())
	if portfolioName == "" {
		return
	}

	start := time.Now()
	if err := portfolioSvc.RefreshTodaySnapshot(ctx, portfolioName); err != nil {
		logger.Warn().Err(err).Str("portfolio", portfolioName).Msg("Timeline incremental update failed")
		return
	}
	logger.Info().Str("portfolio", portfolioName).Dur("elapsed", time.Since(start)).Msg("Timeline incremental update complete")
}
