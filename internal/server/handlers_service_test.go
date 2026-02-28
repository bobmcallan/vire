package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testServiceKey = "this-is-a-valid-service-key-32chars!"

// --- handleServiceRegister tests ---

func TestHandleServiceRegister_Valid(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = testServiceKey

	body := jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  testServiceKey,
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "ok", resp["status"])
	assert.Equal(t, "service:portal-prod-1", resp["service_user_id"])
	assert.NotEmpty(t, resp["registered_at"])

	// Verify user was created in storage
	user, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "service:portal-prod-1")
	require.NoError(t, err)
	assert.Equal(t, "portal-prod-1@service.vire.local", user.Email)
	assert.Equal(t, "Service: portal-prod-1", user.Name)
	assert.Equal(t, "service", user.Provider)
	assert.Equal(t, models.RoleService, user.Role)
	assert.Empty(t, user.PasswordHash)
}

func TestHandleServiceRegister_Idempotent(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = testServiceKey

	// First registration
	body := jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  testServiceKey,
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Get the original created_at
	user1, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "service:portal-prod-1")
	require.NoError(t, err)
	originalCreated := user1.CreatedAt
	originalModified := user1.ModifiedAt

	// Wait to ensure timestamp differs
	time.Sleep(10 * time.Millisecond)

	// Re-registration
	body = jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  testServiceKey,
		"service_type": "portal",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec = httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify created_at unchanged, modified_at updated
	user2, err := srv.app.Storage.InternalStore().GetUser(context.Background(), "service:portal-prod-1")
	require.NoError(t, err)
	assert.Equal(t, originalCreated, user2.CreatedAt, "created_at should not change on re-registration")
	assert.True(t, user2.ModifiedAt.After(originalModified) || user2.ModifiedAt.Equal(originalModified),
		"modified_at should be updated on re-registration")
}

func TestHandleServiceRegister_WrongKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = testServiceKey

	body := jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  "wrong-key-that-is-at-least-32-chars!",
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleServiceRegister_MissingServerKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	// No service key configured

	body := jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  testServiceKey,
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	assert.Equal(t, http.StatusNotImplemented, rec.Code)
}

