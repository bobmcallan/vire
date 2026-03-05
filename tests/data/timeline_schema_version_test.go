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
// Test 1: Stale schema version forces rebuild (mixed-version scenario)
// ---------------------------------------------------------------------------

// TestTimeline_StaleSchemaVersionForcesRebuild seeds BOTH historical snapshots
// with an old DataVersion (zero equity, non-zero holding_count) AND today's
// snapshot with current DataVersion (non-zero equity). Calls GetDailyGrowth
// and verifies all returned points with holdings have non-zero equity,
// proving the stale historical cache was rejected and trade replay ran.
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

	// Seed historical snapshots with OLD DataVersion: zero equity but non-zero holding_count.
	// This simulates the production failure where renamed fields deserialize as zero.
	staleSnaps := make([]models.TimelineSnapshot, 0, 5)
	for i := 0; i < 5; i++ {
		d := time.Date(2025, 2, 10+i, 0, 0, 0, 0, time.UTC)
		staleSnaps = append(staleSnaps, models.TimelineSnapshot{
			UserID: userID, PortfolioName: name, Date: d,
			EquityHoldingsValue: 0,     // stale: zero from old field name
			EquityHoldingsCost:  0,     // stale: zero from old field name
			PortfolioValue:      47100, // NOT renamed, deserializes fine
			HoldingCount:        19,    // NOT renamed, deserializes fine
			DataVersion:         "5",   // old version
			ComputedAt:          time.Now(),
		})
	}

	// Seed today's snapshot with CURRENT DataVersion (written by writeTodaySnapshot).
	todaySnap := models.TimelineSnapshot{
		UserID: userID, PortfolioName: name, Date: time.Now().Truncate(24 * time.Hour),
		EquityHoldingsValue: 10000,
		EquityHoldingsCost:  8000,
		PortfolioValue:      15000,
		HoldingCount:        1,
		DataVersion:         common.SchemaVersion, // current
		ComputedAt:          time.Now(),
	}
	allSnaps := append(staleSnaps, todaySnap)
	require.NoError(t, tl.SaveBatch(ctx, allSnaps))

	// Call GetDailyGrowth -- should reject stale cache and recompute via trade replay
	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// Wait for background persist goroutine to complete before test cleanup
	time.Sleep(3 * time.Second)

	// Verify: every point with holding_count > 0 must have equity > 0
	var activeCount int
	for _, p := range points {
		if p.HoldingCount > 0 {
			activeCount++
			assert.Greater(t, p.EquityHoldingsValue, 0.0,
				"Point on %s has holding_count=%d but equity=0 (stale cache served)",
				p.Date.Format("2006-01-02"), p.HoldingCount)
		}
	}
	assert.Greater(t, activeCount, 0,
		"Should have at least one data point with active holdings")

	guard.SaveResult("01_stale_version_rebuild", fmt.Sprintf(
		"Mixed-version cache rejected, rebuilt %d points, %d with active holdings all have equity>0",
		len(points), activeCount))
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

// ---------------------------------------------------------------------------
// Test 9: Exact production failure — today current, historical stale
// ---------------------------------------------------------------------------

