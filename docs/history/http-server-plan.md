# Vire HTTP Server Architecture Plan

**Date:** 2026-02-10
**Status:** COMPLETED (2026-02-10)
**Reference:** quaero project (`/home/bobmc/development/quaero`)

---

## Problem Statement

Vire MCP runs as a stdio-only binary inside Docker. Each Claude interaction spawns a new process via `docker exec -i vire-mcp ./vire-mcp`, which:

1. **Full startup every time** — config load, storage init, API client creation, schema check, cache warm (~2-3s)
2. **No persistent state** — warm cache, scheduler, and in-memory logs die when the process exits
3. **No observability** — `docker logs` shows nothing (container entrypoint is `tail -f /dev/null`, not the binary)
4. **No health check** — container healthcheck only verifies the binary exists (`test -x ./vire-mcp`)
5. **No API access** — only reachable via MCP stdio; no REST, no web UI, no external integrations

## Solution: Two-Binary Architecture

### Binary 1: `cmd/vire-server/` — The Service

Long-running HTTP server on a configurable port (default `8501`). Starts once, runs continuously.

**Responsibilities:**
- MCP over Streamable HTTP at `/mcp` (using mcp-go's `StreamableHTTPServer`)
- REST API at `/api/*` for external integrations
- Health endpoint at `/api/health` for Docker healthcheck
- Version endpoint at `/api/version`
- All service initialization (storage, API clients, cache, scheduler)
- Persistent warm cache and scheduled price refresh (already implemented, currently wasted)
- Arbor logging with console + file writers (already implemented)
- Future: web UI at `/` for portfolio dashboard

**Key design:**
```
vire-server
├── Loads config, initializes storage/clients/services (once)
├── Creates MCPServer with all tools registered
├── Wraps in StreamableHTTPServer for HTTP MCP transport
├── Adds REST API routes (health, version, diagnostics)
├── Starts HTTP server on :8501
├── Warm cache + scheduler run as background goroutines
└── Logs to console + file (visible via docker logs)
```

### Binary 2: `cmd/vire-mcp/` — The Stdio Proxy

Lightweight stdio wrapper for Claude Desktop / MCP client compatibility. Does NOT initialize services.

**Responsibilities:**
- Reads JSON-RPC from stdin
- Forwards to `http://localhost:8501/mcp` (or configurable URL)
- Writes JSON-RPC response to stdout
- Zero service initialization — just HTTP forwarding

**Key design:**
```
vire-mcp (stdio proxy)
├── Reads VIRE_SERVER_URL env var (default: http://localhost:8501)
├── Reads JSON-RPC request from stdin
├── POSTs to {VIRE_SERVER_URL}/mcp
├── Writes response to stdout
└── Exits (or loops for session)
```

**Startup time:** <10ms (no config, no storage, no API clients)

---

## Implementation Plan

### Phase A: Create `cmd/vire-server/`

**A.1 — Extract service initialization into shared package**

Currently `cmd/vire-mcp/main.go` contains all initialization logic. Extract into a shared internal package so both binaries can reference it without duplication.

Create `internal/app/app.go`:
```go
type App struct {
    Config          *common.Config
    Logger          *common.Logger
    Storage         *storage.StorageManager
    EODHDClient     *eodhd.Client
    NavexaClient    *navexa.Client
    GeminiClient    *gemini.Client
    MarketService   *market.Service
    PortfolioService *portfolio.Service
    ReportService   *report.Service
    SignalService   *signal.Service
    StrategyService *strategy.Service
    PlanService     *plan.Service
    WatchlistService *watchlist.Service
    MCPServer       *server.MCPServer
}

func NewApp(configPath string) (*App, error) { ... }
func (a *App) Close() { ... }
```

**Files to create:**
- `internal/app/app.go` — App struct with init, tool registration, and cleanup

**Files to modify:**
- `cmd/vire-mcp/main.go` — refactor to use `app.NewApp()` (keeps working as-is during transition)

**A.2 — Create HTTP server entry point**

Create `cmd/vire-server/main.go`:
```go
func main() {
    app, err := app.NewApp(configPath)
    // ...

    // MCP over Streamable HTTP
    httpMCP := server.NewStreamableHTTPServer(app.MCPServer,
        server.WithEndpointPath("/mcp"),
        server.WithStateLess(true),
    )

    // HTTP mux
    mux := http.NewServeMux()
    mux.Handle("/mcp", httpMCP)
    mux.HandleFunc("/api/health", healthHandler)
    mux.HandleFunc("/api/version", versionHandler)

    // Start server
    srv := &http.Server{
        Addr:         fmt.Sprintf(":%d", port),
        Handler:      mux,
        ReadTimeout:  30 * time.Second,
        WriteTimeout: 120 * time.Second,
        IdleTimeout:  60 * time.Second,
    }
    srv.ListenAndServe()
}
```

**Files to create:**
- `cmd/vire-server/main.go` — HTTP server entry point

**A.3 — Add REST API handlers**

Minimal REST endpoints for health, version, and diagnostics.

| Endpoint | Method | Response |
|----------|--------|----------|
| `/api/health` | GET | `{"status":"ok"}` |
| `/api/version` | GET | `{"version":"0.2.22","build":"...","commit":"..."}` |
| `/api/diagnostics` | GET | Same data as `get_diagnostics` MCP tool |

**Files to create:**
- `cmd/vire-server/api.go` — REST handlers

### Phase B: Refactor `cmd/vire-mcp/` to Stdio Proxy

**B.1 — Replace service initialization with HTTP client**

Strip out all service init. Replace with an HTTP client that forwards JSON-RPC to the server.

```go
func main() {
    serverURL := os.Getenv("VIRE_SERVER_URL")
    if serverURL == "" {
        serverURL = "http://localhost:8501"
    }

    // Read JSON-RPC from stdin, POST to server, write response to stdout
    proxy := NewStdioProxy(serverURL + "/mcp")
    proxy.Run()
}
```

The proxy reads newline-delimited JSON-RPC from stdin, sends each as an HTTP POST to the server's `/mcp` endpoint, and writes the response to stdout.

**Files to modify:**
- `cmd/vire-mcp/main.go` — replace with proxy implementation

**Files to remove:**
- `cmd/vire-mcp/handlers.go` — tool handlers (moved to shared app)
- `cmd/vire-mcp/tools.go` — tool definitions (moved to shared app)
- `cmd/vire-mcp/formatters.go` — response formatters (moved to shared app)
- `cmd/vire-mcp/warmcache.go` — cache warming (moved to server)
- `cmd/vire-mcp/scheduler.go` — price scheduler (moved to server)
- `cmd/vire-mcp/rebuild.go` — rebuild logic (moved to shared app)

**B.2 — Update test harness**

Tests currently test handlers directly. Move handler tests to `internal/app/` or keep them alongside the handlers in their new location.

### Phase C: Docker & Deploy Updates

**C.1 — Update Dockerfile**

Build both binaries. Server is the entrypoint.

```dockerfile
# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="..." -o vire-server ./cmd/vire-server
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="..." -o vire-mcp ./cmd/vire-mcp

# Runtime
COPY --from=builder /build/vire-server .
COPY --from=builder /build/vire-mcp .

EXPOSE 8501
ENTRYPOINT ["./vire-server"]
```

**C.2 — Update docker-compose.yml**

```yaml
services:
  vire-mcp:
    container_name: vire-mcp
    entrypoint: ["./vire-server"]       # was: ["tail", "-f", "/dev/null"]
    ports:
      - "${VIRE_PORT:-8501}:8501"
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8501/api/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

**Changes from current setup:**
- `entrypoint` becomes the server binary (not `tail -f /dev/null`)
- `ports` exposed for HTTP access
- `healthcheck` uses HTTP endpoint (not `test -x ./vire-mcp`)
- `stdin_open: true` no longer needed (server is long-running)
- `docker logs` now shows real server output

**C.3 — Update deploy.sh**

- Remove `docker exec` references
- MCP clients (Claude Desktop) connect via HTTP to `http://localhost:8501/mcp`
- Or use `docker exec vire-mcp ./vire-mcp` as stdio proxy (connects to server internally)
- `docker logs vire-mcp` shows real logs (no more `tail -f /dev/null`)

**C.4 — Update Claude Desktop MCP config**

Before (stdio via docker exec):
```json
{
  "mcpServers": {
    "vire": {
      "command": "docker",
      "args": ["exec", "-i", "vire-mcp", "./vire-mcp"]
    }
  }
}
```

After (Streamable HTTP — preferred):
```json
{
  "mcpServers": {
    "vire": {
      "url": "http://localhost:8501/mcp"
    }
  }
}
```

Or (stdio proxy — fallback for clients that don't support HTTP):
```json
{
  "mcpServers": {
    "vire": {
      "command": "docker",
      "args": ["exec", "-i", "vire-mcp", "./vire-mcp"]
    }
  }
}
```

---

## File Movement Summary

| Current Location | New Location | Notes |
|------------------|-------------|-------|
| `cmd/vire-mcp/main.go` | `cmd/vire-server/main.go` + `internal/app/app.go` | Service init moves to shared app |
| `cmd/vire-mcp/handlers.go` | `internal/app/handlers.go` | Shared between server + tests |
| `cmd/vire-mcp/tools.go` | `internal/app/tools.go` | Tool definitions |
| `cmd/vire-mcp/formatters.go` | `internal/app/formatters.go` | Response formatting |
| `cmd/vire-mcp/warmcache.go` | `internal/app/warmcache.go` | Server-only (always running) |
| `cmd/vire-mcp/scheduler.go` | `internal/app/scheduler.go` | Server-only (always running) |
| `cmd/vire-mcp/rebuild.go` | `internal/app/rebuild.go` | Schema version check |
| `cmd/vire-mcp/main.go` (new) | — | Thin stdio proxy (~50 lines) |

---

## What This Solves

| Problem | Before | After |
|---------|--------|-------|
| Startup per request | ~2-3s (full init) | <10ms (HTTP proxy) or 0ms (direct HTTP) |
| Warm cache persistence | Lost on process exit | Persistent (server is long-running) |
| Scheduler persistence | Lost on process exit | Persistent |
| `docker logs` | Empty (entrypoint is `tail`) | Real server logs |
| Health check | `test -x ./vire-mcp` (useless) | HTTP `/api/health` (real) |
| External API access | None | REST at `/api/*` |
| MCP transport | stdio only | Streamable HTTP + stdio proxy |
| Future web UI | Not possible | Serve at `/` |

---

## Dependencies

- **mcp-go v0.43.2** — already has `StreamableHTTPServer` with `Start()`, `ServeHTTP()`, and `WithEndpointPath()`. No library upgrade needed.
- **Phase 3 (Concurrency)** — independent of this refactor. Can be done before or after.
- **Phase 5 (Cleanup)** — merge into this refactor (dead code removal during file moves).

## Implementation Order

```
Phase A: Create vire-server (additive — nothing breaks)
  A.1 Extract shared app package from cmd/vire-mcp/main.go
  A.2 Create cmd/vire-server/main.go with HTTP + StreamableHTTP MCP
  A.3 Add REST API endpoints (health, version, diagnostics)

Phase B: Refactor vire-mcp to proxy (breaking change — coordinated)
  B.1 Replace cmd/vire-mcp/ with stdio proxy (~50 lines)
  B.2 Move tests to internal/app/

Phase C: Docker & deployment
  C.1 Update Dockerfile (build both, server as entrypoint)
  C.2 Update docker-compose.yml (ports, healthcheck, no stdin_open)
  C.3 Update deploy.sh and docs
  C.4 Update Claude Desktop config
```

Phase A is fully additive — the existing `vire-mcp` stdio binary continues to work unchanged. Phase B is the breaking change that replaces the standalone MCP binary with a thin proxy. Phase C updates deployment.

---

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| mcp-go StreamableHTTP untested in vire | Phase A.2 — validate before any destructive changes |
| Claude Desktop doesn't support HTTP MCP | Stdio proxy (Phase B) provides fallback |
| Port conflicts in Docker | Configurable via `VIRE_PORT` env var |
| Test breakage from file moves | Phase A is additive; Phase B moves tests alongside handlers |
| Schema version migration | No schema change — same storage layer |
