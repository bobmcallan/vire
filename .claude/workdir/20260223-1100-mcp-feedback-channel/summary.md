# Summary: MCP Feedback Channel

**Date:** 2026-02-23
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/models/feedback.go` | New — Feedback struct, FeedbackSummary, category/severity/status constants |
| `internal/models/feedback_test.go` | New — Unit tests for model constants and validation |
| `internal/interfaces/storage.go` | Added FeedbackStore interface, FeedbackListOptions, StorageManager accessor |
| `internal/storage/surrealdb/feedbackstore.go` | New — SurrealDB implementation (Create, Get, List, Update, BulkUpdateStatus, Delete, Summary) |
| `internal/storage/surrealdb/feedbackstore_test.go` | New — Unit tests for feedback store |
| `internal/storage/surrealdb/feedbackstore_stress_test.go` | New — Stress tests for edge cases and hostile inputs |
| `internal/storage/surrealdb/testhelper_test.go` | Updated — Added FeedbackStore mock |
| `internal/storage/surrealdb/manager.go` | Added mcp_feedback table, feedbackStore field, constructor, accessor |
| `internal/server/handlers_feedback.go` | New — All HTTP handlers (submit, list, get, update, bulk update, delete, summary, route dispatcher) |
| `internal/server/handlers_feedback_test.go` | New — Unit tests for handlers |
| `internal/server/handlers_feedback_stress_test.go` | New — Stress tests for handlers (input validation, auth, edge cases) |
| `internal/server/routes.go` | Added /api/feedback and /api/feedback/ route registrations |
| `internal/server/catalog.go` | Added submit_feedback MCP tool definition, extended get_diagnostics with feedback params |
| `internal/server/catalog_test.go` | Updated — Tests for submit_feedback tool in catalog |
| `internal/services/portfolio/service_test.go` | Updated — Added FeedbackStore to mock |
| `internal/services/jobmanager/manager_test.go` | Updated — Added FeedbackStore to mock |
| `internal/services/report/devils_advocate_test.go` | Updated — Added FeedbackStore to mock |
| `internal/services/market/bulk_stress_test.go` | Updated — Added FeedbackStore to mock |
| `internal/services/market/service_test.go` | Updated — Added FeedbackStore to mock |
| `internal/services/plan/service_test.go` | Updated — Added FeedbackStore to mock |
| `tests/api/feedback_test.go` | New — API integration tests (full CRUD lifecycle) |
| `tests/data/feedbackstore_test.go` | New — Data layer integration tests |
| `README.md` | Updated — Features, MCP tools, endpoints, storage sections |
| `docs/features/20260223-mcp-feedback-channel.md` | Updated — Implementation note about /api/feedback vs /api/v1/feedback |
| `.claude/skills/develop/SKILL.md` | Updated — FeedbackStore reference |

## Tests
- Unit tests: `internal/models/feedback_test.go`, `internal/storage/surrealdb/feedbackstore_test.go`, `internal/server/handlers_feedback_test.go`, `internal/server/catalog_test.go`
- Stress tests: `internal/storage/surrealdb/feedbackstore_stress_test.go`, `internal/server/handlers_feedback_stress_test.go`
- Integration tests: `tests/api/feedback_test.go`, `tests/data/feedbackstore_test.go`
- Test results: All passing after 3 feedback loop rounds (ID deserialization fix, nil slice fix, deterministic sort tiebreaker)
- go vet: Clean

## Documentation Updated
- README.md — Features list, MCP Tools section, endpoint table, Storage section
- docs/features/20260223-mcp-feedback-channel.md — Implementation note
- .claude/skills/develop/SKILL.md — FeedbackStore reference

## Devils-Advocate Findings
- **Severity sort was lexicographic** — Fixed to use numeric mapping (high=3, medium=2, low=1) for correct sort ordering
- **No request body size limit** — Fixed by adding `http.MaxBytesReader` to POST handler to prevent oversized payloads

## API Endpoints Implemented
- `POST /api/feedback` — Submit feedback (202 Accepted)
- `GET /api/feedback` — List with filters (status, severity, category, ticker, portfolio_name, session_id, since, before, page, per_page, sort)
- `GET /api/feedback/summary` — Aggregate counts by status/severity/category
- `GET /api/feedback/{id}` — Get single feedback item
- `PATCH /api/feedback/{id}` — Update status + resolution notes (admin only)
- `PATCH /api/feedback/bulk` — Bulk status update (admin only)
- `DELETE /api/feedback/{id}` — Hard delete (admin only)
- `GET /api/diagnostics?include_feedback=true` — Extended diagnostics with feedback

## MCP Tool
- `submit_feedback` — registered in tool catalog with all parameters from requirements doc

## Notes
- API uses `/api/feedback` (not `/api/v1/feedback` from the requirements doc) to match existing codebase conventions
- SurrealDB table is `mcp_feedback` (schemaless, auto-created on startup)
- Feedback IDs use `fb_` prefix for easy identification
- Admin endpoints protected by `requireAdmin()` middleware
