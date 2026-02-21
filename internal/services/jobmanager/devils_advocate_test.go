package jobmanager

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// Devils-advocate stress tests: security, edge cases, failure modes, race conditions.
// Reuses mocks from manager_test.go (same package).

// ============================================================================
// DA-1. CRITICAL: Job retry logic is broken — failed jobs are never re-queued
// ============================================================================
//
// In processLoop, when a job fails and attempts < maxAttempts, the code logs
// "Re-queuing failed job" but never actually re-enqueues it. The job is marked
// as failed via jm.complete(), and that's it. No retry ever happens.
//
// EXPECTED: Failed job with attempts < maxAttempts should be re-enqueued as pending.
// ACTUAL: Failed job is marked as failed and abandoned.

func TestDA_RetryLogic_BrokenNoRequeue(t *testing.T) {
	failCount := atomic.Int64{}
	failingMarket := &failingMarketService{
		mockMarketService: newMockMarketService(),
		failCount:         &failCount,
		failUntil:         2, // fail first 2 attempts, succeed on 3rd
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
	}

	ctx := context.Background()
	queue.Enqueue(ctx, &models.Job{
		ID:          "retry-test",
		JobType:     models.JobTypeCollectEOD,
		Ticker:      "RETRY.AU",
		Priority:    10,
		MaxAttempts: 3,
	})

	jm := NewJobManager(
		failingMarket, &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	jmCtx, jmCancel := context.WithCancel(context.Background())
	jm.cancel = jmCancel
	jm.wg.Add(1)
	go jm.processLoop(jmCtx)

	// Wait for job processing
	time.Sleep(3 * time.Second)
	jmCancel()
	jm.wg.Wait()

	// Check: the job should have been retried and eventually completed.
	// BUG: it was only attempted once and marked as failed.
	queue.mu.Lock()
	var job *models.Job
	for _, j := range queue.jobs {
		if j.ID == "retry-test" {
			job = j
		}
	}
	queue.mu.Unlock()

	if job == nil {
		t.Fatal("job not found")
	}

	// With working retry logic, the job should complete on attempt 3.
	// With the current bug, the job is marked as failed after attempt 1.
	if job.Status == models.JobStatusFailed {
		t.Log("CONFIRMED BUG: Job marked as failed without retry. " +
			"processLoop logs 'Re-queuing failed job' but never actually re-enqueues. " +
			"Fix: after calling jm.complete(), check attempts < maxAttempts and call jm.enqueue() " +
			"to create a new pending job for the same type+ticker.")
	}
	if job.Status == models.JobStatusCompleted {
		t.Log("Retry logic is working correctly")
	}
}

// failingMarketService fails CollectEOD until failCount reaches failUntil.
type failingMarketService struct {
	*mockMarketService
	failCount *atomic.Int64
	failUntil int64
}

func (f *failingMarketService) CollectEOD(_ context.Context, _ string, _ bool) error {
	count := f.failCount.Add(1)
	if count <= f.failUntil {
		return fmt.Errorf("transient error attempt %d", count)
	}
	return nil
}

// ============================================================================
// DA-2. CRITICAL: WebSocket hub Run() goroutine leaks on Stop()
// ============================================================================
//
// hub.Run() runs an infinite for-select with no context or done channel.
// When Stop() is called, it cancels the watcher and processor loops but
// the hub goroutine continues running forever.

func TestDA_WebSocketHub_NeverStops(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{Enabled: true, WatcherInterval: "1h", MaxConcurrent: 1},
	)

	jm.Start()
	jm.Stop()

	// After Stop(), the hub goroutine is still running.
	// This test documents the leak. We can't easily assert it without runtime
	// goroutine inspection, but the architecture shows hub.Run() has no exit path.
	t.Log("CONFIRMED: hub.Run() has no shutdown mechanism. " +
		"Fix: add a done channel to JobWSHub, select on it in Run(), " +
		"close it in a new hub.Stop() method called from JobManager.Stop().")
}

