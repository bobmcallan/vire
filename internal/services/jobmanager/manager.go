// Package jobmanager provides a background job manager with a priority queue
// for market data collection driven by the stock index.
package jobmanager

import (
	"context"
	"sync"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// JobManager runs the watcher and processor loops for queue-driven data collection.
// The watcher scans the stock index for stale data and enqueues jobs.
// Processor goroutines dequeue and execute jobs concurrently.
type JobManager struct {
	market  interfaces.MarketService
	signal  interfaces.SignalService
	storage interfaces.StorageManager
	logger  *common.Logger
	hub     *JobWSHub
	config  common.JobManagerConfig

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewJobManager creates a new job manager.
func NewJobManager(
	market interfaces.MarketService,
	signal interfaces.SignalService,
	storage interfaces.StorageManager,
	logger *common.Logger,
	config common.JobManagerConfig,
) *JobManager {
	return &JobManager{
		market:  market,
		signal:  signal,
		storage: storage,
		logger:  logger,
		hub:     NewJobWSHub(logger),
		config:  config,
	}
}

// Start launches the watcher loop, processor pool, and WebSocket hub.
// Safe to call multiple times — stops any existing loops before starting.
func (jm *JobManager) Start() {
	if jm.cancel != nil {
		jm.Stop()
	}

	ctx, cancel := context.WithCancel(context.Background())
	jm.cancel = cancel

	// Start WebSocket hub
	go jm.hub.Run()

	// Start watcher loop
	jm.wg.Add(1)
	go jm.watchLoop(ctx)

	// Start processor pool
	maxConc := jm.config.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 5
	}
	for i := 0; i < maxConc; i++ {
		jm.wg.Add(1)
		go jm.processLoop(ctx)
	}

	jm.logger.Info().
		Str("watcher_interval", jm.config.WatcherInterval).
		Int("max_concurrent", maxConc).
		Msg("Job manager started (queue mode)")
}

// Stop cancels all loops and waits for completion.
func (jm *JobManager) Stop() {
	if jm.cancel != nil {
		jm.cancel()
		jm.cancel = nil
	}
	jm.wg.Wait()
	jm.logger.Info().Msg("Job manager stopped")
}

// Hub returns the WebSocket hub for external handler registration.
func (jm *JobManager) Hub() *JobWSHub {
	return jm.hub
}

// processLoop continuously dequeues and executes jobs.
func (jm *JobManager) processLoop(ctx context.Context) {
	defer jm.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
			job, err := jm.dequeue(ctx)
			if err != nil {
				jm.logger.Warn().Err(err).Msg("Processor: dequeue error")
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
					continue
				}
			}
			if job == nil {
				// Queue empty, sleep briefly
				select {
				case <-ctx.Done():
					return
				case <-time.After(1 * time.Second):
					continue
				}
			}

			start := time.Now()
			execErr := jm.executeJob(ctx, job)
			durationMS := time.Since(start).Milliseconds()

			if execErr != nil {
				jm.logger.Warn().
					Str("job_id", job.ID).
					Str("job_type", job.JobType).
					Str("ticker", job.Ticker).
					Int64("duration_ms", durationMS).
					Err(execErr).
					Msg("Job failed")

				// Re-queue if under max attempts
				if job.Attempts < job.MaxAttempts {
					jm.logger.Info().
						Str("job_id", job.ID).
						Int("attempt", job.Attempts).
						Int("max", job.MaxAttempts).
						Msg("Re-queuing failed job")

					job.Status = models.JobStatusPending
					job.Error = ""
					if err := jm.storage.JobQueueStore().Enqueue(ctx, job); err != nil {
						jm.logger.Warn().Str("job_id", job.ID).Err(err).Msg("Failed to re-enqueue job")
					} else {
						continue // Skip complete() — job is re-queued
					}
				}
			} else {
				jm.logger.Debug().
					Str("job_id", job.ID).
					Str("job_type", job.JobType).
					Str("ticker", job.Ticker).
					Int64("duration_ms", durationMS).
					Msg("Job completed")
			}

			jm.complete(ctx, job, execErr, durationMS)
		}
	}
}
