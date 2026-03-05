# Summary: Feedback Media Attachments

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/models/feedback.go` | Added `FeedbackAttachment` struct, `Attachments` field on Feedback, validation constants (MaxAttachmentSize 5MB, MaxAttachments 10, ValidAttachmentTypes) |
| `internal/interfaces/storage.go` | Updated `FeedbackStore.Update` signature with `attachments *[]models.FeedbackAttachment` 8th param |
| `internal/storage/surrealdb/feedbackstore.go` | Added `attachments` to SELECT fields, UPSERT in Create, conditional update in Update |
| `internal/server/handlers_feedback.go` | Added attachment validation/conversion in submit and update handlers, `validateAttachment` helper with `filepath.Base` sanitization |
| `internal/server/catalog.go` | Added `attachments` param to `feedback_submit` and `feedback_update` tool definitions |
| `docs/architecture/26-02-27-storage.md` | Added attachments to FeedbackStore section |
| `docs/architecture/26-02-27-api.md` | Added attachments to Feedback Endpoints section |
| `tests/data/feedbackstore_test.go` | Fixed existing Update() calls with nil 8th param |
| `internal/storage/surrealdb/feedbackstore_test.go` | Fixed existing Update() calls with nil 8th param |
| `internal/storage/surrealdb/feedbackstore_stress_test.go` | Fixed existing Update() calls with nil 8th param |

## Tests

- Unit tests: 10 added (2 model validation + 8 handler tests)
- Integration tests: 14 created (7 data layer + 7 API layer)
- Test results: 128 data tests pass, 10 API pass, 368 API skipped (no Docker), 25 internal packages pass
- Fix rounds: 1 (reviewer found alignment issue + false positive unused import)

## Architecture

- Architect approved: all 6 checkpoints passed, no fixes needed
- Docs updated: storage.md and api.md

## Devils-Advocate

- 5 findings: 2 fixed (path traversal sanitization, double decode elimination), 3 acknowledged as low-priority post-launch (aggregate size limit, case-insensitive content types, content types with params)

## Design

- **Storage**: Inline base64 in SurrealDB (no blob store needed for feedback-scale data)
- **Supported types**: image/png, image/jpeg, image/gif, image/webp, application/json, text/csv, text/plain
- **Limits**: Max 10 attachments, 5MB each
- **Update semantics**: nil = preserve existing, empty slice = clear, non-nil = replace