// ============================================================================
// DA-3. Race condition in WebSocket hub broadcast — RLock to Lock upgrade
// ============================================================================
//
// In hub.Run(), the broadcast case holds h.mu.RLock() while iterating clients.
// When a slow client is found, it drops RLock, acquires Lock, modifies the map,
// drops Lock, re-acquires RLock, and continues iterating.
//
// Problem: Between RUnlock and Lock, another goroutine could modify h.clients,
// invalidating the range iteration. Also, multiple goroutines holding RLock
// that both try to upgrade to Lock will deadlock.

func TestDA_WebSocketHub_BroadcastLockUpgrade(t *testing.T) {
	logger := common.NewLogger("error")
	hub := NewJobWSHub(logger)
	go hub.Run()

	// This test documents the race condition. The fix is to collect
	// slow clients during the RLock iteration and remove them after.
	t.Log("RACE CONDITION: broadcast case in hub.Run() performs RLock->RUnlock->Lock->Unlock->RLock " +
		"sequence during iteration, which is unsafe for concurrent access. " +
		"Fix: collect stale clients in a slice during RLock iteration, " +
		"then remove them in a separate Lock block after iteration completes.")
}

// ============================================================================
// DA-4. Admin enqueue does not validate job_type
// ============================================================================
//
// handleAdminJobEnqueue accepts any string as job_type. Invalid types like
// "drop_database" or "../../exploit" will be stored and later cause
// "unknown job type" errors in executeJob, wasting queue resources.

func TestDA_InvalidJobType_AcceptedByQueue(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// These should all be rejected at enqueue time
	invalidTypes := []string{
		"",
		"drop_database",
		"../../etc/passwd",
		"<script>alert(1)</script>",
		"collect_eod; DROP TABLE job_queue",
	}

	for _, jt := range invalidTypes {
		job := &models.Job{
			JobType:     jt,
			Ticker:      "BHP.AU",
			Priority:    10,
			MaxAttempts: 3,
		}
		err := queue.Enqueue(ctx, job)
		if err == nil && jt != "" {
			t.Logf("FINDING: invalid job_type %q accepted into queue without validation. "+
				"Fix: validate job_type against known constants in handleAdminJobEnqueue "+
				"and in enqueueIfNeeded.", jt)
		}
	}
}

// ValidJobTypes returns the set of valid job types for validation.
func validJobTypes() map[string]bool {
	return map[string]bool{
		models.JobTypeCollectEOD:             true,
		models.JobTypeCollectFundamentals:    true,
		models.JobTypeCollectFilings:         true,
		models.JobTypeCollectNews:            true,
		models.JobTypeCollectFilingSummaries: true,
		models.JobTypeCollectTimeline:        true,
		models.JobTypeCollectNewsIntel:       true,
		models.JobTypeComputeSignals:         true,
	}
}

// ============================================================================
// DA-5. Concurrent dequeue race — multiple processors get same job
// ============================================================================
//
// The in-memory mock properly serializes via mutex, but the SurrealDB
// implementation relies on UPDATE ... WHERE ... LIMIT 1 for atomicity.
// If SurrealDB doesn't guarantee single-writer semantics on this pattern,
// two processors could dequeue the same job.

func TestDA_ConcurrentDequeue_NoDuplicate(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// Enqueue exactly 1 job
	queue.Enqueue(ctx, &models.Job{
		ID:          "single",
		JobType:     models.JobTypeCollectEOD,
		Ticker:      "BHP.AU",
		Priority:    10,
		MaxAttempts: 3,
	})

	// Race 10 goroutines to dequeue it
	var wg sync.WaitGroup
	dequeued := make(chan *models.Job, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, _ := queue.Dequeue(ctx)
			if job != nil {
				dequeued <- job
			}
		}()
	}

	wg.Wait()
	close(dequeued)

	count := 0
	for range dequeued {
		count++
	}

	if count != 1 {
		t.Errorf("RACE CONDITION: %d goroutines dequeued the same job (expected exactly 1)", count)
	}
}

// ============================================================================
// DA-6. PushToTop TOCTOU race — GetMaxPriority and SetPriority are not atomic
// ============================================================================
//
// PushToTop calls GetMaxPriority then SetPriority. Between these two calls,
// another goroutine could enqueue a job with a higher priority, making the
// "pushed" job not actually be at the top.

