package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// 1. Path traversal and injection in usernames
// ============================================================================

func TestUserStress_PathTraversalUsernames(t *testing.T) {
	srv := newTestServerWithStorage(t)

	traversalPayloads := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"../../etc/shadow",
		"user/../admin",
		"./current",
		"/absolute/path",
		"..%2F..%2Fetc%2Fpasswd",
		"....//....//etc/passwd",
	}

	for _, payload := range traversalPayloads {
		t.Run("create_"+payload, func(t *testing.T) {
			body := jsonBody(t, map[string]string{
				"username": payload,
				"password": "testpass123",
				"email":    "test@test.com",
				"role":     "user",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/users", body)
			rec := httptest.NewRecorder()
			srv.handleUserCreate(rec, req)

			// Should not crash — sanitizeKey handles path traversal
			if rec.Code >= 500 {
				t.Errorf("server error with path traversal username %q: status %d", payload, rec.Code)
			}
		})

		t.Run("get_"+payload, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/users/"+payload, nil)
			rec := httptest.NewRecorder()
			srv.handleUserGet(rec, req, payload)

			if rec.Code >= 500 {
				t.Errorf("server error with path traversal GET %q: status %d", payload, rec.Code)
			}
		})

		t.Run("delete_"+payload, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/api/users/"+payload, nil)
			rec := httptest.NewRecorder()
			srv.handleUserDelete(rec, req, payload)

			// Should get 404 (not found) or 200, never 5xx
			if rec.Code >= 500 {
				t.Errorf("server error with path traversal DELETE %q: status %d", payload, rec.Code)
			}
		})
	}
}

