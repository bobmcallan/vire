# Summary: Fix get_stock_data include param + Add user context to feedback

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/server/handlers.go` | Replaced `r.URL.Query().Get("include")` with `parseStockDataInclude()` helper supporting both array and CSV formats |
| `internal/models/feedback.go` | Added UserID, UserName, UserEmail, UpdatedByUserID, UpdatedByUserName, UpdatedByUserEmail fields |
| `internal/interfaces/storage.go` | Updated FeedbackStore.Update() signature with user identity params |
| `internal/storage/surrealdb/feedbackstore.go` | Added user fields to feedbackSelectFields, Create SQL, Update SQL |
| `internal/server/handlers_feedback.go` | Extract UserContext in submit + update handlers, look up user name/email, TrimSpace on UserID |
| `docs/architecture/api.md` | Added Feedback Endpoints section |
| `docs/features/20260223-mcp-feedback-channel.md` | Added user context columns to schema table |

## Tests
- Unit tests: 10 include param tests (parseStockDataInclude), all pass
- Stress tests: 24 include param + 12 feedback user context security tests
- Integration tests: 17 feedback tests (all pass), stockdata include tests
- Fix rounds: 1 (test expectation alignment — tests expected 401/403 on intentionally public handler)

## Architecture
- Architect reviewed and approved — flagged updated_by fields being write-only, implementer added to model/select
- docs/architecture/api.md updated with feedback endpoints
- docs/architecture/storage.md already accurate

## Devils-Advocate
- 36 stress tests across both features
- 3 findings: broken test expectations (fixed), whitespace UserID (TrimSpace added), bulk update user context (deferred — scope expansion)
- No security vulnerabilities found

## Notes
- Feedback update endpoint is intentionally public (no admin requirement) — MCP clients can update status
- Bulk update and delete remain admin-only
- updated_by fields capture who last updated a feedback item (audit trail)
- Pre-existing integration test bugs found and fixed (createAdminUser helper used wrong endpoint)
