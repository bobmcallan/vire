# Summary: Add update_feedback MCP tool

**Date:** 2026-02-23
**Status:** completed

## What Changed

| File | Change |
|------|--------|
| `internal/server/catalog.go` | Added `update_feedback` tool definition (PATCH /api/feedback/{id}) with id, status, resolution_notes params |
| `internal/server/handlers_feedback.go` | Removed `requireAdmin()` from `handleFeedbackUpdate` so MCP clients can update feedback status |
| `internal/server/catalog_test.go` | Updated expected tool count from 27 to 29 (was already stale at 27, actual was 28 before this change) |

## Tests
- All catalog tests pass (9/9)
- Build clean, vet clean

## Notes
- Bulk update and delete endpoints still require admin — only single-item update is opened to MCP
- The `submit_feedback` endpoint already had no admin requirement, so this is consistent
