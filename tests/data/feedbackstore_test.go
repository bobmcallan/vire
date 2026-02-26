package data

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedbackLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	// Create
	fb := &models.Feedback{
		SessionID:   "sess_001",
		ClientType:  "claude-desktop",
		Category:    models.FeedbackCategoryDataAnomaly,
		Severity:    models.FeedbackSeverityHigh,
		Description: "Price divergence for BHP.AX",
		Ticker:      "BHP.AX",
		ToolName:    "get_portfolio",
	}
	require.NoError(t, store.Create(ctx, fb))
	assert.NotEmpty(t, fb.ID, "ID should be auto-generated")
	assert.True(t, len(fb.ID) > 3 && fb.ID[:3] == "fb_", "ID should start with fb_ prefix")
	assert.Equal(t, models.FeedbackStatusNew, fb.Status, "default status should be 'new'")
	assert.False(t, fb.CreatedAt.IsZero(), "created_at should be set")
	assert.False(t, fb.UpdatedAt.IsZero(), "updated_at should be set")

	// Read
	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, fb.ID, got.ID)
	assert.Equal(t, "sess_001", got.SessionID)
	assert.Equal(t, models.FeedbackCategoryDataAnomaly, got.Category)
	assert.Equal(t, models.FeedbackSeverityHigh, got.Severity)
	assert.Equal(t, "Price divergence for BHP.AX", got.Description)
	assert.Equal(t, "BHP.AX", got.Ticker)
	assert.Equal(t, "get_portfolio", got.ToolName)
	assert.Equal(t, models.FeedbackStatusNew, got.Status)

	// Update
	require.NoError(t, store.Update(ctx, fb.ID, models.FeedbackStatusAcknowledged, "Looking into it", "", "", ""))

	updated, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, models.FeedbackStatusAcknowledged, updated.Status)
	assert.Equal(t, "Looking into it", updated.ResolutionNotes)

	// Delete
	require.NoError(t, store.Delete(ctx, fb.ID))

	deleted, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted, "feedback should be nil after deletion")
}

func TestFeedbackList(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	// Create several entries with different attributes
	entries := []models.Feedback{
		{Category: models.FeedbackCategoryDataAnomaly, Severity: models.FeedbackSeverityHigh, Description: "anomaly 1", Ticker: "BHP.AX", SessionID: "sess_a"},
		{Category: models.FeedbackCategoryDataAnomaly, Severity: models.FeedbackSeverityLow, Description: "anomaly 2", Ticker: "CBA.AX", SessionID: "sess_b"},
		{Category: models.FeedbackCategorySyncDelay, Severity: models.FeedbackSeverityMedium, Description: "delay 1", Ticker: "BHP.AX", SessionID: "sess_a"},
		{Category: models.FeedbackCategoryToolError, Severity: models.FeedbackSeverityHigh, Description: "tool error", PortfolioName: "smsf", SessionID: "sess_c"},
	}

	for i := range entries {
		require.NoError(t, store.Create(ctx, &entries[i]))
		// Small sleep to ensure different created_at timestamps for ordering tests
		time.Sleep(10 * time.Millisecond)
	}

	t.Run("all", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{})
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.Len(t, items, 4)
	})

	t.Run("filter_by_category", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Category: models.FeedbackCategoryDataAnomaly,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, items, 2)
		for _, item := range items {
			assert.Equal(t, models.FeedbackCategoryDataAnomaly, item.Category)
		}
	})

	t.Run("filter_by_severity", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Severity: models.FeedbackSeverityHigh,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, items, 2)
	})

	t.Run("filter_by_ticker", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Ticker: "BHP.AX",
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, items, 2)
	})

	t.Run("filter_by_portfolio_name", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			PortfolioName: "smsf",
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, items, 1)
		assert.Equal(t, "smsf", items[0].PortfolioName)
	})

	t.Run("filter_by_session_id", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			SessionID: "sess_a",
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, items, 2)
	})

	t.Run("pagination", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Page:    1,
			PerPage: 2,
		})
		require.NoError(t, err)
		assert.Equal(t, 4, total)
		assert.Len(t, items, 2)

		items2, _, err := store.List(ctx, interfaces.FeedbackListOptions{
			Page:    2,
			PerPage: 2,
		})
		require.NoError(t, err)
		assert.Len(t, items2, 2)

		// No overlap
		assert.NotEqual(t, items[0].ID, items2[0].ID)
	})

	t.Run("sort_created_at_asc", func(t *testing.T) {
		items, _, err := store.List(ctx, interfaces.FeedbackListOptions{
			Sort: "created_at_asc",
		})
		require.NoError(t, err)
		assert.Len(t, items, 4)
		// First should be "anomaly 1" (created first)
		assert.Equal(t, "anomaly 1", items[0].Description)
	})

	t.Run("sort_created_at_desc", func(t *testing.T) {
		items, _, err := store.List(ctx, interfaces.FeedbackListOptions{
			Sort: "created_at_desc",
		})
		require.NoError(t, err)
		assert.Len(t, items, 4)
		// First should be "tool error" (created last)
		assert.Equal(t, "tool error", items[0].Description)
	})
}

