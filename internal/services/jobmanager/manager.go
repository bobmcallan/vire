// Package jobmanager provides a background job manager with a priority queue
// for market data collection driven by the stock index.
package jobmanager

import (
	"context"
	"fmt"
	"runtime/debug"
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

	heavySem chan struct{} // semaphore limiting concurrent PDF-heavy jobs
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewJobManager creates a new job manager.
func NewJobManager(
	market interfaces.MarketService,
	signal interfaces.SignalService,
	storage interfaces.StorageManager,
	logger *common.Logger,
	config common.JobManagerConfig,
) *JobManager {
	heavyLimit := config.GetHeavyJobLimit()
	return &JobManager{
		market:   market,
		signal:   signal,
		storage:  storage,
		logger:   logger,
		hub:      NewJobWSHub(logger),
		config:   config,
		heavySem: make(chan struct{}, heavyLimit),
	}
}

// safeGo launches a goroutine with panic recovery and logging.
func (jm *JobManager) safeGo(name string, fn func()) {
	jm.wg.Add(1)
	go func() {
		defer jm.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				jm.logger.Error().
					Str("goroutine", name).
					Str("panic", fmt.Sprintf("%v", r)).
					Str("stack", string(debug.Stack())).
					Msg("Recovered from panic in job manager goroutine")
			}
		}()
		fn()
	}()
}

// Start launches the watcher loop, processor pool, and WebSocket hub.
// Safe to call multiple times — stops any existing loops before starting.
func (jm *JobManager) Start() {
	if jm.cancel != nil {
		jm.Stop()
	}

	ctx, cancel := context.WithCancel(context.Background())
	jm.cancel = cancel

	// Reset orphaned jobs from previous crash
	if count, err := jm.storage.JobQueueStore().ResetRunningJobs(ctx); err != nil {
		jm.logger.Warn().Err(err).Msg("Failed to reset orphaned running jobs")
	} else if count > 0 {
		jm.logger.Info().Int("count", count).Msg("Reset orphaned running jobs to pending")
	}

	// Start WebSocket hub
	jm.safeGo("websocket-hub", func() { jm.hub.Run() })

	// Start watcher loop
	jm.safeGo("watcher", func() { jm.watchLoop(ctx) })

	// Start processor pool
	maxConc := jm.config.MaxConcurrent
	if maxConc <= 0 {
		maxConc = 5
	}
	for i := 0; i < maxConc; i++ {
		name := fmt.Sprintf("processor-%d", i)
		jm.safeGo(name, func() { jm.processLoop(ctx) })
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
	jm.hub.Stop()
	jm.wg.Wait()
	jm.logger.Info().Msg("Job manager stopped")
}

// Hub returns the WebSocket hub for external handler registration.
func (jm *JobManager) Hub() *JobWSHub {
	return jm.hub
}

// isHeavyJob returns true for job types that involve large PDF downloads or parsing.
// These jobs are rate-limited by the heavy job semaphore to prevent OOM.
func isHeavyJob(jobType string) bool {
	return jobType == models.JobTypeCollectFilings || jobType == models.JobTypeCollectFilingSummaries
}

// processLoop continuously dequeues and executes jobs.
func (jm *JobManager) processLoop(ctx context.Context) {

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

			// Acquire heavy job semaphore for PDF-intensive jobs
			heavy := isHeavyJob(job.JobType)
			if heavy {
				select {
				case jm.heavySem <- struct{}{}:
					// acquired
				case <-ctx.Done():
					return
				}
			}

			start := time.Now()
			execErr := func() (jobErr error) {
				if heavy {
					defer func() { <-jm.heavySem }()
				}
				return jm.executeJob(ctx, job)
			}()
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
