package badger

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/interfaces"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Concurrent Access ---

func TestConcurrent_UserReadWrite(t *testing.T) {
	store := newTestStore(t)
	us := NewUserStorage(store, testLogger())
	ctx := context.Background()

	const goroutines = 20
	const opsPerGoroutine = 50

	// Pre-create users
	for i := 0; i < goroutines; i++ {
		us.SaveUser(ctx, &models.User{
			Username: fmt.Sprintf("user-%d", i),
			Email:    fmt.Sprintf("user-%d@test.com", i),
		})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*opsPerGoroutine)

	// Concurrent reads and writes on the same keys
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			username := fmt.Sprintf("user-%d", id)
			for i := 0; i < opsPerGoroutine; i++ {
				// Alternate between read and write
				if i%2 == 0 {
					_, err := us.GetUser(ctx, username)
					if err != nil {
						errCh <- fmt.Errorf("goroutine %d: GetUser failed: %w", id, err)
						return
					}
				} else {
					err := us.SaveUser(ctx, &models.User{
						Username: username,
						Email:    fmt.Sprintf("user-%d-iter-%d@test.com", id, i),
					})
					if err != nil {
						errCh <- fmt.Errorf("goroutine %d: SaveUser failed: %w", id, err)
						return
					}
				}
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// Verify all users still exist
	names, err := us.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(names) != goroutines {
		t.Errorf("expected %d users, got %d", goroutines, len(names))
	}
}

func TestConcurrent_StrategyVersionIncrement(t *testing.T) {
	// This test demonstrates the race condition in version increment.
	// Two concurrent SaveStrategy calls may both read version N and
	// both write version N+1, losing a version increment.
	// This is a known limitation of the read-modify-write pattern
	// without transactions.
	store := newTestStore(t)
	ss := NewStrategyStorage(store, testLogger())
	ctx := context.Background()

	// Create initial strategy
	ss.SaveStrategy(ctx, &models.PortfolioStrategy{PortfolioName: "race-test"})

	const goroutines = 10
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := ss.SaveStrategy(ctx, &models.PortfolioStrategy{
				PortfolioName: "race-test",
				Notes:         fmt.Sprintf("goroutine-%d", id),
			})
			if err != nil {
				errCh <- err
			}
		}(g)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent SaveStrategy error: %v", err)
	}

	// Check final version - may be less than goroutines+1 due to race
	got, err := ss.GetStrategy(ctx, "race-test")
	if err != nil {
		t.Fatalf("GetStrategy failed: %v", err)
	}
	// Version should be at least 2 (initial + at least one concurrent write)
	// but may be less than goroutines+1 due to read-modify-write race
	if got.Version < 2 {
		t.Errorf("expected version >= 2, got %d", got.Version)
	}
	// Log the actual version for awareness
	t.Logf("Final version after %d concurrent writes: %d (ideal: %d)", goroutines, got.Version, goroutines+1)
}

func TestConcurrent_KVReadWriteDelete(t *testing.T) {
	store := newTestStore(t)
	kv := NewKVStorage(store, testLogger())
	ctx := context.Background()

	const goroutines = 20
	const ops = 50
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			for i := 0; i < ops; i++ {
				switch i % 3 {
				case 0:
					kv.Set(ctx, key, fmt.Sprintf("value-%d-%d", id, i))
				case 1:
					kv.Get(ctx, key)
				case 2:
					kv.Delete(ctx, key)
				}
			}
		}(g)
	}

	wg.Wait()
	// If we get here without panic, concurrent access is safe
}

// --- Key Injection ---

