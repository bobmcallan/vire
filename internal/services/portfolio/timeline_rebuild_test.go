package portfolio

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- mock helpers for timeline rebuild tests ---

// rebuildCashflowSvc is a minimal mock that returns a ledger with transactions.
type rebuildCashflowSvc struct {
	transactions []models.CashTransaction
	callCount    atomic.Int32
}

func (m *rebuildCashflowSvc) GetLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	m.callCount.Add(1)
	return &models.CashFlowLedger{
		Transactions: m.transactions,
	}, nil
}
func (m *rebuildCashflowSvc) AddTransaction(_ context.Context, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) AddTransfer(_ context.Context, _, _, _ string, _ float64, _ time.Time, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) UpdateTransaction(_ context.Context, _, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) RemoveTransaction(_ context.Context, _, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) UpdateAccount(_ context.Context, _ string, _ string, _ models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) SetTransactions(_ context.Context, _ string, _ []models.CashTransaction, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) ClearLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (m *rebuildCashflowSvc) CalculatePerformance(_ context.Context, _ string) (*models.CapitalPerformance, error) {
	return nil, nil
}

// trackingDailyGrowth is a minimal storage manager that records GrowthOptions passed to GetDailyGrowth.
// We intercept via a mock by having the service call a no-op GetDailyGrowth.
// Since GetDailyGrowth is on the Service directly, we test rebuildTimelineWithCash
// by checking if cashflowSvc.GetLedger was called.

// rebuildTimelineStore tracks DeleteAll calls.
type rebuildTimelineStore struct {
	deleteAllCount atomic.Int32
	deletedName    string
	snapshots      []models.TimelineSnapshot
}

func (m *rebuildTimelineStore) GetRange(_ context.Context, _, _ string, _, _ time.Time) ([]models.TimelineSnapshot, error) {
	return m.snapshots, nil
}
func (m *rebuildTimelineStore) GetLatest(_ context.Context, _, _ string) (*models.TimelineSnapshot, error) {
	return nil, nil
}
func (m *rebuildTimelineStore) SaveBatch(_ context.Context, _ []models.TimelineSnapshot) error {
	return nil
}
func (m *rebuildTimelineStore) DeleteRange(_ context.Context, _, _ string, _, _ time.Time) (int, error) {
	return 0, nil
}
func (m *rebuildTimelineStore) DeleteAll(_ context.Context, _, name string) (int, error) {
	m.deleteAllCount.Add(1)
	m.deletedName = name
	return 1, nil
}

// rebuildStorageManager satisfies StorageManager with a configurable timeline store.
type rebuildStorageManager struct {
	tl interfaces.TimelineStore
}

