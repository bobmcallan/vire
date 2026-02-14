# Summary: HTTP Server Architecture Refactor

**Date:** 2026-02-10
**Status:** Completed
**Version:** 0.3.0

## What Changed

| File | Change |
|------|--------|
| `internal/app/app.go` (NEW) | App struct with NewApp(), Close(), StartWarmCache(), StartPriceScheduler(), registerTools() — all service init extracted from old main.go |
| `internal/app/handlers.go` (MOVED) | Tool handlers moved from cmd/vire-mcp/ (2091 lines) |
| `internal/app/tools.go` (MOVED) | Tool definitions moved from cmd/vire-mcp/ (553 lines) |
| `internal/app/formatters.go` (MOVED) | Response formatters moved from cmd/vire-mcp/ (1322 lines) |
| `internal/app/warmcache.go` (MOVED) | Cache warming moved from cmd/vire-mcp/ |
| `internal/app/scheduler.go` (MOVED) | Price scheduler moved from cmd/vire-mcp/ |
| `internal/app/rebuild.go` (MOVED) | Schema version check moved from cmd/vire-mcp/ |
| `internal/app/app_test.go` (NEW) | 5 tests: App init, tool registration, get_version e2e, Close idempotency, invalid config |
| `internal/app/*_test.go` (MOVED) | All test files moved from cmd/vire-mcp/ |
| `cmd/vire-server/main.go` (NEW) | HTTP server on :4242, StreamableHTTPServer for MCP, REST API, graceful shutdown |
| `cmd/vire-server/server_test.go` (NEW) | 5 tests: health, version, MCP POST, MCP GET, method-not-allowed |
| `cmd/vire-mcp/main.go` (REPLACED) | Thin stdio proxy (~117 lines) forwarding JSON-RPC to HTTP server |
| `cmd/vire-mcp/proxy_test.go` (NEW) | 4 tests: JSON-RPC forwarding, server unavailable, stdin close, content-type |
| `docker/Dockerfile` | Builds both binaries, vire-server as entrypoint, EXPOSE 4242, wget healthcheck |
| `docker/docker-compose.yml` | vire-server entrypoint, port 4242:4242, HTTP healthcheck |
| `docker/docker-compose.ghcr.yml` | Same updates as docker-compose.yml |
| `scripts/deploy.sh` | Updated log/health/MCP URL hints |
| `README.md` | Architecture section, endpoints table, deployment instructions for both transports |
| `docs/http-server-plan.md` | Marked COMPLETED |
| `docs/performance-plan.md` | Added architecture refactor note |
| `.version` | 0.2.22 -> 0.3.0 |
| `.claude/skills/develop/SKILL.md` | Updated Key Directories table |

## Tests
- 14 new tests across 3 packages (internal/app: 5, cmd/vire-server: 5, cmd/vire-mcp: 4)
- All existing tests preserved and passing in new locations
- Full suite: 20 packages, all green
- Docker container: deployed, healthy, E2E validated

## Documentation Updated
- README.md — two-binary architecture, endpoints, deployment
- docs/http-server-plan.md — COMPLETED
- docs/performance-plan.md — refactor note
- .claude/skills/develop/SKILL.md — directory table
- 12 skill files reviewed, all transport-agnostic (no changes needed)

## Devils-Advocate Findings
| Finding | Severity | Resolution |
|---------|----------|------------|
| SSE streaming in proxy | CRITICAL | Not a problem — WithStateLess(true) on POST returns JSON, not SSE |
| No server notifications | CRITICAL | Accepted — documented as known limitation |
| Concurrent storage safety | CRITICAL | Accepted — atomic file ops + single-user serialization |
| WriteTimeout too short | HIGH | Fixed: 120s -> 300s |
| Warm cache detached context | HIGH | Fixed: cancel func stored on App, called in Close() |
| Data race in proxy test | BLOCKING | Fixed: channel for receivedContentType |
| Graceful shutdown ordering | HIGH | Fixed: HTTP -> scheduler -> warmcache -> storage |
| ReadAll on SSE response | MEDIUM | Accepted as low risk with WithStateLess(true) |
| Redundant ENTRYPOINT | LOW | Cosmetic, not blocking |
| TOCTOU in storage | MEDIUM | Pre-existing, out of scope |

## Notes
- Version bumped to 0.3.0 (architecture-level change)
- Three reviewer suggestions deferred: interface types in App struct, ServerConfig in config.go, internal/server/ package — all valid but not blocking for current scope
- The `/api/diagnostics` REST endpoint from the plan was descoped — diagnostics available via MCP `get_diagnostics` tool
- HEAD request support added to health/version handlers for BusyBox wget compatibility