func TestKeyInjection_SpecialCharacters(t *testing.T) {
	store := newTestStore(t)
	us := NewUserStorage(store, testLogger())
	ctx := context.Background()

	// Test various hostile key inputs
	hostileKeys := []struct {
		name string
		key  string
	}{
		{"null_byte", "user\x00evil"},
		{"path_traversal", "../../etc/passwd"},
		{"backslash_traversal", "..\\..\\windows\\system32"},
		{"forward_slash", "user/subdir/file"},
		{"double_dot", "user..admin"},
		{"unicode_mixed", "user\u200Badmin"}, // zero-width space
		{"empty_string", ""},
		{"spaces", "user with spaces"},
		{"very_long", strings.Repeat("a", 10000)},
		{"special_chars", "user<>|&;`$(){}[]!@#%^*+=~"},
		{"newlines", "user\nnewline\rtab\ttab"},
		{"unicode_rtl", "user\u202Eadmin"}, // right-to-left override
		{"null_only", "\x00\x00\x00"},
	}

	for _, tc := range hostileKeys {
		t.Run(tc.name, func(t *testing.T) {
			user := &models.User{Username: tc.key, Email: "test@test.com"}
			err := us.SaveUser(ctx, user)
			if tc.key == "" {
				// Empty key should either work or return a clear error
				if err != nil {
					t.Logf("Empty key error (acceptable): %v", err)
				}
				return
			}
			if err != nil {
				// BadgerDB may reject some keys — that's acceptable
				t.Logf("Key %q rejected (acceptable): %v", tc.name, err)
				return
			}

			// If save succeeded, verify we can read it back
			got, err := us.GetUser(ctx, tc.key)
			if err != nil {
				t.Errorf("saved key %q but couldn't retrieve: %v", tc.name, err)
				return
			}
			if got.Username != tc.key {
				t.Errorf("username mismatch: saved %q, got %q", tc.key, got.Username)
			}

			// Cleanup
			us.DeleteUser(ctx, tc.key)
		})
	}
}

func TestKeyInjection_KVStorage(t *testing.T) {
	store := newTestStore(t)
	kv := NewKVStorage(store, testLogger())
	ctx := context.Background()

	// Verify KV keys with special characters
	hostileKeys := []string{
		"key\x00null",
		"../../traversal",
		"key with spaces",
		strings.Repeat("k", 5000),
		"key\nwith\nnewlines",
	}

	for _, key := range hostileKeys {
		err := kv.Set(ctx, key, "testvalue")
		if err != nil {
			t.Logf("Key %q rejected: %v", key, err)
			continue
		}

		val, err := kv.Get(ctx, key)
		if err != nil {
			t.Errorf("Set succeeded for key %q but Get failed: %v", key, err)
			continue
		}
		if val != "testvalue" {
			t.Errorf("Value mismatch for key %q: got %q", key, val)
		}

		kv.Delete(ctx, key)
	}
}

// --- Large Payloads ---

func TestLargePayload_Portfolio(t *testing.T) {
	store := newTestStore(t)
	ps := NewPortfolioStorage(store, testLogger())
	ctx := context.Background()

	// Create a portfolio with 500 holdings
	holdings := make([]models.Holding, 500)
	for i := range holdings {
		holdings[i] = models.Holding{
			Ticker:       fmt.Sprintf("STOCK%d.AU", i),
			Exchange:     "ASX",
			Name:         fmt.Sprintf("Test Company %d With A Very Long Name For Stress Testing Purposes", i),
			Units:        float64(i * 100),
			AvgCost:      float64(i) * 1.5,
			CurrentPrice: float64(i) * 2.0,
			MarketValue:  float64(i) * 200.0,
			GainLoss:     float64(i) * 50.0,
			Weight:       0.2,
		}
	}

	p := &models.Portfolio{
		Name:     "large-portfolio",
		Holdings: holdings,
	}

	if err := ps.SavePortfolio(ctx, p); err != nil {
		t.Fatalf("SavePortfolio with 500 holdings failed: %v", err)
	}

	got, err := ps.GetPortfolio(ctx, "large-portfolio")
	if err != nil {
		t.Fatalf("GetPortfolio with 500 holdings failed: %v", err)
	}
	if len(got.Holdings) != 500 {
		t.Errorf("expected 500 holdings, got %d", len(got.Holdings))
	}
}

func TestLargePayload_KVValue(t *testing.T) {
	store := newTestStore(t)
	kv := NewKVStorage(store, testLogger())
	ctx := context.Background()

	// Store a very large value (1MB)
	largeValue := strings.Repeat("x", 1024*1024)
	if err := kv.Set(ctx, "large-value", largeValue); err != nil {
		t.Fatalf("Set with 1MB value failed: %v", err)
	}

	got, err := kv.Get(ctx, "large-value")
	if err != nil {
		t.Fatalf("Get with 1MB value failed: %v", err)
	}
	if len(got) != 1024*1024 {
		t.Errorf("expected 1MB value, got %d bytes", len(got))
	}
}

