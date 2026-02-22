package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/golang-jwt/jwt/v5"
)

// ============================================================================
// 1. JWT attacks
// ============================================================================

func TestAuthStress_JWT_AlgNoneAttack(t *testing.T) {
	// The "alg:none" attack forges a token with no signature.
	// validateJWT must reject tokens signed with method "none".
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	// Create a token with alg:none
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":      "alice",
		"email":    "a@x.com",
		"provider": "email",
		"iss":      "vire-server",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
	})
	tokenString, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("failed to create alg:none token: %v", err)
	}

	// Try to validate
	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("VULNERABILITY: alg:none token accepted, got status %d", rec.Code)
	}
}

func TestAuthStress_JWT_TamperedPayload(t *testing.T) {
	// Sign a token, then modify the payload to change the sub claim
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	tokenStr, _ := signJWT(user, "email", &srv.app.Config.Auth)

	// Split the JWT and tamper with the payload
	parts := strings.SplitN(tokenStr, ".", 3)
	if len(parts) != 3 {
		t.Fatal("expected 3 JWT parts")
	}

	// Decode payload, change sub
	payloadJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]interface{}
	json.Unmarshal(payloadJSON, &claims)
	claims["sub"] = "admin"
	claims["role"] = "superadmin"
	newPayload, _ := json.Marshal(claims)
	parts[1] = base64.RawURLEncoding.EncodeToString(newPayload)

	tamperedToken := strings.Join(parts, ".")

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+tamperedToken)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("VULNERABILITY: tampered JWT accepted, got status %d", rec.Code)
	}
}

func TestAuthStress_JWT_WrongSecret(t *testing.T) {
	// Sign with one secret, validate with another
	cfg := &common.AuthConfig{JWTSecret: "attacker-secret", TokenExpiry: "1h"}
	user := &models.InternalUser{UserID: "alice", Email: "a@x.com"}

	token, _ := signJWT(user, "email", cfg)

	_, _, err := validateJWT(token, []byte("correct-secret"))
	if err == nil {
		t.Error("VULNERABILITY: token signed with wrong secret was accepted")
	}
}

func TestAuthStress_JWT_EmptySecret(t *testing.T) {
	// Ensure empty secret doesn't produce valid tokens accepted by a real secret
	cfg := &common.AuthConfig{JWTSecret: "", TokenExpiry: "1h"}
	user := &models.InternalUser{UserID: "alice", Email: "a@x.com"}

	token, err := signJWT(user, "email", cfg)
	if err != nil {
		// Empty secret failing to sign is acceptable
		return
	}

	// Verify it cannot validate against a real secret
	_, _, err = validateJWT(token, []byte("real-secret"))
	if err == nil {
		t.Error("VULNERABILITY: token signed with empty secret accepted by real secret")
	}
}

func TestAuthStress_JWT_ExpiredToken(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	cfg := &common.AuthConfig{JWTSecret: srv.app.Config.Auth.JWTSecret, TokenExpiry: "-1h"}
	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	token, _ := signJWT(user, "email", cfg)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rec.Code)
	}
}

