package data

import (
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Create
	user := &models.InternalUser{
		UserID:       "lifecycle_user",
		Email:        "lifecycle@test.com",
		PasswordHash: "bcrypt_hash",
		Provider:     "email",
		Role:         "user",
		CreatedAt:    time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	// Read
	got, err := store.GetUser(ctx, "lifecycle_user")
	require.NoError(t, err)
	assert.Equal(t, "lifecycle@test.com", got.Email)
	assert.Equal(t, "user", got.Role)

	// Update
	got.Email = "updated@test.com"
	got.Role = "admin"
	require.NoError(t, store.SaveUser(ctx, got))

	updated, err := store.GetUser(ctx, "lifecycle_user")
	require.NoError(t, err)
	assert.Equal(t, "updated@test.com", updated.Email)
	assert.Equal(t, "admin", updated.Role)

	// List should contain the user
	users, err := store.ListUsers(ctx)
	require.NoError(t, err)
	assert.Contains(t, users, "lifecycle_user")

	// Delete
	require.NoError(t, store.DeleteUser(ctx, "lifecycle_user"))

	_, err = store.GetUser(ctx, "lifecycle_user")
	assert.Error(t, err)
}

func TestUserKVLifecycle(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Set multiple KVs
	require.NoError(t, store.SetUserKV(ctx, "kvlc", "theme", "dark"))
	require.NoError(t, store.SetUserKV(ctx, "kvlc", "lang", "en"))
	require.NoError(t, store.SetUserKV(ctx, "kvlc", "tz", "UTC"))

	// Read single
	kv, err := store.GetUserKV(ctx, "kvlc", "theme")
	require.NoError(t, err)
	assert.Equal(t, "dark", kv.Value)

	// List all for user
	kvs, err := store.ListUserKV(ctx, "kvlc")
	require.NoError(t, err)
	assert.Len(t, kvs, 3)

	// Update
	require.NoError(t, store.SetUserKV(ctx, "kvlc", "theme", "light"))
	kv, err = store.GetUserKV(ctx, "kvlc", "theme")
	require.NoError(t, err)
	assert.Equal(t, "light", kv.Value)

	// Delete
	require.NoError(t, store.DeleteUserKV(ctx, "kvlc", "theme"))
	_, err = store.GetUserKV(ctx, "kvlc", "theme")
	assert.Error(t, err)

	// Remaining KVs still exist
	kvs, err = store.ListUserKV(ctx, "kvlc")
	require.NoError(t, err)
	assert.Len(t, kvs, 2)
}

func TestSystemKV(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Set
	require.NoError(t, store.SetSystemKV(ctx, "default_portfolio", "smsf"))
	require.NoError(t, store.SetSystemKV(ctx, "schema_version", "2.0"))

	// Get
	val, err := store.GetSystemKV(ctx, "default_portfolio")
	require.NoError(t, err)
	assert.Equal(t, "smsf", val)

	val, err = store.GetSystemKV(ctx, "schema_version")
	require.NoError(t, err)
	assert.Equal(t, "2.0", val)

	// Overwrite
	require.NoError(t, store.SetSystemKV(ctx, "default_portfolio", "growth"))
	val, err = store.GetSystemKV(ctx, "default_portfolio")
	require.NoError(t, err)
	assert.Equal(t, "growth", val)

	// Missing key
	_, err = store.GetSystemKV(ctx, "nonexistent")
	assert.Error(t, err)
}

func TestGetUserByEmail(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Setup: create users with emails
	alice := &models.InternalUser{
		UserID:    "google_alice",
		Email:     "alice@example.com",
		Provider:  "google",
		Role:      "user",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	bob := &models.InternalUser{
		UserID:    "github_bob",
		Email:     "bob@example.com",
		Provider:  "github",
		Role:      "user",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, alice))
	require.NoError(t, store.SaveUser(ctx, bob))

	tests := []struct {
		name      string
		email     string
		wantID    string
		wantFound bool
	}{
		{"exact match", "alice@example.com", "google_alice", true},
		{"case insensitive uppercase", "ALICE@EXAMPLE.COM", "google_alice", true},
		{"case insensitive mixed", "Alice@Example.COM", "google_alice", true},
		{"different user", "bob@example.com", "github_bob", true},
		{"not found", "nobody@example.com", "", false},
		{"empty email", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := store.GetUserByEmail(ctx, tt.email)
			if tt.wantFound {
				require.NoError(t, err)
				assert.Equal(t, tt.wantID, got.UserID)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestGetUserByEmail_NoAccidentalMatchOnEmpty(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Create a user with empty email
	user := &models.InternalUser{
		UserID:    "github_noemail",
		Email:     "",
		Provider:  "github",
		Role:      "user",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	// Searching for empty email should not match the user with empty email
	_, err := store.GetUserByEmail(ctx, "")
	assert.Error(t, err, "empty email search should not match users with empty email")
}

func TestGetUserByEmail_ReturnsFirstMatch(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Create a user and verify GetUserByEmail returns the full user object
	user := &models.InternalUser{
		UserID:    "google_fullcheck",
		Email:     "fullcheck@example.com",
		Name:      "Full Check",
		Provider:  "google",
		Role:      "admin",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	got, err := store.GetUserByEmail(ctx, "fullcheck@example.com")
	require.NoError(t, err)
	assert.Equal(t, "google_fullcheck", got.UserID)
	assert.Equal(t, "fullcheck@example.com", got.Email)
	assert.Equal(t, "Full Check", got.Name)
	assert.Equal(t, "google", got.Provider)
	assert.Equal(t, "admin", got.Role)
}

func TestConcurrentUserAccess(t *testing.T) {
	mgr := testManager(t)
	store := mgr.InternalStore()
	ctx := testContext()

	// Create initial user
	user := &models.InternalUser{
		UserID:       "concurrent_user",
		Email:        "concurrent@test.com",
		PasswordHash: "hash",
		Role:         "user",
		CreatedAt:    time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	// Run concurrent reads
	var wg sync.WaitGroup
	errs := make([]error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = store.GetUser(ctx, "concurrent_user")
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		assert.NoError(t, err)
	}
}
