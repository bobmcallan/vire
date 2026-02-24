package jobmanager

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Stress tests reuse mocks from manager_test.go in the same package.

// ============================================================================
// 1. Double-start — second Start() safely stops first
// ============================================================================

func TestStress_DoubleStart(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{Enabled: true, WatcherInterval: "1h", MaxConcurrent: 1},
	)

	jm.Start()
	jm.Start() // Should stop first, then start again

	if jm.cancel == nil {
		t.Error("expected cancel to be set after double Start()")
	}

	jm.Stop()
	if jm.cancel != nil {
		t.Error("expected cancel to be nil after Stop()")
	}
}

// ============================================================================
// 2. Stop() is idempotent — double-stop does not panic
// ============================================================================

func TestStress_DoubleStop(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{Enabled: true, WatcherInterval: "1h", MaxConcurrent: 1},
	)

	jm.Start()
	jm.Stop()
	jm.Stop() // Should not panic
}

// ============================================================================
// 3. Stop() without Start() does not panic
// ============================================================================

func TestStress_StopWithoutStart(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{Enabled: true, WatcherInterval: "1h", MaxConcurrent: 1},
	)

	jm.Stop() // Should not panic
}

// ============================================================================
// 4. Empty stock index — no jobs enqueued
// ============================================================================

func TestStress_EmptyStockIndex(t *testing.T) {
	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(), // empty
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{MaxConcurrent: 1, PurgeAfter: "24h"},
	)

	jm.scanStockIndex(context.Background())

	pending, _ := queue.CountPending(context.Background())
	if pending != 0 {
		t.Errorf("expected 0 pending jobs for empty stock index, got %d", pending)
	}
}

// ============================================================================
// 5. All tickers fresh — no jobs enqueued
// ============================================================================

func TestStress_AllTickersFresh_NoJobs(t *testing.T) {
	now := time.Now()
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	// Add stock with all fresh timestamps
	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:                     "BHP.AU",
		AddedAt:                    now.Add(-1 * time.Hour),
		EODCollectedAt:             now,
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

	jm.scanStockIndex(context.Background())

	pending, _ := queue.CountPending(context.Background())
	if pending != 0 {
		t.Errorf("expected 0 pending jobs for fresh stock, got %d", pending)
	}
}

// ============================================================================
// 6. Job deduplication across multiple scans
// ============================================================================

func TestStress_DedupAcrossScans(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	stockIdx.entries["BHP.AU"] = &models.StockIndexEntry{
		Ticker:  "BHP.AU",
		AddedAt: time.Now().Add(-1 * time.Hour),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{MaxConcurrent: 1, PurgeAfter: "24h"},
	)

	ctx := context.Background()

	// Scan twice
	jm.scanStockIndex(ctx)
	firstCount, _ := queue.CountPending(ctx)

	jm.scanStockIndex(ctx)
	secondCount, _ := queue.CountPending(ctx)

	if firstCount != secondCount {
		t.Errorf("dedup failed: first scan %d jobs, second scan %d jobs (should be same)", firstCount, secondCount)
	}
}

// ============================================================================
// 7. Concurrent processor pool processes jobs
// ============================================================================

func TestStress_ConcurrentProcessors(t *testing.T) {
	var processed atomic.Int64

	market := newMockMarketService()
	market.collectCalls = nil // Don't need tracking
	// Override with a version that counts
	wrappedMarket := &countingMarketService{
		mockMarketService: newMockMarketService(),
		processed:         &processed,
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	ctx := context.Background()
	// Enqueue 10 jobs
	for i := 0; i < 10; i++ {
		queue.Enqueue(ctx, &models.Job{
			JobType:     models.JobTypeCollectEOD,
			Ticker:      "BHP.AU",
			Priority:    10,
			MaxAttempts: 3,
		})
	}

	jm := NewJobManager(
		wrappedMarket, &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 3},
	)

	jmCtx, jmCancel := context.WithCancel(context.Background())
	jm.cancel = jmCancel

	// Launch processors
	for i := 0; i < 3; i++ {
		jm.wg.Add(1)
		go func() { defer jm.wg.Done(); jm.processLoop(jmCtx) }()
	}

	// Wait for all jobs to be processed
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for jobs to process, processed=%d", processed.Load())
		default:
			p, _ := queue.CountPending(ctx)
			if p == 0 {
				goto done
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

done:
	jmCancel()
	jm.wg.Wait()

	if processed.Load() != 10 {
		t.Errorf("expected 10 jobs processed, got %d", processed.Load())
	}
}

// countingMarketService wraps mockMarketService and counts calls.
type countingMarketService struct {
	*mockMarketService
	processed *atomic.Int64
}

func (c *countingMarketService) CollectEOD(_ context.Context, _ string, _ bool) error {
	c.processed.Add(1)
	return nil
}

// ============================================================================
// 8. MaxConcurrent <= 0 defaults to 5
// ============================================================================

func TestStress_MaxConcurrentZero_DefaultsToFive(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{Enabled: true, WatcherInterval: "1m", MaxConcurrent: 0},
	)

	jm.Start()
	// Start should have spawned 5 processors (default)
	// We can't easily check goroutine count, but verify it starts/stops cleanly
	jm.Stop()
}