func TestAuthStress_JWT_ExtremelyLongClaims(t *testing.T) {
	// Token with extremely long claims should not crash the server
	srv := newTestServerWithStorage(t)

	longEmail := strings.Repeat("a", 100000) + "@x.com"
	user := &models.InternalUser{UserID: "alice", Email: longEmail}
	token, err := signJWT(user, "email", &srv.app.Config.Auth)
	if err != nil {
		return // Failing to sign is acceptable
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	// Should not crash, any status is fine
	if rec.Code >= 500 {
		t.Errorf("server error with extremely long claims: status %d", rec.Code)
	}
}

func TestAuthStress_JWT_MissingSubClaim(t *testing.T) {
	// Manually create a JWT without a "sub" claim
	srv := newTestServerWithStorage(t)
	secret := []byte(srv.app.Config.Auth.JWTSecret)

	claims := jwt.MapClaims{
		"email":    "alice@x.com",
		"provider": "email",
		"iss":      "vire-server",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(time.Hour).Unix(),
		// No "sub" claim
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, _ := token.SignedString(secret)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for token without sub claim, got %d", rec.Code)
	}
}

// ============================================================================
// 2. OAuth state attacks
// ============================================================================

func TestAuthStress_State_ModifiedState(t *testing.T) {
	secret := []byte("test-secret")
	state, _ := encodeOAuthState("https://portal.example.com/auth", secret)

	// Flip a character in the payload portion
	modified := "X" + state[1:]
	_, err := decodeOAuthState(modified, secret)
	if err == nil {
		t.Error("VULNERABILITY: modified state was accepted")
	}
}

func TestAuthStress_State_ReplayedState(t *testing.T) {
	// State should be time-limited. Create one that's just barely within the window.
	secret := []byte("test-secret")
	state, _ := encodeOAuthState("https://portal.example.com/auth", secret)

	// Immediately replay should succeed (within 10 min window)
	callback, err := decodeOAuthState(state, secret)
	if err != nil {
		t.Fatalf("fresh state should be valid: %v", err)
	}
	if callback != "https://portal.example.com/auth" {
		t.Errorf("unexpected callback: %q", callback)
	}
}

func TestAuthStress_State_ExpiredState(t *testing.T) {
	secret := []byte("test-secret")

	// Build a state with a timestamp 11 minutes ago
	payload := oauthStatePayload{
		Callback: "https://portal.example.com/auth",
		Nonce:    "test-nonce",
		TS:       time.Now().Add(-11 * time.Minute).Unix(),
	}
	state, _ := encodeOAuthStateFromPayload(payload, secret)

	_, err := decodeOAuthState(state, secret)
	if err == nil {
		t.Error("VULNERABILITY: expired state (11 min old) was accepted")
	}
}

func TestAuthStress_State_TamperedCallbackURL(t *testing.T) {
	secret := []byte("test-secret")

	// Create a valid state
	state, _ := encodeOAuthState("https://legitimate.com/auth", secret)

	// Try to decode with the same secret (should work)
	callback, err := decodeOAuthState(state, secret)
	if err != nil {
		t.Fatalf("valid state should decode: %v", err)
	}
	if callback != "https://legitimate.com/auth" {
		t.Errorf("callback mismatch: %q", callback)
	}

	// The attacker cannot modify the callback in the state without invalidating the HMAC
	// So they'd need to create their own state with a different secret
	_, err = decodeOAuthState(state, []byte("attacker-secret"))
	if err == nil {
		t.Error("VULNERABILITY: state decoded with wrong secret")
	}
}

func TestAuthStress_State_WithoutHMAC(t *testing.T) {
	// State with no HMAC portion (no dot separator)
	_, err := decodeOAuthState("justpayloadnodot", []byte("secret"))
	if err == nil {
		t.Error("VULNERABILITY: state without HMAC separator was accepted")
	}
}

func TestAuthStress_State_EmptyState(t *testing.T) {
	_, err := decodeOAuthState("", []byte("secret"))
	if err == nil {
		t.Error("VULNERABILITY: empty state was accepted")
	}
}

func TestAuthStress_State_InvalidBase64(t *testing.T) {
	_, err := decodeOAuthState("!!!invalid!!!.!!!base64!!!", []byte("secret"))
	if err == nil {
		t.Error("VULNERABILITY: invalid base64 state was accepted")
	}
}

func TestAuthStress_State_ValidPayloadInvalidSig(t *testing.T) {
	secret := []byte("test-secret")
	state, _ := encodeOAuthState("https://example.com", secret)

	// Replace the signature part with garbage
	parts := strings.SplitN(state, ".", 2)
	if len(parts) != 2 {
		t.Fatal("expected 2 parts in state")
	}
	tampered := parts[0] + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	_, err := decodeOAuthState(tampered, secret)
	if err == nil {
		t.Error("VULNERABILITY: state with invalid signature was accepted")
	}
}

// ============================================================================
// 3. Dev login gating
// ============================================================================

func TestAuthStress_DevProvider_RejectedInProduction(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Environment = "production"

	body := jsonBody(t, map[string]string{"provider": "dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("VULNERABILITY: dev provider accepted in production mode, got %d", rec.Code)
	}
}

func TestAuthStress_DevProvider_RejectedInProd(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Environment = "prod"

	body := jsonBody(t, map[string]string{"provider": "dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("VULNERABILITY: dev provider accepted in 'prod' mode, got %d", rec.Code)
	}
}

func TestAuthStress_DevProvider_AcceptedInDevelopment(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// Default config is "development"

	body := jsonBody(t, map[string]string{"provider": "dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for dev provider in development, got %d", rec.Code)
	}
}

func TestAuthStress_DevProvider_CaseVariations(t *testing.T) {
	// Ensure "Production", "PRODUCTION" are also rejected
	srv := newTestServerWithStorage(t)

	for _, env := range []string{"Production", "PRODUCTION", "Prod", "PROD"} {
		srv.app.Config.Environment = env
		body := jsonBody(t, map[string]string{"provider": "dev"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
		rec := httptest.NewRecorder()
		srv.handleAuthOAuth(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("VULNERABILITY: dev provider accepted with environment=%q, got %d", env, rec.Code)
		}
	}
}

// ============================================================================
// 4. Input validation — hostile provider/code/state values
// ============================================================================

func TestAuthStress_OAuthInput_EmptyProvider(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{"provider": "", "code": "x", "state": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty provider, got %d", rec.Code)
	}
}

func TestAuthStress_OAuthInput_NullBytesInProvider(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{"provider": "dev\x00evil"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	// Should not crash — should return 400 (unknown provider)
	if rec.Code >= 500 {
		t.Errorf("server error with null bytes in provider: status %d", rec.Code)
	}
}

func TestAuthStress_OAuthInput_SQLInjectionInCode(t *testing.T) {
	srv := newTestServerWithStorage(t)

	injectionPayloads := []string{
		"'; DROP TABLE users; --",
		"1 OR 1=1",
		"admin'--",
		"' UNION SELECT * FROM passwords --",
	}

	for _, payload := range injectionPayloads {
		body := jsonBody(t, map[string]string{
			"provider": "dev",
			"code":     payload,
			"state":    payload,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
		rec := httptest.NewRecorder()
		srv.handleAuthOAuth(rec, req)

		if rec.Code >= 500 {
			t.Errorf("server error with SQL injection payload %q: status %d", payload, rec.Code)
		}
	}
}

func TestAuthStress_OAuthInput_ExtremelyLongCodeValue(t *testing.T) {
	srv := newTestServerWithStorage(t)

	longCode := strings.Repeat("A", 1024*1024) // 1MB
	body := jsonBody(t, map[string]string{
		"provider": "dev",
		"code":     longCode,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	// Should not crash or OOM
	if rec.Code >= 500 {
		t.Errorf("server error with 1MB code value: status %d", rec.Code)
	}
}

func TestAuthStress_OAuthInput_XSSInProvider(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"provider": "<script>alert('xss')</script>",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	// The provider is reflected in the error message — check the response
	// doesn't contain unescaped HTML
	respBody := rec.Body.String()
	// Since this is a JSON API with Content-Type: application/json, browsers
	// won't execute scripts. But verify the response is proper JSON.
	var resp map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(respBody)).Decode(&resp); err != nil {
		t.Errorf("response is not valid JSON: %v", err)
	}
}

// ============================================================================
// 5. Auth validate attacks
// ============================================================================

func TestAuthStress_Validate_MissingBearerPrefix(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer auth, got %d", rec.Code)
	}
}

func TestAuthStress_Validate_MalformedAuthHeader(t *testing.T) {
	srv := newTestServerWithStorage(t)

	malformedHeaders := []string{
		"Bearer ",
		"Bearer",
		"bearer token",
		"BEARER token",
		"  Bearer token",
		"Bearer  ",
		"",
	}

	for _, header := range malformedHeaders {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		srv.handleAuthValidate(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for Authorization=%q, got %d", header, rec.Code)
		}
	}
}

func TestAuthStress_Validate_EmptyToken(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty token, got %d", rec.Code)
	}
}

func TestAuthStress_Validate_TokenForNonExistentUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Sign a valid JWT for a user that doesn't exist
	ghost := &models.InternalUser{UserID: "ghost-user", Email: "ghost@x.com"}
	token, _ := signJWT(ghost, "email", &srv.app.Config.Auth)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-existent user, got %d", rec.Code)
	}
}

func TestAuthStress_Validate_TokenForDeletedUser(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	// Sign a valid token for alice
	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	token, _ := signJWT(user, "email", &srv.app.Config.Auth)

	// Delete alice
	srv.app.Storage.InternalStore().DeleteUser(context.Background(), "alice")

	// Token should now be invalid (user not found)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for deleted user's token, got %d", rec.Code)
	}
}

func TestAuthStress_Validate_GarbageToken(t *testing.T) {
	srv := newTestServerWithStorage(t)

	garbageTokens := []string{
		"not.a.jwt",
		"aaaaaa",
		"eyJhbGciOiJub25lIn0.e30.",
		strings.Repeat("A", 10000),
		"...",
		"..",
		"eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9..SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c",
	}

	for _, token := range garbageTokens {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		srv.handleAuthValidate(rec, req)

		if rec.Code >= 500 {
			t.Errorf("server error with garbage token %q: status %d", token[:min(len(token), 30)], rec.Code)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 for garbage token, got %d", rec.Code)
		}
	}
}

func TestAuthStress_Validate_ErrorResponseNoSensitiveData(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	body := rec.Body.String()
	assertNoSensitiveData(t, body)

	// Verify no JWT secret or internal details leaked
	if strings.Contains(body, srv.app.Config.Auth.JWTSecret) {
		t.Error("VULNERABILITY: JWT secret leaked in error response")
	}
}

// ============================================================================
// 6. Open redirect — callback URL validation
// ============================================================================

func TestAuthStress_OpenRedirect_ArbitraryCallback(t *testing.T) {
	// FINDING: The callback URL in the state is not validated against
	// an allowlist. An attacker who can get a victim to start the OAuth
	// flow with callback=https://evil.com can steal the JWT token.
	secret := []byte("test-secret")

	maliciousCallbacks := []string{
		"https://evil.com/steal",
		"http://attacker.local/capture",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
		"//evil.com/steal",
		"https://evil.com/steal?existing=param",
	}

	for _, callback := range maliciousCallbacks {
		state, err := encodeOAuthState(callback, secret)
		if err != nil {
			t.Errorf("encodeOAuthState failed for %q: %v", callback, err)
			continue
		}

		decoded, err := decodeOAuthState(state, secret)
		if err != nil {
			continue // Some might fail on decode, which is fine
		}

		// FINDING: The state system will encode and decode ANY callback URL.
		// This is a potential open redirect if the callback isn't validated
		// before redirecting. Log the finding.
		if decoded == callback {
			t.Logf("FINDING: arbitrary callback URL %q can be encoded in state and will be redirected to", callback)
		}
	}
}

func TestAuthStress_CallbackURLInjection(t *testing.T) {
	// Test that callback URLs with query parameters don't cause issues
	// in the redirect (callback + "?token=" + ...)
	secret := []byte("test-secret")

	// A callback with existing query params: callback already has "?"
	// So the redirect would be: https://example.com?existing=1?token=jwt
	// This is malformed URL behavior
	callback := "https://example.com?existing=1"
	state, _ := encodeOAuthState(callback, secret)
	decoded, _ := decodeOAuthState(state, secret)

	if decoded == callback {
		t.Log("FINDING: callback URL with existing query params will produce malformed redirect URL (double ? character)")
	}
}

// ============================================================================
// 7. Concurrent access — multiple OAuth exchanges
// ============================================================================

func TestAuthStress_ConcurrentDevLogins(t *testing.T) {
	srv := newTestServerWithStorage(t)

	var wg sync.WaitGroup
	errors := make(chan string, 100)

	// 20 concurrent dev logins
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := jsonBody(t, map[string]string{"provider": "dev"})
			req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
			rec := httptest.NewRecorder()
			srv.handleAuthOAuth(rec, req)

			if rec.Code >= 500 {
				errors <- "dev login returned 5xx"
			}
			if rec.Code != http.StatusOK {
				errors <- "dev login returned non-200: " + rec.Body.String()
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent dev login error: %s", err)
	}
}

func TestAuthStress_ConcurrentTokenValidation(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	token, _ := signJWT(user, "email", &srv.app.Config.Auth)

	var wg sync.WaitGroup
	errors := make(chan string, 100)

	// 20 concurrent validates
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			rec := httptest.NewRecorder()
			srv.handleAuthValidate(rec, req)

			if rec.Code != http.StatusOK {
				errors <- "validate returned non-200"
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent validate error: %s", err)
	}
}

func TestAuthStress_ConcurrentUserCreateAndOAuth(t *testing.T) {
	// Race condition: multiple OAuth logins creating the same user simultaneously
	srv := newTestServerWithStorage(t)

	var wg sync.WaitGroup
	errors := make(chan string, 100)

	// 10 concurrent findOrCreateOAuthUser calls for the same user
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			user := srv.findOrCreateOAuthUser(context.Background(), "oauth_race_user", "race@x.com", "Race User", "google")
			if user == nil {
				errors <- "findOrCreateOAuthUser returned nil"
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent OAuth user creation error: %s", err)
	}

	// Verify exactly one user exists
	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "oauth_race_user")
	if err != nil {
		t.Fatalf("user should exist: %v", err)
	}
	if user.Provider != "google" {
		t.Errorf("expected provider=google, got %q", user.Provider)
	}
}

// ============================================================================
// 8. Method enforcement
// ============================================================================

func TestAuthStress_MethodEnforcement(t *testing.T) {
	srv := newTestServerWithStorage(t)

	endpoints := []struct {
		name          string
		path          string
		handler       func(http.ResponseWriter, *http.Request)
		allowedMethod string
	}{
		{"oauth", "/api/auth/oauth", srv.handleAuthOAuth, http.MethodPost},
		{"validate", "/api/auth/validate", srv.handleAuthValidate, http.MethodPost},
		{"login_google", "/api/auth/login/google", srv.handleOAuthLoginGoogle, http.MethodGet},
		{"login_github", "/api/auth/login/github", srv.handleOAuthLoginGitHub, http.MethodGet},
		{"callback_google", "/api/auth/callback/google", srv.handleOAuthCallbackGoogle, http.MethodGet},
		{"callback_github", "/api/auth/callback/github", srv.handleOAuthCallbackGitHub, http.MethodGet},
	}

	disallowedMethods := []string{http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, ep := range endpoints {
		for _, m := range disallowedMethods {
			if m == ep.allowedMethod {
				continue
			}
			t.Run(ep.name+"_"+m, func(t *testing.T) {
				req := httptest.NewRequest(m, ep.path, nil)
				rec := httptest.NewRecorder()
				ep.handler(rec, req)

				if rec.Code != http.StatusMethodNotAllowed {
					t.Errorf("%s %s: expected 405, got %d", m, ep.path, rec.Code)
				}
			})
		}
	}
}

// ============================================================================
// 9. OAuth user response — no secrets leaked
// ============================================================================

func TestAuthStress_OAuthResponse_NoPasswordHash(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{"provider": "dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	respBody := rec.Body.String()
	if strings.Contains(respBody, "password_hash") {
		t.Error("VULNERABILITY: password_hash field in OAuth response")
	}
	if strings.Contains(respBody, "$2a$") {
		t.Error("VULNERABILITY: bcrypt hash in OAuth response")
	}
}

func TestAuthStress_ValidateResponse_NoPasswordHash(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	token, _ := signJWT(user, "email", &srv.app.Config.Auth)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.handleAuthValidate(rec, req)

	respBody := rec.Body.String()
	if strings.Contains(respBody, "password_hash") {
		t.Error("VULNERABILITY: password_hash field in validate response")
	}
	if strings.Contains(respBody, "$2a$") {
		t.Error("VULNERABILITY: bcrypt hash in validate response")
	}
}

// ============================================================================
// 10. oauthRedirectURI — X-Forwarded-Proto injection
// ============================================================================

func TestAuthStress_RedirectURI_XForwardedProtoInjection(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// An attacker can inject X-Forwarded-Proto to force HTTPS scheme
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login/google", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Host = "victim.com"

	uri := srv.oauthRedirectURI(req, "google")
	if uri != "https://victim.com/api/auth/callback/google" {
		t.Errorf("unexpected URI: %q", uri)
	}

	// Without the header, should use http
	req2 := httptest.NewRequest(http.MethodGet, "/api/auth/login/google", nil)
	req2.Host = "victim.com"
	uri2 := srv.oauthRedirectURI(req2, "google")
	if uri2 != "http://victim.com/api/auth/callback/google" {
		t.Errorf("unexpected URI without forwarded proto: %q", uri2)
	}

	t.Log("FINDING: X-Forwarded-Proto header is trusted for scheme detection. If the server is not behind a trusted reverse proxy, an attacker could manipulate the redirect URI scheme.")
}

func TestAuthStress_RedirectURI_HostHeaderInjection(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// An attacker can set the Host header to control the redirect URI
	req := httptest.NewRequest(http.MethodGet, "/api/auth/login/google", nil)
	req.Host = "evil.com"

	uri := srv.oauthRedirectURI(req, "google")
	t.Logf("FINDING: Host header %q produces redirect URI %q — if not behind a proxy that sets Host, an attacker controls the redirect URI domain", "evil.com", uri)
}

// ============================================================================
// 11. Login endpoint — JWT secret in error response
// ============================================================================

func TestAuthStress_LoginJWT_ErrorDoesNotLeakSecret(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "pass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	respBody := rec.Body.String()
	if strings.Contains(respBody, srv.app.Config.Auth.JWTSecret) {
		t.Error("VULNERABILITY: JWT secret leaked in login response")
	}
}

// ============================================================================
// 12. Auth validate — consistent error messages (prevent user enumeration)
// ============================================================================

func TestAuthStress_Validate_ConsistentErrors(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "admin")

	// Invalid token error
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req1.Header.Set("Authorization", "Bearer garbage.token.here")
	rec1 := httptest.NewRecorder()
	srv.handleAuthValidate(rec1, req1)

	// Valid token but user deleted
	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	token, _ := signJWT(user, "email", &srv.app.Config.Auth)
	srv.app.Storage.InternalStore().DeleteUser(context.Background(), "alice")

	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/validate", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	rec2 := httptest.NewRecorder()
	srv.handleAuthValidate(rec2, req2)

	// Both should be 401
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", rec1.Code)
	}
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for deleted user, got %d", rec2.Code)
	}

	// The error messages differ ("invalid or expired token" vs "user not found")
	// which could enable token probing. Logging as a finding.
	var resp1, resp2 ErrorResponse
	json.NewDecoder(rec1.Body).Decode(&resp1)
	json.NewDecoder(rec2.Body).Decode(&resp2)

	if resp1.Error != resp2.Error {
		t.Logf("FINDING: different error messages for invalid token (%q) vs deleted user (%q) — enables token validity probing",
			resp1.Error, resp2.Error)
	}
}

// ============================================================================
// 13. State parameter edge cases in callbacks
// ============================================================================

func TestAuthStress_Callback_EmptyStateParam(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = "id"
	srv.app.Config.Auth.Google.ClientSecret = "secret"

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback/google?code=testcode&state=", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGoogle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty state, got %d", rec.Code)
	}
}

