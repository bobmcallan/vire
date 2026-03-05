package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	tcommon "github.com/bobmcallan/vire/tests/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTimelineSnapshot(userID, name string, date time.Time, equityValue float64) models.TimelineSnapshot {
	return models.TimelineSnapshot{
		UserID:                  userID,
		PortfolioName:           name,
		Date:                    date,
		EquityHoldingsValue:     equityValue,
		EquityHoldingsCost:      equityValue * 0.8,
		EquityHoldingsReturn:    equityValue * 0.2,
		EquityHoldingsReturnPct: 25.0,
		HoldingCount:            5,
		CapitalGross:            10000,
		CapitalAvailable:        2000,
		PortfolioValue:          equityValue + 2000,
		CapitalContributionsNet: equityValue * 0.8,
		DataVersion:             "13",
		ComputedAt:              time.Now(),
	}
}

func TestTimelineStore_Lifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	// Save batch
	snapshots := []models.TimelineSnapshot{
		makeTimelineSnapshot("user1", "smsf", d1, 100000),
		makeTimelineSnapshot("user1", "smsf", d2, 101000),
		makeTimelineSnapshot("user1", "smsf", d3, 102000),
	}
	err := store.SaveBatch(ctx, snapshots)
	require.NoError(t, err, "SaveBatch should succeed")
	guard.SaveResult("01_save_batch", fmt.Sprintf("saved %d snapshots", len(snapshots)))

	// GetRange: full range
	got, err := store.GetRange(ctx, "user1", "smsf", d1, d3)
	require.NoError(t, err)
	require.Len(t, got, 3, "GetRange full range should return 3 snapshots")
	assert.Equal(t, 100000.0, got[0].EquityHoldingsValue)
	assert.Equal(t, 102000.0, got[2].EquityHoldingsValue)
	guard.SaveResult("02_get_range_full", fmt.Sprintf("returned %d snapshots, first=%.0f last=%.0f", len(got), got[0].EquityHoldingsValue, got[2].EquityHoldingsValue))

	// GetRange: subset
	subset, err := store.GetRange(ctx, "user1", "smsf", d2, d2)
	require.NoError(t, err)
	require.Len(t, subset, 1, "GetRange single date should return 1 snapshot")
	assert.Equal(t, 101000.0, subset[0].EquityHoldingsValue)

	// GetLatest
	latest, err := store.GetLatest(ctx, "user1", "smsf")
	require.NoError(t, err)
	require.NotNil(t, latest, "GetLatest should return non-nil")
	assert.Equal(t, 102000.0, latest.EquityHoldingsValue)
	guard.SaveResult("03_get_latest", fmt.Sprintf("latest equity_value=%.0f", latest.EquityHoldingsValue))

	// Upsert: overwrite d2 with new value
	updated := makeTimelineSnapshot("user1", "smsf", d2, 115000)
	err = store.SaveBatch(ctx, []models.TimelineSnapshot{updated})
	require.NoError(t, err)

	upserted, err := store.GetRange(ctx, "user1", "smsf", d2, d2)
	require.NoError(t, err)
	require.Len(t, upserted, 1)
	assert.Equal(t, 115000.0, upserted[0].EquityHoldingsValue, "Upsert should overwrite existing value")
	guard.SaveResult("04_upsert", fmt.Sprintf("upserted d2 to equity_value=%.0f", upserted[0].EquityHoldingsValue))

	// DeleteRange: remove d2 only
	_, err = store.DeleteRange(ctx, "user1", "smsf", d2, d2)
	require.NoError(t, err)

	afterDelete, err := store.GetRange(ctx, "user1", "smsf", d1, d3)
	require.NoError(t, err)
	assert.Len(t, afterDelete, 2, "DeleteRange should remove only d2")
	guard.SaveResult("05_delete_range", fmt.Sprintf("after deleting d2: %d snapshots remain", len(afterDelete)))

	// DeleteAll
	_, err = store.DeleteAll(ctx, "user1", "smsf")
	require.NoError(t, err)

	empty, err := store.GetRange(ctx, "user1", "smsf", d1, d3)
	require.NoError(t, err)
	assert.Empty(t, empty, "DeleteAll should remove all snapshots")
	guard.SaveResult("06_delete_all", "all snapshots deleted")
}

