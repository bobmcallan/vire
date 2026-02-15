package userdb

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
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
	store, err := NewStore(logger, filepath.Join(dir, "userdb"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// --- Composite Key Injection ---

// CRITICAL: compositeKey = "user_id:subject:key"
// A userID containing ":" can create composite keys that collide with
// other users' data. For example:
//
//	userID="alice:portfolio" subject="" key="SMSF" => "alice:portfolio::SMSF"
//	userID="alice" subject="portfolio" key="SMSF" => "alice:portfolio:SMSF"
//
// These are different due to the empty subject, but:
//
//	userID="alice:portfolio:SMSF" subject="" key="" => "alice:portfolio:SMSF::"
//	userID="alice" subject="portfolio" key="SMSF" => "alice:portfolio:SMSF"
//
// Different. But:
//
//	userID="a" subject="b" key="c" => "a:b:c"
//	userID="a:b" subject="c" key="" => "a:b:c:"
//
// Different. However:
//
//	userID="a" subject="b:c" key="d" => "a:b:c:d"
//	userID="a" subject="b" key="c:d" => "a:b:c:d"
//
// THESE ARE THE SAME! subject="b:c" key="d" aliases subject="b" key="c:d".
func TestKeyInjection_CompositeKeyCollision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Setup: alice stores a portfolio named "SMSF"
	legitimateRecord := &models.UserRecord{
		UserID:  "alice",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   `{"name":"SMSF","holdings":[{"ticker":"BHP.AU"}]}`,
	}
	if err := store.Put(ctx, legitimateRecord); err != nil {
		t.Fatalf("Put legitimate record failed: %v", err)
	}

	// Attack vector 1: userID with colons trying to access alice's data
	// Composite key "alice:portfolio:SMSF" could be reached by:
	// userID="alice:portfolio" subject="SMSF" key=""
	attackRecord1 := &models.UserRecord{
		UserID:  "alice:portfolio",
		Subject: "SMSF",
		Key:     "",
		Value:   `{"hacked":true}`,
	}
	err := store.Put(ctx, attackRecord1)
	if err != nil {
		t.Logf("Attack vector 1 (empty key) rejected: %v", err)
	} else {
		// Check if alice's record was overwritten
		aliceRec, err := store.Get(ctx, "alice", "portfolio", "SMSF")
		if err != nil {
			t.Errorf("Alice's record became inaccessible after attack: %v", err)
		} else if strings.Contains(aliceRec.Value, "hacked") {
			t.Errorf("VULNERABILITY: Attack overwrote alice's portfolio! Value=%s", aliceRec.Value)
		}
	}

	// Attack vector 2: subject with colons
	// userID="alice" subject="portfolio:SMSF" key="" produces "alice:portfolio:SMSF:"
	// vs legitimate "alice:portfolio:SMSF" — trailing colon differs.
	// But what about: subject="portfolio" key="SMSF" vs subject="" key="portfolio:SMSF"?
	// "alice::portfolio:SMSF" vs "alice:portfolio:SMSF" — leading double colon differs.

	// Most dangerous: key contains colons
	// userID="alice" subject="portfolio" key="SMSF:extra" => "alice:portfolio:SMSF:extra"
	// This is unambiguous but creates a 4-segment key. It's different from any 3-segment key.
	// HOWEVER: if we ever parse the key by splitting on ":", we'd get wrong segments.
	err = store.Put(ctx, &models.UserRecord{
		UserID:  "alice",
		Subject: "portfolio",
		Key:     "SMSF:hacked",
		Value:   `{"name":"SMSF:hacked"}`,
	})
	if err == nil {
		// Verify the colon key doesn't interfere with the regular "SMSF" key
		rec, err := store.Get(ctx, "alice", "portfolio", "SMSF")
		if err != nil {
			t.Errorf("legitimate SMSF record inaccessible after colon-key insert: %v", err)
		} else {
			t.Logf("SMSF record intact after colon-key insert: %s", rec.Value)
		}
	}
}

// TestKeyInjection_SubjectCollision verifies that colons in subject names
// cannot create cross-subject access.
func TestKeyInjection_SubjectCollision(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Store a strategy for alice
	store.Put(ctx, &models.UserRecord{
		UserID:  "alice",
		Subject: "strategy",
		Key:     "SMSF",
		Value:   `{"strategy":"growth"}`,
	})

	// Store a portfolio for alice
	store.Put(ctx, &models.UserRecord{
		UserID:  "alice",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   `{"holdings":[]}`,
	})

	// Verify cross-subject isolation
	strategy, _ := store.Get(ctx, "alice", "strategy", "SMSF")
	portfolio, _ := store.Get(ctx, "alice", "portfolio", "SMSF")

	if strategy.Value == portfolio.Value {
		t.Error("strategy and portfolio records should have different values")
	}
	if strategy.Subject != "strategy" {
		t.Errorf("strategy record has wrong subject: %s", strategy.Subject)
	}
	if portfolio.Subject != "portfolio" {
		t.Errorf("portfolio record has wrong subject: %s", portfolio.Subject)
	}

	// List should only return the requested subject
	strategies, _ := store.List(ctx, "alice", "strategy")
	for _, s := range strategies {
		if s.Subject != "strategy" {
			t.Errorf("List(strategy) returned record with subject=%s", s.Subject)
		}
	}
	portfolios, _ := store.List(ctx, "alice", "portfolio")
	for _, p := range portfolios {
		if p.Subject != "portfolio" {
			t.Errorf("List(portfolio) returned record with subject=%s", p.Subject)
		}
	}
}

// --- Cross-User Isolation ---

func TestCrossUserIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Alice and Bob each have a portfolio named "SMSF"
	store.Put(ctx, &models.UserRecord{
		UserID:  "alice",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   `{"name":"SMSF","owner":"alice"}`,
	})
	store.Put(ctx, &models.UserRecord{
		UserID:  "bob",
		Subject: "portfolio",
		Key:     "SMSF",
		Value:   `{"name":"SMSF","owner":"bob"}`,
	})

	// Verify isolation
	aliceRec, _ := store.Get(ctx, "alice", "portfolio", "SMSF")
	bobRec, _ := store.Get(ctx, "bob", "portfolio", "SMSF")

	if strings.Contains(aliceRec.Value, "bob") {
		t.Error("alice's record contains bob's data")
	}
	if strings.Contains(bobRec.Value, "alice") {
		t.Error("bob's record contains alice's data")
	}

	// Delete alice's portfolio should not affect bob's
	store.Delete(ctx, "alice", "portfolio", "SMSF")
	bobRec, err := store.Get(ctx, "bob", "portfolio", "SMSF")
	if err != nil {
		t.Fatalf("bob's record deleted when alice's was removed: %v", err)
	}
	if !strings.Contains(bobRec.Value, "bob") {
		t.Error("bob's record was corrupted after alice's deletion")
	}

	// List should only show bob's portfolios now
	aliceList, _ := store.List(ctx, "alice", "portfolio")
	if len(aliceList) != 0 {
		t.Errorf("alice should have 0 portfolios after delete, got %d", len(aliceList))
	}
	bobList, _ := store.List(ctx, "bob", "portfolio")
	if len(bobList) != 1 {
		t.Errorf("bob should still have 1 portfolio, got %d", len(bobList))
	}
}

