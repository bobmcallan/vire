package surrealdb

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetUser(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	user := &models.InternalUser{
		UserID:       "testuser1",
		Email:        "test@example.com",
		PasswordHash: "hash123",
		Provider:     "email",
		Role:         "user",
		CreatedAt:    time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	got, err := store.GetUser(ctx, "testuser1")
	require.NoError(t, err)
	assert.Equal(t, "testuser1", got.UserID)
	assert.Equal(t, "test@example.com", got.Email)
	assert.Equal(t, "email", got.Provider)
	assert.Equal(t, "user", got.Role)
}

func TestGetUserNotFound(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	_, err := store.GetUser(ctx, "nonexistent")
	require.Error(t, err)
}

func TestSaveUser(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	tests := []struct {
		name string
		user *models.InternalUser
	}{
		{
			name: "basic user",
			user: &models.InternalUser{
				UserID:       "save_basic",
				Email:        "basic@test.com",
				PasswordHash: "hash",
				Provider:     "email",
				Role:         "user",
				CreatedAt:    time.Now().Truncate(time.Second),
			},
		},
		{
			name: "admin user",
			user: &models.InternalUser{
				UserID:       "save_admin",
				Email:        "admin@test.com",
				PasswordHash: "adminhash",
				Provider:     "google",
				Role:         "admin",
				CreatedAt:    time.Now().Truncate(time.Second),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.SaveUser(ctx, tt.user)
			require.NoError(t, err)

			got, err := store.GetUser(ctx, tt.user.UserID)
			require.NoError(t, err)
			assert.Equal(t, tt.user.UserID, got.UserID)
			assert.Equal(t, tt.user.Email, got.Email)
			assert.Equal(t, tt.user.Role, got.Role)
		})
	}
}

func TestSaveUserOverwrite(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	user := &models.InternalUser{
		UserID:       "overwrite_user",
		Email:        "v1@test.com",
		PasswordHash: "hash1",
		Role:         "user",
		CreatedAt:    time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	user.Email = "v2@test.com"
	user.Role = "admin"
	require.NoError(t, store.SaveUser(ctx, user))

	got, err := store.GetUser(ctx, "overwrite_user")
	require.NoError(t, err)
	assert.Equal(t, "v2@test.com", got.Email)
	assert.Equal(t, "admin", got.Role)
}

func TestDeleteUser(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	user := &models.InternalUser{
		UserID:       "delete_me",
		Email:        "delete@test.com",
		PasswordHash: "hash",
		Role:         "user",
		CreatedAt:    time.Now().Truncate(time.Second),
	}
	require.NoError(t, store.SaveUser(ctx, user))

	err := store.DeleteUser(ctx, "delete_me")
	require.NoError(t, err)

	_, err = store.GetUser(ctx, "delete_me")
	assert.Error(t, err)
}

func TestDeleteNonexistentUser(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	// SurrealDB Delete on non-existent record should not error
	err := store.DeleteUser(ctx, "ghost_user")
	assert.NoError(t, err)
}

func TestListUsers(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	for _, id := range []string{"list_a", "list_b", "list_c"} {
		require.NoError(t, store.SaveUser(ctx, &models.InternalUser{
			UserID:       id,
			Email:        id + "@test.com",
			PasswordHash: "hash",
			Role:         "user",
			CreatedAt:    time.Now().Truncate(time.Second),
		}))
	}

	users, err := store.ListUsers(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(users), 3)
	assert.Contains(t, users, "list_a")
	assert.Contains(t, users, "list_b")
	assert.Contains(t, users, "list_c")
}

func TestSetUserKV(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	err := store.SetUserKV(ctx, "kvuser", "theme", "dark")
	require.NoError(t, err)

	kv, err := store.GetUserKV(ctx, "kvuser", "theme")
	require.NoError(t, err)
	assert.Equal(t, "kvuser", kv.UserID)
	assert.Equal(t, "theme", kv.Key)
	assert.Equal(t, "dark", kv.Value)
}

func TestGetUserKVNotFound(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	_, err := store.GetUserKV(ctx, "nobody", "nokey")
	assert.Error(t, err)
}

func TestSetUserKVOverwrite(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	require.NoError(t, store.SetUserKV(ctx, "kvuser2", "lang", "en"))
	require.NoError(t, store.SetUserKV(ctx, "kvuser2", "lang", "fr"))

	kv, err := store.GetUserKV(ctx, "kvuser2", "lang")
	require.NoError(t, err)
	assert.Equal(t, "fr", kv.Value)
}

func TestDeleteUserKV(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	require.NoError(t, store.SetUserKV(ctx, "kvuser3", "temp", "value"))

	err := store.DeleteUserKV(ctx, "kvuser3", "temp")
	require.NoError(t, err)

	_, err = store.GetUserKV(ctx, "kvuser3", "temp")
	assert.Error(t, err)
}

func TestListUserKV(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	require.NoError(t, store.SetUserKV(ctx, "kvlist", "key1", "val1"))
	require.NoError(t, store.SetUserKV(ctx, "kvlist", "key2", "val2"))
	require.NoError(t, store.SetUserKV(ctx, "kvlist", "key3", "val3"))

	kvs, err := store.ListUserKV(ctx, "kvlist")
	require.NoError(t, err)
	assert.Len(t, kvs, 3)

	keys := make(map[string]string)
	for _, kv := range kvs {
		keys[kv.Key] = kv.Value
	}
	assert.Equal(t, "val1", keys["key1"])
	assert.Equal(t, "val2", keys["key2"])
	assert.Equal(t, "val3", keys["key3"])
}

func TestGetSystemKV(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	require.NoError(t, store.SetSystemKV(ctx, "version", "1.0.0"))

	val, err := store.GetSystemKV(ctx, "version")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", val)
}

func TestGetSystemKVNotFound(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	_, err := store.GetSystemKV(ctx, "missing_key")
	assert.Error(t, err)
}

func TestSetSystemKV(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	tests := []struct {
		key   string
		value string
	}{
		{"sys_key1", "sys_val1"},
		{"sys_key2", "sys_val2"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			require.NoError(t, store.SetSystemKV(ctx, tt.key, tt.value))

			got, err := store.GetSystemKV(ctx, tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestSetSystemKVOverwrite(t *testing.T) {
	db := testDB(t)
	store := NewInternalStore(db, testLogger())
	ctx := context.Background()

	require.NoError(t, store.SetSystemKV(ctx, "overwrite_key", "v1"))
	require.NoError(t, store.SetSystemKV(ctx, "overwrite_key", "v2"))

	val, err := store.GetSystemKV(ctx, "overwrite_key")
	require.NoError(t, err)
	assert.Equal(t, "v2", val)
}
