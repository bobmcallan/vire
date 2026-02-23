package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleFeedbackSubmit_Success(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "BHP price is negative",
		"ticker":      "BHP.AU",
		"severity":    "high",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.True(t, resp["accepted"].(bool))
	assert.NotEmpty(t, resp["feedback_id"])
	assert.Contains(t, resp["feedback_id"].(string), "fb_")
}

func TestHandleFeedbackSubmit_DefaultSeverity(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Something looks off",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Verify the feedback was stored with default severity
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	require.Equal(t, http.StatusOK, getRec.Code)
	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
	assert.Equal(t, "medium", fb["severity"])
	assert.Equal(t, "new", fb["status"])
}

func TestHandleFeedbackSubmit_MissingCategory(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"description": "Something wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackSubmit_InvalidCategory(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "invalid_cat",
		"description": "Something wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackSubmit_EmptyDescription(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "   ",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackSubmit_InvalidSeverity(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Something wrong",
		"severity":    "critical",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackList_Empty(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback", nil)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, float64(0), resp["total"])
	assert.Equal(t, float64(1), resp["page"])
	assert.Equal(t, float64(20), resp["per_page"])
}

func TestHandleFeedbackList_WithItems(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create two feedback entries
	for _, cat := range []string{"data_anomaly", "tool_error"} {
		body := jsonBody(t, map[string]interface{}{
			"category":    cat,
			"description": "Test feedback for " + cat,
		})
		req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
		rec := httptest.NewRecorder()
		srv.handleFeedbackRoot(rec, req)
		require.Equal(t, http.StatusAccepted, rec.Code)
	}

	// List
	req := httptest.NewRequest(http.MethodGet, "/api/feedback", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, float64(2), resp["total"])
	items := resp["items"].([]interface{})
	assert.Len(t, items, 2)
}

func TestHandleFeedbackGet_NotFound(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackGet(rec, req, "nonexistent")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandleFeedbackSummary_Empty(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback/summary", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackSummary(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, float64(0), resp["total"])
}

func TestHandleFeedbackUpdate_RequiresAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/fb_test123", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(rec, req, "fb_test123")

	// No X-Vire-User-ID header, should be 401
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleFeedbackDelete_RequiresAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/feedback/fb_test123", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackDelete(rec, req, "fb_test123")

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleFeedbackBulkUpdate_RequiresAdmin(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"ids":    []string{"fb_1", "fb_2"},
		"status": "dismissed",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/bulk", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackBulkUpdate(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHandleFeedbackSubmit_WithObservedExpectedValues(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":       "calculation_error",
		"description":    "Net return looks wrong",
		"ticker":         "CBA.AU",
		"observed_value": map[string]interface{}{"net_return": -5.2},
		"expected_value": map[string]interface{}{"net_return": 3.1},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Retrieve and check values stored
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	require.Equal(t, http.StatusOK, getRec.Code)
	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
	assert.Equal(t, "CBA.AU", fb["ticker"])
	assert.NotNil(t, fb["observed_value"])
	assert.NotNil(t, fb["expected_value"])
}

func TestHandleFeedbackUpdate_AdminCanUpdate(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create an admin user
	createTestUser(t, srv, "admin_fb", "admin_fb@test.com", "password123", "admin")

	// Submit feedback
	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Test feedback to update",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	fbID := createResp["feedback_id"].(string)

	// Update as admin
	updateBody := jsonBody(t, map[string]interface{}{
		"status":           "acknowledged",
		"resolution_notes": "Looking into it",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateReq.Header.Set("X-Vire-User-ID", "admin_fb")
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	require.Equal(t, http.StatusOK, updateRec.Code)

	var updated map[string]interface{}
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))
	assert.Equal(t, "acknowledged", updated["status"])
	assert.Equal(t, "Looking into it", updated["resolution_notes"])
}

func TestHandleFeedbackDelete_AdminCanDelete(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create an admin user
	createTestUser(t, srv, "admin_del", "admin_del@test.com", "password123", "admin")

	// Submit feedback
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Delete me",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	fbID := createResp["feedback_id"].(string)

	// Delete as admin
	delReq := httptest.NewRequest(http.MethodDelete, "/api/feedback/"+fbID, nil)
	delReq.Header.Set("X-Vire-User-ID", "admin_del")
	delRec := httptest.NewRecorder()
	srv.handleFeedbackDelete(delRec, delReq, fbID)

	assert.Equal(t, http.StatusNoContent, delRec.Code)

	// Verify gone
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	assert.Equal(t, http.StatusNotFound, getRec.Code)
}

func TestHandleFeedbackRoot_MethodNotAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPut, "/api/feedback", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}