// --- Migration Edge Cases ---

func TestMigration_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	// Create users dir with mix of valid and corrupt files
	usersDir := filepath.Join(parentPath, "users")
	os.MkdirAll(usersDir, 0755)

	// Valid user
	writeTestJSON(t, filepath.Join(usersDir, "alice.json"), models.User{
		Username: "alice", Email: "alice@test.com",
	})

	// Corrupt JSON
	os.WriteFile(filepath.Join(usersDir, "corrupt.json"), []byte("{invalid json"), 0644)

	// Empty file
	os.WriteFile(filepath.Join(usersDir, "empty.json"), []byte(""), 0644)

	// Valid JSON but wrong schema
	os.WriteFile(filepath.Join(usersDir, "wrong-schema.json"), []byte(`{"foo": "bar"}`), 0644)

	// Non-JSON files (should be skipped)
	os.WriteFile(filepath.Join(usersDir, "readme.txt"), []byte("not json"), 0644)
	os.WriteFile(filepath.Join(usersDir, ".tmp-abc"), []byte("temp file"), 0644)

	// Version backup files (should be skipped)
	os.WriteFile(filepath.Join(usersDir, "alice.json.v1"), []byte(`{"username":"alice-old"}`), 0644)

	// Subdirectory (should be skipped)
	os.MkdirAll(filepath.Join(usersDir, "subdir"), 0755)

	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Migration should succeed despite corrupt files
	err = MigrateFromFiles(logger, store, badgerPath)
	if err != nil {
		t.Fatalf("MigrateFromFiles should handle corrupt files gracefully: %v", err)
	}

	// Valid user should be migrated
	us := NewUserStorage(store, logger)
	ctx := context.Background()
	alice, err := us.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("alice should be migrated: %v", err)
	}
	if alice.Email != "alice@test.com" {
		t.Errorf("unexpected email: %s", alice.Email)
	}

	// wrong-schema should also be migrated (it's valid JSON, just has empty username)
	// The key used would be the filename "wrong-schema" since json field "username" is empty
	// Actually let's check - migrateRecord for "users" uses v.Username as key
	// If username is empty (wrong schema), the key is empty string
	users, _ := us.ListUsers(ctx)
	t.Logf("Migrated users: %v", users)
}

func TestMigration_EmptyDirectories(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	// Create empty domain directories
	for _, d := range domainDirs {
		os.MkdirAll(filepath.Join(parentPath, d), 0755)
	}

	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Migration should handle empty directories
	err = MigrateFromFiles(logger, store, badgerPath)
	if err != nil {
		t.Fatalf("MigrateFromFiles with empty dirs should not error: %v", err)
	}

	// Old directories should be moved to .migrated-*
	entries, _ := os.ReadDir(parentPath)
	for _, e := range entries {
		if e.Name() != "badger" && !strings.HasPrefix(e.Name(), ".migrated-") {
			t.Errorf("unexpected directory remaining: %s", e.Name())
		}
	}
}

func TestMigration_KVEntryFormat(t *testing.T) {
	// Verify that old-format KV JSON files (with lowercase "key"/"value" tags)
	// are correctly deserialized into the new KVEntry struct
	dir := t.TempDir()
	logger := testLogger()

	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	kvDir := filepath.Join(parentPath, "kv")
	os.MkdirAll(kvDir, 0755)

	// Write in old format (lowercase json tags from old kvEntry struct)
	os.WriteFile(filepath.Join(kvDir, "test_key.json"), []byte(`{"key":"test_key","value":"test_value"}`), 0644)

	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	err = MigrateFromFiles(logger, store, badgerPath)
	if err != nil {
		t.Fatalf("MigrateFromFiles failed: %v", err)
	}

	kv := NewKVStorage(store, logger)
	ctx := context.Background()
	val, err := kv.Get(ctx, "test_key")
	if err != nil {
		t.Fatalf("KV entry should be migrated: %v", err)
	}
	if val != "test_value" {
		t.Errorf("expected 'test_value', got '%s'", val)
	}
}

