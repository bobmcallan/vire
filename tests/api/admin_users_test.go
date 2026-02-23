package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// --- Helpers ---

// setupAdminAndUser creates an admin user via the dev OAuth provider and a regular
// user via POST /api/users, returning their IDs.
// The dev provider creates "dev_user" with role "admin".
// Role is ignored on user endpoints, so the regular user always gets role "user".
func setupAdminAndUser(t *testing.T, env *common.Env) (adminID, userID string) {
	t.Helper()

	// Bootstrap admin via dev OAuth provider
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "dev OAuth failed: %s", string(body))

	// Create regular user via POST /api/users (role is ignored, defaults to "user")
	resp, err = env.HTTPPost("/api/users", map[string]interface{}{
		"username": "roleuser",
		"email":    "roleuser@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	return "dev_user", "roleuser"
}

// withUserID returns headers that identify the requesting user.
func withUserID(userID string) map[string]string {
	return map[string]string{"X-Vire-User-ID": userID}
}

// readBody reads and returns the response body as bytes.
func readBody(t *testing.T, body io.ReadCloser) []byte {
	t.Helper()
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	return data
}

// --- GET /api/admin/users ---

func TestAdminListUsers_Unauthenticated(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPGet("/api/admin/users")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

func TestAdminListUsers_NonAdmin(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	_, userID := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(userID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
}

func TestAdminListUsers_Success(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminID, _ := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("list_users_response", string(body))

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	users := result["users"].([]interface{})
	assert.GreaterOrEqual(t, len(users), 2, "should contain at least admin + user")

	// Verify all user entries have expected fields and no password_hash
	expectedFields := []string{"id", "email", "name", "provider", "role", "created_at"}
	for _, u := range users {
		entry := u.(map[string]interface{})
		for _, field := range expectedFields {
			_, exists := entry[field]
			assert.True(t, exists, "expected field %q in user entry", field)
		}
		_, hasHash := entry["password_hash"]
		assert.False(t, hasHash, "password_hash must never appear in list_users response")
	}

	// Verify no bcrypt hashes in raw response
	assert.NotContains(t, string(body), "$2a$", "bcrypt hash must never appear in response body")
}

func TestAdminListUsers_MethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/admin/users", map[string]interface{}{"foo": "bar"})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 405, resp.StatusCode)
}

func TestAdminListUsers_ShowsCorrectRoles(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminID, _ := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(readBody(t, resp.Body), &result))
	users := result["users"].([]interface{})

	roleMap := make(map[string]string)
	for _, u := range users {
		entry := u.(map[string]interface{})
		roleMap[entry["id"].(string)] = entry["role"].(string)
	}

	assert.Equal(t, "admin", roleMap["dev_user"])
	assert.Equal(t, "user", roleMap["roleuser"])
}

// --- PATCH /api/admin/users/{id}/role ---

func TestAdminUpdateUserRole_Unauthenticated(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/someone/role",
		map[string]string{"role": "admin"}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

func TestAdminUpdateUserRole_NonAdmin(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	_, userID := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/dev_user/role",
		map[string]string{"role": "user"}, withUserID(userID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode)
}

func TestAdminUpdateUserRole_PromoteToAdmin(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminID, userID := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/"+userID+"/role",
		map[string]string{"role": "admin"}, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("promote_to_admin_response", string(body))

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "admin", result["role"])
	assert.Equal(t, userID, result["id"])
}

func TestAdminUpdateUserRole_DemoteToUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminID, _ := setupAdminAndUser(t, env)

	// Create a second user and promote them to admin via the admin endpoint
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "admin2",
		"email":    "admin2@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Promote admin2 to admin
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/admin2/role",
		map[string]string{"role": "admin"}, withUserID(adminID))
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	// Now demote admin2 back to user
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/admin2/role",
		map[string]string{"role": "user"}, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(readBody(t, resp.Body), &result))
	assert.Equal(t, "user", result["role"])
}

func TestAdminUpdateUserRole_InvalidRole(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminID, userID := setupAdminAndUser(t, env)

	invalidRoles := []string{"", "superadmin", "root", "Admin", "USER"}
	for _, role := range invalidRoles {
		t.Run("role_"+role, func(t *testing.T) {
			resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/"+userID+"/role",
				map[string]string{"role": role}, withUserID(adminID))
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, 400, resp.StatusCode, "role %q should be rejected", role)
		})
	}
}

func TestAdminUpdateUserRole_SelfDemotion(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminID, _ := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/"+adminID+"/role",
		map[string]string{"role": "user"}, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)

	body := readBody(t, resp.Body)
	assert.Contains(t, string(body), "Cannot remove your own admin role")
}

func TestAdminUpdateUserRole_TargetNotFound(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminID, _ := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodPatch, "/api/admin/users/nonexistent/role",
		map[string]string{"role": "admin"}, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode)
}

