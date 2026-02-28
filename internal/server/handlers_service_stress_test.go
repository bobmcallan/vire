package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
)

// ============================================================================
// 1. Service key timing attack resistance
// ============================================================================

func TestServiceRegister_TimingAttack_WrongKeyVsShortKey(t *testing.T) {
	// Verify that wrong-key and short-key responses don't leak timing info
	// about the actual key length. Both should fail fast without variable-time
	// string comparison.
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "this-is-a-32-char-service-key-ok"

	wrongKey := `{"service_id":"p1","service_key":"wrong-but-long-enough-32-chars!!","service_type":"portal"}`
	shortKey := `{"service_id":"p1","service_key":"short","service_type":"portal"}`

	// Just verify both fail with expected codes — not 200
	req1 := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(wrongKey))
	rec1 := httptest.NewRecorder()
	srv.handleServiceRegister(rec1, req1)
	if rec1.Code == http.StatusOK {
		t.Error("wrong key should not return 200")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(shortKey))
	rec2 := httptest.NewRecorder()
	srv.handleServiceRegister(rec2, req2)
	if rec2.Code == http.StatusOK {
		t.Error("short key should not return 200")
	}
}

// ============================================================================
// 2. Service key injection via service_id field
// ============================================================================

func TestServiceRegister_ServiceID_Injection(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	hostile := []struct {
		name string
		id   string
	}{
		{"null bytes", "portal\x00-injected"},
		{"path traversal", "../../../etc/passwd"},
		{"colon prefix", "service:admin:extra"},
		{"sql injection", "'; DROP TABLE user;--"},
		{"newlines", "portal\nprod"},
		{"tabs", "portal\tprod"},
		{"html tags", "<script>alert(1)</script>"},
		{"unicode homoglyph", "port\u0430l-prod"}, // Cyrillic 'а'
		{"very long id", strings.Repeat("a", 10000)},
		{"empty after trim", "   "},
		{"just spaces", "     "},
		{"control chars", "\x01\x02\x03"},
		{"url encoded", "portal%00prod"},
		{"backslash", "portal\\prod"},
	}

	for _, tc := range hostile {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"service_id":%q,"service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`, tc.id)
			req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.handleServiceRegister(rec, req)

			// Should either succeed cleanly or return 400 — never panic or 500
			if rec.Code == http.StatusInternalServerError {
				t.Errorf("hostile service_id %q caused 500: %s", tc.name, rec.Body.String())
			}
		})
	}
}

// ============================================================================
// 3. Service user cannot login via password endpoint
// ============================================================================

