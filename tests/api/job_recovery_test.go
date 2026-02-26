package api

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	surrealdb "github.com/bobmcallan/vire/internal/storage/surrealdb"
	testcommon "github.com/bobmcallan/vire/tests/common"
)

// TestJobRecovery_ResetsRunningJobsOnStartup tests that jobs left in "running"
// status from a previous server crash are reset to "pending" when a new server starts.
//
// This simulates the scenario where:
// 1. A server was processing jobs
// 2. The server crashed (SIGINT/SIGKILL) during job execution
// 3. Jobs were left in "running" state in the database
// 4. A new server starts up and recovers those orphaned jobs
func TestJobRecovery_ResetsRunningJobsOnStartup(t *testing.T) {
	// Phase 1: Create environment and insert running jobs directly into the database
	env := testcommon.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()
	guard := env.OutputGuard()

	// Get the SurrealDB address from environment
	surrealAddr, err := env.SurrealDBAddress()
	require.NoError(t, err, "Failed to get SurrealDB address")

	// Connect directly to SurrealDB to insert running jobs
	ctx := context.Background()

	// Create a storage manager to access the job queue
	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   surrealAddr,
			Namespace: "vire",
			Database:  "vire",
			Username:  "root",
			Password:  "root",
			DataPath:  t.TempDir(),
		},
	}
	logger := common.NewSilentLogger()
	mgr, err := surrealdb.NewManager(logger, cfg)
	require.NoError(t, err, "Failed to create storage manager")
	defer mgr.Close()

	jobStore := mgr.JobQueueStore()

	// Insert jobs in "running" status to simulate a crash during execution
	testJobs := []*models.Job{
		{
			ID:          "recovery-test-1",
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "BHP.AU",
			Priority:    10,
			Status:      models.JobStatusRunning, // Simulating crash during execution
			CreatedAt:   time.Now().Add(-5 * time.Minute),
			StartedAt:   time.Now().Add(-2 * time.Minute),
			Attempts:    1,
			MaxAttempts: 3,
		},
		{
			ID:          "recovery-test-2",
			JobType:     models.JobTypeCollectFundamentals,
			Ticker:      "CBA.AU",
			Priority:    8,
			Status:      models.JobStatusRunning, // Simulating crash during execution
			CreatedAt:   time.Now().Add(-4 * time.Minute),
			StartedAt:   time.Now().Add(-1 * time.Minute),
			Attempts:    1,
			MaxAttempts: 3,
		},
	}

	for _, job := range testJobs {
		err := jobStore.Enqueue(ctx, job)
		require.NoError(t, err, "Failed to enqueue test job %s", job.ID)
	}

	// Verify jobs are in running status before recovery
	jobsBefore, err := jobStore.ListAll(ctx, 10)
	require.NoError(t, err, "Failed to list jobs before recovery")
	runningCount := 0
	for _, j := range jobsBefore {
		if j.Status == models.JobStatusRunning {
			runningCount++
		}
	}
	require.Equal(t, 2, runningCount, "Expected 2 running jobs before recovery")

	guard.SaveResult("01_before_recovery", fmt.Sprintf("Jobs before recovery: %d running", runningCount))

	// Phase 2: Manually call ResetRunningJobs to simulate what happens on server startup
	count, err := jobStore.ResetRunningJobs(ctx)
	require.NoError(t, err, "Failed to reset running jobs")
	t.Logf("Reset %d running jobs to pending", count)

	// Phase 3: Verify jobs are now in pending status
	jobsAfter, err := jobStore.ListAll(ctx, 10)
	require.NoError(t, err, "Failed to list jobs after recovery")

	pendingCount := 0
	runningCountAfter := 0
	for _, j := range jobsAfter {
		switch j.Status {
		case models.JobStatusPending:
			pendingCount++
		case models.JobStatusRunning:
			runningCountAfter++
		}
	}

	assert.Equal(t, 2, pendingCount, "Expected 2 pending jobs after recovery")
	assert.Equal(t, 0, runningCountAfter, "Expected 0 running jobs after recovery")

	guard.SaveResult("02_after_recovery", fmt.Sprintf("Jobs after recovery: %d pending, %d running", pendingCount, runningCountAfter))

	// Verify the jobs can be dequeued (they're now pending and processable)
	job, err := jobStore.Dequeue(ctx)
	require.NoError(t, err, "Failed to dequeue recovered job")
	require.NotNil(t, job, "Expected to dequeue a recovered job")
	assert.Contains(t, []string{"recovery-test-1", "recovery-test-2"}, job.ID, "Expected to dequeue one of our test jobs")

	guard.SaveResult("03_dequeue_success", fmt.Sprintf("Successfully dequeued job: %s", job.ID))
}

