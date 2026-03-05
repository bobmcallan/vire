package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bobmcallan/vire/internal/common"
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

func TestHandleFeedbackUpdate_NoAuthRequired(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// handleFeedbackUpdate does NOT require admin — any client can update.
	// Non-existent feedback ID returns 404.
	body := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/feedback/fb_test123", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(rec, req, "fb_test123")

	assert.Equal(t, http.StatusNotFound, rec.Code)
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
		"observed_value": map[string]interface{}{"holding_return_net": -5.2},
		"expected_value": map[string]interface{}{"holding_return_net": 3.1},
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

func TestHandleFeedbackSubmit_WithUserContext(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create a user so the handler can look up name/email
	createTestUser(t, srv, "submitter1", "submitter1@test.com", "password123", "user")

	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Price looks wrong",
		"ticker":      "BHP.AU",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	// Inject UserContext into request context
	ctx := common.WithUserContext(req.Context(), &common.UserContext{UserID: "submitter1"})
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Retrieve and verify user fields are populated
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	require.Equal(t, http.StatusOK, getRec.Code)
	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
	assert.Equal(t, "submitter1", fb["user_id"])
	assert.Equal(t, "submitter1@test.com", fb["user_email"])
}

func TestHandleFeedbackSubmit_WithoutUserContext(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "No auth context",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()

	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

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
	// user_id should be absent (omitempty) or empty
	_, hasUserID := fb["user_id"]
	if hasUserID {
		assert.Empty(t, fb["user_id"])
	}
}

func TestHandleFeedbackRoot_MethodNotAllowed(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodPut, "/api/feedback", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

// --- Bug fix tests ---

func TestHandleFeedbackUpdate_PreservesResolutionNotes(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create an admin user for the update
	createTestUser(t, srv, "admin_notes", "admin_notes@test.com", "password123", "admin")

	// Submit feedback
	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Test resolution notes preservation",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	fbID := createResp["feedback_id"].(string)

	// Step 1: Update with both status and resolution notes
	updateBody := jsonBody(t, map[string]interface{}{
		"status":           "acknowledged",
		"resolution_notes": "Investigating the anomaly",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateReq.Header.Set("X-Vire-User-ID", "admin_notes")
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)
	require.Equal(t, http.StatusOK, updateRec.Code)

	var updateResp map[string]interface{}
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updateResp))
	assert.Equal(t, "acknowledged", updateResp["status"])
	assert.Equal(t, "Investigating the anomaly", updateResp["resolution_notes"])

	// Step 2: Update ONLY status — resolution_notes should be preserved
	statusOnlyBody := jsonBody(t, map[string]interface{}{
		"status": "resolved",
	})
	statusReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, statusOnlyBody)
	statusReq.Header.Set("X-Vire-User-ID", "admin_notes")
	statusRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(statusRec, statusReq, fbID)
	require.Equal(t, http.StatusOK, statusRec.Code)

	var statusResp map[string]interface{}
	require.NoError(t, json.NewDecoder(statusRec.Body).Decode(&statusResp))
	assert.Equal(t, "resolved", statusResp["status"])
	assert.Equal(t, "Investigating the anomaly", statusResp["resolution_notes"],
		"resolution_notes should be preserved when not provided in PATCH body")
}

