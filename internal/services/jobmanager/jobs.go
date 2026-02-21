package jobmanager

import (
	"context"
	"fmt"
	"time"
)

// JobRun records the outcome of a single job cycle (legacy, kept for /api/jobs/status).
type JobRun struct {
	ID               string    `json:"id"`
	StartedAt        time.Time `json:"started_at"`
	CompletedAt      time.Time `json:"completed_at"`
	Status           string    `json:"status"`
	TickersProcessed int       `json:"tickers_processed"`
	TickersDetailed  int       `json:"tickers_detailed"`
	Errors           []string  `json:"errors,omitempty"`
	DurationMS       int64     `json:"duration_ms"`
}

// LastJobRun retrieves the most recent job run info from storage.
func (jm *JobManager) LastJobRun(ctx context.Context) *JobRun {
	store := jm.storage.InternalStore()

	status, _ := store.GetSystemKV(ctx, "last_job_run_status")
	if status == "" {
		return nil
	}

	run := &JobRun{Status: status}

	if ts, err := store.GetSystemKV(ctx, "last_job_run_at"); err == nil {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			run.CompletedAt = t
		}
	}

	if dur, err := store.GetSystemKV(ctx, "last_job_run_duration_ms"); err == nil {
		fmt.Sscanf(dur, "%d", &run.DurationMS)
	}

	if tc, err := store.GetSystemKV(ctx, "last_job_run_tickers"); err == nil {
		fmt.Sscanf(tc, "%d", &run.TickersProcessed)
	}

	return run
}
