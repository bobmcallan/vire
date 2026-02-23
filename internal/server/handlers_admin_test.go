package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setAdminContext attaches a UserContext with the given role to the request.
func setAdminContext(t *testing.T, req *http.Request, userID, role string) *http.Request {
	t.Helper()
	uc := &common.UserContext{UserID: userID, Role: role}
	return req.WithContext(common.WithUserContext(req.Context(), uc))
}

// --- requireAdmin tests ---

func TestRequireAdmin_ContextRole(t *testing.T) {
	srv := newTestServerWithStorage(t)

	tests := []struct {
		name     string
		role     string
		wantOK   bool
		wantCode int
	}{
		{"admin_allowed", models.RoleAdmin, true, http.StatusOK},
		{"user_forbidden", models.RoleUser, false, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
			req = setAdminContext(t, req, "testuser", tt.role)
			rec := httptest.NewRecorder()

			result := srv.requireAdmin(rec, req)

			assert.Equal(t, tt.wantOK, result)
			if !tt.wantOK {
				assert.Equal(t, tt.wantCode, rec.Code)
			}
		})
	}
}

func TestRequireAdmin_FallbackDB_Admin(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	// No UserContext — fall back to DB lookup via X-Vire-User-ID header
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-Vire-User-ID", "admin1")
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	assert.True(t, result, "admin user via DB fallback should pass")
}

func TestRequireAdmin_FallbackDB_NonAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "regular", "regular@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-Vire-User-ID", "regular")
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireAdmin_NoAuth(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAdmin_UserNotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("X-Vire-User-ID", "nonexistent")
	rec := httptest.NewRecorder()

	result := srv.requireAdmin(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- handleAdminListUsers tests ---

func TestHandleAdminListUsers_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")
	createTestUser(t, srv, "user2", "user2@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	users := resp["users"].([]interface{})
	assert.Len(t, users, 3)
	assert.Equal(t, float64(3), resp["count"])

	// Verify all expected fields present, no password_hash
	expectedFields := []string{"id", "email", "name", "provider", "role", "created_at"}
	for _, u := range users {
		entry := u.(map[string]interface{})
		for _, field := range expectedFields {
			_, exists := entry[field]
			assert.True(t, exists, "expected field %q in user entry", field)
		}
		_, hasHash := entry["password_hash"]
		assert.False(t, hasHash, "password_hash must not appear in response")
	}
}

func TestHandleAdminListUsers_NoPasswordHashExposed(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "secretpass", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, "$2a$", "bcrypt hash must not appear in response")
	assert.NotContains(t, body, "password_hash", "password_hash field must not appear")
}

func TestHandleAdminListUsers_NonAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "user1", models.RoleUser)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleAdminListUsers_Unauthenticated(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleAdminListUsers_MethodNotAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", nil)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleAdminListUsers_SingleUser(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	users := resp["users"].([]interface{})
	assert.Len(t, users, 1)

	user := users[0].(map[string]interface{})
	assert.Equal(t, "admin1", user["id"])
	assert.Equal(t, "admin1@x.com", user["email"])
	assert.Equal(t, "admin", user["role"])
	assert.NotNil(t, user["created_at"])
}

// --- handleAdminUpdateUserRole tests ---

func TestHandleAdminUpdateUserRole_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user1")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "admin", resp["role"])
	assert.Equal(t, "user1", resp["id"])

	// No password_hash in response
	_, hasHash := resp["password_hash"]
	assert.False(t, hasHash, "password_hash must not appear in response")
}

func TestHandleAdminUpdateUserRole_DemoteOtherUser(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "admin2", "admin2@x.com", "pass", "admin")

	body := jsonBody(t, map[string]string{"role": "user"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/admin2/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "admin2")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "user", resp["role"])
}

func TestHandleAdminUpdateUserRole_InvalidRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	invalidRoles := []struct {
		name string
		role string
	}{
		{"empty_string", ""},
		{"superadmin", "superadmin"},
		{"root", "root"},
		{"capitalized_Admin", "Admin"},
		{"uppercase_USER", "USER"},
		{"with_leading_space", " admin"},
		{"with_trailing_space", "admin "},
	}

	for _, tt := range invalidRoles {
		t.Run(tt.name, func(t *testing.T) {
			body := jsonBody(t, map[string]string{"role": tt.role})
			req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
			req = setAdminContext(t, req, "admin1", models.RoleAdmin)
			rec := httptest.NewRecorder()
			srv.handleAdminUpdateUserRole(rec, req, "user1")

			assert.Equal(t, http.StatusBadRequest, rec.Code, "role %q should return 400", tt.role)
		})
	}
}