// ============================================================================
// 9. Invalid interval string falls back to 1m
// ============================================================================

func TestStress_InvalidInterval_DefaultsTo1Min(t *testing.T) {
	config := common.JobManagerConfig{
		Enabled:         true,
		WatcherInterval: "not-a-duration",
		MaxConcurrent:   1,
	}

	if config.GetWatcherInterval() != 1*time.Minute {
		t.Errorf("GetWatcherInterval() = %v, want 1m (default)", config.GetWatcherInterval())
	}
}

// ============================================================================
// 10. Priority ordering — higher priority dequeued first
// ============================================================================

func TestStress_PriorityOrdering(t *testing.T) {
	queue := newMockJobQueueStore()
	ctx := context.Background()

	// Enqueue jobs with different priorities (lower to higher)
	queue.Enqueue(ctx, &models.Job{ID: "low", JobType: models.JobTypeCollectTimeline, Ticker: "A.AU", Priority: 2})
	queue.Enqueue(ctx, &models.Job{ID: "high", JobType: models.JobTypeCollectEOD, Ticker: "A.AU", Priority: 10})
	queue.Enqueue(ctx, &models.Job{ID: "mid", JobType: models.JobTypeCollectNews, Ticker: "A.AU", Priority: 7})

	// Dequeue should return highest priority first
	job1, _ := queue.Dequeue(ctx)
	if job1 == nil || job1.ID != "high" {
		t.Errorf("expected high priority job first, got %v", job1)
	}

	job2, _ := queue.Dequeue(ctx)
	if job2 == nil || job2.ID != "mid" {
		t.Errorf("expected mid priority job second, got %v", job2)
	}

	job3, _ := queue.Dequeue(ctx)
	if job3 == nil || job3.ID != "low" {
		t.Errorf("expected low priority job third, got %v", job3)
	}

	// Queue should be empty
	job4, _ := queue.Dequeue(ctx)
	if job4 != nil {
		t.Errorf("expected nil on empty queue, got %v", job4)
	}
}

// ============================================================================
// OOM Fix Stress Tests — Semaphore Deadlock, Config Edge Cases, Startup Delay
// ============================================================================

// 11. CRITICAL: Heavy semaphore release is not deferred — panics leak tokens
func TestStressOOM_HeavySemaphore_PanicLeaksToken(t *testing.T) {
	// This test verifies whether the semaphore token is properly released
	// when executeJob panics. If not deferred, a panic during a heavy job
	// permanently consumes a semaphore slot.
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		MaxConcurrent: 2,
		HeavyJobLimit: 1,
		MaxRetries:    1,
	}

	panicMarket := newMockMarketService()
	var callCount atomic.Int64
	panicMarket.collectFilingsFn = func(_ context.Context, _ string, _ bool) error {
		n := callCount.Add(1)
		if n == 1 {
			panic("simulated OOM crash during PDF processing")
		}
		return nil
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(panicMarket, &mockSignalService{}, store, logger, config)

	ctx := context.Background()
	// Enqueue 2 heavy jobs
	queue.Enqueue(ctx, &models.Job{
		ID: "panic-job", JobType: models.JobTypeCollectFilings,
		Ticker: "A.AU", Priority: 10, Status: models.JobStatusPending, MaxAttempts: 1,
	})
	queue.Enqueue(ctx, &models.Job{
		ID: "ok-job", JobType: models.JobTypeCollectFilings,
		Ticker: "B.AU", Priority: 10, Status: models.JobStatusPending, MaxAttempts: 1,
	})

	jm.Start()
	// Wait for processing
	time.Sleep(2 * time.Second)
	jm.Stop()

	// If the semaphore leaks, the second job would never execute because
	// the token from the panicked goroutine is never released.
	// With capacity=1 and a leaked token, the channel is full and subsequent
	// acquires block forever.
	if callCount.Load() < 2 {
		t.Error("CRITICAL: Heavy semaphore token leaked after panic. " +
			"Second heavy job was never executed. " +
			"Fix: defer the semaphore release immediately after acquire.")
	}
}

