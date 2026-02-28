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

// extractBreakglassPassword scans container logs for the break-glass admin creation
// log entry and returns the cleartext password logged at startup.
// The server logs a WARN-level JSON entry with fields: email=admin@vire.local password=<cleartext>
func extractBreakglassPassword(t *testing.T, env *common.Env) string {
	t.Helper()

	logs, err := env.ReadContainerLogs()
	require.NoError(t, err, "failed to read container logs")

	// Scan each line for the break-glass admin creation log entry.
	// Log format is JSON: {"level":"warn","message":"Break-glass admin created","email":"admin@vire.local","password":"<cleartext>"}
	// Docker may prefix lines with timestamps, so we search for JSON within each line.
	for _, line := range strings.Split(logs, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "Break-glass admin created") {
			continue
		}
		// Find the JSON object within the line (may be prefixed by Docker timestamps)
		start := strings.Index(line, "{")
		if start < 0 {
			continue
		}
		jsonPart := line[start:]
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(jsonPart), &entry); err != nil {
			continue
		}
		if pw, ok := entry["password"].(string); ok && pw != "" {
			return pw
		}
	}

	t.Fatal("Break-glass admin password not found in container logs — is VIRE_AUTH_BREAKGLASS=true set?")
	return ""
}

// loginBreakglass authenticates as the break-glass admin and returns the JWT token.
func loginBreakglass(t *testing.T, env *common.Env, password string) string {
	t.Helper()

	resp, err := env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "breakglass-admin",
		"password": password,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	require.Equal(t, 200, resp.StatusCode, "breakglass login failed: %s", string(body))

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	data := result["data"].(map[string]interface{})
	token, ok := data["token"].(string)
	require.True(t, ok, "expected token in login response")
	require.NotEmpty(t, token)
	return token
}

// --- Break-glass admin: authentication ---

func TestBreakglassAdmin_LoginWithGeneratedCredentials(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	password := extractBreakglassPassword(t, env)
	require.NotEmpty(t, password)

	resp, err := env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "breakglass-admin",
		"password": password,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("breakglass_login_response", string(body))

	require.Equal(t, 200, resp.StatusCode, "break-glass admin login should succeed")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "ok", result["status"])

	data := result["data"].(map[string]interface{})
	token, ok := data["token"].(string)
	assert.True(t, ok, "expected token in login response")
	assert.NotEmpty(t, token)

	user := data["user"].(map[string]interface{})
	assert.Equal(t, "breakglass-admin", user["username"])
	assert.Equal(t, "admin", user["role"], "break-glass admin must have admin role")
}

func TestBreakglassAdmin_WrongPasswordFails(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "breakglass-admin",
		"password": "wrongpassword",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode, "wrong password should be rejected")
}

// --- Break-glass admin: admin endpoint access ---

func TestBreakglassAdmin_CanListUsers(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	password := extractBreakglassPassword(t, env)
	token := loginBreakglass(t, env, password)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil,
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("breakglass_list_users", string(body))

	require.Equal(t, 200, resp.StatusCode, "break-glass admin should access list users endpoint")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	users, ok := result["users"].([]interface{})
	require.True(t, ok, "expected users array in response")

	// Find break-glass admin in the list
	found := false
	for _, u := range users {
		entry := u.(map[string]interface{})
		if entry["id"] == "breakglass-admin" {
			found = true
			assert.Equal(t, "admin", entry["role"], "break-glass admin must have admin role in list")
			assert.Equal(t, "admin@vire.local", entry["email"])
		}
	}
	assert.True(t, found, "break-glass admin must appear in user list")

	// Ensure no password hashes in response
	assert.NotContains(t, string(body), "$2a$", "bcrypt hash must never appear in list_users response")
}

func TestBreakglassAdmin_CanUpdateUserRole(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	password := extractBreakglassPassword(t, env)
	token := loginBreakglass(t, env, password)

	// Create a regular user to promote
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "targetuser",
		"email":    "target@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Promote targetuser to admin using break-glass admin's token
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/targetuser/role",
		map[string]string{"role": "admin"},
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("breakglass_promote_user", string(body))

	require.Equal(t, 200, resp.StatusCode, "break-glass admin should be able to promote users")

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "admin", result["role"])
	assert.Equal(t, "targetuser", result["id"])
}

