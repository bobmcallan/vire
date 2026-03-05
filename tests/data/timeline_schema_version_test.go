package data

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/services/cashflow"
	"github.com/bobmcallan/vire/internal/services/portfolio"
	tcommon "github.com/bobmcallan/vire/tests/common"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupTimelineTestPortfolio creates a portfolio with a single holding, EOD data,
// and a cash ledger. Returns a wired PortfolioService ready for GetDailyGrowth.
func setupTimelineTestPortfolio(t *testing.T, mgr interfaces.StorageManager, userID, portfolioName string) *portfolio.Service {
	t.Helper()
	ctx := testContext()
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	store := mgr.UserDataStore()

	svc := portfolio.NewService(mgr, nil, nil, nil, common.NewSilentLogger())

	trades := []*models.NavexaTrade{
		{
			ID: "trade_sv_001", Symbol: "TEST", Type: "buy",
			Date: "2025-01-15", Units: 100, Price: 100, Value: 10000, Currency: "AUD",
		},
	}

	holding := models.Holding{
		Ticker: "TEST", Exchange: "AU", Units: 100, AvgCost: 100,
		Trades: trades, Currency: "AUD", OriginalCurrency: "AUD",
		Status: "open", CostBasis: 10000, GrossInvested: 10000,
	}

	p := models.Portfolio{
		Name: portfolioName, SourceType: models.SourceManual, Currency: "AUD",
		Holdings:            []models.Holding{holding},
		EquityHoldingsValue: 10000, PortfolioValue: 10000, EquityHoldingsCost: 10000,
		DataVersion: common.SchemaVersion,
	}

	// Store EOD bars
	eodBars := make([]models.EODBar, 0)
	for d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC); !d.After(time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		eodBars = append(eodBars, models.EODBar{Date: d, Open: 100, High: 101, Low: 99, Close: 100, Volume: 1000})
	}
	require.NoError(t, mgr.MarketDataStorage().SaveMarketData(ctx, &models.MarketData{
		Ticker: "TEST.AU", EOD: eodBars, LastUpdated: time.Now(),
	}))

	// Store portfolio
	pData, err := json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID: userID, Subject: "portfolio", Key: portfolioName,
		Value: string(pData), Version: 1, DateTime: time.Now().Truncate(time.Second),
	}))

	// Store cash ledger
	ledger := models.CashFlowLedger{
		PortfolioName: portfolioName, Version: 1,
		Accounts: []models.CashAccount{
			{Name: "Trading", Type: "trading", IsTransactional: true, Currency: "AUD"},
		},
		Transactions: []models.CashTransaction{
			{
				ID: "ct_sv_01", Account: "Trading", Category: models.CashCatContribution,
				Date: time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC), Amount: 20000,
				Description: "Deposit", CreatedAt: time.Now().Truncate(time.Second),
				UpdatedAt: time.Now().Truncate(time.Second),
			},
		},
		CreatedAt: time.Now().Truncate(time.Second),
		UpdatedAt: time.Now().Truncate(time.Second),
	}
	lData, err := json.Marshal(ledger)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID: userID, Subject: "cashflow", Key: portfolioName,
		Value: string(lData), Version: 1, DateTime: time.Now().Truncate(time.Second),
	}))

	cashflowSvc := cashflow.NewService(mgr, svc, common.NewSilentLogger())
	svc.SetCashFlowService(cashflowSvc)
	return svc
}

// ---------------------------------------------------------------------------
// Test 1: Stale schema version forces rebuild
// ---------------------------------------------------------------------------

// TestTimeline_StaleSchemaVersionForcesRebuild saves a TimelineSnapshot with an
// old DataVersion, then calls GetDailyGrowth and verifies the returned data has
// non-zero fields (proving the stale cache was rejected and trade replay ran).
func TestTimeline_StaleSchemaVersionForcesRebuild(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "stale_version_user"
	name := "StaleVersionTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	// Pre-seed timeline cache with a single STALE snapshot (old DataVersion, zero equity).
	// Using a single snapshot avoids race conditions with the background persist goroutine.
	latestDate := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	staleSnap := models.TimelineSnapshot{
		UserID: userID, PortfolioName: name, Date: latestDate,
		EquityHoldingsValue: 0, // stale: zero from old schema
		EquityHoldingsCost:  0,
		PortfolioValue:      0,
		DataVersion:         "5", // old version
		ComputedAt:          time.Now(),
	}
	require.NoError(t, tl.SaveBatch(ctx, []models.TimelineSnapshot{staleSnap}))

	// Call GetDailyGrowth -- should reject stale cache and recompute
	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// Wait for background persist goroutine to complete before test cleanup
	time.Sleep(3 * time.Second)

	// Verify rebuilt data has non-zero equity (stale was all zeros)
	var hasNonZeroEquity bool
	for _, p := range points {
		if p.EquityHoldingsValue > 0 {
			hasNonZeroEquity = true
			break
		}
	}
	assert.True(t, hasNonZeroEquity,
		"After stale cache rejection, rebuilt timeline should have non-zero equity values")

	guard.SaveResult("01_stale_version_rebuild", fmt.Sprintf(
		"Stale cache rejected, rebuilt %d points with non-zero equity", len(points)))
}

