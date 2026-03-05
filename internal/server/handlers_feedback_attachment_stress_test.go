package server

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bobmcallan/vire/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// 1. Base64 bomb — extremely large base64 strings and memory pressure
// ============================================================================

func TestFeedbackAttachmentStress_OversizedBase64(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// 6MB of raw data -> ~8MB base64. Exceeds the 5MB limit.
	rawData := strings.Repeat("A", 6*1024*1024)
	encoded := base64.StdEncoding.EncodeToString([]byte(rawData))

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "oversized attachment",
		"attachments": []map[string]interface{}{
			{
				"filename":     "big.png",
				"content_type": "image/png",
				"data":         encoded,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"attachment exceeding MaxAttachmentSize should be rejected")
	assert.Contains(t, rec.Body.String(), "exceeds max size",
		"error should mention size limit")
}

func TestFeedbackAttachmentStress_ExactlyAtSizeLimit(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Exactly 5MB of raw data
	rawData := make([]byte, models.MaxAttachmentSize)
	for i := range rawData {
		rawData[i] = 0x42
	}
	encoded := base64.StdEncoding.EncodeToString(rawData)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "exact limit",
		"attachments": []map[string]interface{}{
			{
				"filename":     "exact.png",
				"content_type": "image/png",
				"data":         encoded,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code,
		"attachment exactly at MaxAttachmentSize should be accepted")
}

func TestFeedbackAttachmentStress_OneByteBeyondLimit(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// 5MB + 1 byte
	rawData := make([]byte, models.MaxAttachmentSize+1)
	for i := range rawData {
		rawData[i] = 0x43
	}
	encoded := base64.StdEncoding.EncodeToString(rawData)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "one byte over",
		"attachments": []map[string]interface{}{
			{
				"filename":     "over.png",
				"content_type": "image/png",
				"data":         encoded,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"attachment 1 byte over MaxAttachmentSize should be rejected")
}

// ============================================================================
// 2. Malformed base64 — decoding edge cases
// ============================================================================

func TestFeedbackAttachmentStress_MalformedBase64(t *testing.T) {
	srv := newTestServerWithStorage(t)

	malformedPayloads := []struct {
		name string
		data string
	}{
		{"not_base64", "this is not base64!@#$%"},
		{"truncated_padding", "SGVsbG8=="}, // extra padding
		{"null_bytes", "SGVs\x00bG8="},
		{"unicode_in_base64", "SGVsbG8g8J+Ygg=="},
		{"only_padding", "===="},
		{"empty_string", ""},
		{"spaces_only", "    "},
		{"newlines", "SGVs\nbG8="},
		{"base64url_variant", "SGVsbG8-_w"}, // URL-safe variant, not standard
	}

	for _, tc := range malformedPayloads {
		t.Run(tc.name, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "malformed base64 test",
				"attachments": []map[string]interface{}{
					{
						"filename":     "test.png",
						"content_type": "image/png",
						"data":         tc.data,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			// Empty data and malformed base64 should both be rejected
			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"malformed base64 %q should be rejected", tc.name)

			// Should not crash or return 500
			assert.NotEqual(t, http.StatusInternalServerError, rec.Code,
				"malformed base64 should never cause a server error")
		})
	}
}

// ============================================================================
// 3. Content-type abuse — type confusion and smuggling
// ============================================================================

func TestFeedbackAttachmentStress_DisallowedContentTypes(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test data"))

	disallowedTypes := []string{
		"application/pdf",
		"video/mp4",
		"audio/mpeg",
		"application/x-executable",
		"application/octet-stream",
		"text/html",
		"application/javascript",
		"image/svg+xml", // SVG can contain XSS
		"application/xml",
		"multipart/form-data",
		"", // empty
	}

	for _, ct := range disallowedTypes {
		name := ct
		if name == "" {
			name = "(empty)"
		}
		t.Run(name, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "disallowed type test",
				"attachments": []map[string]interface{}{
					{
						"filename":     "test.bin",
						"content_type": ct,
						"data":         validBase64,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"content type %q should be rejected", ct)
		})
	}
}

func TestFeedbackAttachmentStress_ContentTypeCaseSensitivity(t *testing.T) {
	// FINDING: ValidAttachmentTypes uses exact string match.
	// Uppercase or mixed-case variants should be rejected.
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test data"))

	casedTypes := []string{
		"Image/PNG",
		"IMAGE/PNG",
		"image/PNG",
		"Image/Png",
		"IMAGE/JPEG",
		"Text/Plain",
	}

	for _, ct := range casedTypes {
		t.Run(ct, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "case sensitivity test",
				"attachments": []map[string]interface{}{
					{
						"filename":     "test.png",
						"content_type": ct,
						"data":         validBase64,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"case-varied content type %q should be rejected (strict match)", ct)
		})
	}

	t.Log("NOTE: Content types are case-sensitive. 'Image/PNG' is rejected while 'image/png' is accepted. " +
		"RFC 2045 says media types are case-insensitive. Consider normalizing with strings.ToLower().")
}

func TestFeedbackAttachmentStress_ContentTypeWithParams(t *testing.T) {
	// FINDING: Content types with parameters (charset, boundary) are not in ValidAttachmentTypes.
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test data"))

	paramTypes := []string{
		"text/plain; charset=utf-8",
		"application/json; charset=utf-8",
		"image/png; name=photo.png",
	}

	for _, ct := range paramTypes {
		t.Run(ct, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "content type with params",
				"attachments": []map[string]interface{}{
					{
						"filename":     "test.txt",
						"content_type": ct,
						"data":         validBase64,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code,
				"content type with params %q should be rejected (strict match)", ct)
		})
	}

	t.Log("NOTE: Content types with parameters (e.g. 'text/plain; charset=utf-8') are rejected. " +
		"This is by design but may surprise MCP clients that include charset params.")
}

// ============================================================================
// 4. Attachment count limits
// ============================================================================

func TestFeedbackAttachmentStress_TooManyAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("tiny"))
	attachments := make([]map[string]interface{}, models.MaxAttachments+1)
	for i := range attachments {
		attachments[i] = map[string]interface{}{
			"filename":     "file.png",
			"content_type": "image/png",
			"data":         validBase64,
		}
	}

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "too many attachments",
		"attachments": attachments,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"more than MaxAttachments should be rejected")
	assert.Contains(t, rec.Body.String(), "too many attachments")
}