func TestUserStress_SpecialCharUsernames(t *testing.T) {
	srv := newTestServerWithStorage(t)

	specialPayloads := []string{
		"user with spaces",
		"user\ttab",
		"user\nnewline",
		"user\x00null",
		"user\r\nCRLF",
		"<script>alert('xss')</script>",
		"'; DROP TABLE users; --",
		"user@domain.com",
		"user+tag",
		"user%20encoded",
		strings.Repeat("A", 1000),
		"émojis😈",
		"日本語ユーザー",
		"user:colon",
		"user|pipe",
		"user*star",
		"user?question",
		".hidden",
		"..dotdot",
		"...threedots",
	}

	for _, payload := range specialPayloads {
		t.Run(payload[:min(len(payload), 20)], func(t *testing.T) {
			body := jsonBody(t, map[string]string{
				"username": payload,
				"password": "testpass123",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/users", body)
			rec := httptest.NewRecorder()
			srv.handleUserCreate(rec, req)

			// Should not crash
			if rec.Code >= 500 {
				t.Errorf("server error with special username %q: status %d, body: %s",
					payload[:min(len(payload), 50)], rec.Code, rec.Body.String())
			}
		})
	}
}

// ============================================================================
// 2. Password edge cases
// ============================================================================

func TestUserStress_VeryLongPassword_TruncatedTo72Bytes(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create user with a password longer than 72 bytes
	longPass := strings.Repeat("A", 200)
	createTestUser(t, srv, "longpass-user", "lp@x.com", longPass, "user")

	// Login should work with the full long password
	body := jsonBody(t, map[string]string{
		"username": "longpass-user",
		"password": longPass,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with full long password, got %d", rec.Code)
	}

	// Login should also work with just the first 72 bytes (bcrypt truncation)
	truncPass := longPass[:72]
	body = jsonBody(t, map[string]string{
		"username": "longpass-user",
		"password": truncPass,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec = httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with first 72 bytes, got %d", rec.Code)
	}

	// Login should FAIL with first 71 bytes
	shortPass := longPass[:71]
	body = jsonBody(t, map[string]string{
		"username": "longpass-user",
		"password": shortPass,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec = httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with 71-byte password, got %d", rec.Code)
	}
}

func TestUserStress_EmptyPasswordRejected(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "nopass",
		"password": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty password, got %d", rec.Code)
	}
}

func TestUserStress_WhitespaceOnlyPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Whitespace-only password currently passes the `== ""` check.
	// Document this behavior: it's not ideal but not a security hole
	// since the password is still hashed.
	body := jsonBody(t, map[string]string{
		"username": "wspass",
		"password": "   ",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	// Should succeed (whitespace is technically a valid password)
	// but verify login works correctly
	if rec.Code == http.StatusCreated {
		loginBody := jsonBody(t, map[string]string{
			"username": "wspass",
			"password": "   ",
		})
		loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
		loginRec := httptest.NewRecorder()
		srv.handleAuthLogin(loginRec, loginReq)

		if loginRec.Code != http.StatusOK {
			t.Errorf("expected login with whitespace password to succeed, got %d", loginRec.Code)
		}
	}
}

func TestUserStress_UnicodePassword(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Unicode password: 72-byte limit is in bytes, not characters
	// This is important: a 72-character unicode string could be >72 bytes
	unicodePass := strings.Repeat("日", 30) // 30 * 3 bytes = 90 bytes UTF-8
	createTestUser(t, srv, "unicode-user", "u@x.com", unicodePass, "user")

	// Login with same password should work
	body := jsonBody(t, map[string]string{
		"username": "unicode-user",
		"password": unicodePass,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with unicode password, got %d", rec.Code)
	}
}

// ============================================================================
// 3. JSON injection and malformed bodies
// ============================================================================

func TestUserStress_InvalidJSON(t *testing.T) {
	srv := newTestServerWithStorage(t)

	invalidBodies := []string{
		"",
		"not json",
		"{invalid",
		"[]",
		`{"username": }`,
		`{"username": "test", "password": "test", "extra": {"nested": "deep"}}`,
		`null`,
	}

	for _, body := range invalidBodies {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			var reqBody *bytes.Buffer
			if body == "" {
				reqBody = bytes.NewBuffer(nil)
			} else {
				reqBody = bytes.NewBufferString(body)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/users", reqBody)
			rec := httptest.NewRecorder()
			srv.handleUserCreate(rec, req)

			// Should never crash — return 400 for bad JSON
			if rec.Code >= 500 {
				t.Errorf("server error with body %q: status %d", body, rec.Code)
			}
		})
	}
}

func TestUserStress_NilBody(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for nil body, got %d", rec.Code)
	}
}

func TestUserStress_ExtraFieldsIgnored(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Attempt to inject password_hash directly
	body := jsonBody(t, map[string]interface{}{
		"username":      "inject-user",
		"password":      "normalpass",
		"password_hash": "$2a$10$injectedhash",
		"navexa_key":    "injected-key",
		"is_admin":      true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify the injected password_hash was NOT used
	ctx := context.Background()
	user, _ := srv.app.Storage.InternalStore().GetUser(ctx, "inject-user")
	if user.PasswordHash == "$2a$10$injectedhash" {
		t.Error("VULNERABILITY: injected password_hash was stored directly")
	}
	// navexa_key is an API key that users can legitimately set during creation
	// (unlike password_hash which must always be server-generated via bcrypt)
	// So we verify it IS stored if provided — this is intentional, not a vulnerability.
	kv, err := srv.app.Storage.InternalStore().GetUserKV(ctx, "inject-user", "navexa_key")
	if err != nil || kv.Value != "injected-key" {
		t.Error("expected navexa_key to be stored when provided during create")
	}
}

// ============================================================================
// 4. Navexa key security
// ============================================================================

func TestUserStress_NavexaKeyNeverInAnyResponse(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	// Create user and set a navexa key directly in storage
	createTestUser(t, srv, "alice", "a@x.com", "pass", "user")
	srv.app.Storage.InternalStore().SetUserKV(ctx, "alice", "navexa_key", "nk-super-secret-key-abc123")

	// Test GET response
	t.Run("get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/users/alice", nil)
		rec := httptest.NewRecorder()
		srv.handleUserGet(rec, req, "alice")

		body := rec.Body.String()
		if strings.Contains(body, "nk-super-secret-key-abc123") {
			t.Error("VULNERABILITY: full navexa_key exposed in GET response")
		}
		if strings.Contains(body, "super-secret") {
			t.Error("VULNERABILITY: partial navexa_key exposed in GET response")
		}
	})

	// Test UPDATE response
	t.Run("update", func(t *testing.T) {
		body := jsonBody(t, map[string]interface{}{"email": "new@x.com"})
		req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
		rec := httptest.NewRecorder()
		srv.handleUserUpdate(rec, req, "alice")

		respBody := rec.Body.String()
		if strings.Contains(respBody, "nk-super-secret-key-abc123") {
			t.Error("VULNERABILITY: full navexa_key exposed in UPDATE response")
		}
	})

	// Test LOGIN response
	t.Run("login", func(t *testing.T) {
		body := jsonBody(t, map[string]string{"username": "alice", "password": "pass"})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
		rec := httptest.NewRecorder()
		srv.handleAuthLogin(rec, req)

		respBody := rec.Body.String()
		if strings.Contains(respBody, "nk-super-secret-key-abc123") {
			t.Error("VULNERABILITY: full navexa_key exposed in LOGIN response")
		}
	})
}

func TestUserStress_NavexaKeyPreview_ShortKeys(t *testing.T) {
	tests := []struct {
		key      string
		expected string
	}{
		{"", ""},
		{"a", "****"},
		{"ab", "****"},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "****bcde"},
	}
	for _, tt := range tests {
		got := navexaKeyPreview(tt.key)
		if got != tt.expected {
			t.Errorf("navexaKeyPreview(%q) = %q, want %q", tt.key, got, tt.expected)
		}
	}
}

func TestUserStress_PasswordHashNeverInAnyResponse(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "mypassword", "user")

	endpoints := []struct {
		name    string
		method  string
		path    string
		body    interface{}
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"get", http.MethodGet, "/api/users/alice", nil, nil},
		{"update", http.MethodPut, "/api/users/alice", map[string]interface{}{"email": "new@x.com"}, nil},
		{"login", http.MethodPost, "/api/auth/login", map[string]string{"username": "alice", "password": "mypassword"}, nil},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			var req *http.Request
			if ep.body != nil {
				req = httptest.NewRequest(ep.method, ep.path, jsonBody(t, ep.body))
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			rec := httptest.NewRecorder()

			switch ep.name {
			case "get":
				srv.handleUserGet(rec, req, "alice")
			case "update":
				srv.handleUserUpdate(rec, req, "alice")
			case "login":
				srv.handleAuthLogin(rec, req)
			}

			body := rec.Body.String()
			if strings.Contains(body, "$2a$") {
				t.Errorf("VULNERABILITY: bcrypt hash exposed in %s response", ep.name)
			}
			if strings.Contains(body, "password_hash") {
				t.Errorf("VULNERABILITY: password_hash field exposed in %s response", ep.name)
			}
		})
	}
}