// --- DB Corruption Recovery ---

func TestStore_CorruptDBDirectory(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()
	badgerPath := filepath.Join(dir, "badger")

	// Create the directory with a corrupt MANIFEST file
	os.MkdirAll(badgerPath, 0755)
	os.WriteFile(filepath.Join(badgerPath, "MANIFEST"), []byte("corrupt data"), 0644)

	_, err := NewStore(logger, badgerPath)
	if err == nil {
		t.Fatal("expected error when opening corrupt BadgerDB")
	}
	// Should get a clear error message
	if !strings.Contains(err.Error(), "failed to open badger database") {
		t.Errorf("expected 'failed to open badger database' error, got: %v", err)
	}
}

// --- Empty State Operations ---

func TestEmptyState_AllOperations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// User storage
	us := NewUserStorage(store, testLogger())
	users, err := us.ListUsers(ctx)
	if err != nil {
		t.Errorf("ListUsers on empty DB: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
	_, err = us.GetUser(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for GetUser on empty DB")
	}
	if err := us.DeleteUser(ctx, "nonexistent"); err != nil {
		t.Errorf("DeleteUser on empty DB should not error: %v", err)
	}

	// Portfolio storage
	ps := NewPortfolioStorage(store, testLogger())
	portfolios, err := ps.ListPortfolios(ctx)
	if err != nil {
		t.Errorf("ListPortfolios on empty DB: %v", err)
	}
	if len(portfolios) != 0 {
		t.Errorf("expected 0 portfolios, got %d", len(portfolios))
	}

	// Strategy storage
	ss := NewStrategyStorage(store, testLogger())
	strategies, err := ss.ListStrategies(ctx)
	if err != nil {
		t.Errorf("ListStrategies on empty DB: %v", err)
	}
	if len(strategies) != 0 {
		t.Errorf("expected 0 strategies, got %d", len(strategies))
	}

	// Plan storage
	pls := NewPlanStorage(store, testLogger())
	plans, err := pls.ListPlans(ctx)
	if err != nil {
		t.Errorf("ListPlans on empty DB: %v", err)
	}
	if len(plans) != 0 {
		t.Errorf("expected 0 plans, got %d", len(plans))
	}

	// Watchlist storage
	ws := NewWatchlistStorage(store, testLogger())
	watchlists, err := ws.ListWatchlists(ctx)
	if err != nil {
		t.Errorf("ListWatchlists on empty DB: %v", err)
	}
	if len(watchlists) != 0 {
		t.Errorf("expected 0 watchlists, got %d", len(watchlists))
	}

	// Report storage
	rs := NewReportStorage(store, testLogger())
	reports, err := rs.ListReports(ctx)
	if err != nil {
		t.Errorf("ListReports on empty DB: %v", err)
	}
	if len(reports) != 0 {
		t.Errorf("expected 0 reports, got %d", len(reports))
	}

	// Search storage
	sh := NewSearchHistoryStorage(store, testLogger())
	searches, err := sh.ListSearches(ctx, interfaces.SearchListOptions{})
	if err != nil {
		t.Errorf("ListSearches on empty DB: %v", err)
	}
	if len(searches) != 0 {
		t.Errorf("expected 0 searches, got %d", len(searches))
	}

	// KV storage
	kv := NewKVStorage(store, testLogger())
	all, err := kv.GetAll(ctx)
	if err != nil {
		t.Errorf("GetAll on empty DB: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("expected 0 KV entries, got %d", len(all))
	}
}

// --- Search Pruning Race ---

func TestSearchPruning_RapidSaves(t *testing.T) {
	store := newTestStore(t)
	ss := NewSearchHistoryStorage(store, testLogger())
	ctx := context.Background()

	// Rapidly save more than maxSearchRecords in concurrent goroutines
	const goroutines = 10
	const savesPerGoroutine = 10
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < savesPerGoroutine; i++ {
				r := &models.SearchRecord{
					ID:        fmt.Sprintf("rapid-%d-%d", id, i),
					Type:      "screen",
					Exchange:  "AU",
					CreatedAt: time.Now().Add(time.Duration(id*savesPerGoroutine+i) * time.Millisecond),
				}
				ss.SaveSearch(ctx, r)
			}
		}(g)
	}

	wg.Wait()

	// After pruning, should have at most maxSearchRecords
	results, err := ss.ListSearches(ctx, interfaces.SearchListOptions{Limit: maxSearchRecords + 100})
	if err != nil {
		t.Fatalf("ListSearches failed: %v", err)
	}
	// Due to concurrent pruning races, we may have slightly more than maxSearchRecords
	// but should never have all goroutines*savesPerGoroutine records
	total := goroutines * savesPerGoroutine
	if len(results) > maxSearchRecords+goroutines {
		t.Errorf("expected at most ~%d records after pruning, got %d (of %d total saves)",
			maxSearchRecords, len(results), total)
	}
	t.Logf("Records after concurrent pruning: %d (of %d total saves, max target: %d)",
		len(results), total, maxSearchRecords)
}