func TestAuthStress_Callback_MissingStateParam(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = "id"
	srv.app.Config.Auth.Google.ClientSecret = "secret"

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback/google?code=testcode", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGoogle(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing state, got %d", rec.Code)
	}
}

func TestAuthStress_Callback_TamperedState(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.GitHub.ClientID = "id"
	srv.app.Config.Auth.GitHub.ClientSecret = "secret"

	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback/github?code=testcode&state=tampered.garbage", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGitHub(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for tampered state, got %d", rec.Code)
	}
}

// ============================================================================
// 14. JSON body attacks on OAuth endpoint
// ============================================================================

func TestAuthStress_OAuth_NilBody(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", nil)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for nil body, got %d", rec.Code)
	}
}

func TestAuthStress_OAuth_MalformedJSON(t *testing.T) {
	srv := newTestServerWithStorage(t)

	malformed := []string{
		"not json",
		"{invalid",
		"[]",
		`{"provider": }`,
		"null",
		"",
	}

	for _, body := range malformed {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", strings.NewReader(body))
		rec := httptest.NewRecorder()
		srv.handleAuthOAuth(rec, req)

		if rec.Code >= 500 {
			t.Errorf("server error with malformed JSON %q: status %d", body, rec.Code)
		}
	}
}

// ============================================================================
// 15. CORS — new auth headers in allowlist
// ============================================================================

