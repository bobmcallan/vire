package surrealdb

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func makeSnapshot(userID, name string, date time.Time, equityValue float64) models.TimelineSnapshot {
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

func TestTimelineStore_SaveBatchAndGetRange(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeSnapshot("user1", "smsf", d1, 100000),
		makeSnapshot("user1", "smsf", d2, 101000),
		makeSnapshot("user1", "smsf", d3, 102000),
	}

	if err := store.SaveBatch(ctx, snapshots); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	// Get all three
	got, err := store.GetRange(ctx, "user1", "smsf", d1, d3)
	if err != nil {
		t.Fatalf("GetRange failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(got))
	}
	if got[0].EquityHoldingsValue != 100000 {
		t.Errorf("expected first equity_value 100000, got %f", got[0].EquityHoldingsValue)
	}
	if got[2].EquityHoldingsValue != 102000 {
		t.Errorf("expected third equity_value 102000, got %f", got[2].EquityHoldingsValue)
	}

	// Get subset
	got, err = store.GetRange(ctx, "user1", "smsf", d2, d2)
	if err != nil {
		t.Fatalf("GetRange subset failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(got))
	}
	if got[0].EquityHoldingsValue != 101000 {
		t.Errorf("expected equity_value 101000, got %f", got[0].EquityHoldingsValue)
	}
}

func TestTimelineStore_GetLatest(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeSnapshot("user1", "smsf", d1, 100000),
		makeSnapshot("user1", "smsf", d2, 105000),
	}

	if err := store.SaveBatch(ctx, snapshots); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	latest, err := store.GetLatest(ctx, "user1", "smsf")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if latest == nil {
		t.Fatal("expected non-nil latest")
	}
	if latest.EquityHoldingsValue != 105000 {
		t.Errorf("expected latest equity_value 105000, got %f", latest.EquityHoldingsValue)
	}
}

func TestTimelineStore_GetLatest_Empty(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	latest, err := store.GetLatest(ctx, "user1", "nonexistent")
	if err != nil {
		t.Fatalf("GetLatest failed: %v", err)
	}
	if latest != nil {
		t.Errorf("expected nil for empty portfolio, got %+v", latest)
	}
}

func TestTimelineStore_SaveBatch_Upsert(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Save initial
	snap1 := makeSnapshot("user1", "smsf", d1, 100000)
	if err := store.SaveBatch(ctx, []models.TimelineSnapshot{snap1}); err != nil {
		t.Fatalf("SaveBatch failed: %v", err)
	}

	// Upsert with updated value
	snap2 := makeSnapshot("user1", "smsf", d1, 110000)
	if err := store.SaveBatch(ctx, []models.TimelineSnapshot{snap2}); err != nil {
		t.Fatalf("SaveBatch upsert failed: %v", err)
	}

	got, err := store.GetRange(ctx, "user1", "smsf", d1, d1)
	if err != nil {
		t.Fatalf("GetRange failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 snapshot after upsert, got %d", len(got))
	}
	if got[0].EquityHoldingsValue != 110000 {
		t.Errorf("expected upserted equity_value 110000, got %f", got[0].EquityHoldingsValue)
	}
}

func TestTimelineStore_DeleteAll(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeSnapshot("user1", "smsf", d1, 100000),
		makeSnapshot("user1", "smsf", d2, 101000),
	}
	store.SaveBatch(ctx, snapshots)

	_, err := store.DeleteAll(ctx, "user1", "smsf")
	if err != nil {
		t.Fatalf("DeleteAll failed: %v", err)
	}

	got, err := store.GetRange(ctx, "user1", "smsf", d1, d2)
	if err != nil {
		t.Fatalf("GetRange after delete failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 snapshots after DeleteAll, got %d", len(got))
	}
}

func TestTimelineStore_DeleteRange(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	d2 := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	d3 := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeSnapshot("user1", "smsf", d1, 100000),
		makeSnapshot("user1", "smsf", d2, 101000),
		makeSnapshot("user1", "smsf", d3, 102000),
	}
	store.SaveBatch(ctx, snapshots)

	// Delete only d2
	_, err := store.DeleteRange(ctx, "user1", "smsf", d2, d2)
	if err != nil {
		t.Fatalf("DeleteRange failed: %v", err)
	}

	got, err := store.GetRange(ctx, "user1", "smsf", d1, d3)
	if err != nil {
		t.Fatalf("GetRange after DeleteRange failed: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 snapshots after DeleteRange, got %d", len(got))
	}
}

func TestTimelineStore_UserIsolation(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	snapshots := []models.TimelineSnapshot{
		makeSnapshot("user1", "smsf", d1, 100000),
		makeSnapshot("user2", "smsf", d1, 200000),
	}
	store.SaveBatch(ctx, snapshots)

	// user1 should only see their data
	got, _ := store.GetRange(ctx, "user1", "smsf", d1, d1)
	if len(got) != 1 || got[0].EquityHoldingsValue != 100000 {
		t.Errorf("user isolation failed: expected user1 data only, got %+v", got)
	}

	// user2 should only see their data
	got, _ = store.GetRange(ctx, "user2", "smsf", d1, d1)
	if len(got) != 1 || got[0].EquityHoldingsValue != 200000 {
		t.Errorf("user isolation failed: expected user2 data only, got %+v", got)
	}
}

func TestTimelineStore_SaveBatch_Empty(t *testing.T) {
	db := testDB(t)
	store := NewTimelineStore(db, testLogger())
	ctx := context.Background()

	err := store.SaveBatch(ctx, nil)
	if err != nil {
		t.Fatalf("SaveBatch with nil should not error, got: %v", err)
	}

	err = store.SaveBatch(ctx, []models.TimelineSnapshot{})
	if err != nil {
		t.Fatalf("SaveBatch with empty slice should not error, got: %v", err)
	}
}