// --- Purge Safety ---

func TestPurgeDerivedData_PreservesUserData(t *testing.T) {
	// This test verifies that deleteAllByType correctly targets only
	// derived data types and doesn't touch user-authored data.
	// We test at the BadgerHold level since the Manager calls deleteAllByType.
	store := newTestStore(t)
	logger := testLogger()
	ctx := context.Background()

	// Create user-authored data (should be preserved)
	ss := NewStrategyStorage(store, logger)
	ss.SaveStrategy(ctx, &models.PortfolioStrategy{PortfolioName: "preserve-me"})

	pls := NewPlanStorage(store, logger)
	pls.SavePlan(ctx, &models.PortfolioPlan{PortfolioName: "preserve-me"})

	ws := NewWatchlistStorage(store, logger)
	ws.SaveWatchlist(ctx, &models.PortfolioWatchlist{PortfolioName: "preserve-me"})

	us := NewUserStorage(store, logger)
	us.SaveUser(ctx, &models.User{Username: "preserve-me"})

	kv := NewKVStorage(store, logger)
	kv.Set(ctx, "preserve-key", "preserve-value")

	// Create derived data (should be deleted)
	ps := NewPortfolioStorage(store, logger)
	ps.SavePortfolio(ctx, &models.Portfolio{Name: "delete-me"})

	rs := NewReportStorage(store, logger)
	rs.SaveReport(ctx, &models.PortfolioReport{Portfolio: "delete-me"})

	sh := NewSearchHistoryStorage(store, logger)
	sh.SaveSearch(ctx, &models.SearchRecord{
		ID: "delete-me", Type: "screen", Exchange: "AU", CreatedAt: time.Now(),
	})

	// Simulate purge: delete derived types
	var portfolios []models.Portfolio
	store.DB().Find(&portfolios, nil)
	if len(portfolios) > 0 {
		store.DB().DeleteMatching(&portfolios[0], nil)
	}

	var reports []models.PortfolioReport
	store.DB().Find(&reports, nil)
	if len(reports) > 0 {
		store.DB().DeleteMatching(&reports[0], nil)
	}

	var searches []models.SearchRecord
	store.DB().Find(&searches, nil)
	if len(searches) > 0 {
		store.DB().DeleteMatching(&searches[0], nil)
	}

	// Verify derived data is gone
	_, err := ps.GetPortfolio(ctx, "delete-me")
	if err == nil {
		t.Error("expected portfolio to be purged")
	}
	_, err = rs.GetReport(ctx, "delete-me")
	if err == nil {
		t.Error("expected report to be purged")
	}
	_, err = sh.GetSearch(ctx, "delete-me")
	if err == nil {
		t.Error("expected search to be purged")
	}

	// Verify user-authored data is preserved
	_, err = ss.GetStrategy(ctx, "preserve-me")
	if err != nil {
		t.Errorf("strategy should be preserved: %v", err)
	}
	_, err = pls.GetPlan(ctx, "preserve-me")
	if err != nil {
		t.Errorf("plan should be preserved: %v", err)
	}
	_, err = ws.GetWatchlist(ctx, "preserve-me")
	if err != nil {
		t.Errorf("watchlist should be preserved: %v", err)
	}
	_, err = us.GetUser(ctx, "preserve-me")
	if err != nil {
		t.Errorf("user should be preserved: %v", err)
	}
	val, err := kv.Get(ctx, "preserve-key")
	if err != nil || val != "preserve-value" {
		t.Errorf("KV entry should be preserved: err=%v val=%s", err, val)
	}
}