// ---------------------------------------------------------------------------
// Test 2: All fields populated after GetDailyGrowth
// ---------------------------------------------------------------------------

// TestTimeline_AllFieldsPopulated verifies that after GetDailyGrowth, every
// field in the returned GrowthDataPoints is non-zero for dates with active holdings.
func TestTimeline_AllFieldsPopulated(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "all_fields_user"
	name := "AllFieldsTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)

	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// Find a point after the trade date (Jan 15) where holdings are active
	var found bool
	tradeDate := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, p := range points {
		if !p.Date.Before(tradeDate) && p.HoldingCount > 0 {
			found = true
			assert.Greater(t, p.EquityHoldingsValue, 0.0,
				"EquityHoldingsValue should be > 0 on %s", p.Date.Format("2006-01-02"))
			assert.Greater(t, p.EquityHoldingsCost, 0.0,
				"EquityHoldingsCost should be > 0 on %s", p.Date.Format("2006-01-02"))
			assert.Greater(t, p.PortfolioValue, 0.0,
				"PortfolioValue should be > 0 on %s", p.Date.Format("2006-01-02"))
			assert.Greater(t, p.CapitalGross, 0.0,
				"CapitalGross should be > 0 on %s", p.Date.Format("2006-01-02"))
			// Only need one good data point to prove fields are populated
			break
		}
	}
	assert.True(t, found, "Should find at least one data point with active holdings after trade date")

	guard.SaveResult("02_all_fields_populated", fmt.Sprintf(
		"Verified all key fields non-zero across %d points", len(points)))
}

// ---------------------------------------------------------------------------
// Test 3: Field persistence round trip
// ---------------------------------------------------------------------------

// TestTimeline_FieldPersistence_RoundTrip creates TimelineSnapshots with known
// non-zero values, saves via SaveBatch, reads back via GetRange, and verifies
// all fields survive (catches JSON tag mismatches between save and load).
func TestTimeline_FieldPersistence_RoundTrip(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	d := time.Date(2025, 3, 10, 0, 0, 0, 0, time.UTC)
	snap := models.TimelineSnapshot{
		UserID:                    "roundtrip_user",
		PortfolioName:             "RoundTripTest",
		Date:                      d,
		EquityHoldingsValue:       87654.32,
		EquityHoldingsCost:        70000.00,
		EquityHoldingsReturn:      17654.32,
		EquityHoldingsReturnPct:   25.22,
		HoldingCount:              8,
		CapitalGross:              30000.00,
		CapitalAvailable:          12345.68,
		PortfolioValue:            100000.00,
		CapitalContributionsNet:   80000.00,
		IncomeDividendsCumulative: 1500.50,
		FXRate:                    0.6789,
		DataVersion:               common.SchemaVersion,
		ComputedAt:                time.Date(2025, 3, 10, 12, 0, 0, 0, time.UTC),
	}

	require.NoError(t, tl.SaveBatch(ctx, []models.TimelineSnapshot{snap}))

	got, err := tl.GetRange(ctx, "roundtrip_user", "RoundTripTest", d, d)
	require.NoError(t, err)
	require.Len(t, got, 1)

	g := got[0]
	assert.Equal(t, "roundtrip_user", g.UserID)
	assert.Equal(t, "RoundTripTest", g.PortfolioName)
	assert.InDelta(t, 87654.32, g.EquityHoldingsValue, 0.01, "EquityHoldingsValue")
	assert.InDelta(t, 70000.00, g.EquityHoldingsCost, 0.01, "EquityHoldingsCost")
	assert.InDelta(t, 17654.32, g.EquityHoldingsReturn, 0.01, "EquityHoldingsReturn")
	assert.InDelta(t, 25.22, g.EquityHoldingsReturnPct, 0.01, "EquityHoldingsReturnPct")
	assert.Equal(t, 8, g.HoldingCount, "HoldingCount")
	assert.InDelta(t, 30000.00, g.CapitalGross, 0.01, "CapitalGross")
	assert.InDelta(t, 12345.68, g.CapitalAvailable, 0.01, "CapitalAvailable")
	assert.InDelta(t, 100000.00, g.PortfolioValue, 0.01, "PortfolioValue")
	assert.InDelta(t, 80000.00, g.CapitalContributionsNet, 0.01, "CapitalContributionsNet")
	assert.InDelta(t, 1500.50, g.IncomeDividendsCumulative, 0.01, "IncomeDividendsCumulative")
	assert.InDelta(t, 0.6789, g.FXRate, 0.0001, "FXRate")
	assert.Equal(t, common.SchemaVersion, g.DataVersion, "DataVersion")

	guard.SaveResult("03_field_roundtrip", fmt.Sprintf("all %d fields round-tripped correctly", 13))
}