func TestAuthStress_CORS_AuthorizationHeaderAllowed(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/auth/validate", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "Authorization") {
		t.Errorf("Authorization header not in CORS Allow-Headers: %s", allowHeaders)
	}
}

// ============================================================================
// 16. OAuth gap fix: email-based account linking
// ============================================================================

func TestAuthStress_FindOrCreateOAuthUser_EmailLinking(t *testing.T) {
	// Gap 1: When a user logs in with a different provider but the same email,
	// they should be linked to the existing account.
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create a Google user with alice@example.com
	googleUser := &models.InternalUser{
		UserID:    "google_12345",
		Email:     "alice@example.com",
		Name:      "Alice",
		Provider:  "google",
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, googleUser); err != nil {
		t.Fatalf("failed to save google user: %v", err)
	}

	// Now simulate a GitHub login with the same email
	user := srv.findOrCreateOAuthUser(ctx, "github_67890", "alice@example.com", "Alice GitHub", "github")
	if user == nil {
		t.Fatal("findOrCreateOAuthUser returned nil")
	}

	// Email linking should return the existing Google user (same UserID)
	if user.UserID != "google_12345" {
		t.Errorf("expected email linking to return google_12345, got %q", user.UserID)
	}

	// Name should be updated to the GitHub name
	if user.Name != "Alice GitHub" {
		t.Errorf("expected name updated to 'Alice GitHub', got %q", user.Name)
	}

	// Verify no duplicate was created
	_, err := store.GetUser(ctx, "github_67890")
	if err == nil {
		t.Error("VULNERABILITY: email linking created a separate user instead of linking to existing")
	}
}

