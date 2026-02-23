# Requirements: MCP Feedback Channel

**Date:** 2026-02-23
**Requested:** Implement a lightweight feedback channel allowing MCP clients (Claude) to report data quality issues, calculation anomalies, and observations back to vire-server in real time.

## Scope

### In Scope
- `Feedback` model in `internal/models/feedback.go`
- `FeedbackStore` interface in `internal/interfaces/storage.go`
- SurrealDB implementation in `internal/storage/surrealdb/feedbackstore.go`
- Storage manager wiring (table definition, store instantiation, accessor)
- HTTP handlers in `internal/server/handlers_feedback.go`
- Route registration in `internal/server/routes.go`
- MCP tool catalog entries in `internal/server/catalog.go` (`submit_feedback`)
- Extension of `get_diagnostics` to optionally include recent feedback
- Admin endpoints for listing, triaging, updating, and deleting feedback
- Summary/stats endpoint for dashboard
- Bulk status update endpoint
- Unit tests for store and handlers

### Out of Scope
- vire-portal UI (separate codebase)
- Two-way feedback (vire responding to Claude)
- Automated remediation
- Export to external issue trackers

## Approach

### Storage
- New SurrealDB table `mcp_feedback` (schemaless, following existing patterns)
- Record IDs use `fb_` prefix + short UUID
- Append-only by default; only admin can update status/notes or hard delete

### Model
- `Feedback` struct with fields matching the requirements doc: id, session_id, client_type, category, severity, description, ticker, portfolio_name, tool_name, observed_value (json.RawMessage), expected_value (json.RawMessage), status, resolution_notes, created_at, updated_at
- Category constants: data_anomaly, sync_delay, calculation_error, missing_data, schema_change, tool_error, observation
- Severity constants: low, medium, high
- Status constants: new, acknowledged, resolved, dismissed

### API Endpoints (using `/api/feedback` pattern, not `/api/v1/`)
- `POST /api/feedback` — submit feedback (202 Accepted, returns feedback_id)
- `GET /api/feedback` — list with filters (status, severity, category, ticker, portfolio_name, session_id, since, before, page, per_page, sort)
- `GET /api/feedback/summary` — aggregate counts by status/severity/category
- `GET /api/feedback/{id}` — get single item
- `PATCH /api/feedback/{id}` — update status + resolution_notes (admin)
- `PATCH /api/feedback/bulk` — bulk status update (admin)
- `DELETE /api/feedback/{id}` — hard delete (admin)

### MCP Tool
- `submit_feedback` tool in catalog mapping to `POST /api/feedback`
- Parameters: category (enum, required), description (required), ticker, portfolio_name, tool_name, observed_value, expected_value, severity (enum, default medium)

### Diagnostics Extension
- Add `include_feedback`, `feedback_since`, `feedback_severity`, `feedback_status` params to `get_diagnostics`
- When enabled, append recent feedback items to diagnostics output

### Route Structure
- `mux.HandleFunc("/api/feedback", s.handleFeedback)` — POST (submit) and GET (list)
- `mux.HandleFunc("/api/feedback/", s.routeFeedback)` — sub-routes for /{id}, /summary, /bulk

## Files Expected to Change

| File | Change |
|------|--------|
| `internal/models/feedback.go` | New — Feedback struct and constants |
| `internal/interfaces/storage.go` | Add FeedbackStore interface and FeedbackListOptions |
| `internal/storage/surrealdb/feedbackstore.go` | New — SurrealDB implementation |
| `internal/storage/surrealdb/manager.go` | Add table, store field, constructor call, accessor |
| `internal/server/handlers_feedback.go` | New — all feedback HTTP handlers |
| `internal/server/routes.go` | Add feedback route registrations |
| `internal/server/catalog.go` | Add submit_feedback MCP tool definition |
| `internal/server/handlers.go` | Extend handleDiagnostics for feedback inclusion |
