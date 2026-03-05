package data

import (
	"encoding/base64"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFeedbackCreate_WithSingleAttachment(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	pngData := base64.StdEncoding.EncodeToString([]byte("fake-png-bytes"))

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryDataAnomaly,
		Severity:    models.FeedbackSeverityHigh,
		Description: "Price chart screenshot",
		Ticker:      "BHP.AX",
		Attachments: []models.FeedbackAttachment{
			{
				Filename:    "chart.png",
				ContentType: "image/png",
				SizeBytes:   len("fake-png-bytes"),
				Data:        pngData,
			},
		},
	}
	require.NoError(t, store.Create(ctx, fb))
	assert.NotEmpty(t, fb.ID)

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Attachments, 1)
	assert.Equal(t, "chart.png", got.Attachments[0].Filename)
	assert.Equal(t, "image/png", got.Attachments[0].ContentType)
	assert.Equal(t, len("fake-png-bytes"), got.Attachments[0].SizeBytes)
	assert.Equal(t, pngData, got.Attachments[0].Data)
}

func TestFeedbackCreate_WithMultipleAttachments(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	attachments := []models.FeedbackAttachment{
		{
			Filename:    "screenshot.png",
			ContentType: "image/png",
			SizeBytes:   10,
			Data:        base64.StdEncoding.EncodeToString([]byte("png-data01")),
		},
		{
			Filename:    "config.json",
			ContentType: "application/json",
			SizeBytes:   14,
			Data:        base64.StdEncoding.EncodeToString([]byte(`{"key":"value"}`)),
		},
		{
			Filename:    "export.csv",
			ContentType: "text/csv",
			SizeBytes:   7,
			Data:        base64.StdEncoding.EncodeToString([]byte("a,b,c\n")),
		},
	}

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryCalculationError,
		Description: "Multiple supporting files",
		Attachments: attachments,
	}
	require.NoError(t, store.Create(ctx, fb))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Attachments, 3)

	// Verify each attachment round-trips correctly
	for i, want := range attachments {
		assert.Equal(t, want.Filename, got.Attachments[i].Filename, "attachment %d filename", i)
		assert.Equal(t, want.ContentType, got.Attachments[i].ContentType, "attachment %d content_type", i)
		assert.Equal(t, want.Data, got.Attachments[i].Data, "attachment %d data", i)
	}
}

func TestFeedbackCreate_WithNoAttachments(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	fb := &models.Feedback{
		Category:    models.FeedbackCategoryObservation,
		Description: "No attachments here",
	}
	require.NoError(t, store.Create(ctx, fb))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.Attachments, "attachments should be empty/nil when none provided")
	assert.Equal(t, "No attachments here", got.Description)
}

func TestFeedbackUpdate_AddAttachments(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	// Create feedback without attachments
	fb := &models.Feedback{
		Category:    models.FeedbackCategoryMissingData,
		Description: "Will add attachment later",
	}
	require.NoError(t, store.Create(ctx, fb))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Attachments)

	// Update to add attachments
	atts := []models.FeedbackAttachment{
		{
			Filename:    "evidence.png",
			ContentType: "image/png",
			SizeBytes:   8,
			Data:        base64.StdEncoding.EncodeToString([]byte("evidence")),
		},
	}
	require.NoError(t, store.Update(ctx, fb.ID,
		models.FeedbackStatusAcknowledged, "Adding evidence", "", "", "",
		&atts))

	got, err = store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, models.FeedbackStatusAcknowledged, got.Status)
	require.Len(t, got.Attachments, 1)
	assert.Equal(t, "evidence.png", got.Attachments[0].Filename)
}

func TestFeedbackUpdate_ReplaceAttachments(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	// Create feedback with initial attachment
	fb := &models.Feedback{
		Category:    models.FeedbackCategorySyncDelay,
		Description: "Initial attachment",
		Attachments: []models.FeedbackAttachment{
			{
				Filename:    "old.png",
				ContentType: "image/png",
				SizeBytes:   3,
				Data:        base64.StdEncoding.EncodeToString([]byte("old")),
			},
		},
	}
	require.NoError(t, store.Create(ctx, fb))

	// Replace with new attachments
	newAtts := []models.FeedbackAttachment{
		{
			Filename:    "new1.jpeg",
			ContentType: "image/jpeg",
			SizeBytes:   4,
			Data:        base64.StdEncoding.EncodeToString([]byte("new1")),
		},
		{
			Filename:    "new2.webp",
			ContentType: "image/webp",
			SizeBytes:   4,
			Data:        base64.StdEncoding.EncodeToString([]byte("new2")),
		},
	}
	require.NoError(t, store.Update(ctx, fb.ID,
		models.FeedbackStatusAcknowledged, "", "", "", "",
		&newAtts))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.Len(t, got.Attachments, 2)
	assert.Equal(t, "new1.jpeg", got.Attachments[0].Filename)
	assert.Equal(t, "new2.webp", got.Attachments[1].Filename)
}

func TestFeedbackUpdate_ClearAttachments(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	// Create feedback with attachment
	fb := &models.Feedback{
		Category:    models.FeedbackCategoryToolError,
		Description: "Has attachment to clear",
		Attachments: []models.FeedbackAttachment{
			{
				Filename:    "temp.png",
				ContentType: "image/png",
				SizeBytes:   4,
				Data:        base64.StdEncoding.EncodeToString([]byte("temp")),
			},
		},
	}
	require.NoError(t, store.Create(ctx, fb))

	// Clear attachments by passing empty slice
	empty := []models.FeedbackAttachment{}
	require.NoError(t, store.Update(ctx, fb.ID,
		models.FeedbackStatusAcknowledged, "", "", "", "",
		&empty))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	assert.Empty(t, got.Attachments, "attachments should be empty after clearing")
}

func TestFeedbackUpdate_NilAttachments_PreservesExisting(t *testing.T) {
	mgr := testManager(t)
	store := mgr.FeedbackStore()
	ctx := testContext()

	pngData := base64.StdEncoding.EncodeToString([]byte("preserved"))

	// Create feedback with attachment
	fb := &models.Feedback{
		Category:    models.FeedbackCategorySchemaChange,
		Description: "Attachment should survive update",
		Attachments: []models.FeedbackAttachment{
			{
				Filename:    "keep.png",
				ContentType: "image/png",
				SizeBytes:   9,
				Data:        pngData,
			},
		},
	}
	require.NoError(t, store.Create(ctx, fb))

	// Update status only, pass nil for attachments
	require.NoError(t, store.Update(ctx, fb.ID,
		models.FeedbackStatusResolved, "Fixed it", "", "", "",
		nil))

	got, err := store.Get(ctx, fb.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, models.FeedbackStatusResolved, got.Status)
	assert.Equal(t, "Fixed it", got.ResolutionNotes)
	require.Len(t, got.Attachments, 1, "attachment should be preserved when nil passed")
	assert.Equal(t, "keep.png", got.Attachments[0].Filename)
	assert.Equal(t, pngData, got.Attachments[0].Data)
}
