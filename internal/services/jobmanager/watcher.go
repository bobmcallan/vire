package jobmanager

import (
	"context"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// watchLoop periodically scans the stock index for stale data and enqueues jobs.
func (jm *JobManager) watchLoop(ctx context.Context) {
	const backoffMax = 30 * time.Second

	// Stagger startup to let the server stabilize before enqueuing jobs
	startupDelay := jm.config.GetWatcherStartupDelay()
	if startupDelay > 0 {
		jm.logger.Info().Dur("delay", startupDelay).Msg("Watcher: startup delay before first scan")
		select {
		case <-ctx.Done():
			return
		case <-time.After(startupDelay):
		}
	}

	interval := jm.config.GetWatcherInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	backoff := time.Duration(0)

	scan := func() {
		if ok := jm.scanStockIndex(ctx); ok {
			backoff = 0
		} else {
			// Exponential backoff on DB errors — sleep before next attempt
			if backoff == 0 {
				backoff = 2 * time.Second
			} else {
				backoff *= 2
				if backoff > backoffMax {
					backoff = backoffMax
				}
			}
			jm.logger.Warn().Dur("backoff", backoff).Msg("Watcher: DB error, backing off before next scan")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}

	// Run initial scan after startup delay
	scan()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			scan()
		}
	}
}

// scanStockIndex reads all entries from the stock index and enqueues jobs for stale components.
// Returns true on success, false on DB error (used by watchLoop for backoff).
func (jm *JobManager) scanStockIndex(ctx context.Context) bool {
	entries, err := jm.storage.StockIndexStore().List(ctx)
	if err != nil {
		jm.logger.Warn().Err(err).Msg("Watcher: failed to list stock index")
		return false
	}

	if len(entries) == 0 {
		jm.logger.Debug().Msg("Watcher: stock index is empty")
		return true
	}

	enqueued := 0
	// Track exchanges with stale EOD tickers for bulk jobs
	staleEODExchanges := make(map[string]bool)

	for _, entry := range entries {
		n, hasStaleEOD := jm.enqueueStaleJobs(ctx, entry)
		enqueued += n
		if hasStaleEOD {
			if ex := eohdExchangeFromTicker(entry.Ticker); ex != "" {
				staleEODExchanges[ex] = true
			}
		}
	}

	// Enqueue one bulk EOD job per exchange that has stale tickers
	for exchange := range staleEODExchanges {
		if err := jm.EnqueueIfNeeded(ctx, models.JobTypeCollectEODBulk, exchange, models.PriorityCollectEODBulk); err != nil {
			jm.logger.Warn().
				Str("exchange", exchange).
				Err(err).
				Msg("Watcher: failed to enqueue bulk EOD job")
		} else {
			enqueued++
		}
	}

	if enqueued > 0 {
		jm.logger.Info().Int("enqueued", enqueued).Int("tickers", len(entries)).Msg("Watcher: scan complete")
	} else {
		jm.logger.Debug().Int("tickers", len(entries)).Msg("Watcher: scan complete, no stale data")
	}

	// Purge old completed jobs
	jm.purgeOldJobs(ctx)
	return true
}

