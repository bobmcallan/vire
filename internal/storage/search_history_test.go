package storage

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bobmccarthy/vire/internal/common"
	"github.com/bobmccarthy/vire/internal/interfaces"
	"github.com/bobmccarthy/vire/internal/models"
)

func newTestSearchHistoryDB(t *testing.T) *searchHistoryStorage {
	t.Helper()
	dir, err := os.MkdirTemp("", "vire-test-search-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	logger := common.NewLogger("error")
	db, err := NewBadgerDB(logger, &common.BadgerConfig{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	return newSearchHistoryStorage(db, logger)
}

func TestSearchHistory_SaveAndGet(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	record := &models.SearchRecord{
		ID:          "search-test-1",
		Type:        "screen",
		Exchange:    "AU",
		Filters:     `{"exchange":"AU","max_pe":20}`,
		ResultCount: 5,
		Results:     `[{"ticker":"BHP.AU"}]`,
		CreatedAt:   time.Now(),
	}

	if err := s.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	got, err := s.GetSearch(ctx, "search-test-1")
	if err != nil {
		t.Fatalf("GetSearch failed: %v", err)
	}

	if got.Type != "screen" {
		t.Errorf("Expected type 'screen', got '%s'", got.Type)
	}
	if got.Exchange != "AU" {
		t.Errorf("Expected exchange 'AU', got '%s'", got.Exchange)
	}
	if got.ResultCount != 5 {
		t.Errorf("Expected result count 5, got %d", got.ResultCount)
	}
}

func TestSearchHistory_AutoGeneratesID(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	record := &models.SearchRecord{
		Type:        "snipe",
		Exchange:    "US",
		Filters:     `{}`,
		ResultCount: 3,
		Results:     `[]`,
		CreatedAt:   time.Now(),
	}

	if err := s.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	if record.ID == "" {
		t.Fatal("Expected auto-generated ID")
	}
	if len(record.ID) < 10 {
		t.Errorf("Expected non-trivial ID, got '%s'", record.ID)
	}

	// Should be retrievable by the auto-generated ID
	got, err := s.GetSearch(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetSearch failed: %v", err)
	}
	if got.Exchange != "US" {
		t.Errorf("Expected exchange 'US', got '%s'", got.Exchange)
	}
}

func TestSearchHistory_GetNotFound(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	_, err := s.GetSearch(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("Expected error for non-existent search")
	}
}

func TestSearchHistory_Delete(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	record := &models.SearchRecord{
		ID:          "search-delete-test",
		Type:        "funnel",
		Exchange:    "AU",
		Filters:     `{}`,
		ResultCount: 0,
		Results:     `[]`,
		CreatedAt:   time.Now(),
	}

	if err := s.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}

	if err := s.DeleteSearch(ctx, "search-delete-test"); err != nil {
		t.Fatalf("DeleteSearch failed: %v", err)
	}

	_, err := s.GetSearch(ctx, "search-delete-test")
	if err == nil {
		t.Fatal("Expected error after deletion")
	}
}

func TestSearchHistory_ListAll(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "search-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-3 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "search-2", Type: "snipe", Exchange: "US", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "search-3", Type: "funnel", Exchange: "AU", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := s.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := s.ListSearches(ctx, interfaces.SearchListOptions{})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Should be sorted by CreatedAt descending (most recent first)
	if results[0].ID != "search-3" {
		t.Errorf("Expected most recent first (search-3), got %s", results[0].ID)
	}
	if results[2].ID != "search-1" {
		t.Errorf("Expected oldest last (search-1), got %s", results[2].ID)
	}
}

func TestSearchHistory_ListFilterByType(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "s-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-3 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "s-2", Type: "snipe", Exchange: "AU", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "s-3", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := s.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := s.ListSearches(ctx, interfaces.SearchListOptions{Type: "screen"})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("Expected 2 screen results, got %d", len(results))
	}
}

func TestSearchHistory_ListFilterByExchange(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	now := time.Now()
	records := []*models.SearchRecord{
		{ID: "e-1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-2 * time.Hour), Results: `[]`, Filters: `{}`},
		{ID: "e-2", Type: "screen", Exchange: "US", CreatedAt: now.Add(-1 * time.Hour), Results: `[]`, Filters: `{}`},
	}

	for _, r := range records {
		if err := s.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := s.ListSearches(ctx, interfaces.SearchListOptions{Exchange: "AU"})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 AU result, got %d", len(results))
	}
	if results[0].ID != "e-1" {
		t.Errorf("Expected e-1, got %s", results[0].ID)
	}
}

func TestSearchHistory_ListWithLimit(t *testing.T) {
	s := newTestSearchHistoryDB(t)
	ctx := context.Background()

	now := time.Now()
	for i := 0; i < 10; i++ {
		r := &models.SearchRecord{
			ID:        fmt.Sprintf("lim-%d", i),
			Type:      "screen",
			Exchange:  "AU",
			CreatedAt: now.Add(time.Duration(-10+i) * time.Hour),
			Results:   `[]`,
			Filters:   `{}`,
		}
		if err := s.SaveSearch(ctx, r); err != nil {
			t.Fatalf("SaveSearch failed: %v", err)
		}
	}

	results, err := s.ListSearches(ctx, interfaces.SearchListOptions{Limit: 3})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results with limit, got %d", len(results))
	}
}
