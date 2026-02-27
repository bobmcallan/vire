package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Internal OAuth API Helpers ---

// internalOAuthSession holds the response from a session create/get call.
type internalOAuthSession struct {
	SessionID     string    `json:"session_id"`
	ClientID      string    `json:"client_id"`
	RedirectURI   string    `json:"redirect_uri"`
	State         string    `json:"state"`
	CodeChallenge string    `json:"code_challenge"`
	CodeMethod    string    `json:"code_method"`
	Scope         string    `json:"scope"`
	UserID        string    `json:"user_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// internalOAuthClient holds the response from an internal client save/get call.
type internalOAuthClientResponse struct {
	ClientID                string    `json:"client_id"`
	ClientName              string    `json:"client_name"`
	RedirectURIs            []string  `json:"redirect_uris"`
	GrantTypes              []string  `json:"grant_types,omitempty"`
	ResponseTypes           []string  `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string    `json:"token_endpoint_auth_method,omitempty"`
	CreatedAt               time.Time `json:"created_at"`
}

// internalOAuthCode holds the response from a code get call.
type internalOAuthCode struct {
	Code                string    `json:"code"`
	ClientID            string    `json:"client_id"`
	UserID              string    `json:"user_id"`
	RedirectURI         string    `json:"redirect_uri"`
	CodeChallenge       string    `json:"code_challenge"`
	CodeChallengeMethod string    `json:"code_challenge_method"`
	Scope               string    `json:"scope"`
	ExpiresAt           time.Time `json:"expires_at"`
	Used                bool      `json:"used"`
	CreatedAt           time.Time `json:"created_at"`
}

