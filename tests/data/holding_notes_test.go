package data

import (
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/bobmcallan/vire/internal/services/holdingnotes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHoldingNoteAddOrUpdate creates a new holding note via AddOrUpdateNote.
func TestHoldingNoteAddOrUpdate(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	note := &models.HoldingNote{
		Ticker:           "BHP.AU",
		Name:             "BHP Group",
		AssetType:        models.AssetTypeASXStock,
		LiquidityProfile: models.LiquidityHigh,
		Thesis:           "Diversified mining, consistent dividends",
		KnownBehaviours:  "Strong cyclical patterns, sensitive to commodity prices",
		StaleDays:        90,
	}

	result, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify note was added
	assert.Equal(t, "TestPortfolio", result.PortfolioName)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "BHP.AU", result.Items[0].Ticker)
	assert.Equal(t, "BHP Group", result.Items[0].Name)
	assert.Equal(t, models.AssetTypeASXStock, result.Items[0].AssetType)
	assert.False(t, result.Items[0].CreatedAt.IsZero())
	assert.False(t, result.Items[0].ReviewedAt.IsZero())
}

// TestHoldingNoteGet retrieves all holding notes for a portfolio.
func TestHoldingNoteGet(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	// Add a note first
	note := &models.HoldingNote{
		Ticker:    "CBA.AU",
		Name:      "Commonwealth Bank",
		AssetType: models.AssetTypeASXStock,
	}
	_, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note)
	require.NoError(t, err)

	// Get notes
	result, err := svc.GetNotes(ctx, "TestPortfolio")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "TestPortfolio", result.PortfolioName)
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "CBA.AU", result.Items[0].Ticker)
}

// TestHoldingNoteGetNotFound returns empty/nil when portfolio has no notes.
func TestHoldingNoteGetNotFound(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	result, err := svc.GetNotes(ctx, "NonExistentPortfolio")
	// Service may return error or empty collection for non-existent
	if result != nil && err == nil {
		assert.Len(t, result.Items, 0, "should have no items for non-existent portfolio")
	}
}

// TestHoldingNoteUpdateMergeSemantics verifies partial updates preserve existing fields.
func TestHoldingNoteUpdateMergeSemantics(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	// Create initial note
	note := &models.HoldingNote{
		Ticker:           "NAB.AU",
		Name:             "National Australia Bank",
		AssetType:        models.AssetTypeASXStock,
		LiquidityProfile: models.LiquidityHigh,
		Thesis:           "Big 4 bank, defensive",
		KnownBehaviours:  "Market-sensitive",
		Notes:            "Hold for dividend yield",
	}
	_, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note)
	require.NoError(t, err)

	// Update with partial data
	update := &models.HoldingNote{
		Thesis: "Updated thesis",
		// Other fields left empty (zero values) — should be preserved
	}
	result, err := svc.UpdateNote(ctx, "TestPortfolio", "NAB.AU", update)
	require.NoError(t, err)

	// Verify merged result
	found, idx := result.FindByTicker("NAB.AU")
	require.NotNil(t, found)
	assert.Equal(t, "Updated thesis", found.Thesis, "thesis should be updated")
	assert.Equal(t, "National Australia Bank", found.Name, "name should be preserved")
	assert.Equal(t, models.AssetTypeASXStock, found.AssetType, "asset type should be preserved")
	assert.True(t, idx >= 0, "should find the note")
}

// TestHoldingNoteUpsertBehavior verifies AddOrUpdateNote upserts on duplicate ticker.
func TestHoldingNoteUpsertBehavior(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	note1 := &models.HoldingNote{
		Ticker: "WES.AU",
		Name:   "Wesfarmers",
		Thesis: "Original thesis",
	}
	result1, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note1)
	require.NoError(t, err)
	assert.Len(t, result1.Items, 1)

	// Add same ticker with different data (upsert)
	note2 := &models.HoldingNote{
		Ticker: "WES.AU",
		Name:   "Wesfarmers Ltd",
		Thesis: "Updated thesis",
	}
	result2, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note2)
	require.NoError(t, err)

	// Should still be only 1 item (upserted, not duplicated)
	assert.Len(t, result2.Items, 1)
	assert.Equal(t, "Wesfarmers Ltd", result2.Items[0].Name)
	assert.Equal(t, "Updated thesis", result2.Items[0].Thesis)
}

