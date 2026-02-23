package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 1. Input validation — hostile payloads at the handler level
// ============================================================================

func TestFeedbackStress_Submit_SQLInjectionInCategory(t *testing.T) {
	srv := newTestServerWithStorage(t)

	injections := []string{
		"'; DROP TABLE mcp_feedback; --",
		"data_anomaly' OR '1'='1",
		`data_anomaly"; DELETE FROM mcp_feedback; --`,
	}

	for _, payload := range injections {
		t.Run(payload[:min(len(payload), 30)], func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    payload,
				"description": "injection test",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			// Invalid category should be rejected at validation
			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"SQL injection in category should be rejected by validation")
		})
	}
}

func TestFeedbackStress_Submit_SQLInjectionInDescription(t *testing.T) {
	srv := newTestServerWithStorage(t)

	injections := []string{
		"'; DROP TABLE mcp_feedback; --",
		`"; DELETE FROM mcp_feedback WHERE true; --`,
		"test'); UPDATE mcp_feedback SET status='resolved'; --",
	}

	for _, payload := range injections {
		t.Run(payload[:min(len(payload), 30)], func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": payload,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			// Description passes validation — SQL injection must be prevented
			// by parameterized queries. Should return 202.
			require.Equal(t, http.StatusAccepted, rec.Code)

			// Verify the payload was stored literally
			var resp map[string]interface{}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			fbID := resp["feedback_id"].(string)

			getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
			getRec := httptest.NewRecorder()
			srv.handleFeedbackGet(getRec, getReq, fbID)

			require.Equal(t, http.StatusOK, getRec.Code)
			var fb map[string]interface{}
			require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
			assert.Equal(t, payload, fb["description"],
				"SQL injection payload should be stored literally, not executed")
		})
	}
}

func TestFeedbackStress_Submit_XSSInDescription(t *testing.T) {
	srv := newTestServerWithStorage(t)

	xssPayloads := []string{
		`<script>alert('xss')</script>`,
		`<img src=x onerror=alert(1)>`,
		`<svg/onload=alert(1)>`,
		`" onclick="alert(1)`,
		`javascript:alert(1)`,
	}

	for _, payload := range xssPayloads {
		t.Run(payload[:min(len(payload), 25)], func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": payload,
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			require.Equal(t, http.StatusAccepted, rec.Code)

			// Response is JSON (Content-Type: application/json), so XSS won't
			// execute in a browser. But verify the response is proper JSON.
			var resp map[string]interface{}
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp),
				"response should be valid JSON even with XSS payload")
		})
	}
}

func TestFeedbackStress_Submit_VeryLargeDescription(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// FINDING: No body size limit. DecodeJSON uses json.NewDecoder without
	// http.MaxBytesReader. A malicious client could send a multi-GB body.
	sizes := []struct {
		name string
		size int
	}{
		{"10KB", 10 * 1024},
		{"100KB", 100 * 1024},
		{"1MB", 1024 * 1024},
	}

	for _, tc := range sizes {
		t.Run(tc.name, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": strings.Repeat("A", tc.size),
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			// Should not crash
			if rec.Code >= 500 {
				t.Errorf("server error with %s description: status %d", tc.name, rec.Code)
			}
		})
	}

	t.Log("FINDING: No request body size limit on POST /api/feedback. " +
		"DecodeJSON does not use http.MaxBytesReader. A malicious client could " +
		"send an arbitrarily large body, consuming server memory. " +
		"Recommendation: add http.MaxBytesReader in DecodeJSON or middleware.")
}

func TestFeedbackStress_Submit_NullBody(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPost, "/api/feedback", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFeedbackStress_Submit_MalformedJSON(t *testing.T) {
	srv := newTestServerWithStorage(t)

	malformed := []string{
		"not json",
		"{invalid",
		"[]",
		`{"category": }`,
		"null",
		"",
		"{{{{",
		`{"category": "observation", "description": `,
	}

	for _, body := range malformed {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			if rec.Code >= 500 {
				t.Errorf("server error with malformed JSON %q: status %d", body, rec.Code)
			}
		})
	}
}

// ============================================================================
// 2. Authentication/Authorization — admin endpoints
// ============================================================================

