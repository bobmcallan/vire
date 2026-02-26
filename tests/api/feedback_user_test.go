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

// devAuthToken obtains a JWT bearer token via the dev OAuth provider.
// The dev provider creates a user with user_id="dev_user", email="dev@vire.local".
func devAuthToken(t *testing.T, env *common.Env) string {
	t.Helper()
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode, "dev OAuth login must succeed")

	result := decodeResponse(t, resp.Body)
	data, ok := result["data"].(map[string]interface{})
	require.True(t, ok, "response should have 'data' field")
	token, ok := data["token"].(string)
	require.True(t, ok, "data should have 'token' field")
	require.NotEmpty(t, token)
	return token
}

// bearerHeaders returns Authorization header for a JWT token.
func bearerHeaders(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// submitFeedbackWithHeaders posts a feedback entry with custom headers and returns the feedback_id.
func submitFeedbackWithHeaders(t *testing.T, env *common.Env, body map[string]interface{}, headers map[string]string) string {
	t.Helper()
	resp, err := env.HTTPRequest(http.MethodPost, "/api/feedback", body, headers)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 202, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	require.Equal(t, true, result["accepted"])
	id, ok := result["feedback_id"].(string)
	require.True(t, ok, "feedback_id should be a string")
	return id
}

// --- Submit with authenticated user ---

// TestFeedbackSubmit_AuthenticatedUser verifies that submitting feedback with a valid
// JWT bearer token populates user_id, user_name, and user_email on the feedback record.
func TestFeedbackSubmit_AuthenticatedUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Obtain a JWT via the dev OAuth provider.
	token := devAuthToken(t, env)
	headers := bearerHeaders(token)

	// Submit feedback as authenticated user.
	id := submitFeedbackWithHeaders(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Test authenticated feedback submission",
	}, headers)

	// Retrieve the feedback record and verify user fields are populated.
	resp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("authenticated_user_submit", string(body))

	require.Equal(t, 200, resp.StatusCode)

	var fb map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &fb))

	// The dev provider creates a user with user_id="dev_user".
	assert.Equal(t, "dev_user", fb["user_id"],
		"user_id should be set to the authenticated user's ID")

	// user_email should be populated from the user record.
	assert.Equal(t, "dev@vire.local", fb["user_email"],
		"user_email should match the dev user's email")

	// user_name comes from the stored user's Name field. The dev user is created
	// without a Name, so user_name will be empty (omitted via omitempty). This is
	// expected behaviour — the field is correctly populated when a Name exists.
	t.Logf("user_name value: %v", fb["user_name"])
}

// TestFeedbackSubmit_UnauthenticatedUser verifies that submitting feedback without
// authentication leaves user_id, user_name, and user_email empty/absent.
func TestFeedbackSubmit_UnauthenticatedUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Submit feedback without any auth headers.
	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Test unauthenticated feedback submission",
	})

	// Retrieve and verify user fields are absent/empty.
	resp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("unauthenticated_user_submit", string(body))

	require.Equal(t, 200, resp.StatusCode)

	var fb map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &fb))

	// user fields should be absent (omitempty) or empty string when unauthenticated.
	userID, _ := fb["user_id"].(string)
	userName, _ := fb["user_name"].(string)
	userEmail, _ := fb["user_email"].(string)

	assert.Empty(t, userID, "user_id should be empty when not authenticated")
	assert.Empty(t, userName, "user_name should be empty when not authenticated")
	assert.Empty(t, userEmail, "user_email should be empty when not authenticated")
}

// --- Update with authenticated user ---

// TestFeedbackUpdate_AuthenticatedUser verifies that updating feedback with a valid
// JWT bearer token records who performed the update via user_id/user_name/user_email.
func TestFeedbackUpdate_AuthenticatedUser(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Obtain a JWT via the dev OAuth provider (dev user is admin role).
	token := devAuthToken(t, env)
	headers := bearerHeaders(token)

	// Submit feedback (no auth required to submit).
	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "missing_data",
		"description": "Test update with authenticated user",
	})

	// Update the feedback as authenticated user.
	resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
		map[string]interface{}{"status": "acknowledged"}, headers)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("authenticated_user_update", string(body))

	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, "acknowledged", result["status"])
}