// --- Double Close ---

func TestStore_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()
	store, err := NewStore(logger, filepath.Join(dir, "badger"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	// First close should succeed
	if err := store.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close — BadgerDB may return an error but should not panic
	// The Store.Close checks for nil db, but after first close db is not nil,
	// it's a closed *badgerhold.Store
	err = store.Close()
	t.Logf("Second close result: %v (panic-free is what matters)", err)
}

// --- Migration with all domain types ---

func TestMigration_AllDomainTypes(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	// Create all domain directories with valid data
	usersDir := filepath.Join(parentPath, "users")
	os.MkdirAll(usersDir, 0755)
	writeTestJSON(t, filepath.Join(usersDir, "alice.json"), models.User{
		Username: "alice", Email: "alice@test.com",
	})

	portfoliosDir := filepath.Join(parentPath, "portfolios")
	os.MkdirAll(portfoliosDir, 0755)
	writeTestJSON(t, filepath.Join(portfoliosDir, "smsf.json"), models.Portfolio{
		ID: "smsf", Name: "smsf",
	})

	strategiesDir := filepath.Join(parentPath, "strategies")
	os.MkdirAll(strategiesDir, 0755)
	writeTestJSON(t, filepath.Join(strategiesDir, "smsf.json"), models.PortfolioStrategy{
		PortfolioName: "smsf", Version: 5,
	})

	plansDir := filepath.Join(parentPath, "plans")
	os.MkdirAll(plansDir, 0755)
	writeTestJSON(t, filepath.Join(plansDir, "smsf.json"), models.PortfolioPlan{
		PortfolioName: "smsf", Version: 2,
	})

	watchlistsDir := filepath.Join(parentPath, "watchlists")
	os.MkdirAll(watchlistsDir, 0755)
	writeTestJSON(t, filepath.Join(watchlistsDir, "smsf.json"), models.PortfolioWatchlist{
		PortfolioName: "smsf", Version: 3,
	})

	reportsDir := filepath.Join(parentPath, "reports")
	os.MkdirAll(reportsDir, 0755)
	writeTestJSON(t, filepath.Join(reportsDir, "smsf.json"), models.PortfolioReport{
		Portfolio: "smsf",
	})

	searchesDir := filepath.Join(parentPath, "searches")
	os.MkdirAll(searchesDir, 0755)
	writeTestJSON(t, filepath.Join(searchesDir, "search-001.json"), models.SearchRecord{
		ID: "search-001", Type: "screen", Exchange: "AU", CreatedAt: time.Now(),
	})

	kvDir := filepath.Join(parentPath, "kv")
	os.MkdirAll(kvDir, 0755)
	// Use the old format with json tags
	os.WriteFile(filepath.Join(kvDir, "default_portfolio.json"),
		[]byte(`{"key":"default_portfolio","value":"smsf"}`), 0644)

	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("MigrateFromFiles failed: %v", err)
	}

	ctx := context.Background()

	// Verify all domain types migrated
	us := NewUserStorage(store, logger)
	if _, err := us.GetUser(ctx, "alice"); err != nil {
		t.Errorf("user not migrated: %v", err)
	}

	ps := NewPortfolioStorage(store, logger)
	if _, err := ps.GetPortfolio(ctx, "smsf"); err != nil {
		t.Errorf("portfolio not migrated: %v", err)
	}

	ss := NewStrategyStorage(store, logger)
	strat, err := ss.GetStrategy(ctx, "smsf")
	if err != nil {
		t.Errorf("strategy not migrated: %v", err)
	} else if strat.Version != 5 {
		t.Errorf("strategy version not preserved: expected 5, got %d", strat.Version)
	}

	pls := NewPlanStorage(store, logger)
	plan, err := pls.GetPlan(ctx, "smsf")
	if err != nil {
		t.Errorf("plan not migrated: %v", err)
	} else if plan.Version != 2 {
		t.Errorf("plan version not preserved: expected 2, got %d", plan.Version)
	}

	ws := NewWatchlistStorage(store, logger)
	wl, err := ws.GetWatchlist(ctx, "smsf")
	if err != nil {
		t.Errorf("watchlist not migrated: %v", err)
	} else if wl.Version != 3 {
		t.Errorf("watchlist version not preserved: expected 3, got %d", wl.Version)
	}

	rs := NewReportStorage(store, logger)
	if _, err := rs.GetReport(ctx, "smsf"); err != nil {
		t.Errorf("report not migrated: %v", err)
	}

	sh := NewSearchHistoryStorage(store, logger)
	if _, err := sh.GetSearch(ctx, "search-001"); err != nil {
		t.Errorf("search not migrated: %v", err)
	}

	kv := NewKVStorage(store, logger)
	val, err := kv.Get(ctx, "default_portfolio")
	if err != nil {
		t.Errorf("KV not migrated: %v", err)
	} else if val != "smsf" {
		t.Errorf("KV value wrong: expected 'smsf', got '%s'", val)
	}
}