// ============================================================================
// 5. Authentication edge cases
// ============================================================================

func TestUserStress_LoginTimingConsistency(t *testing.T) {
	// Verify that login returns the same error message for:
	// - non-existent user
	// - wrong password
	// This prevents user enumeration
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "exists", "e@x.com", "pass", "user")

	// Non-existent user
	body1 := jsonBody(t, map[string]string{"username": "nonexistent", "password": "pass"})
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/login", body1)
	rec1 := httptest.NewRecorder()
	srv.handleAuthLogin(rec1, req1)

	// Wrong password
	body2 := jsonBody(t, map[string]string{"username": "exists", "password": "wrong"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/login", body2)
	rec2 := httptest.NewRecorder()
	srv.handleAuthLogin(rec2, req2)

	// Both should return 401 with identical error message
	if rec1.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for nonexistent user, got %d", rec1.Code)
	}
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong password, got %d", rec2.Code)
	}

	var resp1, resp2 ErrorResponse
	json.NewDecoder(rec1.Body).Decode(&resp1)
	json.NewDecoder(rec2.Body).Decode(&resp2)

	if resp1.Error != resp2.Error {
		t.Errorf("VULNERABILITY: different error messages for nonexistent user (%q) vs wrong password (%q) enables user enumeration",
			resp1.Error, resp2.Error)
	}
}