// --- Purge Safety ---

func TestPurgeDerivedData_PreservesUserAuthored(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// User-authored data (should survive purge)
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "strategy", Key: "SMSF",
		Value: `{"strategy":"growth"}`,
	})
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "plan", Key: "SMSF",
		Value: `{"plan":"rebalance"}`,
	})
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "watchlist", Key: "SMSF",
		Value: `{"tickers":["BHP.AU"]}`,
	})

	// Derived data (should be purged)
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "portfolio", Key: "SMSF",
		Value: `{"holdings":[]}`,
	})
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "report", Key: "SMSF",
		Value: `{"report":"data"}`,
	})
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "search", Key: "search-001",
		Value: `{"type":"screen"}`,
	})

	// Purge derived subjects (portfolio, report, search)
	portfolioCount, err := store.DeleteBySubject(ctx, "portfolio")
	if err != nil {
		t.Fatalf("DeleteBySubject(portfolio) failed: %v", err)
	}
	if portfolioCount != 1 {
		t.Errorf("expected 1 portfolio deleted, got %d", portfolioCount)
	}

	reportCount, _ := store.DeleteBySubject(ctx, "report")
	if reportCount != 1 {
		t.Errorf("expected 1 report deleted, got %d", reportCount)
	}

	searchCount, _ := store.DeleteBySubject(ctx, "search")
	if searchCount != 1 {
		t.Errorf("expected 1 search deleted, got %d", searchCount)
	}

	// Verify derived data is gone
	_, err = store.Get(ctx, "alice", "portfolio", "SMSF")
	if err == nil {
		t.Error("portfolio should be purged")
	}
	_, err = store.Get(ctx, "alice", "report", "SMSF")
	if err == nil {
		t.Error("report should be purged")
	}
	_, err = store.Get(ctx, "alice", "search", "search-001")
	if err == nil {
		t.Error("search should be purged")
	}

	// Verify user-authored data survives
	strategy, err := store.Get(ctx, "alice", "strategy", "SMSF")
	if err != nil {
		t.Errorf("strategy should survive purge: %v", err)
	} else if !strings.Contains(strategy.Value, "growth") {
		t.Errorf("strategy data corrupted after purge: %s", strategy.Value)
	}

	plan, err := store.Get(ctx, "alice", "plan", "SMSF")
	if err != nil {
		t.Errorf("plan should survive purge: %v", err)
	} else if !strings.Contains(plan.Value, "rebalance") {
		t.Errorf("plan data corrupted after purge: %s", plan.Value)
	}

	watchlist, err := store.Get(ctx, "alice", "watchlist", "SMSF")
	if err != nil {
		t.Errorf("watchlist should survive purge: %v", err)
	} else if !strings.Contains(watchlist.Value, "BHP.AU") {
		t.Errorf("watchlist data corrupted after purge: %s", watchlist.Value)
	}
}

