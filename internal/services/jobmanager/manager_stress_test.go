package jobmanager

import (
	"context"
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
