package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// validServiceKey is a 32+ character key used in tests where the server has a key configured.
const validServiceKey = "test-service-key-minimum-32-chars-ok"

// withServiceKey returns headers that identify the caller as a service.
func withServiceID(serviceUserID string) map[string]string {
	return map[string]string{"X-Vire-Service-ID": serviceUserID}
}

// registerService performs a POST /api/services/register and returns the response.
// The env must have VIRE_SERVICE_KEY set to validServiceKey.
func registerService(t *testing.T, env *common.Env, serviceID, serviceKey, serviceType string) *http.Response {
	t.Helper()
	resp, err := env.HTTPPost("/api/services/register", map[string]interface{}{
		"service_id":   serviceID,
		"service_key":  serviceKey,
		"service_type": serviceType,
	})
	require.NoError(t, err)
	return resp
}

// --- POST /api/services/register ---

func TestServiceRegister_ValidRegistration(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp := registerService(t, env, "portal-prod-1", validServiceKey, "portal")
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("register_valid_response", string(body))

	require.Equal(t, 200, resp.StatusCode, "valid registration should return 200: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, "ok", result["status"])
	assert.Equal(t, "service:portal-prod-1", result["service_user_id"])
	assert.NotEmpty(t, result["registered_at"], "registered_at should be set")
}

func TestServiceRegister_WrongKeyReturns403(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp := registerService(t, env, "portal-prod-1", "wrong-key-that-does-not-match-server", "portal")
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode, "wrong service_key should return 403")
}

func TestServiceRegister_NoServerKeyReturns501(t *testing.T) {
	env := common.NewEnv(t) // no VIRE_SERVICE_KEY set
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp := registerService(t, env, "portal-prod-1", validServiceKey, "portal")
	defer resp.Body.Close()

	assert.Equal(t, 501, resp.StatusCode, "missing server key should return 501 Not Implemented")
}

func TestServiceRegister_EmptyServiceIDReturns400(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp := registerService(t, env, "", validServiceKey, "portal")
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode, "empty service_id should return 400")
}

func TestServiceRegister_ShortKeyReturns400(t *testing.T) {
	shortKey := "short-key" // less than 32 characters
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": shortKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp := registerService(t, env, "portal-prod-1", shortKey, "portal")
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode, "short service_key (<32 chars) should return 400")
}

func TestServiceRegister_Idempotent(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// First registration
	resp := registerService(t, env, "portal-idempotent", validServiceKey, "portal")
	body1 := readBody(t, resp.Body)
	resp.Body.Close()
	guard.SaveResult("register_first", string(body1))
	require.Equal(t, 200, resp.StatusCode, "first registration should succeed: %s", string(body1))

	var result1 map[string]interface{}
	require.NoError(t, json.Unmarshal(body1, &result1))
	firstRegisteredAt := result1["registered_at"].(string)

	// Second registration (idempotent — re-registration on restart)
	resp = registerService(t, env, "portal-idempotent", validServiceKey, "portal")
	body2 := readBody(t, resp.Body)
	resp.Body.Close()
	guard.SaveResult("register_second", string(body2))
	require.Equal(t, 200, resp.StatusCode, "second registration should succeed: %s", string(body2))

	var result2 map[string]interface{}
	require.NoError(t, json.Unmarshal(body2, &result2))

	assert.Equal(t, "ok", result2["status"])
	assert.Equal(t, "service:portal-idempotent", result2["service_user_id"])

	// registered_at should be updated (modified_at reflects heartbeat)
	// We allow it to be the same if the re-registration happens within the same second
	secondRegisteredAt := result2["registered_at"].(string)
	assert.NotEmpty(t, secondRegisteredAt)
	_ = firstRegisteredAt // both should be valid timestamps
}

