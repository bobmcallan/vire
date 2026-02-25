package jobmanager

import (
	"context"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

// enqueue adds a job to the queue and broadcasts a "job_queued" event.
func (jm *JobManager) enqueue(ctx context.Context, job *models.Job) error {
	if err := jm.storage.JobQueueStore().Enqueue(ctx, job); err != nil {
		return err
	}

	if jm.hub != nil {
		pending, _ := jm.storage.JobQueueStore().CountPending(ctx)
		jm.hub.Broadcast(models.JobEvent{
			Type:      "job_queued",
			Job:       job,
			Timestamp: time.Now(),
			QueueSize: pending,
		})
	}

	return nil
}

// dequeue gets the highest-priority pending job and broadcasts a "job_started" event.
func (jm *JobManager) dequeue(ctx context.Context) (*models.Job, error) {
	job, err := jm.storage.JobQueueStore().Dequeue(ctx)
	if err != nil || job == nil {
		return job, err
	}

	if jm.hub != nil {
		pending, _ := jm.storage.JobQueueStore().CountPending(ctx)
		jm.hub.Broadcast(models.JobEvent{
			Type:      "job_started",
			Job:       job,
			Timestamp: time.Now(),
			QueueSize: pending,
		})
	}

	return job, nil
}

// complete marks a job as completed/failed and broadcasts the corresponding event.
func (jm *JobManager) complete(ctx context.Context, job *models.Job, execErr error, durationMS int64) {
	if err := jm.storage.JobQueueStore().Complete(ctx, job.ID, execErr, durationMS); err != nil {
		jm.logger.Warn().Str("job_id", job.ID).Err(err).Msg("Failed to complete job in queue")
	}

	// Update stock index timestamp on success
	if execErr == nil {
		jm.updateStockIndexTimestamp(ctx, job)
	}

	if jm.hub != nil {
		eventType := "job_completed"
		if execErr != nil {
			eventType = "job_failed"
		}
		pending, _ := jm.storage.JobQueueStore().CountPending(ctx)
		// Update job fields for the broadcast
		job.DurationMS = durationMS
		if execErr != nil {
			job.Status = models.JobStatusFailed
			job.Error = execErr.Error()
		} else {
			job.Status = models.JobStatusCompleted
		}
		jm.hub.Broadcast(models.JobEvent{
			Type:      eventType,
			Job:       job,
			Timestamp: time.Now(),
			QueueSize: pending,
		})
	}
}

// PushToTop sets a job's priority to max(current_max) + 1.
func (jm *JobManager) PushToTop(ctx context.Context, id string) error {
	maxPriority, err := jm.storage.JobQueueStore().GetMaxPriority(ctx)
	if err != nil {
		return err
	}
	return jm.storage.JobQueueStore().SetPriority(ctx, id, maxPriority+1)
}

// EnqueueIfNeeded checks for an existing pending job with the same type+ticker
// and only enqueues if none exists (dedup).
func (jm *JobManager) EnqueueIfNeeded(ctx context.Context, jobType, ticker string, priority int) error {
	exists, err := jm.storage.JobQueueStore().HasPendingJob(ctx, jobType, ticker)
	if err != nil {
		return err
	}
	if exists {
		return nil // Already queued
	}

	job := &models.Job{
		JobType:     jobType,
		Ticker:      ticker,
		Priority:    priority,
		Status:      models.JobStatusPending,
		CreatedAt:   time.Now(),
		MaxAttempts: jm.config.GetMaxRetries(),
	}
	return jm.enqueue(ctx, job)
}
