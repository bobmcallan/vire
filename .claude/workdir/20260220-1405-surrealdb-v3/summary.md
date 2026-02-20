# Summary: Update SurrealDB to v3.0.0

**Date:** 2026-02-20
**Status:** Completed

## What Changed

| File | Change |
|------|--------|
| `internal/storage/surrealdb/manager.go` | Added `DEFINE TABLE IF NOT EXISTS` for all 6 tables after connection (v3 errors on querying non-existent tables) |
| `internal/storage/surrealdb/internalstore.go` | Added `isNotFoundError()` helper; `DeleteUser` and `DeleteUserKV` now tolerate deleting non-existent records (v3 returns error with ONLY keyword on missing records) |
| `internal/storage/surrealdb/userstore.go` | `Delete` now tolerates deleting non-existent records (same v3 behavior) |
| `internal/storage/surrealdb/testhelper_test.go` | Added `DEFINE TABLE IF NOT EXISTS` for all 6 tables in test DB setup |
| `tests/common/surrealdb.go` | Docker image `v2.2.1` -> `v3.0.0` |
| `tests/docker/docker-compose.yml` | Docker image `v2.2.1` -> `v3.0.0` |
| `README.md` | Updated SurrealDB version references from v2.2.1 to v3.0.0 |
| `.claude/skills/test-common/SKILL.md` | Updated SurrealDB version references |
| `.claude/skills/test-execute/SKILL.md` | Updated SurrealDB version references |
| vire-infra `docker/vire-stack.yml` | Docker image `v2.2.1` -> `v3.0.0` |

## Tests
- Unit tests pass against SurrealDB v3.0.0 container
- `go vet ./...` clean
- `go build ./...` clean
- Server builds and starts successfully

## Documentation Updated
- `README.md` — version references updated (prerequisite, Quick Start, Running SurrealDB sections)
- `.claude/skills/test-common/SKILL.md` — image version references
- `.claude/skills/test-execute/SKILL.md` — image version reference

## Devils-Advocate Findings

| # | Severity | Issue | Resolution |
|---|----------|-------|------------|
| 1 | CRITICAL | v3 errors on querying non-existent tables | Added DEFINE TABLE IF NOT EXISTS for all 6 tables at manager init |
| 2 | MODERATE | v3 returns error when deleting non-existent record with ONLY keyword | Added `isNotFoundError()` helper to tolerate missing-record deletes |
| 3 | LOW | `wait.ForLog("Started web server")` may differ in v3 | Verified — same log message in v3 |
| 4 | N/A | Go SDK compatibility | v1.3.0 already supports v3 — no SDK change needed |

## Notes

- **Go SDK unchanged:** `surrealdb.go` v1.3.0 already supports SurrealDB v3 (v3 support added in v1.1.0)
- **Query syntax unchanged:** All SurrealQL patterns (UPSERT, DELETE RETURN BEFORE, SELECT WHERE) work in v3
- **Storage backend unchanged:** `surrealkv://` still supported in v3
- **Startup command unchanged:** `start --user root --pass root` syntax same in v3
- **Two repos changed:** vire (this repo) and vire-infra (production Docker stack)