func TestTimelineStore_UserIsolation(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeTimelineSnapshot("alice", "smsf", d1, 100000),
		makeTimelineSnapshot("bob", "smsf", d1, 200000),
	}
	require.NoError(t, store.SaveBatch(ctx, snapshots))

	// Alice sees only her data
	aliceData, err := store.GetRange(ctx, "alice", "smsf", d1, d1)
	require.NoError(t, err)
	require.Len(t, aliceData, 1)
	assert.Equal(t, 100000.0, aliceData[0].EquityHoldingsValue)

	// Bob sees only his data
	bobData, err := store.GetRange(ctx, "bob", "smsf", d1, d1)
	require.NoError(t, err)
	require.Len(t, bobData, 1)
	assert.Equal(t, 200000.0, bobData[0].EquityHoldingsValue)

	guard.SaveResult("01_user_isolation", fmt.Sprintf("alice=%.0f bob=%.0f", aliceData[0].EquityHoldingsValue, bobData[0].EquityHoldingsValue))

	// Deleting alice's data doesn't affect bob
	_, err = store.DeleteAll(ctx, "alice", "smsf")
	require.NoError(t, err)

	bobStillExists, err := store.GetRange(ctx, "bob", "smsf", d1, d1)
	require.NoError(t, err)
	assert.Len(t, bobStillExists, 1, "Bob's data should survive Alice's DeleteAll")
	guard.SaveResult("02_delete_isolation", "alice deleted, bob unaffected")
}

func TestTimelineStore_PortfolioIsolation(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeTimelineSnapshot("user1", "smsf", d1, 100000),
		makeTimelineSnapshot("user1", "personal", d1, 50000),
	}
	require.NoError(t, store.SaveBatch(ctx, snapshots))

	smsf, err := store.GetRange(ctx, "user1", "smsf", d1, d1)
	require.NoError(t, err)
	require.Len(t, smsf, 1)
	assert.Equal(t, 100000.0, smsf[0].EquityHoldingsValue)

	personal, err := store.GetRange(ctx, "user1", "personal", d1, d1)
	require.NoError(t, err)
	require.Len(t, personal, 1)
	assert.Equal(t, 50000.0, personal[0].EquityHoldingsValue)

	guard.SaveResult("01_portfolio_isolation", fmt.Sprintf("smsf=%.0f personal=%.0f", smsf[0].EquityHoldingsValue, personal[0].EquityHoldingsValue))
}

func TestTimelineStore_EmptyResults(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	// GetLatest on empty store
	latest, err := store.GetLatest(ctx, "nobody", "empty")
	require.NoError(t, err)
	assert.Nil(t, latest, "GetLatest on empty store should return nil")

	// GetRange on empty store
	empty, err := store.GetRange(ctx, "nobody", "empty", time.Now().AddDate(-1, 0, 0), time.Now())
	require.NoError(t, err)
	assert.Empty(t, empty, "GetRange on empty store should return empty slice")

	// SaveBatch with nil/empty slices
	require.NoError(t, store.SaveBatch(ctx, nil), "SaveBatch nil should not error")
	require.NoError(t, store.SaveBatch(ctx, []models.TimelineSnapshot{}), "SaveBatch empty should not error")

	guard.SaveResult("01_empty_results", "all empty-state operations succeeded")
}

