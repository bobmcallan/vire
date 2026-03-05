# Requirements: Feedback Media Attachments

## Scope

Add support for single and multiple media attachments on feedback items. Supported types:
- **Images**: PNG, JPEG, GIF, WebP
- **Data files**: JSON, CSV, plain text

Attachments are stored as base64 inline in the feedback record (no blob storage — keeps it simple, SurrealDB handles it fine for reasonable sizes).

### Out of scope
- Streaming/multipart upload (MCP clients send JSON)
- Video, audio, PDF
- Image resizing/thumbnails

---

## Design

### Attachment Model

Add to `internal/models/feedback.go`:

```go
// FeedbackAttachment represents a media file attached to a feedback item.
type FeedbackAttachment struct {
    Filename    string `json:"filename"`
    ContentType string `json:"content_type"`
    SizeBytes   int    `json:"size_bytes"`
    Data        string `json:"data"` // base64-encoded
}
```

Add to `Feedback` struct:
```go
Attachments []FeedbackAttachment `json:"attachments,omitempty"`
```

### Validation Constants

```go
// MaxAttachmentSize is the maximum size of a single attachment (5MB).
const MaxAttachmentSize = 5 * 1024 * 1024

// MaxAttachments is the maximum number of attachments per feedback item.
const MaxAttachments = 10

// ValidAttachmentTypes lists allowed content types.
var ValidAttachmentTypes = map[string]bool{
    "image/png":  true,
    "image/jpeg": true,
    "image/gif":  true,
    "image/webp": true,
    "application/json": true,
    "text/csv":   true,
    "text/plain": true,
}
```

---

## Files to Change

### 1. `internal/models/feedback.go`

- Add `FeedbackAttachment` struct (as above)
- Add `Attachments []FeedbackAttachment` field to `Feedback` struct after `ExpectedValue`
- Add validation constants: `MaxAttachmentSize`, `MaxAttachments`, `ValidAttachmentTypes`

### 2. `internal/server/handlers_feedback.go`

**handleFeedbackSubmit** (line 58-134):
- Add `Attachments` field to the submit body struct:
  ```go
  Attachments []struct {
      Filename    string `json:"filename"`
      ContentType string `json:"content_type"`
      Data        string `json:"data"` // base64-encoded
  } `json:"attachments"`
  ```
- After existing validation (line 91), add attachment validation:
  ```go
  // Validate attachments
  if len(body.Attachments) > models.MaxAttachments {
      WriteError(w, http.StatusBadRequest, fmt.Sprintf("too many attachments: max %d", models.MaxAttachments))
      return
  }
  for i, att := range body.Attachments {
      if att.Filename == "" {
          WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment %d: filename is required", i))
          return
      }
      if att.ContentType == "" {
          WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment %d: content_type is required", i))
          return
      }
      if !models.ValidAttachmentTypes[att.ContentType] {
          WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment %d: unsupported content_type %q", i, att.ContentType))
          return
      }
      if att.Data == "" {
          WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment %d: data is required", i))
          return
      }
      // Decode to validate base64 and check size
      decoded, err := base64.StdEncoding.DecodeString(att.Data)
      if err != nil {
          WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment %d: invalid base64 data", i))
          return
      }
      if len(decoded) > models.MaxAttachmentSize {
          WriteError(w, http.StatusBadRequest, fmt.Sprintf("attachment %d: exceeds max size of %d bytes", i, models.MaxAttachmentSize))
          return
      }
  }
  ```
- Convert body attachments to model attachments (after line 102):
  ```go
  for _, att := range body.Attachments {
      decoded, _ := base64.StdEncoding.DecodeString(att.Data)
      fb.Attachments = append(fb.Attachments, models.FeedbackAttachment{
          Filename:    att.Filename,
          ContentType: att.ContentType,
          SizeBytes:   len(decoded),
          Data:        att.Data,
      })
  }
  ```
- Add `"encoding/base64"` and `"fmt"` to imports

**handleFeedbackUpdate** (line 250-310):
- Add `Attachments` field to the update body struct (same struct as submit):
  ```go
  Attachments *[]struct {
      Filename    string `json:"filename"`
      ContentType string `json:"content_type"`
      Data        string `json:"data"`
  } `json:"attachments"`
  ```
  Note: pointer-to-slice so we can distinguish "not provided" (nil) from "set to empty" (empty slice).
- Same validation as submit when `body.Attachments != nil`
- Convert to model attachments and pass to store Update

**FeedbackStore.Update interface change**: The current `Update` signature has positional params. Since we're adding attachments, change the update approach:
- Add an `UpdateFeedback` struct to pass update fields (cleaner than more positional params)
- Actually — keep it simpler. Just add `attachments` param to the existing Update call.

**Revised approach for Update**: Since the current Update signature is already long, extend with one more param:
```go
Update(ctx context.Context, id string, status, resolutionNotes, userID, userName, userEmail string, attachments *[]models.FeedbackAttachment) error
```
When `attachments` is nil, don't touch the field. When non-nil (including empty slice), replace attachments.