// TestPurge_DoesNotDeleteStrategies specifically verifies strategies survive
// all purge paths.
func TestPurge_DoesNotDeleteStrategies(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create strategies for multiple users
	for _, user := range []string{"alice", "bob", "charlie"} {
		for _, portfolio := range []string{"SMSF", "Trading", "Growth"} {
			store.Put(ctx, &models.UserRecord{
				UserID:  user,
				Subject: "strategy",
				Key:     portfolio,
				Value:   fmt.Sprintf(`{"user":"%s","portfolio":"%s"}`, user, portfolio),
			})
		}
	}

	// Purge all "derived" subjects
	for _, subject := range []string{"portfolio", "report", "search"} {
		store.DeleteBySubject(ctx, subject)
	}

	// Verify ALL strategies survive
	for _, user := range []string{"alice", "bob", "charlie"} {
		strategies, err := store.List(ctx, user, "strategy")
		if err != nil {
			t.Errorf("List strategies for %s failed: %v", user, err)
			continue
		}
		if len(strategies) != 3 {
			t.Errorf("%s should have 3 strategies after purge, got %d", user, len(strategies))
		}
	}
}

// --- Concurrent Access ---

func TestConcurrent_UserRecord_ReadWrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const goroutines = 20
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*opsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", id)
			for i := 0; i < opsPerGoroutine; i++ {
				key := fmt.Sprintf("portfolio-%d", i%5)
				if i%2 == 0 {
					err := store.Put(ctx, &models.UserRecord{
						UserID:  userID,
						Subject: "portfolio",
						Key:     key,
						Value:   fmt.Sprintf(`{"iter":%d}`, i),
					})
					if err != nil {
						errCh <- fmt.Errorf("goroutine %d: Put failed: %w", id, err)
						return
					}
				} else {
					_, err := store.Get(ctx, userID, "portfolio", key)
					if err != nil && !strings.Contains(err.Error(), "not found") {
						errCh <- fmt.Errorf("goroutine %d: Get failed: %w", id, err)
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
}

func TestConcurrent_MixedSubjects(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	subjects := []string{"portfolio", "strategy", "plan", "watchlist", "report", "search"}
	const goroutines = 12
	const ops = 30
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", id%3)
			subject := subjects[id%len(subjects)]
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("key-%d", i)
				switch i % 4 {
				case 0:
					store.Put(ctx, &models.UserRecord{
						UserID: userID, Subject: subject, Key: key,
						Value: fmt.Sprintf(`{"g":%d,"i":%d}`, id, i),
					})
				case 1:
					store.Get(ctx, userID, subject, key)
				case 2:
					store.List(ctx, userID, subject)
				case 3:
					store.Delete(ctx, userID, subject, key)
				}
			}
		}(g)
	}

	wg.Wait()
	// Reaching here without panic means concurrent access is safe
}

// --- Large Payloads ---

