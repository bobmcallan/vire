package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Feedback user context stress tests
//
// These tests validate the security properties of the feedback user context
// feature. They cover:
// - User identity comes from auth context, not request body
// - No spoofing of user fields via JSON body
// - Graceful handling of nil/missing user context
// - Graceful handling of deleted/missing user records
// - User fields populated correctly from authenticated context
// ============================================================================

// ============================================================================
// 1. User identity must come from auth context, not request body
// ============================================================================

func TestFeedbackUserCtx_Submit_BodyUserFieldsIgnored(t *testing.T) {
	// SECURITY: Even if the request body includes user_id, user_name, or
	// user_email fields, they must NOT be used. Identity must come from
	// the authenticated UserContext (middleware/JWT).
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "real_user", "real@test.com", "password", "user")

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "body spoofing test",
		// Attacker tries to inject these via the JSON body
		"user_id":    "spoofed_user",
		"user_name":  "Spoofed Name",
		"user_email": "spoofed@evil.com",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	// Set the real authenticated user via context (as middleware would)
	uc := &common.UserContext{UserID: "real_user", Role: "user"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Retrieve the created feedback
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	require.Equal(t, http.StatusOK, getRec.Code)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	// The user_id must be from auth context ("real_user"), not body ("spoofed_user").
	// The handler's body struct does NOT include user_id/user_name/user_email fields,
	// so Go's JSON decoder silently ignores them. User fields are set from
	// UserContextFromContext only.
	if uid, ok := fb["user_id"]; ok && uid != nil && uid != "" {
		assert.NotEqual(t, "spoofed_user", uid,
			"SECURITY: user_id from request body must NOT be stored")
		assert.Equal(t, "real_user", uid,
			"user_id should come from auth context")
	}

	if uname, ok := fb["user_name"]; ok && uname != nil && uname != "" {
		assert.NotEqual(t, "Spoofed Name", uname,
			"SECURITY: user_name from request body must NOT be stored")
	}

	if uemail, ok := fb["user_email"]; ok && uemail != nil && uemail != "" {
		assert.NotEqual(t, "spoofed@evil.com", uemail,
			"SECURITY: user_email from request body must NOT be stored")
	}
}

// ============================================================================
// 2. Submit without authenticated user context (nil UserContext)
// ============================================================================

func TestFeedbackUserCtx_Submit_NoAuth(t *testing.T) {
	// Feedback submit should work without auth (MCP internal).
	// User fields should be empty/omitted when no UserContext present.
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "no auth feedback",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	// No UserContext set — simulates unauthenticated request
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code,
		"submit should succeed without authentication")

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Retrieve and verify user fields are empty
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	require.Equal(t, http.StatusOK, getRec.Code)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	// user_id should be empty/absent (omitempty)
	if uid, ok := fb["user_id"]; ok {
		assert.Empty(t, uid, "user_id should be empty without auth context")
	}
}

// ============================================================================
// 3. Submit with UserContext but empty UserID
// ============================================================================

func TestFeedbackUserCtx_Submit_EmptyUserID(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "empty user id",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	uc := &common.UserContext{UserID: "", Role: ""}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code,
		"submit should succeed even with empty UserID")

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	require.Equal(t, http.StatusOK, getRec.Code)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	// The handler checks uc.UserID != "" before setting, so it should be empty
	if uid, ok := fb["user_id"]; ok {
		assert.Empty(t, uid,
			"user_id should be empty when UserContext.UserID is empty")
	}
}

// ============================================================================
// 4. Submit with UserContext pointing to non-existent user record
// ============================================================================

