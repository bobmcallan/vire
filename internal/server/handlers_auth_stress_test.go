package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
			user := srv.findOrCreateOAuthUser(context.Background(), "oauth_race_user", "race@x.com", "google")
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