func TestLargePayload_Portfolio(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create a portfolio with 500 holdings serialized as JSON
	type holding struct {
		Ticker       string  `json:"ticker"`
		Exchange     string  `json:"exchange"`
		Name         string  `json:"name"`
		Units        float64 `json:"units"`
		AvgCost      float64 `json:"avg_cost"`
		CurrentPrice float64 `json:"current_price"`
		MarketValue  float64 `json:"market_value"`
		GainLoss     float64 `json:"gain_loss"`
	}

	holdings := make([]holding, 500)
	for i := range holdings {
		holdings[i] = holding{
			Ticker:       fmt.Sprintf("STOCK%d.AU", i),
			Exchange:     "ASX",
			Name:         fmt.Sprintf("Test Company %d With A Very Long Name For Stress Testing", i),
			Units:        float64(i * 100),
			AvgCost:      float64(i) * 1.5,
			CurrentPrice: float64(i) * 2.0,
			MarketValue:  float64(i) * 200.0,
			GainLoss:     float64(i) * 50.0,
		}
	}

	data, err := json.Marshal(map[string]interface{}{
		"name":     "large-portfolio",
		"holdings": holdings,
	})
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	rec := &models.UserRecord{
		UserID:  "alice",
		Subject: "portfolio",
		Key:     "large-portfolio",
		Value:   string(data),
	}

	if err := store.Put(ctx, rec); err != nil {
		t.Fatalf("Put large portfolio failed: %v", err)
	}

	got, err := store.Get(ctx, "alice", "portfolio", "large-portfolio")
	if err != nil {
		t.Fatalf("Get large portfolio failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got.Value), &parsed); err != nil {
		t.Fatalf("failed to parse retrieved portfolio: %v", err)
	}

	holdingsSlice, ok := parsed["holdings"].([]interface{})
	if !ok {
		t.Fatal("holdings field missing or wrong type")
	}
	if len(holdingsSlice) != 500 {
		t.Errorf("expected 500 holdings, got %d", len(holdingsSlice))
	}
}

func TestLargePayload_1MB_Value(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	largeValue := strings.Repeat("x", 1024*1024)
	rec := &models.UserRecord{
		UserID:  "alice",
		Subject: "report",
		Key:     "big-report",
		Value:   largeValue,
	}

	if err := store.Put(ctx, rec); err != nil {
		t.Fatalf("Put 1MB value failed: %v", err)
	}

	got, err := store.Get(ctx, "alice", "report", "big-report")
	if err != nil {
		t.Fatalf("Get 1MB value failed: %v", err)
	}
	if len(got.Value) != 1024*1024 {
		t.Errorf("expected 1MB value, got %d bytes", len(got.Value))
	}
}

// --- Search Pruning ---

func TestSearchPruning_ExceedsMax(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert more than maxSearchRecords
	for i := 0; i < maxSearchRecords+20; i++ {
		store.Put(ctx, &models.UserRecord{
			UserID:   "alice",
			Subject:  "search",
			Key:      fmt.Sprintf("search-%03d", i),
			Value:    fmt.Sprintf(`{"type":"screen","index":%d}`, i),
			DateTime: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	// Should be pruned to maxSearchRecords
	results, err := store.List(ctx, "alice", "search")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(results) > maxSearchRecords {
		t.Errorf("expected at most %d records after pruning, got %d", maxSearchRecords, len(results))
	}
}

func TestSearchPruning_ConcurrentSaves(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const goroutines = 10
	const savesPerGoroutine = 10
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < savesPerGoroutine; i++ {
				store.Put(ctx, &models.UserRecord{
					UserID:   "alice",
					Subject:  "search",
					Key:      fmt.Sprintf("rapid-%d-%d", id, i),
					Value:    `{"type":"screen"}`,
					DateTime: time.Now().Add(time.Duration(id*savesPerGoroutine+i) * time.Millisecond),
				})
			}
		}(g)
	}

	wg.Wait()

	results, _ := store.List(ctx, "alice", "search")
	total := goroutines * savesPerGoroutine
	// Due to concurrent pruning, may have slightly more than max but not all
	if len(results) > maxSearchRecords+goroutines {
		t.Errorf("expected at most ~%d records, got %d (of %d saves)",
			maxSearchRecords, len(results), total)
	}
	t.Logf("Records after concurrent pruning: %d (of %d saves, target: %d)",
		len(results), total, maxSearchRecords)
}

// --- Search pruning per-user isolation ---

func TestSearchPruning_PerUserIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Alice has maxSearchRecords+10 searches
	for i := 0; i < maxSearchRecords+10; i++ {
		store.Put(ctx, &models.UserRecord{
			UserID:   "alice",
			Subject:  "search",
			Key:      fmt.Sprintf("alice-search-%03d", i),
			Value:    `{"type":"screen"}`,
			DateTime: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	// Bob has 5 searches
	for i := 0; i < 5; i++ {
		store.Put(ctx, &models.UserRecord{
			UserID:   "bob",
			Subject:  "search",
			Key:      fmt.Sprintf("bob-search-%03d", i),
			Value:    `{"type":"snipe"}`,
			DateTime: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	// Alice should be pruned to maxSearchRecords
	aliceSearches, _ := store.List(ctx, "alice", "search")
	if len(aliceSearches) > maxSearchRecords {
		t.Errorf("alice should have at most %d searches, got %d", maxSearchRecords, len(aliceSearches))
	}

	// Bob should still have all 5
	bobSearches, _ := store.List(ctx, "bob", "search")
	if len(bobSearches) != 5 {
		t.Errorf("bob should have 5 searches (alice's pruning shouldn't affect bob), got %d", len(bobSearches))
	}
}

// --- Version Increment ---

func TestVersionIncrement(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// First Put — version 1
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "strategy", Key: "SMSF",
		Value: `{"v":1}`,
	})
	rec, _ := store.Get(ctx, "alice", "strategy", "SMSF")
	if rec.Version != 1 {
		t.Errorf("expected version 1, got %d", rec.Version)
	}

	// Second Put — version 2
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "strategy", Key: "SMSF",
		Value: `{"v":2}`,
	})
	rec, _ = store.Get(ctx, "alice", "strategy", "SMSF")
	if rec.Version != 2 {
		t.Errorf("expected version 2, got %d", rec.Version)
	}

	// Third Put — version 3
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "strategy", Key: "SMSF",
		Value: `{"v":3}`,
	})
	rec, _ = store.Get(ctx, "alice", "strategy", "SMSF")
	if rec.Version != 3 {
		t.Errorf("expected version 3, got %d", rec.Version)
	}
}

// --- Query ordering ---

func TestQuery_OrderingAndLimit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		store.Put(ctx, &models.UserRecord{
			UserID:   "alice",
			Subject:  "search",
			Key:      fmt.Sprintf("search-%02d", i),
			Value:    fmt.Sprintf(`{"index":%d}`, i),
			DateTime: baseTime.Add(time.Duration(i) * time.Hour),
		})
	}

	// Default order: datetime_desc
	results, _ := store.Query(ctx, "alice", "search", interfaces.QueryOptions{})
	if len(results) != 10 {
		t.Fatalf("expected 10 results, got %d", len(results))
	}
	// Most recent should be first
	for i := 1; i < len(results); i++ {
		if results[i].DateTime.After(results[i-1].DateTime) {
			t.Errorf("results not in descending order at index %d", i)
		}
	}

	// Ascending order
	results, _ = store.Query(ctx, "alice", "search", interfaces.QueryOptions{OrderBy: "datetime_asc"})
	for i := 1; i < len(results); i++ {
		if results[i].DateTime.Before(results[i-1].DateTime) {
			t.Errorf("results not in ascending order at index %d", i)
		}
	}

	// With limit
	results, _ = store.Query(ctx, "alice", "search", interfaces.QueryOptions{Limit: 3})
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit, got %d", len(results))
	}
}