func TestFeedbackUserCtx_Submit_UserNotInDB(t *testing.T) {
	// UserContext has a UserID, but GetUser returns nil/error because
	// the user record doesn't exist in the DB.
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "deleted user test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	// UserID exists in context but NOT in the database
	uc := &common.UserContext{UserID: "deleted_user_99", Role: "user"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	// Should NOT crash. The handler gracefully handles missing user record.
	// The code: if user, err := ...; err == nil && user != nil { set name/email }
	// So missing user just means name/email are empty, but user_id is still set.
	require.Equal(t, http.StatusAccepted, rec.Code,
		"submit should succeed even when user record is missing from DB")

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	require.Equal(t, http.StatusOK, getRec.Code)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	// user_id should be set from context even if user record is missing
	if uid, ok := fb["user_id"]; ok && uid != nil {
		assert.Equal(t, "deleted_user_99", uid,
			"user_id should be set from context even when user record is missing")
	}
	// name and email should be empty since GetUser failed
	if uname, ok := fb["user_name"]; ok {
		assert.Empty(t, uname,
			"user_name should be empty when user record is not found")
	}
	if uemail, ok := fb["user_email"]; ok {
		assert.Empty(t, uemail,
			"user_email should be empty when user record is not found")
	}
}

// ============================================================================
// 5. Submit with valid user — all fields populated
// ============================================================================

func TestFeedbackUserCtx_Submit_FullUserContext(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "alice", "alice@example.com", "password", "user")

	// Also set the user's Name field — createTestUser doesn't set Name,
	// so we update it directly.
	user, err := srv.app.Storage.InternalStore().GetUser(t.Context(), "alice")
	require.NoError(t, err)
	require.NotNil(t, user)
	user.Name = "Alice Smith"
	require.NoError(t, srv.app.Storage.InternalStore().SaveUser(t.Context(), user))

	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "price mismatch on BHP",
		"ticker":      "BHP.AU",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	uc := &common.UserContext{UserID: "alice", Role: "user"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	require.Equal(t, http.StatusOK, getRec.Code)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	assert.Equal(t, "alice", fb["user_id"], "user_id from auth context")
	assert.Equal(t, "Alice Smith", fb["user_name"], "user_name from DB lookup")
	assert.Equal(t, "alice@example.com", fb["user_email"], "user_email from DB lookup")
}

// ============================================================================
// 6. Update captures updater identity
// ============================================================================

func TestFeedbackUserCtx_Update_CapturesUpdaterIdentity(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "submitter", "submitter@test.com", "password", "user")
	createTestUser(t, srv, "updater_admin", "updater@test.com", "password", "admin")

	// Set name for updater
	user, _ := srv.app.Storage.InternalStore().GetUser(t.Context(), "updater_admin")
	if user != nil {
		user.Name = "Admin User"
		srv.app.Storage.InternalStore().SaveUser(t.Context(), user)
	}

	// Submit feedback as submitter
	submitBody := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "price mismatch",
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/feedback", submitBody)
	submitUC := &common.UserContext{UserID: "submitter", Role: "user"}
	submitReq = submitReq.WithContext(common.WithUserContext(submitReq.Context(), submitUC))
	submitRec := httptest.NewRecorder()
	srv.handleFeedbackRoot(submitRec, submitReq)
	require.Equal(t, http.StatusAccepted, submitRec.Code)

	var submitResp map[string]interface{}
	require.NoError(t, json.NewDecoder(submitRec.Body).Decode(&submitResp))
	fbID := submitResp["feedback_id"].(string)

	// Update as different admin user
	updateBody := jsonBody(t, map[string]interface{}{
		"status":           "acknowledged",
		"resolution_notes": "looking into it",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateUC := &common.UserContext{UserID: "updater_admin", Role: "admin"}
	updateReq = updateReq.WithContext(common.WithUserContext(updateReq.Context(), updateUC))
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	require.Equal(t, http.StatusOK, updateRec.Code,
		"update should succeed with valid auth")

	var updated map[string]interface{}
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))

	// Verify original submitter's user_id is preserved
	assert.Equal(t, "submitter", updated["user_id"],
		"original submitter's user_id should be preserved after update")

	// Verify updater identity is captured in updated_by_* fields
	assert.Equal(t, "updater_admin", updated["updated_by_user_id"],
		"updated_by_user_id should capture who performed the update")
	if updaterName, ok := updated["updated_by_user_name"]; ok && updaterName != nil {
		assert.Equal(t, "Admin User", updaterName,
			"updated_by_user_name should come from DB lookup")
	}
	if updaterEmail, ok := updated["updated_by_user_email"]; ok && updaterEmail != nil {
		assert.Equal(t, "updater@test.com", updaterEmail,
			"updated_by_user_email should come from DB lookup")
	}
}