// createInternalSession POSTs to /api/internal/oauth/sessions and returns status + body.
func createInternalSession(t *testing.T, env *common.Env, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/sessions", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// getInternalSession GETs /api/internal/oauth/sessions/{id} and returns status + body.
func getInternalSession(t *testing.T, env *common.Env, sessionID string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPGet("/api/internal/oauth/sessions/" + sessionID)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// patchInternalSession PATCHes /api/internal/oauth/sessions/{id} and returns status + body.
func patchInternalSession(t *testing.T, env *common.Env, sessionID string, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPatch, "/api/internal/oauth/sessions/"+sessionID, body, nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// deleteInternalSession DELETEs /api/internal/oauth/sessions/{id} and returns status.
func deleteInternalSession(t *testing.T, env *common.Env, sessionID string) int {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodDelete, "/api/internal/oauth/sessions/"+sessionID, nil, nil)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// saveInternalClient POSTs to /api/internal/oauth/clients and returns status + body.
func saveInternalClient(t *testing.T, env *common.Env, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/clients", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// getInternalClient GETs /api/internal/oauth/clients/{id} and returns status + body.
func getInternalClient(t *testing.T, env *common.Env, clientID string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPGet("/api/internal/oauth/clients/" + clientID)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// deleteInternalClient DELETEs /api/internal/oauth/clients/{id} and returns status.
func deleteInternalClient(t *testing.T, env *common.Env, clientID string) int {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodDelete, "/api/internal/oauth/clients/"+clientID, nil, nil)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// saveInternalCode POSTs to /api/internal/oauth/codes and returns status + body.
func saveInternalCode(t *testing.T, env *common.Env, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/codes", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// getInternalCode GETs /api/internal/oauth/codes/{code} and returns status + body.
func getInternalCode(t *testing.T, env *common.Env, code string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPGet("/api/internal/oauth/codes/" + code)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// markCodeUsed PATCHes /api/internal/oauth/codes/{code}/used and returns status.
func markCodeUsed(t *testing.T, env *common.Env, code string) int {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPatch, "/api/internal/oauth/codes/"+code+"/used", nil, nil)
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// saveInternalToken POSTs to /api/internal/oauth/tokens and returns status + body.
func saveInternalToken(t *testing.T, env *common.Env, body map[string]interface{}) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/tokens", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// lookupInternalToken POSTs to /api/internal/oauth/tokens/lookup and returns status + body.
func lookupInternalToken(t *testing.T, env *common.Env, token string) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/tokens/lookup", map[string]interface{}{
		"token": token,
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// revokeInternalToken POSTs to /api/internal/oauth/tokens/revoke and returns status.
func revokeInternalToken(t *testing.T, env *common.Env, token string) int {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/tokens/revoke", map[string]interface{}{
		"token": token,
	})
	require.NoError(t, err)
	resp.Body.Close()
	return resp.StatusCode
}

// purgeInternalTokens POSTs to /api/internal/oauth/tokens/purge and returns status + body.
func purgeInternalTokens(t *testing.T, env *common.Env) (int, map[string]interface{}) {
	t.Helper()
	resp, err := env.HTTPPost("/api/internal/oauth/tokens/purge", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if len(raw) == 0 {
		return resp.StatusCode, nil
	}
	var result map[string]interface{}
	_ = json.Unmarshal(raw, &result)
	return resp.StatusCode, result
}

// --- 1. Session Lifecycle ---

func TestInternalOAuth_SessionLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	sessionID := fmt.Sprintf("sess-%d", time.Now().UnixNano())

	// Step 1: Create session
	createBody := map[string]interface{}{
		"session_id":     sessionID,
		"client_id":      "client-lifecycle-test",
		"redirect_uri":   "http://localhost:3000/callback",
		"state":          "state-abc123",
		"code_challenge": "challenge-xyz",
		"code_method":    "S256",
		"scope":          "vire",
	}
	status, result := createInternalSession(t, env, createBody)
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_create_session", string(raw))
	assert.Equal(t, 201, status, "create session should return 201")

	// Step 2: Get session by ID — verify fields
	status, result = getInternalSession(t, env, sessionID)
	raw, _ = json.Marshal(result)
	guard.SaveResult("02_get_session", string(raw))
	require.Equal(t, 200, status, "get session should return 200")
	assert.Equal(t, sessionID, result["session_id"], "session_id should match")
	assert.Equal(t, "client-lifecycle-test", result["client_id"], "client_id should match")
	assert.Equal(t, "http://localhost:3000/callback", result["redirect_uri"], "redirect_uri should match")
	assert.Equal(t, "state-abc123", result["state"], "state should match")
	assert.Equal(t, "challenge-xyz", result["code_challenge"], "code_challenge should match")
	assert.Equal(t, "S256", result["code_method"], "code_method should match")
	assert.Equal(t, "vire", result["scope"], "scope should match")
	assert.Empty(t, result["user_id"], "user_id should be empty initially")
	assert.NotEmpty(t, result["created_at"], "created_at should be set")

	// Step 3: Update session — set user_id after login
	status, result = patchInternalSession(t, env, sessionID, map[string]interface{}{
		"user_id": "user-after-login",
	})
	raw, _ = json.Marshal(result)
	guard.SaveResult("03_update_session", string(raw))
	assert.Equal(t, 200, status, "patch session should return 200")

	// Step 4: Get session again — verify user_id is now set
	status, result = getInternalSession(t, env, sessionID)
	raw, _ = json.Marshal(result)
	guard.SaveResult("04_get_session_after_update", string(raw))
	require.Equal(t, 200, status, "get session after update should return 200")
	assert.Equal(t, "user-after-login", result["user_id"], "user_id should be updated")

	// Step 5: Delete session
	status = deleteInternalSession(t, env, sessionID)
	guard.SaveResult("05_delete_session", fmt.Sprintf("status=%d", status))
	assert.Equal(t, 200, status, "delete session should return 200")

	// Step 6: Get deleted session — should return 404
	status, result = getInternalSession(t, env, sessionID)
	raw, _ = json.Marshal(result)
	guard.SaveResult("06_get_deleted_session", string(raw))
	assert.Equal(t, 404, status, "get after delete should return 404")
}

// --- 2. Session By Client ID ---

func TestInternalOAuth_SessionByClientID(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	clientID := fmt.Sprintf("client-byclient-%d", time.Now().UnixNano())
	sessionID1 := fmt.Sprintf("sess-byclient-1-%d", time.Now().UnixNano())
	sessionID2 := fmt.Sprintf("sess-byclient-2-%d", time.Now().UnixNano())

	// Create first session for this client
	status, _ := createInternalSession(t, env, map[string]interface{}{
		"session_id":     sessionID1,
		"client_id":      clientID,
		"redirect_uri":   "http://localhost:3000/callback",
		"state":          "state-first",
		"code_challenge": "challenge-first",
		"code_method":    "S256",
		"scope":          "vire",
	})
	require.Equal(t, 201, status, "first session create should succeed")

	// Create second (later) session for same client
	status, _ = createInternalSession(t, env, map[string]interface{}{
		"session_id":     sessionID2,
		"client_id":      clientID,
		"redirect_uri":   "http://localhost:3000/callback",
		"state":          "state-second",
		"code_challenge": "challenge-second",
		"code_method":    "S256",
		"scope":          "vire",
	})
	require.Equal(t, 201, status, "second session create should succeed")

	// Get session by client_id — should return latest non-expired
	resp, err := env.HTTPGet("/api/internal/oauth/sessions?client_id=" + clientID)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_get_by_client_id", string(raw))

	require.Equal(t, 200, resp.StatusCode, "get by client_id should return 200")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &result))
	// The result should be a valid session for the given client
	assert.Equal(t, clientID, result["client_id"], "returned session should match client_id")
	// session_id should be one of the two we created
	returnedSessionID, _ := result["session_id"].(string)
	assert.True(t,
		returnedSessionID == sessionID1 || returnedSessionID == sessionID2,
		"returned session_id should be one we created, got %s", returnedSessionID)
}

// --- 3. Session Expiry Fields ---

func TestInternalOAuth_SessionCreatedAtIsSet(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	sessionID := fmt.Sprintf("sess-created-%d", time.Now().UnixNano())
	before := time.Now().UTC()

	status, _ := createInternalSession(t, env, map[string]interface{}{
		"session_id":     sessionID,
		"client_id":      "client-expiry-test",
		"redirect_uri":   "http://localhost:3000/callback",
		"state":          "state-expiry",
		"code_challenge": "challenge-expiry",
		"code_method":    "S256",
		"scope":          "vire",
	})
	require.Equal(t, 201, status)

	status, result := getInternalSession(t, env, sessionID)
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_session_created_at", string(raw))
	require.Equal(t, 200, status)

	createdAtStr, _ := result["created_at"].(string)
	require.NotEmpty(t, createdAtStr, "created_at must be present")

	createdAt, err := time.Parse(time.RFC3339Nano, createdAtStr)
	if err != nil {
		// Try simpler format
		createdAt, err = time.Parse(time.RFC3339, createdAtStr)
	}
	require.NoError(t, err, "created_at must be a parseable time: %s", createdAtStr)
	assert.True(t, createdAt.After(before.Add(-1*time.Second)), "created_at should be recent")
}

// --- 4. Client Lifecycle ---

func TestInternalOAuth_ClientLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	clientID := fmt.Sprintf("int-client-%d", time.Now().UnixNano())

	// Step 1: Save client
	saveBody := map[string]interface{}{
		"client_id":                  clientID,
		"client_name":                "Internal Test Client",
		"redirect_uris":              []string{"http://localhost:3000/callback"},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"token_endpoint_auth_method": "client_secret_basic",
	}
	status, result := saveInternalClient(t, env, saveBody)
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_save_client", string(raw))
	assert.Equal(t, 201, status, "save client should return 201 Created")

	// Step 2: Get client — verify all fields
	status, result = getInternalClient(t, env, clientID)
	raw, _ = json.Marshal(result)
	guard.SaveResult("02_get_client", string(raw))
	require.Equal(t, 200, status, "get client should return 200")
	assert.Equal(t, clientID, result["client_id"], "client_id should match")
	assert.Equal(t, "Internal Test Client", result["client_name"], "client_name should match")

	redirectURIs, ok := result["redirect_uris"].([]interface{})
	assert.True(t, ok, "redirect_uris should be an array")
	assert.Contains(t, redirectURIs, "http://localhost:3000/callback")

	grantTypes, ok := result["grant_types"].([]interface{})
	assert.True(t, ok, "grant_types should be an array")
	assert.Contains(t, grantTypes, "authorization_code")
	assert.Contains(t, grantTypes, "refresh_token")

	responseTypes, ok := result["response_types"].([]interface{})
	assert.True(t, ok, "response_types should be an array")
	assert.Contains(t, responseTypes, "code")

	assert.Equal(t, "client_secret_basic", result["token_endpoint_auth_method"])

	// Step 3: Delete client
	status = deleteInternalClient(t, env, clientID)
	guard.SaveResult("03_delete_client", fmt.Sprintf("status=%d", status))
	assert.Equal(t, 200, status, "delete client should return 200")

	// Step 4: Get deleted client — should return 404
	status, result = getInternalClient(t, env, clientID)
	raw, _ = json.Marshal(result)
	guard.SaveResult("04_get_deleted_client", string(raw))
	assert.Equal(t, 404, status, "get after delete should return 404")
}

// --- 5. Code Lifecycle ---

func TestInternalOAuth_CodeLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	code := fmt.Sprintf("authcode-%d", time.Now().UnixNano())

	// Step 1: Save auth code
	saveBody := map[string]interface{}{
		"code":                  code,
		"client_id":             "client-code-test",
		"user_id":               "user-code-test",
		"redirect_uri":          "http://localhost:3000/callback",
		"code_challenge":        "challenge-for-code",
		"code_challenge_method": "S256",
		"scope":                 "vire",
		"expires_at":            time.Now().Add(5 * time.Minute).Format(time.RFC3339),
	}
	status, result := saveInternalCode(t, env, saveBody)
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_save_code", string(raw))
	assert.Equal(t, 201, status, "save code should return 201")

	// Step 2: Get code — verify fields and used=false
	status, result = getInternalCode(t, env, code)
	raw, _ = json.Marshal(result)
	guard.SaveResult("02_get_code", string(raw))
	require.Equal(t, 200, status, "get code should return 200")
	assert.Equal(t, code, result["code"], "code should match")
	assert.Equal(t, "client-code-test", result["client_id"])
	assert.Equal(t, "user-code-test", result["user_id"])
	assert.Equal(t, false, result["used"], "code should not be used initially")

	// Step 3: Mark code as used
	status = markCodeUsed(t, env, code)
	guard.SaveResult("03_mark_used", fmt.Sprintf("status=%d", status))
	assert.Equal(t, 200, status, "mark code used should return 200")

	// Step 4: Get code — verify used=true
	status, result = getInternalCode(t, env, code)
	raw, _ = json.Marshal(result)
	guard.SaveResult("04_get_code_after_used", string(raw))
	require.Equal(t, 200, status, "get code after marking used should return 200")
	assert.Equal(t, true, result["used"], "code should be marked as used")
}

// --- 6. Token Lifecycle ---

func TestInternalOAuth_TokenLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	plaintextToken := fmt.Sprintf("plaintext-refresh-token-%d", time.Now().UnixNano())

	// Step 1: Save token (plaintext — server hashes internally)
	saveBody := map[string]interface{}{
		"token":      plaintextToken,
		"client_id":  "client-token-test",
		"user_id":    "user-token-test",
		"scope":      "vire",
		"expires_at": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}
	status, result := saveInternalToken(t, env, saveBody)
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_save_token", string(raw))
	assert.Equal(t, 201, status, "save token should return 201")

	// Step 2: Lookup token by plaintext — should find it
	status, result = lookupInternalToken(t, env, plaintextToken)
	raw, _ = json.Marshal(result)
	guard.SaveResult("02_lookup_token", string(raw))
	require.Equal(t, 200, status, "lookup token should return 200")
	assert.Equal(t, "client-token-test", result["client_id"])
	assert.Equal(t, "user-token-test", result["user_id"])
	// TokenHash must NOT be exposed in API response
	assert.Empty(t, result["token_hash"], "token_hash must not be exposed in API response")

	// Step 3: Revoke token by plaintext
	status = revokeInternalToken(t, env, plaintextToken)
	guard.SaveResult("03_revoke_token", fmt.Sprintf("status=%d", status))
	assert.Equal(t, 200, status, "revoke token should return 200")

	// Step 4: Lookup revoked token — should return 404 or indicate revoked
	status, result = lookupInternalToken(t, env, plaintextToken)
	raw, _ = json.Marshal(result)
	guard.SaveResult("04_lookup_revoked_token", string(raw))
	// Either 404 (not found) or 200 with revoked=true
	if status == 200 {
		revoked, _ := result["revoked"].(bool)
		assert.True(t, revoked, "revoked token should show revoked=true")
	} else {
		assert.Equal(t, 404, status, "revoked token should return 404")
	}
}

