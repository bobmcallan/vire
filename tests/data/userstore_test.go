package data

import (
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecordLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	record := &models.UserRecord{
		UserID:   "rlc_user",
		Subject:  "portfolio",
		Key:      "smsf",
		Value:    `{"name":"SMSF Growth","tickers":["BHP","CBA"]}`,
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}

	// Put
	require.NoError(t, store.Put(ctx, record))

	// Get
	got, err := store.Get(ctx, "rlc_user", "portfolio", "smsf")
	require.NoError(t, err)
	assert.Equal(t, record.Value, got.Value)
	assert.Equal(t, 1, got.Version)

	// List
	records, err := store.List(ctx, "rlc_user", "portfolio")
	require.NoError(t, err)
	assert.Len(t, records, 1)

	// Update
	record.Value = `{"name":"SMSF Growth V2"}`
	record.Version = 2
	require.NoError(t, store.Put(ctx, record))

	updated, err := store.Get(ctx, "rlc_user", "portfolio", "smsf")
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)

	// Delete
	require.NoError(t, store.Delete(ctx, "rlc_user", "portfolio", "smsf"))
	_, err = store.Get(ctx, "rlc_user", "portfolio", "smsf")
	assert.Error(t, err)
}

func TestMultipleSubjects(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	subjects := []string{"portfolio", "strategy", "plan", "watchlist", "report", "search"}
	for _, sub := range subjects {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "multisub_user",
			Subject:  sub,
			Key:      "item1",
			Value:    fmt.Sprintf(`{"%s":"data"}`, sub),
			Version:  1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}

	for _, sub := range subjects {
		t.Run(sub, func(t *testing.T) {
			records, err := store.List(ctx, "multisub_user", sub)
			require.NoError(t, err)
			require.Len(t, records, 1)
			assert.Equal(t, sub, records[0].Subject)
		})
	}
}

func TestQueryOrdering(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "queryord_user",
			Subject:  "report",
			Key:      fmt.Sprintf("report_%d", i),
			Value:    fmt.Sprintf("report_%d", i),
			Version:  1,
			DateTime: base.Add(time.Duration(i) * 24 * time.Hour),
		}))
	}

	t.Run("ascending", func(t *testing.T) {
		records, err := store.Query(ctx, "queryord_user", "report", interfaces.QueryOptions{
			OrderBy: "datetime_asc",
		})
		require.NoError(t, err)
		require.Len(t, records, 5)
		for i := 1; i < len(records); i++ {
			assert.True(t, !records[i].DateTime.Before(records[i-1].DateTime),
				"record %d should be >= record %d", i, i-1)
		}
	})

	t.Run("descending", func(t *testing.T) {
		records, err := store.Query(ctx, "queryord_user", "report", interfaces.QueryOptions{
			OrderBy: "datetime_desc",
		})
		require.NoError(t, err)
		require.Len(t, records, 5)
		for i := 1; i < len(records); i++ {
			assert.True(t, !records[i].DateTime.After(records[i-1].DateTime),
				"record %d should be <= record %d", i, i-1)
		}
	})

	t.Run("with limit", func(t *testing.T) {
		records, err := store.Query(ctx, "queryord_user", "report", interfaces.QueryOptions{
			Limit: 3,
		})
		require.NoError(t, err)
		assert.Len(t, records, 3)
	})
}

func TestDeleteBySubject(t *testing.T) {
	mgr := testManager(t)
	store := mgr.UserDataStore()
	ctx := testContext()

	// Create records across multiple subjects
	for _, key := range []string{"a", "b", "c"} {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "dbs_user",
			Subject:  "report",
			Key:      key,
			Value:    "data",
			Version:  1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "dbs_user",
		Subject:  "strategy",
		Key:      "s1",
		Value:    "keep",
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	count, err := store.DeleteBySubject(ctx, "report")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Strategy should remain
	got, err := store.Get(ctx, "dbs_user", "strategy", "s1")
	require.NoError(t, err)
	assert.Equal(t, "keep", got.Value)
}