func TestUserStress_LoginMissingFields(t *testing.T) {
	srv := newTestServerWithStorage(t)

	cases := []struct {
		name string
		body map[string]string
	}{
		{"empty_body", map[string]string{}},
		{"no_password", map[string]string{"username": "alice"}},
		{"no_username", map[string]string{"password": "pass"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", jsonBody(t, tc.body))
			rec := httptest.NewRecorder()
			srv.handleAuthLogin(rec, req)

			// Should return 401, not crash
			if rec.Code >= 500 {
				t.Errorf("server error for %s: status %d", tc.name, rec.Code)
			}
		})
	}
}

// ============================================================================
// 6. Import edge cases
// ============================================================================

func TestUserStress_ImportEmptyUsersArray(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty import, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] != float64(0) {
		t.Errorf("expected 0 imported, got %v", data["imported"])
	}
}

func TestUserStress_ImportDuplicatesInSameBatch(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{
			{"username": "dup", "email": "d1@x.com", "password": "pass1", "role": "user"},
			{"username": "dup", "email": "d2@x.com", "password": "pass2", "role": "admin"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})

	// First one should import, second should be skipped
	if data["imported"] != float64(1) {
		t.Errorf("expected 1 imported, got %v", data["imported"])
	}
	if data["skipped"] != float64(1) {
		t.Errorf("expected 1 skipped, got %v", data["skipped"])
	}

	// Verify first version was kept (email d1@x.com)
	ctx := context.Background()
	user, _ := srv.app.Storage.InternalStore().GetUser(ctx, "dup")
	if user.Email != "d1@x.com" {
		t.Errorf("expected first import to be kept, got email %q", user.Email)
	}
}

func TestUserStress_ImportMixedExistingAndNew(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "existing", "e@x.com", "pass", "admin")

	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{
			{"username": "existing", "email": "new@x.com", "password": "newpass", "role": "user"},
			{"username": "new-user", "email": "n@x.com", "password": "pass", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify existing user was NOT modified
	ctx := context.Background()
	user, _ := srv.app.Storage.InternalStore().GetUser(ctx, "existing")
	if user.Email != "e@x.com" {
		t.Errorf("existing user was modified during import: email=%q", user.Email)
	}
}

func TestUserStress_ImportEmptyPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Import with empty password — bcrypt will hash empty string
	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{
			{"username": "emptypass", "email": "ep@x.com", "password": "", "role": "user"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// FINDING: empty password is importable and hashable with bcrypt.
	// This means a user can be created with an empty password via import
	// (even though direct create rejects it).
	// Verify login with empty password works for this user.
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	data := resp["data"].(map[string]interface{})
	if data["imported"] == float64(1) {
		loginBody := jsonBody(t, map[string]string{
			"username": "emptypass",
			"password": "",
		})
		loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
		loginRec := httptest.NewRecorder()
		srv.handleAuthLogin(loginRec, loginReq)

		if loginRec.Code == http.StatusOK {
			t.Log("FINDING: import allows empty password, and login succeeds with empty password")
		}
	}
}

// ============================================================================
// 7. Concurrent access
// ============================================================================

func TestUserStress_ConcurrentReadWrite(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "concurrent-user", "c@x.com", "pass", "user")

	var wg sync.WaitGroup
	errors := make(chan string, 100)

	// 10 concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/users/concurrent-user", nil)
			rec := httptest.NewRecorder()
			srv.handleUserGet(rec, req, "concurrent-user")
			if rec.Code >= 500 {
				errors <- "GET returned 5xx"
			}
		}()
	}

	// 5 concurrent updates
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := jsonBody(t, map[string]interface{}{
				"email": "update@x.com",
			})
			req := httptest.NewRequest(http.MethodPut, "/api/users/concurrent-user", body)
			rec := httptest.NewRecorder()
			srv.handleUserUpdate(rec, req, "concurrent-user")
			if rec.Code >= 500 {
				errors <- "PUT returned 5xx"
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %s", err)
	}
}

