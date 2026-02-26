package jobmanager

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// Reuses mocks from manager_test.go (same package).

// ============================================================================
// computeSignals error return: Bug fix — return error when EOD is missing
// ============================================================================

// TestComputeSignals_NilMarketData verifies that computeSignals returns an error
// (not nil) when GetMarketData returns no data for the ticker.
// Before the fix: returned nil, causing updateStockIndexTimestamp to mark signals
// as fresh, preventing retries.
// After the fix: returns a descriptive error.
func TestComputeSignals_NilMarketData(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
		signals:    newMockSignalStorage(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()
	err := jm.computeSignals(ctx, "BHP.AU")

	if err == nil {
		t.Error("computeSignals should return an error when market data is not found, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// TestComputeSignals_NilEODSlice verifies that computeSignals returns an error
// when market data exists but EOD slice is nil.
func TestComputeSignals_NilEODSlice(t *testing.T) {
	market := &mockMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				// EOD field not set — defaults to nil slice
			},
		},
	}
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     market,
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
		signals:    newMockSignalStorage(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()
	err := jm.computeSignals(ctx, "BHP.AU")

	if err == nil {
		t.Error("computeSignals should return an error when EOD slice is nil, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// TestComputeSignals_EmptyEODSlice verifies that computeSignals returns an error
// when market data exists but EOD slice is empty.
func TestComputeSignals_EmptyEODSlice(t *testing.T) {
	market := &mockMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD:    []models.EODBar{}, // empty EOD slice
			},
		},
	}
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     market,
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
		signals:    newMockSignalStorage(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()
	err := jm.computeSignals(ctx, "BHP.AU")

	if err == nil {
		t.Error("computeSignals should return an error when EOD slice is empty, got nil")
	} else {
		t.Logf("Got expected error: %v", err)
	}
}

// TestComputeSignals_WithEODData verifies that computeSignals succeeds (returns nil)
// when market data with non-empty EOD is available.
func TestComputeSignals_WithEODData(t *testing.T) {
	market := &mockMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {
				Ticker: "BHP.AU",
				EOD: []models.EODBar{
					{Date: time.Now(), Open: 40.0, High: 41.0, Low: 39.5, Close: 40.5, Volume: 1000000},
				},
			},
		},
	}
	signals := newMockSignalStorage()
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     market,
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
		signals:    signals,
	}

	// mockSignalService.ComputeSignals returns a TickerSignals (happy path)
	sigSvc := &mockSignalService{
		computeFn: func(ctx context.Context, ticker string, md *models.MarketData) (*models.TickerSignals, error) {
			return &models.TickerSignals{Ticker: ticker}, nil
		},
	}

	jm := NewJobManager(
		newMockMarketService(), sigSvc, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()
	err := jm.computeSignals(ctx, "BHP.AU")

	if err != nil {
		t.Errorf("computeSignals should succeed when EOD data is present, got: %v", err)
	}
}

// TestComputeSignals_ErrorMessageContainsTicker verifies the error message includes
// the ticker for diagnostics.
func TestComputeSignals_ErrorMessageContainsTicker(t *testing.T) {
	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: newMockStockIndexStore(),
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
		signals:    newMockSignalStorage(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()
	err := jm.computeSignals(ctx, "CBA.AU")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error message should be descriptive (either contain ticker or "market data" or "EOD")
	msg := err.Error()
	if !strings.Contains(msg, "CBA.AU") && !strings.Contains(msg, "market data") && !strings.Contains(msg, "EOD") {
		t.Errorf("error message should reference ticker or data, got: %q", msg)
	}
}

// ============================================================================
// Watcher skip logic: Bug fix — skip compute_signals when EODCollectedAt is zero
// ============================================================================

// TestEnqueueStaleJobs_SkipsSignalsWhenNoEOD verifies that enqueueStaleJobs
// does NOT enqueue a compute_signals job for a ticker that has never had EOD collected.
// Before the fix: the signals job was enqueued regardless, leading to repeated failures.
// After the fix: signals job is skipped when EODCollectedAt is zero.
func TestEnqueueStaleJobs_SkipsSignalsWhenNoEOD(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	logger := common.NewLogger("error")
	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		logger,
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()

	// Stock entry with zero EODCollectedAt (EOD never collected)
	entry := &models.StockIndexEntry{
		Ticker:             "BHP.AU",
		Code:               "BHP",
		Exchange:           "AU",
		AddedAt:            time.Now().Add(-2 * time.Hour), // Not a new stock
		EODCollectedAt:     time.Time{},                    // Zero — never collected
		SignalsCollectedAt: time.Time{},                    // Also zero — stale by default
	}

	n, _ := jm.enqueueStaleJobs(ctx, entry)
	t.Logf("Enqueued %d jobs", n)

	// Verify that compute_signals was NOT enqueued
	hasSignalsJob := false
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeComputeSignals && j.Ticker == "BHP.AU" {
			hasSignalsJob = true
		}
	}

	if hasSignalsJob {
		t.Error("compute_signals should NOT be enqueued when EODCollectedAt is zero")
	} else {
		t.Log("Correctly skipped compute_signals for ticker with no EOD data")
	}
}

// TestEnqueueStaleJobs_EnqueuesSignalsWhenEODExists verifies that enqueueStaleJobs
// DOES enqueue a compute_signals job when EOD data has been collected but signals are stale.
func TestEnqueueStaleJobs_EnqueuesSignalsWhenEODExists(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	logger := common.NewLogger("error")
	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		logger,
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()

	// Stock entry with recent EOD collection but stale signals
	entry := &models.StockIndexEntry{
		Ticker:             "BHP.AU",
		Code:               "BHP",
		Exchange:           "AU",
		AddedAt:            time.Now().Add(-24 * time.Hour),   // Old stock
		EODCollectedAt:     time.Now().Add(-30 * time.Minute), // Fresh EOD
		SignalsCollectedAt: time.Time{},                       // Stale signals
	}

	n, _ := jm.enqueueStaleJobs(ctx, entry)
	t.Logf("Enqueued %d jobs", n)

	// Verify that compute_signals WAS enqueued
	hasSignalsJob := false
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeComputeSignals && j.Ticker == "BHP.AU" {
			hasSignalsJob = true
		}
	}

	if !hasSignalsJob {
		t.Error("compute_signals SHOULD be enqueued when EOD data exists and signals are stale")
	} else {
		t.Log("Correctly enqueued compute_signals for ticker with EOD data")
	}
}