// enqueueStaleJobs checks each data component's freshness and enqueues jobs for stale ones.
// EOD is excluded from per-ticker checks — it is handled via bulk EOD jobs per exchange.
// Returns the number of jobs enqueued and whether this ticker has stale EOD data.
func (jm *JobManager) enqueueStaleJobs(ctx context.Context, entry *models.StockIndexEntry) (int, bool) {
	enqueued := 0

	// Determine if this is a new stock (added recently, no collection timestamps)
	isNew := time.Since(entry.AddedAt) < 5*time.Minute && entry.EODCollectedAt.IsZero()

	// Check if EOD is stale (reported to caller for bulk job grouping)
	hasStaleEOD := !common.IsFresh(entry.EODCollectedAt, common.FreshnessTodayBar)

	// Job types and their corresponding freshness checks (EOD excluded — handled via bulk)
	type check struct {
		jobType   string
		timestamp time.Time
		ttl       time.Duration
		priority  int
	}

	checks := []check{
		{models.JobTypeCollectFundamentals, entry.FundamentalsCollectedAt, common.FreshnessFundamentals, models.PriorityCollectFundamentals},
		{models.JobTypeCollectFilings, entry.FilingsCollectedAt, common.FreshnessFilings, models.PriorityCollectFilings},           // Index collection (fast, but still needs periodic refresh)
		{models.JobTypeCollectFilingPdfs, entry.FilingsPdfsCollectedAt, common.FreshnessFilings, models.PriorityCollectFilingPdfs}, // PDF downloads (slow)
		{models.JobTypeCollectNews, entry.NewsCollectedAt, common.FreshnessNews, models.PriorityCollectNews},
		{models.JobTypeCollectFilingSummaries, entry.FilingSummariesCollectedAt, common.FreshnessFilings, models.PriorityCollectFilingSummaries},
		{models.JobTypeCollectTimeline, entry.TimelineCollectedAt, common.FreshnessTimeline, models.PriorityCollectTimeline},
		{models.JobTypeCollectNewsIntel, entry.NewsIntelCollectedAt, common.FreshnessNews, models.PriorityCollectNewsIntel},
		{models.JobTypeComputeSignals, entry.SignalsCollectedAt, common.FreshnessSignals, models.PriorityComputeSignals},
	}

	for _, c := range checks {
		// Skip compute_signals if EOD has never been collected — signals require EOD data
		if c.jobType == models.JobTypeComputeSignals && entry.EODCollectedAt.IsZero() {
			continue
		}
		if !common.IsFresh(c.timestamp, c.ttl) {
			priority := c.priority
			if isNew {
				priority = models.PriorityNewStock
			}
			if err := jm.EnqueueIfNeeded(ctx, c.jobType, entry.Ticker, priority); err != nil {
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

	return enqueued, hasStaleEOD
}

// EnqueueTickerJobs enqueues background jobs for stale data components
// across the given tickers. Respects freshness TTLs — only stale
// components are enqueued. Intended for demand-driven collection
// triggered by portfolio requests.
func (jm *JobManager) EnqueueTickerJobs(ctx context.Context, tickers []string) int {
	enqueued := 0
	staleEODExchanges := make(map[string]bool)

	for _, ticker := range tickers {
		entry, err := jm.storage.StockIndexStore().Get(ctx, ticker)
		if err != nil {
			continue // ticker not in stock index yet
		}
		n, hasStaleEOD := jm.enqueueStaleJobs(ctx, entry)
		enqueued += n
		if hasStaleEOD {
			if ex := eohdExchangeFromTicker(entry.Ticker); ex != "" {
				staleEODExchanges[ex] = true
			}
		}
	}

	// Enqueue bulk EOD per exchange (same as watcher)
	for exchange := range staleEODExchanges {
		if err := jm.EnqueueIfNeeded(ctx, models.JobTypeCollectEODBulk, exchange, models.PriorityCollectEODBulk); err == nil {
			enqueued++
		}
	}

	if enqueued > 0 {
		jm.logger.Info().Int("enqueued", enqueued).Int("tickers", len(tickers)).Msg("Demand-driven: enqueued stale jobs for portfolio tickers")
	}
	return enqueued
}

// EnqueueSlowDataJobs enqueues background jobs for slow data components
// (filings PDFs, AI summaries, timeline, news intel) for a single ticker.
// Bypasses freshness checks — always enqueues if no pending job exists.
// Intended for force-refresh of individual stock data.
// Note: Filing index is collected in the fast path (CollectCoreMarketData).
func (jm *JobManager) EnqueueSlowDataJobs(ctx context.Context, ticker string) int {
	if ticker == "" {
		return 0
	}
	enqueued := 0
	slowJobs := []struct {
		jobType  string
		priority int
	}{
		{models.JobTypeCollectFilingPdfs, models.PriorityCollectFilingPdfs}, // Changed from CollectFilings
		{models.JobTypeCollectFilingSummaries, models.PriorityCollectFilingSummaries},
		{models.JobTypeCollectTimeline, models.PriorityCollectTimeline},
		{models.JobTypeCollectNews, models.PriorityCollectNews},
		{models.JobTypeCollectNewsIntel, models.PriorityCollectNewsIntel},
		{models.JobTypeComputeSignals, models.PriorityComputeSignals},
	}
	for _, j := range slowJobs {
		if err := jm.EnqueueIfNeeded(ctx, j.jobType, ticker, j.priority); err == nil {
			enqueued++
		}
	}
	if enqueued > 0 {
		jm.logger.Info().Str("ticker", ticker).Int("enqueued", enqueued).Msg("Force refresh: enqueued slow data jobs")
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

// eohdExchangeFromTicker extracts the EODHD exchange code from a ticker string.
// "BHP.AU" -> "AU", "AAPL.US" -> "US". Returns "" if no dot separator found.
func eohdExchangeFromTicker(ticker string) string {
	for i := len(ticker) - 1; i >= 0; i-- {
		if ticker[i] == '.' {
			return ticker[i+1:]
		}
	}
	return ""
}