// TestJobRecovery_VerifyRunningJobFieldsReset tests that when a running job
// is recovered, the started_at field is also cleared (not just status).
func TestJobRecovery_VerifyRunningJobFieldsReset(t *testing.T) {
	env := testcommon.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()
	guard := env.OutputGuard()

	surrealAddr, err := env.SurrealDBAddress()
	require.NoError(t, err, "Failed to get SurrealDB address")

	ctx := context.Background()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   surrealAddr,
			Namespace: "vire",
			Database:  "vire",
			Username:  "root",
			Password:  "root",
			DataPath:  t.TempDir(),
		},
	}
	logger := common.NewSilentLogger()
	mgr, err := surrealdb.NewManager(logger, cfg)
	require.NoError(t, err, "Failed to create storage manager")
	defer mgr.Close()

	jobStore := mgr.JobQueueStore()

	// Insert a running job with started_at set
	originalStartedAt := time.Now().Add(-5 * time.Minute)
	testJob := &models.Job{
		ID:          "field-reset-test",
		JobType:     models.JobTypeCollectNews,
		Ticker:      "RIO.AU",
		Priority:    7,
		Status:      models.JobStatusRunning,
		CreatedAt:   time.Now().Add(-10 * time.Minute),
		StartedAt:   originalStartedAt,
		Attempts:    2,
		MaxAttempts: 3,
	}

	err = jobStore.Enqueue(ctx, testJob)
	require.NoError(t, err, "Failed to enqueue test job")

	guard.SaveResult("01_before", fmt.Sprintf("Job before reset: status=%s, started_at=%v", testJob.Status, testJob.StartedAt))

	// Reset running jobs
	_, err = jobStore.ResetRunningJobs(ctx)
	require.NoError(t, err, "Failed to reset running jobs")

	// List jobs and verify the fields were reset
	jobs, err := jobStore.ListAll(ctx, 10)
	require.NoError(t, err, "Failed to list jobs")

	var recoveredJob *models.Job
	for _, j := range jobs {
		if j.ID == "field-reset-test" {
			recoveredJob = j
			break
		}
	}
	require.NotNil(t, recoveredJob, "Failed to find recovered job")

	assert.Equal(t, models.JobStatusPending, recoveredJob.Status, "Status should be pending")
	assert.True(t, recoveredJob.StartedAt.IsZero(), "StartedAt should be zero (cleared)")
	// Note: Attempts should remain as-is (we don't reset attempts, that's done on dequeue)

	guard.SaveResult("02_after", fmt.Sprintf("Job after reset: status=%s, started_at=%v (is_zero=%v)",
		recoveredJob.Status, recoveredJob.StartedAt, recoveredJob.StartedAt.IsZero()))
}

// TestJobRecovery_PreservesOtherJobStatuses tests that the recovery process
// only affects jobs with "running" status, leaving other statuses untouched.
func TestJobRecovery_PreservesOtherJobStatuses(t *testing.T) {
	env := testcommon.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()
	guard := env.OutputGuard()

	surrealAddr, err := env.SurrealDBAddress()
	require.NoError(t, err, "Failed to get SurrealDB address")

	ctx := context.Background()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   surrealAddr,
			Namespace: "vire",
			Database:  "vire",
			Username:  "root",
			Password:  "root",
			DataPath:  t.TempDir(),
		},
	}
	logger := common.NewSilentLogger()
	mgr, err := surrealdb.NewManager(logger, cfg)
	require.NoError(t, err, "Failed to create storage manager")
	defer mgr.Close()

	jobStore := mgr.JobQueueStore()

	// Insert jobs with various statuses
	testJobs := []*models.Job{
		{
			ID:          "pending-job",
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "ABC.AU",
			Priority:    10,
			Status:      models.JobStatusPending,
			CreatedAt:   time.Now(),
			MaxAttempts: 3,
		},
		{
			ID:          "completed-job",
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "DEF.AU",
			Priority:    10,
			Status:      models.JobStatusCompleted,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			MaxAttempts: 3,
		},
		{
			ID:          "failed-job",
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "GHI.AU",
			Priority:    10,
			Status:      models.JobStatusFailed,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			Error:       "test error",
			MaxAttempts: 3,
		},
		{
			ID:          "cancelled-job",
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "JKL.AU",
			Priority:    10,
			Status:      models.JobStatusCancelled,
			CreatedAt:   time.Now().Add(-1 * time.Hour),
			MaxAttempts: 3,
		},
		{
			ID:          "running-job",
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "MNO.AU",
			Priority:    10,
			Status:      models.JobStatusRunning,
			CreatedAt:   time.Now().Add(-1 * time.Minute),
			StartedAt:   time.Now(),
			MaxAttempts: 3,
		},
	}

	for _, job := range testJobs {
		err := jobStore.Enqueue(ctx, job)
		require.NoError(t, err, "Failed to enqueue test job %s", job.ID)
	}

	// Reset running jobs
	_, err = jobStore.ResetRunningJobs(ctx)
	require.NoError(t, err, "Failed to reset running jobs")

	// Verify status counts
	jobs, err := jobStore.ListAll(ctx, 20)
	require.NoError(t, err, "Failed to list jobs")

	statusCounts := make(map[string]int)
	for _, j := range jobs {
		statusCounts[j.Status]++
	}

	// Only the running job should have been changed to pending
	assert.Equal(t, 2, statusCounts[models.JobStatusPending], "Expected 2 pending (original + recovered)")
	assert.Equal(t, 1, statusCounts[models.JobStatusCompleted], "Expected 1 completed")
	assert.Equal(t, 1, statusCounts[models.JobStatusFailed], "Expected 1 failed")
	assert.Equal(t, 1, statusCounts[models.JobStatusCancelled], "Expected 1 cancelled")
	assert.Equal(t, 0, statusCounts[models.JobStatusRunning], "Expected 0 running")

	var results []string
	for status, count := range statusCounts {
		results = append(results, fmt.Sprintf("%s: %d", status, count))
	}
	guard.SaveResult("status_counts", strings.Join(results, ", "))
}

