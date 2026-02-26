package surrealdb

import (
	"context"
	"strconv"
	"testing"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedbackStore_CreateAndGet(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryDataAnomaly,
		Severity:    models.FeedbackSeverityHigh,
		Description: "BHP price is negative",
		Ticker:      "BHP.AU",
	}

	err := store.Create(ctx, fb)
	require.NoError(t, err)
	assert.NotEmpty(t, fb.ID)
	assert.Contains(t, fb.ID, "fb_")
	assert.Equal(t, models.FeedbackStatusNew, fb.Status)

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, fb.ID, got.ID)
	assert.Equal(t, "BHP.AU", got.Ticker)
	assert.Equal(t, models.FeedbackSeverityHigh, got.Severity)
}

func TestFeedbackStore_GetNotFound(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	got, err := store.Get(ctx, "fb_nonexistent")
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestFeedbackStore_List(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	for i, cat := range []string{models.FeedbackCategoryDataAnomaly, models.FeedbackCategoryToolError, models.FeedbackCategoryObservation} {
		fb := &models.Feedback{
			Category:    cat,
			Description: "Test " + strconv.Itoa(i),
			Severity:    models.FeedbackSeverityMedium,
		}
		require.NoError(t, store.Create(ctx, fb))
	}

	items, total, err := store.List(ctx, interfaces.FeedbackListOptions{})
	require.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Len(t, items, 3)
}

func TestFeedbackStore_ListWithFilters(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	for _, cat := range []string{models.FeedbackCategoryDataAnomaly, models.FeedbackCategoryToolError, models.FeedbackCategoryDataAnomaly} {
		fb := &models.Feedback{
			Category:    cat,
			Description: "Test for " + cat,
		}
		require.NoError(t, store.Create(ctx, fb))
	}

	items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
		Category: models.FeedbackCategoryDataAnomaly,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, items, 2)
}

func TestFeedbackStore_ListPagination(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		fb := &models.Feedback{
			Category:    models.FeedbackCategoryObservation,
			Description: "Entry " + strconv.Itoa(i),
		}
		require.NoError(t, store.Create(ctx, fb))
	}

	items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
		Page:    1,
		PerPage: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, 5, total)
	assert.Len(t, items, 2)

	items, _, err = store.List(ctx, interfaces.FeedbackListOptions{
		Page:    3,
		PerPage: 2,
	})
	require.NoError(t, err)
	assert.Len(t, items, 1)
}

func TestFeedbackStore_Update(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryDataAnomaly,
		Description: "Something wrong",
	}
	require.NoError(t, store.Create(ctx, fb))

	err := store.Update(ctx, fb.ID, models.FeedbackStatusAcknowledged, "Looking into it", "", "", "")
	require.NoError(t, err)

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FeedbackStatusAcknowledged, got.Status)
	assert.Equal(t, "Looking into it", got.ResolutionNotes)
}

func TestFeedbackStore_BulkUpdateStatus(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	var ids []string
	for i := 0; i < 3; i++ {
		fb := &models.Feedback{
			Category:    models.FeedbackCategoryObservation,
			Description: "Bulk " + strconv.Itoa(i),
		}
		require.NoError(t, store.Create(ctx, fb))
		ids = append(ids, fb.ID)
	}

	updated, err := store.BulkUpdateStatus(ctx, ids, models.FeedbackStatusDismissed, "Not actionable")
	require.NoError(t, err)
	assert.Equal(t, 3, updated)

	for _, id := range ids {
		got, err := store.Get(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, models.FeedbackStatusDismissed, got.Status)
	}
}

func TestFeedbackStore_Delete(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryToolError,
		Description: "Delete me",
	}
	require.NoError(t, store.Create(ctx, fb))

	err := store.Delete(ctx, fb.ID)
	require.NoError(t, err)

	got, err := store.Get(ctx, fb.ID)
	assert.NoError(t, err)
	assert.Nil(t, got)
}

func TestFeedbackStore_Summary(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	entries := []struct {
		cat    string
		sev    string
		status string
	}{
		{models.FeedbackCategoryDataAnomaly, models.FeedbackSeverityHigh, models.FeedbackStatusNew},
		{models.FeedbackCategoryToolError, models.FeedbackSeverityMedium, models.FeedbackStatusNew},
		{models.FeedbackCategoryObservation, models.FeedbackSeverityLow, models.FeedbackStatusResolved},
	}

	for _, e := range entries {
		fb := &models.Feedback{
			Category:    e.cat,
			Severity:    e.sev,
			Description: "Summary test",
			Status:      e.status,
		}
		require.NoError(t, store.Create(ctx, fb))
	}

	summary, err := store.Summary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 3, summary.Total)
	assert.Equal(t, 2, summary.ByStatus[models.FeedbackStatusNew])
	assert.Equal(t, 1, summary.ByStatus[models.FeedbackStatusResolved])
	assert.Equal(t, 1, summary.BySeverity[models.FeedbackSeverityHigh])
	assert.Equal(t, 1, summary.BySeverity[models.FeedbackSeverityMedium])
	assert.Equal(t, 1, summary.BySeverity[models.FeedbackSeverityLow])
	assert.NotNil(t, summary.OldestUnresolved)
}

func TestFeedbackStore_DefaultValues(t *testing.T) {
	db := testDB(t)
	store := NewFeedbackStore(db, testLogger())
	ctx := context.Background()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "No severity or status set",
	}
	require.NoError(t, store.Create(ctx, fb))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FeedbackSeverityMedium, got.Severity)
	assert.Equal(t, models.FeedbackStatusNew, got.Status)
	assert.False(t, got.CreatedAt.IsZero())
	assert.False(t, got.UpdatedAt.IsZero())
}
