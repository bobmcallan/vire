package portfolio

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
)

// Devils-advocate stress tests for the DataVersion check in tryTimelineCache().
// These target edge cases: empty DataVersion (legacy snapshots), future versions,
// concurrent access with stale caches, partial rebuild interruption, and
// unnecessary rebuild loops.

// ---------------------------------------------------------------------------
// Helper: build a Service with a pre-populated timeline store
// ---------------------------------------------------------------------------

func buildCacheTestService(t *testing.T, snapshots map[string][]models.TimelineSnapshot) *Service {
	t.Helper()
	logger := common.NewLogger("error")
	tls := &stubTimelineStore{snapshots: snapshots}
	storage := &stubStorageManager{
		marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
		userDataStore: newMemUserDataStore(),
		timelineStore: tls,
	}
	return NewService(storage, nil, nil, nil, logger)
}

// ---------------------------------------------------------------------------
// 1. Empty DataVersion (old snapshots pre-dating the field)
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_EmptyDataVersion(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -10)

	snaps := []models.TimelineSnapshot{
		{UserID: "u1", PortfolioName: "p1", Date: from, DataVersion: "", PortfolioValue: 1000},
		{UserID: "u1", PortfolioName: "p1", Date: now, DataVersion: "", PortfolioValue: 1100},
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})
	cached, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)

	assert.False(t, ok, "Empty DataVersion must cause cache miss (legacy data)")
	assert.Nil(t, cached)
}

// ---------------------------------------------------------------------------
// 2. Future DataVersion (higher than current SchemaVersion, e.g. downgrade)
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_FutureDataVersion(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -5)

	snaps := []models.TimelineSnapshot{
		{UserID: "u1", PortfolioName: "p1", Date: from, DataVersion: "999", PortfolioValue: 1000},
		{UserID: "u1", PortfolioName: "p1", Date: now, DataVersion: "999", PortfolioValue: 1100},
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})
	cached, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)

	assert.False(t, ok, "Future DataVersion must cause cache miss (version mismatch is bidirectional)")
	assert.Nil(t, cached)
}

// ---------------------------------------------------------------------------
// 3. Stale DataVersion (explicitly old, e.g. "5")
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_StaleDataVersion(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -5)

	snaps := []models.TimelineSnapshot{
		{UserID: "u1", PortfolioName: "p1", Date: from, DataVersion: "5", PortfolioValue: 1000},
		{UserID: "u1", PortfolioName: "p1", Date: now, DataVersion: "5", PortfolioValue: 1100},
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})
	cached, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)

	assert.False(t, ok, "Stale DataVersion must cause cache miss")
	assert.Nil(t, cached)
}

// ---------------------------------------------------------------------------
// 4. Current DataVersion should still be a cache hit (sanity check)
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_CurrentVersionHit(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -5)

	snaps := make([]models.TimelineSnapshot, 0, 6)
	for i := 0; i <= 5; i++ {
		snaps = append(snaps, models.TimelineSnapshot{
			UserID:         "u1",
			PortfolioName:  "p1",
			Date:           from.AddDate(0, 0, i),
			DataVersion:    common.SchemaVersion,
			PortfolioValue: float64(1000 + i*10),
		})
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})
	cached, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)

	assert.True(t, ok, "Current DataVersion must be a cache hit")
	assert.Len(t, cached, 6)
}

// ---------------------------------------------------------------------------
// 5. Mixed DataVersion snapshots — latest has stale version
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_MixedVersions_LatestStale(t *testing.T) {
	// The DataVersion check is on GetLatest, not per-snapshot. If the latest
	// snapshot is stale, the entire cache must be rejected even if some snapshots
	// have the current version (indicates partial/interrupted rebuild).
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -3)

	snaps := []models.TimelineSnapshot{
		{UserID: "u1", PortfolioName: "p1", Date: from, DataVersion: common.SchemaVersion, PortfolioValue: 1000},
		{UserID: "u1", PortfolioName: "p1", Date: from.AddDate(0, 0, 1), DataVersion: common.SchemaVersion, PortfolioValue: 1010},
		// Latest snapshot is stale — indicates interrupted rebuild
		{UserID: "u1", PortfolioName: "p1", Date: now, DataVersion: "5", PortfolioValue: 1020},
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})
	cached, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)

	assert.False(t, ok, "Latest snapshot stale => whole cache rejected")
	assert.Nil(t, cached)
}

// ---------------------------------------------------------------------------
// 6. Concurrent tryTimelineCache calls with stale cache
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_ConcurrentStaleAccess(t *testing.T) {
	// When cache is stale, multiple concurrent GetDailyGrowth calls will all
	// get cache misses. Verify no panics, data races, or inconsistencies.
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -5)

	snaps := make([]models.TimelineSnapshot, 0, 6)
	for i := 0; i <= 5; i++ {
		snaps = append(snaps, models.TimelineSnapshot{
			UserID:         "u1",
			PortfolioName:  "p1",
			Date:           from.AddDate(0, 0, i),
			DataVersion:    "old-version",
			PortfolioValue: float64(1000 + i*10),
		})
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})

	var wg sync.WaitGroup
	var missCount atomic.Int64
	goroutines := 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)
			if !ok {
				missCount.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(goroutines), missCount.Load(),
		"All concurrent calls must get cache miss for stale data (no race-based false hits)")
}