func TestHandleFeedbackList_InvalidStatusFilter(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback?status=invalid_status", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackList_InvalidSeverityFilter(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback?severity=critical", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackList_InvalidCategoryFilter(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback?category=not_a_category", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleFeedbackList_ValidFiltersAccepted(t *testing.T) {
	srv := newTestServerWithStorage(t)

	req := httptest.NewRequest(http.MethodGet, "/api/feedback?status=new&severity=high&category=data_anomaly", nil)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

// --- Attachment tests ---

func TestHandleFeedbackSubmit_WithAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	pngData := base64.StdEncoding.EncodeToString([]byte("fake-png-data"))
	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Screenshot of bad data",
		"attachments": []map[string]interface{}{
			{
				"filename":     "screenshot.png",
				"content_type": "image/png",
				"data":         pngData,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Retrieve and verify attachments
	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	require.Equal(t, http.StatusOK, getRec.Code)
	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
	atts, ok := fb["attachments"].([]interface{})
	require.True(t, ok, "attachments should be an array")
	require.Len(t, atts, 1)
	att := atts[0].(map[string]interface{})
	assert.Equal(t, "screenshot.png", att["filename"])
	assert.Equal(t, "image/png", att["content_type"])
	assert.Equal(t, pngData, att["data"])
	assert.Equal(t, float64(len("fake-png-data")), att["size_bytes"])
}

func TestHandleFeedbackSubmit_TooManyAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	attachments := make([]map[string]interface{}, 11)
	for i := range attachments {
		attachments[i] = map[string]interface{}{
			"filename":     "file.png",
			"content_type": "image/png",
			"data":         base64.StdEncoding.EncodeToString([]byte("x")),
		}
	}
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Too many files",
		"attachments": attachments,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "too many attachments")
}

func TestHandleFeedbackSubmit_InvalidContentType(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Bad type",
		"attachments": []map[string]interface{}{
			{
				"filename":     "doc.pdf",
				"content_type": "application/pdf",
				"data":         base64.StdEncoding.EncodeToString([]byte("pdf")),
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "unsupported content_type")
}

func TestHandleFeedbackSubmit_InvalidBase64(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Bad base64",
		"attachments": []map[string]interface{}{
			{
				"filename":     "img.png",
				"content_type": "image/png",
				"data":         "not-valid-base64!!!",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "invalid base64")
}

func TestHandleFeedbackSubmit_OversizedAttachment(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Create data slightly over 5MB
	bigData := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 5*1024*1024+1)))
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Huge file",
		"attachments": []map[string]interface{}{
			{
				"filename":     "big.png",
				"content_type": "image/png",
				"data":         bigData,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "exceeds max size")
}

func TestHandleFeedbackSubmit_MissingFilename(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "No filename",
		"attachments": []map[string]interface{}{
			{
				"content_type": "image/png",
				"data":         base64.StdEncoding.EncodeToString([]byte("x")),
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "filename is required")
}

func TestHandleFeedbackSubmit_FilenameSanitized(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "Path traversal attempt",
		"attachments": []map[string]interface{}{
			{
				"filename":     "../../../etc/passwd",
				"content_type": "text/plain",
				"data":         base64.StdEncoding.EncodeToString([]byte("test")),
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)

	require.Equal(t, http.StatusOK, getRec.Code)
	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
	atts := fb["attachments"].([]interface{})
	att := atts[0].(map[string]interface{})
	assert.Equal(t, "passwd", att["filename"], "path traversal components should be stripped")
}

func TestHandleFeedbackSubmit_NoAttachments_Unchanged(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "No attachments",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
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
	_, hasAtts := fb["attachments"]
	assert.False(t, hasAtts, "attachments should be omitted when empty")
}

func TestHandleFeedbackUpdate_WithAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit feedback without attachments
	body := jsonBody(t, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "Something wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var createResp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&createResp))
	fbID := createResp["feedback_id"].(string)

	// Update with attachments
	csvData := base64.StdEncoding.EncodeToString([]byte("a,b,c\n1,2,3"))
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
		"attachments": []map[string]interface{}{
			{
				"filename":     "data.csv",
				"content_type": "text/csv",
				"data":         csvData,
			},
		},
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	require.Equal(t, http.StatusOK, updateRec.Code)

	var updated map[string]interface{}
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))
	assert.Equal(t, "acknowledged", updated["status"])
	atts, ok := updated["attachments"].([]interface{})
	require.True(t, ok)
	require.Len(t, atts, 1)
	att := atts[0].(map[string]interface{})
	assert.Equal(t, "data.csv", att["filename"])
}
