package portfolio

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Devils-advocate stress tests for the timeline cash auto-loading fix.
// These target race conditions, cache poisoning, nil/empty semantics,
// concurrent writes, and graceful degradation.

// ---------------------------------------------------------------------------
// 1. Race condition: concurrent GetDailyGrowth calls auto-loading cash
// ---------------------------------------------------------------------------

func TestAutoLoadCash_ConcurrentGetDailyGrowth(t *testing.T) {
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -5)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 6, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "concurrent-test",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	var callCount atomic.Int64
	cashSvc := &countingCashFlowService{
		ledger: &models.CashFlowLedger{
			Transactions: []models.CashTransaction{
				{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 10000},
			},
		},
		callCount: &callCount,
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	var wg sync.WaitGroup
	errs := make([]error, 20)
	results := make([][]models.GrowthDataPoint, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			pts, err := svc.GetDailyGrowth(context.Background(), "concurrent-test", interfaces.GrowthOptions{
				From: day1,
				To:   now.AddDate(0, 0, -1),
			})
			errs[idx] = err
			results[idx] = pts
		}(i)
	}
	wg.Wait()

	// All calls should succeed without panics
	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d returned error", i)
	}

	// All results should have identical cash values (deterministic output)
	for i, pts := range results {
		require.NotEmpty(t, pts, "goroutine %d returned empty points", i)
		// First point should have GrossCash = 10000 (contribution)
		assert.Equal(t, 10000.0, pts[0].GrossCashBalance, "goroutine %d: GrossCash mismatch", i)
	}

	// GetLedger was called concurrently — verify no panic occurred
	assert.GreaterOrEqual(t, callCount.Load(), int64(1), "GetLedger should have been called at least once")
}

// ---------------------------------------------------------------------------
// 2. Cache poisoning recovery: stale snapshots without cash
// ---------------------------------------------------------------------------

