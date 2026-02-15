package internaldb

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
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

func TestUserCRUD(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Save user
	user := &models.InternalUser{
		UserID:       "alice",
		Email:        "alice@example.com",
		PasswordHash: "hash123",
		Role:         "admin",
	}
	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	// Get user
	got, err := store.GetUser(ctx, "alice")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.UserID != "alice" || got.Email != "alice@example.com" {
		t.Errorf("got %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}

	// Update user (preserves CreatedAt)
	created := got.CreatedAt
	user.Email = "alice2@example.com"
	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatalf("SaveUser update: %v", err)
	}
	got, _ = store.GetUser(ctx, "alice")
	if got.Email != "alice2@example.com" {
		t.Error("Email not updated")
	}
	if !got.CreatedAt.Equal(created) {
		t.Error("CreatedAt should be preserved on update")
	}

	// List users
	ids, err := store.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(ids) != 1 || ids[0] != "alice" {
		t.Errorf("ListUsers: got %v", ids)
	}

	// Delete user
	if err := store.DeleteUser(ctx, "alice"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}

	// Verify deleted
	_, err = store.GetUser(ctx, "alice")
	if err == nil {
		t.Error("GetUser after delete should fail")
	}
}

func TestUserNotFound(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	_, err := store.GetUser(ctx, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent user")
	}
}

func TestUserKVCRUD(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Set
	if err := store.SetUserKV(ctx, "alice", "display_currency", "AUD"); err != nil {
		t.Fatalf("SetUserKV: %v", err)
	}

	// Get
	kv, err := store.GetUserKV(ctx, "alice", "display_currency")
	if err != nil {
		t.Fatalf("GetUserKV: %v", err)
	}
	if kv.Value != "AUD" || kv.Version != 1 {
		t.Errorf("got %+v", kv)
	}

	// Update (version increment)
	if err := store.SetUserKV(ctx, "alice", "display_currency", "USD"); err != nil {
		t.Fatalf("SetUserKV update: %v", err)
	}
	kv, _ = store.GetUserKV(ctx, "alice", "display_currency")
	if kv.Value != "USD" || kv.Version != 2 {
		t.Errorf("expected USD/v2, got %s/v%d", kv.Value, kv.Version)
	}

	// Add another key
	store.SetUserKV(ctx, "alice", "navexa_key", "secret123")

	// List
	kvs, err := store.ListUserKV(ctx, "alice")
	if err != nil {
		t.Fatalf("ListUserKV: %v", err)
	}
	if len(kvs) != 2 {
		t.Errorf("expected 2 KV entries, got %d", len(kvs))
	}

	// Delete
	if err := store.DeleteUserKV(ctx, "alice", "navexa_key"); err != nil {
		t.Fatalf("DeleteUserKV: %v", err)
	}
	kvs, _ = store.ListUserKV(ctx, "alice")
	if len(kvs) != 1 {
		t.Errorf("expected 1 KV entry after delete, got %d", len(kvs))
	}
}

func TestUserKVNotFound(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	_, err := store.GetUserKV(ctx, "nobody", "nothing")
	if err == nil {
		t.Error("expected error for nonexistent KV")
	}
}

func TestSystemKV(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Set system KV
	if err := store.SetSystemKV(ctx, "vire_schema_version", "3"); err != nil {
		t.Fatalf("SetSystemKV: %v", err)
	}

	// Get system KV
	val, err := store.GetSystemKV(ctx, "vire_schema_version")
	if err != nil {
		t.Fatalf("GetSystemKV: %v", err)
	}
	if val != "3" {
		t.Errorf("expected '3', got '%s'", val)
	}

	// Get nonexistent returns empty string (not error)
	val, err = store.GetSystemKV(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("GetSystemKV nonexistent: %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for nonexistent key, got '%s'", val)
	}
}

func TestDeleteUserCascadesKV(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	// Create user with KV entries
	store.SaveUser(ctx, &models.InternalUser{UserID: "bob", Email: "bob@test.com"})
	store.SetUserKV(ctx, "bob", "display_currency", "USD")
	store.SetUserKV(ctx, "bob", "navexa_key", "secret")

	// Delete user
	store.DeleteUser(ctx, "bob")

	// KV entries should also be deleted
	kvs, _ := store.ListUserKV(ctx, "bob")
	if len(kvs) != 0 {
		t.Errorf("expected 0 KV entries after user delete, got %d", len(kvs))
	}
}

func TestNewStoreInvalidPath(t *testing.T) {
	logger := common.NewLogger("debug")

	// Use a path that can't be created
	_, err := NewStore(logger, "/dev/null/impossible")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestCloseNilDB(t *testing.T) {
	store := &Store{}
	if err := store.Close(); err != nil {
		t.Errorf("Close on nil db should not error: %v", err)
	}
}

func TestUserKVDateTime(t *testing.T) {
	store := newUnitTestStore(t)
	ctx := context.Background()

	before := time.Now()
	store.SetUserKV(ctx, "alice", "key1", "val1")
	after := time.Now()

	kv, _ := store.GetUserKV(ctx, "alice", "key1")
	if kv.DateTime.Before(before) || kv.DateTime.After(after) {
		t.Error("DateTime should be between before and after")
	}
}

func TestNewStoreCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := dir + "/nested/deep"
	logger := common.NewLogger("debug")

	store, err := NewStore(logger, subdir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	store.Close()

	if _, err := os.Stat(subdir); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}