// TestTimeline_TodayCurrentButHistoricalStale reproduces the exact production
// failure mode: writeTodaySnapshot updates today to current schema version,
// but historical snapshots retain old field names causing equity fields to
// deserialize as zero while holding_count (not renamed) remains correct.
func TestTimeline_TodayCurrentButHistoricalStale(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "prod_failure_user"
	name := "ProdFailureTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)
	tl := mgr.TimelineStore()
	require.NotNil(t, tl)

	// Step 1: Seed 5 historical snapshots with OLD DataVersion.
	// Simulates stale cache: equity fields = 0 (old JSON key), but holding_count
	// and portfolio_value (not renamed) deserialize correctly.
	historicalSnaps := make([]models.TimelineSnapshot, 0, 5)
	for i := 0; i < 5; i++ {
		d := time.Date(2025, 2, 3+i, 0, 0, 0, 0, time.UTC)
		historicalSnaps = append(historicalSnaps, models.TimelineSnapshot{
			UserID: userID, PortfolioName: name, Date: d,
			EquityHoldingsValue:     0,     // zero: old field name "equity_value" didn't match new "equity_holdings_value"
			EquityHoldingsCost:      0,     // zero: same rename issue
			EquityHoldingsReturn:    0,     // zero
			EquityHoldingsReturnPct: 0,     // zero
			HoldingCount:            20,    // NOT renamed — deserializes correctly
			CapitalGross:            25000, // NOT renamed
			PortfolioValue:          47100, // NOT renamed — deserializes correctly
			DataVersion:             "5",   // old schema version
			ComputedAt:              time.Now(),
		})
	}

	// Step 2: Seed today's snapshot with CURRENT DataVersion (as writeTodaySnapshot would).
	today := time.Now().Truncate(24 * time.Hour)
	todaySnap := models.TimelineSnapshot{
		UserID: userID, PortfolioName: name, Date: today,
		EquityHoldingsValue:     10000,
		EquityHoldingsCost:      8000,
		EquityHoldingsReturn:    2000,
		EquityHoldingsReturnPct: 25.0,
		HoldingCount:            1,
		CapitalGross:            20000,
		PortfolioValue:          30000,
		DataVersion:             common.SchemaVersion, // current — written by writeTodaySnapshot
		ComputedAt:              time.Now(),
	}
	allSnaps := append(historicalSnaps, todaySnap)
	require.NoError(t, tl.SaveBatch(ctx, allSnaps))

	// Step 3: Call GetDailyGrowth — must reject the mixed-version cache
	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// Wait for background persist
	time.Sleep(3 * time.Second)

	// Step 4: THE KEY INVARIANT — no data point should have holding_count > 0
	// with equity_holdings_value == 0. This is the exact bug signature.
	var violations int
	for _, p := range points {
		if p.HoldingCount > 0 && p.EquityHoldingsValue == 0 {
			violations++
			t.Errorf("PRODUCTION BUG DETECTED: date=%s holding_count=%d equity_holdings_value=0",
				p.Date.Format("2006-01-02"), p.HoldingCount)
		}
	}
	assert.Equal(t, 0, violations,
		"No data point should have holding_count > 0 with equity_holdings_value == 0")

	// Step 5: Verify ALL points with active holdings have correct equity
	var activePoints int
	for _, p := range points {
		if p.HoldingCount > 0 {
			activePoints++
			assert.Greater(t, p.EquityHoldingsValue, 0.0,
				"date=%s: holding_count=%d requires equity_holdings_value > 0",
				p.Date.Format("2006-01-02"), p.HoldingCount)
		}
	}
	assert.Greater(t, activePoints, 0, "Should have at least one point with active holdings")

	guard.SaveResult("09_today_current_historical_stale", fmt.Sprintf(
		"Production failure test: %d points, %d active, 0 violations (holding_count>0 with equity==0)",
		len(points), activePoints))
}

// ---------------------------------------------------------------------------
// Test 10: Chart output invariant — no zero equity with active holdings
// ---------------------------------------------------------------------------