// TestHoldingNoteRemove deletes a note by ticker.
func TestHoldingNoteRemove(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	// Add two notes
	note1 := &models.HoldingNote{Ticker: "BHP.AU"}
	note2 := &models.HoldingNote{Ticker: "CBA.AU"}
	svc.AddOrUpdateNote(ctx, "TestPortfolio", note1)
	svc.AddOrUpdateNote(ctx, "TestPortfolio", note2)

	// Remove one
	result, err := svc.RemoveNote(ctx, "TestPortfolio", "BHP.AU")
	require.NoError(t, err)

	// Verify only CBA remains
	assert.Len(t, result.Items, 1)
	assert.Equal(t, "CBA.AU", result.Items[0].Ticker)
}

// TestHoldingNoteStaleDetection verifies IsStale calculation.
func TestHoldingNoteStaleDetection(t *testing.T) {
	note := &models.HoldingNote{
		Ticker:     "BHP.AU",
		StaleDays:  30,
		ReviewedAt: time.Now().Add(-35 * 24 * time.Hour), // 35 days ago
	}

	assert.True(t, note.IsStale(), "note reviewed 35 days ago with 30-day stale threshold should be stale")

	note.ReviewedAt = time.Now().Add(-20 * 24 * time.Hour) // 20 days ago
	assert.False(t, note.IsStale(), "note reviewed 20 days ago with 30-day stale threshold should not be stale")

	note.ReviewedAt = time.Time{} // zero value
	assert.True(t, note.IsStale(), "note with zero ReviewedAt should be stale")
}

// TestHoldingNoteSignalConfidenceDerival verifies DeriveSignalConfidence rules.
func TestHoldingNoteSignalConfidenceDerival(t *testing.T) {
	tests := []struct {
		name     string
		note     *models.HoldingNote
		expected models.SignalConfidence
	}{
		{
			name: "ETF always low confidence",
			note: &models.HoldingNote{
				AssetType:        models.AssetTypeETF,
				LiquidityProfile: models.LiquidityHigh,
			},
			expected: models.SignalConfidenceLow,
		},
		{
			name: "ASX stock with high liquidity → high confidence",
			note: &models.HoldingNote{
				AssetType:        models.AssetTypeASXStock,
				LiquidityProfile: models.LiquidityHigh,
			},
			expected: models.SignalConfidenceHigh,
		},
		{
			name: "ASX stock with medium liquidity → high confidence",
			note: &models.HoldingNote{
				AssetType:        models.AssetTypeASXStock,
				LiquidityProfile: models.LiquidityMedium,
			},
			expected: models.SignalConfidenceHigh,
		},
		{
			name: "ASX stock with low liquidity → medium confidence",
			note: &models.HoldingNote{
				AssetType:        models.AssetTypeASXStock,
				LiquidityProfile: models.LiquidityLow,
			},
			expected: models.SignalConfidenceMedium,
		},
		{
			name: "US equity always high confidence",
			note: &models.HoldingNote{
				AssetType:        models.AssetTypeUSEquity,
				LiquidityProfile: models.LiquidityLow,
			},
			expected: models.SignalConfidenceHigh,
		},
		{
			name:     "nil note → medium confidence",
			note:     nil,
			expected: models.SignalConfidenceMedium,
		},
		{
			name: "unknown asset type → medium confidence",
			note: &models.HoldingNote{
				AssetType:        models.AssetType("unknown"),
				LiquidityProfile: models.LiquidityHigh,
			},
			expected: models.SignalConfidenceMedium,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := tt.note.DeriveSignalConfidence()
			assert.Equal(t, tt.expected, conf)
		})
	}
}

// TestHoldingNoteNoteMap verifies O(1) lookup via NoteMap with case normalization.
func TestHoldingNoteNoteMap(t *testing.T) {
	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "TestPortfolio",
		Items: []models.HoldingNote{
			{Ticker: "BHP.AU"},
			{Ticker: "cba.au"}, // lowercase
			{Ticker: "NAB.AU"},
		},
	}

	m := notes.NoteMap()

	// NoteMap should normalize all tickers to uppercase
	assert.NotNil(t, m["BHP.AU"])
	assert.NotNil(t, m["CBA.AU"])
	assert.NotNil(t, m["NAB.AU"])
	assert.Len(t, m, 3)
}

// TestHoldingNoteFindByTicker verifies case-insensitive lookup.
func TestHoldingNoteFindByTicker(t *testing.T) {
	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "TestPortfolio",
		Items: []models.HoldingNote{
			{Ticker: "BHP.AU", Name: "BHP Group"},
		},
	}

	// Exact match
	found, idx := notes.FindByTicker("BHP.AU")
	assert.NotNil(t, found)
	assert.Equal(t, 0, idx)
	assert.Equal(t, "BHP Group", found.Name)

	// Case-insensitive match (lowercase)
	found, idx = notes.FindByTicker("bhp.au")
	assert.NotNil(t, found)
	assert.Equal(t, 0, idx)

	// Case-insensitive match (mixed case)
	found, idx = notes.FindByTicker("Bhp.Au")
	assert.NotNil(t, found)

	// Not found
	found, idx = notes.FindByTicker("UNKNOWN")
	assert.Nil(t, found)
	assert.Equal(t, -1, idx)
}

