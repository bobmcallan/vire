package api

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bobmcallan/vire/tests/common"
)

func TestFeedbackSubmit_WithAttachments(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	pngData := base64.StdEncoding.EncodeToString([]byte("fake-png-content"))
	jsonData := base64.StdEncoding.EncodeToString([]byte(`{"debug":"info"}`))

	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "data_anomaly",
		"severity":    "high",
		"description": "Anomaly with screenshots",
		"ticker":      "BHP.AX",
		"attachments": []map[string]interface{}{
			{
				"filename":     "screenshot.png",
				"content_type": "image/png",
				"data":         pngData,
			},
			{
				"filename":     "debug.json",
				"content_type": "application/json",
				"data":         jsonData,
			},
		},
	})

	// Retrieve and verify attachments round-trip
	resp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("submit_with_attachments", string(body))

	require.Equal(t, 200, resp.StatusCode)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	atts, ok := result["attachments"].([]interface{})
	require.True(t, ok, "attachments should be an array")
	require.Len(t, atts, 2)

	att0 := atts[0].(map[string]interface{})
	assert.Equal(t, "screenshot.png", att0["filename"])
	assert.Equal(t, "image/png", att0["content_type"])
	assert.Equal(t, pngData, att0["data"])
	assert.Equal(t, float64(len("fake-png-content")), att0["size_bytes"])

	att1 := atts[1].(map[string]interface{})
	assert.Equal(t, "debug.json", att1["filename"])
	assert.Equal(t, "application/json", att1["content_type"])
}

func TestFeedbackSubmit_TooManyAttachments(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Build 11 attachments (max is 10)
	attachments := make([]map[string]interface{}, 11)
	for i := range attachments {
		attachments[i] = map[string]interface{}{
			"filename":     "file.png",
			"content_type": "image/png",
			"data":         base64.StdEncoding.EncodeToString([]byte("x")),
		}
	}

	resp, err := env.HTTPPost("/api/feedback", map[string]interface{}{
		"category":    "observation",
		"description": "Too many attachments",
		"attachments": attachments,
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("too_many_attachments", string(body))

	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, string(body), "too many attachments")
}

func TestFeedbackSubmit_InvalidContentType(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/feedback", map[string]interface{}{
		"category":    "observation",
		"description": "Bad content type",
		"attachments": []map[string]interface{}{
			{
				"filename":     "video.mp4",
				"content_type": "video/mp4",
				"data":         base64.StdEncoding.EncodeToString([]byte("video")),
			},
		},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("invalid_content_type", string(body))

	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, string(body), "unsupported content_type")
}