// TestEnqueueStaleJobs_SignalsNotDoubleEnqueued verifies that when signals are fresh,
// they are NOT re-enqueued even when EOD is present.
func TestEnqueueStaleJobs_SignalsNotDoubleEnqueued(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	logger := common.NewLogger("error")
	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		logger,
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()

	// Stock with fresh EOD and fresh signals
	entry := &models.StockIndexEntry{
		Ticker:             "BHP.AU",
		Code:               "BHP",
		Exchange:           "AU",
		AddedAt:            time.Now().Add(-24 * time.Hour),
		EODCollectedAt:     time.Now().Add(-30 * time.Minute),
		SignalsCollectedAt: time.Now().Add(-15 * time.Minute), // Recent signals
	}

	jm.enqueueStaleJobs(ctx, entry)

	// Count signals jobs
	signalsCount := 0
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeComputeSignals && j.Ticker == "BHP.AU" {
			signalsCount++
		}
	}

	if signalsCount > 0 {
		t.Errorf("compute_signals should NOT be enqueued when signals are fresh, got %d jobs", signalsCount)
	}
}

// TestEnqueueStaleJobs_NewStock_NoEOD verifies that a new stock (just added, no EOD yet)
// does not enqueue compute_signals.
func TestEnqueueStaleJobs_NewStock_NoEOD(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
	}

	logger := common.NewLogger("error")
	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		logger,
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	ctx := context.Background()

	// Brand new stock: added 1 minute ago, no EOD collected
	entry := &models.StockIndexEntry{
		Ticker:         "NEW.AU",
		Code:           "NEW",
		Exchange:       "AU",
		AddedAt:        time.Now().Add(-1 * time.Minute), // Just added
		EODCollectedAt: time.Time{},                      // Never collected
	}

	jm.enqueueStaleJobs(ctx, entry)

	// compute_signals should not be enqueued
	for _, j := range queue.jobs {
		if j.JobType == models.JobTypeComputeSignals && j.Ticker == "NEW.AU" {
			t.Error("compute_signals should NOT be enqueued for a brand new stock with no EOD")
		}
	}
}

// ============================================================================
// Verify signals timestamp is NOT updated on computeSignals failure
// ============================================================================

