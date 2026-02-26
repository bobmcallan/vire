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

// createAdminUser bootstraps an admin user via the dev OAuth provider and returns
// the user ID for use in X-Vire-User-ID header. The dev provider creates "dev_user"
// with role=admin. POST /api/users ignores the role field, so dev OAuth is required.
func createAdminUser(t *testing.T, env *common.Env) string {
	t.Helper()
	resp, err := env.HTTPPost("/api/auth/oauth", map[string]interface{}{
		"provider": "dev",
	})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	return "dev_user"
}

// adminHeaders returns headers with the admin user ID.
func adminHeaders(userID string) map[string]string {
	return map[string]string{"X-Vire-User-ID": userID}
}

// submitFeedback posts a feedback entry and returns the feedback_id.
func submitFeedback(t *testing.T, env *common.Env, body map[string]interface{}) string {
	t.Helper()
	resp, err := env.HTTPPost("/api/feedback", body)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, 202, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	require.Equal(t, true, result["accepted"])
	id, ok := result["feedback_id"].(string)
	require.True(t, ok, "feedback_id should be a string")
	return id
}

// --- Submit Feedback ---

func TestFeedbackSubmit(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	tests := []struct {
		name       string
		body       map[string]interface{}
		wantStatus int
		wantAccept bool
	}{
		{
			name: "all_fields",
			body: map[string]interface{}{
				"session_id":     "sess_001",
				"client_type":    "claude-desktop",
				"category":       "data_anomaly",
				"severity":       "high",
				"description":    "Price divergence detected for BHP.AX",
				"ticker":         "BHP.AX",
				"portfolio_name": "smsf",
				"tool_name":      "get_portfolio",
				"observed_value": map[string]interface{}{"price": 42.50},
				"expected_value": map[string]interface{}{"price": 45.00},
			},
			wantStatus: 202,
			wantAccept: true,
		},
		{
			name: "required_fields_only",
			body: map[string]interface{}{
				"category":    "observation",
				"description": "General observation about portfolio sync",
			},
			wantStatus: 202,
			wantAccept: true,
		},
		{
			name: "invalid_category",
			body: map[string]interface{}{
				"category":    "not_a_category",
				"description": "test",
			},
			wantStatus: 400,
		},
		{
			name: "empty_category",
			body: map[string]interface{}{
				"category":    "",
				"description": "test",
			},
			wantStatus: 400,
		},
		{
			name: "empty_description",
			body: map[string]interface{}{
				"category":    "observation",
				"description": "",
			},
			wantStatus: 400,
		},
		{
			name: "whitespace_description",
			body: map[string]interface{}{
				"category":    "observation",
				"description": "   ",
			},
			wantStatus: 400,
		},
		{
			name: "severity_default_to_medium",
			body: map[string]interface{}{
				"category":    "sync_delay",
				"description": "Sync took longer than expected",
			},
			wantStatus: 202,
			wantAccept: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := env.HTTPPost("/api/feedback", tt.body)
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			guard.SaveResult("submit_"+tt.name, string(body))

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			if tt.wantAccept {
				var result map[string]interface{}
				require.NoError(t, json.Unmarshal(body, &result))
				assert.Equal(t, true, result["accepted"])
				assert.Contains(t, result, "feedback_id")
			}
		})
	}
}

func TestFeedbackSubmit_SeverityDefault(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	// Submit without severity
	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Testing default severity",
	})

	// Get it back and verify severity is "medium"
	resp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	assert.Equal(t, "medium", result["severity"])
	assert.Equal(t, "new", result["status"])
}

func TestFeedbackSubmit_ObservedExpectedValues(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	id := submitFeedback(t, env, map[string]interface{}{
		"category":       "calculation_error",
		"description":    "Net return mismatch",
		"observed_value": map[string]interface{}{"net_return": -150.25},
		"expected_value": map[string]interface{}{"net_return": 200.00},
	})

	resp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, 200, resp.StatusCode)
	result := decodeResponse(t, resp.Body)
	assert.NotNil(t, result["observed_value"])
	assert.NotNil(t, result["expected_value"])
}

// --- List Feedback ---