func TestTimelineStore_LargeBatch(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	// Simulate ~3 years of daily data (1095 days)
	const days = 1095
	start := time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)

	snapshots := make([]models.TimelineSnapshot, days)
	for i := range days {
		d := start.AddDate(0, 0, i)
		snapshots[i] = makeTimelineSnapshot("user1", "smsf", d, 100000+float64(i)*100)
	}

	startTime := time.Now()
	err := store.SaveBatch(ctx, snapshots)
	saveDuration := time.Since(startTime)
	require.NoError(t, err, "SaveBatch of %d snapshots should succeed", days)
	guard.SaveResult("01_save_large_batch", fmt.Sprintf("saved %d snapshots in %v", days, saveDuration))

	// Verify full range query
	startTime = time.Now()
	end := start.AddDate(0, 0, days-1)
	got, err := store.GetRange(ctx, "user1", "smsf", start, end)
	queryDuration := time.Since(startTime)
	require.NoError(t, err)
	assert.Len(t, got, days, "GetRange should return all %d snapshots", days)
	guard.SaveResult("02_query_large_range", fmt.Sprintf("queried %d snapshots in %v", len(got), queryDuration))

	// Verify ordering: first date should be earliest, last should be latest
	assert.True(t, got[0].Date.Before(got[len(got)-1].Date), "Results should be sorted ascending by date")

	// GetLatest should return the most recent
	latest, err := store.GetLatest(ctx, "user1", "smsf")
	require.NoError(t, err)
	assert.Equal(t, end, latest.Date.Truncate(24*time.Hour), "GetLatest should return the last day")
	guard.SaveResult("03_latest_after_large_batch", fmt.Sprintf("latest date=%s equity=%.0f", latest.Date.Format("2006-01-02"), latest.EquityHoldingsValue))
}

func TestTimelineStore_FieldPersistence(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	d := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	snap := models.TimelineSnapshot{
		UserID:                  "user1",
		PortfolioName:           "smsf",
		Date:                    d,
		EquityHoldingsValue:     150000.50,
		EquityHoldingsCost:      120000.25,
		EquityHoldingsReturn:    30000.25,
		EquityHoldingsReturnPct: 25.0,
		HoldingCount:            12,
		CapitalGross:            45000.75,
		CapitalAvailable:        15000.50,
		PortfolioValue:          165001.00,
		CapitalContributionsNet: 135000.00,
		FXRate:                  0.6543,
		DataVersion:             "13",
		ComputedAt:              time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC),
	}

	require.NoError(t, store.SaveBatch(ctx, []models.TimelineSnapshot{snap}))

	got, err := store.GetRange(ctx, "user1", "smsf", d, d)
	require.NoError(t, err)
	require.Len(t, got, 1)

	g := got[0]
	assert.Equal(t, "user1", g.UserID)
	assert.Equal(t, "smsf", g.PortfolioName)
	assert.InDelta(t, 150000.50, g.EquityHoldingsValue, 0.01)
	assert.InDelta(t, 120000.25, g.EquityHoldingsCost, 0.01)
	assert.InDelta(t, 30000.25, g.EquityHoldingsReturn, 0.01)
	assert.InDelta(t, 25.0, g.EquityHoldingsReturnPct, 0.01)
	assert.Equal(t, 12, g.HoldingCount)
	assert.InDelta(t, 45000.75, g.CapitalGross, 0.01)
	assert.InDelta(t, 15000.50, g.CapitalAvailable, 0.01)
	assert.InDelta(t, 165001.00, g.PortfolioValue, 0.01)
	assert.InDelta(t, 135000.00, g.CapitalContributionsNet, 0.01)
	assert.InDelta(t, 0.6543, g.FXRate, 0.0001)
	assert.Equal(t, "13", g.DataVersion)

	guard.SaveResult("01_field_persistence", fmt.Sprintf("all %d fields round-tripped correctly", 13))
}

func TestTimelineStore_PurgeDerivedData(t *testing.T) {
	mgr := testManager(t)
	store := mgr.TimelineStore()
	ctx := testContext()
	guard := tcommon.NewTestOutputGuard(t)

	d := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	snap := makeTimelineSnapshot("user1", "smsf", d, 100000)
	require.NoError(t, store.SaveBatch(ctx, []models.TimelineSnapshot{snap}))

	// Verify data exists
	got, err := store.GetRange(ctx, "user1", "smsf", d, d)
	require.NoError(t, err)
	require.Len(t, got, 1)

	// Purge derived data
	_, err = mgr.PurgeDerivedData(ctx)
	require.NoError(t, err)

	// Timeline data should be cleared
	afterPurge, err := store.GetRange(ctx, "user1", "smsf", d, d)
	require.NoError(t, err)
	assert.Empty(t, afterPurge, "Timeline data should be purged by PurgeDerivedData")

	guard.SaveResult("01_purge", "timeline data cleared by PurgeDerivedData")
}