func TestFeedbackBulkUpdate(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	fb1 := &models.Feedback{Category: models.FeedbackCategoryObservation, Description: "obs 1"}
	fb2 := &models.Feedback{Category: models.FeedbackCategoryObservation, Description: "obs 2"}
	fb3 := &models.Feedback{Category: models.FeedbackCategoryObservation, Description: "obs 3"}

	require.NoError(t, store.Create(ctx, fb1))
	require.NoError(t, store.Create(ctx, fb2))
	require.NoError(t, store.Create(ctx, fb3))

	// Bulk update first two
	updated, err := store.BulkUpdateStatus(ctx, []string{fb1.ID, fb2.ID}, models.FeedbackStatusDismissed, "Bulk dismissed")
	require.NoError(t, err)
	assert.Equal(t, 2, updated)

	// Verify updates
	got1, err := store.Get(ctx, fb1.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FeedbackStatusDismissed, got1.Status)
	assert.Equal(t, "Bulk dismissed", got1.ResolutionNotes)

	got2, err := store.Get(ctx, fb2.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FeedbackStatusDismissed, got2.Status)

	// Third should remain unchanged
	got3, err := store.Get(ctx, fb3.ID)
	require.NoError(t, err)
	assert.Equal(t, models.FeedbackStatusNew, got3.Status)
}

func TestFeedbackSummary(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	t.Run("empty", func(t *testing.T) {
		summary, err := store.Summary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, summary.Total)
		assert.Nil(t, summary.OldestUnresolved)
	})

	// Create mixed entries
	fb1 := &models.Feedback{Category: models.FeedbackCategoryDataAnomaly, Severity: models.FeedbackSeverityHigh, Description: "a1"}
	fb2 := &models.Feedback{Category: models.FeedbackCategoryDataAnomaly, Severity: models.FeedbackSeverityLow, Description: "a2"}
	fb3 := &models.Feedback{Category: models.FeedbackCategorySyncDelay, Severity: models.FeedbackSeverityMedium, Description: "d1"}

	require.NoError(t, store.Create(ctx, fb1))
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, store.Create(ctx, fb2))
	require.NoError(t, store.Create(ctx, fb3))

	// Resolve one
	require.NoError(t, store.Update(ctx, fb2.ID, models.FeedbackStatusResolved, "fixed", "", "", ""))

	t.Run("with_data", func(t *testing.T) {
		summary, err := store.Summary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, summary.Total)

		assert.Equal(t, 2, summary.ByStatus[models.FeedbackStatusNew])
		assert.Equal(t, 1, summary.ByStatus[models.FeedbackStatusResolved])

		assert.Equal(t, 1, summary.BySeverity[models.FeedbackSeverityHigh])
		assert.Equal(t, 1, summary.BySeverity[models.FeedbackSeverityLow])
		assert.Equal(t, 1, summary.BySeverity[models.FeedbackSeverityMedium])

		assert.Equal(t, 2, summary.ByCategory[models.FeedbackCategoryDataAnomaly])
		assert.Equal(t, 1, summary.ByCategory[models.FeedbackCategorySyncDelay])

		// Oldest unresolved should be fb1's created_at (earliest "new" item)
		require.NotNil(t, summary.OldestUnresolved)
	})
}

func TestFeedbackGetNotFound(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	got, err := store.Get(ctx, "fb_nonexistent")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestFeedbackDeleteIdempotent(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	// Delete non-existent should not error
	err := store.Delete(ctx, "fb_nonexistent")
	assert.NoError(t, err)
}

func TestFeedbackDefaultValues(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "Testing defaults",
	}
	require.NoError(t, store.Create(ctx, fb))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, models.FeedbackStatusNew, got.Status, "default status should be 'new'")
	assert.Equal(t, models.FeedbackSeverityMedium, got.Severity, "default severity should be 'medium'")
	assert.True(t, len(got.ID) > 3, "auto-generated ID should have length > 3")
}

func TestFeedbackListDateFilter(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	before := time.Now()
	time.Sleep(50 * time.Millisecond)

	fb := &models.Feedback{Category: models.FeedbackCategoryObservation, Description: "after mark"}
	require.NoError(t, store.Create(ctx, fb))

	time.Sleep(50 * time.Millisecond)
	after := time.Now()

	t.Run("since_filter", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Since: &before,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, items, 1)
	})

	t.Run("before_filter", func(t *testing.T) {
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Before: &after,
		})
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, items, 1)
	})

	t.Run("no_results_future_since", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		items, total, err := store.List(ctx, interfaces.FeedbackListOptions{
			Since: &future,
		})
		require.NoError(t, err)
		assert.Equal(t, 0, total)
		assert.Empty(t, items)
	})
}