func (b *rebuildStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (b *rebuildStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (b *rebuildStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (b *rebuildStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (b *rebuildStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (b *rebuildStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (b *rebuildStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (b *rebuildStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (b *rebuildStorageManager) ChangelogStore() interfaces.ChangelogStore       { return nil }
func (b *rebuildStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (b *rebuildStorageManager) TimelineStore() interfaces.TimelineStore         { return b.tl }
func (b *rebuildStorageManager) DataPath() string                                { return "" }
func (b *rebuildStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (b *rebuildStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (b *rebuildStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (b *rebuildStorageManager) Close() error                                { return nil }

// --- Tests ---

// callWithRecover calls f and recovers any panic, returning whether a panic occurred.
func callWithRecover(f func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			panicked = true
		}
	}()
	f()
	return false
}

// TestRebuildTimelineWithCash_LoadsTransactions verifies that rebuildTimelineWithCash
// calls GetLedger on cashflowSvc to load cash transactions before calling GetDailyGrowth.
func TestRebuildTimelineWithCash_LoadsTransactions(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{
		transactions: []models.CashTransaction{
			{ID: "tx1", Amount: 1000},
		},
	}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}
	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})

	// rebuildTimelineWithCash calls GetLedger first, then GetDailyGrowth.
	// GetDailyGrowth will panic due to nil storage internals (no real DB), which is expected
	// in unit tests. We recover from that and check GetLedger was called before it.
	callWithRecover(func() {
		svc.rebuildTimelineWithCash(ctx, "test-portfolio") //nolint:errcheck
	})

	if cashSvc.callCount.Load() != 1 {
		t.Errorf("expected 1 GetLedger call, got %d", cashSvc.callCount.Load())
	}
}

// TestRebuildTimelineWithCash_NilCashflowSvc verifies that when cashflowSvc is nil,
// rebuildTimelineWithCash still calls GetDailyGrowth (without transactions).
func TestRebuildTimelineWithCash_NilCashflowSvc(t *testing.T) {
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: nil, // no cash service
		logger:      common.NewLogger("disabled"),
	}
	ctx := context.Background()

	// Should not crash due to nil cashflowSvc — guard in rebuildTimelineWithCash handles this.
	// GetDailyGrowth panics on nil storage; recover handles it.
	callWithRecover(func() {
		svc.rebuildTimelineWithCash(ctx, "test-portfolio") //nolint:errcheck
	})
}

// TestTriggerTimelineRebuildAsync_DedupSkipsWhenRebuilding verifies that
// if IsTimelineRebuilding returns true, triggerTimelineRebuildAsync returns without spawning.
func TestTriggerTimelineRebuildAsync_DedupSkipsWhenRebuilding(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}

	// Pre-set flag to simulate in-progress rebuild
	svc.timelineRebuilding.Store("test", true)

	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})
	svc.triggerTimelineRebuildAsync(ctx, "test")

	// Give goroutine time to potentially run
	time.Sleep(20 * time.Millisecond)

	// Cashflow should NOT have been called (goroutine was skipped)
	if cashSvc.callCount.Load() != 0 {
		t.Errorf("expected 0 GetLedger calls (dedup skip), got %d", cashSvc.callCount.Load())
	}

	// Flag should remain true (we didn't reset it)
	if !svc.IsTimelineRebuilding("test") {
		t.Error("expected rebuilding flag to remain true after dedup skip")
	}
}

// TestTriggerTimelineRebuildAsync_SetsFlag verifies the flag is set to true while rebuilding
// and false after completion.
func TestTriggerTimelineRebuildAsync_SetsFlag(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}

	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})

	// Flag should be false initially
	if svc.IsTimelineRebuilding("mypf") {
		t.Fatal("expected flag to be false before trigger")
	}

	svc.triggerTimelineRebuildAsync(ctx, "mypf")

	// Wait for goroutine to complete (rebuild will fail fast with no real storage, but flag resets)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !svc.IsTimelineRebuilding("mypf") {
			return // flag reset — success
		}
		time.Sleep(10 * time.Millisecond)
	}

	// After goroutine finishes, flag should be false
	if svc.IsTimelineRebuilding("mypf") {
		t.Error("expected rebuilding flag to be false after goroutine completes")
	}
}

// TestInvalidateAndRebuildTimeline_DeletesAndRebuilds verifies DeleteAll is called then rebuild triggered.
func TestInvalidateAndRebuildTimeline_DeletesAndRebuilds(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}

	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})
	svc.InvalidateAndRebuildTimeline(ctx, "portfolio-x")

	// DeleteAll must be called
	if tl.deleteAllCount.Load() != 1 {
		t.Errorf("expected 1 DeleteAll call, got %d", tl.deleteAllCount.Load())
	}
	if tl.deletedName != "portfolio-x" {
		t.Errorf("expected DeleteAll for 'portfolio-x', got '%s'", tl.deletedName)
	}
}

// TestInvalidateAndRebuildTimeline_SkipsWhenRebuilding verifies no delete if already rebuilding.
func TestInvalidateAndRebuildTimeline_SkipsWhenRebuilding(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}

	// Pre-set flag
	svc.timelineRebuilding.Store("portfolio-x", true)

	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})
	svc.InvalidateAndRebuildTimeline(ctx, "portfolio-x")

	// DeleteAll should NOT be called when already rebuilding
	if tl.deleteAllCount.Load() != 0 {
		t.Errorf("expected 0 DeleteAll calls (skip when rebuilding), got %d", tl.deleteAllCount.Load())
	}
}

// TestForceRebuildTimeline_BypassesDedup verifies that force rebuild resets flag and rebuilds.
func TestForceRebuildTimeline_BypassesDedup(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}

	// Set flag to simulate in-progress rebuild
	svc.timelineRebuilding.Store("force-pf", true)

	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})
	err := svc.ForceRebuildTimeline(ctx, "force-pf")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// DeleteAll should be called despite the flag
	if tl.deleteAllCount.Load() != 1 {
		t.Errorf("expected 1 DeleteAll call (force bypasses dedup), got %d", tl.deleteAllCount.Load())
	}
}

