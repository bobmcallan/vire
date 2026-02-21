package surrealdb

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
)

func TestStockIndexStore_UpsertAndGet(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	entry := &models.StockIndexEntry{
		Ticker:   "BHP.AU",
		Code:     "BHP",
		Exchange: "AU",
		Name:     "BHP Group",
		Source:   "portfolio",
	}

	if err := store.Upsert(ctx, entry); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	got, err := store.Get(ctx, "BHP.AU")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.Ticker != "BHP.AU" {
		t.Errorf("expected ticker BHP.AU, got %s", got.Ticker)
	}
	if got.Code != "BHP" {
		t.Errorf("expected code BHP, got %s", got.Code)
	}
	if got.Exchange != "AU" {
		t.Errorf("expected exchange AU, got %s", got.Exchange)
	}
	if got.Name != "BHP Group" {
		t.Errorf("expected name BHP Group, got %s", got.Name)
	}
	if got.Source != "portfolio" {
		t.Errorf("expected source portfolio, got %s", got.Source)
	}
	if got.AddedAt.IsZero() {
		t.Error("expected AddedAt to be set")
	}
}

func TestStockIndexStore_Upsert_UpdatesExisting(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	entry := &models.StockIndexEntry{
		Ticker:   "BHP.AU",
		Code:     "BHP",
		Exchange: "AU",
		Source:   "portfolio",
	}
	store.Upsert(ctx, entry)

	// Upsert again with different source
	entry2 := &models.StockIndexEntry{
		Ticker:   "BHP.AU",
		Code:     "BHP",
		Exchange: "AU",
		Source:   "watchlist",
	}
	store.Upsert(ctx, entry2)

	got, _ := store.Get(ctx, "BHP.AU")
	if got.Source != "watchlist" {
		t.Errorf("expected source to be updated to watchlist, got %s", got.Source)
	}
}

func TestStockIndexStore_List(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	store.Upsert(ctx, &models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU", Source: "portfolio"})
	store.Upsert(ctx, &models.StockIndexEntry{Ticker: "RIO.AU", Code: "RIO", Exchange: "AU", Source: "portfolio"})

	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestStockIndexStore_UpdateTimestamp(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	store.Upsert(ctx, &models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU", Source: "portfolio"})

	now := time.Now().Truncate(time.Second)
	if err := store.UpdateTimestamp(ctx, "BHP.AU", "eod_collected_at", now); err != nil {
		t.Fatalf("UpdateTimestamp failed: %v", err)
	}

	got, _ := store.Get(ctx, "BHP.AU")
	if got.EODCollectedAt.IsZero() {
		t.Error("expected EODCollectedAt to be set")
	}
}

func TestStockIndexStore_UpdateTimestamp_InvalidField(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	err := store.UpdateTimestamp(ctx, "BHP.AU", "invalid_field", time.Now())
	if err == nil {
		t.Error("expected error for invalid field")
	}
}

func TestStockIndexStore_Delete(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	store.Upsert(ctx, &models.StockIndexEntry{Ticker: "BHP.AU", Code: "BHP", Exchange: "AU", Source: "portfolio"})

	if err := store.Delete(ctx, "BHP.AU"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Get(ctx, "BHP.AU")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestStockIndexStore_Get_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewStockIndexStore(db, testLogger())
	ctx := context.Background()

	_, err := store.Get(ctx, "NONEXISTENT.AU")
	if err == nil {
		t.Error("expected error for non-existent entry")
	}
}