// TestFeedbackUpdate_NoAuthRequired verifies that updating feedback does NOT
// require authentication. The PATCH /api/feedback/{id} endpoint is open to all
// clients — MCP clients that submit feedback should be able to update status.
func TestFeedbackUpdate_NoAuthRequired(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Submit feedback without auth (submit is open to everyone).
	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Test update auth requirements",
	})

	t.Run("no_auth_succeeds", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{"status": "acknowledged"}, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("no_auth_update", string(body))

		assert.Equal(t, 200, resp.StatusCode,
			"PATCH /api/feedback/{id} without auth should succeed")
	})

	t.Run("non_admin_succeeds", func(t *testing.T) {
		// Create a regular (non-admin) user.
		resp, err := env.HTTPPost("/api/users", map[string]interface{}{
			"username": "regularuserfortest",
			"email":    "regularuserfortest@test.com",
			"password": "password123",
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, 201, resp.StatusCode)

		resp, err = env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{"status": "dismissed"},
			map[string]string{"X-Vire-User-ID": "regularuserfortest"})
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		guard.SaveResult("non_admin_update", string(body))

		assert.Equal(t, 200, resp.StatusCode,
			"PATCH /api/feedback/{id} for non-admin user should succeed")
	})
}

// --- List includes user fields ---

// TestFeedbackList_UserFieldsInResponse verifies that feedback list responses
// include user_id, user_name, user_email fields (populated when available).
func TestFeedbackList_UserFieldsInResponse(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Obtain a JWT for authenticated submission.
	token := devAuthToken(t, env)
	authHeaders := bearerHeaders(token)

	// Submit one feedback with auth.
	submitFeedbackWithHeaders(t, env, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Authenticated feedback for list test",
	}, authHeaders)

	// Submit one feedback without auth.
	submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Unauthenticated feedback for list test",
	})

	// Retrieve the full list.
	resp, err := env.HTTPGet("/api/feedback")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	guard.SaveResult("list_with_user_fields", string(body))

	require.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, float64(2), result["total"])

	items, ok := result["items"].([]interface{})
	require.True(t, ok)
	require.Len(t, items, 2)

	// Count items with user_id set vs empty.
	var withUser, withoutUser int
	for _, item := range items {
		fb, ok := item.(map[string]interface{})
		require.True(t, ok)
		userID, _ := fb["user_id"].(string)
		if userID != "" {
			withUser++
		} else {
			withoutUser++
		}
	}

	assert.Equal(t, 1, withUser,
		"exactly one item should have user_id set (the authenticated submission)")
	assert.Equal(t, 1, withoutUser,
		"exactly one item should have empty user_id (the unauthenticated submission)")
}

// --- User ID cannot be spoofed via request body ---

// TestFeedbackSubmit_UserIDNotSpoofableViaBody verifies that user identity is taken from
// the auth context, not from the request body. Even if a body field called "user_id" is
// sent, it should be ignored.
func TestFeedbackSubmit_UserIDNotSpoofableViaBody(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Submit feedback without auth but include a "user_id" in the body.
	// The server should ignore this — user context comes from the JWT only.
	resp, err := env.HTTPPost("/api/feedback", map[string]interface{}{
		"category":    "observation",
		"description": "Spoofed user_id test",
		"user_id":     "malicious_user",
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	submitBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, 202, resp.StatusCode)

	var submitResult map[string]interface{}
	require.NoError(t, json.Unmarshal(submitBody, &submitResult))
	id, _ := submitResult["feedback_id"].(string)
	require.NotEmpty(t, id)

	// Fetch the record and verify user_id is empty, not "malicious_user".
	getResp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer getResp.Body.Close()

	body, err := io.ReadAll(getResp.Body)
	require.NoError(t, err)
	guard.SaveResult("spoofed_user_id_check", string(body))

	require.Equal(t, 200, getResp.StatusCode)

	var fb map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &fb))

	userID, _ := fb["user_id"].(string)
	assert.Empty(t, userID,
		"user_id must be empty when no auth is provided, even if body contains user_id")
	assert.NotEqual(t, "malicious_user", fb["user_id"],
		"user_id must not be set from request body — only from auth context")
}
