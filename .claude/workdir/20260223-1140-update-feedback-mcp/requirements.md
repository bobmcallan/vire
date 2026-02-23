# Requirements: Add update_feedback MCP tool

**Date:** 2026-02-23
**Requested:** fb_8c613647 — MCP tool gap: no tool to update feedback item status. Need an update_feedback MCP tool.

## Scope
- **In scope:** Add `update_feedback` MCP tool definition in catalog; remove admin-only restriction from PATCH handler
- **Out of scope:** Bulk update MCP tool, delete MCP tool, new storage methods

## Approach
The HTTP handler already exists (`PATCH /api/feedback/{id}` in `handlers_feedback.go`). Two changes:
1. Add tool definition to `catalog.go` with `id`, `status`, and `resolution_notes` params
2. Remove `requireAdmin()` from `handleFeedbackUpdate` — MCP clients that submit feedback should be able to update it

## Files Expected to Change
- `internal/server/catalog.go` — new tool definition
- `internal/server/handlers_feedback.go` — remove admin requirement
- `internal/server/catalog_test.go` — update tool count
