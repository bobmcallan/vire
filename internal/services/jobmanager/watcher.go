package jobmanager

import (
	"context"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// watchLoop periodically scans the stock index for stale data and enqueues jobs.
func (jm *JobManager) watchLoop(ctx context.Context) {
	defer jm.wg.Done()

	interval := jm.config.GetWatcherInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run an initial scan immediately
	jm.scanStockIndex(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jm.scanStockIndex(ctx)
		}
	}
}

// scanStockIndex reads all entries from the stock index and enqueues jobs for stale components.
func (jm *JobManager) scanStockIndex(ctx context.Context) {
	entries, err := jm.storage.StockIndexStore().List(ctx)
	if err != nil {
		jm.logger.Warn().Err(err).Msg("Watcher: failed to list stock index")
		return
	}

	if len(entries) == 0 {
		jm.logger.Debug().Msg("Watcher: stock index is empty")
		return
	}

	enqueued := 0
	for _, entry := range entries {
		n := jm.enqueueStaleJobs(ctx, entry)
		enqueued += n
	}

	if enqueued > 0 {
		jm.logger.Info().Int("enqueued", enqueued).Int("tickers", len(entries)).Msg("Watcher: scan complete")
	} else {
		jm.logger.Debug().Int("tickers", len(entries)).Msg("Watcher: scan complete, no stale data")
	}

	// Purge old completed jobs
	jm.purgeOldJobs(ctx)
}

// enqueueStaleJobs checks each data component's freshness and enqueues jobs for stale ones.
func (jm *JobManager) enqueueStaleJobs(ctx context.Context, entry *models.StockIndexEntry) int {
	enqueued := 0

	// Determine if this is a new stock (added recently, no collection timestamps)
	isNew := time.Since(entry.AddedAt) < 5*time.Minute && entry.EODCollectedAt.IsZero()

	// Job types and their corresponding freshness checks
	type check struct {
		jobType   string
		timestamp time.Time
		ttl       time.Duration
		priority  int
	}

	checks := []check{
		{models.JobTypeCollectEOD, entry.EODCollectedAt, common.FreshnessTodayBar, models.PriorityCollectEOD},
		{models.JobTypeCollectFundamentals, entry.FundamentalsCollectedAt, common.FreshnessFundamentals, models.PriorityCollectFundamentals},
		{models.JobTypeCollectFilings, entry.FilingsCollectedAt, common.FreshnessFilings, models.PriorityCollectFilings},
		{models.JobTypeCollectNews, entry.NewsCollectedAt, common.FreshnessNews, models.PriorityCollectNews},
		{models.JobTypeCollectFilingSummaries, entry.FilingSummariesCollectedAt, common.FreshnessFilings, models.PriorityCollectFilingSummaries},
		{models.JobTypeCollectTimeline, entry.TimelineCollectedAt, common.FreshnessTimeline, models.PriorityCollectTimeline},
		{models.JobTypeCollectNewsIntel, entry.NewsIntelCollectedAt, common.FreshnessNews, models.PriorityCollectNewsIntel},
		{models.JobTypeComputeSignals, entry.SignalsCollectedAt, common.FreshnessSignals, models.PriorityComputeSignals},
	}

	for _, c := range checks {
		if !common.IsFresh(c.timestamp, c.ttl) {
			priority := c.priority
			if isNew {
				priority = models.PriorityNewStock
			}
			if err := jm.enqueueIfNeeded(ctx, c.jobType, entry.Ticker, priority); err != nil {
				jm.logger.Warn().
					Str("ticker", entry.Ticker).
					Str("job_type", c.jobType).
					Err(err).
					Msg("Watcher: failed to enqueue job")
			} else {
				enqueued++
			}
		}
	}

	return enqueued
}

// purgeOldJobs removes completed/failed jobs older than the configured purge duration.
func (jm *JobManager) purgeOldJobs(ctx context.Context) {
	purgeAfter := jm.config.GetPurgeAfter()
	cutoff := time.Now().Add(-purgeAfter)
	if _, err := jm.storage.JobQueueStore().PurgeCompleted(ctx, cutoff); err != nil {
		jm.logger.Warn().Err(err).Msg("Watcher: failed to purge old jobs")
	}
}