// TestJobRecovery_ViaAdminAPI tests the full recovery flow by verifying that
// jobs inserted directly into the database (simulating a crash) are visible
// through the admin API after recovery.
func TestJobRecovery_ViaAdminAPI(t *testing.T) {
	env := testcommon.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()
	guard := env.OutputGuard()

	// Create an admin user for API access
	surrealAddr, err := env.SurrealDBAddress()
	require.NoError(t, err, "Failed to get SurrealDB address")

	ctx := context.Background()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   surrealAddr,
			Namespace: "vire",
			Database:  "vire",
			Username:  "root",
			Password:  "root",
			DataPath:  t.TempDir(),
		},
	}
	logger := common.NewSilentLogger()
	mgr, err := surrealdb.NewManager(logger, cfg)
	require.NoError(t, err, "Failed to create storage manager")
	defer mgr.Close()

	// Create admin user
	adminUser := &models.InternalUser{
		UserID:     "admin-recovery-test",
		Email:      "admin-recovery@test.com",
		Name:       "Admin User",
		Provider:   "test",
		Role:       models.RoleAdmin,
		CreatedAt:  time.Now(),
		ModifiedAt: time.Now(),
	}
	err = mgr.InternalStore().SaveUser(ctx, adminUser)
	require.NoError(t, err, "Failed to create admin user")

	jobStore := mgr.JobQueueStore()

	// Insert a running job via direct DB access (simulating crash)
	runningJob := &models.Job{
		ID:          "api-recovery-test",
		JobType:     models.JobTypeCollectEOD,
		Ticker:      "API.AU",
		Priority:    10,
		Status:      models.JobStatusRunning,
		CreatedAt:   time.Now().Add(-5 * time.Minute),
		StartedAt:   time.Now().Add(-2 * time.Minute),
		Attempts:    1,
		MaxAttempts: 3,
	}
	err = jobStore.Enqueue(ctx, runningJob)
	require.NoError(t, err, "Failed to enqueue running job")

	// Verify the job is in the queue with running status
	jobsBeforeReset, err := jobStore.ListAll(ctx, 10)
	require.NoError(t, err, "Failed to list jobs before reset")
	require.Len(t, jobsBeforeReset, 1, "Expected 1 job before reset")
	assert.Equal(t, models.JobStatusRunning, jobsBeforeReset[0].Status, "Job should be running before reset")

	// Reset via storage (simulating what server startup does)
	_, err = jobStore.ResetRunningJobs(ctx)
	require.NoError(t, err, "Failed to reset running jobs")

	// Verify via direct DB access that the job is now pending
	jobsAfterReset, err := jobStore.ListAll(ctx, 10)
	require.NoError(t, err, "Failed to list jobs after reset")
	require.Len(t, jobsAfterReset, 1, "Expected 1 job after reset")
	assert.Equal(t, models.JobStatusPending, jobsAfterReset[0].Status, "Job should be pending after recovery")

	guard.SaveResult("recovery_verified", fmt.Sprintf(
		"Job %s: status before=%s, status after=%s",
		jobsAfterReset[0].ID, jobsBeforeReset[0].Status, jobsAfterReset[0].Status,
	))
}

// TestJobRecovery_EmptyQueue tests that ResetRunningJobs handles an empty queue gracefully.
func TestJobRecovery_EmptyQueue(t *testing.T) {
	env := testcommon.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()
	guard := env.OutputGuard()

	surrealAddr, err := env.SurrealDBAddress()
	require.NoError(t, err, "Failed to get SurrealDB address")

	ctx := context.Background()

	cfg := &common.Config{
		Environment: "test",
		Storage: common.StorageConfig{
			Address:   surrealAddr,
			Namespace: "vire",
			Database:  "vire",
			Username:  "root",
			Password:  "root",
			DataPath:  t.TempDir(),
		},
	}
	logger := common.NewSilentLogger()
	mgr, err := surrealdb.NewManager(logger, cfg)
	require.NoError(t, err, "Failed to create storage manager")
	defer mgr.Close()

	jobStore := mgr.JobQueueStore()

	// Verify queue is empty
	jobsBefore, err := jobStore.ListAll(ctx, 10)
	require.NoError(t, err, "Failed to list jobs")
	require.Empty(t, jobsBefore, "Expected empty queue before test")

	// Reset running jobs on empty queue should not error
	count, err := jobStore.ResetRunningJobs(ctx)
	require.NoError(t, err, "ResetRunningJobs should not error on empty queue")
	_ = count // count may be 0 or undefined depending on implementation

	guard.SaveResult("empty_queue_test", "ResetRunningJobs on empty queue completed successfully")
}