func TestServiceRegister_ServiceUserAppearsInAdminList(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register service
	resp := registerService(t, env, "portal-list-check", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	guard.SaveResult("register_response", string(body))
	require.Equal(t, 200, resp.StatusCode, "registration should succeed: %s", string(body))

	// Set up admin to list users
	adminID, _ := setupAdminAndUser(t, env)

	// List users via admin endpoint
	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	listBody := readBody(t, resp.Body)
	guard.SaveResult("admin_list_users", string(listBody))
	require.Equal(t, 200, resp.StatusCode)

	var listResult map[string]interface{}
	require.NoError(t, json.Unmarshal(listBody, &listResult))
	users := listResult["users"].([]interface{})

	// Find the service user
	var serviceEntry map[string]interface{}
	for _, u := range users {
		entry := u.(map[string]interface{})
		if entry["id"] == "service:portal-list-check" {
			serviceEntry = entry
			break
		}
	}
	require.NotNil(t, serviceEntry, "service user must appear in admin user list")

	assert.Equal(t, "portal-list-check@service.vire.local", serviceEntry["email"])
	assert.Equal(t, "Service: portal-list-check", serviceEntry["name"])
	assert.Equal(t, "service", serviceEntry["provider"])
	assert.Equal(t, "service", serviceEntry["role"])

	// Ensure no password hash leaked
	assert.NotContains(t, string(listBody), "$2a$", "bcrypt hash must never appear in list_users response")
}

// --- Service user: admin endpoint access ---

func TestServiceUser_CanListUsers(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register service user
	resp := registerService(t, env, "portal-auth-test", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	serviceUserID := "service:portal-auth-test"

	// Create a regular user (to confirm list has content)
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "listcheckuser",
		"email":    "listcheck@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Service user can GET /api/admin/users
	resp, err = env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withServiceID(serviceUserID))
	require.NoError(t, err)
	defer resp.Body.Close()

	listBody := readBody(t, resp.Body)
	guard.SaveResult("service_list_users", string(listBody))

	assert.Equal(t, 200, resp.StatusCode, "service user should be able to list users: %s", string(listBody))

	var listResult map[string]interface{}
	require.NoError(t, json.Unmarshal(listBody, &listResult))
	users := listResult["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 1, "user list should not be empty")
}

func TestServiceUser_CanUpdateUserRole(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register service user
	resp := registerService(t, env, "portal-role-updater", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	serviceUserID := "service:portal-role-updater"

	// Create target user to promote
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "promotetarget",
		"email":    "promote@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Service user can PATCH /api/admin/users/{id}/role
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/promotetarget/role",
		map[string]string{"role": "admin"}, withServiceID(serviceUserID))
	require.NoError(t, err)
	defer resp.Body.Close()

	patchBody := readBody(t, resp.Body)
	guard.SaveResult("service_patch_role", string(patchBody))

	assert.Equal(t, 200, resp.StatusCode, "service user should be able to update roles: %s", string(patchBody))

	var patchResult map[string]interface{}
	require.NoError(t, json.Unmarshal(patchBody, &patchResult))
	assert.Equal(t, "admin", patchResult["role"])
	assert.Equal(t, "promotetarget", patchResult["id"])
}

func TestServiceUser_CannotAccessJobEndpoints(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Register service user
	resp := registerService(t, env, "portal-jobs-check", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	serviceUserID := "service:portal-jobs-check"

	// Service user must NOT access job management endpoints
	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/jobs", nil, withServiceID(serviceUserID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode, "service user must not access job endpoints")
}

func TestServiceUser_CannotPromoteToServiceRole(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register service user
	resp := registerService(t, env, "portal-service-promo", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	serviceUserID := "service:portal-service-promo"

	// Create target user
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "svcpromocheck",
		"email":    "svcpromocheck@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Attempt to set role to "service" — must be rejected
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/svcpromocheck/role",
		map[string]string{"role": "service"}, withServiceID(serviceUserID))
	require.NoError(t, err)
	defer resp.Body.Close()

	patchBody := readBody(t, resp.Body)
	guard.SaveResult("reject_service_role", string(patchBody))

	assert.Equal(t, 400, resp.StatusCode, "setting role to 'service' must be rejected: %s", string(patchBody))
}

// Admin user also cannot promote to service role via PATCH endpoint.
func TestAdminUser_CannotPromoteToServiceRole(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminID, userID := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/"+userID+"/role",
		map[string]string{"role": "service"}, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("admin_reject_service_role", string(body))

	assert.Equal(t, 400, resp.StatusCode, "admin setting role to 'service' must be rejected: %s", string(body))
}

// --- POST /api/auth/login: block service users ---

func TestAuthLogin_RejectsServiceUser(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register a service user
	resp := registerService(t, env, "portal-login-block", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	// Attempt to login as service user via password login
	// The email for the service user is "portal-login-block@service.vire.local"
	// Username used for login is service user ID "service:portal-login-block" or email
	resp, err := env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "service:portal-login-block",
		"password": "anypassword",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	loginBody := readBody(t, resp.Body)
	guard.SaveResult("service_login_attempt", string(loginBody))

	assert.Equal(t, 403, resp.StatusCode, "service account login must return 403: %s", string(loginBody))

	var loginResult map[string]interface{}
	require.NoError(t, json.Unmarshal(loginBody, &loginResult))
	errMsg, _ := loginResult["error"].(string)
	assert.True(t, strings.Contains(strings.ToLower(errMsg), "service"), "error should mention service accounts")
}

// --- POST /api/admin/services/tidy ---

func TestServiceTidy_PurgesStaleServiceUsers(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register a service user
	resp := registerService(t, env, "portal-tidy-test", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	// Bootstrap admin user to call tidy
	adminID, _ := setupAdminAndUser(t, env)

	// Call tidy endpoint — service user was just registered so it should NOT be purged (recent)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/admin/services/tidy", nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	tidyBody := readBody(t, resp.Body)
	guard.SaveResult("tidy_response", string(tidyBody))

	require.Equal(t, 200, resp.StatusCode, "tidy should return 200: %s", string(tidyBody))

	var tidyResult map[string]interface{}
	require.NoError(t, json.Unmarshal(tidyBody, &tidyResult))

	// Fields must be present
	_, hasPurged := tidyResult["purged"]
	_, hasRemaining := tidyResult["remaining"]
	assert.True(t, hasPurged, "tidy response must include 'purged' field")
	assert.True(t, hasRemaining, "tidy response must include 'remaining' field")

	// Recently registered service user should not be purged
	purged := tidyResult["purged"].(float64)
	assert.Equal(t, float64(0), purged, "recently registered service user should not be purged")

	remaining := tidyResult["remaining"].(float64)
	assert.GreaterOrEqual(t, remaining, float64(1), "tidy should report at least one remaining service user")
}

func TestServiceTidy_NonAdminReturns403(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Register a service user (to use as the caller)
	resp := registerService(t, env, "portal-tidy-noauth", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	serviceUserID := "service:portal-tidy-noauth"

	// Service user must NOT be able to call tidy (admin-only endpoint)
	resp, err := env.HTTPRequest(http.MethodPost, "/api/admin/services/tidy", nil, withServiceID(serviceUserID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode, "service user must not call tidy endpoint")
}

func TestServiceTidy_UnauthenticatedReturns401(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/admin/services/tidy", nil, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode, "unauthenticated tidy should return 401")
}

func TestServiceTidy_NonServiceUsersNotAffected(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Set up admin and regular users
	adminID, _ := setupAdminAndUser(t, env)

	// Call tidy — no service users registered
	resp, err := env.HTTPRequest(http.MethodPost, "/api/admin/services/tidy", nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	tidyBody := readBody(t, resp.Body)
	guard.SaveResult("tidy_no_services", string(tidyBody))

	require.Equal(t, 200, resp.StatusCode, "tidy should return 200 even with no service users: %s", string(tidyBody))

	var tidyResult map[string]interface{}
	require.NoError(t, json.Unmarshal(tidyBody, &tidyResult))

	purged := tidyResult["purged"].(float64)
	assert.Equal(t, float64(0), purged, "no service users should be purged when none exist")

	// Verify regular users still exist in the system
	resp, err = env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	var listResult map[string]interface{}
	require.NoError(t, json.Unmarshal(readBody(t, resp.Body), &listResult))
	users := listResult["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 2, "non-service users must survive tidy")
}

// --- Service user: self-demotion guard ---

func TestServiceUser_SelfDemotionGuardDoesNotApply(t *testing.T) {
	// Service users are not admins — self-demotion guard only applies to admin users.
	// A service user updating another user's role should not hit the self-demotion guard.
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_SERVICE_KEY": validServiceKey},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Register service user
	resp := registerService(t, env, "portal-self-guard", validServiceKey, "portal")
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "service registration must succeed: %s", string(body))

	serviceUserID := "service:portal-self-guard"

	// Create a regular admin user (not self)
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "demotabletarget",
		"email":    "demotable@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Service user promotes this user to admin first — requires another admin
	// Use dev OAuth to bootstrap admin
	oauthResp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	oauthResp.Body.Close()
	require.Equal(t, 200, oauthResp.StatusCode)

	// Promote demotabletarget to admin
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/demotabletarget/role",
		map[string]string{"role": "admin"}, withUserID("dev_user"))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	// Service user can demote a different admin without triggering self-demotion guard
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/demotabletarget/role",
		map[string]string{"role": "user"}, withServiceID(serviceUserID))
	require.NoError(t, err)
	defer resp.Body.Close()

	demoteBody := readBody(t, resp.Body)
	guard.SaveResult("service_demote_other", string(demoteBody))

	assert.Equal(t, 200, resp.StatusCode, "service user should be able to demote a different admin: %s", string(demoteBody))
}
