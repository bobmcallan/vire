# Requirements: vire-infra SurrealDB Compliance Update

**Date:** 2026-02-20
**Requested:** Review vire-infra and update to comply with the latest SurrealDB refactor

## Scope

**In scope:**
- Integrate SurrealDB into `docker/vire-stack.yml` (currently separate in `vire-data.yml`)
- Add `[storage]` section to `docker/vire-service.toml` and `docker/vire-service.toml.docker`
- Update `scripts/compose-deploy.sh` to handle SurrealDB service + health checks
- Update `scripts/compose-update.sh` to include surrealdb as a service option
- Update architecture/design docs to reflect SurrealDB instead of BadgerDB/file-based storage
- Update `docs/instruction-storage-separation.md` to reflect SurrealDB reality
- Pin SurrealDB image to `v2.2.1` (matching test containers)

**Out of scope:**
- Terraform cloud infrastructure changes (GCS/Firestore modules stay as-is for future cloud)
- vire-portal BadgerDB references (portal is a separate codebase)
- CI/CD workflow changes (no SurrealDB in the deploy pipeline yet)
- Credential rotation or security hardening of SurrealDB auth

## Approach

### 1. Docker Compose Integration
Merge SurrealDB from `vire-data.yml` into `vire-stack.yml` as a first-class service. The server service gets `depends_on: surrealdb` with a health check condition. Remove `vire-data.yml` (now redundant). Pin image to `surrealdb/surrealdb:v2.2.1`.

SurrealDB health check: `["CMD", "curl", "-f", "http://localhost:8000/health"]` or use `isready` — need to verify what SurrealDB supports.

### 2. Config Files
Add `[storage]` section to both TOML files matching vire's `StorageConfig`:
```toml
[storage]
address = "ws://surrealdb:8000/rpc"
namespace = "vire"
database = "vire"
username = "root"
password = "root"
```
The address uses `surrealdb` (Docker service name) instead of `localhost`.

### 3. Deploy Script
- Deploy SurrealDB as part of the stack (it's in vire-stack.yml now)
- Add SurrealDB health check to the wait loop
- Add SurrealDB to access info output

### 4. Update Script
- Add `surrealdb` as a valid `--service` option
- Update the `all` case to include surrealdb

### 5. Documentation Updates
- `docs/design-two-service-architecture.md`: Update "User profile in BadgerDB" → SurrealDB, token storage references, portal storage references
- `docs/architecture-per-user-deployment.md`: Update storage structure section, file-based references → SurrealDB
- `docs/instruction-storage-separation.md`: Mark as superseded by SurrealDB — the UserStore/DataStore/FileStore concepts are replaced by SurrealDB tables
- `docs/plan-portal-mcp-integration.md`: Update BadgerDB references

## Files Expected to Change

| File | Change |
|------|--------|
| `docker/vire-stack.yml` | Add surrealdb service, server depends_on, remove server-data volume |
| `docker/vire-data.yml` | DELETE (merged into vire-stack.yml) |
| `docker/vire-service.toml` | Add `[storage]` section |
| `docker/vire-service.toml.docker` | Add `[storage]` section |
| `scripts/compose-deploy.sh` | Add SurrealDB health check, update access info |
| `scripts/compose-update.sh` | Add surrealdb service option |
| `docs/design-two-service-architecture.md` | Update BadgerDB → SurrealDB references |
| `docs/architecture-per-user-deployment.md` | Update storage architecture sections |
| `docs/instruction-storage-separation.md` | Mark superseded, add SurrealDB note |
| `docs/plan-portal-mcp-integration.md` | Update BadgerDB references |