// TestComputeSignals_FailureDoesNotUpdateTimestamp verifies that when computeSignals
// returns an error, updateStockIndexTimestamp is not called for the signals field.
// This ensures the job system correctly marks the job as failed (retryable).
func TestComputeSignals_FailureDoesNotUpdateTimestamp(t *testing.T) {
	stockIdx := newMockStockIndexStore()
	ctx := context.Background()

	// Pre-insert a stock index entry with zero signals timestamp
	entry := &models.StockIndexEntry{
		Ticker:             "BHP.AU",
		Code:               "BHP",
		Exchange:           "AU",
		AddedAt:            time.Now().Add(-1 * time.Hour),
		EODCollectedAt:     time.Time{}, // No EOD
		SignalsCollectedAt: time.Time{}, // No signals
	}
	stockIdx.Upsert(ctx, entry)

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   newMockJobQueueStore(),
		files:      newMockFileStore(),
		signals:    newMockSignalStorage(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	// Call computeSignals — should fail because no EOD data
	err := jm.computeSignals(ctx, "BHP.AU")
	if err == nil {
		t.Fatal("expected computeSignals to return error (no EOD data)")
	}

	// Now check: if we only call computeSignals (not processLoop), the timestamp
	// in the stock index should remain zero — NOT updated to now.
	// Note: updateStockIndexTimestamp is called by processLoop after executeJob,
	// only when err == nil. So if computeSignals returns error, timestamp stays zero.
	// We verify this by checking the stock index entry directly.
	after, fetchErr := stockIdx.Get(ctx, "BHP.AU")
	if fetchErr != nil {
		t.Fatalf("failed to get stock index entry: %v", fetchErr)
	}

	if !after.SignalsCollectedAt.IsZero() {
		t.Errorf("SignalsCollectedAt should remain zero after computeSignals failure, got: %v", after.SignalsCollectedAt)
	} else {
		t.Log("SignalsCollectedAt correctly remains zero after computeSignals failure")
	}
}

// ============================================================================
// processLoop integration: failed computeSignals job must NOT update timestamp
// ============================================================================

// TestProcessLoop_ComputeSignals_FailureNoTimestamp verifies that when a
// compute_signals job fails (due to missing EOD), the stock index timestamp
// is NOT updated (the job stays retryable).
func TestProcessLoop_ComputeSignals_FailureNoTimestamp(t *testing.T) {
	queue := newMockJobQueueStore()
	stockIdx := newMockStockIndexStore()
	ctx := context.Background()

	// Insert stock entry with no EOD
	entry := &models.StockIndexEntry{
		Ticker:             "BHP.AU",
		Code:               "BHP",
		Exchange:           "AU",
		AddedAt:            time.Now().Add(-1 * time.Hour),
		EODCollectedAt:     time.Time{},
		SignalsCollectedAt: time.Time{},
	}
	stockIdx.Upsert(ctx, entry)

	// Enqueue a compute_signals job
	queue.Enqueue(ctx, &models.Job{
		ID:          "signals-test",
		JobType:     models.JobTypeComputeSignals,
		Ticker:      "BHP.AU",
		Priority:    5,
		MaxAttempts: 1, // Single attempt for test speed
	})

	store := &mockStorageManager{
		internal:   &mockInternalStore{kv: make(map[string]string)},
		market:     &mockMarketDataStorage{data: make(map[string]*models.MarketData)},
		stockIndex: stockIdx,
		jobQueue:   queue,
		files:      newMockFileStore(),
		signals:    newMockSignalStorage(),
	}

	jm := NewJobManager(
		newMockMarketService(), &mockSignalService{}, store,
		common.NewLogger("error"),
		common.JobManagerConfig{WatcherInterval: "1h", MaxConcurrent: 1},
	)

	// Run the process loop briefly to pick up and fail the job
	jmCtx, jmCancel := context.WithCancel(context.Background())
	jm.cancel = jmCancel
	jm.wg.Add(1)
	go func() { defer jm.wg.Done(); jm.processLoop(jmCtx) }()

	// Wait for the job to be processed
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		queue.mu.Lock()
		var job *models.Job
		for _, j := range queue.jobs {
			if j.ID == "signals-test" {
				job = j
			}
		}
		queue.mu.Unlock()

		if job != nil && (job.Status == models.JobStatusFailed || job.Status == models.JobStatusCompleted) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	jmCancel()
	jm.wg.Wait()

	// Verify the job failed (not completed)
	queue.mu.Lock()
	var job *models.Job
	for _, j := range queue.jobs {
		if j.ID == "signals-test" {
			job = j
		}
	}
	queue.mu.Unlock()

	if job == nil {
		t.Fatal("job not found in queue")
	}

	if job.Status == models.JobStatusCompleted {
		t.Error("compute_signals job should have FAILED (not completed) when EOD data is missing")
	} else {
		t.Logf("Job correctly marked as %s", job.Status)
	}

	// Also verify that the signals timestamp was NOT updated
	after, err := stockIdx.Get(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("failed to get stock index entry: %v", err)
	}

	if !after.SignalsCollectedAt.IsZero() {
		t.Errorf("SignalsCollectedAt should be zero after job failure, got: %v", after.SignalsCollectedAt)
	} else {
		t.Log("SignalsCollectedAt correctly remains zero after job failure")
	}
}