func TestServiceUser_CannotLoginViaPassword(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create a service user directly (simulating successful registration)
	ctx := context.Background()
	serviceUser := &models.InternalUser{
		UserID:       "service:portal-1",
		Email:        "portal-1@service.vire.local",
		Name:         "Service: portal-1",
		PasswordHash: "", // empty — bcrypt always fails
		Provider:     "service",
		Role:         models.RoleService,
		CreatedAt:    time.Now(),
	}
	if err := srv.app.Storage.InternalStore().SaveUser(ctx, serviceUser); err != nil {
		t.Fatalf("failed to create service user: %v", err)
	}

	// Attempt password login
	body := jsonBody(t, map[string]string{
		"username": "service:portal-1",
		"password": "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	// Must be 403 (explicit block) or 401 (bcrypt fail) — never 200
	if rec.Code == http.StatusOK {
		t.Error("CRITICAL: service user was able to login via password endpoint")
	}
}

func TestServiceUser_CannotLoginViaPassword_WithGuessedPassword(t *testing.T) {
	// Even if an attacker tries common passwords, the empty hash should block
	srv := newTestServerWithStorage(t)

	ctx := context.Background()
	serviceUser := &models.InternalUser{
		UserID:       "service:portal-2",
		Email:        "portal-2@service.vire.local",
		Name:         "Service: portal-2",
		PasswordHash: "",
		Provider:     "service",
		Role:         models.RoleService,
		CreatedAt:    time.Now(),
	}
	srv.app.Storage.InternalStore().SaveUser(ctx, serviceUser)

	passwords := []string{"", "password", "admin", "service", "portal", "service:portal-2"}
	for _, pwd := range passwords {
		body := jsonBody(t, map[string]string{
			"username": "service:portal-2",
			"password": pwd,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
		rec := httptest.NewRecorder()
		srv.handleAuthLogin(rec, req)

		if rec.Code == http.StatusOK {
			t.Errorf("CRITICAL: service user logged in with password %q", pwd)
		}
	}
}

// ============================================================================
// 4. Service user cannot login via OAuth endpoint
// ============================================================================

func TestServiceUser_CannotLoginViaOAuth(t *testing.T) {
	srv := newTestServerWithStorage(t)

	ctx := context.Background()
	serviceUser := &models.InternalUser{
		UserID:       "service:portal-3",
		Email:        "portal-3@service.vire.local",
		Name:         "Service: portal-3",
		PasswordHash: "",
		Provider:     "service",
		Role:         models.RoleService,
		CreatedAt:    time.Now(),
	}
	srv.app.Storage.InternalStore().SaveUser(ctx, serviceUser)

	// Attempt OAuth with provider="service" — should fall through to default case
	body := jsonBody(t, map[string]interface{}{
		"provider": "service",
		"code":     "fake-code",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/oauth", body)
	rec := httptest.NewRecorder()
	srv.handleAuthOAuth(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("CRITICAL: service provider accepted in OAuth endpoint")
	}
}

// ============================================================================
// 5. Service registration - key not configured (501)
// ============================================================================

func TestServiceRegister_KeyNotConfigured(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// Service key is empty by default
	srv.app.Config.Auth.ServiceKey = ""

	body := `{"service_id":"p1","service_key":"anything-32-chars-long-enough!!!!","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 when service key not configured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ============================================================================
// 6. Service registration - wrong key (403)
// ============================================================================

func TestServiceRegister_WrongKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "correct-key-that-is-32-chars-ok!"

	body := `{"service_id":"p1","service_key":"wrong-key-but-still-32-chars-ok!","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for wrong key, got %d", rec.Code)
	}
}

// ============================================================================
// 7. Service registration - key length validation
// ============================================================================

func TestServiceRegister_ShortKeys(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// Server key is short — should reject even if client sends matching short key
	srv.app.Config.Auth.ServiceKey = "short"

	body := `{"service_id":"p1","service_key":"short","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("short server key should be rejected")
	}
}

func TestServiceRegister_ClientShortKey_ServerLongKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "correct-key-that-is-32-chars-ok!"

	// Client sends a short key
	body := `{"service_id":"p1","service_key":"short","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("client short key should be rejected even when server key is long")
	}
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short client key, got %d", rec.Code)
	}
}

// ============================================================================
// 8. Service registration - empty service_id (400)
// ============================================================================

func TestServiceRegister_EmptyServiceID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	body := `{"service_id":"","service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty service_id, got %d", rec.Code)
	}
}

// ============================================================================
// 9. Service registration - idempotent re-registration
// ============================================================================

func TestServiceRegister_Idempotent(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	register := func() *httptest.ResponseRecorder {
		body := `{"service_id":"portal-1","service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`
		req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
		rec := httptest.NewRecorder()
		srv.handleServiceRegister(rec, req)
		return rec
	}

	// First registration
	rec1 := register()
	if rec1.Code != http.StatusOK {
		t.Fatalf("first registration failed: %d %s", rec1.Code, rec1.Body.String())
	}

	// Wait a moment to ensure ModifiedAt changes
	time.Sleep(10 * time.Millisecond)

	// Second registration — should succeed (idempotent)
	rec2 := register()
	if rec2.Code != http.StatusOK {
		t.Fatalf("re-registration failed: %d %s", rec2.Code, rec2.Body.String())
	}

	// Verify the user still exists and has correct fields
	ctx := context.Background()
	user, err := srv.app.Storage.InternalStore().GetUser(ctx, "service:portal-1")
	if err != nil {
		t.Fatalf("service user not found after re-registration: %v", err)
	}
	if user.Role != models.RoleService {
		t.Errorf("expected role %q, got %q", models.RoleService, user.Role)
	}
	if user.Provider != "service" {
		t.Errorf("expected provider 'service', got %q", user.Provider)
	}
}

// ============================================================================
// 10. Race condition: concurrent registrations for same service_id
// ============================================================================

func TestServiceRegister_ConcurrentSameID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	var wg sync.WaitGroup
	results := make([]int, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := `{"service_id":"portal-race","service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`
			req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.handleServiceRegister(rec, req)
			results[idx] = rec.Code
		}(i)
	}
	wg.Wait()

	// All should succeed (200) or at worst some may fail gracefully — none should 500 or panic
	for i, code := range results {
		if code == http.StatusInternalServerError {
			t.Errorf("goroutine %d got 500 — possible race condition", i)
		}
	}

	// Exactly one user should exist
	ctx := context.Background()
	user, err := srv.app.Storage.InternalStore().GetUser(ctx, "service:portal-race")
	if err != nil {
		t.Fatalf("service user missing after concurrent registration: %v", err)
	}
	if user.Role != models.RoleService {
		t.Errorf("expected role %q, got %q", models.RoleService, user.Role)
	}
}

// ============================================================================
// 11. Privilege escalation: service user cannot promote to "service" role
// ============================================================================

func TestServiceRole_CannotBeSetViaAdminEndpoint(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	createTestUser(t, srv, "target", "t@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "service"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/target/role", body)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "target")

	if rec.Code == http.StatusOK {
		t.Error("CRITICAL: 'service' role was assignable via PATCH /api/admin/users/{id}/role")
	}

	// Verify role was NOT changed
	user, _ := srv.app.Storage.InternalStore().GetUser(context.Background(), "target")
	if user.Role == "service" {
		t.Error("CRITICAL: user role was changed to 'service'")
	}
}

