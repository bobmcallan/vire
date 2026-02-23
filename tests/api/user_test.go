package api

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

// decodeResponse reads and decodes a JSON response body.
func decodeResponse(t *testing.T, resp io.ReadCloser) map[string]interface{} {
	t.Helper()
	body, err := io.ReadAll(resp)
	require.NoError(t, err)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))
	return result
}

func TestCreateUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "testcreate",
		"email":    "testcreate@example.com",
		"password": "securepassword123",
		"role":     "user",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 201, resp.StatusCode)

	result := decodeResponse(t, resp.Body)
	assert.Equal(t, "ok", result["status"])
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "testcreate", data["username"])
	assert.Equal(t, "testcreate@example.com", data["email"])
	assert.Equal(t, "user", data["role"])
}

func TestCreateUserDuplicate(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	body := map[string]interface{}{
		"username": "dupuser",
		"email":    "dup@example.com",
		"password": "password123",
		"role":     "user",
	}

	resp, err := env.HTTPPost("/api/users", body)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode)

	// Second create should fail with 409
	resp, err = env.HTTPPost("/api/users", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 409, resp.StatusCode)
}

func TestGetUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create user first
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "testget",
		"email":    "testget@example.com",
		"password": "password123",
		"role":     "user",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Get user
	resp, err = env.HTTPGet("/api/users/testget")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	assert.Equal(t, "ok", result["status"])
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "testget", data["username"])
	assert.Equal(t, "testget@example.com", data["email"])
}

func TestGetUserNotFound(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	resp, err := env.HTTPGet("/api/users/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 404, resp.StatusCode)
}

func TestUpdateUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create user
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "testupdate",
		"email":    "testupdate@example.com",
		"password": "password123",
		"role":     "user",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Update user (role field is ignored on this endpoint)
	resp, err = env.HTTPPut("/api/users/testupdate", map[string]interface{}{
		"email": "updated@example.com",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "updated@example.com", data["email"])
	assert.Equal(t, "user", data["role"])
}

func TestDeleteUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create user
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "testdelete",
		"email":    "testdelete@example.com",
		"password": "password123",
		"role":     "user",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 201, resp.StatusCode)

	// Delete user
	resp, err = env.HTTPDelete("/api/users/testdelete")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)

	// Verify deleted
	resp, err = env.HTTPGet("/api/users/testdelete")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)
}

func TestCheckUsername(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Check available username
	resp, err := env.HTTPGet("/api/users/check/availableuser")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, true, data["available"])
	assert.Equal(t, "availableuser", data["username"])
}

func TestCheckUsernameTaken(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Create user first
	resp, err := env.HTTPPost("/api/users", map[string]interface{}{
		"username": "takenuser",
		"email":    "taken@example.com",
		"password": "password123",
		"role":     "user",
	})
	require.NoError(t, err)
	resp.Body.Close()

	// Check taken username
	resp, err = env.HTTPGet("/api/users/check/takenuser")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, false, data["available"])
}

func TestUpsertUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Upsert new user (create)
	resp, err := env.HTTPPost("/api/users/upsert", map[string]interface{}{
		"username": "upsertuser",
		"email":    "upsert@example.com",
		"password": "password123",
		"role":     "user",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode)

	// Upsert existing user (update — role field is ignored on this endpoint)
	resp2, err := env.HTTPPost("/api/users/upsert", map[string]interface{}{
		"username": "upsertuser",
		"email":    "upsert_v2@example.com",
	})
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, 200, resp2.StatusCode)

	result := decodeResponse(t, resp2.Body)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "upsert_v2@example.com", data["email"])
	assert.Equal(t, "user", data["role"])
}