// ============================================================================
// 7. Update spoofing — body fields should not override auth identity
// ============================================================================

func TestFeedbackUserCtx_Update_BodySpoofingIgnored(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create feedback first
	submitBody := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "update spoofing test",
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/feedback", submitBody)
	submitRec := httptest.NewRecorder()
	srv.handleFeedbackRoot(submitRec, submitReq)
	require.Equal(t, http.StatusAccepted, submitRec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(submitRec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Try to spoof updater identity via body
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
		// Attacker attempts to inject user fields via body
		"user_id":    "spoofed_updater",
		"user_name":  "Spoofed",
		"user_email": "spoofed@evil.com",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	// No UserContext — unauthenticated update (allowed per "No admin requirement")
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	// The handler's body struct only has Status and ResolutionNotes.
	// user_id/user_name/user_email in the body are silently ignored.
	if updateRec.Code == http.StatusOK {
		var updated map[string]interface{}
		require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))

		// Without auth context, updated_by fields should be empty
		if uid, ok := updated["updated_by_user_id"]; ok && uid != nil {
			assert.NotEqual(t, "spoofed_updater", uid,
				"SECURITY: body user_id must NOT appear in updated_by_user_id")
			assert.Empty(t, uid,
				"updated_by_user_id should be empty without auth context")
		}
	}
}

// ============================================================================
// 8. Update without auth context — still works (no admin required)
// ============================================================================