### 3. `internal/interfaces/storage.go`

Update `FeedbackStore.Update` signature:
```go
Update(ctx context.Context, id string, status, resolutionNotes, userID, userName, userEmail string, attachments *[]models.FeedbackAttachment) error
```

### 4. `internal/storage/surrealdb/feedbackstore.go`

**feedbackSelectFields** (line 17): Add `attachments` to the SELECT list.

**Create** (line 34): Add `attachments = $attachments` to the UPSERT and add `"attachments": fb.Attachments` to vars.

**Update** (line 204): Add conditional `attachments` update when the parameter is non-nil:
```go
func (s *FeedbackStore) Update(ctx context.Context, id string, status, resolutionNotes, userID, userName, userEmail string, attachments *[]models.FeedbackAttachment) error {
    sql := "UPDATE $rid SET status = $status, resolution_notes = $notes, updated_by_user_id = $uid, updated_by_user_name = $uname, updated_by_user_email = $uemail, updated_at = $now"
    vars := map[string]any{...}
    if attachments != nil {
        sql += ", attachments = $attachments"
        vars["attachments"] = *attachments
    }
    ...
}
```

### 5. `internal/server/catalog.go`

**feedback_submit** tool definition: Add `attachments` param:
```go
{
    Name:        "attachments",
    Type:        "array",
    Description: "Optional media attachments. Each item: {filename, content_type, data (base64)}. Supported types: image/png, image/jpeg, image/gif, image/webp, application/json, text/csv, text/plain. Max 10 attachments, 5MB each.",
    In:          "body",
},
```

**feedback_update** tool definition: Add same `attachments` param.

**feedback_get_item**: No change needed — attachments are part of the Feedback struct and will be returned automatically.

### 6. Update all existing callers of `FeedbackStore.Update`

Search for all calls to `store.Update(ctx, ...)` in the codebase. The handler in `handlers_feedback.go` line 297 calls it. Also `BulkUpdateStatus` does NOT call `Update` (it has its own SQL), so no change needed there.

The handler call at line 297 becomes:
```go
// Convert body attachments if provided
var attachments *[]models.FeedbackAttachment
if body.Attachments != nil {
    atts := make([]models.FeedbackAttachment, 0, len(*body.Attachments))
    for _, att := range *body.Attachments {
        decoded, _ := base64.StdEncoding.DecodeString(att.Data)
        atts = append(atts, models.FeedbackAttachment{
            Filename:    att.Filename,
            ContentType: att.ContentType,
            SizeBytes:   len(decoded),
            Data:        att.Data,
        })
    }
    attachments = &atts
}

if err := store.Update(ctx, id, status, notes, userID, userName, userEmail, attachments); err != nil {
```

---

## Test Cases

### Unit tests (implementer writes in `internal/` near the code)

1. `TestFeedbackAttachment_ValidTypes` — validate all allowed content types
2. `TestFeedbackAttachment_InvalidTypes` — reject disallowed types (e.g. application/pdf, video/mp4)

### Integration tests (test-creator writes in `tests/`)

**Data layer** (`tests/data/feedback_attachments_test.go`):
1. `TestFeedbackCreate_WithSingleAttachment` — create feedback with 1 PNG attachment, verify persisted
2. `TestFeedbackCreate_WithMultipleAttachments` — create with 3 attachments (PNG, JSON, CSV), verify all returned
3. `TestFeedbackCreate_WithNoAttachments` — verify existing behavior unchanged
4. `TestFeedbackUpdate_AddAttachments` — update to add attachments to existing feedback
5. `TestFeedbackUpdate_ReplaceAttachments` — update to replace attachments
6. `TestFeedbackUpdate_ClearAttachments` — update with empty slice to clear attachments
7. `TestFeedbackUpdate_NilAttachments_PreservesExisting` — update with nil doesn't touch attachments

**API layer** (`tests/api/feedback_attachments_test.go`):
1. `TestFeedbackSubmit_WithAttachments` — POST with attachments, verify returned in GET
2. `TestFeedbackSubmit_TooManyAttachments` — >10 attachments returns 400
3. `TestFeedbackSubmit_InvalidContentType` — unsupported type returns 400
4. `TestFeedbackSubmit_InvalidBase64` — bad base64 returns 400
5. `TestFeedbackSubmit_OversizedAttachment` — >5MB returns 400
6. `TestFeedbackUpdate_WithAttachments` — PATCH to add/replace attachments
7. `TestFeedbackList_IncludesAttachmentMetadata` — list includes attachments

---

## Integration Points

- Model: `internal/models/feedback.go` — add struct + field + constants
- Interface: `internal/interfaces/storage.go:136` — update Update signature
- Storage: `internal/storage/surrealdb/feedbackstore.go` — update Create, Update, select fields
- Handlers: `internal/server/handlers_feedback.go:58,250` — update submit and update handlers
- Catalog: `internal/server/catalog.go` — update tool definitions for feedback_submit and feedback_update