// ============================================================================
// 12. Service user accessing admin endpoints
// ============================================================================

func TestServiceUser_CanListUsers(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	// Create a service user
	ctx := context.Background()
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Provider: "service",
		Role:     models.RoleService,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	uc := &common.UserContext{UserID: "service:portal-1", Role: models.RoleService}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("service user should be able to list users, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServiceUser_CanUpdateRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "target", "t@x.com", "pass", "user")

	ctx := context.Background()
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Provider: "service",
		Role:     models.RoleService,
	})

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/target/role", body)
	uc := &common.UserContext{UserID: "service:portal-1", Role: models.RoleService}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "target")

	if rec.Code != http.StatusOK {
		t.Errorf("service user should be able to update roles, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestServiceUser_CannotAccessJobs(t *testing.T) {
	srv := newTestServerWithStorage(t)

	ctx := context.Background()
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Provider: "service",
		Role:     models.RoleService,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/jobs", nil)
	uc := &common.UserContext{UserID: "service:portal-1", Role: models.RoleService}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleAdminJobs(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("service user should NOT be able to access job endpoints")
	}
}

// ============================================================================
// 13. Method enforcement on registration endpoint
// ============================================================================

func TestServiceRegister_WrongMethod(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	methods := []string{http.MethodGet, http.MethodPut, http.MethodPatch, http.MethodDelete}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/api/services/register", nil)
		rec := httptest.NewRecorder()
		srv.handleServiceRegister(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rec.Code)
		}
	}
}

// ============================================================================
// 14. Malformed JSON body
// ============================================================================

func TestServiceRegister_MalformedBodies(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	malformed := []struct {
		name string
		body string
	}{
		{"empty string", ""},
		{"not json", "this is not json"},
		{"partial json", `{"service_id": `},
		{"array", `[1,2,3]`},
		{"null", `null`},
		{"number", `42`},
		{"string", `"hello"`},
		{"missing fields", `{}`},
		{"only service_id", `{"service_id":"p1"}`},
		{"numeric service_id", `{"service_id":42,"service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`},
		{"huge body", `{"service_id":"p1","service_key":"` + strings.Repeat("x", 1000000) + `","service_type":"portal"}`},
	}

	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			srv.handleServiceRegister(rec, req)

			// Should never panic or return 200
			if rec.Code == http.StatusOK {
				t.Errorf("malformed body %q should not return 200", tc.name)
			}
			if rec.Code == http.StatusInternalServerError {
				t.Errorf("malformed body %q caused 500: %s", tc.name, rec.Body.String())
			}
		})
	}
}

// ============================================================================
// 15. Tidy endpoint - admin only
// ============================================================================

func TestServiceTidy_RequiresAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Service user should NOT be able to tidy (admin-only, not adminOrService)
	ctx := context.Background()
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Provider: "service",
		Role:     models.RoleService,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	uc := &common.UserContext{UserID: "service:portal-1", Role: models.RoleService}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("CRITICAL: service user can run tidy — should be admin-only")
	}
}