func TestHandleAdminUpdateUserRole_SelfDemotionBlocked(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	body := jsonBody(t, map[string]string{"role": "user"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/admin1/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "admin1")

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["error"], "Cannot remove your own admin role")

	// Verify role was NOT changed
	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "admin1")
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Role, "role should remain admin after blocked self-demotion")
}

func TestHandleAdminUpdateUserRole_SelfToAdminAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	// Setting own role to admin (no-op) should succeed
	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/admin1/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "admin1")

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestHandleAdminUpdateUserRole_RequiresAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")
	createTestUser(t, srv, "user2", "user2@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user2/role", body)
	req = setAdminContext(t, req, "user1", models.RoleUser)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user2")

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleAdminUpdateUserRole_Unauthenticated(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user1")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleAdminUpdateUserRole_TargetNotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/nonexistent/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "nonexistent")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleAdminUpdateUserRole_MethodNotAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/user1/role", nil)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user1")

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandleAdminUpdateUserRole_Persistence(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user1")

	require.Equal(t, http.StatusOK, rec.Code)

	// Verify via direct storage lookup
	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "user1")
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Role, "role change should be persisted")
}

// --- Role validation on user handlers ---

func TestHandleUserCreate_DefaultsToUserRole(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "pass",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, models.RoleUser, user.Role, "default role should be 'user'")
}

func TestHandleUserCreate_IgnoresRoleField(t *testing.T) {
	// User endpoints ignore the role field entirely.
	// Role changes must go through PATCH /api/admin/users/{id}/role.
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]string{
		"username": "alice",
		"password": "pass",
		"role":     "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users", body)
	rec := httptest.NewRecorder()
	srv.handleUserCreate(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, models.RoleUser, user.Role, "role should always be 'user' regardless of input")
}

func TestHandleUserUpsert_IgnoresRoleField(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"username": "newuser",
		"email":    "new@x.com",
		"password": "pass123",
		"role":     "admin",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/users/upsert", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpsert(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "newuser")
	require.NoError(t, err)
	assert.Equal(t, models.RoleUser, user.Role, "upsert should always assign 'user' role")
}

func TestHandleUserUpdate_IgnoresRoleField(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@x.com", "pass", "user")

	body := jsonBody(t, map[string]interface{}{
		"role": "admin",
	})
	req := httptest.NewRequest(http.MethodPut, "/api/users/alice", body)
	rec := httptest.NewRecorder()
	srv.handleUserUpdate(rec, req, "alice")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "alice")
	require.NoError(t, err)
	assert.Equal(t, models.RoleUser, user.Role, "role should not change via user update endpoint")
}

// --- JWT role claim ---

func TestSignJWT_IncludesRoleClaim(t *testing.T) {
	tests := []struct {
		name string
		role string
	}{
		{"admin", "admin"},
		{"user", "user"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &common.AuthConfig{
				JWTSecret:   "test-secret-key",
				TokenExpiry: "1h",
			}
			user := &models.InternalUser{
				UserID: "testuser",
				Email:  "test@example.com",
				Role:   tt.role,
			}

			token, err := signJWT(user, "email", cfg)
			require.NoError(t, err)

			_, claims, err := validateJWT(token, []byte(cfg.JWTSecret))
			require.NoError(t, err)
			assert.Equal(t, tt.role, claims["role"], "JWT should contain role claim")
		})
	}
}

// --- Middleware role population ---

func TestMiddleware_PopulatesRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:       "user-with-role",
		Email:        "u@x.com",
		PasswordHash: "hash",
		Role:         "admin",
	}))

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Vire-User-ID", "user-with-role")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NotNil(t, capturedUC, "UserContext should be populated")
	assert.Equal(t, "admin", capturedUC.Role, "role should be populated from storage")
}

func TestMiddleware_PopulatesUserRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:       "regular-user",
		Email:        "r@x.com",
		PasswordHash: "hash",
		Role:         "user",
	}))

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Vire-User-ID", "regular-user")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NotNil(t, capturedUC)
	assert.Equal(t, "user", capturedUC.Role)
}

// --- Route dispatch tests ---

func TestRouteAdminUsers_ListUsers(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.routeAdminUsers(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRouteAdminUsers_UpdateRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.routeAdminUsers(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestRouteAdminUsers_UnknownSubpath(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/user1/unknown", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.routeAdminUsers(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}