func TestFeedbackList(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Submit several feedback entries with different attributes
	submitFeedback(t, env, map[string]interface{}{
		"category":    "data_anomaly",
		"severity":    "high",
		"description": "High severity anomaly",
		"ticker":      "BHP.AX",
		"session_id":  "sess_list_1",
	})
	submitFeedback(t, env, map[string]interface{}{
		"category":    "sync_delay",
		"severity":    "low",
		"description": "Low severity delay",
		"ticker":      "CBA.AX",
		"session_id":  "sess_list_2",
	})
	submitFeedback(t, env, map[string]interface{}{
		"category":    "data_anomaly",
		"severity":    "medium",
		"description": "Medium anomaly",
		"ticker":      "BHP.AX",
		"session_id":  "sess_list_1",
	})

	t.Run("list_all", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("list_all", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		assert.Equal(t, float64(3), result["total"])
		assert.Equal(t, float64(1), result["page"])
		assert.Equal(t, float64(20), result["per_page"])
		assert.Equal(t, float64(1), result["pages"])

		items := result["items"].([]interface{})
		assert.Len(t, items, 3)
	})

	t.Run("filter_by_category", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback?category=data_anomaly")
		require.NoError(t, err)
		defer resp.Body.Close()

		result := decodeResponse(t, resp.Body)
		assert.Equal(t, float64(2), result["total"])
	})

	t.Run("filter_by_severity", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback?severity=high")
		require.NoError(t, err)
		defer resp.Body.Close()

		result := decodeResponse(t, resp.Body)
		assert.Equal(t, float64(1), result["total"])
	})

	t.Run("filter_by_ticker", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback?ticker=BHP.AX")
		require.NoError(t, err)
		defer resp.Body.Close()

		result := decodeResponse(t, resp.Body)
		assert.Equal(t, float64(2), result["total"])
	})

	t.Run("filter_by_session_id", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback?session_id=sess_list_2")
		require.NoError(t, err)
		defer resp.Body.Close()

		result := decodeResponse(t, resp.Body)
		assert.Equal(t, float64(1), result["total"])
	})

	t.Run("pagination", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback?per_page=2&page=1")
		require.NoError(t, err)
		defer resp.Body.Close()

		result := decodeResponse(t, resp.Body)
		assert.Equal(t, float64(3), result["total"])
		assert.Equal(t, float64(2), result["per_page"])
		assert.Equal(t, float64(2), result["pages"])
		items := result["items"].([]interface{})
		assert.Len(t, items, 2)

		// Page 2
		resp2, err := env.HTTPGet("/api/feedback?per_page=2&page=2")
		require.NoError(t, err)
		defer resp2.Body.Close()

		result2 := decodeResponse(t, resp2.Body)
		items2 := result2["items"].([]interface{})
		assert.Len(t, items2, 1)
	})

	t.Run("sort_created_at_asc", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback?sort=created_at_asc")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
		result := decodeResponse(t, resp.Body)
		items := result["items"].([]interface{})
		assert.Len(t, items, 3)
		// First item should be the earliest created
		first := items[0].(map[string]interface{})
		assert.Equal(t, "High severity anomaly", first["description"])
	})
}

// --- Get Feedback ---

func TestFeedbackGet(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "tool_error",
		"severity":    "high",
		"description": "get_portfolio returned empty holdings",
		"ticker":      "ASX.AX",
		"tool_name":   "get_portfolio",
	})

	t.Run("existing", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback/" + id)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("get_existing", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		assert.Equal(t, id, result["id"])
		assert.Equal(t, "tool_error", result["category"])
		assert.Equal(t, "high", result["severity"])
		assert.Equal(t, "get_portfolio returned empty holdings", result["description"])
		assert.Equal(t, "ASX.AX", result["ticker"])
		assert.Equal(t, "get_portfolio", result["tool_name"])
		assert.Equal(t, "new", result["status"])
		assert.NotEmpty(t, result["created_at"])
		assert.NotEmpty(t, result["updated_at"])
	})

	t.Run("not_found", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback/fb_nonexistent")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 404, resp.StatusCode)
	})
}

// --- Update Feedback (admin) ---