// --- 7. Token Purge ---

func TestInternalOAuth_TokenPurge(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Save an already-expired token
	expiredToken := fmt.Sprintf("expired-token-%d", time.Now().UnixNano())
	saveBody := map[string]interface{}{
		"token":      expiredToken,
		"client_id":  "client-purge-test",
		"user_id":    "user-purge-test",
		"scope":      "vire",
		"expires_at": time.Now().Add(-1 * time.Hour).Format(time.RFC3339), // already expired
	}
	status, _ := saveInternalToken(t, env, saveBody)
	guard.SaveResult("01_save_expired_token", fmt.Sprintf("status=%d", status))
	require.Equal(t, 201, status, "save expired token should succeed")

	// Purge expired tokens
	status, result := purgeInternalTokens(t, env)
	raw, _ := json.Marshal(result)
	guard.SaveResult("02_purge_tokens", string(raw))
	assert.Equal(t, 200, status, "purge should return 200")

	// After purge, the expired token should not be found
	status, result = lookupInternalToken(t, env, expiredToken)
	raw, _ = json.Marshal(result)
	guard.SaveResult("03_lookup_after_purge", string(raw))
	assert.Equal(t, 404, status, "purged expired token should return 404")
}

// --- 8. Error Cases ---

func TestInternalOAuth_ErrorCases(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// 8a: Get non-existent session
	status, result := getInternalSession(t, env, "nonexistent-session-id")
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_get_nonexistent_session", string(raw))
	assert.Equal(t, 404, status, "get non-existent session should return 404")

	// 8b: Get non-existent client
	status, result = getInternalClient(t, env, "nonexistent-client-id")
	raw, _ = json.Marshal(result)
	guard.SaveResult("02_get_nonexistent_client", string(raw))
	assert.Equal(t, 404, status, "get non-existent client should return 404")

	// 8c: Get non-existent code
	status, result = getInternalCode(t, env, "nonexistent-auth-code")
	raw, _ = json.Marshal(result)
	guard.SaveResult("03_get_nonexistent_code", string(raw))
	assert.Equal(t, 404, status, "get non-existent code should return 404")

	// 8d: Lookup non-existent token
	status, result = lookupInternalToken(t, env, "nonexistent-plaintext-token")
	raw, _ = json.Marshal(result)
	guard.SaveResult("04_lookup_nonexistent_token", string(raw))
	assert.Equal(t, 404, status, "lookup non-existent token should return 404")

	// 8e: POST session with empty body
	resp, err := env.HTTPPost("/api/internal/oauth/sessions", map[string]interface{}{})
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, _ = io.ReadAll(resp.Body)
	guard.SaveResult("05_create_session_empty_body", string(raw))
	assert.Equal(t, 400, resp.StatusCode, "POST with empty body should return 400")

	// 8f: POST session missing required fields (session_id is required)
	status, result = createInternalSession(t, env, map[string]interface{}{
		"client_id": "some-client",
		// missing session_id, redirect_uri, state, code_challenge
	})
	raw, _ = json.Marshal(result)
	guard.SaveResult("06_create_session_missing_fields", string(raw))
	assert.Equal(t, 400, status, "POST with missing required fields should return 400")
}