func TestFeedbackStress_Update_NonAdminUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create a regular (non-admin) user
	createTestUser(t, srv, "regular_user", "regular@test.com", "password", "user")

	body := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/fb_test", body)
	req.Header.Set("X-Vire-User-ID", "regular_user")
	rec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(rec, req, "fb_test")

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"non-admin user should get 403 Forbidden on PATCH")
}

func TestFeedbackStress_Delete_NonAdminUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	createTestUser(t, srv, "regular_del", "regular_del@test.com", "password", "user")

	req := httptest.NewRequest(http.MethodDelete, "/api/feedback/fb_test", nil)
	req.Header.Set("X-Vire-User-ID", "regular_del")
	rec := httptest.NewRecorder()
	srv.handleFeedbackDelete(rec, req, "fb_test")

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"non-admin user should get 403 Forbidden on DELETE")
}

func TestFeedbackStress_BulkUpdate_NonAdminUser(t *testing.T) {
	srv := newTestServerWithStorage(t)

	createTestUser(t, srv, "regular_bulk", "regular_bulk@test.com", "password", "user")

	body := jsonBody(t, map[string]interface{}{
		"ids":    []string{"fb_1"},
		"status": "dismissed",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/bulk", body)
	req.Header.Set("X-Vire-User-ID", "regular_bulk")
	rec := httptest.NewRecorder()
	srv.handleFeedbackBulkUpdate(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code,
		"non-admin user should get 403 Forbidden on bulk update")
}

func TestFeedbackStress_Submit_NoAuth(t *testing.T) {
	// Submit should work without authentication (MCP gateway internal)
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "No auth needed",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	// No X-Vire-User-ID header
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code,
		"POST /api/feedback should not require authentication")
}

func TestFeedbackStress_List_NoAuth(t *testing.T) {
	// FINDING: GET /api/feedback (list) does not require authentication.
	// This means any client can enumerate all feedback entries.
	// For an MCP server this may be acceptable, but worth noting.
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code,
		"GET /api/feedback should work without auth")
	t.Log("NOTE: GET /api/feedback does not require authentication. " +
		"All feedback entries are publicly readable.")
}

func TestFeedbackStress_Summary_NoAuth(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/summary", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackSummary(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code,
		"GET /api/feedback/summary should work without auth")
}

func TestFeedbackStress_Get_NoAuth(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create a feedback entry first
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "readable by anyone",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Read without auth
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	assert.Equal(t, http.StatusOK, getRec.Code)
}

func TestFeedbackStress_Update_MissingUserIDHeader(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/fb_test", body)
	// No X-Vire-User-ID header
	rec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(rec, req, "fb_test")

	assert.Equal(t, http.StatusUnauthorized, rec.Code,
		"missing X-Vire-User-ID should return 401")
}

// ============================================================================
// 3. Edge cases — pagination, routing, method enforcement
// ============================================================================

func TestFeedbackStress_List_PaginationBoundaries(t *testing.T) {
	srv := newTestServerWithStorage(t)

	cases := []struct {
		name   string
		query  string
		expect int // expected HTTP status
	}{
		{"page_0", "?page=0", http.StatusOK},
		{"page_negative", "?page=-1", http.StatusOK},
		{"page_string", "?page=abc", http.StatusOK},
		{"per_page_0", "?per_page=0", http.StatusOK},
		{"per_page_negative", "?per_page=-1", http.StatusOK},
		{"per_page_over_100", "?per_page=9999", http.StatusOK},
		{"per_page_string", "?per_page=abc", http.StatusOK},
		{"page_very_large", "?page=999999999", http.StatusOK},
		{"per_page_very_large", "?per_page=999999999", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/feedback"+tc.query, nil)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)
			assert.Equal(t, tc.expect, rec.Code)
		})
	}
}

