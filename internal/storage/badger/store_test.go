package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Test helpers ---

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLogger("error")
	store, err := NewStore(logger, filepath.Join(dir, "badger"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func testLogger() *common.Logger {
	return common.NewLogger("error")
}

// --- Store tests ---

func TestStore_OpenClose(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()
	store, err := NewStore(logger, filepath.Join(dir, "badger"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	if store.DB() == nil {
		t.Fatal("expected non-nil DB")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestStore_CloseNilDB(t *testing.T) {
	store := &Store{}
	if err := store.Close(); err != nil {
		t.Fatalf("Close on nil DB should not error: %v", err)
	}
}

// --- User Storage tests ---

func TestUserStorage_CRUD(t *testing.T) {
	store := newTestStore(t)
	us := NewUserStorage(store, testLogger())
	ctx := context.Background()

	// Get non-existent
	_, err := us.GetUser(ctx, "alice")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}

	// Save
	user := &models.User{Username: "alice", Email: "alice@example.com", Role: "admin"}
	if err := us.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	// Get existing
	got, err := us.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got.Username != "alice" || got.Email != "alice@example.com" {
		t.Errorf("unexpected user: %+v", got)
	}

	// Update
	user.Email = "alice@new.com"
	if err := us.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser (update) failed: %v", err)
	}
	got, _ = us.GetUser(ctx, "alice")
	if got.Email != "alice@new.com" {
		t.Errorf("expected updated email, got %s", got.Email)
	}

	// List
	us.SaveUser(ctx, &models.User{Username: "bob"})
	names, err := us.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 users, got %d", len(names))
	}

	// Delete
	if err := us.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
	_, err = us.GetUser(ctx, "alice")
	if err == nil {
		t.Fatal("expected error after delete")
	}

	// Delete non-existent (should not error)
	if err := us.DeleteUser(ctx, "nonexistent"); err != nil {
		t.Fatalf("DeleteUser non-existent should not error: %v", err)
	}
}

// --- Portfolio Storage tests ---

func TestPortfolioStorage_CRUD(t *testing.T) {
	store := newTestStore(t)
	ps := NewPortfolioStorage(store, testLogger())
	ctx := context.Background()

	// Save with empty ID (should default to Name)
	p := &models.Portfolio{Name: "smsf-growth"}
	if err := ps.SavePortfolio(ctx, p); err != nil {
		t.Fatalf("SavePortfolio failed: %v", err)
	}
	if p.ID != "smsf-growth" {
		t.Errorf("expected ID to default to Name, got %s", p.ID)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Error("expected CreatedAt and UpdatedAt to be set")
	}

	// Get
	got, err := ps.GetPortfolio(ctx, "smsf-growth")
	if err != nil {
		t.Fatalf("GetPortfolio failed: %v", err)
	}
	if got.Name != "smsf-growth" {
		t.Errorf("unexpected portfolio name: %s", got.Name)
	}

	// List
	ps.SavePortfolio(ctx, &models.Portfolio{Name: "trading"})
	names, err := ps.ListPortfolios(ctx)
	if err != nil {
		t.Fatalf("ListPortfolios failed: %v", err)
	}
	if len(names) != 2 {
		t.Errorf("expected 2 portfolios, got %d", len(names))
	}

	// Delete
	if err := ps.DeletePortfolio(ctx, "smsf-growth"); err != nil {
		t.Fatalf("DeletePortfolio failed: %v", err)
	}
	_, err = ps.GetPortfolio(ctx, "smsf-growth")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// --- Strategy Storage tests ---

func TestStrategyStorage_VersionIncrement(t *testing.T) {
	store := newTestStore(t)
	ss := NewStrategyStorage(store, testLogger())
	ctx := context.Background()

	// First save — version 1, default disclaimer
	s := &models.PortfolioStrategy{PortfolioName: "smsf"}
	if err := ss.SaveStrategy(ctx, s); err != nil {
		t.Fatalf("SaveStrategy failed: %v", err)
	}
	if s.Version != 1 {
		t.Errorf("expected version 1, got %d", s.Version)
	}
	if s.Disclaimer != models.DefaultDisclaimer {
		t.Errorf("expected default disclaimer, got %s", s.Disclaimer)
	}
	if s.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	createdAt := s.CreatedAt

	// Second save — version 2, CreatedAt preserved
	s2 := &models.PortfolioStrategy{PortfolioName: "smsf", Disclaimer: "custom"}
	if err := ss.SaveStrategy(ctx, s2); err != nil {
		t.Fatalf("SaveStrategy (update) failed: %v", err)
	}
	if s2.Version != 2 {
		t.Errorf("expected version 2, got %d", s2.Version)
	}
	if s2.Disclaimer != "custom" {
		t.Errorf("expected custom disclaimer on update, got %s", s2.Disclaimer)
	}
	if !s2.CreatedAt.Equal(createdAt) {
		t.Error("expected CreatedAt to be preserved on update")
	}

	// Get
	got, err := ss.GetStrategy(ctx, "smsf")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}

	// List
	names, _ := ss.ListStrategies(ctx)
	if len(names) != 1 {
		t.Errorf("expected 1 strategy, got %d", len(names))
	}

	// Delete
	ss.DeleteStrategy(ctx, "smsf")
	_, err = ss.GetStrategy(ctx, "smsf")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// --- Plan Storage tests ---

func TestPlanStorage_VersionIncrement(t *testing.T) {
	store := newTestStore(t)
	ps := NewPlanStorage(store, testLogger())
	ctx := context.Background()

	plan := &models.PortfolioPlan{PortfolioName: "smsf"}
	if err := ps.SavePlan(ctx, plan); err != nil {
		t.Fatalf("SavePlan failed: %v", err)
	}
	if plan.Version != 1 {
		t.Errorf("expected version 1, got %d", plan.Version)
	}
	createdAt := plan.CreatedAt

	// Update
	plan2 := &models.PortfolioPlan{PortfolioName: "smsf", Notes: "updated"}
	if err := ps.SavePlan(ctx, plan2); err != nil {
		t.Fatalf("SavePlan (update) failed: %v", err)
	}
	if plan2.Version != 2 {
		t.Errorf("expected version 2, got %d", plan2.Version)
	}
	if !plan2.CreatedAt.Equal(createdAt) {
		t.Error("expected CreatedAt to be preserved")
	}

	// List
	names, _ := ps.ListPlans(ctx)
	if len(names) != 1 {
		t.Errorf("expected 1 plan, got %d", len(names))
	}

	// Delete
	ps.DeletePlan(ctx, "smsf")
	_, err := ps.GetPlan(ctx, "smsf")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// --- Watchlist Storage tests ---

func TestWatchlistStorage_VersionIncrement(t *testing.T) {
	store := newTestStore(t)
	ws := NewWatchlistStorage(store, testLogger())
	ctx := context.Background()

	wl := &models.PortfolioWatchlist{PortfolioName: "smsf"}
	if err := ws.SaveWatchlist(ctx, wl); err != nil {
		t.Fatalf("SaveWatchlist failed: %v", err)
	}
	if wl.Version != 1 {
		t.Errorf("expected version 1, got %d", wl.Version)
	}
	createdAt := wl.CreatedAt

	// Update
	wl2 := &models.PortfolioWatchlist{PortfolioName: "smsf", Notes: "updated"}
	if err := ws.SaveWatchlist(ctx, wl2); err != nil {
		t.Fatalf("SaveWatchlist (update) failed: %v", err)
	}
	if wl2.Version != 2 {
		t.Errorf("expected version 2, got %d", wl2.Version)
	}
	if !wl2.CreatedAt.Equal(createdAt) {
		t.Error("expected CreatedAt to be preserved")
	}

	// List
	names, _ := ws.ListWatchlists(ctx)
	if len(names) != 1 {
		t.Errorf("expected 1 watchlist, got %d", len(names))
	}

	// Delete
	ws.DeleteWatchlist(ctx, "smsf")
	_, err := ws.GetWatchlist(ctx, "smsf")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// --- Report Storage tests ---

func TestReportStorage_CRUD(t *testing.T) {
	store := newTestStore(t)
	rs := NewReportStorage(store, testLogger())
	ctx := context.Background()

	report := &models.PortfolioReport{
		Portfolio:   "smsf",
		GeneratedAt: time.Now(),
	}
	if err := rs.SaveReport(ctx, report); err != nil {
		t.Fatalf("SaveReport failed: %v", err)
	}

	got, err := rs.GetReport(ctx, "smsf")
	if err != nil {
		t.Fatalf("GetReport failed: %v", err)
	}
	if got.Portfolio != "smsf" {
		t.Errorf("unexpected portfolio: %s", got.Portfolio)
	}

	names, _ := rs.ListReports(ctx)
	if len(names) != 1 {
		t.Errorf("expected 1 report, got %d", len(names))
	}

	rs.DeleteReport(ctx, "smsf")
	_, err = rs.GetReport(ctx, "smsf")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

// --- Search History Storage tests ---

func TestSearchStorage_CRUD(t *testing.T) {
	store := newTestStore(t)
	ss := NewSearchHistoryStorage(store, testLogger())
	ctx := context.Background()

	record := &models.SearchRecord{
		Type:        "screen",
		Exchange:    "AU",
		ResultCount: 5,
		CreatedAt:   time.Now(),
	}
	if err := ss.SaveSearch(ctx, record); err != nil {
		t.Fatalf("SaveSearch failed: %v", err)
	}
	if record.ID == "" {
		t.Error("expected ID to be auto-generated")
	}

	got, err := ss.GetSearch(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetSearch failed: %v", err)
	}
	if got.Type != "screen" {
		t.Errorf("unexpected type: %s", got.Type)
	}

	ss.DeleteSearch(ctx, record.ID)
	_, err = ss.GetSearch(ctx, record.ID)
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestSearchStorage_ListFiltering(t *testing.T) {
	store := newTestStore(t)
	ss := NewSearchHistoryStorage(store, testLogger())
	ctx := context.Background()

	now := time.Now()

	// Create records with different types and exchanges
	records := []*models.SearchRecord{
		{ID: "s1", Type: "screen", Exchange: "AU", CreatedAt: now.Add(-3 * time.Minute)},
		{ID: "s2", Type: "screen", Exchange: "US", CreatedAt: now.Add(-2 * time.Minute)},
		{ID: "s3", Type: "snipe", Exchange: "AU", CreatedAt: now.Add(-1 * time.Minute)},
		{ID: "s4", Type: "funnel", Exchange: "AU", CreatedAt: now},
	}
	for _, r := range records {
		ss.SaveSearch(ctx, r)
	}

	// Filter by type
	results, _ := ss.ListSearches(ctx, interfaces.SearchListOptions{Type: "screen"})
	if len(results) != 2 {
		t.Errorf("expected 2 screen results, got %d", len(results))
	}

	// Filter by exchange
	results, _ = ss.ListSearches(ctx, interfaces.SearchListOptions{Exchange: "AU"})
	if len(results) != 3 {
		t.Errorf("expected 3 AU results, got %d", len(results))
	}

	// Filter by both
	results, _ = ss.ListSearches(ctx, interfaces.SearchListOptions{Type: "screen", Exchange: "AU"})
	if len(results) != 1 {
		t.Errorf("expected 1 screen+AU result, got %d", len(results))
	}

	// No filter with limit
	results, _ = ss.ListSearches(ctx, interfaces.SearchListOptions{Limit: 2})
	if len(results) != 2 {
		t.Errorf("expected 2 results with limit, got %d", len(results))
	}
	// Should be newest first
	if results[0].ID != "s4" {
		t.Errorf("expected newest first, got %s", results[0].ID)
	}

	// Default limit (20) — all 4 should be returned
	results, _ = ss.ListSearches(ctx, interfaces.SearchListOptions{})
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}
}

func TestSearchStorage_Pruning(t *testing.T) {
	store := newTestStore(t)
	ss := NewSearchHistoryStorage(store, testLogger())
	ctx := context.Background()

	// Insert more than maxSearchRecords
	for i := 0; i < maxSearchRecords+10; i++ {
		r := &models.SearchRecord{
			ID:        fmt.Sprintf("prune-%03d", i),
			Type:      "screen",
			Exchange:  "AU",
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		}
		ss.SaveSearch(ctx, r)
	}

	// Should be pruned to maxSearchRecords
	results, _ := ss.ListSearches(ctx, interfaces.SearchListOptions{Limit: maxSearchRecords + 20})
	if len(results) != maxSearchRecords {
		t.Errorf("expected %d records after pruning, got %d", maxSearchRecords, len(results))
	}
}

// --- KV Storage tests ---

func TestKVStorage_CRUD(t *testing.T) {
	store := newTestStore(t)
	kv := NewKVStorage(store, testLogger())
	ctx := context.Background()

	// Get non-existent
	_, err := kv.Get(ctx, "missing")
	if err == nil {
		t.Fatal("expected error for non-existent key")
	}

	// Set
	if err := kv.Set(ctx, "api_key", "secret123"); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get
	val, err := kv.Get(ctx, "api_key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "secret123" {
		t.Errorf("expected secret123, got %s", val)
	}

	// Update
	kv.Set(ctx, "api_key", "updated")
	val, _ = kv.Get(ctx, "api_key")
	if val != "updated" {
		t.Errorf("expected updated, got %s", val)
	}

	// GetAll
	kv.Set(ctx, "another", "value")
	all, err := kv.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 entries, got %d", len(all))
	}

	// Delete
	kv.Delete(ctx, "api_key")
	_, err = kv.Get(ctx, "api_key")
	if err == nil {
		t.Fatal("expected error after delete")
	}

	// Delete non-existent (should not error)
	if err := kv.Delete(ctx, "nonexistent"); err != nil {
		t.Fatalf("Delete non-existent should not error: %v", err)
	}
}

// --- Migration tests ---

func TestMigrateFromFiles(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	// Set up old file layout
	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	// Create old directories with JSON files
	usersDir := filepath.Join(parentPath, "users")
	os.MkdirAll(usersDir, 0755)
	writeTestJSON(t, filepath.Join(usersDir, "alice.json"), models.User{
		Username: "alice", Email: "alice@test.com", Role: "admin",
	})
	writeTestJSON(t, filepath.Join(usersDir, "bob.json"), models.User{
		Username: "bob", Email: "bob@test.com",
	})

	portfoliosDir := filepath.Join(parentPath, "portfolios")
	os.MkdirAll(portfoliosDir, 0755)
	writeTestJSON(t, filepath.Join(portfoliosDir, "smsf.json"), models.Portfolio{
		ID: "smsf", Name: "smsf",
	})

	kvDir := filepath.Join(parentPath, "kv")
	os.MkdirAll(kvDir, 0755)
	writeTestJSON(t, filepath.Join(kvDir, "default_portfolio.json"), KVEntry{
		Key: "default_portfolio", Value: "smsf",
	})

	strategiesDir := filepath.Join(parentPath, "strategies")
	os.MkdirAll(strategiesDir, 0755)
	writeTestJSON(t, filepath.Join(strategiesDir, "smsf.json"), models.PortfolioStrategy{
		PortfolioName: "smsf", Version: 3,
	})

	// Create store and run migration
	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("MigrateFromFiles failed: %v", err)
	}

	// Verify migrated data
	us := NewUserStorage(store, logger)
	ctx := context.Background()

	alice, err := us.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("expected alice to be migrated: %v", err)
	}
	if alice.Email != "alice@test.com" {
		t.Errorf("unexpected email: %s", alice.Email)
	}

	users, _ := us.ListUsers(ctx)
	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	ps := NewPortfolioStorage(store, logger)
	portfolio, err := ps.GetPortfolio(ctx, "smsf")
	if err != nil {
		t.Fatalf("expected smsf portfolio to be migrated: %v", err)
	}
	if portfolio.Name != "smsf" {
		t.Errorf("unexpected portfolio name: %s", portfolio.Name)
	}

	kv := NewKVStorage(store, logger)
	val, err := kv.Get(ctx, "default_portfolio")
	if err != nil {
		t.Fatalf("expected KV entry to be migrated: %v", err)
	}
	if val != "smsf" {
		t.Errorf("unexpected KV value: %s", val)
	}

	ss := NewStrategyStorage(store, logger)
	strat, err := ss.GetStrategy(ctx, "smsf")
	if err != nil {
		t.Fatalf("expected strategy to be migrated: %v", err)
	}
	if strat.Version != 3 {
		t.Errorf("expected strategy version 3, got %d", strat.Version)
	}

	// Verify old directories were moved
	if _, err := os.Stat(usersDir); !os.IsNotExist(err) {
		t.Error("expected old users directory to be moved")
	}

	// Verify running migration again is a no-op
	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("second MigrateFromFiles should be no-op: %v", err)
	}
}

func TestMigrateFromFiles_NoOldData(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	badgerPath := filepath.Join(dir, "user", "badger")
	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Should return nil without error
	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("MigrateFromFiles with no old data should not error: %v", err)
	}
}

// --- Helpers ---

func writeTestJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test JSON: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test JSON %s: %v", path, err)
	}
}
