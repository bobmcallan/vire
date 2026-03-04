package portfolio

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

func TestSnapshotsToGrowthPoints(t *testing.T) {
	d1 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 3, 2, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		{
			Date:               d1,
			EquityValue:        100000,
			NetEquityCost:      80000,
			NetEquityReturn:    20000,
			NetEquityReturnPct: 25.0,
			HoldingCount:       5,
			GrossCashBalance:   10000,
			NetCashBalance:     2000,
			PortfolioValue:     102000,
			NetCapitalDeployed: 80000,
		},
		{
			Date:               d2,
			EquityValue:        105000,
			NetEquityCost:      80000,
			NetEquityReturn:    25000,
			NetEquityReturnPct: 31.25,
			HoldingCount:       5,
			GrossCashBalance:   10000,
			NetCashBalance:     2000,
			PortfolioValue:     107000,
			NetCapitalDeployed: 80000,
		},
	}

	points := snapshotsToGrowthPoints(snapshots)
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}

	if points[0].EquityValue != 100000 {
		t.Errorf("expected equity_value 100000, got %f", points[0].EquityValue)
	}
	if points[0].HoldingCount != 5 {
		t.Errorf("expected holding_count 5, got %d", points[0].HoldingCount)
	}
	if points[1].PortfolioValue != 107000 {
		t.Errorf("expected portfolio_value 107000, got %f", points[1].PortfolioValue)
	}
	if !points[0].Date.Equal(d1) {
		t.Errorf("expected date %v, got %v", d1, points[0].Date)
	}
}

func TestSnapshotsToGrowthPoints_Empty(t *testing.T) {
	points := snapshotsToGrowthPoints(nil)
	if len(points) != 0 {
		t.Errorf("expected 0 points for nil input, got %d", len(points))
	}
}

// --- backfillTimelineIfEmpty tests ---

// minimalTimelineStore is a mock that tracks GetRange calls.
type minimalTimelineStore struct {
	snapshots   []models.TimelineSnapshot
	getRangeCnt atomic.Int32
}

func (m *minimalTimelineStore) GetRange(_ context.Context, _, _ string, _, _ time.Time) ([]models.TimelineSnapshot, error) {
	m.getRangeCnt.Add(1)
	return m.snapshots, nil
}
func (m *minimalTimelineStore) GetLatest(_ context.Context, _, _ string) (*models.TimelineSnapshot, error) {
	return nil, fmt.Errorf("not found")
}
func (m *minimalTimelineStore) SaveBatch(_ context.Context, _ []models.TimelineSnapshot) error {
	return nil
}
func (m *minimalTimelineStore) DeleteRange(_ context.Context, _, _ string, _, _ time.Time) (int, error) {
	return 0, nil
}
func (m *minimalTimelineStore) DeleteAll(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

// backfillStorageManager implements StorageManager with a configurable TimelineStore.
type backfillStorageManager struct {
	tl interfaces.TimelineStore
}

func (b *backfillStorageManager) InternalStore() interfaces.InternalStore         { return nil }
func (b *backfillStorageManager) UserDataStore() interfaces.UserDataStore         { return nil }
func (b *backfillStorageManager) MarketDataStorage() interfaces.MarketDataStorage { return nil }
func (b *backfillStorageManager) SignalStorage() interfaces.SignalStorage         { return nil }
func (b *backfillStorageManager) StockIndexStore() interfaces.StockIndexStore     { return nil }
func (b *backfillStorageManager) JobQueueStore() interfaces.JobQueueStore         { return nil }
func (b *backfillStorageManager) FileStore() interfaces.FileStore                 { return nil }
func (b *backfillStorageManager) FeedbackStore() interfaces.FeedbackStore         { return nil }
func (b *backfillStorageManager) ChangelogStore() interfaces.ChangelogStore       { return nil }
func (b *backfillStorageManager) OAuthStore() interfaces.OAuthStore               { return nil }
func (b *backfillStorageManager) TimelineStore() interfaces.TimelineStore         { return b.tl }
func (b *backfillStorageManager) DataPath() string                                { return "" }
func (b *backfillStorageManager) WriteRaw(_, _ string, _ []byte) error            { return nil }
func (b *backfillStorageManager) PurgeDerivedData(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (b *backfillStorageManager) PurgeReports(_ context.Context) (int, error) { return 0, nil }
func (b *backfillStorageManager) Close() error                                { return nil }

func TestBackfillTimelineIfEmpty_SkipsWhenNoTimelineStore(t *testing.T) {
	svc := &Service{
		storage: &backfillStorageManager{tl: nil},
		logger:  common.NewLogger("disabled"),
	}
	ctx := context.Background()
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Trades: []*models.NavexaTrade{{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 10}}},
		},
	}
	// Should return immediately without panic
	svc.backfillTimelineIfEmpty(ctx, portfolio)
}