func TestFeedbackUpdate(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminUser := createAdminUser(t, env)
	headers := adminHeaders(adminUser)

	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "missing_data",
		"description": "Missing dividend data for BHP",
	})

	t.Run("update_status", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{"status": "acknowledged"}, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("update_status", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Equal(t, "acknowledged", result["status"])
	})

	t.Run("update_with_resolution_notes", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{
				"status":           "resolved",
				"resolution_notes": "Dividend data added via EODHD sync",
			}, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("update_resolved", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Equal(t, "resolved", result["status"])
		assert.Equal(t, "Dividend data added via EODHD sync", result["resolution_notes"])
	})

	t.Run("update_invalid_status", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{"status": "invalid_status"}, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("update_not_found", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/fb_nonexistent",
			map[string]interface{}{"status": "acknowledged"}, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 404, resp.StatusCode)
	})

	t.Run("update_no_auth", func(t *testing.T) {
		// handleFeedbackUpdate does NOT require authentication —
		// MCP clients that submit feedback should be able to update status.
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{"status": "dismissed"}, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("update_non_admin", func(t *testing.T) {
		// Create a non-admin user
		resp, err := env.HTTPPost("/api/users", map[string]interface{}{
			"username": "regularuser",
			"email":    "regular@test.com",
			"password": "password123",
		})
		require.NoError(t, err)
		resp.Body.Close()
		require.Equal(t, 201, resp.StatusCode)

		// handleFeedbackUpdate does NOT require admin — any user can update.
		resp, err = env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
			map[string]interface{}{"status": "new"},
			map[string]string{"X-Vire-User-ID": "regularuser"})
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})
}

// --- Bulk Update ---

func TestFeedbackBulkUpdate(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminUser := createAdminUser(t, env)
	headers := adminHeaders(adminUser)

	id1 := submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Observation 1",
	})
	id2 := submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Observation 2",
	})

	t.Run("bulk_update_multiple", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/bulk",
			map[string]interface{}{
				"ids":              []string{id1, id2},
				"status":           "dismissed",
				"resolution_notes": "Bulk dismissed",
			}, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("bulk_update", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Equal(t, float64(2), result["updated"])

		// Verify both are dismissed
		for _, id := range []string{id1, id2} {
			getResp, err := env.HTTPGet("/api/feedback/" + id)
			require.NoError(t, err)
			defer getResp.Body.Close()
			r := decodeResponse(t, getResp.Body)
			assert.Equal(t, "dismissed", r["status"])
		}
	})

	t.Run("bulk_update_empty_ids", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/bulk",
			map[string]interface{}{
				"ids":    []string{},
				"status": "dismissed",
			}, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 400, resp.StatusCode)
	})

	t.Run("bulk_update_no_auth", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/bulk",
			map[string]interface{}{
				"ids":    []string{id1},
				"status": "resolved",
			}, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 401, resp.StatusCode)
	})
}

// --- Delete Feedback (admin) ---

func TestFeedbackDelete(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	adminUser := createAdminUser(t, env)
	headers := adminHeaders(adminUser)

	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "schema_change",
		"description": "Schema change detected",
	})

	t.Run("delete_existing", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodDelete, "/api/feedback/"+id, nil, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 204, resp.StatusCode)

		// Verify deleted
		getResp, err := env.HTTPGet("/api/feedback/" + id)
		require.NoError(t, err)
		defer getResp.Body.Close()
		assert.Equal(t, 404, getResp.StatusCode)
	})

	t.Run("delete_nonexistent", func(t *testing.T) {
		resp, err := env.HTTPRequest(http.MethodDelete, "/api/feedback/fb_gone", nil, headers)
		require.NoError(t, err)
		defer resp.Body.Close()

		// Delete of non-existent should succeed silently (204)
		assert.Equal(t, 204, resp.StatusCode)
	})

	t.Run("delete_no_auth", func(t *testing.T) {
		id2 := submitFeedback(t, env, map[string]interface{}{
			"category":    "observation",
			"description": "Should not be deletable without auth",
		})
		resp, err := env.HTTPRequest(http.MethodDelete, "/api/feedback/"+id2, nil, nil)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 401, resp.StatusCode)
	})
}

// --- Summary ---