func TestFeedbackSubmit_InvalidBase64(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	resp, err := env.HTTPPost("/api/feedback", map[string]interface{}{
		"category":    "observation",
		"description": "Bad base64",
		"attachments": []map[string]interface{}{
			{
				"filename":     "broken.png",
				"content_type": "image/png",
				"data":         "not-valid-base64!!!@@@",
			},
		},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("invalid_base64", string(body))

	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, string(body), "invalid base64")
}

func TestFeedbackSubmit_OversizedAttachment(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// 5MB + 1 byte exceeds the MaxAttachmentSize limit
	oversized := strings.Repeat("A", 5*1024*1024+1)
	oversizedB64 := base64.StdEncoding.EncodeToString([]byte(oversized))

	resp, err := env.HTTPPost("/api/feedback", map[string]interface{}{
		"category":    "observation",
		"description": "Oversized attachment",
		"attachments": []map[string]interface{}{
			{
				"filename":     "huge.png",
				"content_type": "image/png",
				"data":         oversizedB64,
			},
		},
	})
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("oversized_attachment", string(body))

	assert.Equal(t, 400, resp.StatusCode)
	assert.Contains(t, string(body), "exceeds max size")
}

func TestFeedbackUpdate_WithAttachments(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Submit feedback without attachments
	id := submitFeedback(t, env, map[string]interface{}{
		"category":    "missing_data",
		"description": "Will add attachment via update",
	})

	// Update to add attachments
	csvData := base64.StdEncoding.EncodeToString([]byte("a,b,c\n1,2,3\n"))
	resp, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
		map[string]interface{}{
			"status": "acknowledged",
			"attachments": []map[string]interface{}{
				{
					"filename":     "export.csv",
					"content_type": "text/csv",
					"data":         csvData,
				},
			},
		}, nil)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("update_add_attachments", string(body))

	assert.Equal(t, 200, resp.StatusCode)

	// Verify via GET
	getResp, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer getResp.Body.Close()

	result := decodeResponse(t, getResp.Body)
	atts, ok := result["attachments"].([]interface{})
	require.True(t, ok)
	require.Len(t, atts, 1)

	att := atts[0].(map[string]interface{})
	assert.Equal(t, "export.csv", att["filename"])
	assert.Equal(t, "text/csv", att["content_type"])
	assert.Equal(t, csvData, att["data"])

	// Update again without attachments field -- should preserve existing
	resp2, err := env.HTTPRequest(http.MethodPatch, "/api/feedback/"+id,
		map[string]interface{}{
			"status": "resolved",
		}, nil)
	require.NoError(t, err)
	defer resp2.Body.Close()

	body2, _ := io.ReadAll(resp2.Body)
	guard.SaveResult("update_preserve_attachments", string(body2))

	assert.Equal(t, 200, resp2.StatusCode)

	getResp2, err := env.HTTPGet("/api/feedback/" + id)
	require.NoError(t, err)
	defer getResp2.Body.Close()

	result2 := decodeResponse(t, getResp2.Body)
	atts2, ok := result2["attachments"].([]interface{})
	require.True(t, ok, "attachments should be preserved after status-only update")
	assert.Len(t, atts2, 1)
}

func TestFeedbackList_IncludesAttachmentMetadata(t *testing.T) {
	env := common.NewEnv(t)
	if env == nil {
		return
	}
	defer env.Cleanup()

	guard := env.OutputGuard()

	// Submit with attachment
	submitFeedback(t, env, map[string]interface{}{
		"category":    "data_anomaly",
		"description": "With attachment",
		"attachments": []map[string]interface{}{
			{
				"filename":     "evidence.png",
				"content_type": "image/png",
				"data":         base64.StdEncoding.EncodeToString([]byte("evidence-data")),
			},
		},
	})
	// Submit without attachment
	submitFeedback(t, env, map[string]interface{}{
		"category":    "observation",
		"description": "Without attachment",
	})

	resp, err := env.HTTPGet("/api/feedback")
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	guard.SaveResult("list_with_attachments", string(body))

	assert.Equal(t, 200, resp.StatusCode)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Equal(t, float64(2), result["total"])
	items := result["items"].([]interface{})
	require.Len(t, items, 2)

	// Find the item with attachments
	var withAtt, withoutAtt map[string]interface{}
	for _, item := range items {
		m := item.(map[string]interface{})
		if m["description"] == "With attachment" {
			withAtt = m
		} else {
			withoutAtt = m
		}
	}

	require.NotNil(t, withAtt, "should find feedback with attachment")
	require.NotNil(t, withoutAtt, "should find feedback without attachment")

	atts, ok := withAtt["attachments"].([]interface{})
	require.True(t, ok, "attachments should be present in list response")
	assert.Len(t, atts, 1)

	att := atts[0].(map[string]interface{})
	assert.Equal(t, "evidence.png", att["filename"])
	assert.Equal(t, "image/png", att["content_type"])

	// Item without attachments should have nil/empty attachments
	noAtts := withoutAtt["attachments"]
	if noAtts != nil {
		// If present, should be empty array
		arr, ok := noAtts.([]interface{})
		if ok {
			assert.Empty(t, arr)
		}
	}
}
