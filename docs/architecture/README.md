# Vire Architecture

System architecture reference for the Vire MCP server. Maintained by the architect role during development.

## System Overview

Single Go binary (`vire-mcp`) serving MCP over SSE on port 4242. Caddy handles TLS termination. Production deployment via Docker.

## Documents

| Document | Covers |
|----------|--------|
| [Directory Structure](directories.md) | Project layout, key packages |
| [Storage](storage.md) | 3-area storage layout, stores, migration |
| [API Surface](api.md) | HTTP endpoints, middleware, user context |
| [Auth & OAuth](auth.md) | JWT, OAuth providers, MCP OAuth 2.1 |
| [Services](services.md) | Market, portfolio, signals, reports, cashflow |
| [Job Manager](jobs.md) | Queue, watcher, processor, job types |
| [Admin](admin.md) | Admin API, stock index |

## Conventions

- Module path: `github.com/bobmcallan/vire`
- Config: `vire.toml` / `config/vire-service.toml`, defaults in `internal/common/config.go`
- Schema version in `internal/common/version.go` — bump when model changes invalidate cached data
- API keys resolved via `ResolveAPIKey()` (env > KV store > config fallback)
