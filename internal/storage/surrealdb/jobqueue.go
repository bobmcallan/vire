package surrealdb

import (
	"context"
	"fmt"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/google/uuid"
	"github.com/surrealdb/surrealdb.go"
	surrealmodels "github.com/surrealdb/surrealdb.go/pkg/models"
)

// jobSelectFields lists the fields to select from job_queue, aliasing job_id to id for struct mapping.
const jobSelectFields = "job_id as id, job_type, ticker, priority, status, created_at, started_at, completed_at, error, attempts, max_attempts, duration_ms"

// JobQueueStore implements interfaces.JobQueueStore using SurrealDB.
type JobQueueStore struct {
	db     *surrealdb.DB
	logger *common.Logger
}

// NewJobQueueStore creates a new JobQueueStore.
func NewJobQueueStore(db *surrealdb.DB, logger *common.Logger) *JobQueueStore {
	return &JobQueueStore{db: db, logger: logger}
}

func (s *JobQueueStore) Enqueue(ctx context.Context, job *models.Job) error {
	if job.ID == "" {
		job.ID = uuid.New().String()[:8]
	}
	if job.Status == "" {
		job.Status = models.JobStatusPending
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = 3
	}

	sql := `UPSERT $rid SET
		job_id = $job_id, job_type = $job_type, ticker = $ticker, priority = $priority,
		status = $status, created_at = $created_at, started_at = $started_at,
		completed_at = $completed_at, error = $error, attempts = $attempts,
		max_attempts = $max_attempts, duration_ms = $duration_ms`
	vars := map[string]any{
		"rid":          surrealmodels.NewRecordID("job_queue", job.ID),
		"job_id":       job.ID,
		"job_type":     job.JobType,
		"ticker":       job.Ticker,
		"priority":     job.Priority,
		"status":       job.Status,
		"created_at":   job.CreatedAt,
		"started_at":   job.StartedAt,
		"completed_at": job.CompletedAt,
		"error":        job.Error,
		"attempts":     job.Attempts,
		"max_attempts": job.MaxAttempts,
		"duration_ms":  job.DurationMS,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to enqueue job: %w", err)
	}
	return nil
}

func (s *JobQueueStore) Dequeue(ctx context.Context) (*models.Job, error) {
	// Two-step dequeue: SELECT highest priority pending job, then UPDATE it to running.
	// Step 1: Find the candidate (alias job_id as id for struct mapping)
	selectSQL := "SELECT " + jobSelectFields + " FROM job_queue WHERE status = $pending ORDER BY priority DESC, created_at ASC LIMIT 1"
	vars := map[string]any{
		"pending": models.JobStatusPending,
	}

	candidates, err := surrealdb.Query[[]models.Job](ctx, s.db, selectSQL, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to select candidate job: %w", err)
	}

	if candidates == nil || len(*candidates) == 0 || len((*candidates)[0].Result) == 0 {
		return nil, nil
	}

	candidate := (*candidates)[0].Result[0]

	// Step 2: Atomically claim the job — only update if still pending (prevents double-claim)
	now := time.Now()
	updateSQL := `UPDATE $rid SET status = $running, started_at = $now, attempts = attempts + 1 WHERE status = $pending`
	updateVars := map[string]any{
		"rid":     surrealmodels.NewRecordID("job_queue", candidate.ID),
		"running": models.JobStatusRunning,
		"pending": models.JobStatusPending,
		"now":     now,
	}

	_, err = surrealdb.Query[any](ctx, s.db, updateSQL, updateVars)
	if err != nil {
		return nil, fmt.Errorf("failed to dequeue job: %w", err)
	}

	// Return the updated job
	candidate.Status = models.JobStatusRunning
	candidate.StartedAt = now
	candidate.Attempts++
	return &candidate, nil
}

func (s *JobQueueStore) Complete(ctx context.Context, id string, jobErr error, durationMS int64) error {
	now := time.Now()
	status := models.JobStatusCompleted
	errStr := ""
	if jobErr != nil {
		status = models.JobStatusFailed
		errStr = jobErr.Error()
	}

	sql := "UPDATE $rid SET status = $status, completed_at = $now, error = $error, duration_ms = $dur"
	vars := map[string]any{
		"rid":    surrealmodels.NewRecordID("job_queue", id),
		"status": status,
		"now":    now,
		"error":  errStr,
		"dur":    durationMS,
	}

	if _, err := surrealdb.Query[[]models.Job](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to complete job: %w", err)
	}
	return nil
}

func (s *JobQueueStore) Cancel(ctx context.Context, id string) error {
	sql := "UPDATE $rid SET status = $status WHERE status = $pending"
	vars := map[string]any{
		"rid":     surrealmodels.NewRecordID("job_queue", id),
		"status":  models.JobStatusCancelled,
		"pending": models.JobStatusPending,
	}

	if _, err := surrealdb.Query[[]models.Job](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to cancel job: %w", err)
	}
	return nil
}

func (s *JobQueueStore) SetPriority(ctx context.Context, id string, priority int) error {
	sql := "UPDATE $rid SET priority = $priority"
	vars := map[string]any{
		"rid":      surrealmodels.NewRecordID("job_queue", id),
		"priority": priority,
	}

	if _, err := surrealdb.Query[[]models.Job](ctx, s.db, sql, vars); err != nil {
		return fmt.Errorf("failed to set priority: %w", err)
	}
	return nil
}

