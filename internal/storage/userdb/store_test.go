package userdb

import (
	"context"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

func newUnitTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLogger("debug")
	store, err := NewStore(logger, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestUserRecordCRUD(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Put
	rec := &models.UserRecord{
		UserID:  "alice",
		Subject: "portfolio",
		Key:     "smsf-growth",
		Value:   `{"name":"smsf-growth","holdings":[]}`,
	}
	if err := store.Put(ctx, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Get
	got, err := store.Get(ctx, "alice", "portfolio", "smsf-growth")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value != `{"name":"smsf-growth","holdings":[]}` {
		t.Errorf("unexpected value: %s", got.Value)
	}
	if got.Version != 1 {
		t.Errorf("expected version 1, got %d", got.Version)
	}

	// Update (version increment)
	rec.Value = `{"name":"smsf-growth","holdings":["BHP"]}`
	if err := store.Put(ctx, rec); err != nil {
		t.Fatalf("Put update: %v", err)
	}
	got, _ = store.Get(ctx, "alice", "portfolio", "smsf-growth")
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}

	// Delete
	if err := store.Delete(ctx, "alice", "portfolio", "smsf-growth"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err = store.Get(ctx, "alice", "portfolio", "smsf-growth")
	if err == nil {
		t.Error("Get after delete should fail")
	}
}

func TestListBySubject(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "portfolio", Key: "p1", Value: "data1"})
	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "portfolio", Key: "p2", Value: "data2"})
	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "strategy", Key: "s1", Value: "data3"})
	store.Put(ctx, &models.UserRecord{UserID: "bob", Subject: "portfolio", Key: "p3", Value: "data4"})

	records, err := store.List(ctx, "alice", "portfolio")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}

	// List different subject
	records, _ = store.List(ctx, "alice", "strategy")
	if len(records) != 1 {
		t.Errorf("expected 1 strategy, got %d", len(records))
	}

	// List different user
	records, _ = store.List(ctx, "bob", "portfolio")
	if len(records) != 1 {
		t.Errorf("expected 1 bob portfolio, got %d", len(records))
	}
}

func TestQuery(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Add search records
	for i := 0; i < 5; i++ {
		store.Put(ctx, &models.UserRecord{
			UserID:  "alice",
			Subject: "search",
			Key:     "s" + string(rune('0'+i)),
			Value:   `{"type":"screen"}`,
		})
	}

	// Query with limit
	results, err := store.Query(ctx, "alice", "search", interfaces.QueryOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Verify descending order (default)
	for i := 1; i < len(results); i++ {
		if results[i].DateTime.After(results[i-1].DateTime) {
			t.Error("expected descending order")
		}
	}
}

func TestQueryAscending(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "search", Key: "s1", Value: "a"})
	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "search", Key: "s2", Value: "b"})

	results, _ := store.Query(ctx, "alice", "search", interfaces.QueryOptions{OrderBy: "datetime_asc"})
	if len(results) != 2 {
		t.Fatalf("expected 2, got %d", len(results))
	}
	if results[0].DateTime.After(results[1].DateTime) {
		t.Error("expected ascending order")
	}
}

func TestDeleteBySubject(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "portfolio", Key: "p1", Value: "d"})
	store.Put(ctx, &models.UserRecord{UserID: "bob", Subject: "portfolio", Key: "p2", Value: "d"})
	store.Put(ctx, &models.UserRecord{UserID: "alice", Subject: "strategy", Key: "s1", Value: "d"})

	count, err := store.DeleteBySubject(ctx, "portfolio")
	if err != nil {
		t.Fatalf("DeleteBySubject: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deleted, got %d", count)
	}

	// Strategy should survive
	records, _ := store.List(ctx, "alice", "strategy")
	if len(records) != 1 {
		t.Errorf("strategy should survive, got %d", len(records))
	}
}

func TestAutoSearchPruning(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Add 55 search records
	for i := 0; i < 55; i++ {
		store.Put(ctx, &models.UserRecord{
			UserID:  "alice",
			Subject: "search",
			Key:     "s" + string(rune(i/26+'a')) + string(rune(i%26+'a')),
			Value:   `{}`,
		})
	}

	// Should be pruned to 50
	records, _ := store.List(ctx, "alice", "search")
	if len(records) > 50 {
		t.Errorf("expected max 50 after pruning, got %d", len(records))
	}
}

func TestGetNotFound(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	_, err := store.Get(ctx, "nobody", "portfolio", "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent record")
	}
}

func TestDeleteNonexistent(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Should not error
	err := store.Delete(ctx, "nobody", "portfolio", "nonexistent")
	if err != nil {
		t.Errorf("Delete nonexistent should not error: %v", err)
	}
}

func TestCloseNilDB(t *testing.T) {
	store := &Store{}
	if err := store.Close(); err != nil {
		t.Errorf("Close on nil db should not error: %v", err)
	}
}

func TestDeleteBySubjects(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	store.Put(ctx, &models.UserRecord{UserID: "u", Subject: "portfolio", Key: "k1", Value: "d"})
	store.Put(ctx, &models.UserRecord{UserID: "u", Subject: "report", Key: "k2", Value: "d"})
	store.Put(ctx, &models.UserRecord{UserID: "u", Subject: "strategy", Key: "k3", Value: "d"})

	total, err := store.DeleteBySubjects(ctx, "portfolio", "report")
	if err != nil {
		t.Fatalf("DeleteBySubjects: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2, got %d", total)
	}

	// Strategy should survive
	records, _ := store.List(ctx, "u", "strategy")
	if len(records) != 1 {
		t.Error("strategy should survive")
	}
}
