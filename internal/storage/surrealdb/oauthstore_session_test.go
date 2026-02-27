package surrealdb

import (
	"context"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthStore_SaveAndGetSession(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	sess := &models.OAuthSession{
		SessionID:     "sess-001",
		ClientID:      "client-abc",
		RedirectURI:   "http://localhost:3000/callback",
		State:         "random-state",
		CodeChallenge: "challenge123",
		CodeMethod:    "S256",
		Scope:         "vire",
		CreatedAt:     time.Now(),
	}

	err := store.SaveSession(ctx, sess)
	require.NoError(t, err)

	got, err := store.GetSession(ctx, "sess-001")
	require.NoError(t, err)
	assert.Equal(t, "sess-001", got.SessionID)
	assert.Equal(t, "client-abc", got.ClientID)
	assert.Equal(t, "http://localhost:3000/callback", got.RedirectURI)
	assert.Equal(t, "random-state", got.State)
	assert.Equal(t, "challenge123", got.CodeChallenge)
	assert.Equal(t, "S256", got.CodeMethod)
	assert.Equal(t, "vire", got.Scope)
	assert.Empty(t, got.UserID)
}

func TestOAuthStore_GetSession_Expired(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	sess := &models.OAuthSession{
		SessionID: "sess-expired",
		ClientID:  "client-exp",
		CreatedAt: time.Now().Add(-15 * time.Minute), // expired (> 10 min)
	}
	err := store.SaveSession(ctx, sess)
	require.NoError(t, err)

	_, err = store.GetSession(ctx, "sess-expired")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOAuthStore_GetSession_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	_, err := store.GetSession(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOAuthStore_GetSessionByClientID(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	// Create two sessions for same client
	sess1 := &models.OAuthSession{
		SessionID: "sess-cid-1",
		ClientID:  "client-multi",
		State:     "state-1",
		CreatedAt: time.Now().Add(-5 * time.Minute),
	}
	sess2 := &models.OAuthSession{
		SessionID: "sess-cid-2",
		ClientID:  "client-multi",
		State:     "state-2",
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.SaveSession(ctx, sess1))
	require.NoError(t, store.SaveSession(ctx, sess2))

	// Should return the latest session
	got, err := store.GetSessionByClientID(ctx, "client-multi")
	require.NoError(t, err)
	assert.Equal(t, "sess-cid-2", got.SessionID)
}

func TestOAuthStore_GetSessionByClientID_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	_, err := store.GetSessionByClientID(ctx, "no-such-client")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestOAuthStore_UpdateSessionUserID(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	sess := &models.OAuthSession{
		SessionID: "sess-update",
		ClientID:  "client-u",
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.SaveSession(ctx, sess))

	err := store.UpdateSessionUserID(ctx, "sess-update", "user-42")
	require.NoError(t, err)

	got, err := store.GetSession(ctx, "sess-update")
	require.NoError(t, err)
	assert.Equal(t, "user-42", got.UserID)
}

func TestOAuthStore_DeleteSession(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	sess := &models.OAuthSession{
		SessionID: "sess-delete",
		ClientID:  "client-d",
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.SaveSession(ctx, sess))

	err := store.DeleteSession(ctx, "sess-delete")
	require.NoError(t, err)

	_, err = store.GetSession(ctx, "sess-delete")
	assert.Error(t, err)
}

func TestOAuthStore_DeleteSession_NotFound(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	// Should not error on deleting non-existent session
	err := store.DeleteSession(ctx, "nonexistent-sess")
	assert.NoError(t, err)
}

func TestOAuthStore_PurgeExpiredSessions(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	// Create one expired and one fresh session
	expired := &models.OAuthSession{
		SessionID: "sess-purge-old",
		ClientID:  "client-old",
		CreatedAt: time.Now().Add(-15 * time.Minute),
	}
	fresh := &models.OAuthSession{
		SessionID: "sess-purge-new",
		ClientID:  "client-new",
		CreatedAt: time.Now(),
	}
	require.NoError(t, store.SaveSession(ctx, expired))
	require.NoError(t, store.SaveSession(ctx, fresh))

	_, err := store.PurgeExpiredSessions(ctx)
	require.NoError(t, err)

	// Fresh session should still be accessible
	got, err := store.GetSession(ctx, "sess-purge-new")
	require.NoError(t, err)
	assert.Equal(t, "sess-purge-new", got.SessionID)

	// Expired session should be gone (already expired via TTL, and now purged from DB)
	_, err = store.GetSession(ctx, "sess-purge-old")
	assert.Error(t, err)
}

func TestOAuthStore_SaveClient_NewFields(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	client := &models.OAuthClient{
		ClientID:                "cl-new-fields",
		ClientName:              "Portal Client",
		RedirectURIs:            []string{"http://localhost/cb"},
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
		TokenEndpointAuthMethod: "client_secret_post",
		CreatedAt:               time.Now(),
	}
	require.NoError(t, store.SaveClient(ctx, client))

	got, err := store.GetClient(ctx, "cl-new-fields")
	require.NoError(t, err)
	assert.Equal(t, "cl-new-fields", got.ClientID)
	assert.Equal(t, []string{"authorization_code", "refresh_token"}, got.GrantTypes)
	assert.Equal(t, []string{"code"}, got.ResponseTypes)
	assert.Equal(t, "client_secret_post", got.TokenEndpointAuthMethod)
}

func TestOAuthStore_SaveClient_BackwardCompatible(t *testing.T) {
	db := testDB(t)
	store := NewOAuthStore(db, testLogger())
	ctx := context.Background()

	// Save without new fields (backward compatibility)
	client := &models.OAuthClient{
		ClientID:     "cl-compat",
		ClientName:   "Old Client",
		RedirectURIs: []string{"http://localhost/cb"},
		CreatedAt:    time.Now(),
	}
	require.NoError(t, store.SaveClient(ctx, client))

	got, err := store.GetClient(ctx, "cl-compat")
	require.NoError(t, err)
	assert.Equal(t, "cl-compat", got.ClientID)
	assert.Empty(t, got.GrantTypes)
	assert.Empty(t, got.ResponseTypes)
	assert.Empty(t, got.TokenEndpointAuthMethod)
}