// TestHoldingNoteSaveNotes persists and retrieves via storage.
func TestHoldingNoteSaveNotes(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "TestPortfolio",
		Version:       1,
		Notes:         "Portfolio-level notes",
		Items: []models.HoldingNote{
			{
				Ticker:    "BHP.AU",
				AssetType: models.AssetTypeASXStock,
				Thesis:    "Diversified miner",
			},
		},
	}

	err := svc.SaveNotes(ctx, notes)
	require.NoError(t, err)

	// Retrieve and verify
	retrieved, err := svc.GetNotes(ctx, "TestPortfolio")
	require.NoError(t, err)
	assert.Equal(t, "Portfolio-level notes", retrieved.Notes)
	assert.Len(t, retrieved.Items, 1)
	assert.Equal(t, "BHP.AU", retrieved.Items[0].Ticker)
	assert.Equal(t, "Diversified miner", retrieved.Items[0].Thesis)
}

// TestHoldingNoteEmptyCollection handles empty holdings gracefully.
func TestHoldingNoteEmptyCollection(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	empty := &models.PortfolioHoldingNotes{
		PortfolioName: "EmptyPortfolio",
		Items:         []models.HoldingNote{},
	}

	err := svc.SaveNotes(ctx, empty)
	require.NoError(t, err)

	retrieved, err := svc.GetNotes(ctx, "EmptyPortfolio")
	require.NoError(t, err)

	assert.Equal(t, "EmptyPortfolio", retrieved.PortfolioName)
	assert.Len(t, retrieved.Items, 0)
}

// TestHoldingNoteMultipleTickers handles multiple notes in portfolio.
func TestHoldingNoteMultipleTickers(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	tickers := []string{"BHP.AU", "CBA.AU", "NAB.AU", "WES.AU"}
	for i, ticker := range tickers {
		note := &models.HoldingNote{
			Ticker: ticker,
			Name:   "Company " + ticker,
			Thesis: "Thesis " + ticker,
		}
		_, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note)
		require.NoError(t, err, "add note %d", i)
	}

	result, err := svc.GetNotes(ctx, "TestPortfolio")
	require.NoError(t, err)
	assert.Len(t, result.Items, 4)

	// Verify all tickers present
	m := result.NoteMap()
	for _, ticker := range tickers {
		assert.NotNil(t, m[ticker])
	}
}

// TestHoldingNoteVersionIncrement verifies version increments on save.
func TestHoldingNoteVersionIncrement(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	notes := &models.PortfolioHoldingNotes{
		PortfolioName: "TestPortfolio",
		Version:       0,
		Items:         []models.HoldingNote{},
	}

	svc.SaveNotes(ctx, notes)
	assert.Equal(t, 1, notes.Version, "version should increment")

	svc.SaveNotes(ctx, notes)
	assert.Equal(t, 2, notes.Version, "version should increment again")
}

// TestHoldingNoteTimestamps verifies CreatedAt, ReviewedAt, UpdatedAt are set correctly.
func TestHoldingNoteTimestamps(t *testing.T) {
	mgr := testManager(t)
	logger := common.NewLogger("error")
	svc := holdingnotes.NewService(mgr, logger)
	ctx := testContext()

	before := time.Now()

	note := &models.HoldingNote{
		Ticker: "BHP.AU",
		Thesis: "Test",
	}

	result, err := svc.AddOrUpdateNote(ctx, "TestPortfolio", note)
	require.NoError(t, err)

	after := time.Now()

	item := result.Items[0]
	assert.True(t, item.CreatedAt.After(before), "CreatedAt should be after before time")
	assert.True(t, item.CreatedAt.Before(after), "CreatedAt should be before after time")
	assert.True(t, item.ReviewedAt.After(before), "ReviewedAt should be set on creation")
	assert.True(t, item.UpdatedAt.After(before), "UpdatedAt should be set on creation")
}

// TestHoldingNoteValidAssetType verifies ValidAssetType function.
func TestHoldingNoteValidAssetType(t *testing.T) {
	assert.True(t, models.ValidAssetType(models.AssetTypeETF))
	assert.True(t, models.ValidAssetType(models.AssetTypeASXStock))
	assert.True(t, models.ValidAssetType(models.AssetTypeUSEquity))
	assert.False(t, models.ValidAssetType(models.AssetType("unknown")))
	assert.False(t, models.ValidAssetType(models.AssetType("")))
}