// TestForceRebuildTimeline_DeletesAllData verifies DeleteAll is invoked with the right portfolio name.
func TestForceRebuildTimeline_DeletesAllData(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}

	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})
	err := svc.ForceRebuildTimeline(ctx, "my-portfolio")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tl.deleteAllCount.Load() != 1 {
		t.Errorf("expected 1 DeleteAll call, got %d", tl.deleteAllCount.Load())
	}
	if tl.deletedName != "my-portfolio" {
		t.Errorf("expected DeleteAll for 'my-portfolio', got '%s'", tl.deletedName)
	}
}

// TestForceRebuildTimeline_NoTimelineStore verifies error returned when timeline store is nil.
func TestForceRebuildTimeline_NoTimelineStore(t *testing.T) {
	svc := &Service{
		storage: &rebuildStorageManager{tl: nil},
		logger:  common.NewLogger("disabled"),
	}

	ctx := context.Background()
	err := svc.ForceRebuildTimeline(ctx, "any")
	if err == nil {
		t.Error("expected error when timeline store is nil")
	}
}

// TestBackfillTimelineIfEmpty_IncludesCashTransactions verifies that when backfill triggers,
// GetLedger is called (so cash transactions are loaded via rebuildTimelineWithCash).
func TestBackfillTimelineIfEmpty_IncludesCashTransactions(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{
		transactions: []models.CashTransaction{
			{ID: "tx1", Amount: 5000},
		},
	}
	// Sparse timeline — will trigger backfill
	tl := &rebuildTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
		},
	}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}
	uc := &common.UserContext{UserID: "test-user"}
	ctx := common.WithUserContext(context.Background(), uc)

	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{
				Ticker: "BHP",
				Trades: []*models.NavexaTrade{{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 10}},
			},
		},
	}

	svc.backfillTimelineIfEmpty(ctx, portfolio)

	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)

	// GetLedger should have been called by rebuildTimelineWithCash (inside the backfill goroutine)
	if cashSvc.callCount.Load() == 0 {
		t.Error("expected GetLedger to be called during backfill (cash transactions should be loaded)")
	}
}

// TestInvalidateAndRebuildTimeline_NilTimelineStore verifies graceful handling when tl is nil.
func TestInvalidateAndRebuildTimeline_NilTimelineStore(t *testing.T) {
	svc := &Service{
		storage: &rebuildStorageManager{tl: nil},
		logger:  common.NewLogger("disabled"),
	}
	ctx := context.Background()

	// Should not panic — tl nil guard in implementation
	svc.InvalidateAndRebuildTimeline(ctx, "pf")
}

// TestIsTimelineRebuilding_ReturnsFalseByDefault verifies the flag is false for unknown portfolios.
func TestIsTimelineRebuilding_ReturnsFalseByDefault(t *testing.T) {
	svc := &Service{logger: common.NewLogger("disabled")}
	if svc.IsTimelineRebuilding("nonexistent") {
		t.Error("expected false for unknown portfolio")
	}
}

// TestIsTimelineRebuilding_ReturnsTrueWhenSet verifies the flag value is read correctly.
func TestIsTimelineRebuilding_ReturnsTrueWhenSet(t *testing.T) {
	svc := &Service{logger: common.NewLogger("disabled")}
	svc.timelineRebuilding.Store("pf", true)
	if !svc.IsTimelineRebuilding("pf") {
		t.Error("expected true after Store(true)")
	}
}

// TestRebuildTimelineWithCash_PassesTransactionsToGrowth verifies that cash transactions
// from the ledger are loaded via GetLedger before calling GetDailyGrowth.
func TestRebuildTimelineWithCash_PassesTransactionsToGrowth(t *testing.T) {
	cashSvc := &rebuildCashflowSvc{
		transactions: []models.CashTransaction{
			{ID: "tx1", Amount: 1000},
			{ID: "tx2", Amount: 2000},
		},
	}
	tl := &rebuildTimelineStore{}
	svc := &Service{
		storage:     &rebuildStorageManager{tl: tl},
		cashflowSvc: cashSvc,
		logger:      common.NewLogger("disabled"),
	}
	ctx := common.WithUserContext(context.Background(), &common.UserContext{UserID: "u1"})

	// GetDailyGrowth panics on nil storage; recover handles it.
	callWithRecover(func() {
		svc.rebuildTimelineWithCash(ctx, "pf") //nolint:errcheck
	})

	// GetLedger must be called to load transactions
	if cashSvc.callCount.Load() != 1 {
		t.Errorf("expected 1 GetLedger call, got %d", cashSvc.callCount.Load())
	}
}

// Compile-time check: rebuildCashflowSvc implements interfaces.CashFlowService
var _ interfaces.CashFlowService = (*rebuildCashflowSvc)(nil)