func TestFeedbackStress_List_HostileQueryParams(t *testing.T) {
	srv := newTestServerWithStorage(t)

	hostileQueries := []string{
		"?status='; DROP TABLE mcp_feedback; --",
		"?category=' OR 1=1; --",
		"?ticker=BHP'; DELETE mcp_feedback; --",
		"?session_id=test%00null%00byte",
		`?sort='; DELETE FROM mcp_feedback; --`,
		"?since=not-a-date",
		"?before=not-a-date",
		"?since=2026-01-01T00:00:00Z&before=2025-01-01T00:00:00Z", // since > before
	}

	for _, query := range hostileQueries {
		name := query
		if len(name) > 40 {
			name = name[:40]
		}
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/feedback"+query, nil)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			if rec.Code >= 500 {
				t.Errorf("server error with hostile query %s: status %d", query, rec.Code)
			}
		})
	}
}

func TestFeedbackStress_Route_PathTraversal(t *testing.T) {
	srv := newTestServerWithStorage(t)

	traversalPaths := []string{
		"/api/feedback/../admin/jobs",
		"/api/feedback/../../etc/passwd",
		"/api/feedback/%2e%2e/admin",
		"/api/feedback/..%2f..%2fadmin",
	}

	for _, path := range traversalPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			srv.routeFeedback(rec, req)

			// Path traversal attempts should be treated as feedback IDs
			// and return 404 (not found) — not leak other endpoints
			if rec.Code >= 500 {
				t.Errorf("server error with path traversal %s: status %d", path, rec.Code)
			}
		})
	}
}

func TestFeedbackStress_Route_MethodEnforcement(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Test each endpoint with disallowed methods
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"root_put", http.MethodPut, "/api/feedback"},
		{"root_delete", http.MethodDelete, "/api/feedback"},
		{"root_patch", http.MethodPatch, "/api/feedback"},
		{"summary_post", http.MethodPost, "/api/feedback/summary"},
		{"summary_put", http.MethodPut, "/api/feedback/summary"},
		{"summary_delete", http.MethodDelete, "/api/feedback/summary"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()

			if tc.path == "/api/feedback" {
				srv.handleFeedbackRoot(rec, req)
			} else {
				srv.routeFeedback(rec, req)
			}

			assert.Equal(t, http.StatusMethodNotAllowed, rec.Code,
				"%s %s should return 405", tc.method, tc.path)
		})
	}
}

// ============================================================================
// 4. Data integrity — error response leaks
// ============================================================================

func TestFeedbackStress_Submit_ErrorResponse_NoInternalDetails(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category": "invalid",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	respBody := rec.Body.String()
	assertNoSensitiveData(t, respBody)
}

func TestFeedbackStress_Get_NonExistentID(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/fb_doesnotexist", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackGet(rec, req, "fb_doesnotexist")

	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Error response should not leak internal details
	assertNoSensitiveData(t, rec.Body.String())
}

// ============================================================================
// 5. Concurrent access — race conditions
// ============================================================================

func TestFeedbackStress_ConcurrentSubmits(t *testing.T) {
	srv := newTestServerWithStorage(t)

	var wg sync.WaitGroup
	errors := make(chan string, 100)
	ids := make(chan string, 50)

	// 50 concurrent feedback submissions
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "concurrent test",
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			if rec.Code != http.StatusAccepted {
				errors <- rec.Body.String()
				return
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				errors <- "failed to decode response"
				return
			}
			ids <- resp["feedback_id"].(string)
		}(i)
	}

	wg.Wait()
	close(errors)
	close(ids)

	for err := range errors {
		t.Errorf("concurrent submit error: %s", err)
	}

	// Verify all IDs are unique
	seen := make(map[string]bool)
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate feedback ID in concurrent submission: %s", id)
		}
		seen[id] = true
	}

	if len(seen) < 50 {
		t.Errorf("expected 50 unique IDs, got %d", len(seen))
	}
}

// ============================================================================
// 6. Update validation edge cases
// ============================================================================