func (s *JobQueueStore) GetMaxPriority(ctx context.Context) (int, error) {
	sql := "SELECT math::max(priority) AS max_priority FROM job_queue WHERE status = $pending GROUP ALL"
	vars := map[string]any{"pending": models.JobStatusPending}

	type maxResult struct {
		MaxPriority int `json:"max_priority"`
	}

	results, err := surrealdb.Query[[]maxResult](ctx, s.db, sql, vars)
	if err != nil {
		return 0, fmt.Errorf("failed to get max priority: %w", err)
	}

	if results != nil && len(*results) > 0 && len((*results)[0].Result) > 0 {
		return (*results)[0].Result[0].MaxPriority, nil
	}
	return 0, nil
}

func (s *JobQueueStore) ListPending(ctx context.Context, limit int) ([]*models.Job, error) {
	if limit <= 0 {
		limit = 100
	}
	sql := "SELECT " + jobSelectFields + " FROM job_queue WHERE status = $pending ORDER BY priority DESC, created_at ASC LIMIT $limit"
	vars := map[string]any{"pending": models.JobStatusPending, "limit": limit}
	return s.queryJobs(ctx, sql, vars)
}

func (s *JobQueueStore) ListAll(ctx context.Context, limit int) ([]*models.Job, error) {
	if limit <= 0 {
		limit = 100
	}
	sql := "SELECT " + jobSelectFields + " FROM job_queue ORDER BY created_at DESC LIMIT $limit"
	vars := map[string]any{"limit": limit}
	return s.queryJobs(ctx, sql, vars)
}

func (s *JobQueueStore) ListByTicker(ctx context.Context, ticker string) ([]*models.Job, error) {
	sql := "SELECT " + jobSelectFields + " FROM job_queue WHERE ticker = $ticker ORDER BY created_at DESC"
	vars := map[string]any{"ticker": ticker}
	return s.queryJobs(ctx, sql, vars)
}

func (s *JobQueueStore) CountPending(ctx context.Context) (int, error) {
	sql := "SELECT count() AS cnt FROM job_queue WHERE status = $pending GROUP ALL"
	vars := map[string]any{"pending": models.JobStatusPending}

	type countResult struct {
		Cnt int `json:"cnt"`
	}

	results, err := surrealdb.Query[[]countResult](ctx, s.db, sql, vars)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending: %w", err)
	}

	if results != nil && len(*results) > 0 && len((*results)[0].Result) > 0 {
		return (*results)[0].Result[0].Cnt, nil
	}
	return 0, nil
}

func (s *JobQueueStore) HasPendingJob(ctx context.Context, jobType, ticker string) (bool, error) {
	sql := "SELECT count() AS cnt FROM job_queue WHERE job_type = $type AND ticker = $ticker AND status = $pending GROUP ALL"
	vars := map[string]any{
		"type":    jobType,
		"ticker":  ticker,
		"pending": models.JobStatusPending,
	}

	type countResult struct {
		Cnt int `json:"cnt"`
	}

	results, err := surrealdb.Query[[]countResult](ctx, s.db, sql, vars)
	if err != nil {
		return false, fmt.Errorf("failed to check pending job: %w", err)
	}

	if results != nil && len(*results) > 0 && len((*results)[0].Result) > 0 {
		return (*results)[0].Result[0].Cnt > 0, nil
	}
	return false, nil
}

func (s *JobQueueStore) PurgeCompleted(ctx context.Context, olderThan time.Time) (int, error) {
	sql := "DELETE FROM job_queue WHERE status IN [$completed, $failed] AND completed_at < $cutoff"
	vars := map[string]any{
		"completed": models.JobStatusCompleted,
		"failed":    models.JobStatusFailed,
		"cutoff":    olderThan,
	}

	if _, err := surrealdb.Query[any](ctx, s.db, sql, vars); err != nil {
		return 0, fmt.Errorf("failed to purge completed jobs: %w", err)
	}
	// SurrealDB DELETE doesn't return count easily, return 0
	return 0, nil
}

func (s *JobQueueStore) CancelByTicker(ctx context.Context, ticker string) (int, error) {
	sql := "UPDATE job_queue SET status = $cancelled WHERE ticker = $ticker AND status = $pending"
	vars := map[string]any{
		"cancelled": models.JobStatusCancelled,
		"ticker":    ticker,
		"pending":   models.JobStatusPending,
	}

	if _, err := surrealdb.Query[[]models.Job](ctx, s.db, sql, vars); err != nil {
		return 0, fmt.Errorf("failed to cancel jobs by ticker: %w", err)
	}
	return 0, nil
}

// queryJobs is a helper that runs a query and returns a slice of Job pointers.
func (s *JobQueueStore) queryJobs(ctx context.Context, sql string, vars map[string]any) ([]*models.Job, error) {
	results, err := surrealdb.Query[[]models.Job](ctx, s.db, sql, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to query jobs: %w", err)
	}

	var jobs []*models.Job
	if results != nil && len(*results) > 0 {
		for i := range (*results)[0].Result {
			jobs = append(jobs, &(*results)[0].Result[i])
		}
	}
	return jobs, nil
}

// ResetRunningJobs resets all jobs with status "running" back to "pending".
// Called on startup to recover jobs that were in-flight when the process crashed.
func (s *JobQueueStore) ResetRunningJobs(ctx context.Context) (int, error) {
	sql := `UPDATE job_queue SET status = $pending, started_at = NONE WHERE status = $running`
	_, err := surrealdb.Query[any](ctx, s.db, sql, map[string]interface{}{
		"pending": models.JobStatusPending,
		"running": models.JobStatusRunning,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to reset running jobs: %w", err)
	}
	// SurrealDB UPDATE doesn't easily return affected count; return 0
	return 0, nil
}

// Compile-time check
var _ interfaces.JobQueueStore = (*JobQueueStore)(nil)