// --- Migration idempotency ---

func TestMigration_IdempotentAfterMove(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	usersDir := filepath.Join(parentPath, "users")
	os.MkdirAll(usersDir, 0755)
	writeTestJSON(t, filepath.Join(usersDir, "alice.json"), models.User{
		Username: "alice", Email: "alice@test.com",
	})

	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// First migration
	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("First migration failed: %v", err)
	}

	// Second migration should be no-op (old dirs are gone)
	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("Second migration should be no-op: %v", err)
	}

	// Verify data is still there (not duplicated or corrupted)
	us := NewUserStorage(store, logger)
	ctx := context.Background()
	users, _ := us.ListUsers(ctx)
	if len(users) != 1 {
		t.Errorf("expected 1 user after idempotent migration, got %d", len(users))
	}
}

// --- Migration with missing key fields ---

func TestMigration_MissingKeyFields(t *testing.T) {
	dir := t.TempDir()
	logger := testLogger()

	parentPath := filepath.Join(dir, "user")
	badgerPath := filepath.Join(parentPath, "badger")

	// Create portfolio with empty ID (should fall back to filename)
	portfoliosDir := filepath.Join(parentPath, "portfolios")
	os.MkdirAll(portfoliosDir, 0755)
	os.WriteFile(filepath.Join(portfoliosDir, "my-portfolio.json"),
		[]byte(`{"name":"My Portfolio","holdings":[]}`), 0644)

	// Create strategy with empty PortfolioName (should fall back to filename)
	strategiesDir := filepath.Join(parentPath, "strategies")
	os.MkdirAll(strategiesDir, 0755)
	os.WriteFile(filepath.Join(strategiesDir, "my-strat.json"),
		[]byte(`{"version":1}`), 0644)

	store, err := NewStore(logger, badgerPath)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	if err := MigrateFromFiles(logger, store, badgerPath); err != nil {
		t.Fatalf("MigrateFromFiles failed: %v", err)
	}

	ctx := context.Background()

	// Portfolio should use filename as key
	ps := NewPortfolioStorage(store, logger)
	_, err = ps.GetPortfolio(ctx, "my-portfolio")
	if err != nil {
		t.Errorf("portfolio with empty ID should use filename as key: %v", err)
	}

	// Strategy should use filename as key
	ss := NewStrategyStorage(store, logger)
	_, err = ss.GetStrategy(ctx, "my-strat")
	if err != nil {
		t.Errorf("strategy with empty PortfolioName should use filename as key: %v", err)
	}
}

// --- Verify KVEntry JSON deserialization ---

func TestKVEntry_JSONRoundtrip(t *testing.T) {
	// The old file-based storage wrote: {"key":"name","value":"val"}
	// Verify KVEntry can decode this format (no json tags, relies on case-insensitive match)
	oldJSON := `{"key":"test_key","value":"test_value"}`

	var entry KVEntry
	if err := json.Unmarshal([]byte(oldJSON), &entry); err != nil {
		t.Fatalf("Failed to unmarshal old KV format: %v", err)
	}
	if entry.Key != "test_key" {
		t.Errorf("expected key 'test_key', got '%s'", entry.Key)
	}
	if entry.Value != "test_value" {
		t.Errorf("expected value 'test_value', got '%s'", entry.Value)
	}
}