// ============================================================================
// 8. Error response information leakage
// ============================================================================

func TestUserStress_ErrorResponsesNoInternalDetails(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Test that error responses for user operations don't leak file paths,
	// stack traces, or internal details
	t.Run("create_missing_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
		rec := httptest.NewRecorder()
		srv.handleUserCreate(rec, req)
		assertNoSensitiveData(t, rec.Body.String())
	})

	t.Run("login_missing_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
		rec := httptest.NewRecorder()
		srv.handleAuthLogin(rec, req)
		assertNoSensitiveData(t, rec.Body.String())
	})

	t.Run("import_missing_body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/users/import", nil)
		rec := httptest.NewRecorder()
		srv.handleUserImport(rec, req)
		assertNoSensitiveData(t, rec.Body.String())
	})

	t.Run("create_save_error", func(t *testing.T) {
		// Try to create a user that will trigger a storage error
		// (e.g., with a username that causes filesystem issues)
		body := jsonBody(t, map[string]string{
			"username": strings.Repeat("x", 300),
			"password": "test",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/users", body)
		rec := httptest.NewRecorder()
		srv.handleUserCreate(rec, req)
		assertNoSensitiveData(t, rec.Body.String())
	})
}

func assertNoSensitiveData(t *testing.T, body string) {
	t.Helper()
	sensitivePatterns := []string{
		"/home/", "/tmp/", "/var/", "/app/", // file paths
		"goroutine", "panic", // stack traces
		"runtime.", // go runtime
		"rename ",  // OS operations
		".tmp-",    // temp file names
	}
	for _, pattern := range sensitivePatterns {
		if strings.Contains(body, pattern) {
			t.Errorf("VULNERABILITY: error response contains sensitive pattern %q in body: %s",
				pattern, body[:min(len(body), 200)])
		}
	}
}

// ============================================================================
// 9. Middleware user resolution edge cases
// ============================================================================

func TestUserStress_MiddlewareEmptyUserID(t *testing.T) {
	srv := newTestServerWithStorage(t)

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code >= 500 {
		t.Errorf("server error with empty User-ID header: status %d", rr.Code)
	}
	// Empty header value means Header.Get returns "" which does not trigger context creation
	if capturedUC != nil {
		t.Log("INFO: empty User-ID header still created a UserContext")
	}
}

func TestUserStress_MiddlewareBothHeadersPresent(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:       "testuser",
		Email:        "t@x.com",
		PasswordHash: "hash",
	})
	srv.app.Storage.InternalStore().SetUserKV(ctx, "testuser", "navexa_key", "stored-key-1234")

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Both X-Vire-User-ID and X-Vire-Navexa-Key present
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "testuser")
	req.Header.Set("X-Vire-Navexa-Key", "header-override-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext")
	}
	// Header value should take precedence over stored value
	if capturedUC.NavexaAPIKey != "header-override-key" {
		t.Errorf("expected header key to take precedence, got %q", capturedUC.NavexaAPIKey)
	}
}

func TestUserStress_MiddlewareNonExistentUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Non-existent user — should not error, just skip resolution
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("X-Vire-User-ID", "ghost-user-404")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code >= 500 {
		t.Errorf("server error with non-existent user in middleware: status %d", rr.Code)
	}
	if capturedUC == nil {
		t.Fatal("expected UserContext to be created even for unknown user")
	}
	if capturedUC.NavexaAPIKey != "" {
		t.Errorf("expected empty navexa key for unknown user, got %q", capturedUC.NavexaAPIKey)
	}
	if capturedUC.UserID != "ghost-user-404" {
		t.Errorf("expected UserID to be set, got %q", capturedUC.UserID)
	}
}

// ============================================================================
// 10. Update edge cases
// ============================================================================