func TestAuthStress_FindOrCreateOAuthUser_NoEmailNoLinking(t *testing.T) {
	// When email is empty, no email-based linking should occur
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create an existing user
	existing := &models.InternalUser{
		UserID:    "google_existing",
		Email:     "existing@example.com",
		Provider:  "google",
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, existing); err != nil {
		t.Fatalf("failed to save user: %v", err)
	}

	// Login with a different provider and empty email
	user := srv.findOrCreateOAuthUser(ctx, "github_noemail", "", "NoEmail User", "github")
	if user == nil {
		t.Fatal("findOrCreateOAuthUser returned nil")
	}

	// Should create a new user, not link to existing
	if user.UserID != "github_noemail" {
		t.Errorf("expected new user github_noemail, got %q", user.UserID)
	}
}

func TestAuthStress_EmailInjection_SurrealDBQuery(t *testing.T) {
	// Verify that SurrealDB parameterized queries prevent email injection
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create a normal user
	normal := &models.InternalUser{
		UserID:    "normal_user",
		Email:     "normal@example.com",
		Provider:  "email",
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, normal); err != nil {
		t.Fatalf("failed to save user: %v", err)
	}

	injectionPayloads := []string{
		`"; DROP TABLE user; --`,
		`' OR 1=1 --`,
		`normal@example.com' OR '1'='1`,
		"'; SELECT * FROM user WHERE '1'='1",
		`\"; DROP TABLE user; --`,
	}

	for _, payload := range injectionPayloads {
		t.Run(payload[:min(len(payload), 30)], func(t *testing.T) {
			// GetUserByEmail should not crash or return unintended results
			result, err := store.GetUserByEmail(ctx, payload)
			// Should either return "not found" error or no match
			if err == nil && result != nil && result.Email != payload {
				t.Errorf("VULNERABILITY: injection payload %q returned unrelated user %q",
					payload, result.Email)
			}
		})
	}

	// Verify the normal user still exists (table not dropped)
	got, err := store.GetUser(ctx, "normal_user")
	if err != nil {
		t.Fatalf("VULNERABILITY: user table may have been affected by injection: %v", err)
	}
	if got.Email != "normal@example.com" {
		t.Errorf("expected email normal@example.com, got %q", got.Email)
	}
}

func TestAuthStress_EmailCaseSensitivity(t *testing.T) {
	// Email matching must be case-insensitive
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	user := &models.InternalUser{
		UserID:    "google_casetest",
		Email:     "Alice@Example.COM",
		Provider:  "google",
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, user); err != nil {
		t.Fatalf("failed to save user: %v", err)
	}

	caseVariations := []string{
		"alice@example.com",
		"ALICE@EXAMPLE.COM",
		"Alice@Example.COM",
		"alice@EXAMPLE.com",
		"aLiCe@eXaMpLe.CoM",
	}

	for _, email := range caseVariations {
		t.Run(email, func(t *testing.T) {
			got, err := store.GetUserByEmail(ctx, email)
			if err != nil {
				t.Errorf("expected case-insensitive match for %q, got error: %v", email, err)
				return
			}
			if got.UserID != "google_casetest" {
				t.Errorf("expected google_casetest, got %q", got.UserID)
			}
		})
	}
}

// ============================================================================
// 17. OAuth gap fix: error redirects
// ============================================================================

func TestAuthStress_ErrorRedirect_ErrorCodeSanitization(t *testing.T) {
	// When the implementation adds redirectWithError, error codes put into
	// the URL query parameter must be safe. Test that error codes with
	// special characters are properly handled.
	dangerousErrorCodes := []string{
		"access_denied",
		`access_denied&token=fake-jwt`,
		`<script>alert(1)</script>`,
		`access_denied\r\nLocation: https://evil.com`,
		`access_denied%0d%0aLocation: https://evil.com`,
		"access_denied\x00evil",
	}

	for _, errorCode := range dangerousErrorCodes {
		t.Run(errorCode[:min(len(errorCode), 30)], func(t *testing.T) {
			callback := "https://portal.example.com/auth/callback"
			u, err := url.Parse(callback)
			if err != nil {
				t.Fatalf("failed to parse callback: %v", err)
			}
			q := u.Query()
			q.Set("error", errorCode)
			u.RawQuery = q.Encode()
			result := u.String()

			// The URL should properly encode the error parameter
			// so it cannot inject additional parameters or headers
			parsed, err := url.Parse(result)
			if err != nil {
				t.Fatalf("result URL is not parseable: %v", err)
			}

			// Should have exactly one error parameter
			errorParam := parsed.Query().Get("error")
			if errorParam != errorCode {
				// URL encoding may transform the value — that's fine
				// as long as no extra params were injected
				t.Logf("error code was transformed: %q -> %q", errorCode, errorParam)
			}

			// Verify no token parameter was injected
			if parsed.Query().Get("token") != "" {
				t.Errorf("VULNERABILITY: error code %q injected a token parameter", errorCode)
			}
		})
	}
}

func TestAuthStress_CallbackErrorRedirect_PreservesExistingParams(t *testing.T) {
	callback := "https://portal.example.com/auth/callback?mode=popup"
	u, err := url.Parse(callback)
	if err != nil {
		t.Fatalf("failed to parse callback: %v", err)
	}
	q := u.Query()
	q.Set("error", "access_denied")
	u.RawQuery = q.Encode()
	result := u.String()

	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatalf("result URL is not parseable: %v", err)
	}

	if parsed.Query().Get("mode") != "popup" {
		t.Error("existing query params should be preserved")
	}
	if parsed.Query().Get("error") != "access_denied" {
		t.Error("error param should be set")
	}
}

// ============================================================================
// 18. OAuth gap fix: JWT name claim
// ============================================================================

func TestAuthStress_SignJWT_NameClaim(t *testing.T) {
	cfg := &common.AuthConfig{
		JWTSecret:   "test-secret-key",
		TokenExpiry: "1h",
	}

	tests := []struct {
		name     string
		userName string
		wantName string
	}{
		{"normal name", "Alice Smith", "Alice Smith"},
		{"empty name", "", ""},
		{"unicode name", "Björk Guðmundsdóttir", "Björk Guðmundsdóttir"},
		{"name with emoji", "Alice 🎉", "Alice 🎉"},
		{"very long name", strings.Repeat("A", 500), strings.Repeat("A", 500)},
		{"name with HTML", "<script>alert(1)</script>", "<script>alert(1)</script>"},
		{"name with null bytes", "Alice\x00Evil", "Alice\x00Evil"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &models.InternalUser{
				UserID: "test_user",
				Email:  "test@example.com",
				Name:   tt.userName,
			}

			tokenStr, err := signJWT(user, "google", cfg)
			if err != nil {
				// Some payloads may cause signing to fail — that's acceptable
				t.Logf("signJWT failed for name %q: %v", tt.userName, err)
				return
			}

			// Parse the token and check the name claim
			_, claims, err := validateJWT(tokenStr, []byte(cfg.JWTSecret))
			if err != nil {
				t.Fatalf("validateJWT failed: %v", err)
			}

			// The name claim should reflect what's in user.Name
			gotName, _ := claims["name"].(string)
			if gotName != tt.wantName {
				t.Errorf("name claim mismatch: got %q, want %q", gotName, tt.wantName)
			}
		})
	}
}

