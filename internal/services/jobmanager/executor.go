package jobmanager

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// executeJob dispatches a job to the correct service method based on job type.
func (jm *JobManager) executeJob(ctx context.Context, job *models.Job) error {
	switch job.JobType {
	case models.JobTypeCollectEOD:
		return jm.market.CollectEOD(ctx, job.Ticker, false)
	case models.JobTypeCollectEODBulk:
		return jm.market.CollectBulkEOD(ctx, job.Ticker, false) // Ticker = exchange code (e.g. "AU")
	case models.JobTypeCollectFundamentals:
		return jm.market.CollectFundamentals(ctx, job.Ticker, false)
	case models.JobTypeCollectFilings:
		return jm.market.CollectFilingsIndex(ctx, job.Ticker, false) // Fast: index only
	case models.JobTypeCollectFilingPdfs:
		return jm.market.CollectFilingPdfs(ctx, job.Ticker, false) // Slow: PDF downloads
	case models.JobTypeCollectNews:
		return jm.market.CollectNews(ctx, job.Ticker, false)
	case models.JobTypeCollectFilingSummaries:
		return jm.market.CollectFilingSummaries(ctx, job.Ticker, false)
	case models.JobTypeCollectTimeline:
		return jm.market.CollectTimeline(ctx, job.Ticker, false)
	case models.JobTypeCollectNewsIntel:
		return jm.market.CollectNewsIntelligence(ctx, job.Ticker, false)
	case models.JobTypeComputeSignals:
		return jm.computeSignals(ctx, job.Ticker)
	default:
		return fmt.Errorf("unknown job type: %s", job.JobType)
	}
}

// computeSignals computes and saves signals for a ticker.
func (jm *JobManager) computeSignals(ctx context.Context, ticker string) error {
	md, err := jm.storage.MarketDataStorage().GetMarketData(ctx, ticker)
	if err != nil {
		return fmt.Errorf("failed to get market data for signals: %w", err)
	}
	if md == nil || len(md.EOD) == 0 {
		return nil
	}

	sigs, err := jm.signal.ComputeSignals(ctx, ticker, md)
	if err != nil {
		return fmt.Errorf("failed to compute signals: %w", err)
	}

	if err := jm.storage.SignalStorage().SaveSignals(ctx, sigs); err != nil {
		return fmt.Errorf("failed to save signals: %w", err)
	}

	return nil
}

// updateStockIndexTimestamp updates the corresponding freshness timestamp on the stock index.
func (jm *JobManager) updateStockIndexTimestamp(ctx context.Context, job *models.Job) {
	field := models.TimestampFieldForJobType(job.JobType)
	if field == "" || job.Ticker == "" {
		return
	}

	if err := jm.storage.StockIndexStore().UpdateTimestamp(ctx, job.Ticker, field, time.Now()); err != nil {
		jm.logger.Warn().
			Str("ticker", job.Ticker).
			Str("field", field).
			Err(err).
			Msg("Failed to update stock index timestamp")
	}
}