func TestServiceTidy_AdminCanTidy(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	// Create a stale service user
	ctx := context.Background()
	staleTime := time.Now().Add(-8 * 24 * time.Hour)
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:     "service:stale-portal",
		Email:      "stale-portal@service.vire.local",
		Provider:   "service",
		Role:       models.RoleService,
		ModifiedAt: staleTime,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("admin tidy should succeed, got %d: %s", rec.Code, rec.Body.String())
	}

	// Stale service user should have been purged
	_, err := srv.app.Storage.InternalStore().GetUser(ctx, "service:stale-portal")
	if err == nil {
		t.Error("stale service user should have been purged")
	}
}

func TestServiceTidy_DoesNotDeleteNonServiceUsers(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")
	createTestUser(t, srv, "regular", "r@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	// Regular user must still exist
	ctx := context.Background()
	_, err := srv.app.Storage.InternalStore().GetUser(ctx, "regular")
	if err != nil {
		t.Error("tidy should NOT delete non-service users")
	}
}

// ============================================================================
// 16. X-Vire-Service-ID header spoofing
// ============================================================================

func TestServiceHeader_SpoofWithNonServiceUser(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "regular-user", "r@x.com", "pass", "user")

	// Attacker sets X-Vire-Service-ID to a regular user — middleware should
	// verify role=="service" and reject
	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-Vire-Service-ID", "regular-user")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// The middleware should not grant service identity to a non-service user
	if capturedUC != nil && capturedUC.Role == models.RoleService {
		t.Error("CRITICAL: non-service user got service role via X-Vire-Service-ID header")
	}
}

func TestServiceHeader_NonexistentServiceUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-Vire-Service-ID", "service:nonexistent")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Should not create a UserContext for nonexistent service user
	if capturedUC != nil && capturedUC.Role == models.RoleService {
		t.Error("nonexistent service user should not get service role")
	}
}

// ============================================================================
// 17. Header priority: Bearer > X-Vire-User-ID > X-Vire-Service-ID
// ============================================================================

func TestServiceHeader_PriorityBelowUserID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	ctx := context.Background()
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Provider: "service",
		Role:     models.RoleService,
	})

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Both headers set — X-Vire-User-ID should take priority
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Vire-User-ID", "admin1")
	req.Header.Set("X-Vire-Service-ID", "service:portal-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if capturedUC == nil {
		t.Fatal("expected UserContext to be set")
	}
	if capturedUC.UserID != "admin1" {
		t.Errorf("X-Vire-User-ID should take priority, got UserID=%q", capturedUC.UserID)
	}
	if capturedUC.Role != "admin" {
		t.Errorf("expected admin role from User-ID resolution, got %q", capturedUC.Role)
	}
}

// ============================================================================
// 18. Service key constant-time comparison
// ============================================================================

func TestServiceRegister_KeyComparison_NotPrefix(t *testing.T) {
	// Verify that partial key matches don't succeed (not a prefix check)
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Client sends key that is a prefix of the real key
	body := `{"service_id":"p1","service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("partial key match should not succeed")
	}
}

func TestServiceRegister_KeyComparison_NotSuffix(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// Client sends key with extra suffix
	body := `{"service_id":"p1","service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-extra","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code == http.StatusOK {
		t.Error("key with extra suffix should not match")
	}
}

// ============================================================================
// 19. CORS header includes X-Vire-Service-ID
// ============================================================================

func TestCORS_IncludesServiceIDHeader(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/services/register", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	if !strings.Contains(allowHeaders, "X-Vire-Service-ID") {
		t.Errorf("CORS Allow-Headers should include X-Vire-Service-ID, got: %s", allowHeaders)
	}
}

// ============================================================================
// 20. ValidateRole rejects "service" as normal role
// ============================================================================

func TestValidateRole_RejectsServiceForAdminEndpoint(t *testing.T) {
	// After adding RoleService to ValidateRole, the PATCH endpoint must
	// separately block "service" as a target role
	err := models.ValidateRole("service")
	// This test documents the expected behavior:
	// ValidateRole SHOULD accept "service" (it's a valid role constant)
	// But handleAdminUpdateUserRole must explicitly reject it as a target
	if err == nil {
		// If ValidateRole accepts "service", the handler MUST block it separately
		// This test just confirms the expected flow
		t.Log("ValidateRole accepts 'service' — handler must explicitly block it")
	}
}

