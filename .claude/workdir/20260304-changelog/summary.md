# Summary: Changelog System

**Status:** completed
**Date:** 2026-03-04
**Schema:** 14 → 15

## Changes

| File | Change |
|------|--------|
| `internal/models/changelog.go` | **NEW** — ChangelogEntry struct (service, version, build, markdown content, author tracking) |
| `internal/interfaces/storage.go` | Added ChangelogStore interface, ChangelogListOptions, StorageManager accessor |
| `internal/storage/surrealdb/changelogstore.go` | **NEW** — SurrealDB implementation (CRUD, paginated list, merge-update) |
| `internal/storage/surrealdb/manager.go` | Added changelogStore field, table define, init, accessor |
| `internal/server/handlers_changelog.go` | **NEW** — HTTP handlers with auth guards (POST=adminOrService, PATCH/DELETE=admin, GET=open) |
| `internal/server/routes.go` | Added changelog route registration |
| `internal/server/catalog.go` | Added 4 MCP tool definitions |
| `internal/server/catalog_test.go` | Tool count assertion (already 70 with changelog tools) |
| `internal/common/version.go` | Schema version 14 → 15 |
| `tests/data/changelog_test.go` | **NEW** — 20 integration tests (CRUD, pagination, filtering, merge semantics) |
| 13 mock test files | Added ChangelogStore() stub to mock StorageManagers |

## New MCP Tools (4)

| Tool | Method | Path |
|------|--------|------|
| `changelog_list` | GET | `/api/changelog` |
| `changelog_add` | POST | `/api/changelog` |
| `changelog_update` | PATCH | `/api/changelog/{id}` |
| `changelog_delete` | DELETE | `/api/changelog/{id}` |

## Tests

- Integration tests: 20/20 PASS (changelog_test.go)
- Unit tests: all changelog-related pass
- Pre-existing failures: app (API keys), roleEscalation (live DB), purgeCharts (blob migration)
- Fix rounds: 0

## Architecture

- ChangelogStore follows FeedbackStore pattern (dedicated SurrealDB table, UPSERT, pagination)
- Auth: POST = requireAdminOrService (admin + portal service user), PATCH/DELETE = requireAdmin, GET = open
- ID format: "cl_" + 8 hex chars
- SurrealDB table: `changelog`
- Merge-update semantics on PATCH (only non-empty fields updated)

## Devils-Advocate

- No blockers found
- 6 findings, all low/medium severity, consistent with existing codebase patterns
- No SurrealQL injection risks (all parameterized queries)

## Notes

- Schema bump 14→15 purges cached derived data on restart
- Portal can submit changelog entries via service user registration
- Content field is markdown, max 50000 chars
- Service name is free-form text, max 100 chars