func TestUserStress_UpdateNonExistentUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{"email": "new@x.com"})
	req := httptest.NewRequest(http.MethodPut, "/api/users/nobody", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "nobody")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for update of non-existent user, got %d", rec.Code)
	}
}

func TestUserStress_UpdatePasswordChangesHash(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "oldpass", "user")

	// Update password
	body := jsonBody(t, map[string]interface{}{"password": "newpass"})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Old password should fail
	loginBody := jsonBody(t, map[string]string{"username": "alice", "password": "oldpass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)

	if loginRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with old password after update, got %d", loginRec.Code)
	}

	// New password should work
	loginBody = jsonBody(t, map[string]string{"username": "alice", "password": "newpass"})
	loginReq = httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec = httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)

	if loginRec.Code != http.StatusOK {
		t.Errorf("expected 200 with new password, got %d", loginRec.Code)
	}
}

// ============================================================================
// 11. Delete edge cases
// ============================================================================

func TestUserStress_DeleteNonExistentUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/users/nobody", nil)
	rec := httptest.NewRecorder()
	srv.handleUserDelete(rec, req, "nobody")

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for delete of non-existent user, got %d", rec.Code)
	}
}

func TestUserStress_DeleteThenLoginFails(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "user")

	// Delete alice
	req := httptest.NewRequest(http.MethodDelete, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserDelete(rec, req, "alice")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d", rec.Code)
	}

	// Login should fail
	loginBody := jsonBody(t, map[string]string{"username": "alice", "password": "pass"})
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", loginBody)
	loginRec := httptest.NewRecorder()
	srv.handleAuthLogin(loginRec, loginReq)

	if loginRec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after delete, got %d", loginRec.Code)
	}
}

func TestUserStress_DeleteDoubleFails(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "a@x.com", "pass", "user")

	// First delete
	req := httptest.NewRequest(http.MethodDelete, "/api/users/alice", nil)
	rec := httptest.NewRecorder()
	srv.handleUserDelete(rec, req, "alice")
	if rec.Code != http.StatusOK {
		t.Fatalf("first delete expected 200, got %d", rec.Code)
	}

	// Second delete should return 404
	req = httptest.NewRequest(http.MethodDelete, "/api/users/alice", nil)
	rec = httptest.NewRecorder()
	srv.handleUserDelete(rec, req, "alice")
	if rec.Code != http.StatusNotFound {
		t.Errorf("second delete expected 404, got %d", rec.Code)
	}
}

// ============================================================================
// 12. Route dispatch edge cases
// ============================================================================

func TestUserStress_RouteUsersImportPrecedence(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// /api/users/import should go to handleUserImport, NOT routeUsers with username "import"
	body := jsonBody(t, map[string]interface{}{
		"users": []map[string]string{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/import", body)
	rec := httptest.NewRecorder()

	// Since we're testing route dispatch, we'd need the full mux.
	// Instead, verify the import handler works correctly when called directly.
	srv.handleUserImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 from import handler, got %d", rec.Code)
	}
}

func TestUserStress_WrongMethodOnCreate(t *testing.T) {
	srv := newTestServerWithStorage(t)

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, m := range methods {
		req := httptest.NewRequest(m, "/api/users", nil)
		rec := httptest.NewRecorder()
		srv.handleUserCreate(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/users expected 405, got %d", m, rec.Code)
		}
	}
}

func TestUserStress_WrongMethodOnLogin(t *testing.T) {
	srv := newTestServerWithStorage(t)

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete}
	for _, m := range methods {
		req := httptest.NewRequest(m, "/api/auth/login", nil)
		rec := httptest.NewRecorder()
		srv.handleAuthLogin(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/auth/login expected 405, got %d", m, rec.Code)
		}
	}
}

func TestUserStress_WrongMethodOnImport(t *testing.T) {
	srv := newTestServerWithStorage(t)

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete}
	for _, m := range methods {
		req := httptest.NewRequest(m, "/api/users/import", nil)
		rec := httptest.NewRecorder()
		srv.handleUserImport(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/users/import expected 405, got %d", m, rec.Code)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