func TestFeedbackAttachmentStress_ExactlyMaxAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("tiny"))
	attachments := make([]map[string]interface{}, models.MaxAttachments)
	for i := range attachments {
		attachments[i] = map[string]interface{}{
			"filename":     "file.png",
			"content_type": "image/png",
			"data":         validBase64,
		}
	}

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "max attachments",
		"attachments": attachments,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code,
		"exactly MaxAttachments should be accepted")
}

// ============================================================================
// 5. Missing or empty required fields in attachments
// ============================================================================

func TestFeedbackAttachmentStress_MissingFilename(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test"))

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "missing filename",
		"attachments": []map[string]interface{}{
			{
				"content_type": "image/png",
				"data":         validBase64,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "filename")
}

func TestFeedbackAttachmentStress_MissingContentType(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test"))

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "missing content type",
		"attachments": []map[string]interface{}{
			{
				"filename": "test.png",
				"data":     validBase64,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "content_type")
}

func TestFeedbackAttachmentStress_MissingData(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "missing data",
		"attachments": []map[string]interface{}{
			{
				"filename":     "test.png",
				"content_type": "image/png",
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "data")
}

// ============================================================================
// 6. Empty and nil attachment arrays
// ============================================================================

func TestFeedbackAttachmentStress_EmptyAttachmentsArray(t *testing.T) {
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "empty attachments",
		"attachments": []map[string]interface{}{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code,
		"empty attachments array should be accepted (no attachments)")
}

func TestFeedbackAttachmentStress_NilAttachments(t *testing.T) {
	// Omit the attachments field entirely — existing behavior preserved
	srv := newTestServerWithStorage(t)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "no attachments field",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code,
		"missing attachments field should be accepted")
}

func TestFeedbackAttachmentStress_NullAttachmentsJSON(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Explicitly set attachments to null in JSON
	body := strings.NewReader(`{"category":"observation","description":"null attachments","attachments":null}`)
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code,
		"null attachments should be accepted (treated as no attachments)")
}

// ============================================================================
// 7. Filename injection — path traversal, special chars
// ============================================================================

func TestFeedbackAttachmentStress_FilenameInjection(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test"))

	// Filenames are sanitized via filepath.Base() to strip path components.
	hostileFilenames := []struct {
		name     string
		filename string
		expected string // expected sanitized filename
	}{
		{"path_traversal", "../../../etc/passwd", "passwd"},
		{"absolute_path", "/etc/shadow", "shadow"},
		{"null_byte", "test\x00.png", "test\x00.png"}, // null byte passes through Base()
		{"very_long", strings.Repeat("A", 1000) + ".png", strings.Repeat("A", 1000) + ".png"},
		{"unicode", "\u202E\u0066\u0069\u006C\u0065.png", "\u202E\u0066\u0069\u006C\u0065.png"},
		{"dot_dot", "..", ".."},  // filepath.Base("..") = ".." — handler falls back to original
		{"single_dot", ".", "."}, // filepath.Base(".") = "." — handler falls back to original
		{"backslash", `..\..\etc\passwd`, "passwd"},
		{"html_in_filename", "<script>alert(1)</script>.png", "<script>alert(1)</script>.png"},
	}

	for _, tc := range hostileFilenames {
		t.Run(tc.name, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "filename injection test",
				"attachments": []map[string]interface{}{
					{
						"filename":     tc.filename,
						"content_type": "image/png",
						"data":         validBase64,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			// Should not crash
			if rec.Code >= 500 {
				t.Errorf("server error with hostile filename %q: status %d", tc.filename, rec.Code)
			}

			// If accepted, verify filename was sanitized
			if rec.Code == http.StatusAccepted {
				var resp map[string]interface{}
				require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
				fbID := resp["feedback_id"].(string)

				getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
				getRec := httptest.NewRecorder()
				srv.handleFeedbackGet(getRec, getReq, fbID)
				if getRec.Code == http.StatusOK {
					var fb map[string]interface{}
					require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
					if atts, ok := fb["attachments"].([]interface{}); ok && len(atts) > 0 {
						att := atts[0].(map[string]interface{})
						assert.Equal(t, tc.expected, att["filename"],
							"filename %q should be sanitized to %q", tc.filename, tc.expected)
					}
				}
			}
		})
	}
}

// ============================================================================
// 8. SizeBytes field integrity — computed vs claimed
// ============================================================================

func TestFeedbackAttachmentStress_SizeBytesComputed(t *testing.T) {
	// Verify that SizeBytes is computed server-side from the actual decoded data,
	// not taken from any client-provided value.
	srv := newTestServerWithStorage(t)

	rawData := []byte("hello world")
	encoded := base64.StdEncoding.EncodeToString(rawData)

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "size bytes check",
		"attachments": []map[string]interface{}{
			{
				"filename":     "test.txt",
				"content_type": "text/plain",
				"data":         encoded,
				"size_bytes":   999999, // client tries to set this
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())

	// Fetch and verify size_bytes is computed, not client-supplied
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
	getRec := httptest.NewRecorder()
	srv.handleFeedbackGet(getRec, getReq, fbID)
	require.Equal(t, http.StatusOK, getRec.Code)

	var fb map[string]interface{}
	require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))

	attachments, ok := fb["attachments"].([]interface{})
	require.True(t, ok, "attachments should be an array")
	require.Len(t, attachments, 1)

	att := attachments[0].(map[string]interface{})
	sizeBytes := int(att["size_bytes"].(float64))
	assert.Equal(t, len(rawData), sizeBytes,
		"size_bytes should be computed from actual decoded data length, not client-provided value")
}

// ============================================================================
// 9. Concurrent submissions with large attachments — race conditions
// ============================================================================

func TestFeedbackAttachmentStress_ConcurrentLargeAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// 1MB attachment, 20 concurrent submissions
	rawData := make([]byte, 1024*1024)
	for i := range rawData {
		rawData[i] = byte(i % 256)
	}
	encoded := base64.StdEncoding.EncodeToString(rawData)

	var wg sync.WaitGroup
	errors := make(chan string, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "concurrent attachment test",
				"attachments": []map[string]interface{}{
					{
						"filename":     "data.png",
						"content_type": "image/png",
						"data":         encoded,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			if rec.Code != http.StatusAccepted {
				errors <- rec.Body.String()
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent attachment submit error: %s", err)
	}
}

// ============================================================================
// 10. Update endpoint — attachment manipulation
// ============================================================================

func TestFeedbackAttachmentStress_Update_AddAttachmentsToExisting(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit without attachments
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "will add attachments",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Update to add attachments
	validBase64 := base64.StdEncoding.EncodeToString([]byte("new data"))
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
		"attachments": []map[string]interface{}{
			{
				"filename":     "added.png",
				"content_type": "image/png",
				"data":         validBase64,
			},
		},
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	assert.Equal(t, http.StatusOK, updateRec.Code,
		"update with attachments should succeed")

	// Verify attachments were added
	var updated map[string]interface{}
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))
	attachments, ok := updated["attachments"].([]interface{})
	if ok {
		assert.Len(t, attachments, 1, "should have 1 attachment after update")
	}
}

func TestFeedbackAttachmentStress_Update_ClearAttachments(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit with attachment
	validBase64 := base64.StdEncoding.EncodeToString([]byte("data"))
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "will clear attachments",
		"attachments": []map[string]interface{}{
			{
				"filename":     "file.png",
				"content_type": "image/png",
				"data":         validBase64,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Update with empty attachments array to clear
	updateBody := jsonBody(t, map[string]interface{}{
		"status":      "acknowledged",
		"attachments": []map[string]interface{}{},
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	assert.Equal(t, http.StatusOK, updateRec.Code,
		"update with empty attachments array should clear them")
}

func TestFeedbackAttachmentStress_Update_NilAttachmentsPreserves(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit with attachment
	validBase64 := base64.StdEncoding.EncodeToString([]byte("preserve me"))
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "preserve attachments",
		"attachments": []map[string]interface{}{
			{
				"filename":     "keep.png",
				"content_type": "image/png",
				"data":         validBase64,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Update status only — no attachments field
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	require.Equal(t, http.StatusOK, updateRec.Code)

	// Verify attachment is preserved
	var updated map[string]interface{}
	require.NoError(t, json.NewDecoder(updateRec.Body).Decode(&updated))
	attachments, ok := updated["attachments"].([]interface{})
	if ok {
		assert.Len(t, attachments, 1, "attachment should be preserved when not updating attachments")
	}
}

func TestFeedbackAttachmentStress_Update_InvalidAttachment(t *testing.T) {
	srv := newTestServerWithStorage(t)

	// Submit feedback first
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "will try invalid update",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)
	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	fbID := resp["feedback_id"].(string)

	// Try update with invalid attachment
	updateBody := jsonBody(t, map[string]interface{}{
		"status": "acknowledged",
		"attachments": []map[string]interface{}{
			{
				"filename":     "bad.exe",
				"content_type": "application/x-executable",
				"data":         base64.StdEncoding.EncodeToString([]byte("malware")),
			},
		},
	})
	updateReq := httptest.NewRequest(http.MethodPatch, "/api/feedback/"+fbID, updateBody)
	updateRec := httptest.NewRecorder()
	srv.handleFeedbackUpdate(updateRec, updateReq, fbID)

	assert.Equal(t, http.StatusBadRequest, updateRec.Code,
		"invalid content type in update should be rejected")
}

// ============================================================================
// 11. SQL injection via attachment fields
// ============================================================================

func TestFeedbackAttachmentStress_SQLInjectionInAttachmentFields(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test"))

	// SQL injection in filename (parameterized queries should prevent this)
	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "sql injection in attachment",
		"attachments": []map[string]interface{}{
			{
				"filename":     "'; DROP TABLE mcp_feedback; --",
				"content_type": "image/png",
				"data":         validBase64,
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	// Should be accepted (filename passes through) or rejected for other reasons
	// but NEVER cause a 500 error
	if rec.Code >= 500 {
		t.Errorf("SQL injection in filename caused server error: %s", rec.Body.String())
	}

	// If accepted, verify data integrity
	if rec.Code == http.StatusAccepted {
		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		fbID := resp["feedback_id"].(string)

		getReq := httptest.NewRequest(http.MethodGet, "/api/feedback/"+fbID, nil)
		getRec := httptest.NewRecorder()
		srv.handleFeedbackGet(getRec, getReq, fbID)
		require.Equal(t, http.StatusOK, getRec.Code,
			"feedback with SQL injection in filename should still be retrievable")

		var fb map[string]interface{}
		require.NoError(t, json.NewDecoder(getRec.Body).Decode(&fb))
		attachments := fb["attachments"].([]interface{})
		att := attachments[0].(map[string]interface{})
		assert.Equal(t, "'; DROP TABLE mcp_feedback; --", att["filename"],
			"SQL injection payload should be stored literally")
	}
}

// ============================================================================
// 12. Total payload size — aggregate attachment size
// ============================================================================

func TestFeedbackAttachmentStress_AggregateSizeMemoryPressure(t *testing.T) {
	// FINDING: Each attachment can be up to 5MB, with 10 attachments max.
	// That's 50MB of decoded data, plus ~67MB in base64 encoding.
	// The handler decodes each attachment to validate size, meaning
	// the server temporarily holds both the base64 string AND decoded bytes.
	// For 10x 5MB attachments: ~117MB of heap usage per request.
	//
	// With no request body size limit (noted in existing stress tests),
	// a malicious client could send 10x 5MB attachments repeatedly
	// to exhaust server memory.
	srv := newTestServerWithStorage(t)

	// Create 10 attachments at 4MB each (under per-file limit, large aggregate)
	rawData := make([]byte, 4*1024*1024)
	for i := range rawData {
		rawData[i] = byte(i % 256)
	}
	encoded := base64.StdEncoding.EncodeToString(rawData)

	attachments := make([]map[string]interface{}, models.MaxAttachments)
	for i := range attachments {
		attachments[i] = map[string]interface{}{
			"filename":     "big.png",
			"content_type": "image/png",
			"data":         encoded,
		}
	}

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "aggregate size test",
		"attachments": attachments,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
	rec := httptest.NewRecorder()
	srv.handleFeedbackRoot(rec, req)

	// Should be accepted (each file is under limit)
	assert.Equal(t, http.StatusAccepted, rec.Code,
		"10 attachments at 4MB each should be accepted (under per-file limit)")

	t.Log("FINDING: No aggregate attachment size limit. 10 attachments x 5MB = 50MB per request. " +
		"Combined with no request body size limit, this allows ~67MB base64 payloads. " +
		"The handler decodes each attachment, meaning ~117MB heap usage per request. " +
		"Recommendation: add an aggregate size limit or a request body size limit.")
}

// ============================================================================
// 13. Double decoding — base64 decoded twice?
// ============================================================================

func TestFeedbackAttachmentStress_DoubleDecoding(t *testing.T) {
	// Requirements show base64 is decoded TWICE in submit handler:
	// once for validation, once for SizeBytes computation.
	// Verify the stored data field is the original base64 (not double-encoded).
	srv := newTestServerWithStorage(t)

	originalData := "Hello, World!"
	encoded := base64.StdEncoding.EncodeToString([]byte(originalData))

	body := jsonBody(t, map[string]interface{}{
		"category":    "observation",
		"description": "double decode check",
		"attachments": []map[string]interface{}{
			{
				"filename":     "test.txt",
				"content_type": "text/plain",
				"data":         encoded,
			},
		},
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

	attachments := fb["attachments"].([]interface{})
	att := attachments[0].(map[string]interface{})

	// The stored data should be the ORIGINAL base64, decodable to the original text
	storedData := att["data"].(string)
	decoded, err := base64.StdEncoding.DecodeString(storedData)
	require.NoError(t, err, "stored data should be valid base64")
	assert.Equal(t, originalData, string(decoded),
		"stored base64 should decode to original data")
}

// ============================================================================
// 14. All valid content types — smoke test
// ============================================================================

func TestFeedbackAttachmentStress_AllValidContentTypes(t *testing.T) {
	srv := newTestServerWithStorage(t)

	validBase64 := base64.StdEncoding.EncodeToString([]byte("test content"))

	for ct := range models.ValidAttachmentTypes {
		t.Run(ct, func(t *testing.T) {
			body := jsonBody(t, map[string]interface{}{
				"category":    "observation",
				"description": "valid type test",
				"attachments": []map[string]interface{}{
					{
						"filename":     "test.dat",
						"content_type": ct,
						"data":         validBase64,
					},
				},
			})
			req := httptest.NewRequest(http.MethodPost, "/api/feedback", body)
			rec := httptest.NewRecorder()
			srv.handleFeedbackRoot(rec, req)

			assert.Equal(t, http.StatusAccepted, rec.Code,
				"valid content type %q should be accepted", ct)
		})
	}
}