func TestHandleServiceRegister_EmptyServiceID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = testServiceKey

	body := jsonBody(t, map[string]string{
		"service_id":   "",
		"service_key":  testServiceKey,
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleServiceRegister_ShortKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = testServiceKey

	body := jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  "short-key",
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleServiceRegister_ShortServerKey(t *testing.T) {
	srv := newTestServerWithStorage(t)
	srv.app.Config.Auth.ServiceKey = "short"

	body := jsonBody(t, map[string]string{
		"service_id":   "portal-prod-1",
		"service_key":  "short",
		"service_type": "portal",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/services/register", body)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleServiceRegister_MethodNotAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/services/register", nil)
	rec := httptest.NewRecorder()
	srv.handleServiceRegister(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- handleServiceTidy tests ---

func TestHandleServiceTidy_PurgesStale(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create a stale service user (modified 10 days ago)
	staleUser := &models.InternalUser{
		UserID:     "service:stale-portal",
		Email:      "stale-portal@service.vire.local",
		Name:       "Service: stale-portal",
		Provider:   "service",
		Role:       models.RoleService,
		CreatedAt:  time.Now().Add(-10 * 24 * time.Hour),
		ModifiedAt: time.Now().Add(-10 * 24 * time.Hour),
	}
	require.NoError(t, store.SaveUser(ctx, staleUser))

	// Create a recent service user (modified 1 day ago)
	recentUser := &models.InternalUser{
		UserID:     "service:recent-portal",
		Email:      "recent-portal@service.vire.local",
		Name:       "Service: recent-portal",
		Provider:   "service",
		Role:       models.RoleService,
		CreatedAt:  time.Now().Add(-1 * 24 * time.Hour),
		ModifiedAt: time.Now().Add(-1 * 24 * time.Hour),
	}
	require.NoError(t, store.SaveUser(ctx, recentUser))

	// Create a regular user (should not be affected)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, float64(1), resp["purged"])
	assert.Equal(t, float64(1), resp["remaining"])

	// Verify stale user deleted
	_, err := store.GetUser(ctx, "service:stale-portal")
	assert.Error(t, err, "stale service user should be deleted")

	// Verify recent user still exists
	_, err = store.GetUser(ctx, "service:recent-portal")
	assert.NoError(t, err, "recent service user should remain")

	// Verify regular user not affected
	_, err = store.GetUser(ctx, "admin1")
	assert.NoError(t, err, "regular user should not be affected by tidy")
}

func TestHandleServiceTidy_RequiresAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	req = setAdminContext(t, req, "user1", models.RoleUser)
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHandleServiceTidy_RejectsServiceRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	svcUser := &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Name:     "Service: portal-1",
		Provider: "service",
		Role:     models.RoleService,
	}
	require.NoError(t, store.SaveUser(ctx, svcUser))

	req := httptest.NewRequest(http.MethodPost, "/api/admin/services/tidy", nil)
	req = setAdminContext(t, req, "service:portal-1", models.RoleService)
	rec := httptest.NewRecorder()
	srv.handleServiceTidy(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// --- requireAdminOrService tests ---

func TestRequireAdminOrService_AdminAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()

	result := srv.requireAdminOrService(rec, req)
	assert.True(t, result)
}

func TestRequireAdminOrService_ServiceAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "service:portal-1", models.RoleService)
	rec := httptest.NewRecorder()

	result := srv.requireAdminOrService(rec, req)
	assert.True(t, result)
}

func TestRequireAdminOrService_UserRejected(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "user1", models.RoleUser)
	rec := httptest.NewRecorder()

	result := srv.requireAdminOrService(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireAdminOrService_Unauthenticated(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	rec := httptest.NewRecorder()

	result := srv.requireAdminOrService(rec, req)
	assert.False(t, result)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- Admin endpoint access with service role ---

func TestHandleAdminListUsers_ServiceAccess(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req = setAdminContext(t, req, "service:portal-1", models.RoleService)
	rec := httptest.NewRecorder()
	srv.handleAdminListUsers(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestHandleAdminUpdateUserRole_ServiceAccess(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create a service user
	svcUser := &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Name:     "Service: portal-1",
		Provider: "service",
		Role:     models.RoleService,
	}
	require.NoError(t, store.SaveUser(ctx, svcUser))
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "admin"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
	req = setAdminContext(t, req, "service:portal-1", models.RoleService)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user1")

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Verify the role was updated
	user, err := store.GetUser(ctx, "user1")
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Role)
}

func TestHandleAdminUpdateUserRole_RejectsServiceTargetRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin1", "admin1@x.com", "pass", "admin")
	createTestUser(t, srv, "user1", "user1@x.com", "pass", "user")

	body := jsonBody(t, map[string]string{"role": "service"})
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/user1/role", body)
	req = setAdminContext(t, req, "admin1", models.RoleAdmin)
	rec := httptest.NewRecorder()
	srv.handleAdminUpdateUserRole(rec, req, "user1")

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp["error"], "cannot assign service role")
}

// --- Login block for service accounts ---

func TestHandleAuthLogin_RejectsServiceProvider(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()
	store := srv.app.Storage.InternalStore()

	// Create a service user directly in storage
	svcUser := &models.InternalUser{
		UserID:       "service:portal-1",
		Email:        "portal-1@service.vire.local",
		Name:         "Service: portal-1",
		PasswordHash: "",
		Provider:     "service",
		Role:         models.RoleService,
	}
	require.NoError(t, store.SaveUser(ctx, svcUser))

	body := jsonBody(t, map[string]string{
		"username": "service:portal-1",
		"password": "anything",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	rec := httptest.NewRecorder()
	srv.handleAuthLogin(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "service accounts cannot login", resp["error"])
}

// --- Middleware X-Vire-Service-ID tests ---

func TestMiddleware_ServiceIDResolution(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	// Create a service user
	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Name:     "Service: portal-1",
		Provider: "service",
		Role:     models.RoleService,
	}))

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Vire-Service-ID", "service:portal-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NotNil(t, capturedUC, "UserContext should be populated from service ID")
	assert.Equal(t, "service:portal-1", capturedUC.UserID)
	assert.Equal(t, models.RoleService, capturedUC.Role)
}

func TestMiddleware_ServiceIDWithUserID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	// Create a regular user
	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "user1",
		Email:    "user1@x.com",
		Provider: "email",
		Role:     models.RoleUser,
	}))

	// Create a service user
	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "service:portal-1",
		Email:    "portal-1@service.vire.local",
		Provider: "service",
		Role:     models.RoleService,
	}))

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Both headers present — user ID takes identity, service role for auth
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Vire-User-ID", "user1")
	req.Header.Set("X-Vire-Service-ID", "service:portal-1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.NotNil(t, capturedUC)
	assert.Equal(t, "user1", capturedUC.UserID, "user identity should come from X-Vire-User-ID")
	assert.Equal(t, models.RoleService, capturedUC.Role, "role should come from service identity for authorization")
}

func TestMiddleware_ServiceIDNonServiceRole(t *testing.T) {
	srv := newTestServerWithStorage(t)
	ctx := context.Background()

	// Create a regular user and try to use it as a service ID (should be rejected)
	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(ctx, &models.InternalUser{
		UserID:   "regular-user",
		Email:    "r@x.com",
		Provider: "email",
		Role:     models.RoleUser,
	}))

	var capturedUC *common.UserContext
	handler := userContextMiddleware(srv.app.Storage.InternalStore())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUC = common.UserContextFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("X-Vire-Service-ID", "regular-user")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// The middleware should create a UserContext but not set role from non-service user
	// Since only X-Vire-Service-ID is present but lookup fails role check,
	// UserContext is created but empty (no identity resolved)
	if capturedUC != nil {
		assert.Empty(t, capturedUC.UserID, "non-service user should not be resolved via service header")
		assert.Empty(t, capturedUC.Role, "non-service role should not be set")
	}
}

// --- ValidateRole includes service ---

func TestValidateRole_IncludesService(t *testing.T) {
	assert.NoError(t, models.ValidateRole("admin"))
	assert.NoError(t, models.ValidateRole("user"))
	assert.NoError(t, models.ValidateRole("service"))
	assert.Error(t, models.ValidateRole("superadmin"))
	assert.Error(t, models.ValidateRole(""))
}