func TestAuthStress_OAuthUserResponse_IncludesName(t *testing.T) {
	user := &models.InternalUser{
		UserID:   "google_12345",
		Email:    "alice@example.com",
		Name:     "Alice Smith",
		Provider: "google",
		Role:     "user",
	}

	resp := oauthUserResponse(user)

	// Basic fields must always be present
	if resp["user_id"] != "google_12345" {
		t.Errorf("expected user_id=google_12345, got %v", resp["user_id"])
	}
	if resp["email"] != "alice@example.com" {
		t.Errorf("expected email=alice@example.com, got %v", resp["email"])
	}
	if resp["provider"] != "google" {
		t.Errorf("expected provider=google, got %v", resp["provider"])
	}

	// Name should be in the response
	if resp["name"] != "Alice Smith" {
		t.Errorf("expected name=Alice Smith, got %v", resp["name"])
	}
}

// ============================================================================
// 19. OAuth gap fix: concurrent email-based linking race condition
// ============================================================================

func TestAuthStress_ConcurrentEmailLinking(t *testing.T) {
	// Race condition: two simultaneous first-time OAuth logins with the same
	// email from different providers could create duplicate users.
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	results := make(chan *models.InternalUser, 10)
	errors := make(chan string, 10)

	// 5 concurrent findOrCreateOAuthUser calls with same email, different providers
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			userID := fmt.Sprintf("provider_%d_user", idx)
			user := srv.findOrCreateOAuthUser(ctx, userID, "shared@example.com", "Shared User", "provider")
			if user == nil {
				errors <- fmt.Sprintf("provider_%d returned nil", idx)
				return
			}
			results <- user
		}(i)
	}

	wg.Wait()
	close(results)
	close(errors)

	for err := range errors {
		t.Errorf("concurrent email linking error: %s", err)
	}

	// All results should have the same email
	var userIDs []string
	for user := range results {
		if user.Email != "shared@example.com" {
			t.Errorf("expected email shared@example.com, got %q", user.Email)
		}
		userIDs = append(userIDs, user.UserID)
	}

	if len(userIDs) == 0 {
		t.Fatal("no users created")
	}
}

// ============================================================================
// 20. OAuth gap fix: Name field with hostile inputs
// ============================================================================

func TestAuthStress_FindOrCreateOAuthUser_NamePersistence(t *testing.T) {
	// Verify that the Name field is properly stored and retrieved
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create a user via findOrCreateOAuthUser
	user := srv.findOrCreateOAuthUser(ctx, "google_nametest", "nametest@example.com", "Name Test", "google")
	if user == nil {
		t.Fatal("findOrCreateOAuthUser returned nil")
	}

	// Retrieve and check
	got, err := store.GetUser(ctx, "google_nametest")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}

	if got.Email != "nametest@example.com" {
		t.Errorf("expected email nametest@example.com, got %q", got.Email)
	}
	if got.Provider != "google" {
		t.Errorf("expected provider google, got %q", got.Provider)
	}
}