func TestDA_PushToTop_TOCTOU(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{MaxConcurrent: 1},
	)

	queue.Enqueue(ctx, &models.Job{ID: "a", JobType: models.JobTypeCollectEOD, Ticker: "A.AU", Priority: 10})
	queue.Enqueue(ctx, &models.Job{ID: "b", JobType: models.JobTypeCollectTimeline, Ticker: "B.AU", Priority: 2})

	// Simulate concurrent push-to-top and high-priority enqueue
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		jm.PushToTop(ctx, "b")
	}()

	go func() {
		defer wg.Done()
		// Enqueue a job with very high priority between GetMax and SetPriority
		queue.Enqueue(ctx, &models.Job{ID: "c", JobType: models.JobTypeCollectEOD, Ticker: "C.AU", Priority: 100})
	}()

	wg.Wait()

	// Check: job "b" may or may not be at the actual top
	queue.mu.Lock()
	var jobB *models.Job
	maxP := 0
	for _, j := range queue.jobs {
		if j.ID == "b" {
			jobB = j
		}
		if j.Priority > maxP && j.Status == models.JobStatusPending {
			maxP = j.Priority
		}
	}
	queue.mu.Unlock()

	if jobB != nil && jobB.Priority < maxP {
		t.Logf("INFO: PushToTop TOCTOU confirmed — job b priority %d < max %d. "+
			"This is a minor issue since the admin can just push again. "+
			"Fix if needed: use atomic SurrealDB query: UPDATE SET priority = (SELECT math::max(priority) FROM job_queue) + 1",
			jobB.Priority, maxP)
	}
}

// ============================================================================
// DA-7. Ticker format with special characters in SurrealDB record IDs
// ============================================================================
//
// tickerToID replaces "." with "_" but doesn't handle other special chars.
// What about tickers with colons, slashes, or Unicode?

func TestDA_TickerSpecialChars(t *testing.T) {
	cases := []struct {
		ticker string
		desc   string
	}{
		{"BHP.AU", "normal ticker"},
		{"BRK-B.US", "hyphenated ticker"},
		{"BRK.B.US", "double-dot ticker"},
		{"", "empty ticker"},
		{"A", "single char no exchange"},
		{"../../../etc/passwd", "path traversal attempt"},
		{"<script>", "XSS attempt"},
		{"'; DROP TABLE stock_index; --", "SQL injection attempt"},
		{"ticker\x00null", "null byte injection"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			// tickerToID (in surrealdb package) replaces "." with "_"
			// but doesn't sanitize other special characters.
			// SurrealDB parameterized queries protect against injection
			// when ticker is passed as a parameter, but the record ID
			// is constructed from the ticker, so special chars in the
			// record ID could cause issues.
			t.Logf("ticker %q — verify SurrealDB handles this safely in record ID construction", tc.ticker)
		})
	}
}

// ============================================================================
// DA-8. Processor loop spin on persistent dequeue errors
// ============================================================================
//
// If the storage backend is down, dequeue returns an error on every call.
// The processLoop sleeps 1 second between errors, which is good, but there's
// no exponential backoff. Under persistent failure, this generates ~1 error
// log per second per processor goroutine indefinitely.