func TestBackfillTimelineIfEmpty_SkipsWhenNoTrades(t *testing.T) {
	tl := &minimalTimelineStore{}
	svc := &Service{
		storage: &backfillStorageManager{tl: tl},
		logger:  common.NewLogger("disabled"),
	}
	ctx := context.Background()
	portfolio := &models.Portfolio{Name: "test", Holdings: []models.Holding{}}

	svc.backfillTimelineIfEmpty(ctx, portfolio)

	// GetRange should NOT be called — no trades means no backfill needed
	if tl.getRangeCnt.Load() != 0 {
		t.Errorf("expected 0 GetRange calls (no trades), got %d", tl.getRangeCnt.Load())
	}
}

func TestBackfillTimelineIfEmpty_SkipsWhenHistorySufficient(t *testing.T) {
	// Generate enough snapshots to exceed 50% of expected days.
	// Trade from 30 days ago → ~30 expected days → need >15 snapshots.
	tradeDate := time.Now().AddDate(0, 0, -30)
	tradeDateStr := tradeDate.Format("2006-01-02")
	snapshots := make([]models.TimelineSnapshot, 20)
	for i := range snapshots {
		snapshots[i] = models.TimelineSnapshot{
			Date:        tradeDate.AddDate(0, 0, i),
			EquityValue: 100000,
		}
	}
	tl := &minimalTimelineStore{snapshots: snapshots}
	svc := &Service{
		storage: &backfillStorageManager{tl: tl},
		logger:  common.NewLogger("disabled"),
	}
	uc := &common.UserContext{UserID: "test-user", NavexaAPIKey: "key"}
	ctx := common.WithUserContext(context.Background(), uc)
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Trades: []*models.NavexaTrade{{Date: tradeDateStr, Type: "Buy", Units: 100, Price: 10}}},
		},
	}

	svc.backfillTimelineIfEmpty(ctx, portfolio)

	// GetRange IS called to check existing history; sufficient snapshots → no backfill
	if tl.getRangeCnt.Load() != 1 {
		t.Errorf("expected 1 GetRange call (history check), got %d", tl.getRangeCnt.Load())
	}
}

func TestBackfillTimelineIfEmpty_TriggersWhenHistorySparse(t *testing.T) {
	// 1 snapshot covering a multi-month range is sparse — should trigger backfill.
	tl := &minimalTimelineStore{
		snapshots: []models.TimelineSnapshot{
			{Date: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), EquityValue: 100000},
		},
	}
	svc := &Service{
		storage: &backfillStorageManager{tl: tl},
		logger:  common.NewLogger("disabled"),
	}
	uc := &common.UserContext{UserID: "test-user", NavexaAPIKey: "key"}
	ctx := common.WithUserContext(context.Background(), uc)
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Trades: []*models.NavexaTrade{{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 10}}},
		},
	}

	svc.backfillTimelineIfEmpty(ctx, portfolio)

	// GetRange called; sparse history detected → backfill goroutine spawned
	if tl.getRangeCnt.Load() != 1 {
		t.Errorf("expected 1 GetRange call, got %d", tl.getRangeCnt.Load())
	}
	time.Sleep(50 * time.Millisecond) // let goroutine start
}

