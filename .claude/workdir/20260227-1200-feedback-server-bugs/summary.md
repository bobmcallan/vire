# Summary: Fix feedback server bugs

**Status:** completed

## Changes

| File | Change |
|------|--------|
| `internal/server/handlers_feedback.go` | Fix PATCH resolution_notes overwrite; add filter validation on list endpoint |
| `internal/server/catalog.go` | Add missing `session_id`, `before`, `sort` params to get_feedback tool |
| `internal/server/handlers_feedback_test.go` | Add 5 unit tests for bug fixes |
| `internal/server/catalog_test.go` | Add catalog param completeness test |

## Bugs Fixed

### Bug 1: PATCH /api/feedback/{id} overwrites resolution_notes (Critical)
- **Problem:** When updating only status, resolution_notes was reset to empty string
- **Cause:** `body.ResolutionNotes` defaults to `""` and was passed directly to store.Update()
- **Fix:** Added preservation logic matching the existing status pattern — if notes are empty, use existing value

### Bug 2: get_feedback catalog missing params (Medium)
- **Problem:** MCP clients couldn't discover `session_id`, `before`, `sort` filter capabilities
- **Cause:** Catalog definition only had 8 of 11 supported params
- **Fix:** Added 3 missing parameter definitions to catalog

### Bug 3: List endpoint silently ignores invalid filters (Low-Medium)
- **Problem:** Invalid status/severity/category filter values returned empty results without error
- **Cause:** No validation before passing filters to SurrealDB
- **Fix:** Added validation against ValidFeedbackStatuses/Severities/Categories maps, returning 400 for invalid values

## Tests
- 5 unit tests added for bug fixes
- 1 catalog test added for param completeness
- TestHandleFeedbackUpdate_PreservesResolutionNotes — verifies notes survive status-only update
- TestHandleFeedbackList_InvalidStatusFilter — verifies 400 on bad status
- TestHandleFeedbackList_InvalidSeverityFilter — verifies 400 on bad severity
- TestHandleFeedbackList_InvalidCategoryFilter — verifies 400 on bad category
- TestHandleFeedbackList_ValidFiltersAccepted — verifies valid filters work
- TestBuildToolCatalog_GetFeedbackHasAllParams — verifies all 11 params present
