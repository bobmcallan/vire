package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// --- Privilege escalation stress tests ---

func TestRoleEscalation_UserUpdate_MustNotAllowRoleChange(t *testing.T) {
	// A non-admin user must not be able to change their own role to admin
	// via PUT /api/users/{id}. This is the primary privilege escalation vector.
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "mallory", "m@evil.com", "pass", "user")

	body := jsonBody(t, map[string]interface{}{
		"role": "admin",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/mallory", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "mallory")

	// After this call, mallory must NOT be admin
	user, _ := srv.app.Storage.InternalStore().GetUser(req.Context(), "mallory")
	if user.Role == models.RoleAdmin {
		t.Error("CRITICAL: non-admin user was able to escalate to admin via PUT /api/users/{id}")
	}
}

func TestRoleEscalation_UserUpsert_MustNotAllowRoleChange(t *testing.T) {
	// A non-admin must not be able to change role via upsert
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "mallory", "m@evil.com", "pass", "user")

	body := jsonBody(t, map[string]interface{}{
		"username": "mallory",
		"role":     "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	user, _ := srv.app.Storage.InternalStore().GetUser(req.Context(), "mallory")
	if user.Role == models.RoleAdmin {
		t.Error("CRITICAL: non-admin user was able to escalate to admin via POST /api/users/upsert")
	}
}

func TestRoleEscalation_UserCreate_MustDefaultToUser(t *testing.T) {
	// New user creation must always result in role=user regardless of input
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "mallory",
		"password": "pass",
		"role":     "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	user, _ := srv.app.Storage.InternalStore().GetUser(req.Context(), "mallory")
	if user != nil && user.Role == models.RoleAdmin {
		t.Error("CRITICAL: unauthenticated user creation allowed admin role assignment")
	}
}

// --- Hostile input tests for role validation ---

func TestUpdateUserRole_HostileInputs(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	createTestUser(t, srv, "target", "t@x.com", "pass", "user")

	hostile := []struct {
		name string
		body string
	}{
		{"empty role", `{"role": ""}`},
		{"whitespace role", `{"role": "  "}`},
		{"case variation Admin", `{"role": "Admin"}`},
		{"case variation ADMIN", `{"role": "ADMIN"}`},
		{"superadmin", `{"role": "superadmin"}`},
		{"root", `{"role": "root"}`},
		{"sql injection", `{"role": "admin'; DROP TABLE users;--"}`},
		{"null bytes", `{"role": "admin\u0000"}`},
		{"very long string", `{"role": "` + strings.Repeat("a", 10000) + `"}`},
		{"unicode homoglyph", `{"role": "\u0430dmin"}`},
		{"newlines", `{"role": "admin\n"}`},
		{"tabs", `{"role": "\tadmin"}`},
		{"html injection", `{"role": "<script>alert('xss')</script>"}`},
		{"json injection", `{"role": "{\"nested\":true}"}`},
	}

	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/target/role",
				strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			uc := &common.UserContext{UserID: "admin1", Role: "admin"}
			req = req.WithContext(common.WithUserContext(req.Context(), uc))
			rec := httptest.NewRecorder()
			srv.handleAdminUpdateUserRole(rec, req, "target")

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400 for hostile input %q, got %d: %s",
					tc.name, rec.Code, rec.Body.String())
			}

			// Verify role was not changed
			user, _ := srv.app.Storage.InternalStore().GetUser(req.Context(), "target")
			if user.Role != "user" {
				t.Errorf("role was changed to %q by hostile input %q", user.Role, tc.name)
			}
		})
	}
}

// --- requireAdmin bypass attempts ---

func TestRequireAdmin_EmptyRoleInContext(t *testing.T) {
	// If UserContext has empty Role, should fall through to DB lookup
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "user1", "u@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	uc := &common.UserContext{UserID: "user1", Role: ""}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	req.Header.Set("X-Vire-User-ID", "user1")
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	if result {
		t.Error("requireAdmin should return false for non-admin user via DB fallback")
	}
}

func TestRequireAdmin_ForgedUserID(t *testing.T) {
	// Attacker sets X-Vire-User-ID to a non-existent user
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-Vire-User-ID", "nonexistent-admin")
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	if result {
		t.Error("requireAdmin should return false for non-existent user")
	}
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-existent user, got %d", rec.Code)
	}
}

func TestRequireAdmin_UserRoleInContextNotAdmin(t *testing.T) {
	// Even if X-Vire-User-ID points to a user, the context role matters
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	uc := &common.UserContext{UserID: "anyone", Role: "user"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	if result {
		t.Error("requireAdmin should return false for role=user in context")
	}
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rec.Code)
	}
}

// --- Self-demotion bypass attempts ---

func TestSelfDemotion_CaseVariation(t *testing.T) {
	// User IDs are case-sensitive, so this tests that exact match is required
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "Admin1", "a@x.com", "pass", "admin")

	body := jsonBody(t, map[string]string{"role": "user"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/Admin1/role", body)
	uc := &common.UserContext{UserID: "Admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "Admin1")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for self-demotion, got %d", rec.Code)
	}
}