func TestFeedbackStress_Update_InvalidStatus(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_inv", "admin_inv@test.com", "password", "admin")

	// Submit feedback first
	submitBody := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "update test",
	})
	submitReq := httptest.NewRequest(http.MethodPost, "/api/feedback", submitBody)
	submitRec := httptest.NewRecorder()
	srv.handleFeedbackRoot(submitRec, submitReq)
	require.Equal(t, http.StatusAccepted, submitRec.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.NewDecoder(submitRec.Body).Decode(&createResp))
	fbID := createResp["feedback_id"].(string)

	// Try to update with invalid status
	invalidStatuses := []string{
		"invalid",
		"RESOLVED",
		"New",
		"'; DROP TABLE mcp_feedback; --",
		"",
	}

	for _, status := range invalidStatuses {
		name := status
		if name == "" {
			name = "(empty)"
		}
		t.Run(name, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"status": status,
			})
			req := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, body)
			req.Header.Set("X-Vire-User-ID", "admin_inv")
			rec := httptest.NewRecorder()
			srv.handleFeedbackUpdate(rec, req, fbID)

			// Empty status is treated as "keep existing" — should succeed
			if status == "" {
				assert.Equal(t, http.StatusOK, rec.Code,
					"empty status should keep existing status")
			} else {
				assert.Equal(t, http.StatusBadRequest, rec.Code,
					"invalid status %q should be rejected", status)
			}
		})
	}
}

func TestFeedbackStress_Update_NonExistentID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_ne", "admin_ne@test.com", "password", "admin")

	body := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/fb_nonexistent", body)
	req.Header.Set("X-Vire-User-ID", "admin_ne")
	rec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(rec, req, "fb_nonexistent")

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"updating non-existent feedback should return 404")
}

// ============================================================================
// 7. Bulk update edge cases
// ============================================================================