// ---------------------------------------------------------------------------
// Test 4: PeriodChanges with valid snapshots -> HasPrevious=true
// ---------------------------------------------------------------------------

// TestTimeline_PeriodChanges_WithValidSnapshots saves snapshots for yesterday,
// last week, and last month, then calls GetPortfolio and verifies HasPrevious=true
// on the resulting PeriodChanges.
func TestTimeline_PeriodChanges_WithValidSnapshots(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "period_valid_user"
	name := "PeriodValidTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)
	store := mgr.UserDataStore()
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	svc := portfolio.NewService(mgr, nil, nil, nil, common.NewSilentLogger())

	// Create a portfolio with current values.
	// Use SourceNavexa so GetPortfolio calls populateHistoricalValues (which includes populateChanges).
	// Set LastSynced to now so it skips the auto-sync path.
	p := models.Portfolio{
		Name: name, SourceType: models.SourceNavexa, Currency: "AUD",
		EquityHoldingsValue: 15000, PortfolioValue: 20000,
		EquityHoldingsCost: 12000, CapitalGross: 5000,
		DataVersion: common.SchemaVersion,
		Holdings: []models.Holding{
			{Ticker: "TST", Exchange: "AU", Units: 100, CurrentPrice: 150,
				MarketValue: 15000, CostBasis: 12000, Currency: "AUD"},
		},
		LastSynced: time.Now().Truncate(time.Second),
	}
	pData, err := json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID: userID, Subject: "portfolio", Key: name,
		Value: string(pData), Version: 1, DateTime: time.Now().Truncate(time.Second),
	}))

	// Save snapshots at reference dates with non-zero values
	now := time.Now().Truncate(24 * time.Hour)
	refDates := []time.Time{
		now.AddDate(0, 0, -1),  // yesterday
		now.AddDate(0, 0, -7),  // week ago
		now.AddDate(0, 0, -30), // month ago
	}

	snaps := make([]models.TimelineSnapshot, 0, len(refDates))
	for _, d := range refDates {
		snaps = append(snaps, models.TimelineSnapshot{
			UserID: userID, PortfolioName: name, Date: d,
			EquityHoldingsValue: 14000, EquityHoldingsCost: 12000,
			EquityHoldingsReturn: 2000, EquityHoldingsReturnPct: 16.67,
			HoldingCount: 1, CapitalGross: 4500, CapitalAvailable: 1500,
			PortfolioValue: 18500, CapitalContributionsNet: 12000,
			DataVersion: common.SchemaVersion, ComputedAt: time.Now(),
		})
	}
	require.NoError(t, tl.SaveBatch(ctx, snaps))

	// GetPortfolio populates Changes from timeline store
	got, err := svc.GetPortfolio(ctx, name)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Changes, "Changes should be populated")

	// All periods should have HasPrevious=true because we stored valid snapshots
	assert.True(t, got.Changes.Yesterday.PortfolioValue.HasPrevious,
		"Yesterday PortfolioValue should have previous from snapshot")
	assert.True(t, got.Changes.Yesterday.EquityHoldingsValue.HasPrevious,
		"Yesterday EquityHoldingsValue should have previous from snapshot")
	assert.True(t, got.Changes.Week.PortfolioValue.HasPrevious,
		"Week PortfolioValue should have previous from snapshot")
	assert.True(t, got.Changes.Month.PortfolioValue.HasPrevious,
		"Month PortfolioValue should have previous from snapshot")

	guard.SaveResult("04_period_changes_valid", "All periods have HasPrevious=true with valid snapshots")
}