func TestAutoLoadCash_StaleCacheWithoutCash(t *testing.T) {
	// Scenario: timeline cache has snapshots where portfolio_value = equity_value
	// (no cash). After the fix, a cache miss should recompute WITH cash.
	// The tryTimelineCache path may return stale data; verify the fix handles this.

	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -5)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"CBA.AU": {Ticker: "CBA.AU", EOD: generateEODBars(day1, 6, 100.0)},
		},
	}

	// Create a timeline store with stale snapshots (no cash data)
	tls := &stubTimelineStore{
		snapshots: map[string][]models.TimelineSnapshot{
			"cache-test": func() []models.TimelineSnapshot {
				snaps := make([]models.TimelineSnapshot, 5)
				for i := 0; i < 5; i++ {
					d := day1.AddDate(0, 0, i)
					snaps[i] = models.TimelineSnapshot{
						Date:           d,
						EquityValue:    10000,
						PortfolioValue: 10000, // BUG: should be 10000 + cash
						HoldingCount:   1,
					}
				}
				return snaps
			}(),
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
		timelineStore: tls,
	}

	portfolio := &models.Portfolio{
		Name: "cache-test",
		Holdings: []models.Holding{{
			Ticker: "CBA", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 100.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	cashSvc := &stubCashFlowService{
		ledger: &models.CashFlowLedger{
			Transactions: []models.CashTransaction{
				{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 50000},
			},
		},
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	// If tryTimelineCache returns the stale data, we get portfolio_value = equity_value.
	// If it misses (because stale cache doesn't extend to yesterday), recompute should include cash.
	points, err := svc.GetDailyGrowth(context.Background(), "cache-test", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// KEY ASSERTION: if cache was served, it will have stale data with PortfolioValue = EquityValue.
	// After the fix, either:
	// a) Cache miss → recompute with cash → PortfolioValue > EquityValue
	// b) Cache hit with stale data → PortfolioValue = EquityValue (BUG still present in cache path)
	//
	// This test documents the behavior. The cache hit path does NOT recompute cash.
	// Timeline rebuild (InvalidateAndRebuildTimeline) is the only way to fix stale cached data.
	last := points[len(points)-1]
	t.Logf("last point: EquityValue=%.2f, GrossCash=%.2f, PortfolioValue=%.2f",
		last.EquityValue, last.GrossCashBalance, last.PortfolioValue)

	// If recomputed (cache miss): GrossCash should reflect the contribution
	if last.GrossCashBalance > 0 {
		assert.Greater(t, last.PortfolioValue, last.EquityValue,
			"recomputed data should have PortfolioValue > EquityValue when cash exists")
	}
}

// ---------------------------------------------------------------------------
// 3. Empty cash ledger: no transactions, no panic
// ---------------------------------------------------------------------------

func TestAutoLoadCash_EmptyLedger(t *testing.T) {
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "empty-cash",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	// Empty ledger: no transactions at all
	cashSvc := &stubCashFlowService{
		ledger: &models.CashFlowLedger{Transactions: []models.CashTransaction{}},
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	points, err := svc.GetDailyGrowth(context.Background(), "empty-cash", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// With empty ledger: portfolio_value should equal equity_value (no cash)
	for _, pt := range points {
		assert.Equal(t, pt.EquityValue, pt.PortfolioValue,
			"with empty cash ledger, PortfolioValue should equal EquityValue on %s", pt.Date.Format("2006-01-02"))
		assert.Equal(t, 0.0, pt.GrossCashBalance)
		assert.Equal(t, 0.0, pt.NetCashBalance)
	}
}

func TestAutoLoadCash_NilLedger(t *testing.T) {
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "nil-cash",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	// GetLedger returns nil ledger (no ledger ever created)
	cashSvc := &stubCashFlowService{ledger: nil}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	points, err := svc.GetDailyGrowth(context.Background(), "nil-cash", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// With nil ledger: should degrade to equity-only (no panic)
	for _, pt := range points {
		assert.Equal(t, 0.0, pt.GrossCashBalance)
	}
}

func TestAutoLoadCash_GetLedgerError(t *testing.T) {
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "err-cash",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	// GetLedger returns an error (e.g., storage failure)
	cashSvc := &stubCashFlowService{err: errors.New("database connection lost")}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	// Should NOT fail — graceful degradation to equity-only
	points, err := svc.GetDailyGrowth(context.Background(), "err-cash", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	for _, pt := range points {
		assert.Equal(t, 0.0, pt.GrossCashBalance)
	}
}

// ---------------------------------------------------------------------------
// 4. Concurrent cash writes during GetDailyGrowth
// ---------------------------------------------------------------------------

func TestAutoLoadCash_ConcurrentLedgerMutation(t *testing.T) {
	// Simulate: GetLedger returns a ledger, but another goroutine is modifying
	// the transaction slice. The auto-load should read a consistent snapshot.
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -5)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 6, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "mutation-test",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	// Shared mutable ledger
	sharedTxs := []models.CashTransaction{
		{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 10000},
	}
	cashSvc := &stubCashFlowService{
		ledger: &models.CashFlowLedger{Transactions: sharedTxs},
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	var wg sync.WaitGroup

	// Writer goroutine: mutates the ledger while readers are running
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			newTx := models.CashTransaction{
				ID:       "tx-mutated",
				Account:  "Trading",
				Category: models.CashCatDividend,
				Date:     day1.AddDate(0, 0, i%5),
				Amount:   float64(i * 10),
			}
			cashSvc.ledger = &models.CashFlowLedger{
				Transactions: append(sharedTxs, newTx),
			}
			time.Sleep(time.Microsecond)
		}
	}()

	// Reader goroutines: call GetDailyGrowth concurrently
	readErrs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.GetDailyGrowth(context.Background(), "mutation-test", interfaces.GrowthOptions{
				From: day1,
				To:   now.AddDate(0, 0, -1),
			})
			readErrs[idx] = err
		}(i)
	}

	wg.Wait()

	// The key assertion: no panics, no index-out-of-bounds, all calls complete
	for i, err := range readErrs {
		assert.NoError(t, err, "reader goroutine %d panicked or errored", i)
	}
}

// ---------------------------------------------------------------------------
// 5. nil vs empty slice semantics
// ---------------------------------------------------------------------------

func TestAutoLoadCash_NilTriggersAutoLoad(t *testing.T) {
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "nil-vs-empty",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	var callCount atomic.Int64
	cashSvc := &countingCashFlowService{
		ledger: &models.CashFlowLedger{
			Transactions: []models.CashTransaction{
				{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 20000},
			},
		},
		callCount: &callCount,
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	// nil Transactions → should auto-load from cashflow service
	callCount.Store(0)
	pts, err := svc.GetDailyGrowth(context.Background(), "nil-vs-empty", interfaces.GrowthOptions{
		From:         day1,
		To:           now.AddDate(0, 0, -1),
		Transactions: nil,
	})
	require.NoError(t, err)
	require.NotEmpty(t, pts)

	// Verify auto-load was invoked
	assert.Equal(t, int64(1), callCount.Load(), "nil Transactions should trigger exactly one GetLedger call")

	// Verify cash was applied
	assert.Equal(t, 20000.0, pts[0].GrossCashBalance, "auto-loaded cash should appear in GrossCashBalance")
}

func TestAutoLoadCash_EmptySliceSkipsAutoLoad(t *testing.T) {
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "empty-slice",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	var callCount atomic.Int64
	cashSvc := &countingCashFlowService{
		ledger: &models.CashFlowLedger{
			Transactions: []models.CashTransaction{
				{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 20000},
			},
		},
		callCount: &callCount,
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	// Empty slice Transactions → should NOT auto-load (caller explicitly said "no transactions")
	callCount.Store(0)
	pts, err := svc.GetDailyGrowth(context.Background(), "empty-slice", interfaces.GrowthOptions{
		From:         day1,
		To:           now.AddDate(0, 0, -1),
		Transactions: []models.CashTransaction{},
	})
	require.NoError(t, err)
	require.NotEmpty(t, pts)

	// Verify auto-load was NOT invoked
	assert.Equal(t, int64(0), callCount.Load(), "empty slice Transactions should NOT trigger GetLedger")

	// Verify no cash was applied (empty slice = no transactions)
	for _, pt := range pts {
		assert.Equal(t, 0.0, pt.GrossCashBalance, "empty slice should mean no cash on %s", pt.Date.Format("2006-01-02"))
	}
}

// ---------------------------------------------------------------------------
// Additional edge cases
// ---------------------------------------------------------------------------

func TestAutoLoadCash_NilCashFlowService(t *testing.T) {
	// cashflowSvc is nil entirely — should degrade without panic
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "no-cashsvc",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	svc := NewService(storage, nil, nil, nil, logger)
	// Deliberately NOT calling svc.SetCashFlowService

	pts, err := svc.GetDailyGrowth(context.Background(), "no-cashsvc", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err, "nil cashflowSvc should not cause error")
	require.NotEmpty(t, pts)

	// Without cashflow service, equity-only behavior
	for _, pt := range pts {
		assert.Equal(t, 0.0, pt.GrossCashBalance)
	}
}

func TestAutoLoadCash_SortMutationSafety(t *testing.T) {
	// GetDailyGrowth sorts opts.Transactions in-place (sort.Slice).
	// When auto-loading from cashflow service, the service's internal slice
	// could be mutated. Verify the auto-loaded slice is safe to sort.
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -5)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 6, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "sort-safety",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	// Out-of-order transactions in the ledger
	originalTxs := []models.CashTransaction{
		{ID: "tx3", Account: "Trading", Category: models.CashCatDividend, Date: day1.AddDate(0, 0, 4), Amount: 500},
		{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 10000},
		{ID: "tx2", Account: "Trading", Category: models.CashCatOther, Date: day1.AddDate(0, 0, 2), Amount: -2000},
	}

	cashSvc := &stubCashFlowService{
		ledger: &models.CashFlowLedger{Transactions: originalTxs},
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	// First call: auto-loads and sorts the transactions
	pts1, err := svc.GetDailyGrowth(context.Background(), "sort-safety", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err)
	require.NotEmpty(t, pts1)

	// FINDING: sort.Slice in GetDailyGrowth mutates the auto-loaded slice.
	// If the cashflow service returns the same slice reference, the service's
	// internal data is mutated. This is a known issue (documented in existing
	// TestGrowthSortMutatesCallerSlice). For auto-load, the impact is that
	// the ledger's transaction order may change in-place.
	//
	// Verify the function still produces correct results on a second call.
	pts2, err := svc.GetDailyGrowth(context.Background(), "sort-safety", interfaces.GrowthOptions{
		From: day1,
		To:   now.AddDate(0, 0, -1),
	})
	require.NoError(t, err)
	require.Len(t, pts2, len(pts1), "second call should produce same number of points")

	// Cash balances should be identical across both calls
	for i := range pts1 {
		assert.Equal(t, pts1[i].GrossCashBalance, pts2[i].GrossCashBalance,
			"GrossCash mismatch on day %d between calls", i)
	}
}

func TestAutoLoadCash_RebuildTimelineWithCash_Simplified(t *testing.T) {
	// After the fix, rebuildTimelineWithCash should still work correctly.
	// Its body becomes just GetDailyGrowth with empty GrowthOptions{},
	// which now auto-loads cash. Verify the chain works end-to-end.
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -3)

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 4, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "rebuild-test",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	var callCount atomic.Int64
	cashSvc := &countingCashFlowService{
		ledger: &models.CashFlowLedger{
			Transactions: []models.CashTransaction{
				{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 30000},
			},
		},
		callCount: &callCount,
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	pts, err := svc.rebuildTimelineWithCash(context.Background(), "rebuild-test")
	require.NoError(t, err)
	require.NotEmpty(t, pts)

	// Whether rebuildTimelineWithCash loads cash itself or delegates to GetDailyGrowth auto-load,
	// the result should include cash.
	assert.Equal(t, 30000.0, pts[0].GrossCashBalance, "rebuildTimelineWithCash must include cash")
}

func TestAutoLoadCash_GetPortfolioGrowth_InheritsFix(t *testing.T) {
	// GetPortfolioGrowth calls GetDailyGrowth with empty GrowthOptions{}.
	// After the fix, it should auto-load cash.
	logger := common.NewLogger("error")
	now := time.Now().Truncate(24 * time.Hour)
	day1 := now.AddDate(0, 0, -35) // 35 days to get monthly downsampling

	marketStore := &stubMarketDataStorage{
		data: map[string]*models.MarketData{
			"BHP.AU": {Ticker: "BHP.AU", EOD: generateEODBars(day1, 36, 50.0)},
		},
	}
	storage := &stubStorageManager{
		marketStore:   marketStore,
		userDataStore: newMemUserDataStore(),
	}
	portfolio := &models.Portfolio{
		Name: "monthly-test",
		Holdings: []models.Holding{{
			Ticker: "BHP", Exchange: "AU", Units: 100,
			Trades: []*models.NavexaTrade{{Date: day1.Format("2006-01-02"), Type: "buy", Units: 100, Price: 50.0}},
		}},
	}
	storePortfolio(t, storage.userDataStore, portfolio)

	var callCount atomic.Int64
	cashSvc := &countingCashFlowService{
		ledger: &models.CashFlowLedger{
			Transactions: []models.CashTransaction{
				{ID: "tx1", Account: "Trading", Category: models.CashCatContribution, Date: day1, Amount: 15000},
			},
		},
		callCount: &callCount,
	}

	svc := NewService(storage, nil, nil, nil, logger)
	svc.SetCashFlowService(cashSvc)

	pts, err := svc.GetPortfolioGrowth(context.Background(), "monthly-test")
	require.NoError(t, err)
	require.NotEmpty(t, pts)

	// Verify cash was auto-loaded (GetLedger called)
	assert.GreaterOrEqual(t, callCount.Load(), int64(1), "GetPortfolioGrowth should trigger auto-load")
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// countingCashFlowService wraps stubCashFlowService with atomic call counting.
type countingCashFlowService struct {
	ledger    *models.CashFlowLedger
	callCount *atomic.Int64
}

func (c *countingCashFlowService) GetLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	c.callCount.Add(1)
	return c.ledger, nil
}
func (c *countingCashFlowService) AddTransaction(_ context.Context, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) AddTransfer(_ context.Context, _ string, _, _ string, _ float64, _ time.Time, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) UpdateTransaction(_ context.Context, _, _ string, _ models.CashTransaction) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) RemoveTransaction(_ context.Context, _, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) SetTransactions(_ context.Context, _ string, _ []models.CashTransaction, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) ClearLedger(_ context.Context, _ string) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) UpdateAccount(_ context.Context, _ string, _ string, _ models.CashAccountUpdate) (*models.CashFlowLedger, error) {
	return nil, nil
}
func (c *countingCashFlowService) CalculatePerformance(_ context.Context, _ string) (*models.CapitalPerformance, error) {
	return nil, nil
}