func TestFeedbackStress_BulkUpdate_EmptyIDs(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_bulk_empty", "admin_bulk_empty@test.com", "password", "admin")

	body := jsonBody(t, map[string]interface{}{
		"ids":    []string{},
		"status": "acknowledged",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/bulk", body)
	req.Header.Set("X-Vire-User-ID", "admin_bulk_empty")
	rec := httptest.NewRecorder()
	srv.handleFeedbackBulkUpdate(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"empty IDs array should be rejected")
}

func TestFeedbackStress_BulkUpdate_MissingStatus(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_bulk_ns", "admin_bulk_ns@test.com", "password", "admin")

	body := jsonBody(t, map[string]interface{}{
		"ids": []string{"fb_1"},
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/bulk", body)
	req.Header.Set("X-Vire-User-ID", "admin_bulk_ns")
	rec := httptest.NewRecorder()
	srv.handleFeedbackBulkUpdate(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"missing status should be rejected")
}

func TestFeedbackStress_BulkUpdate_InvalidStatus(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_bulk_is", "admin_bulk_is@test.com", "password", "admin")

	body := jsonBody(t, map[string]interface{}{
		"ids":    []string{"fb_1"},
		"status": "INVALID",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/bulk", body)
	req.Header.Set("X-Vire-User-ID", "admin_bulk_is")
	rec := httptest.NewRecorder()
	srv.handleFeedbackBulkUpdate(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"invalid status should be rejected")
}

// ============================================================================
// 8. Delete edge cases
// ============================================================================

func TestFeedbackStress_Delete_NonExistentID(t *testing.T) {
	// FINDING: Delete on non-existent ID returns 204 (success).
	// SurrealDB delete on non-existent record ID does not error.
	// The handler does not check existence before deleting.
	// This is idempotent behavior, which is generally fine for DELETE.
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_del_ne", "admin_del_ne@test.com", "password", "admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/feedback/fb_nonexistent", nil)
	req.Header.Set("X-Vire-User-ID", "admin_del_ne")
	rec := httptest.NewRecorder()
	srv.handleFeedbackDelete(rec, req, "fb_nonexistent")

	// 204 is acceptable for idempotent DELETE on non-existent resource
	assert.Equal(t, http.StatusNoContent, rec.Code)
	t.Log("NOTE: DELETE on non-existent feedback ID returns 204 (idempotent). " +
		"This is acceptable REST behavior but means the client cannot distinguish " +
		"between 'deleted successfully' and 'never existed'.")
}

func TestFeedbackStress_Delete_HostileID(t *testing.T) {
	srv := newTestServerWithStorage(t)
	createTestUser(t, srv, "admin_del_h", "admin_del_h@test.com", "password", "admin")

	// Create a canary
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "canary for delete test",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	canaryID := resp["feedback_id"].(string)

	// Try hostile delete IDs
	hostileIDs := []string{
		"mcp_feedback:*",
		"*",
		"'; DROP TABLE mcp_feedback; --",
	}

	for _, id := range hostileIDs {
		delReq := httptest.NewRequest(http.MethodDelete, "/api/feedback/"+id, nil)
		delReq.Header.Set("X-Vire-User-ID", "admin_del_h")
		delRec := httptest.NewRecorder()
		srv.handleFeedbackDelete(delRec, delReq, id)
		// Should not crash
		if delRec.Code >= 500 {
			t.Errorf("server error with hostile delete ID %q: status %d", id, delRec.Code)
		}
	}

	// Verify canary still exists
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+canaryID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, canaryID)
	assert.Equal(t, http.StatusOK, getRec.Code,
		"canary should still exist after hostile delete attempts")
}

// ============================================================================
// 9. Diagnostics feedback inclusion
// ============================================================================

func TestFeedbackStress_Diagnostics_IncludeFeedback(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit some feedback
	for i := 0; i < 3; i++ {
		body := jsonBody(t, map[string]interface{}{
			"category":    "observation",
			"description": "diagnostics test",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
		rec := httptest.NewRecorder()
		srv.handleFeedbackRoot(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)
	}

	// Request diagnostics with include_feedback=true
	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics?include_feedback=true", nil)
	rec := httptest.NewRecorder()
	srv.handleDiagnostics(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	recentFeedback, ok := resp["recent_feedback"]
	require.True(t, ok, "recent_feedback should be present when include_feedback=true")
	fbData := recentFeedback.(map[string]interface{})
	assert.Equal(t, float64(3), fbData["total"])
}

func TestFeedbackStress_Diagnostics_NoFeedbackByDefault(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/diagnostics", nil)
	rec := httptest.NewRecorder()
	srv.handleDiagnostics(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	_, ok := resp["recent_feedback"]
	assert.False(t, ok, "recent_feedback should NOT be present without include_feedback=true")
}

// ============================================================================
// 10. Severity sort semantic bug documentation
// ============================================================================

func TestFeedbackStress_SeveritySortOrder(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit feedback with different severities
	for _, sev := range []string{"low", "medium", "high"} {
		body := jsonBody(t, map[string]interface{}{
			"category":    "observation",
			"description": "severity sort test " + sev,
			"severity":    sev,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
		rec := httptest.NewRecorder()
		srv.handleFeedbackRoot(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)
	}

	// List with severity_desc sort
	req := httptest.NewRequest(http.MethodGet, "/api/feedback?sort=severity_desc", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	items := resp["items"].([]interface{})
	require.Len(t, items, 3)

	// Check the order
	severities := make([]string, 3)
	for i, item := range items {
		fb := item.(map[string]interface{})
		severities[i] = fb["severity"].(string)
	}

	// SurrealDB sorts strings lexicographically: "medium" > "low" > "high"
	// This is NOT the desired order (high > medium > low)
	if severities[0] == "medium" && severities[1] == "low" && severities[2] == "high" {
		t.Log("CONFIRMED: severity_desc sort uses lexicographic ordering (medium > low > high). " +
			"High severity items sort LAST. This is a semantic bug. " +
			"Fix: use a CASE expression, e.g. ORDER BY (CASE severity WHEN 'high' THEN 3 WHEN 'medium' THEN 2 ELSE 1 END) DESC")
	}
}

// ============================================================================
// 11. Routing edge cases via routeFeedback
// ============================================================================

func TestFeedbackStress_Route_SpecialSubpaths(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// These paths should be treated as feedback IDs, not special routes
	specialPaths := []string{
		"/api/feedback/summaries", // note: not "summary"
		"/api/feedback/bulks",     // note: not "bulk"
		"/api/feedback/summary/extra",
		"/api/feedback/bulk/extra",
	}

	for _, path := range specialPaths {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			srv.routeFeedback(rec, req)

			// "summaries" and "bulks" should be treated as feedback IDs
			// and return 404 (not found)
			if path == "/api/feedback/summaries" || path == "/api/feedback/bulks" {
				assert.Equal(t, http.StatusNotFound, rec.Code,
					"%s should return 404 (treated as ID lookup)", path)
			}
		})
	}
}
