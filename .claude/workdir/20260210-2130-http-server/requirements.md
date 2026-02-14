# Requirements: HTTP Server Architecture Refactor

**Date:** 2026-02-10
**Requested:** Implement the two-binary HTTP server architecture from docs/http-server-plan.md

## Scope
- Extract service initialization from cmd/vire-mcp/main.go into internal/app/
- Create cmd/vire-server/ with HTTP server, Streamable HTTP MCP, REST API
- Refactor cmd/vire-mcp/ into a lightweight stdio proxy
- Move handlers, tools, formatters, warmcache, scheduler, rebuild to internal/app/
- Move tests alongside their new locations
- Update Docker (Dockerfile, docker-compose.yml) for HTTP server entrypoint
- Update deploy.sh for new architecture
- Update documentation (README.md, affected skills)

## Out of Scope
- Phase 3 (Concurrency) from performance-plan.md
- Web UI (future, placeholder only)
- GHCR docker-compose.ghcr.yml (updated for compatibility but not primary focus)

## Approach
Three-phase implementation per docs/http-server-plan.md:
- Phase A: Create vire-server (additive, nothing breaks)
- Phase B: Refactor vire-mcp to stdio proxy (breaking change)
- Phase C: Docker & deployment updates

## Files Expected to Change
- NEW: internal/app/app.go, tools.go, handlers.go, formatters.go, warmcache.go, scheduler.go, rebuild.go
- NEW: cmd/vire-server/main.go, api.go
- MODIFIED: cmd/vire-mcp/main.go (replaced with proxy)
- REMOVED: cmd/vire-mcp/handlers.go, tools.go, formatters.go, warmcache.go, scheduler.go, rebuild.go
- MOVED: cmd/vire-mcp/*_test.go → internal/app/*_test.go
- MODIFIED: docker/Dockerfile, docker/docker-compose.yml, docker/docker-compose.ghcr.yml
- MODIFIED: scripts/deploy.sh
- MODIFIED: README.md, .version