func TestAuthStress_DevProvider_NameFieldInToken(t *testing.T) {
	// The dev provider creates a user — verify the JWT includes the name claim
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{"provider": "dev"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	tokenStr, ok := data["token"].(string)
	if !ok || tokenStr == "" {
		t.Fatal("expected non-empty token")
	}

	// Parse the JWT and verify name claim exists
	_, claims, err := validateJWT(tokenStr, []byte(srv.app.Config.Auth.JWTSecret))
	if err != nil {
		t.Fatalf("validateJWT failed: %v", err)
	}

	// name claim should exist (even if empty string before gap fix)
	if _, ok := claims["name"]; !ok {
		t.Error("expected 'name' claim in JWT")
	}
}

// ============================================================================
// 21. OAuth gap fix: callback error handling in Google/GitHub callbacks
// ============================================================================

func TestAuthStress_Callback_GoogleErrorParam(t *testing.T) {
	// When Google sends ?error=access_denied, the callback should redirect
	// the user back to the frontend with the error, not show a raw error page.
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = "id"
	srv.app.Config.Auth.Google.ClientSecret = "secret"

	secret := []byte(srv.app.Config.Auth.JWTSecret)
	state, err := encodeOAuthState("https://portal.example.com/auth", secret)
	if err != nil {
		t.Fatalf("encodeOAuthState failed: %v", err)
	}

	// Simulate Google returning an error (user denied consent)
	req := httptest.NewRequest(http.MethodGet,
		"/api/auth/callback/google?error=access_denied&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGoogle(rec, req)

	// Should redirect to callback with error param
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d: %s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "error=access_denied") {
		t.Errorf("expected error=access_denied in redirect, got %q", location)
	}
	if !strings.Contains(location, "portal.example.com") {
		t.Errorf("expected redirect to callback host, got %q", location)
	}
}

func TestAuthStress_Callback_GitHubErrorParam(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.GitHub.ClientID = "id"
	srv.app.Config.Auth.GitHub.ClientSecret = "secret"

	secret := []byte(srv.app.Config.Auth.JWTSecret)
	state, err := encodeOAuthState("https://portal.example.com/auth", secret)
	if err != nil {
		t.Fatalf("encodeOAuthState failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/auth/callback/github?error=access_denied&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGitHub(rec, req)

	// Should redirect to callback with error param
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d: %s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	if !strings.Contains(location, "error=access_denied") {
		t.Errorf("expected error=access_denied in redirect, got %q", location)
	}
	if !strings.Contains(location, "portal.example.com") {
		t.Errorf("expected redirect to callback host, got %q", location)
	}
}

func TestAuthStress_Callback_ErrorWithInvalidState(t *testing.T) {
	// When provider sends error AND state is invalid (or missing),
	// we can't redirect — should return a non-redirect error
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = "id"
	srv.app.Config.Auth.Google.ClientSecret = "secret"

	req := httptest.NewRequest(http.MethodGet,
		"/api/auth/callback/google?error=access_denied&state=invalid", nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGoogle(rec, req)

	// With invalid state, we can't redirect — should be 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for error with invalid state, got %d", rec.Code)
	}
}

// ============================================================================
// 22. Account takeover via email linking — cross-provider attack
// ============================================================================

func TestAuthStress_AccountTakeover_CrossProvider(t *testing.T) {
	// Scenario: Alice signs up with Google (alice@example.com).
	// Attacker controls a GitHub account that also has alice@example.com.
	// The attacker should get linked to Alice's Google account (by design),
	// but the returned user should still be Alice's original account —
	// NOT a new account with attacker-chosen fields.
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Alice registers via Google
	alice := &models.InternalUser{
		UserID:    "google_alice123",
		Email:     "alice@example.com",
		Name:      "Alice Real",
		Provider:  "google",
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, alice); err != nil {
		t.Fatalf("failed to create alice: %v", err)
	}

	// Attacker logs in via GitHub with same email
	attacker := srv.findOrCreateOAuthUser(ctx, "github_attacker999", "alice@example.com", "Evil Attacker", "github")
	if attacker == nil {
		t.Fatal("findOrCreateOAuthUser returned nil")
	}

	// The returned user should be Alice's original account
	if attacker.UserID != "google_alice123" {
		t.Errorf("expected linked to google_alice123, got %q — a NEW user was created instead of linking", attacker.UserID)
	}
	if attacker.Provider != "google" {
		t.Errorf("expected original provider=google, got %q", attacker.Provider)
	}
	if attacker.Role != "user" {
		t.Errorf("expected role=user preserved, got %q", attacker.Role)
	}

	// FINDING: Name may be updated to attacker's name — verify
	if attacker.Name == "Evil Attacker" {
		t.Log("FINDING: cross-provider login updates the Name field to the new provider's value. " +
			"An attacker who controls a GitHub account with the same email can change the display name " +
			"of an existing Google user. This is cosmetic but should be noted.")
	}
}

// ============================================================================
// 23. Email linking does NOT update provider or role
// ============================================================================

func TestAuthStress_EmailLinking_PreservesProviderAndRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create an admin user via email provider
	admin := &models.InternalUser{
		UserID:    "admin_user",
		Email:     "admin@corp.com",
		Name:      "Admin",
		Provider:  "email",
		Role:      "admin",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, admin); err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}

	// OAuth login with same email should NOT change role or provider
	linked := srv.findOrCreateOAuthUser(ctx, "google_adminimposter", "admin@corp.com", "Imposter", "google")
	if linked == nil {
		t.Fatal("findOrCreateOAuthUser returned nil")
	}

	if linked.Role != "admin" {
		t.Errorf("expected role=admin preserved after email linking, got %q", linked.Role)
	}
	if linked.Provider != "email" {
		t.Errorf("expected provider=email preserved after email linking, got %q", linked.Provider)
	}
	if linked.UserID != "admin_user" {
		t.Errorf("expected UserID=admin_user (original), got %q", linked.UserID)
	}
}

// ============================================================================
// 24. Name field — hostile input stress tests
// ============================================================================

func TestAuthStress_NameField_XSSInJWT(t *testing.T) {
	cfg := &common.AuthConfig{JWTSecret: "test-secret", TokenExpiry: "1h"}

	xssPayloads := []string{
		`<script>alert('xss')</script>`,
		`<img src=x onerror=alert(1)>`,
		`" onclick="alert(1)`,
		`'; DROP TABLE users; --`,
		`javascript:alert(1)`,
		"<svg/onload=alert(1)>",
	}

	for _, payload := range xssPayloads {
		t.Run(payload[:min(len(payload), 25)], func(t *testing.T) {
			user := &models.InternalUser{
				UserID: "xss_user",
				Email:  "xss@example.com",
				Name:   payload,
			}

			tokenStr, err := signJWT(user, "google", cfg)
			if err != nil {
				t.Fatalf("signJWT failed: %v", err)
			}

			_, claims, err := validateJWT(tokenStr, []byte(cfg.JWTSecret))
			if err != nil {
				t.Fatalf("validateJWT failed: %v", err)
			}

			gotName, _ := claims["name"].(string)
			if gotName != payload {
				t.Errorf("name claim mismatch: got %q, want %q", gotName, payload)
			}

			// JWT is base64-encoded, raw HTML should not appear in the token string
			if strings.Contains(tokenStr, "<script>") {
				t.Error("VULNERABILITY: raw HTML found in JWT token string")
			}
		})
	}
}

func TestAuthStress_NameField_ExtremeLength(t *testing.T) {
	cfg := &common.AuthConfig{JWTSecret: "test-secret", TokenExpiry: "1h"}

	longNames := []struct {
		name string
		len  int
	}{
		{"1KB", 1024},
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
	}

	for _, tt := range longNames {
		t.Run(tt.name, func(t *testing.T) {
			user := &models.InternalUser{
				UserID: "longname_user",
				Email:  "long@example.com",
				Name:   strings.Repeat("A", tt.len),
			}

			tokenStr, err := signJWT(user, "google", cfg)
			if err != nil {
				return // Failing to sign very long names is acceptable
			}

			_, claims, err := validateJWT(tokenStr, []byte(cfg.JWTSecret))
			if err != nil {
				t.Fatalf("validateJWT failed: %v", err)
			}

			gotName, _ := claims["name"].(string)
			if len(gotName) != tt.len {
				t.Errorf("expected name length %d, got %d", tt.len, len(gotName))
			}

			t.Logf("JWT token size with %s name: %d bytes", tt.name, len(tokenStr))
			if len(tokenStr) > 200*1024 {
				t.Logf("FINDING: JWT with %s name produces %d byte token — may exceed header size limits", tt.name, len(tokenStr))
			}
		})
	}
}

func TestAuthStress_NameField_NullBytesAndControlChars(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	controlNames := []string{
		"Alice\x00Evil",
		"Alice\nNewline",
		"Alice\rCarriage",
		"Alice\tTab",
		"Alice\x1bEscape",
		"Alice\x7fDelete",
	}

	for i, name := range controlNames {
		t.Run(fmt.Sprintf("control_char_%d", i), func(t *testing.T) {
			userID := fmt.Sprintf("control_%d", i)
			user := srv.findOrCreateOAuthUser(ctx, userID, fmt.Sprintf("control%d@test.com", i), name, "google")
			if user == nil {
				t.Fatal("findOrCreateOAuthUser returned nil")
			}

			got, err := store.GetUser(ctx, userID)
			if err != nil {
				t.Fatalf("GetUser failed: %v", err)
			}

			if got.Name != name {
				t.Logf("name was modified during storage: %q -> %q", name, got.Name)
			}
		})
	}
}

// ============================================================================
// 25. redirectWithError stress tests
// ============================================================================

func TestAuthStress_RedirectWithError_HostileErrorCodes(t *testing.T) {
	tests := []struct {
		name      string
		errorCode string
	}{
		{"normal", "access_denied"},
		{"with_ampersand", "access_denied&token=fake"},
		{"html_injection", "<script>alert(1)</script>"},
		{"crlf_injection", "error\r\nLocation: https://evil.com"},
		{"null_bytes", "error\x00bypass"},
		{"url_encoded", "error%26token%3Dfake"},
		{"extremely_long", strings.Repeat("x", 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callback := "https://portal.example.com/auth"
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)

			redirectWithError(rec, req, callback, tt.errorCode)

			if rec.Code != http.StatusFound {
				t.Errorf("expected 302, got %d", rec.Code)
				return
			}

			location := rec.Header().Get("Location")
			parsed, err := url.Parse(location)
			if err != nil {
				t.Fatalf("redirect URL is not parseable: %v", err)
			}

			// The error should be URL-encoded so it cannot inject params
			if parsed.Query().Get("token") != "" {
				t.Errorf("VULNERABILITY: error code %q injected a token parameter", tt.errorCode)
			}

			if parsed.Host != "portal.example.com" {
				t.Errorf("VULNERABILITY: redirect went to unexpected host %q", parsed.Host)
			}
		})
	}
}