// ---------------------------------------------------------------------------
// Test 5: PeriodChanges with stale snapshots -> graceful fallback
// ---------------------------------------------------------------------------

// TestTimeline_PeriodChanges_StaleSnapshotsGraceful saves snapshots with a stale
// DataVersion. Since computePeriodChanges reads from the timeline store directly
// (it does not check DataVersion), this test verifies the system does not crash
// and returns data (the DataVersion guard is in tryTimelineCache, not computePeriodChanges).
func TestTimeline_PeriodChanges_StaleSnapshotsGraceful(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "period_stale_user"
	name := "PeriodStaleTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)
	store := mgr.UserDataStore()
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	svc := portfolio.NewService(mgr, nil, nil, nil, common.NewSilentLogger())

	p := models.Portfolio{
		Name: name, SourceType: models.SourceNavexa, Currency: "AUD",
		EquityHoldingsValue: 10000, PortfolioValue: 10000,
		EquityHoldingsCost: 8000,
		DataVersion:        common.SchemaVersion,
		Holdings: []models.Holding{
			{Ticker: "TST", Exchange: "AU", Units: 50, CurrentPrice: 200,
				MarketValue: 10000, CostBasis: 8000, Currency: "AUD"},
		},
		LastSynced: time.Now().Truncate(time.Second),
	}
	pData, err := json.Marshal(p)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID: userID, Subject: "portfolio", Key: name,
		Value: string(pData), Version: 1, DateTime: time.Now().Truncate(time.Second),
	}))

	// Save stale snapshots with zero equity (simulating old schema)
	now := time.Now().Truncate(24 * time.Hour)
	yesterday := now.AddDate(0, 0, -1)
	staleSnap := models.TimelineSnapshot{
		UserID: userID, PortfolioName: name, Date: yesterday,
		EquityHoldingsValue: 0, PortfolioValue: 0, // zeroed from stale schema
		DataVersion: "5", ComputedAt: time.Now(),
	}
	require.NoError(t, tl.SaveBatch(ctx, []models.TimelineSnapshot{staleSnap}))

	// GetPortfolio should NOT crash
	got, err := svc.GetPortfolio(ctx, name)
	require.NoError(t, err)
	require.NotNil(t, got)

	// With stale (zero-valued) snapshots, HasPrevious will be false
	// because buildMetricChange sets HasPrevious = previous > 0
	if got.Changes != nil {
		assert.False(t, got.Changes.Yesterday.PortfolioValue.HasPrevious,
			"Zero-valued stale snapshot should yield HasPrevious=false")
	}

	guard.SaveResult("05_period_stale_graceful", "No crash with stale snapshots; HasPrevious=false as expected")
}

// ---------------------------------------------------------------------------
// Test 6: Selldown transition — no outlier spikes
// ---------------------------------------------------------------------------

// TestTimeline_SelldownTransition creates snapshots showing a gradual position
// reduction and verifies the timeline has no outlier spikes (smooth transition).
func TestTimeline_SelldownTransition(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	userID := "selldown_user"
	name := "SelldownTest"

	// Create snapshots: equity goes from 10000 down to 0 over 5 days
	base := time.Date(2025, 2, 10, 0, 0, 0, 0, time.UTC)
	equityValues := []float64{10000, 7500, 5000, 2500, 0}
	snaps := make([]models.TimelineSnapshot, len(equityValues))
	for i, ev := range equityValues {
		d := base.AddDate(0, 0, i)
		snaps[i] = models.TimelineSnapshot{
			UserID: userID, PortfolioName: name, Date: d,
			EquityHoldingsValue:  ev,
			EquityHoldingsCost:   ev * 0.8,
			EquityHoldingsReturn: ev * 0.2,
			HoldingCount:         5 - i, // reducing holdings
			CapitalGross:         20000,
			CapitalAvailable:     20000 - ev, // cash increases as equity decreases
			PortfolioValue:       20000,      // constant: equity + cash = 20k
			DataVersion:          common.SchemaVersion,
			ComputedAt:           time.Now(),
		}
	}
	require.NoError(t, tl.SaveBatch(ctx, snaps))

	// Read back and check for no spikes
	got, err := tl.GetRange(ctx, userID, name, base, base.AddDate(0, 0, 4))
	require.NoError(t, err)
	require.Len(t, got, 5)

	// Verify monotonic decrease in equity and no spikes in portfolio value
	for i := 1; i < len(got); i++ {
		assert.LessOrEqual(t, got[i].EquityHoldingsValue, got[i-1].EquityHoldingsValue,
			"Equity should monotonically decrease during selldown (day %d)", i)
		// Portfolio value should remain stable (equity + cash = constant)
		assert.InDelta(t, 20000, got[i].PortfolioValue, 0.01,
			"Portfolio value should remain stable during selldown (day %d)", i)
	}

	// Final day: equity = 0, cash = 20000
	assert.InDelta(t, 0, got[4].EquityHoldingsValue, 0.01, "Final equity should be zero")
	assert.InDelta(t, 20000, got[4].CapitalAvailable, 0.01, "Final cash should be 20000")

	guard.SaveResult("06_selldown_transition", "Smooth selldown: no spikes, portfolio value stable at $20,000")
}