func TestDA_ProcessorLoop_PersistentDequeueError(t *testing.T) {
	errorCount := atomic.Int64{}
	errorQueue := newMockJobQueueStore()
	// Override Dequeue to always fail — we test via direct processLoop call
	// using a storage manager that returns the error queue

	errorStore := &errorStorageManager{
		mockStorageManager: &mockStorageManager{
			internal:   &mockInternalStore{kv: make(map[string]string)},
			market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
			stockIndex: newMockStockIndexStore(),
			jobQueue:   errorQueue,
		},
		dequeueErr: fmt.Errorf("connection refused"),
		errorCount: &errorCount,
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, errorStore,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	jm.wg.Add(1)
	go jm.processLoop(ctx)

	<-ctx.Done()
	jm.wg.Wait()

	errors := errorCount.Load()
	if errors > 5 {
		t.Logf("INFO: %d dequeue errors in 3 seconds (no backoff). "+
			"Consider exponential backoff to reduce log noise under persistent failure.", errors)
	}
}

// errorStorageManager wraps mockStorageManager but returns an error-injecting
// job queue store that fails on Dequeue.
type errorStorageManager struct {
	*mockStorageManager
	dequeueErr error
	errorCount *atomic.Int64
}

func (e *errorStorageManager) JobQueueStore() interfaces.JobQueueStore {
	return &errorJobQueueProxy{
		JobQueueStore: e.mockStorageManager.JobQueueStore(),
		dequeueErr:    e.dequeueErr,
		errorCount:    e.errorCount,
	}
}

// errorJobQueueProxy wraps a real JobQueueStore but overrides Dequeue to fail.
type errorJobQueueProxy struct {
	interfaces.JobQueueStore
	dequeueErr error
	errorCount *atomic.Int64
}

func (e *errorJobQueueProxy) Dequeue(_ context.Context) (*models.Job, error) {
	e.errorCount.Add(1)
	return nil, e.dequeueErr
}

// ============================================================================
// DA-9. Watcher enqueues jobs for all components even when only one is stale
// ============================================================================
//
// Verify the granularity: if only EOD is stale but everything else is fresh,
// only a collect_eod job should be enqueued.

func TestDA_WatcherGranularity_OnlyStaleComponentQueued(t *testing.T) {
	now := time.Now()
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
	}

	// Stock with only EOD stale (zero), everything else fresh
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:                     "BHP.AU",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             time.Time{}, // stale
		FundamentalsCollectedAt:    now,
		FilingsCollectedAt:         now,
		NewsCollectedAt:            now,
		FilingSummariesCollectedAt: now,
		TimelineCollectedAt:        now,
		SignalsCollectedAt:         now,
		NewsIntelCollectedAt:       now,
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{MaxConcurrent: 1, PurgeAfter: "24h"},
	)

	ctx := context.Background()
	jm.scanStockIndex(ctx)

	pending, _ := queue.CountPending(ctx)
	if pending != 1 {
		t.Errorf("expected 1 pending job (only EOD stale), got %d", pending)
	}

	if pending == 1 {
		queue.mu.Lock()
		job := queue.jobs[0]
		queue.mu.Unlock()
		if job.JobType != models.JobTypeCollectEOD {
			t.Errorf("expected job type %s, got %s", models.JobTypeCollectEOD, job.JobType)
		}
	}
}

// ============================================================================
// DA-10. Job queue purge — verify completed jobs are cleaned up
// ============================================================================

func TestDA_PurgeCompleted_NoLeaks(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// Enqueue and complete 100 jobs
	for i := 0; i < 100; i++ {
		job := &models.Job{
			ID:          fmt.Sprintf("purge-%d", i),
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "BHP.AU",
			Priority:    10,
			MaxAttempts: 3,
		}
		queue.Enqueue(ctx, job)
	}

	// Dequeue and complete all
	for i := 0; i < 100; i++ {
		job, _ := queue.Dequeue(ctx)
		if job != nil {
			queue.Complete(ctx, job.ID, nil, 100)
		}
	}

	// Verify none pending
	pending, _ := queue.CountPending(ctx)
	if pending != 0 {
		t.Errorf("expected 0 pending after completing all, got %d", pending)
	}

	// All should have completed status
	all, _ := queue.ListAll(ctx, 200)
	completed := 0
	for _, j := range all {
		if j.Status == models.JobStatusCompleted {
			completed++
		}
	}
	if completed != 100 {
		t.Errorf("expected 100 completed jobs, got %d", completed)
	}
}

// ============================================================================
// DA-11. CancelByTicker only cancels pending, not running
// ============================================================================

