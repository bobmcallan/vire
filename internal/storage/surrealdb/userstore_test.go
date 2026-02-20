package surrealdb

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserStoreGet(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	record := &models.UserRecord{
		UserID:   "user1",
		Subject:  "portfolio",
		Key:      "smsf",
		Value:    `{"name":"SMSF Growth"}`,
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "user1", "portfolio", "smsf")
	require.NoError(t, err)
	assert.Equal(t, "user1", got.UserID)
	assert.Equal(t, "portfolio", got.Subject)
	assert.Equal(t, "smsf", got.Key)
	assert.Equal(t, `{"name":"SMSF Growth"}`, got.Value)
	assert.Equal(t, 1, got.Version)
}

func TestUserStoreGetNotFound(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	_, err := store.Get(ctx, "nobody", "portfolio", "nokey")
	assert.Error(t, err)
}

func TestUserStorePut(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	tests := []struct {
		name   string
		record *models.UserRecord
	}{
		{
			name: "portfolio record",
			record: &models.UserRecord{
				UserID:   "putuser",
				Subject:  "portfolio",
				Key:      "growth",
				Value:    `{"tickers":["BHP","CBA"]}`,
				Version:  1,
				DateTime: time.Now().Truncate(time.Second),
			},
		},
		{
			name: "strategy record",
			record: &models.UserRecord{
				UserID:   "putuser",
				Subject:  "strategy",
				Key:      "value_investing",
				Value:    `{"filters":{"pe_max":15}}`,
				Version:  1,
				DateTime: time.Now().Truncate(time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.Put(ctx, tt.record)
			require.NoError(t, err)

			got, err := store.Get(ctx, tt.record.UserID, tt.record.Subject, tt.record.Key)
			require.NoError(t, err)
			assert.Equal(t, tt.record.Value, got.Value)
		})
	}
}

func TestUserStorePutOverwrite(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	record := &models.UserRecord{
		UserID:   "overwrite",
		Subject:  "portfolio",
		Key:      "main",
		Value:    "v1",
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	record.Value = "v2"
	record.Version = 2
	require.NoError(t, store.Put(ctx, record))

	got, err := store.Get(ctx, "overwrite", "portfolio", "main")
	require.NoError(t, err)
	assert.Equal(t, "v2", got.Value)
	assert.Equal(t, 2, got.Version)
}

func TestUserStoreDelete(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	record := &models.UserRecord{
		UserID:   "deluser",
		Subject:  "portfolio",
		Key:      "todelete",
		Value:    "data",
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.Put(ctx, record))

	err := store.Delete(ctx, "deluser", "portfolio", "todelete")
	require.NoError(t, err)

	_, err = store.Get(ctx, "deluser", "portfolio", "todelete")
	assert.Error(t, err)
}

func TestUserStoreList(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	for i, key := range []string{"a", "b", "c"} {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "listuser",
			Subject:  "strategy",
			Key:      key,
			Value:    key + "_data",
			Version:  i + 1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}

	records, err := store.List(ctx, "listuser", "strategy")
	require.NoError(t, err)
	assert.Len(t, records, 3)
}

func TestUserStoreQuery(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "queryuser",
			Subject:  "report",
			Key:      fmt.Sprintf("report_%d", i),
			Value:    fmt.Sprintf("report_data_%d", i),
			Version:  1,
			DateTime: base.Add(time.Duration(i) * 24 * time.Hour),
		}))
	}

	t.Run("default descending", func(t *testing.T) {
		records, err := store.Query(ctx, "queryuser", "report", interfaces.QueryOptions{})
		require.NoError(t, err)
		assert.Len(t, records, 5)
		// Default is datetime_desc
		if len(records) >= 2 {
			assert.True(t, records[0].DateTime.After(records[1].DateTime) || records[0].DateTime.Equal(records[1].DateTime))
		}
	})

	t.Run("ascending order", func(t *testing.T) {
		records, err := store.Query(ctx, "queryuser", "report", interfaces.QueryOptions{
			OrderBy: "datetime_asc",
		})
		require.NoError(t, err)
		assert.Len(t, records, 5)
		if len(records) >= 2 {
			assert.True(t, records[0].DateTime.Before(records[1].DateTime) || records[0].DateTime.Equal(records[1].DateTime))
		}
	})

	t.Run("with limit", func(t *testing.T) {
		records, err := store.Query(ctx, "queryuser", "report", interfaces.QueryOptions{
			Limit: 2,
		})
		require.NoError(t, err)
		assert.Len(t, records, 2)
	})
}

func TestUserStoreDeleteBySubject(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	for _, key := range []string{"r1", "r2", "r3"} {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "delsub",
			Subject:  "report",
			Key:      key,
			Value:    "data",
			Version:  1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}
	// Also add a record with different subject to verify it's not deleted
	require.NoError(t, store.Put(ctx, &models.UserRecord{
		UserID:   "delsub",
		Subject:  "strategy",
		Key:      "s1",
		Value:    "keep_me",
		Version:  1,
		DateTime: time.Now().Truncate(time.Second),
	}))

	count, err := store.DeleteBySubject(ctx, "report")
	require.NoError(t, err)
	assert.Equal(t, 3, count)

	// Verify strategy record still exists
	got, err := store.Get(ctx, "delsub", "strategy", "s1")
	require.NoError(t, err)
	assert.Equal(t, "keep_me", got.Value)
}

func TestUserStoreDeleteBySubjects(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	for _, sub := range []string{"portfolio", "report", "search"} {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "delmulti",
			Subject:  sub,
			Key:      "item1",
			Value:    "data",
			Version:  1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}

	total, err := store.DeleteBySubjects(ctx, "portfolio", "report", "search")
	require.NoError(t, err)
	assert.Equal(t, 3, total)
}

func TestUserStoreMultipleSubjects(t *testing.T) {
	db := testDB(t)
	store := NewUserStore(db, testLogger())
	ctx := context.Background()

	subjects := []string{"portfolio", "strategy", "plan", "watchlist", "report", "search"}
	for _, sub := range subjects {
		require.NoError(t, store.Put(ctx, &models.UserRecord{
			UserID:   "multisub",
			Subject:  sub,
			Key:      "item",
			Value:    sub + "_data",
			Version:  1,
			DateTime: time.Now().Truncate(time.Second),
		}))
	}

	for _, sub := range subjects {
		t.Run(sub, func(t *testing.T) {
			got, err := store.Get(ctx, "multisub", sub, "item")
			require.NoError(t, err)
			assert.Equal(t, sub+"_data", got.Value)
		})
	}
}