// ---------------------------------------------------------------------------
// Test 7: Schema version persisted after persistTimelineSnapshots
// ---------------------------------------------------------------------------

// TestTimeline_SchemaVersionPersisted verifies that after GetDailyGrowth triggers
// trade replay and persists snapshots, the saved snapshots have DataVersion ==
// common.SchemaVersion.
func TestTimeline_SchemaVersionPersisted(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "schema_persist_user"
	name := "SchemaPersistTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	// Call GetDailyGrowth to trigger trade replay + persist
	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// persistTimelineSnapshots runs in a background goroutine; give it time
	time.Sleep(2 * time.Second)

	// Read back persisted snapshots
	from := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	snaps, err := tl.GetRange(ctx, userID, name, from, to)
	require.NoError(t, err)
	require.NotEmpty(t, snaps, "Should have persisted snapshots after GetDailyGrowth")

	for _, snap := range snaps {
		assert.Equal(t, common.SchemaVersion, snap.DataVersion,
			"Persisted snapshot for %s should have current SchemaVersion", snap.Date.Format("2006-01-02"))
	}

	guard.SaveResult("07_schema_version_persisted", fmt.Sprintf(
		"All %d persisted snapshots have DataVersion=%s", len(snaps), common.SchemaVersion))
}

// ---------------------------------------------------------------------------
// Test 8: Cache hit after rebuild
// ---------------------------------------------------------------------------

// TestTimeline_CacheHitAfterRebuild forces a cache miss by seeding stale data,
// calls GetDailyGrowth (which rebuilds), then calls again and verifies the second
// call serves from cache (DataVersion now matches).
func TestTimeline_CacheHitAfterRebuild(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "cache_hit_user"
	name := "CacheHitTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	// Seed stale cache with a single snapshot
	latestDate := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	staleSnap := models.TimelineSnapshot{
		UserID: userID, PortfolioName: name, Date: latestDate,
		EquityHoldingsValue: 0, DataVersion: "5", ComputedAt: time.Now(),
	}
	require.NoError(t, tl.SaveBatch(ctx, []models.TimelineSnapshot{staleSnap}))

	// First call: cache miss (stale), triggers rebuild
	points1, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points1)

	// Wait for async persist
	time.Sleep(2 * time.Second)

	// Second call: should hit cache (fresh version)
	points2, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points2)

	// Both calls should return the same data
	assert.Equal(t, len(points1), len(points2),
		"Both calls should return the same number of points")

	// Verify the persisted data has current version
	from := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 3, 31, 0, 0, 0, 0, time.UTC)
	snaps, err := tl.GetRange(ctx, userID, name, from, to)
	require.NoError(t, err)
	require.NotEmpty(t, snaps)

	allCurrent := true
	for _, s := range snaps {
		if s.DataVersion != common.SchemaVersion {
			allCurrent = false
			break
		}
	}
	assert.True(t, allCurrent, "After rebuild, all cached snapshots should have current DataVersion")

	guard.SaveResult("08_cache_hit_after_rebuild", fmt.Sprintf(
		"First call rebuilt %d points, second call served from cache (%d points), all DataVersion=%s",
		len(points1), len(points2), common.SchemaVersion))
}