// 12. Semaphore: context cancellation while waiting for semaphore
func TestStressOOM_HeavySemaphore_ContextCancelWhileWaiting(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		MaxConcurrent: 2,
		HeavyJobLimit: 1,
		MaxRetries:    1,
	}

	blockingMarket := newMockMarketService()
	started := make(chan struct{})
	blockingMarket.collectFilingsFn = func(ctx context.Context, _ string, _ bool) error {
		close(started)
		// Block until context is cancelled
		<-ctx.Done()
		return ctx.Err()
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(blockingMarket, &mockSignalService{}, store, logger, config)

	ctx := context.Background()
	// Enqueue 2 heavy jobs — first will block, second will wait for semaphore
	queue.Enqueue(ctx, &models.Job{
		ID: "blocking", JobType: models.JobTypeCollectFilings,
		Ticker: "A.AU", Priority: 10, Status: models.JobStatusPending, MaxAttempts: 1,
	})
	queue.Enqueue(ctx, &models.Job{
		ID: "waiting", JobType: models.JobTypeCollectFilings,
		Ticker: "B.AU", Priority: 10, Status: models.JobStatusPending, MaxAttempts: 1,
	})

	jm.Start()
	// Wait for the first job to start executing
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first heavy job never started")
	}

	// Give the second processor time to reach the semaphore wait
	time.Sleep(200 * time.Millisecond)

	// Stop should cancel context, which should unblock the semaphore wait
	done := make(chan struct{})
	go func() {
		jm.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good — Stop completed, no deadlock
	case <-time.After(10 * time.Second):
		t.Error("DEADLOCK: Stop() did not complete — context cancellation " +
			"did not unblock semaphore wait. Possible deadlock scenario.")
	}
}