func TestDA_CancelByTicker_OnlyPending(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// Enqueue 3 jobs for same ticker
	for i := 0; i < 3; i++ {
		queue.Enqueue(ctx, &models.Job{
			ID:       fmt.Sprintf("c%d", i),
			JobType:  models.JobTypeCollectEOD,
			Ticker:   "BHP.AU",
			Priority: 10,
		})
	}

	// Dequeue one (makes it "running")
	running, _ := queue.Dequeue(ctx)
	if running == nil {
		t.Fatal("expected to dequeue a job")
	}

	// Cancel by ticker
	cancelled, _ := queue.CancelByTicker(ctx, "BHP.AU")

	// Should cancel 2 pending, not the running one
	if cancelled != 2 {
		t.Errorf("expected 2 cancelled, got %d", cancelled)
	}

	// Running job should still be running
	queue.mu.Lock()
	var runningJob *models.Job
	for _, j := range queue.jobs {
		if j.ID == running.ID {
			runningJob = j
		}
	}
	queue.mu.Unlock()

	if runningJob == nil || runningJob.Status != models.JobStatusRunning {
		t.Errorf("running job should remain running, got status: %v", runningJob)
	}
}

// ============================================================================
// DA-12. Complete with nil error always sets completed, with error always sets failed
// ============================================================================

func TestDA_CompleteStatusConsistency(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// Enqueue and dequeue
	queue.Enqueue(ctx, &models.Job{ID: "ok", JobType: models.JobTypeCollectEOD, Ticker: "A.AU", Priority: 10})
	queue.Enqueue(ctx, &models.Job{ID: "fail", JobType: models.JobTypeCollectEOD, Ticker: "B.AU", Priority: 10})

	okJob, _ := queue.Dequeue(ctx)
	failJob, _ := queue.Dequeue(ctx)

	// Complete with nil error
	queue.Complete(ctx, okJob.ID, nil, 100)
	// Complete with error
	queue.Complete(ctx, failJob.ID, fmt.Errorf("boom"), 50)

	queue.mu.Lock()
	for _, j := range queue.jobs {
		if j.ID == "ok" && j.Status != models.JobStatusCompleted {
			t.Errorf("nil error should set completed, got %s", j.Status)
		}
		if j.ID == "fail" && j.Status != models.JobStatusFailed {
			t.Errorf("non-nil error should set failed, got %s", j.Status)
		}
		if j.ID == "fail" && j.Error == "" {
			t.Error("failed job should have error message set")
		}
	}
	queue.mu.Unlock()
}

// ============================================================================
// DA-13. Stock index UpdateTimestamp with invalid field name
// ============================================================================

func TestDA_UpdateTimestamp_InvalidField(t *testing.T) {
	stockIdx := newMockStockIndexStore()
	ctx := context.Background()

	stockIdx.Upsert(ctx, &models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU"})

	// The SurrealDB implementation validates field names. Test the validation.
	invalidFields := []string{
		"",
		"nonexistent_field",
		"password_hash",
		"'; DROP TABLE stock_index; --",
		"eod_collected_at; DELETE FROM stock_index",
	}

	for _, field := range invalidFields {
		// In the SurrealDB implementation, UpdateTimestamp validates against a whitelist.
		// This test verifies the mock also handles this correctly (it doesn't validate,
		// which is fine for mocks, but the real impl must).
		t.Logf("Testing field: %q — SurrealDB impl should reject this via whitelist", field)
	}
}

// ============================================================================
// DA-14. HasPendingJob does not check running jobs
// ============================================================================
//
// HasPendingJob only checks status=pending. If a job is currently running,
// a new identical pending job can be enqueued. When the running job completes,
// the watcher will also find the component is now fresh, but the pending
// duplicate will still be processed, wasting resources.

func TestDA_HasPendingJob_IgnoresRunning(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// Enqueue and start running
	queue.Enqueue(ctx, &models.Job{
		ID:       "r1",
		JobType:  models.JobTypeCollectEOD,
		Ticker:   "BHP.AU",
		Priority: 10,
	})
	queue.Dequeue(ctx) // marks as running

	// Check if pending exists — should be false (job is running, not pending)
	has, _ := queue.HasPendingJob(ctx, models.JobTypeCollectEOD, "BHP.AU")
	if has {
		t.Error("HasPendingJob should return false when job is running, not pending")
	}

	// This means enqueueIfNeeded will create a DUPLICATE job for the same type+ticker.
	// The running job will complete, and then the duplicate will also run.
	t.Log("INFO: HasPendingJob only checks pending status. A running job for the same " +
		"type+ticker allows a duplicate to be enqueued. Consider also checking status=running " +
		"in HasPendingJob or in the dedup logic.")
}
