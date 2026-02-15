package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// --- JWT helpers ---

func TestSignAndValidateJWT_RoundTrip(t *testing.T) {
	cfg := &common.AuthConfig{
		JWTSecret:   "test-secret-key",
		TokenExpiry: "1h",
	}
	user := &models.InternalUser{
		UserID: "alice",
		Email:  "alice@example.com",
	}

	token, err := signJWT(user, "email", cfg)
	if err != nil {
		t.Fatalf("signJWT failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	parsed, claims, err := validateJWT(token, []byte(cfg.JWTSecret))
	if err != nil {
		t.Fatalf("validateJWT failed: %v", err)
	}
	if !parsed.Valid {
		t.Error("expected token to be valid")
	}
	if claims["sub"] != "alice" {
		t.Errorf("expected sub=alice, got %v", claims["sub"])
	}
	if claims["email"] != "alice@example.com" {
		t.Errorf("expected email=alice@example.com, got %v", claims["email"])
	}
	if claims["provider"] != "email" {
		t.Errorf("expected provider=email, got %v", claims["provider"])
	}
	if claims["iss"] != "vire-server" {
		t.Errorf("expected iss=vire-server, got %v", claims["iss"])
	}
}

func TestValidateJWT_ExpiredToken(t *testing.T) {
	cfg := &common.AuthConfig{
		JWTSecret:   "test-secret-key",
		TokenExpiry: "-1h", // negative duration = already expired
	}
	user := &models.InternalUser{
		UserID: "alice",
		Email:  "alice@example.com",
	}

	token, err := signJWT(user, "email", cfg)
	if err != nil {
		t.Fatalf("signJWT failed: %v", err)
	}

	_, _, err = validateJWT(token, []byte(cfg.JWTSecret))
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestValidateJWT_WrongSecret(t *testing.T) {
	cfg := &common.AuthConfig{
		JWTSecret:   "correct-secret",
		TokenExpiry: "1h",
	}
	user := &models.InternalUser{
		UserID: "alice",
		Email:  "alice@example.com",
	}

	token, err := signJWT(user, "email", cfg)
	if err != nil {
		t.Fatalf("signJWT failed: %v", err)
	}

	_, _, err = validateJWT(token, []byte("wrong-secret"))
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

// --- State parameter encoding ---

func TestStateEncodeDecode_RoundTrip(t *testing.T) {
	secret := []byte("test-state-secret")
	callback := "https://portal.example.com/auth/callback"

	state, err := encodeOAuthState(callback, secret)
	if err != nil {
		t.Fatalf("encodeOAuthState failed: %v", err)
	}
	if state == "" {
		t.Fatal("expected non-empty state")
	}

	decoded, err := decodeOAuthState(state, secret)
	if err != nil {
		t.Fatalf("decodeOAuthState failed: %v", err)
	}
	if decoded != callback {
		t.Errorf("expected callback=%q, got %q", callback, decoded)
	}
}

func TestStateParameter_HMACValidation(t *testing.T) {
	secret := []byte("test-state-secret")
	callback := "https://portal.example.com/auth/callback"

	state, err := encodeOAuthState(callback, secret)
	if err != nil {
		t.Fatalf("encodeOAuthState failed: %v", err)
	}

	// Tamper with state
	tampered := state + "x"
	_, err = decodeOAuthState(tampered, secret)
	if err == nil {
		t.Error("expected error for tampered state")
	}

	// Wrong secret
	_, err = decodeOAuthState(state, []byte("wrong-secret"))
	if err == nil {
		t.Error("expected error for wrong secret")
	}
}

func TestStateParameter_Expiry(t *testing.T) {
	secret := []byte("test-state-secret")
	callback := "https://portal.example.com/auth/callback"

	// Create state with an expired timestamp by building it manually
	payload := oauthStatePayload{
		Callback: callback,
		Nonce:    "test-nonce",
		TS:       time.Now().Add(-11 * time.Minute).Unix(), // 11 minutes ago
	}
	state, err := encodeOAuthStateFromPayload(payload, secret)
	if err != nil {
		t.Fatalf("encodeOAuthStateFromPayload failed: %v", err)
	}

	_, err = decodeOAuthState(state, secret)
	if err == nil {
		t.Error("expected error for expired state")
	}
}

// --- POST /api/auth/oauth ---

func TestHandleAuthOAuth_DevProvider(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// Default config is development mode

	body := jsonBody(t, map[string]string{
		"provider": "dev",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	data := resp["data"].(map[string]interface{})
	if data["token"] == nil || data["token"] == "" {
		t.Error("expected non-empty token")
	}
	user := data["user"].(map[string]interface{})
	if user["user_id"] != "dev_user" {
		t.Errorf("expected user_id=dev_user, got %v", user["user_id"])
	}
	if user["provider"] != "dev" {
		t.Errorf("expected provider=dev, got %v", user["provider"])
	}

	// Verify user was persisted
	ctx := context.Background()
	stored, err := srv.app.Storage.InternalStore().GetUser(ctx, "dev_user")
	if err != nil {
		t.Fatalf("expected dev_user to be persisted: %v", err)
	}
	if stored.Provider != "dev" {
		t.Errorf("expected stored provider=dev, got %q", stored.Provider)
	}
}

func TestHandleAuthOAuth_DevProvider_ProductionRejects(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Environment = "production"

	body := jsonBody(t, map[string]string{
		"provider": "dev",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 in production for dev provider, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleAuthOAuth_UnknownProvider(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"provider": "unknown",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown provider, got %d", rec.Code)
	}
}

func TestHandleAuthOAuth_MethodNotAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/oauth", nil)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

// --- POST /api/auth/validate ---

func TestHandleAuthValidate_ValidToken(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "admin")

	// Sign a token for alice
	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	token, err := signJWT(user, "email", &srv.app.Config.Auth)
	if err != nil {
		t.Fatalf("signJWT failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	data := resp["data"].(map[string]interface{})
	userData := data["user"].(map[string]interface{})
	if userData["user_id"] != "alice" {
		t.Errorf("expected user_id=alice, got %v", userData["user_id"])
	}
}

func TestHandleAuthValidate_InvalidToken(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAuthValidate_MissingAuthHeader(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandleAuthValidate_UserNotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Sign a token for a user that doesn't exist in the store
	ghost := &models.InternalUser{UserID: "ghost", Email: "ghost@x.com"}
	token, _ := signJWT(ghost, "email", &srv.app.Config.Auth)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-existent user, got %d", rec.Code)
	}

	// Verify the error message is the same as for invalid tokens (no user enumeration)
	var resp ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Error != "invalid or expired token" {
		t.Errorf("expected unified error message, got %q", resp.Error)
	}
}

// --- POST /api/auth/login returns token ---

func TestHandleAuthLogin_ReturnsToken(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "correctpassword", "admin")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "correctpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	token, ok := data["token"]
	if !ok || token == nil || token == "" {
		t.Error("expected token field in login response")
	}

	// Validate the returned token
	_, claims, err := validateJWT(token.(string), []byte(srv.app.Config.Auth.JWTSecret))
	if err != nil {
		t.Fatalf("login token should be valid: %v", err)
	}
	if claims["sub"] != "alice" {
		t.Errorf("expected sub=alice, got %v", claims["sub"])
	}
	if claims["provider"] != "email" {
		t.Errorf("expected provider=email, got %v", claims["provider"])
	}
}

func TestHandleAuthLogin_FailedLogin_NoToken(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "correctpassword", "admin")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "wrongpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}

	// Should not contain a token
	if bytes.Contains(rec.Body.Bytes(), []byte("token")) {
		respBody := rec.Body.String()
		// Only fail if it's actually a data token (not part of error message)
		var resp map[string]interface{}
		json.NewDecoder(bytes.NewBufferString(respBody)).Decode(&resp)
		if data, ok := resp["data"]; ok && data != nil {
			t.Error("failed login should not return data with token")
		}
	}
}

// --- OAuth login redirects ---

func TestHandleOAuthLoginGoogle_Redirect(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = "google-client-id"

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login/google?callback=https://portal.example.com/auth", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthLoginGoogle(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("expected Location header")
	}
	if !bytes.Contains([]byte(location), []byte("accounts.google.com")) {
		t.Errorf("expected redirect to Google, got %q", location)
	}
	if !bytes.Contains([]byte(location), []byte("google-client-id")) {
		t.Errorf("expected client_id in redirect URL, got %q", location)
	}
}

func TestHandleOAuthLoginGoogle_MissingClientID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// No Google client ID configured

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login/google?callback=https://portal.example.com/auth", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthLoginGoogle(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when Google not configured, got %d", rec.Code)
	}
}

func TestHandleOAuthLoginGitHub_Redirect(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.GitHub.ClientID = "github-client-id"

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login/github?callback=https://portal.example.com/auth", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthLoginGitHub(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if location == "" {
		t.Fatal("expected Location header")
	}
	if !bytes.Contains([]byte(location), []byte("github.com/login/oauth/authorize")) {
		t.Errorf("expected redirect to GitHub, got %q", location)
	}
	if !bytes.Contains([]byte(location), []byte("github-client-id")) {
		t.Errorf("expected client_id in redirect URL, got %q", location)
	}
}

func TestHandleOAuthLoginGitHub_MissingClientID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// No GitHub client ID configured

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login/github?callback=https://portal.example.com/auth", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthLoginGitHub(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when GitHub not configured, got %d", rec.Code)
	}
}

// --- Callback URL validation ---

func TestValidateCallbackURL_ValidHTTPS(t *testing.T) {
	if err := validateCallbackURL("https://portal.example.com/auth/callback", false); err != nil {
		t.Errorf("expected valid, got error: %v", err)
	}
	if err := validateCallbackURL("https://portal.example.com/auth/callback", true); err != nil {
		t.Errorf("expected valid in production, got error: %v", err)
	}
}

func TestValidateCallbackURL_HTTPAllowedInDev(t *testing.T) {
	if err := validateCallbackURL("http://localhost:4241/auth/callback", false); err != nil {
		t.Errorf("expected http allowed in dev, got error: %v", err)
	}
}

func TestValidateCallbackURL_HTTPRejectedInProduction(t *testing.T) {
	if err := validateCallbackURL("http://portal.example.com/auth/callback", true); err == nil {
		t.Error("expected http to be rejected in production")
	}
}

func TestValidateCallbackURL_DangerousSchemes(t *testing.T) {
	dangerous := []string{
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"ftp://files.example.com",
		"",
	}
	for _, cb := range dangerous {
		if err := validateCallbackURL(cb, false); err == nil {
			t.Errorf("expected rejection for %q", cb)
		}
	}
}

func TestValidateCallbackURL_ProtocolRelative(t *testing.T) {
	if err := validateCallbackURL("//evil.com/steal", false); err == nil {
		t.Error("expected rejection for protocol-relative URL")
	}
}

func TestValidateCallbackURL_NoHost(t *testing.T) {
	if err := validateCallbackURL("https:///path", false); err == nil {
		t.Error("expected rejection for URL without host")
	}
}

func TestBuildCallbackRedirectURL_Simple(t *testing.T) {
	result, err := buildCallbackRedirectURL("https://portal.example.com/auth/callback", "my-jwt-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "https://portal.example.com/auth/callback?token=my-jwt-token" {
		t.Errorf("unexpected URL: %q", result)
	}
}

func TestBuildCallbackRedirectURL_ExistingQueryParams(t *testing.T) {
	result, err := buildCallbackRedirectURL("https://portal.example.com/auth/callback?mode=popup", "my-jwt-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should properly append token as a second query parameter
	if !bytes.Contains([]byte(result), []byte("mode=popup")) {
		t.Errorf("expected existing param preserved, got %q", result)
	}
	if !bytes.Contains([]byte(result), []byte("token=my-jwt-token")) {
		t.Errorf("expected token param, got %q", result)
	}
	// Should NOT have double "?"
	if bytes.Count([]byte(result), []byte("?")) != 1 {
		t.Errorf("expected exactly one '?' in URL, got %q", result)
	}
}

// --- Login redirect with invalid callback ---

func TestHandleOAuthLoginGoogle_InvalidCallback(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = "google-client-id"

	badCallbacks := []string{
		"javascript:alert(1)",
		"//evil.com/steal",
		"data:text/html,test",
	}
	for _, cb := range badCallbacks {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/login/google?callback="+cb, nil)
		rec := httptest.NewRecorder()
		srv.handleOAuthLoginGoogle(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for callback=%q, got %d", cb, rec.Code)
		}
	}
}

func TestHandleOAuthLoginGitHub_InvalidCallback(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.GitHub.ClientID = "github-client-id"

	badCallbacks := []string{
		"javascript:alert(1)",
		"//evil.com/steal",
		"data:text/html,test",
	}
	for _, cb := range badCallbacks {
		req := httptest.NewRequest(http.MethodGet, "/api/auth/login/github?callback="+cb, nil)
		rec := httptest.NewRecorder()
		srv.handleOAuthLoginGitHub(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for callback=%q, got %d", cb, rec.Code)
		}
	}
}

// --- Dev provider idempotent ---

func TestHandleAuthOAuth_DevProvider_Idempotent(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// First call creates the user
	body := jsonBody(t, map[string]string{"provider": "dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first call: expected 200, got %d", rec.Code)
	}

	// Second call should also succeed (idempotent)
	body = jsonBody(t, map[string]string{"provider": "dev"})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec = httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("second call: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
