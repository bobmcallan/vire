package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Session endpoint tests ---

func TestInternalOAuth_CreateSession(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"session_id":     "sess-001",
		"client_id":      "client-abc",
		"redirect_uri":   "http://localhost:3000/callback",
		"state":          "random-state",
		"code_challenge": "abc123",
		"code_method":    "S256",
		"scope":          "vire",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/sessions", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp models.OAuthSession
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "sess-001", resp.SessionID)
	assert.Equal(t, "client-abc", resp.ClientID)
	assert.False(t, resp.CreatedAt.IsZero())
}

func TestInternalOAuth_CreateSession_MissingFields(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"session_id": "sess-002",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/sessions", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInternalOAuth_GetSessionByID(t *testing.T) {
	srv := newOAuthTestServer(t)

	// Create session first
	sess := &models.OAuthSession{
		SessionID:   "sess-get-1",
		ClientID:    "client-1",
		RedirectURI: "http://localhost/cb",
		State:       "st",
		Scope:       "vire",
		CreatedAt:   time.Now(),
	}
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.sessions[sess.SessionID] = sess

	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/sessions/sess-get-1", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.OAuthSession
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "sess-get-1", resp.SessionID)
}

func TestInternalOAuth_GetSession_NotFound(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/sessions/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInternalOAuth_GetSessionByClientID(t *testing.T) {
	srv := newOAuthTestServer(t)

	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.sessions["sess-q1"] = &models.OAuthSession{
		SessionID: "sess-q1",
		ClientID:  "client-q",
		CreatedAt: time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/sessions?client_id=client-q", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.OAuthSession
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "sess-q1", resp.SessionID)
}

func TestInternalOAuth_GetSessionByClientID_MissingParam(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/sessions", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInternalOAuth_PatchSession(t *testing.T) {
	srv := newOAuthTestServer(t)

	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.sessions["sess-patch"] = &models.OAuthSession{
		SessionID: "sess-patch",
		ClientID:  "c1",
		CreatedAt: time.Now(),
	}

	body := jsonBody(t, map[string]interface{}{"user_id": "user-123"})
	req := httptest.NewRequest(http.MethodPatch, "/api/internal/oauth/sessions/sess-patch", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "user-123", store.sessions["sess-patch"].UserID)
}

func TestInternalOAuth_PatchSession_MissingUserID(t *testing.T) {
	srv := newOAuthTestServer(t)

	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.sessions["sess-patch2"] = &models.OAuthSession{
		SessionID: "sess-patch2",
		ClientID:  "c2",
		CreatedAt: time.Now(),
	}

	body := jsonBody(t, map[string]interface{}{})
	req := httptest.NewRequest(http.MethodPatch, "/api/internal/oauth/sessions/sess-patch2", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInternalOAuth_DeleteSession(t *testing.T) {
	srv := newOAuthTestServer(t)

	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.sessions["sess-del"] = &models.OAuthSession{
		SessionID: "sess-del",
		ClientID:  "c1",
		CreatedAt: time.Now(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/internal/oauth/sessions/sess-del", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, store.sessions["sess-del"])
}

// --- Client endpoint tests ---

func TestInternalOAuth_SaveClient(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"client_id":                  "cl-001",
		"client_name":                "Test Portal",
		"redirect_uris":              []string{"http://localhost/cb"},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "client_secret_post",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/clients", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	assert.Contains(t, store.clients, "cl-001")
	assert.Equal(t, []string{"authorization_code", "refresh_token"}, store.clients["cl-001"].GrantTypes)
}

func TestInternalOAuth_SaveClient_MissingID(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"client_name": "No ID",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/clients", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInternalOAuth_GetClient(t *testing.T) {
	srv := newOAuthTestServer(t)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.clients["cl-get"] = &models.OAuthClient{
		ClientID:   "cl-get",
		ClientName: "Retrieved",
		CreatedAt:  time.Now(),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/clients/cl-get", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.OAuthClient
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "cl-get", resp.ClientID)
}

func TestInternalOAuth_GetClient_NotFound(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/clients/missing", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInternalOAuth_DeleteClient(t *testing.T) {
	srv := newOAuthTestServer(t)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.clients["cl-del"] = &models.OAuthClient{
		ClientID:  "cl-del",
		CreatedAt: time.Now(),
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/internal/oauth/clients/cl-del", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, store.clients, "cl-del")
}

// --- Code endpoint tests ---

func TestInternalOAuth_SaveCode(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"code":                  "auth-code-001",
		"client_id":             "cl-1",
		"user_id":               "user-1",
		"redirect_uri":          "http://localhost/cb",
		"code_challenge":        "challenge",
		"code_challenge_method": "S256",
		"scope":                 "vire",
		"expires_at":            time.Now().Add(5 * time.Minute),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/codes", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestInternalOAuth_SaveCode_MissingFields(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"code": "auth-code-002",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/codes", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInternalOAuth_GetCode(t *testing.T) {
	srv := newOAuthTestServer(t)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.codes["code-get"] = &models.OAuthCode{
		Code:     "code-get",
		ClientID: "cl-1",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/codes/code-get", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.OAuthCode
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "code-get", resp.Code)
}

func TestInternalOAuth_GetCode_NotFound(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/codes/missing", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInternalOAuth_MarkCodeUsed(t *testing.T) {
	srv := newOAuthTestServer(t)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	store.codes["code-mark"] = &models.OAuthCode{
		Code:     "code-mark",
		ClientID: "cl-1",
		Used:     false,
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/internal/oauth/codes/code-mark/used", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, store.codes["code-mark"].Used)
}

// --- Token endpoint tests ---

func TestInternalOAuth_SaveToken(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"token":      "my-plaintext-refresh-token",
		"client_id":  "cl-tok",
		"user_id":    "user-tok",
		"scope":      "vire",
		"expires_at": time.Now().Add(24 * time.Hour),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/tokens", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Verify token was stored with hash (not plaintext)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	hash := hashRefreshToken("my-plaintext-refresh-token")
	assert.Contains(t, store.tokens, hash)
	assert.Equal(t, "cl-tok", store.tokens[hash].ClientID)
}

func TestInternalOAuth_SaveToken_MissingFields(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{
		"token": "some-token",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/tokens", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInternalOAuth_LookupToken(t *testing.T) {
	srv := newOAuthTestServer(t)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	hash := hashRefreshToken("lookup-token")
	store.tokens[hash] = &models.OAuthRefreshToken{
		TokenHash: hash,
		ClientID:  "cl-lookup",
		UserID:    "user-lookup",
	}

	body := jsonBody(t, map[string]interface{}{"token": "lookup-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/tokens/lookup", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp models.OAuthRefreshToken
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "cl-lookup", resp.ClientID)
}

func TestInternalOAuth_LookupToken_NotFound(t *testing.T) {
	srv := newOAuthTestServer(t)
	body := jsonBody(t, map[string]interface{}{"token": "nonexistent-token"})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/tokens/lookup", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInternalOAuth_RevokeToken(t *testing.T) {
	srv := newOAuthTestServer(t)
	store := srv.app.Storage.OAuthStore().(*memOAuthStore)
	hash := hashRefreshToken("revoke-me")
	store.tokens[hash] = &models.OAuthRefreshToken{
		TokenHash: hash,
		ClientID:  "cl-rev",
		Revoked:   false,
	}

	body := jsonBody(t, map[string]interface{}{"token": "revoke-me"})
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/tokens/revoke", body)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, store.tokens[hash].Revoked)
}

func TestInternalOAuth_PurgeTokens(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/internal/oauth/tokens/purge", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// --- Routing tests ---

func TestInternalOAuth_NotFound(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/internal/oauth/unknown", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInternalOAuth_SessionMethodNotAllowed(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/internal/oauth/sessions", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestInternalOAuth_ClientMethodNotAllowed(t *testing.T) {
	srv := newOAuthTestServer(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/internal/oauth/clients/x", nil)
	rec := httptest.NewRecorder()
	srv.routeInternalOAuth(rec, req)
	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