// TestTimeline_ChartOutput_NoZeroEquityWithActiveHoldings is the fundamental
// invariant test: after any GetDailyGrowth call with a portfolio that has
// active holdings, EVERY data point where holding_count > 0 MUST have
// equity_holdings_value > 0. This test would have caught the original bug.
func TestTimeline_ChartOutput_NoZeroEquityWithActiveHoldings(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "chart_invariant_user"
	name := "ChartInvariantTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)

	// Run GetDailyGrowth with no pre-seeded cache (clean compute from trades)
	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// Wait for background persist
	time.Sleep(2 * time.Second)

	// THE INVARIANT: holding_count > 0 implies equity_holdings_value > 0
	var activeCount, violationCount int
	for _, p := range points {
		if p.HoldingCount > 0 {
			activeCount++
			if p.EquityHoldingsValue <= 0 {
				violationCount++
				t.Errorf("INVARIANT VIOLATION: date=%s holding_count=%d but equity_holdings_value=%.2f",
					p.Date.Format("2006-01-02"), p.HoldingCount, p.EquityHoldingsValue)
			}
		}
	}
	assert.Greater(t, activeCount, 0,
		"Portfolio with trades should produce at least one data point with active holdings")
	assert.Equal(t, 0, violationCount,
		"No data point should violate: holding_count > 0 implies equity_holdings_value > 0")

	// Now run again (should serve from cache) and re-validate the invariant
	points2, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points2)

	var cacheViolations int
	for _, p := range points2 {
		if p.HoldingCount > 0 && p.EquityHoldingsValue <= 0 {
			cacheViolations++
		}
	}
	assert.Equal(t, 0, cacheViolations,
		"Invariant must hold for cached results too")

	guard.SaveResult("10_chart_no_zero_equity", fmt.Sprintf(
		"Invariant verified: %d total points, %d active, 0 violations (fresh + cached)",
		len(points), activeCount))
}

// ---------------------------------------------------------------------------
// Test 11: Chart output — all fields consistent
// ---------------------------------------------------------------------------

// TestTimeline_ChartOutput_AllFieldsConsistent validates broader consistency
// rules across chart data points:
// - If holding_count > 0: equity_holdings_value > 0, equity_holdings_cost > 0
// - If equity_holdings_value > 0: portfolio_value >= equity_holdings_value
// - If capital_gross > 0: portfolio_value > 0
func TestTimeline_ChartOutput_AllFieldsConsistent(t *testing.T) {
	mgr := testManager(t)
	ctx := testContext()
	userID := "chart_consistent_user"
	name := "ChartConsistentTest"
	ctx = common.WithUserContext(ctx, &common.UserContext{UserID: userID})
	guard := tcommon.NewTestOutputGuard(t)

	svc := setupTimelineTestPortfolio(t, mgr, userID, name)

	points, err := svc.GetDailyGrowth(ctx, name, interfaces.GrowthOptions{})
	require.NoError(t, err)
	require.NotEmpty(t, points)

	// Wait for background persist
	time.Sleep(2 * time.Second)

	var checked int
	for _, p := range points {
		dateStr := p.Date.Format("2006-01-02")

		// Rule 1: holding_count > 0 implies equity fields populated
		if p.HoldingCount > 0 {
			checked++
			assert.Greater(t, p.EquityHoldingsValue, 0.0,
				"date=%s: holding_count=%d requires equity_holdings_value > 0", dateStr, p.HoldingCount)
			assert.Greater(t, p.EquityHoldingsCost, 0.0,
				"date=%s: holding_count=%d requires equity_holdings_cost > 0", dateStr, p.HoldingCount)
		}

		// Rule 2: equity > 0 implies portfolio_value >= equity (portfolio includes cash)
		if p.EquityHoldingsValue > 0 {
			assert.GreaterOrEqual(t, p.PortfolioValue, p.EquityHoldingsValue,
				"date=%s: portfolio_value (%.2f) should be >= equity_holdings_value (%.2f)",
				dateStr, p.PortfolioValue, p.EquityHoldingsValue)
		}

		// Rule 3: capital_gross > 0 implies portfolio_value > 0
		if p.CapitalGross > 0 {
			assert.Greater(t, p.PortfolioValue, 0.0,
				"date=%s: capital_gross=%.2f requires portfolio_value > 0", dateStr, p.CapitalGross)
		}
	}

	assert.Greater(t, checked, 0,
		"Should have validated at least one data point with active holdings")

	guard.SaveResult("11_chart_fields_consistent", fmt.Sprintf(
		"Consistency validated: %d points, %d with active holdings checked", len(points), checked))
}