func TestBackfillTimelineIfEmpty_TriggersWhenHistoryEmpty(t *testing.T) {
	tl := &minimalTimelineStore{
		snapshots: nil, // empty — no historical snapshots
	}
	svc := &Service{
		storage: &backfillStorageManager{tl: tl},
		logger:  common.NewLogger("disabled"),
	}
	uc := &common.UserContext{UserID: "test-user", NavexaAPIKey: "key"}
	ctx := common.WithUserContext(context.Background(), uc)
	portfolio := &models.Portfolio{
		Name: "test",
		Holdings: []models.Holding{
			{Ticker: "BHP", Trades: []*models.NavexaTrade{{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 10}}},
		},
	}

	svc.backfillTimelineIfEmpty(ctx, portfolio)

	// GetRange is called to check history — returns empty
	if tl.getRangeCnt.Load() != 1 {
		t.Errorf("expected 1 GetRange call, got %d", tl.getRangeCnt.Load())
	}

	// The goroutine is spawned but will fail (no market data, etc.) — that's expected.
	// We just verify the decision path was correct (GetRange was checked, empty history detected).
	// The actual GetDailyGrowth execution is tested in growth_test.go.
	time.Sleep(50 * time.Millisecond) // let goroutine start
}

func TestComputeTradeHash_Deterministic(t *testing.T) {
	holdings := []models.Holding{
		{
			Ticker:   "BHP",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 45.50, Fees: 9.95},
				{Date: "2024-06-15", Type: "Sell", Units: 50, Price: 52.00, Fees: 9.95},
			},
		},
		{
			Ticker:   "CBA",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2023-11-05", Type: "Buy", Units: 200, Price: 95.00, Fees: 19.95},
			},
		},
	}

	hash1 := computeTradeHash(holdings)
	hash2 := computeTradeHash(holdings)

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash, got %s and %s", hash1, hash2)
	}
	if len(hash1) != 16 {
		t.Errorf("expected 16-char hash, got %d chars: %s", len(hash1), hash1)
	}
}

func TestComputeTradeHash_ChangesOnModification(t *testing.T) {
	holdings1 := []models.Holding{
		{
			Ticker:   "BHP",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 45.50, Fees: 9.95},
			},
		},
	}

	holdings2 := []models.Holding{
		{
			Ticker:   "BHP",
			Exchange: "ASX",
			Trades: []*models.NavexaTrade{
				{Date: "2024-01-10", Type: "Buy", Units: 100, Price: 45.50, Fees: 9.95},
				{Date: "2024-02-20", Type: "Buy", Units: 50, Price: 48.00, Fees: 9.95}, // new trade
			},
		},
	}

	hash1 := computeTradeHash(holdings1)
	hash2 := computeTradeHash(holdings2)

	if hash1 == hash2 {
		t.Error("expected different hashes when trades change")
	}
}

func TestComputeTradeHash_OrderIndependent(t *testing.T) {
	// Holdings in different order should produce the same hash
	// (because we sort by ticker internally)
	holdings1 := []models.Holding{
		{Ticker: "BHP", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 100, Price: 45}}},
		{Ticker: "CBA", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 50, Price: 95}}},
	}
	holdings2 := []models.Holding{
		{Ticker: "CBA", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 50, Price: 95}}},
		{Ticker: "BHP", Exchange: "ASX", Trades: []*models.NavexaTrade{{Date: "2024-01-01", Type: "Buy", Units: 100, Price: 45}}},
	}

	hash1 := computeTradeHash(holdings1)
	hash2 := computeTradeHash(holdings2)

	if hash1 != hash2 {
		t.Errorf("expected same hash regardless of holding order, got %s and %s", hash1, hash2)
	}
}
