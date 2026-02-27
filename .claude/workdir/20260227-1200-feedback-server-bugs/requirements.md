# Requirements: Fix feedback server bugs

## Bug 1: PATCH /api/feedback/{id} overwrites resolution_notes (Critical)

**Problem:** When only `status` is provided in a PATCH body (e.g. `{"status": "acknowledged"}`),
`resolution_notes` is reset to empty string. The `status` field has fallback logic to preserve
the existing value when empty, but `resolution_notes` does not.

**Fix:** Add the same preservation logic for `resolution_notes` — if `body.ResolutionNotes` is
empty, use the existing value from the database.

**File:** `internal/server/handlers_feedback.go` (handleFeedbackUpdate, ~line 278)

## Bug 2: get_feedback catalog missing params (Medium)

**Problem:** The MCP tool catalog for `get_feedback` is missing `sort`, `before`, and `session_id`
parameters that the handler supports. MCP clients can't discover these capabilities.

**Fix:** Add the 3 missing parameter definitions to the catalog.

**File:** `internal/server/catalog.go` (get_feedback definition, ~line 53)

## Bug 3: List endpoint silently ignores invalid filter values (Low-Medium)

**Problem:** Invalid `status`, `severity`, or `category` filter values on GET /api/feedback
are passed to SurrealDB without validation, silently returning empty results instead of
returning a 400 error.

**Fix:** Validate filter values against their respective valid maps before querying.

**File:** `internal/server/handlers_feedback.go` (handleFeedbackList, ~line 137)

## Files expected to change

| File | Change |
|------|--------|
| `internal/server/handlers_feedback.go` | Fix resolution_notes preservation, add filter validation |
| `internal/server/catalog.go` | Add missing sort/before/session_id params |
| `internal/server/handlers_feedback_test.go` | Add tests for fixes |