// --- 9. Session Purge ---

func TestInternalOAuth_SessionPurge(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// POST /api/internal/oauth/sessions/purge
	resp, err := env.HTTPPost("/api/internal/oauth/sessions/purge", nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	guard.SaveResult("01_purge_sessions", string(raw))

	assert.Equal(t, 200, resp.StatusCode, "purge sessions should return 200")
}

// --- 10. Client Save Is Upsert ---

func TestInternalOAuth_ClientSaveUpsert(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	clientID := fmt.Sprintf("upsert-client-%d", time.Now().UnixNano())

	// Save client first time
	status, _ := saveInternalClient(t, env, map[string]interface{}{
		"client_id":     clientID,
		"client_name":   "Original Name",
		"redirect_uris": []string{"http://localhost:3000/callback"},
	})
	require.Equal(t, 201, status, "first save should succeed")

	// Save client second time with updated name (upsert)
	status, _ = saveInternalClient(t, env, map[string]interface{}{
		"client_id":     clientID,
		"client_name":   "Updated Name",
		"redirect_uris": []string{"http://localhost:3000/callback", "http://localhost:4000/callback"},
	})
	assert.Equal(t, 201, status, "upsert save should succeed")

	// Get client — should reflect updated values
	status, result := getInternalClient(t, env, clientID)
	raw, _ := json.Marshal(result)
	guard.SaveResult("01_upserted_client", string(raw))
	require.Equal(t, 200, status)
	assert.Equal(t, "Updated Name", result["client_name"], "client_name should be updated")

	redirectURIs, _ := result["redirect_uris"].([]interface{})
	assert.Len(t, redirectURIs, 2, "redirect_uris should be updated to 2 URIs")
}