// --- Empty State ---

func TestEmptyState_AllOperations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Get non-existent
	_, err := store.Get(ctx, "alice", "portfolio", "SMSF")
	if err == nil {
		t.Error("expected error for Get on empty DB")
	}

	// List on empty DB
	list, err := store.List(ctx, "alice", "portfolio")
	if err != nil {
		t.Errorf("List on empty DB should not error: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 records, got %d", len(list))
	}

	// Query on empty DB
	results, err := store.Query(ctx, "alice", "search", interfaces.QueryOptions{Limit: 10})
	if err != nil {
		t.Errorf("Query on empty DB should not error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}

	// Delete on empty DB
	if err := store.Delete(ctx, "alice", "portfolio", "SMSF"); err != nil {
		t.Errorf("Delete on empty DB should not error: %v", err)
	}

	// DeleteBySubject on empty DB
	count, err := store.DeleteBySubject(ctx, "portfolio")
	if err != nil {
		t.Errorf("DeleteBySubject on empty DB should not error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 deleted, got %d", count)
	}
}

// --- DeleteBySubject across users ---

func TestDeleteBySubject_AcrossAllUsers(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create records for multiple users
	for _, user := range []string{"alice", "bob", "charlie"} {
		store.Put(ctx, &models.UserRecord{
			UserID: user, Subject: "portfolio", Key: "SMSF",
			Value: fmt.Sprintf(`{"user":"%s"}`, user),
		})
		store.Put(ctx, &models.UserRecord{
			UserID: user, Subject: "strategy", Key: "SMSF",
			Value: fmt.Sprintf(`{"user":"%s"}`, user),
		})
	}

	// Delete all portfolios across all users
	count, err := store.DeleteBySubject(ctx, "portfolio")
	if err != nil {
		t.Fatalf("DeleteBySubject failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 portfolios deleted, got %d", count)
	}

	// Verify portfolios are gone for all users
	for _, user := range []string{"alice", "bob", "charlie"} {
		_, err := store.Get(ctx, user, "portfolio", "SMSF")
		if err == nil {
			t.Errorf("%s's portfolio should be deleted", user)
		}
	}

	// Verify strategies are intact for all users
	for _, user := range []string{"alice", "bob", "charlie"} {
		_, err := store.Get(ctx, user, "strategy", "SMSF")
		if err != nil {
			t.Errorf("%s's strategy should survive: %v", user, err)
		}
	}
}

// --- Double Close ---

func TestStore_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")
	store, err := NewStore(logger, filepath.Join(dir, "userdb"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	err = store.Close()
	t.Logf("Second close result: %v (panic-free is what matters)", err)
}

// --- matchSubject inconsistency ---

// matchSubject, List, and Query all use case-sensitive comparison.
func TestMatchSubject_CaseSensitive(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "Portfolio", Key: "SMSF",
		Value: `{"name":"SMSF"}`,
	})
	store.Put(ctx, &models.UserRecord{
		UserID: "alice", Subject: "portfolio", Key: "Trading",
		Value: `{"name":"Trading"}`,
	})

	// List uses case-sensitive comparison
	list, _ := store.List(ctx, "alice", "portfolio")
	if len(list) != 1 {
		t.Errorf("List('portfolio') expected 1, got %d", len(list))
	}

	listUpper, _ := store.List(ctx, "alice", "Portfolio")
	if len(listUpper) != 1 {
		t.Errorf("List('Portfolio') expected 1, got %d", len(listUpper))
	}

	// matchSubject is now case-sensitive, consistent with List/Query
	rec := &models.UserRecord{Subject: "Portfolio"}
	if matchSubject(rec, "portfolio") {
		t.Error("matchSubject should be case-sensitive: 'Portfolio' != 'portfolio'")
	}
	if !matchSubject(rec, "Portfolio") {
		t.Error("matchSubject should match same case")
	}
}

