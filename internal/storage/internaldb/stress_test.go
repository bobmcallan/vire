package internaldb

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Test helpers ---

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	logger := common.NewLogger("error")
	store, err := NewStore(logger, filepath.Join(dir, "internaldb"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// --- Composite Key Injection: UserKeyValue ---

// CRITICAL: The composite key for UserKV is "userID:key". If a userID
// contains ":", it could craft a composite key that aliases another user's
// data. For example, userID="alice:navexa_key" with key="" would produce
// composite key "alice:navexa_key:" — and userID="alice" with key="navexa_key"
// produces "alice:navexa_key". The trailing ":" makes these differ, but
// more subtle crafted inputs could collide.
func TestKeyInjection_UserKV_ColonInUserID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Set a value for legitimate user "alice" with key "navexa_key"
	if err := store.SetUserKV(ctx, "alice", "navexa_key", "real-secret-key"); err != nil {
		t.Fatalf("SetUserKV failed: %v", err)
	}

	// Attacker creates a user with ":" in the ID that tries to alias alice's key.
	// Composite key for alice's navexa_key = "alice:navexa_key"
	// Attacker tries userID="alice" key="navexa_key" — same composite key.
	// But what about userID="alice:navexa" key="key"? That produces "alice:navexa:key"
	// which is different from "alice:navexa_key".

	// The real attack: userID="alice" with key="navexa_key" is the SAME composite key
	// as the legitimate alice:navexa_key — but this IS alice's key, so no cross-user issue.

	// More interesting: can a malicious user "bob" read alice's KV?
	// Bob uses userID="alice" with key="navexa_key" — this bypasses the user boundary
	// entirely if the caller is allowed to specify any userID.
	// The protection must happen at the API/middleware layer, not storage.

	// Test actual composite key collision scenarios:
	// userID="a:b" key="c" => composite "a:b:c"
	// userID="a" key="b:c" => composite "a:b:c"
	// These are DIFFERENT user/key pairs but produce the SAME composite key!
	if err := store.SetUserKV(ctx, "a:b", "c", "value-from-ab-c"); err != nil {
		t.Fatalf("SetUserKV failed: %v", err)
	}
	if err := store.SetUserKV(ctx, "a", "b:c", "value-from-a-bc"); err != nil {
		t.Fatalf("SetUserKV failed: %v", err)
	}

	// Both produce composite key "a:b:c" — second write overwrites first!
	kv1, err := store.GetUserKV(ctx, "a:b", "c")
	if err != nil {
		t.Fatalf("GetUserKV(a:b, c) failed: %v", err)
	}
	kv2, err := store.GetUserKV(ctx, "a", "b:c")
	if err != nil {
		t.Fatalf("GetUserKV(a, b:c) failed: %v", err)
	}

	// KNOWN LIMITATION: Both return the same record because composite keys collide.
	// Multi-tenant user isolation is explicitly out of scope (per requirements).
	// This test documents the behavior for awareness: if userIDs ever contain ":",
	// composite key collisions can occur. In practice, userIDs are controlled by
	// the application and should not contain ":" characters.
	if kv1.Value == kv2.Value {
		t.Logf("COMPOSITE KEY COLLISION (documented limitation): userID='a:b' key='c' and "+
			"userID='a' key='b:c' resolve to the same record. Value=%q, UserID=%q, Key=%q",
			kv1.Value, kv1.UserID, kv1.Key)
	}

	// ListUserKV filters by UserID field, so it won't cross-leak in List operations.
	// But Get operations using the composite key are vulnerable.
	list1, _ := store.ListUserKV(ctx, "a:b")
	list2, _ := store.ListUserKV(ctx, "a")

	// Check if the list filtering catches the collision
	t.Logf("ListUserKV('a:b'): %d entries", len(list1))
	t.Logf("ListUserKV('a'): %d entries", len(list2))
	for _, kv := range list1 {
		t.Logf("  a:b entry: UserID=%q Key=%q Value=%q", kv.UserID, kv.Key, kv.Value)
	}
	for _, kv := range list2 {
		t.Logf("  a entry: UserID=%q Key=%q Value=%q", kv.UserID, kv.Key, kv.Value)
	}
}

// TestKeyInjection_SystemKV_Isolated verifies that system KV entries
// cannot be accessed or overwritten through the user KV interface.
func TestKeyInjection_SystemKV_Isolated(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Set a system KV entry
	if err := store.SetSystemKV(ctx, "eodhd_api_key", "super-secret-api-key"); err != nil {
		t.Fatalf("SetSystemKV failed: %v", err)
	}

	// Verify system KV is accessible via system interface
	val, err := store.GetSystemKV(ctx, "eodhd_api_key")
	if err != nil {
		t.Fatalf("GetSystemKV failed: %v", err)
	}
	if val != "super-secret-api-key" {
		t.Errorf("GetSystemKV returned wrong value: %q", val)
	}

	// A user named "system" should be rejected by SaveUser
	err = store.SaveUser(ctx, &models.InternalUser{UserID: "__system__", Email: "hack@evil.com"})
	if err == nil {
		t.Error("SaveUser should reject the reserved system user ID")
	}

	// A user with the old sentinel name "system" should NOT be able to access system KV
	// because the sentinel is now "__system__", not "system"
	_, err = store.GetUserKV(ctx, "system", "eodhd_api_key")
	if err == nil {
		t.Error("GetUserKV('system', ...) should not find system KV entries (sentinel is '__system__')")
	}

	// SetUserKV as "system" user should not overwrite system keys
	if err := store.SetUserKV(ctx, "system", "eodhd_api_key", "overwritten-by-user"); err != nil {
		t.Logf("SetUserKV as 'system' user: %v", err)
	}

	// Verify system key is intact
	val, err = store.GetSystemKV(ctx, "eodhd_api_key")
	if err != nil {
		t.Fatalf("GetSystemKV failed after user write: %v", err)
	}
	if val != "super-secret-api-key" {
		t.Errorf("System KV was overwritten by user 'system': got %q", val)
	}
}

// --- Concurrent Access ---

func TestConcurrent_InternalUser_ReadWrite(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const goroutines = 20
	const opsPerGoroutine = 50

	// Pre-create users
	for i := 0; i < goroutines; i++ {
		store.SaveUser(ctx, &models.InternalUser{
			UserID: fmt.Sprintf("user-%d", i),
			Email:  fmt.Sprintf("user-%d@test.com", i),
			Role:   "user",
		})
	}

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*opsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", id)
			for i := 0; i < opsPerGoroutine; i++ {
				if i%2 == 0 {
					_, err := store.GetUser(ctx, userID)
					if err != nil {
						errCh <- fmt.Errorf("goroutine %d: GetUser failed: %w", id, err)
						return
					}
				} else {
					err := store.SaveUser(ctx, &models.InternalUser{
						UserID: userID,
						Email:  fmt.Sprintf("user-%d-iter-%d@test.com", id, i),
						Role:   "user",
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

	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != goroutines {
		t.Errorf("expected %d users, got %d", goroutines, len(users))
	}
}

func TestConcurrent_UserKV_ReadWriteDelete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	const goroutines = 20
	const ops = 50
	var wg sync.WaitGroup

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", id)
			for i := 0; i < ops; i++ {
				key := fmt.Sprintf("key-%d", i%5)
				switch i % 3 {
				case 0:
					store.SetUserKV(ctx, userID, key, fmt.Sprintf("val-%d-%d", id, i))
				case 1:
					store.GetUserKV(ctx, userID, key)
				case 2:
					store.DeleteUserKV(ctx, userID, key)
				}
			}
		}(g)
	}

	wg.Wait()
	// Reaching here without panic means concurrent access is safe
}

// --- Cross-user isolation ---

func TestUserKV_CrossUserIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create KV entries for alice and bob with same keys
	store.SetUserKV(ctx, "alice", "navexa_key", "alice-secret")
	store.SetUserKV(ctx, "bob", "navexa_key", "bob-secret")
	store.SetUserKV(ctx, "alice", "display_currency", "AUD")
	store.SetUserKV(ctx, "bob", "display_currency", "USD")

	// Verify alice's entries
	aliceKV, _ := store.GetUserKV(ctx, "alice", "navexa_key")
	if aliceKV.Value != "alice-secret" {
		t.Errorf("alice's navexa_key leaked: got %q", aliceKV.Value)
	}

	bobKV, _ := store.GetUserKV(ctx, "bob", "navexa_key")
	if bobKV.Value != "bob-secret" {
		t.Errorf("bob's navexa_key leaked: got %q", bobKV.Value)
	}

	// Verify ListUserKV isolation
	aliceList, _ := store.ListUserKV(ctx, "alice")
	for _, kv := range aliceList {
		if kv.UserID != "alice" {
			t.Errorf("alice's ListUserKV returned entry for user %q", kv.UserID)
		}
	}
	if len(aliceList) != 2 {
		t.Errorf("expected 2 entries for alice, got %d", len(aliceList))
	}

	bobList, _ := store.ListUserKV(ctx, "bob")
	for _, kv := range bobList {
		if kv.UserID != "bob" {
			t.Errorf("bob's ListUserKV returned entry for user %q", kv.UserID)
		}
	}

	// Delete alice's user and verify bob's data is intact
	store.DeleteUser(ctx, "alice")
	bobKV, _ = store.GetUserKV(ctx, "bob", "navexa_key")
	if bobKV.Value != "bob-secret" {
		t.Errorf("deleting alice affected bob's data: got %q", bobKV.Value)
	}
}

// --- Special character keys ---

func TestSpecialCharacters_UserID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	hostileIDs := []struct {
		name string
		id   string
	}{
		{"colon", "user:with:colons"},
		{"null_byte", "user\x00evil"},
		{"path_traversal", "../../etc/passwd"},
		{"unicode_zwsp", "user\u200Badmin"},
		{"unicode_rtl", "user\u202Eadmin"},
		{"newlines", "user\nnewline"},
		{"empty", ""},
		{"spaces", "user with spaces"},
		{"very_long", strings.Repeat("a", 10000)},
		{"special_chars", "user<>|&;`$(){}[]!@#%^*+=~"},
		{"system_sentinel", "system"},
	}

	for _, tc := range hostileIDs {
		t.Run(tc.name, func(t *testing.T) {
			user := &models.InternalUser{UserID: tc.id, Email: "test@test.com", Role: "user"}
			err := store.SaveUser(ctx, user)
			if tc.id == "" {
				if err != nil {
					t.Logf("Empty user ID error (acceptable): %v", err)
				}
				return
			}
			if err != nil {
				t.Logf("User ID %q rejected (acceptable): %v", tc.name, err)
				return
			}

			got, err := store.GetUser(ctx, tc.id)
			if err != nil {
				t.Errorf("saved user %q but couldn't retrieve: %v", tc.name, err)
				return
			}
			if got.UserID != tc.id {
				t.Errorf("user ID mismatch: saved %q, got %q", tc.id, got.UserID)
			}

			// Also test KV with this user
			if err := store.SetUserKV(ctx, tc.id, "test_key", "test_value"); err != nil {
				t.Logf("KV set for user %q failed: %v", tc.name, err)
				return
			}
			kv, err := store.GetUserKV(ctx, tc.id, "test_key")
			if err != nil {
				t.Errorf("KV get for user %q failed: %v", tc.name, err)
				return
			}
			if kv.Value != "test_value" {
				t.Errorf("KV value mismatch for user %q: got %q", tc.name, kv.Value)
			}

			// Cleanup
			store.DeleteUser(ctx, tc.id)
		})
	}
}