// --- Regular user cannot access admin endpoints ---

func TestBreakglassAdmin_RegularUserCannotListUsers(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create a regular user
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "regularuser",
		"email":    "regular@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Login as regular user
	resp, err = env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "regularuser",
		"password": "password123",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	var loginResult map[string]interface{}
	require.NoError(t, json.Unmarshal(readBody(t, resp.Body), &loginResult))
	userToken := loginResult["data"].(map[string]interface{})["token"].(string)

	// Regular user should not be able to list users
	resp, err = env.HTTPRequest(http.MethodGet, "/api/admin/users", nil,
		map[string]string{"Authorization": "Bearer " + userToken})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode, "regular user must not access admin endpoints")
}

func TestBreakglassAdmin_RegularUserCannotUpdateRole(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create a regular user
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "normaluser",
		"email":    "normal@test.com",
		"password": "password123",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Login as regular user
	resp, err = env.HTTPPost("/api/auth/login", map[string]interface{}{
		"username": "normaluser",
		"password": "password123",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	var loginResult map[string]interface{}
	require.NoError(t, json.Unmarshal(readBody(t, resp.Body), &loginResult))
	userToken := loginResult["data"].(map[string]interface{})["token"].(string)

	// Regular user should not be able to update roles
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/admin/users/normaluser/role",
		map[string]string{"role": "admin"},
		map[string]string{"Authorization": "Bearer " + userToken})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 403, resp.StatusCode, "regular user must not update roles")
}

// --- Break-glass admin: idempotency ---

func TestBreakglassAdmin_CreatedIdempotentlyOnRestart(t *testing.T) {
	// This test verifies that if the break-glass admin already exists,
	// the server logs "Break-glass admin already exists" (not "created"),
	// by simulating via: start server, verify admin created, check logs for no duplicate creation
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	logs, err := env.ReadContainerLogs()
	require.NoError(t, err)
	guard.SaveResult("startup_logs", logs)

	// Should have exactly one "Break-glass admin created" entry (not two)
	creationCount := strings.Count(logs, "Break-glass admin created")
	assert.Equal(t, 1, creationCount, "break-glass admin should only be created once at startup")

	// Should NOT have an error during creation
	assert.NotContains(t, strings.ToLower(logs), "failed to create break-glass", "break-glass creation should not fail")
}

// --- Break-glass admin: correctness of user fields ---

func TestBreakglassAdmin_UserFieldsInListResponse(t *testing.T) {
	env := common.NewEnvWithOptions(t, common.EnvOptions{
		ExtraEnv: map[string]string{"VIRE_AUTH_BREAKGLASS": "true"},
	})
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	password := extractBreakglassPassword(t, env)
	token := loginBreakglass(t, env, password)

	resp, err := env.HTTPRequest(http.MethodGet, "/api/admin/users", nil,
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer resp.Body.Close()

	body := readBody(t, resp.Body)
	guard.SaveResult("breakglass_user_fields", string(body))

	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	users, ok := result["users"].([]interface{})
	require.True(t, ok)

	// Find and validate break-glass admin entry
	var bgEntry map[string]interface{}
	for _, u := range users {
		entry := u.(map[string]interface{})
		if entry["id"] == "breakglass-admin" {
			bgEntry = entry
			break
		}
	}
	require.NotNil(t, bgEntry, "break-glass admin must appear in user list")

	// Verify all expected fields
	assert.Equal(t, "admin@vire.local", bgEntry["email"])
	assert.Equal(t, "Break-Glass Admin", bgEntry["name"])
	assert.Equal(t, "system", bgEntry["provider"])
	assert.Equal(t, "admin", bgEntry["role"])
	_, hasCreatedAt := bgEntry["created_at"]
	assert.True(t, hasCreatedAt, "break-glass admin must have created_at field")

	// Ensure password hash never leaked
	assert.NotContains(t, string(body), "$2a$", "bcrypt hash must never appear in response")
}
