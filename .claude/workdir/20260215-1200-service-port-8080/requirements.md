# Requirements: Update default service port to 8080

**Date:** 2026-02-15
**Requested:** Change the internal/default service port from 4242 to 8080. Keep the Docker external (host-facing) port as 4242.

## Scope

**In scope:**
- Change Go config default port from 4242 to 8080
- Update Dockerfile EXPOSE directives
- Update Docker Compose port mappings (external:internal → 4242:8080)
- Update container-internal health checks to use 8080
- Update container-to-container URLs (e.g., claude_desktop_config.json)
- Update test infrastructure (containers.go, test docker files)
- Update documentation

**Out of scope:**
- Changing the external/host-facing port (stays 4242)
- Changing the VIRE_PORT env var behavior (still controls external port, defaults to 4242)

## Approach

Two categories of port references:
- **Internal** (Go default, EXPOSE, health checks inside container, container-to-container): 4242 → **8080**
- **External** (host-facing curl, deploy script health checks, skill files, README user-facing examples): stays **4242**

### Files to Change — Internal Port (4242 → 8080)

1. `internal/common/config.go` line 140 — default `Port: 4242` → `Port: 8080`
2. `docker/Dockerfile.server` line 50 — `EXPOSE 4242` → `EXPOSE 8080`
3. `tests/docker/Dockerfile.server` line 29 — `EXPOSE 4242` → `EXPOSE 8080`
4. `tests/docker/Dockerfile.server` line 32 — health check `localhost:4242` → `localhost:8080`
5. `docker/docker-compose.yml` line 15 — `"${VIRE_PORT:-4242}:4242"` → `"${VIRE_PORT:-4242}:8080"`
6. `docker/docker-compose.yml` line 21 — health check `localhost:4242` → `localhost:8080`
7. `docker/docker-compose.ghcr.yml` line 9 — `"${VIRE_PORT:-4242}:4242"` → `"${VIRE_PORT:-4242}:8080"`
8. `docker/docker-compose.ghcr.yml` line 13 — health check `localhost:4242` → `localhost:8080`
9. `tests/docker/docker-compose.yml` — health check `localhost:4242` → `localhost:8080`
10. `tests/common/containers.go` lines 121, 135, 149 — port `"4242/tcp"` → `"8080/tcp"`
11. `docker/claude_desktop_config.json` line 8 — `http://vire-server:4242` → `http://vire-server:8080`

### Files to Keep at 4242 — External Port

- `scripts/deploy.sh` line 102 — `curl http://localhost:4242/api/health` (host-facing)
- `.claude/skills/deploy/SKILL.md` — all health check URLs (host-facing)
- `.claude/skills/develop/SKILL.md` — all health check URLs (host-facing)
- `README.md` — user-facing curl examples stay 4242

### Files to Update — Documentation

- `README.md` — update architecture table to show internal port is 8080, external is 4242
- `docker/README.md` — update port reference