// ---------------------------------------------------------------------------
// 7. Concurrent tryTimelineCache with current cache
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_ConcurrentCurrentAccess(t *testing.T) {
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -5)

	snaps := make([]models.TimelineSnapshot, 0, 6)
	for i := 0; i <= 5; i++ {
		snaps = append(snaps, models.TimelineSnapshot{
			UserID:         "u1",
			PortfolioName:  "p1",
			Date:           from.AddDate(0, 0, i),
			DataVersion:    common.SchemaVersion,
			PortfolioValue: float64(1000 + i*10),
		})
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})

	var wg sync.WaitGroup
	var hitCount atomic.Int64
	goroutines := 50

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pts, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)
			if ok && len(pts) > 0 {
				hitCount.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(goroutines), hitCount.Load(),
		"All concurrent calls must get cache hit for current data")
}

// ---------------------------------------------------------------------------
// 8. persistTimelineSnapshots failure does not cause infinite rebuild loop
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_PersistFailure_NoPanic(t *testing.T) {
	// If persistTimelineSnapshots fails (e.g. DB error), the next call to
	// tryTimelineCache should still operate normally (return stale miss or
	// whatever the store returns). It must NOT enter an infinite loop.
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -3)

	// Empty store — no snapshots at all (simulating persist failure leaving nothing)
	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{})

	// Multiple calls — must all return quickly with cache miss
	for i := 0; i < 10; i++ {
		cached, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)
		assert.False(t, ok, "No snapshots => cache miss (iteration %d)", i)
		assert.Nil(t, cached)
	}
}

// ---------------------------------------------------------------------------
// 9. DataVersion check happens BEFORE expensive GetRange call
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_StaleVersion_SkipsGetRange(t *testing.T) {
	// Verify that when DataVersion is stale on the latest snapshot, we don't
	// waste time fetching the full range. We use a counting timeline store.
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -5)

	var getRangeCount atomic.Int64

	tls := &countingTimelineStore{
		inner: &stubTimelineStore{
			snapshots: map[string][]models.TimelineSnapshot{
				"p1": {
					{UserID: "u1", PortfolioName: "p1", Date: from, DataVersion: "old", PortfolioValue: 1000},
					{UserID: "u1", PortfolioName: "p1", Date: now, DataVersion: "old", PortfolioValue: 1100},
				},
			},
		},
		getRangeCount: &getRangeCount,
	}

	logger := common.NewLogger("error")
	storage := &countingTimelineStorageManager{
		stubStorageManager: stubStorageManager{
			marketStore:   &stubMarketDataStorage{data: map[string]*models.MarketData{}},
			userDataStore: newMemUserDataStore(),
		},
		tls: tls,
	}
	svc := NewService(storage, nil, nil, nil, logger)

	_, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)
	assert.False(t, ok, "Stale version must cause cache miss")
	assert.Equal(t, int64(0), getRangeCount.Load(),
		"GetRange must NOT be called when DataVersion is stale (early return optimization)")
}

// ---------------------------------------------------------------------------
// 10. SchemaVersion boundary: version just before current
// ---------------------------------------------------------------------------

func TestStress_TryTimelineCache_PreviousSchemaVersion(t *testing.T) {
	// Even if DataVersion is SchemaVersion-1, it must still be rejected.
	// SchemaVersion is a string, but the check should be equality, not numeric.
	now := time.Now().Truncate(24 * time.Hour)
	from := now.AddDate(0, 0, -3)

	// Use current version minus 1 (as string)
	prevVersion := "14" // SchemaVersion is "15"
	snaps := []models.TimelineSnapshot{
		{UserID: "u1", PortfolioName: "p1", Date: from, DataVersion: prevVersion, PortfolioValue: 1000},
		{UserID: "u1", PortfolioName: "p1", Date: now, DataVersion: prevVersion, PortfolioValue: 1100},
	}

	svc := buildCacheTestService(t, map[string][]models.TimelineSnapshot{"p1": snaps})
	_, ok := svc.tryTimelineCache(context.Background(), "u1", "p1", from, now)

	assert.False(t, ok, "Previous SchemaVersion (one behind) must still cause cache miss")
}

// ---------------------------------------------------------------------------
// countingTimelineStorageManager: overrides TimelineStore() to return a
// countingTimelineStore instead of the embedded stubTimelineStore.
// ---------------------------------------------------------------------------

type countingTimelineStorageManager struct {
	stubStorageManager
	tls *countingTimelineStore
}

func (c *countingTimelineStorageManager) TimelineStore() interfaces.TimelineStore {
	return c.tls
}

// ---------------------------------------------------------------------------
// countingTimelineStore: wraps stubTimelineStore and counts GetRange calls
// ---------------------------------------------------------------------------

type countingTimelineStore struct {
	inner         *stubTimelineStore
	getRangeCount *atomic.Int64
}

func (c *countingTimelineStore) GetRange(ctx context.Context, userID, portfolioName string, from, to time.Time) ([]models.TimelineSnapshot, error) {
	c.getRangeCount.Add(1)
	return c.inner.GetRange(ctx, userID, portfolioName, from, to)
}

func (c *countingTimelineStore) GetLatest(ctx context.Context, userID, portfolioName string) (*models.TimelineSnapshot, error) {
	return c.inner.GetLatest(ctx, userID, portfolioName)
}

func (c *countingTimelineStore) SaveBatch(ctx context.Context, snapshots []models.TimelineSnapshot) error {
	return c.inner.SaveBatch(ctx, snapshots)
}

func (c *countingTimelineStore) DeleteRange(ctx context.Context, userID, portfolioName string, from, to time.Time) (int, error) {
	return c.inner.DeleteRange(ctx, userID, portfolioName, from, to)
}

func (c *countingTimelineStore) DeleteAll(ctx context.Context, userID, portfolioName string) (int, error) {
	return c.inner.DeleteAll(ctx, userID, portfolioName)
}