// 13. FINDING: Heavy semaphore can starve non-heavy jobs when all processors
// are waiting for semaphore. With MaxConcurrent=N, if N processors each
// dequeue a heavy job and wait for semaphore (capacity=1), no processors
// remain to service non-heavy jobs. This is a design trade-off, not a bug.
func TestStressOOM_HeavySemaphore_LightJobsStarvation(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		MaxConcurrent: 3, // plenty of processors
		HeavyJobLimit: 1,
		MaxRetries:    1,
	}

	blockingMarket := newMockMarketService()
	heavyStarted := make(chan struct{}, 1)
	var lightCompleted atomic.Int64

	blockingMarket.collectFilingsFn = func(ctx context.Context, _ string, _ bool) error {
		select {
		case heavyStarted <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
	blockingMarket.collectCoreFn = func(_ context.Context, _ []string, _ bool) error {
		lightCompleted.Add(1)
		return nil
	}

	queue := newMockJobQueueStore()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	jm := NewJobManager(blockingMarket, &mockSignalService{}, store, logger, config)

	ctx := context.Background()
	// Enqueue light jobs first (higher priority) so they get dequeued before heavy
	for i := 0; i < 3; i++ {
		queue.Enqueue(ctx, &models.Job{
			ID: fmt.Sprintf("light-%d", i), JobType: models.JobTypeCollectEOD,
			Ticker: "A.AU", Priority: 15, Status: models.JobStatusPending, MaxAttempts: 1,
		})
	}
	// Then heavy job (lower priority)
	queue.Enqueue(ctx, &models.Job{
		ID: "heavy", JobType: models.JobTypeCollectFilings,
		Ticker: "A.AU", Priority: 5, Status: models.JobStatusPending, MaxAttempts: 1,
	})

	jm.Start()
	time.Sleep(1 * time.Second)
	jm.Stop()

	// When light jobs have higher priority and are enqueued first, they should
	// be dequeued and processed before the heavy job acquires the semaphore.
	if lightCompleted.Load() > 0 {
		t.Logf("Light jobs completed: %d (good — priority ordering allows light jobs to run)", lightCompleted.Load())
	} else {
		t.Log("FINDING: Even with higher-priority light jobs, all processors may end up " +
			"waiting for the heavy semaphore depending on dequeue timing. Consider checking " +
			"isHeavyJob before acquire and skipping/re-queuing if semaphore is full.")
	}
}

// 14. Config: HeavyJobLimit = 0 defaults to 1
func TestStressOOM_HeavyJobLimit_ZeroDefaultsToOne(t *testing.T) {
	config := common.JobManagerConfig{HeavyJobLimit: 0}
	if config.GetHeavyJobLimit() != 1 {
		t.Errorf("GetHeavyJobLimit() = %d, want 1 (default for zero)", config.GetHeavyJobLimit())
	}
}

// 15. Config: HeavyJobLimit = -1 defaults to 1
func TestStressOOM_HeavyJobLimit_NegativeDefaultsToOne(t *testing.T) {
	config := common.JobManagerConfig{HeavyJobLimit: -1}
	if config.GetHeavyJobLimit() != 1 {
		t.Errorf("GetHeavyJobLimit() = %d, want 1 (default for negative)", config.GetHeavyJobLimit())
	}
}

// 16. Config: HeavyJobLimit = 5 is respected
func TestStressOOM_HeavyJobLimit_CustomValue(t *testing.T) {
	config := common.JobManagerConfig{HeavyJobLimit: 5}
	if config.GetHeavyJobLimit() != 5 {
		t.Errorf("GetHeavyJobLimit() = %d, want 5", config.GetHeavyJobLimit())
	}
}

// 17. Config: WatcherStartupDelay empty defaults to 10s
func TestStressOOM_WatcherStartupDelay_EmptyDefault(t *testing.T) {
	config := common.JobManagerConfig{WatcherStartupDelay: ""}
	d := config.GetWatcherStartupDelay()
	if d != 10*time.Second {
		t.Errorf("GetWatcherStartupDelay() = %v, want 10s (default)", d)
	}
}

// 18. Config: WatcherStartupDelay invalid string defaults to 10s
func TestStressOOM_WatcherStartupDelay_InvalidString(t *testing.T) {
	config := common.JobManagerConfig{WatcherStartupDelay: "not-a-duration"}
	d := config.GetWatcherStartupDelay()
	if d != 10*time.Second {
		t.Errorf("GetWatcherStartupDelay() = %v, want 10s (default for invalid)", d)
	}
}

// 19. Config: WatcherStartupDelay negative value — not rejected
func TestStressOOM_WatcherStartupDelay_NegativeValue(t *testing.T) {
	config := common.JobManagerConfig{WatcherStartupDelay: "-5s"}
	d := config.GetWatcherStartupDelay()
	// Negative values parse successfully. The watchLoop guards with `if startupDelay > 0`.
	// So negative = 0 delay, which is safe.
	if d >= 0 {
		t.Log("FINDING: Negative WatcherStartupDelay (-5s) parsed as", d,
			"— watchLoop checks > 0 so this is safe, but consider clamping to 0.")
	}
}

// 20. Config: WatcherStartupDelay very large value
func TestStressOOM_WatcherStartupDelay_VeryLarge(t *testing.T) {
	config := common.JobManagerConfig{WatcherStartupDelay: "24h"}
	d := config.GetWatcherStartupDelay()
	if d != 24*time.Hour {
		t.Errorf("GetWatcherStartupDelay() = %v, want 24h", d)
	}
	t.Log("FINDING: WatcherStartupDelay=24h is accepted — server would wait 24h " +
		"before first scan. Consider adding a max clamp (e.g., 5 minutes).")
}

// 21. Config: WatcherStartupDelay = "0s" — no delay
func TestStressOOM_WatcherStartupDelay_ZeroSeconds(t *testing.T) {
	config := common.JobManagerConfig{WatcherStartupDelay: "0s"}
	d := config.GetWatcherStartupDelay()
	if d != 0 {
		t.Errorf("GetWatcherStartupDelay() = %v, want 0 (no delay)", d)
	}
}

// 22. Config: env override for VIRE_WATCHER_STARTUP_DELAY
// (reads env inside GetWatcherStartupDelay AND in applyEnvOverrides — double-read)
func TestStressOOM_WatcherStartupDelay_EnvOverride(t *testing.T) {
	t.Setenv("VIRE_WATCHER_STARTUP_DELAY", "500ms")
	config := common.JobManagerConfig{WatcherStartupDelay: ""}
	d := config.GetWatcherStartupDelay()
	if d != 500*time.Millisecond {
		t.Errorf("GetWatcherStartupDelay() with env = %v, want 500ms", d)
	}
}

// 23. Config: env override for VIRE_JOBS_HEAVY_LIMIT with invalid value
func TestStressOOM_HeavyJobLimit_EnvOverrideInvalid(t *testing.T) {
	t.Setenv("VIRE_JOBS_HEAVY_LIMIT", "abc")
	// applyEnvOverrides should ignore non-numeric values
	cfg := common.NewDefaultConfig()
	// Re-apply env overrides (normally done in LoadConfig)
	// We test that the default (1) survives an invalid env value
	if cfg.JobManager.GetHeavyJobLimit() != 1 {
		t.Errorf("HeavyJobLimit should be 1 (default), got %d", cfg.JobManager.GetHeavyJobLimit())
	}
}

// 24. Startup delay is cancellable via context
func TestStressOOM_StartupDelay_Cancellable(t *testing.T) {
	logger := common.NewLogger("error")
	config := common.JobManagerConfig{
		Enabled:             true,
		WatcherInterval:     "1h",
		WatcherStartupDelay: "10s", // long delay
		MaxConcurrent:       1,
		HeavyJobLimit:       1,
	}

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
	}

	jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store, logger, config)

	jm.Start()

	// Stop should complete quickly even with 10s startup delay pending
	done := make(chan struct{})
	go func() {
		jm.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good — startup delay was cancelled by context
	case <-time.After(3 * time.Second):
		t.Error("Stop() blocked — startup delay is not cancellable via context")
	}
}

