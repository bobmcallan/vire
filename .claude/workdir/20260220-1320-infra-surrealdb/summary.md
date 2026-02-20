# Infra SurrealDB Migration Summary

**Date:** 2026-02-20
**Repo:** vire-infra
**Context:** Align vire-infra Docker stack and documentation with vire's migration from BadgerHold/file-based storage to SurrealDB (vire commit `5a089fc`).

---

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `docker/vire-stack.yml` | Modified | Added `surrealdb` service, updated `server` with `depends_on` and chart volume, added `surrealdb-data` volume |
| `docker/vire-service.toml` | Modified | Added `[storage]` section with SurrealDB connection config and `data_path` |
| `docker/vire-service.toml.docker` | Modified | Added `[storage]` section with SurrealDB connection config and `data_path` |
| `docker/vire-data.yml` | Deleted | Standalone SurrealDB compose file merged into vire-stack.yml |
| `scripts/compose-deploy.sh` | Modified | Added SurrealDB health check (`vire-db`) to wait loop, access info, and header comment |
| `scripts/compose-update.sh` | Modified | Added `surrealdb` service option, fixed project name from `-p vire` to `-p vire-stack` |
| `docs/design-two-service-architecture.md` | Modified | Updated BadgerDB references to SurrealDB throughout |
| `docs/architecture-per-user-deployment.md` | Modified | Replaced file-based storage description with SurrealDB tables, updated StorageConfig |
| `docs/instruction-storage-separation.md` | Modified | Added deprecation note (pre-SurrealDB historical reference) |
| `docs/plan-portal-mcp-integration.md` | Modified | Updated all BadgerDB/BADGER_PATH references to SurrealDB |

---

## Docker Changes

### SurrealDB Service (new in vire-stack.yml)
- Image: `surrealdb/surrealdb:v2.2.1` (pinned version)
- Container: `vire-db`
- Storage: `surrealkv://data/vire.db` on named volume `surrealdb-data`
- Credentials: root/root (dev only)
- Network: internal only (not exposed to host)
- Healthcheck: `surreal isready --endpoint http://localhost:8000` (uses PATH, no leading `/`)
- Start period: 30s (increased from default 10s to give SurrealDB time for initial DB creation)

### Server Service (updated)
- Added `depends_on: surrealdb: condition: service_healthy` so server waits for SurrealDB
- Added `server-data:/app/data/market` volume for chart files written by `WriteRaw()`
- Chart files (generated PNGs) are persisted across container restarts

### Removed
- `docker/vire-data.yml` deleted (was a standalone SurrealDB compose with exposed ports and external network; now properly integrated into the stack)

---

## Config Changes

Both `vire-service.toml` and `vire-service.toml.docker` received:

```toml
[storage]
address = "ws://surrealdb:8000/rpc"
namespace = "vire"
database = "vire"
username = "root"
password = "root"
data_path = "data/market"
```

These match vire's `StorageConfig` struct in `internal/common/config.go`. The `data_path` field is required for `WriteRaw()` chart file generation.

---

## Script Fixes

### compose-deploy.sh
- Header comment updated: "portal + server" -> "portal + server + surrealdb"
- SurrealDB (`vire-db`) added to health check wait loop
- SurrealDB access info added (internal only)

### compose-update.sh
- **Bug fix:** Project name changed from `-p vire` to `-p vire-stack` to match compose-deploy.sh (previously would create duplicate containers under wrong project)
- Added `surrealdb` as valid `--service` option
- Added surrealdb to `all` case (updated first, before portal and server)

---

## Documentation Updates

### design-two-service-architecture.md
- User profile storage: BadgerDB -> SurrealDB `internal_user` table
- Token storage: BadgerDB -> SurrealDB
- Session revocation: BadgerDB -> SurrealDB
- Docker compose example: Added SurrealDB service, updated depends_on, replaced vire-data volume
- Environment variables: `VIRE_BADGER_PATH` -> `VIRE_STORAGE_ADDRESS`
- GCP architecture diagram: BadgerDB/Firestore -> SurrealDB
- Portal config: `[storage.badger]` -> `[storage]` with SurrealDB fields
- Portal code structure: `badger/` -> `surrealdb/`

### architecture-per-user-deployment.md
- ASCII diagram: "File-based storage (UserStore + DataStore split)" -> "SurrealDB storage"
- Storage manager description: Updated to SurrealDB connection via WebSocket
- Storage structure section: Replaced entire file tree with SurrealDB table descriptions
- Added StorageConfig struct reference
- API key resolution: KV store path updated to SurrealDB `user_kv` table

### instruction-storage-separation.md
- Added deprecation header note pointing to `internal/storage/surrealdb/` as current implementation
- Rest of document preserved as historical reference

### plan-portal-mcp-integration.md
- All BadgerDB references -> SurrealDB (11 occurrences)
- `VIRE_BADGER_PATH` env var -> `VIRE_STORAGE_ADDRESS` (2 occurrences in docker-compose examples)

---

## Devils-Advocate Findings (addressed)

| # | Severity | Issue | Resolution |
|---|----------|-------|------------|
| 1 | BUG | Missing `data_path` in TOML configs + no volume for chart files | Added `data_path = "data/market"` to both configs, added `server-data:/app/data/market` volume |
| 2 | BUG | Project name mismatch (`-p vire` vs `-p vire-stack`) in compose-update.sh | Fixed to `-p vire-stack` |
| 3 | MODERATE | `/surreal` path may not exist in all versions | Changed to `surreal` (PATH resolution) |
| 4 | MODERATE | SurrealDB start_period too short (10s) | Increased to 30s |
| 5 | NOTE | Race condition: server has no retry on SurrealDB connection | Pre-existing vire server issue; `restart: unless-stopped` recovers; `depends_on: service_healthy` mitigates |
| 6 | N/A | Stale BadgerDB references in docs | Addressed in task #4 (documentation updates) |
| 7 | N/A | `.gitignore` entry `docker/vire-data/` | Kept — harmless, covers standalone SurrealDB runs |
| 8 | N/A | Old vire-data.yml used `latest`, new stack pins v2.2.1 | Confirmed improvement, no action needed |
| 9 | MINOR | `--purge` uses system-wide `docker system prune` | Not changed — pre-existing behaviour, not part of SurrealDB migration scope |

---

## Validation Results

- `docker compose -f docker/vire-stack.yml config` validates successfully
- No stale references to `vire-data.yml`, `BadgerDB`, `badger`, `BADGER` remain in docker/ or docs/
- SurrealDB service properly integrated with healthcheck and dependency ordering
- Project names consistent between deploy and update scripts

---

## Notes

- **Production credentials:** The root/root credentials in the storage config are for development only. Production deployments should use proper secrets management.
- **SurrealDB version:** Pinned to v2.2.1 to match the version tested with vire's storage migration.
- **Port exposure:** SurrealDB port 8000 is intentionally not exposed to the host. The old `vire-data.yml` exposed it; the new integrated service keeps it internal-only.
- **Volume layout:** `server-data` volume persists chart files at `/app/data/market` in the container. `surrealdb-data` volume persists the SurrealDB database at `/data` in the SurrealDB container.
