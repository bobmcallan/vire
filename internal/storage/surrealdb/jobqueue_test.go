package surrealdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestJobQueueStore_EnqueueAndDequeue(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	job := &models.Job{
		JobType:     models.JobTypeCollectEOD,
		Ticker:      "BHP.AU",
		Priority:    10,
		MaxAttempts: 3,
	}

	if err := store.Enqueue(ctx, job); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if job.ID == "" {
		t.Error("expected job ID to be set after enqueue")
	}
	if job.Status != models.JobStatusPending {
		t.Errorf("expected status pending, got %s", job.Status)
	}

	// Dequeue should return the job
	got, err := store.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected a job from dequeue")
	}
	if got.Status != models.JobStatusRunning {
		t.Errorf("expected status running after dequeue, got %s", got.Status)
	}
	if got.Ticker != "BHP.AU" {
		t.Errorf("expected ticker BHP.AU, got %s", got.Ticker)
	}
}

func TestJobQueueStore_Dequeue_PriorityOrdering(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	// Enqueue low priority first, high priority second
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectTimeline, Ticker: "A.AU", Priority: 2, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "B.AU", Priority: 10, MaxAttempts: 3})

	// Should dequeue high priority first
	got, _ := store.Dequeue(ctx)
	if got == nil {
		t.Fatal("expected a job")
	}
	if got.Ticker != "B.AU" {
		t.Errorf("expected B.AU (priority 10) first, got %s", got.Ticker)
	}

	got2, _ := store.Dequeue(ctx)
	if got2 == nil {
		t.Fatal("expected second job")
	}
	if got2.Ticker != "A.AU" {
		t.Errorf("expected A.AU (priority 2) second, got %s", got2.Ticker)
	}
}

func TestJobQueueStore_Dequeue_EmptyQueue(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	got, err := store.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue on empty queue failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil from empty queue, got %v", got)
	}
}

func TestJobQueueStore_Complete(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	job := &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3}
	store.Enqueue(ctx, job)

	dequeued, _ := store.Dequeue(ctx)

	// Complete successfully
	if err := store.Complete(ctx, dequeued.ID, nil, 100); err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Job should no longer be pending
	pending, _ := store.CountPending(ctx)
	if pending != 0 {
		t.Errorf("expected 0 pending after complete, got %d", pending)
	}
}

func TestJobQueueStore_Complete_WithError(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	job := &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3}
	store.Enqueue(ctx, job)

	dequeued, _ := store.Dequeue(ctx)

	// Complete with error
	if err := store.Complete(ctx, dequeued.ID, fmt.Errorf("API error"), 50); err != nil {
		t.Fatalf("Complete with error failed: %v", err)
	}
}

func TestJobQueueStore_Cancel(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	job := &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3}
	store.Enqueue(ctx, job)

	if err := store.Cancel(ctx, job.ID); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}

	pending, _ := store.CountPending(ctx)
	if pending != 0 {
		t.Errorf("expected 0 pending after cancel, got %d", pending)
	}
}

func TestJobQueueStore_HasPendingJob(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	// No pending jobs initially
	has, _ := store.HasPendingJob(ctx, models.JobTypeCollectEOD, "BHP.AU")
	if has {
		t.Error("expected no pending job initially")
	}

	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3})

	has, _ = store.HasPendingJob(ctx, models.JobTypeCollectEOD, "BHP.AU")
	if !has {
		t.Error("expected pending job after enqueue")
	}

	// Different ticker should not match
	has, _ = store.HasPendingJob(ctx, models.JobTypeCollectEOD, "RIO.AU")
	if has {
		t.Error("expected no pending job for different ticker")
	}

	// Different job type should not match
	has, _ = store.HasPendingJob(ctx, models.JobTypeCollectFilings, "BHP.AU")
	if has {
		t.Error("expected no pending job for different job type")
	}
}

func TestJobQueueStore_ListPending(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "A.AU", Priority: 10, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "B.AU", Priority: 5, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "C.AU", Priority: 8, MaxAttempts: 3})

	jobs, err := store.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending failed: %v", err)
	}
	if len(jobs) != 3 {
		t.Errorf("expected 3 pending jobs, got %d", len(jobs))
	}
}

func TestJobQueueStore_SetPriority(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	job := &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 5, MaxAttempts: 3}
	store.Enqueue(ctx, job)

	if err := store.SetPriority(ctx, job.ID, 20); err != nil {
		t.Fatalf("SetPriority failed: %v", err)
	}

	maxP, _ := store.GetMaxPriority(ctx)
	if maxP != 20 {
		t.Errorf("expected max priority 20, got %d", maxP)
	}
}

func TestJobQueueStore_CancelByTicker(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectFilings, Ticker: "BHP.AU", Priority: 5, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "RIO.AU", Priority: 10, MaxAttempts: 3})

	store.CancelByTicker(ctx, "BHP.AU")

	pending, _ := store.CountPending(ctx)
	if pending != 1 {
		t.Errorf("expected 1 pending job after CancelByTicker, got %d", pending)
	}
}

func TestJobQueueStore_PurgeCompleted(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	job := &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3}
	store.Enqueue(ctx, job)

	dequeued, _ := store.Dequeue(ctx)
	store.Complete(ctx, dequeued.ID, nil, 100)

	// Purge with future cutoff (should purge everything)
	_, err := store.PurgeCompleted(ctx, time.Now().Add(1*time.Hour))
	if err != nil {
		t.Fatalf("PurgeCompleted failed: %v", err)
	}
}

func TestJobQueueStore_ListAll(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "A.AU", Priority: 10, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "B.AU", Priority: 5, MaxAttempts: 3})

	jobs, err := store.ListAll(ctx, 10)
	if err != nil {
		t.Fatalf("ListAll failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}
}

func TestJobQueueStore_ListByTicker(t *testing.T) {
	db := testDB(t)
	store := NewJobQueueStore(db, testLogger())
	ctx := context.Background()

	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "BHP.AU", Priority: 10, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectFilings, Ticker: "BHP.AU", Priority: 5, MaxAttempts: 3})
	store.Enqueue(ctx, &models.Job{JobType: models.JobTypeCollectEOD, Ticker: "RIO.AU", Priority: 10, MaxAttempts: 3})

	jobs, err := store.ListByTicker(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("ListByTicker failed: %v", err)
	}
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs for BHP.AU, got %d", len(jobs))
	}
}