// --- Special character keys ---

func TestSpecialCharacters_AllFields(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	hostileInputs := []struct {
		name    string
		userID  string
		subject string
		key     string
	}{
		{"null_in_userid", "user\x00evil", "portfolio", "SMSF"},
		{"colon_in_userid", "user:evil", "portfolio", "SMSF"},
		{"colon_in_subject", "alice", "port:folio", "SMSF"},
		{"colon_in_key", "alice", "portfolio", "SM:SF"},
		{"all_colons", "a:b", "c:d", "e:f"},
		{"empty_subject", "alice", "", "SMSF"},
		{"empty_key", "alice", "portfolio", ""},
		{"empty_userid", "", "portfolio", "SMSF"},
		{"unicode_zwsp", "alice", "portfolio", "SMSF\u200B"},
		{"newlines", "alice", "portfolio", "SM\nSF"},
		{"very_long_key", "alice", "portfolio", strings.Repeat("a", 10000)},
	}

	for _, tc := range hostileInputs {
		t.Run(tc.name, func(t *testing.T) {
			rec := &models.UserRecord{
				UserID:  tc.userID,
				Subject: tc.subject,
				Key:     tc.key,
				Value:   `{"test":true}`,
			}
			err := store.Put(ctx, rec)
			if err != nil {
				t.Logf("Rejected (acceptable): %v", err)
				return
			}

			got, err := store.Get(ctx, tc.userID, tc.subject, tc.key)
			if err != nil {
				t.Errorf("Put succeeded but Get failed: %v", err)
				return
			}
			if got.Value != `{"test":true}` {
				t.Errorf("value mismatch: got %q", got.Value)
			}

			// Cleanup
			store.Delete(ctx, tc.userID, tc.subject, tc.key)
		})
	}
}