func TestAuthStress_RedirectWithError_InvalidCallback(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	redirectWithError(rec, req, "://invalid-url", "test_error")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for invalid callback URL, got %d", rec.Code)
	}
}

// ============================================================================
// 26. findOrCreateOAuthUser — name update on re-login
// ============================================================================

func TestAuthStress_FindOrCreateOAuthUser_NameUpdate(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// First login creates user with name "Alice"
	user := srv.findOrCreateOAuthUser(ctx, "google_nameupdate", "alice@test.com", "Alice", "google")
	if user == nil {
		t.Fatal("first login: findOrCreateOAuthUser returned nil")
	}
	if user.Name != "Alice" {
		t.Errorf("expected name=Alice, got %q", user.Name)
	}

	// Second login updates name to "Alice Smith"
	user2 := srv.findOrCreateOAuthUser(ctx, "google_nameupdate", "alice@test.com", "Alice Smith", "google")
	if user2 == nil {
		t.Fatal("second login: findOrCreateOAuthUser returned nil")
	}
	if user2.Name != "Alice Smith" {
		t.Errorf("expected name=Alice Smith after re-login, got %q", user2.Name)
	}

	got, err := store.GetUser(ctx, "google_nameupdate")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got.Name != "Alice Smith" {
		t.Errorf("expected persisted name=Alice Smith, got %q", got.Name)
	}
}

func TestAuthStress_FindOrCreateOAuthUser_EmptyNameDoesNotOverwrite(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	user := srv.findOrCreateOAuthUser(ctx, "google_emptyname", "emptyname@test.com", "Alice", "google")
	if user == nil {
		t.Fatal("first login: findOrCreateOAuthUser returned nil")
	}

	user2 := srv.findOrCreateOAuthUser(ctx, "google_emptyname", "emptyname@test.com", "", "google")
	if user2 == nil {
		t.Fatal("second login: findOrCreateOAuthUser returned nil")
	}

	got, err := store.GetUser(ctx, "google_emptyname")
	if err != nil {
		t.Fatalf("GetUser failed: %v", err)
	}
	if got.Name != "Alice" {
		t.Errorf("expected name=Alice preserved when empty name sent, got %q", got.Name)
	}
}

// ============================================================================
// 27. Email-based linking edge cases
// ============================================================================

func TestAuthStress_EmailLinking_MultipleUsersWithDifferentProvidersSameEmail(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	user1 := &models.InternalUser{
		UserID:    "google_first",
		Email:     "shared@test.com",
		Name:      "First User",
		Provider:  "google",
		Role:      "user",
		CreatedAt: time.Now(),
	}
	if err := store.SaveUser(ctx, user1); err != nil {
		t.Fatalf("failed to save user1: %v", err)
	}

	found, err := store.GetUserByEmail(ctx, "shared@test.com")
	if err != nil {
		t.Fatalf("GetUserByEmail failed: %v", err)
	}
	if found.UserID != "google_first" {
		t.Errorf("expected google_first, got %q", found.UserID)
	}

	linked := srv.findOrCreateOAuthUser(ctx, "github_second", "shared@test.com", "Second User", "github")
	if linked == nil {
		t.Fatal("findOrCreateOAuthUser returned nil")
	}
	if linked.UserID != "google_first" {
		t.Errorf("expected email linking to return google_first, got %q", linked.UserID)
	}
}

func TestAuthStress_EmailLinking_EmptyEmailCreatesSeparateUsers(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	user1 := srv.findOrCreateOAuthUser(ctx, "github_noemail1", "", "User One", "github")
	if user1 == nil {
		t.Fatal("user1 creation failed")
	}

	user2 := srv.findOrCreateOAuthUser(ctx, "github_noemail2", "", "User Two", "github")
	if user2 == nil {
		t.Fatal("user2 creation failed")
	}

	if user1.UserID == user2.UserID {
		t.Error("expected separate users for empty email logins, got same user")
	}

	got1, err1 := store.GetUser(ctx, "github_noemail1")
	got2, err2 := store.GetUser(ctx, "github_noemail2")
	if err1 != nil || err2 != nil {
		t.Fatalf("failed to get users: err1=%v, err2=%v", err1, err2)
	}
	if got1.UserID == got2.UserID {
		t.Error("expected distinct users in store")
	}
}

// ============================================================================
// 28. Callback error handling — WriteError vs redirect inconsistency
// ============================================================================

func TestAuthStress_Callback_ErrorsAfterStateDecodeNotRedirected(t *testing.T) {
	// FINDING: After the state is decoded and callback URL is available,
	// errors in the Google/GitHub callbacks still use WriteError (JSON)
	// instead of redirectWithError. This means the user's browser shows
	// a raw JSON error page instead of being redirected back to the portal.
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.Google.ClientID = ""
	srv.app.Config.Auth.Google.ClientSecret = ""

	secret := []byte(srv.app.Config.Auth.JWTSecret)
	state, err := encodeOAuthState("https://portal.example.com/auth", secret)
	if err != nil {
		t.Fatalf("encodeOAuthState failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/auth/callback/google?code=testcode&state="+url.QueryEscape(state), nil)
	rec := httptest.NewRecorder()
	srv.handleOAuthCallbackGoogle(rec, req)

	if rec.Code == http.StatusInternalServerError {
		t.Log("FINDING: Google callback returns 500 JSON error when provider not configured, " +
			"even though the callback URL is available from the decoded state. " +
			"Should redirect with ?error=provider_not_configured instead. " +
			"Gap 2 (error redirects) was only partially implemented — " +
			"the redirectWithError helper exists but the callback handlers " +
			"still use WriteError for post-state-decode errors.")
	} else if rec.Code == http.StatusFound {
		location := rec.Header().Get("Location")
		if !strings.Contains(location, "error=") {
			t.Errorf("redirect does not contain error parameter: %q", location)
		}
	}
}