func TestFeedbackSummary(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	t.Run("empty_summary", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback/summary")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("summary_empty", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		assert.Equal(t, float64(0), result["total"])
	})

	// Add mixed feedback
	submitFeedback(t, env, map[string]interface{}{
		"category": "data_anomaly", "severity": "high", "description": "anomaly 1",
	})
	submitFeedback(t, env, map[string]interface{}{
		"category": "data_anomaly", "severity": "low", "description": "anomaly 2",
	})
	submitFeedback(t, env, map[string]interface{}{
		"category": "sync_delay", "severity": "medium", "description": "delay 1",
	})

	t.Run("summary_with_data", func(t *testing.T) {
		resp, err := env.HTTPGet("/api/feedback/summary")
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		guard.SaveResult("summary_with_data", string(body))

		assert.Equal(t, 200, resp.StatusCode)
		var result map[string]interface{}
		require.NoError(t, json.Unmarshal(body, &result))

		assert.Equal(t, float64(3), result["total"])

		byStatus := result["by_status"].(map[string]interface{})
		assert.Equal(t, float64(3), byStatus["new"])

		bySeverity := result["by_severity"].(map[string]interface{})
		assert.Equal(t, float64(1), bySeverity["high"])
		assert.Equal(t, float64(1), bySeverity["low"])
		assert.Equal(t, float64(1), bySeverity["medium"])

		byCategory := result["by_category"].(map[string]interface{})
		assert.Equal(t, float64(2), byCategory["data_anomaly"])
		assert.Equal(t, float64(1), byCategory["sync_delay"])

		// All items are "new" so oldest_unresolved should be set
		assert.NotNil(t, result["oldest_unresolved"])
	})
}

// --- Full Lifecycle ---

func TestFeedbackLifecycle(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()
	adminUser := createAdminUser(t, env)
	headers := adminHeaders(adminUser)

	// Step 1: Submit
	id := submitFeedback(t, env, map[string]interface{}{
		"session_id":  "sess_lifecycle",
		"client_type": "claude-cli",
		"category":    "calculation_error",
		"severity":    "high",
		"description": "Breakeven price seems incorrect for BHP",
		"ticker":      "BHP.AX",
		"tool_name":   "get_portfolio",
	})
	t.Logf("Created feedback: %s", id)

	// Step 2: List — should appear
	resp, err := env.HTTPGet("/api/feedback")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	guard.SaveResult("lifecycle_01_list", string(body))

	var listResult map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &listResult))
	assert.Equal(t, float64(1), listResult["total"])

	// Step 3: Get — verify all fields
	resp, err = env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	guard.SaveResult("lifecycle_02_get", string(body))

	var fb map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &fb))
	assert.Equal(t, id, fb["id"])
	assert.Equal(t, "calculation_error", fb["category"])
	assert.Equal(t, "high", fb["severity"])
	assert.Equal(t, "new", fb["status"])
	assert.Equal(t, "BHP.AX", fb["ticker"])

	// Step 4: Update status to acknowledged
	resp, err = env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
		map[string]interface{}{"status": "acknowledged"}, headers)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	guard.SaveResult("lifecycle_03_acknowledge", string(body))

	var ackResult map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &ackResult))
	assert.Equal(t, "acknowledged", ackResult["status"])

	// Step 5: Get — verify updated
	resp, err = env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	var verifyResult map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &verifyResult))
	assert.Equal(t, "acknowledged", verifyResult["status"])

	// Step 6: Delete
	resp, err = env.HTTPRequest(http.MethodDelete, "/api/feedback/"+id, nil, headers)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 204, resp.StatusCode)

	// Step 7: Get — verify 404
	resp, err = env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 404, resp.StatusCode)

	t.Logf("Results saved to: %s", guard.ResultsDir())
}

// --- All Categories ---

func TestFeedbackSubmit_AllCategories(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	categories := []string{
		"data_anomaly", "sync_delay", "calculation_error",
		"missing_data", "schema_change", "tool_error", "observation",
	}

	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			resp, err := env.HTTPPost("/api/feedback", map[string]interface{}{
				"category":    cat,
				"description": "Testing category: " + cat,
			})
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, 202, resp.StatusCode)
		})
	}
}