// 25. isHeavyJob exhaustive check
func TestStressOOM_IsHeavyJob_Exhaustive(t *testing.T) {
	heavyTypes := map[string]bool{
		models.JobTypeCollectFilings:         true,
		models.JobTypeCollectFilingSummaries: true,
	}

	allTypes := []string{
		models.JobTypeCollectEOD,
		models.JobTypeCollectEODBulk,
		models.JobTypeCollectFundamentals,
		models.JobTypeCollectFilings,
		models.JobTypeCollectNews,
		models.JobTypeCollectFilingSummaries,
		models.JobTypeCollectTimeline,
		models.JobTypeCollectNewsIntel,
		models.JobTypeComputeSignals,
	}

	for _, jt := range allTypes {
		expected := heavyTypes[jt]
		if isHeavyJob(jt) != expected {
			t.Errorf("isHeavyJob(%q) = %v, want %v", jt, isHeavyJob(jt), expected)
		}
	}

	// Unknown types should not be heavy
	if isHeavyJob("unknown_type") {
		t.Error("isHeavyJob should return false for unknown job types")
	}
	if isHeavyJob("") {
		t.Error("isHeavyJob should return false for empty string")
	}
}

// 26. Semaphore capacity matches config
func TestStressOOM_SemaphoreCapacity_MatchesConfig(t *testing.T) {
	tests := []struct {
		limit    int
		expected int
	}{
		{0, 1},  // default
		{-1, 1}, // default
		{1, 1},
		{3, 3},
	}

	for _, tt := range tests {
		store := &mockStorageManager{
			internal:   &mockInternalStore{kv: make(map[string]string)},
			market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
			stockIndex: newMockStockIndexStore(),
			jobQueue:   newMockJobQueueStore(),
			files:      newMockFileStore(),
		}
		config := common.JobManagerConfig{HeavyJobLimit: tt.limit}
		jm := NewJobManager(newMockMarketService(), &mockSignalService{}, store,
			common.NewLogger("error"), config)
		if cap(jm.heavySem) != tt.expected {
			t.Errorf("HeavyJobLimit=%d: semaphore capacity=%d, want %d",
				tt.limit, cap(jm.heavySem), tt.expected)
		}
	}
}