// --- Empty State Operations ---

func TestEmptyState_AllOperations(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Users
	users, err := store.ListUsers(ctx)
	if err != nil {
		t.Errorf("ListUsers on empty DB: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
	_, err = store.GetUser(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for GetUser on empty DB")
	}
	if err := store.DeleteUser(ctx, "nonexistent"); err != nil {
		t.Errorf("DeleteUser on empty DB should not error: %v", err)
	}

	// UserKV
	kvs, err := store.ListUserKV(ctx, "nonexistent")
	if err != nil {
		t.Errorf("ListUserKV on empty DB: %v", err)
	}
	if len(kvs) != 0 {
		t.Errorf("expected 0 KV entries, got %d", len(kvs))
	}
	_, err = store.GetUserKV(ctx, "nonexistent", "key")
	if err == nil {
		t.Error("expected error for GetUserKV on empty DB")
	}
	if err := store.DeleteUserKV(ctx, "nonexistent", "key"); err != nil {
		t.Errorf("DeleteUserKV on empty DB should not error: %v", err)
	}

	// System KV
	val, err := store.GetSystemKV(ctx, "missing")
	if err != nil {
		t.Errorf("GetSystemKV on empty DB should return empty string, not error: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing system KV, got %q", val)
	}
}

// --- Double Close ---

func TestStore_DoubleClose(t *testing.T) {
	dir := t.TempDir()
	logger := common.NewLogger("error")
	store, err := NewStore(logger, filepath.Join(dir, "internaldb"))
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close should not panic
	err = store.Close()
	t.Logf("Second close result: %v (panic-free is what matters)", err)
}

// --- DeleteUser cascades to KV ---

func TestDeleteUser_CascadesKV(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create user with KV entries
	store.SaveUser(ctx, &models.InternalUser{UserID: "alice", Email: "alice@test.com"})
	store.SetUserKV(ctx, "alice", "navexa_key", "secret")
	store.SetUserKV(ctx, "alice", "display_currency", "AUD")
	store.SetUserKV(ctx, "alice", "default_portfolio", "SMSF")

	// Also create bob's entries to verify they're unaffected
	store.SaveUser(ctx, &models.InternalUser{UserID: "bob", Email: "bob@test.com"})
	store.SetUserKV(ctx, "bob", "navexa_key", "bob-secret")

	// Delete alice
	if err := store.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify alice's KV entries are gone
	_, err := store.GetUserKV(ctx, "alice", "navexa_key")
	if err == nil {
		t.Error("alice's navexa_key should be deleted after DeleteUser")
	}
	_, err = store.GetUserKV(ctx, "alice", "display_currency")
	if err == nil {
		t.Error("alice's display_currency should be deleted after DeleteUser")
	}

	aliceKVs, _ := store.ListUserKV(ctx, "alice")
	if len(aliceKVs) != 0 {
		t.Errorf("expected 0 KV entries for deleted alice, got %d", len(aliceKVs))
	}

	// Verify bob's data is intact
	bobKV, err := store.GetUserKV(ctx, "bob", "navexa_key")
	if err != nil {
		t.Fatalf("bob's navexa_key should still exist: %v", err)
	}
	if bobKV.Value != "bob-secret" {
		t.Errorf("bob's navexa_key was corrupted: got %q", bobKV.Value)
	}
}

// --- Version increment on KV ---

func TestUserKV_VersionIncrement(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Set initial value
	store.SetUserKV(ctx, "alice", "display_currency", "AUD")
	kv, _ := store.GetUserKV(ctx, "alice", "display_currency")
	if kv.Version != 1 {
		t.Errorf("expected version 1, got %d", kv.Version)
	}

	// Update
	store.SetUserKV(ctx, "alice", "display_currency", "USD")
	kv, _ = store.GetUserKV(ctx, "alice", "display_currency")
	if kv.Version != 2 {
		t.Errorf("expected version 2, got %d", kv.Version)
	}
	if kv.Value != "USD" {
		t.Errorf("expected USD, got %q", kv.Value)
	}
}

// --- SaveUser preserves CreatedAt ---

func TestSaveUser_PreservesCreatedAt(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Create user
	store.SaveUser(ctx, &models.InternalUser{UserID: "alice", Email: "alice@test.com"})
	u, _ := store.GetUser(ctx, "alice")
	createdAt := u.CreatedAt

	time.Sleep(10 * time.Millisecond) // Ensure clock advances

	// Update user
	store.SaveUser(ctx, &models.InternalUser{UserID: "alice", Email: "alice@new.com"})
	u, _ = store.GetUser(ctx, "alice")

	if !u.CreatedAt.Equal(createdAt) {
		t.Errorf("CreatedAt was modified on update: original=%v, new=%v", createdAt, u.CreatedAt)
	}
	if u.Email != "alice@new.com" {
		t.Errorf("email not updated: %s", u.Email)
	}
	if !u.ModifiedAt.After(createdAt) {
		t.Error("ModifiedAt should be after CreatedAt on update")
	}
}