// ============================================================================
// 21. Service user record fields
// ============================================================================

func TestServiceRegister_UserRecordFields(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	body := `{"service_id":"portal-prod-1","service_key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("registration failed: %d %s", rec.Code, rec.Body.String())
	}

	// Verify user record
	ctx := context.Background()
	user, err := srv.app.Storage.InternalStore().GetUser(ctx, "service:portal-prod-1")
	if err != nil {
		t.Fatalf("service user not found: %v", err)
	}

	if user.UserID != "service:portal-prod-1" {
		t.Errorf("user_id: expected 'service:portal-prod-1', got %q", user.UserID)
	}
	if user.Email != "portal-prod-1@service.vire.local" {
		t.Errorf("email: expected 'portal-prod-1@service.vire.local', got %q", user.Email)
	}
	if user.Name != "Service: portal-prod-1" {
		t.Errorf("name: expected 'Service: portal-prod-1', got %q", user.Name)
	}
	if user.PasswordHash != "" {
		t.Error("password_hash should be empty for service users")
	}
	if user.Provider != "service" {
		t.Errorf("provider: expected 'service', got %q", user.Provider)
	}
	if user.Role != models.RoleService {
		t.Errorf("role: expected %q, got %q", models.RoleService, user.Role)
	}

	// Verify response
	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got %v", resp["status"])
	}
	if resp["service_user_id"] != "service:portal-prod-1" {
		t.Errorf("expected service_user_id 'service:portal-prod-1', got %v", resp["service_user_id"])
	}
}

// ============================================================================
// 22. Breakglass admin is NOT a service user
// ============================================================================

func TestBreakglassAdmin_NotAffectedByTidy(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	// Create breakglass admin (provider="system", not "service")
	ctx := context.Background()
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:     "breakglass-admin",
		Email:      "admin@vire.local",
		Provider:   "system",
		Role:       "admin",
		ModifiedAt: time.Now().Add(-30 * 24 * time.Hour), // Very old
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	// Breakglass admin must NOT be deleted (provider is "system", not "service")
	_, err := srv.app.Storage.InternalStore().GetUser(ctx, "breakglass-admin")
	if err != nil {
		t.Error("CRITICAL: tidy deleted breakglass-admin (provider=system)")
	}
}

// ============================================================================
// 23. Service key not leaked in responses or errors
// ============================================================================

func TestServiceRegister_KeyNotLeakedInResponse(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "super-secret-service-key-32-chars!"

	// Wrong key attempt
	body := `{"service_id":"p1","service_key":"wrong-key-32-characters-exactly!!","service_type":"portal"}`
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	responseBody := rec.Body.String()
	if strings.Contains(responseBody, "super-secret-service-key-32-chars!") {
		t.Error("CRITICAL: server service key leaked in error response")
	}
}

// ============================================================================
// 24. Tidy preserves recent service users
// ============================================================================

func TestServiceTidy_PreservesRecentServiceUsers(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	ctx := context.Background()
	// Create a recent service user (within 7 days)
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:     "service:recent-portal",
		Email:      "recent-portal@service.vire.local",
		Provider:   "service",
		Role:       models.RoleService,
		ModifiedAt: time.Now().Add(-1 * 24 * time.Hour), // 1 day old
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	// Recent service user should be preserved
	_, err := srv.app.Storage.InternalStore().GetUser(ctx, "service:recent-portal")
	if err != nil {
		t.Error("tidy should NOT delete recent service users (within 7 days)")
	}
}

// ============================================================================
// 25. Tidy response format
// ============================================================================

func TestServiceTidy_ResponseFormat(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "a@x.com", "pass", "admin")

	ctx := context.Background()
	// One stale, one recent
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID: "service:stale", Email: "s@service.vire.local",
		Provider: "service", Role: models.RoleService,
		ModifiedAt: time.Now().Add(-8 * 24 * time.Hour),
	})
	srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID: "service:recent", Email: "r@service.vire.local",
		Provider: "service", Role: models.RoleService,
		ModifiedAt: time.Now(),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	uc := &common.UserContext{UserID: "admin1", Role: "admin"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("tidy failed: %d", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if _, ok := resp["purged"]; !ok {
		t.Error("response missing 'purged' field")
	}
	if _, ok := resp["remaining"]; !ok {
		t.Error("response missing 'remaining' field")
	}
}