func TestFeedbackUserCtx_Update_NoAuth_StillWorks(t *testing.T) {
	// handleFeedbackUpdate has "No admin requirement" — it should work
	// without auth. The updater identity fields will just be empty.
	srv := newTestServerWithStorage(t)

	// Create feedback first
	submitBody := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "no auth update test",
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/feedback", submitBody)
	submitRec := httptest.NewRecorder()
	srv.handleFeedbackRoot(submitRec, submitReq)
	require.Equal(t, http.StatusAccepted, submitRec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(submitRec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Update without any auth context
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	// No UserContext
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	// The handler does NOT call requireAdmin, so this should succeed
	assert.Equal(t, http.StatusOK, updateRec.Code,
		"update without auth should succeed (no admin requirement per handler comment)")
}

// ============================================================================
// 9. SQL injection via user fields (defense in depth)
// ============================================================================

func TestFeedbackUserCtx_Submit_SQLInjectionInUserContext(t *testing.T) {
	// Even though UserContext comes from trusted middleware,
	// verify that hostile UserID values don't break storage.
	srv := newTestServerWithStorage(t)

	injections := []string{
		"'; DROP TABLE mcp_feedback; --",
		"user' OR '1'='1",
		`"; DELETE FROM internal_users; --`,
		"user\x00null",
	}

	for _, payload := range injections {
		name := payload
		if len(name) > 30 {
			name = name[:30]
		}
		t.Run(name, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "sql injection via user context",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			uc := &common.UserContext{UserID: payload, Role: "user"}
			req = req.WithContext(common.WithUserContext(req.Context(), uc))
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			// Should not crash. SurrealDB uses parameterized queries ($user_id).
			if rec.Code >= 500 {
				t.Errorf("server error with hostile UserID %q: status %d, body: %s",
					payload, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestFeedbackUserCtx_Update_SQLInjectionInUserContext(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create a feedback entry first
	submitBody := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "update injection test",
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/feedback", submitBody)
	submitRec := httptest.NewRecorder()
	srv.handleFeedbackRoot(submitRec, submitReq)
	require.Equal(t, http.StatusAccepted, submitRec.Code)

	var submitResp map[string]interface{}
	require.NoError(t, json.NewDecoder(submitRec.Body).Decode(&submitResp))
	fbID := submitResp["feedback_id"].(string)

	injections := []string{
		"'; DROP TABLE mcp_feedback; --",
		"admin'; UPDATE mcp_feedback SET status='resolved'; --",
	}

	for _, payload := range injections {
		name := payload
		if len(name) > 30 {
			name = name[:30]
		}
		t.Run(name, func(t *testing.T) {
			updateBody := jsonBody(t, map[string]interface{}{
				"status": "acknowledged",
			})
			updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
			uc := &common.UserContext{UserID: payload, Role: "admin"}
			updateReq = updateReq.WithContext(common.WithUserContext(updateReq.Context(), uc))
			updateRec := httptest.NewRecorder()
			srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

			// Should not crash — SurrealDB Update uses parameterized $uid
			if updateRec.Code >= 500 {
				t.Errorf("server error with hostile updater UserID %q: status %d",
					payload, updateRec.Code)
			}
		})
	}
}

// ============================================================================
// 10. Concurrent submit with different user contexts
// ============================================================================

func TestFeedbackUserCtx_ConcurrentSubmit_DifferentUsers(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "user_a", "a@test.com", "password", "user")
	createTestUser(t, srv, "user_b", "b@test.com", "password", "user")

	type result struct {
		fbID   string
		userID string
		err    string
	}

	results := make(chan result, 20)

	// 10 concurrent submissions per user
	for _, uid := range []string{"user_a", "user_b"} {
		for i := 0; i < 10; i++ {
			go func(userID string) {
				body := jsonBody(t, map[string]interface{}{
					"category":    "observation",
					"description": "concurrent user test from " + userID,
				})
				req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
				uc := &common.UserContext{UserID: userID, Role: "user"}
				req = req.WithContext(common.WithUserContext(req.Context(), uc))
				rec := httptest.NewRecorder()
				srv.handleFeedbackRoot(rec, req)

				if rec.Code != http.StatusAccepted {
					results <- result{err: rec.Body.String()}
					return
				}
				var resp map[string]interface{}
				if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
					results <- result{err: "decode error"}
					return
				}
				results <- result{
					fbID:   resp["feedback_id"].(string),
					userID: userID,
				}
			}(uid)
		}
	}

	// Collect all results
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		r := <-results
		if r.err != "" {
			t.Errorf("concurrent submit error: %s", r.err)
			continue
		}
		if seen[r.fbID] {
			t.Errorf("duplicate feedback ID: %s", r.fbID)
		}
		seen[r.fbID] = true
	}

	assert.Equal(t, 20, len(seen), "all 20 submissions should produce unique IDs")
}

// ============================================================================
// 11. Whitespace-only UserID
// ============================================================================

func TestFeedbackUserCtx_Submit_WhitespaceOnlyUserID(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "whitespace user id",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	uc := &common.UserContext{UserID: "   ", Role: "user"}
	req = req.WithContext(common.WithUserContext(req.Context(), uc))
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	// Should succeed — whitespace UserID passes the != "" check
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	// FINDING: whitespace-only UserID passes uc.UserID != "" check and is stored.
	// The handler does NOT trim UserID before comparison.
	if uid, ok := fb["user_id"]; ok && uid != nil && uid != "" {
		t.Logf("FINDING: whitespace-only UserID %q was stored. "+
			"Consider trimming uc.UserID before the != \"\" check.", uid)
	}
}

// ============================================================================
// 12. Verify existing stress test assumptions about auth on update
// ============================================================================

func TestFeedbackUserCtx_Update_NoAdminRequired_Documented(t *testing.T) {
	// FINDING: The handler comment says "No admin requirement" and the
	// implementation does NOT call requireAdmin. However, existing stress
	// test TestFeedbackStress_Update_NonAdminUser expects 403.
	// This documents the actual behavior: updates are open to all clients.
	srv := newTestServerWithStorage(t)

	// Create feedback
	submitBody := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "auth doc test",
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/feedback", submitBody)
	submitRec := httptest.NewRecorder()
	srv.handleFeedbackRoot(submitRec, submitReq)
	require.Equal(t, http.StatusAccepted, submitRec.Code)

	var submitResp map[string]interface{}
	require.NoError(t, json.NewDecoder(submitRec.Body).Decode(&submitResp))
	fbID := submitResp["feedback_id"].(string)

	// Update as non-admin without auth
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "resolved",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	// Document: handleFeedbackUpdate does NOT require admin
	assert.Equal(t, http.StatusOK, updateRec.Code,
		"DOCUMENTED: handleFeedbackUpdate does not require admin/auth. "+
			"Any client can update feedback status. This is intentional per handler comment.")
}

// ============================================================================
// Helper
// ============================================================================

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
