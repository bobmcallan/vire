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

// --- Dev Provider OAuth Flow ---

func TestAuthOAuth_DevProvider(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.Equal(t, "ok", result["status"])
	data := result["data"].(map[string]interface{})

	// Token should be present
	token, ok := data["token"].(string)
	require.True(t, ok, "expected token in response")
	assert.NotEmpty(t, token)

	// User should be present with expected fields
	user := data["user"].(map[string]interface{})
	assert.Equal(t, "dev_user", user["user_id"])
	assert.Equal(t, "dev@vire.local", user["email"])
	assert.Equal(t, "dev", user["provider"])
	assert.Equal(t, "admin", user["role"])
}

func TestAuthOAuth_DevProvider_Idempotent(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// First call
	resp1, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp1.Body.Close()
	require.Equal(t, 200, resp1.StatusCode)

	result1 := decodeResponse(t, resp1.Body)
	data1 := result1["data"].(map[string]interface{})
	user1 := data1["user"].(map[string]interface{})

	// Second call
	resp2, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, 200, resp2.StatusCode)

	result2 := decodeResponse(t, resp2.Body)
	data2 := result2["data"].(map[string]interface{})
	user2 := data2["user"].(map[string]interface{})

	// Same user returned
	assert.Equal(t, user1["user_id"], user2["user_id"])
	assert.Equal(t, user1["email"], user2["email"])
}

func TestAuthOAuth_UnsupportedProvider(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "unsupported",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 400, resp.StatusCode)
}

func TestAuthOAuth_MethodNotAllowed(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPGet("/api/auth/oauth")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 405, resp.StatusCode)
}

// --- Token Validation ---

func TestAuthValidate_ValidToken(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Get a token via dev provider
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	token := data["token"].(string)

	// Validate the token
	validateResp, err := env.HTTPRequest(http.MethodPost, "/api/auth/validate", nil,
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer validateResp.Body.Close()

	assert.Equal(t, 200, validateResp.StatusCode)

	validateResult := decodeResponse(t, validateResp.Body)
	assert.Equal(t, "ok", validateResult["status"])
	validateData := validateResult["data"].(map[string]interface{})
	user := validateData["user"].(map[string]interface{})
	assert.Equal(t, "dev_user", user["user_id"])
}

func TestAuthValidate_InvalidToken(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/auth/validate", nil,
		map[string]string{"Authorization": "Bearer invalid.token.here"})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

func TestAuthValidate_MissingAuthHeader(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPRequest(http.MethodPost, "/api/auth/validate", nil, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 401, resp.StatusCode)
}

// --- Gap 3: JWT Name Claim via API ---

func TestAuthOAuth_DevProvider_UserResponseIncludesName(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	data := result["data"].(map[string]interface{})
	user := data["user"].(map[string]interface{})

	// Gap 3 fix: oauthUserResponse should include "name" field
	_, hasName := user["name"]
	assert.True(t, hasName, "user response should include 'name' field after gap fix")
}

func TestAuthValidate_UserResponseIncludesName(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Get a token via dev provider
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	token := data["token"].(string)

	// Validate the token and check user response
	validateResp, err := env.HTTPRequest(http.MethodPost, "/api/auth/validate", nil,
		map[string]string{"Authorization": "Bearer " + token})
	require.NoError(t, err)
	defer validateResp.Body.Close()
	require.Equal(t, 200, validateResp.StatusCode)

	validateResult := decodeResponse(t, validateResp.Body)
	validateData := validateResult["data"].(map[string]interface{})
	user := validateData["user"].(map[string]interface{})

	// Gap 3 fix: validate response should also include "name" field
	_, hasName := user["name"]
	assert.True(t, hasName, "validate response user should include 'name' field")
}

// --- Gap 2: Error redirect behavior (limited API testing) ---
// Note: Full callback error redirect testing requires valid HMAC-signed state
// parameters and mock OAuth providers. The callback endpoints are tested
// thoroughly in the unit and stress tests (internal/server/handlers_auth_test.go
// and handlers_auth_stress_test.go).

// --- Gap 1: Email-based account linking (limited API testing) ---
// Note: Full email-based account linking requires two different OAuth providers
// to return the same email. This is tested in the unit tests via
// findOrCreateOAuthUser and in the data layer tests via GetUserByEmail.
// The data layer integration tests are in tests/data/internalstore_test.go.

// --- Create user and verify OAuth login can find them ---

func TestAuthOAuth_DevProvider_CreatesUser_ThenGetUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create user via dev provider
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)

	// The dev user should now be retrievable via the users API
	userResp, err := env.HTTPGet("/api/users/dev_user")
	require.NoError(t, err)
	defer userResp.Body.Close()

	assert.Equal(t, 200, userResp.StatusCode)
	userResult := decodeResponse(t, userResp.Body)
	assert.Equal(t, "ok", userResult["status"])
	userData := userResult["data"].(map[string]interface{})
	assert.Equal(t, "dev_user", userData["username"])
	assert.Equal(t, "dev@vire.local", userData["email"])
}