func TestAdminUpdateUserRole_VerifiedViaListUsers(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminID, userID := setupAdminAndUser(t, env)

	// Verify initial role via list
	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(adminID))
	require.NoError(t, err)
	body := readBody(t, resp.Body)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	var listResult map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &listResult))
	users := listResult["users"].([]interface{})
	for _, u := range users {
		entry := u.(map[string]interface{})
		if entry["id"] == userID {
			assert.Equal(t, "user", entry["role"], "initial role should be 'user'")
		}
	}

	// Promote to admin
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/"+userID+"/role",
		map[string]string{"role": "admin"}, withUserID(adminID))
	require.NoError(t, err)
	promoteBody := readBody(t, resp.Body)
	resp.Body.Close()
	guard.SaveResult("promote_response", string(promoteBody))
	require.Equal(t, 200, resp.StatusCode)

	// Verify updated role via list
	resp, err = env.HTTPRequest(http.MethodGet, "/api/admin/users", nil, withUserID(adminID))
	require.NoError(t, err)
	body = readBody(t, resp.Body)
	resp.Body.Close()
	guard.SaveResult("list_after_promote", string(body))
	require.Equal(t, 200, resp.StatusCode)

	require.NoError(t, json.Unmarshal(body, &listResult))
	users = listResult["users"].([]interface{})
	for _, u := range users {
		entry := u.(map[string]interface{})
		if entry["id"] == userID {
			assert.Equal(t, "admin", entry["role"], "role should be updated to 'admin'")
		}
	}
}

// --- Role ignored on existing user endpoints ---

func TestCreateUser_IgnoresRoleField(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Role field is ignored on user creation — user always gets role "user"
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "baduser",
		"email":    "bad@test.com",
		"password": "password123",
		"role":     "superadmin",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode, "role field should be ignored, user should be created")

	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "user", data["role"], "role should always be 'user' regardless of input")
}

func TestCreateUser_DefaultsToUserRole(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "norolegiven",
		"email":    "norole@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 201, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "user", data["role"], "default role should be 'user' when omitted")
}

func TestUpsertUser_IgnoresRoleField(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Role field is ignored on upsert — user always gets role "user"
	resp, err := env.HTTPPost("/api/users/upsert", map[string]interface{}{
		"username": "baduser",
		"email":    "bad@test.com",
		"password": "password123",
		"role":     "root",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode, "role field should be ignored, user should be created")

	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "user", data["role"], "role should always be 'user' regardless of input")
}

func TestUpdateUser_IgnoresRoleField(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create user first
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "validuser",
		"email":    "valid@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Update with role field — should be ignored
	resp, err = env.HTTPPut("/api/users/validuser", map[string]interface{}{
		"role": "admin",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode, "role field should be ignored on update")
}

// --- JWT role claim ---

func TestAuthLogin_JWTContainsRoleClaim(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Create user — role is always "user" on user endpoints
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "jwtuser",
		"email":    "jwt@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Login
	resp, err = env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "jwtuser",
		"password": "password123",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("login_response", string(body))

	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	data := result["data"].(map[string]interface{})
	userData := data["user"].(map[string]interface{})
	assert.Equal(t, "user", userData["role"])

	// Get token and validate it
	token := data["token"].(string)
	require.NotEmpty(t, token)

	// Validate the token - role should be in claims
	validateResp, err := env.HTTPRequest(http.MethodPost, "/api/auth/validate", nil,
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer validateResp.Body.Close()

	vBody := readBody(t, validateResp.Body)
	guard.SaveResult("validate_response", string(vBody))

	require.Equal(t, 200, validateResp.StatusCode)

	var validateResult map[string]interface{}
	require.NoError(t, json.Unmarshal(vBody, &validateResult))
	validateData := validateResult["data"].(map[string]interface{})
	user := validateData["user"].(map[string]interface{})
	assert.Equal(t, "user", user["role"], "role claim should be present in validated JWT")
}

func TestAuthOAuth_DevProvider_JWTContainsRoleClaim(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Dev provider creates an admin user
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	token := data["token"].(string)
	user := data["user"].(map[string]interface{})

	// User should have a role
	assert.Equal(t, "admin", user["role"], "dev provider user should have admin role")

	// Validate the token - role should be in claims
	validateResp, err := env.HTTPRequest(http.MethodPost, "/api/auth/validate", nil,
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer validateResp.Body.Close()
	require.Equal(t, 200, validateResp.StatusCode)

	validateResult := decodeResponse(t, validateResp.Body)
	validateData := validateResult["data"].(map[string]interface{})
	vUser := validateData["user"].(map[string]interface{})
	assert.Equal(t, "admin", vUser["role"], "role should be in validated JWT claims")
}

// --- Route dispatch: /api/admin/users/ with unknown sub-paths ---

func TestAdminUsersRoute_UnknownSubpath(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminID, _ := setupAdminAndUser(t, env)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users/someone/unknown",
		nil, withUserID(adminID))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode)
}