func TestSelfDemotion_ViaHeaderFallback(t *testing.T) {
	// Self-demotion check should also work via X-Vire-User-ID header
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	body := jsonBody(t, map[string]string{"role": "user"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/admin1/role", body)
	// No UserContext, but header is set
	req.Header.Set("X-Vire-User-ID", "admin1")
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "admin1")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for self-demotion via header, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- List users data leak tests ---

func TestListUsers_NeverLeaksPasswordHash(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "u@x.com", "secretpassword123", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	body := rec.Body.String()

	// Must not contain bcrypt hash prefix
	if strings.Contains(body, "$2a$") {
		t.Error("list users response contains bcrypt hash")
	}
	if strings.Contains(body, "password_hash") {
		t.Error("list users response contains password_hash field")
	}
	if strings.Contains(body, "password") {
		t.Error("list users response contains 'password' field")
	}
}

func TestListUsers_NeverLeaksNavexaKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	ctx := context.Background()
	srv.app.Storage.InternalStore().SetUserKV(ctx, "admin1", "navexa_key", "nk-secret-12345")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "nk-secret-12345") {
		t.Error("list users response leaks navexa_key")
	}
	if strings.Contains(body, "navexa") {
		t.Error("list users response contains navexa field")
	}
}

// --- Empty state tests ---

func TestListUsers_EmptyDatabase(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// Need an admin to call this endpoint, so we have to create one
	// But test what happens if ListUsers returns empty after admin creation
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["count"] == nil {
		t.Error("count field is missing")
	}
	if resp["users"] == nil {
		t.Error("users field is missing")
	}
}

// --- Method enforcement ---

func TestUpdateUserRole_WrongMethod(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/admin/users/user1/role", nil)
		uc := &common.UserContext{UserID: "admin1", Role: "admin"}
		req = req.WithContext(common.WithUserContext(req.Context(), uc))
		rec := httptest.NewRecorder()
		srv.handleAdminUpdateUserRole(rec, req, "user1")

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rec.Code)
		}
	}
}

func TestListUsers_WrongMethod(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	methods := []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/admin/users", nil)
		uc := &common.UserContext{UserID: "admin1", Role: "admin"}
		req = req.WithContext(common.WithUserContext(req.Context(), uc))
		rec := httptest.NewRecorder()
		srv.handleAdminListUsers(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rec.Code)
		}
	}
}

// --- Route dispatch edge cases ---

func TestRouteAdminUsers_EmptyUserID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	// Path: /api/admin/users/ (trailing slash, empty user ID)
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users//role", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.routeAdminUsers(rec, req)

	// Empty user ID in path should either 404 or 400, not panic
	if rec.Code == http.StatusOK {
		t.Error("expected error for empty user ID in path")
	}
}

func TestRouteAdminUsers_PathTraversal(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	// Attempt path traversal
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/../../../etc/passwd", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.routeAdminUsers(rec, req)

	// Should not return 200 or expose any file system data
	if rec.Code == http.StatusOK {
		t.Error("path traversal should not return 200")
	}
}

// --- Missing body tests ---

func TestUpdateUserRole_EmptyBody(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	createTestUser(t, srv, "target", "t@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/target/role",
		strings.NewReader(""))
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "target")

	if rec.Code == http.StatusOK {
		t.Error("empty body should not succeed")
	}
}

func TestUpdateUserRole_MalformedJSON(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	createTestUser(t, srv, "target", "t@x.com", "pass", "user")

	malformed := []string{
		`{role: admin}`,
		`not json at all`,
		`{"role": `,
		`null`,
		`[]`,
		`{"role": 42}`,
		`{"role": true}`,
		`{"role": null}`,
	}

	for _, body := range malformed {
		req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/target/role",
			strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		uc := &common.UserContext{UserID: "admin1", Role: "admin"}
		req = req.WithContext(common.WithUserContext(req.Context(), uc))
		rec := httptest.NewRecorder()
		srv.handleAdminUpdateUserRole(rec, req, "target")

		if rec.Code == http.StatusOK {
			t.Errorf("malformed body %q should not succeed", body)
		}

		// Verify role was not changed
		user, _ := srv.app.Storage.InternalStore().GetUser(req.Context(), "target")
		if user.Role != "user" {
			t.Errorf("role changed to %q after malformed input %q", user.Role, body)
		}
	}
}

// --- JWT role claim validation ---

func TestSignJWT_EmptyRole(t *testing.T) {
	cfg := &common.AuthConfig{
		JWTSecret:   "test-secret",
		TokenExpiry: "1h",
	}
	user := &models.InternalUser{
		UserID: "noroleman",
		Email:  "n@x.com",
		Role:   "",
	}

	token, err := signJWT(user, "email", cfg)
	if err != nil {
		t.Fatalf("signJWT failed: %v", err)
	}

	_, claims, err := validateJWT(token, []byte(cfg.JWTSecret))
	if err != nil {
		t.Fatalf("validateJWT failed: %v", err)
	}

	// Empty role should be present as empty string, not cause a panic
	role, ok := claims["role"].(string)
	if !ok {
		t.Error("role claim should be a string even when empty")
	}
	if role != "" {
		t.Errorf("expected empty role claim, got %q", role)
	}
}
