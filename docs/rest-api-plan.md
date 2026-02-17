# Vire REST API Separation Plan

**Date:** 2026-02-10
**Status:** ✅ COMPLETED
**Depends on:** docs/http-server-plan.md (COMPLETED)

> **Implementation Note (2026-02-10):** This plan has been fully implemented.
> - REST API: `internal/server/` (handlers.go, routes.go, server.go)
> - MCP Proxy: `cmd/vire-mcp/` (proxy.go, handlers.go, tools.go, formatters.go)
> - vire-server has zero MCP dependencies
> - All 40 MCP tools map to REST endpoints

---

## Problem Statement

The current architecture embeds the MCP server inside vire-server. The MCPServer, tool definitions, and handler logic all live in `internal/app/` and are directly wired into vire-server. This creates a tight coupling between the HTTP service and the MCP protocol.

The target architecture separates concerns:
- **vire-server** — pure REST API + HTML UI service. No MCP knowledge.
- **vire-mcp** — thin MCP-to-REST translator. Supports `--stdio` (Claude Desktop) or Streamable HTTP (Claude Code/CLI, default).

## Target Architecture

```
┌─────────────────┐    ┌──────────────────────────────────┐
│  Claude Desktop  │    │          Claude Code / CLI        │
│  (MCP stdio)     │    │     (MCP Streamable HTTP)         │
└────────┬────────┘    └───────────────┬──────────────────┘
         │ stdin/stdout                 │ HTTP POST
         │                              │
    ┌────▼──────────────────────────────▼────┐
    │            vire-mcp                     │
    │  --stdio  → reads stdin, writes stdout  │
    │  default  → Streamable HTTP on :8501    │
    │                                         │
    │  MCP tool call → REST request           │
    │  REST response → MCP tool result        │
    └────────────────┬────────────────────────┘
                     │ HTTP REST
                     │
    ┌────────────────▼────────────────────────┐
    │           vire-server (:8500)            │
    │                                          │
    │  REST API  /api/*                        │
    │  HTML UI   / (future)                    │
    │  Health    /api/health                   │
    │  Version   /api/version                  │
    │                                          │
    │  Services, storage, API clients,         │
    │  warm cache, scheduler                   │
    └──────────────────────────────────────────┘
```

**Docker (3 services on one machine):**
```
vire-server      always running     :8500   REST API + HTML UI
vire-mcp         always running     :8501   Streamable HTTP MCP (for Claude Code)
vire-mcp         per session        stdio   spawned by Claude Desktop via docker exec --stdio
```

---

## REST API Design

### Design Principles

1. **JSON in, JSON out** — all endpoints accept/return JSON
2. **RESTful naming** — resource-oriented paths, HTTP verbs for actions
3. **Portfolio as path param** — `/api/portfolios/{name}/...` where `{name}` defaults to the configured default
4. **Consistent error format** — `{"error": "message", "code": "ERROR_CODE"}`
5. **No MCP dependency** — vire-server has zero mcp-go imports

### Endpoint Mapping (40 MCP tools → REST API)

#### System

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `get_version` | `/api/version` | GET | Already exists |
| `get_config` | `/api/config` | GET | |
| `get_diagnostics` | `/api/diagnostics` | GET | `?correlation_id=X&limit=50` |

#### Portfolios

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `list_portfolios` | `/api/portfolios` | GET | |
| `get_portfolio` | `/api/portfolios/{name}` | GET | Fast, cached holdings |
| `portfolio_review` | `/api/portfolios/{name}/review` | POST | Body: `{"focus_signals":[], "include_news":false}` |
| `sync_portfolio` | `/api/portfolios/{name}/sync` | POST | Body: `{"force":false}` |
| `rebuild_data` | `/api/portfolios/{name}/rebuild` | POST | |
| `set_default_portfolio` | `/api/portfolios/default` | PUT | Body: `{"name":"SMSF"}` |
| `get_portfolio_snapshot` | `/api/portfolios/{name}/snapshot` | GET | `?date=2025-01-30` |
| `get_portfolio_history` | `/api/portfolios/{name}/history` | GET | `?from=X&to=Y&format=auto` |

#### Market Data

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `get_stock_data` | `/api/market/stocks/{ticker}` | GET | `?include=price,fundamentals,signals,news` |
| `detect_signals` | `/api/market/signals` | POST | Body: `{"tickers":[], "signal_types":[]}` |
| `collect_market_data` | `/api/market/collect` | POST | Body: `{"tickers":[], "include_news":false}` |

#### Screening

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `stock_screen` | `/api/screen` | POST | Body: `{"exchange":"AU", "limit":5, ...}` |
| `market_snipe` | `/api/screen/snipe` | POST | Body: `{"exchange":"AU", "limit":3, ...}` |
| `funnel_screen` | `/api/screen/funnel` | POST | Body: `{"exchange":"AU", "limit":5, ...}` |
| `list_searches` | `/api/searches` | GET | `?type=screen&exchange=AU&limit=10` |
| `get_search` | `/api/searches/{id}` | GET | |

#### Reports

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `generate_report` | `/api/portfolios/{name}/reports` | POST | Body: `{"force_refresh":false, "include_news":false}` |
| `generate_ticker_report` | `/api/portfolios/{name}/reports/{ticker}` | POST | |
| `list_reports` | `/api/reports` | GET | `?portfolio_name=SMSF` |
| `get_summary` | `/api/portfolios/{name}/summary` | GET | Auto-generates if missing |
| `get_ticker_report` | `/api/portfolios/{name}/reports/{ticker}` | GET | Auto-generates if missing |
| `list_tickers` | `/api/portfolios/{name}/tickers` | GET | |

#### Strategy

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `get_strategy_template` | `/api/strategies/template` | GET | `?account_type=smsf` |
| `get_portfolio_strategy` | `/api/portfolios/{name}/strategy` | GET | |
| `set_portfolio_strategy` | `/api/portfolios/{name}/strategy` | PUT | Body: strategy JSON |
| `delete_portfolio_strategy` | `/api/portfolios/{name}/strategy` | DELETE | |

#### Plans

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `get_portfolio_plan` | `/api/portfolios/{name}/plan` | GET | |
| `set_portfolio_plan` | `/api/portfolios/{name}/plan` | PUT | Body: plan JSON |
| `add_plan_item` | `/api/portfolios/{name}/plan/items` | POST | Body: item JSON |
| `update_plan_item` | `/api/portfolios/{name}/plan/items/{id}` | PATCH | Body: partial item JSON |
| `remove_plan_item` | `/api/portfolios/{name}/plan/items/{id}` | DELETE | |
| `check_plan_status` | `/api/portfolios/{name}/plan/status` | GET | |

#### Watchlist

| MCP Tool | REST Endpoint | Method | Notes |
|----------|--------------|--------|-------|
| `get_watchlist` | `/api/portfolios/{name}/watchlist` | GET | |
| `set_watchlist` | `/api/portfolios/{name}/watchlist` | PUT | Body: watchlist JSON |
| `add_watchlist_item` | `/api/portfolios/{name}/watchlist/items` | POST | Body: item JSON |
| `update_watchlist_item` | `/api/portfolios/{name}/watchlist/items/{ticker}` | PATCH | Body: partial item JSON |
| `remove_watchlist_item` | `/api/portfolios/{name}/watchlist/items/{ticker}` | DELETE | |

---

## Implementation Plan

### Phase 1: REST API Layer on vire-server

Add REST API endpoints that call the same service layer. The MCP endpoint stays during this phase.

**1.1 — Create `internal/server/` package**

```
internal/server/
├── server.go        # Server struct, NewServer(), Start(), Shutdown()
├── routes.go        # Route registration
├── handlers.go      # REST API handlers (HTTP → service calls → JSON response)
├── helpers.go       # WriteJSON, RequireMethod, ParsePortfolioName, etc.
└── middleware.go     # Logging, CORS, correlation ID
```

The REST handlers call the same service interfaces that MCP handlers currently call. No business logic duplication.

**Pattern:**
```go
// REST handler in internal/server/handlers.go
func (s *Server) handlePortfolioReview(w http.ResponseWriter, r *http.Request) {
    name := s.parsePortfolioName(r)
    var req struct {
        FocusSignals []string `json:"focus_signals"`
        IncludeNews  bool     `json:"include_news"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    review, err := s.app.PortfolioService.ReviewPortfolio(r.Context(), name, ...)
    if err != nil {
        WriteJSON(w, 500, ErrorResponse{Error: err.Error()})
        return
    }
    WriteJSON(w, 200, review)
}
```

**1.2 — Add middleware**

- Correlation ID (from arbor)
- Request logging (method, path, duration, status)
- CORS (for future web UI)
- Recovery (panic → 500)

**1.3 — Response format**

REST endpoints return **structured JSON** (not markdown). This differs from MCP handlers which return formatted markdown text. The REST API returns the raw data models; formatting is the client's responsibility.

Example: `GET /api/portfolios/SMSF` returns:
```json
{
  "name": "SMSF",
  "holdings": [...],
  "total_value": 460000,
  "last_synced": "2026-02-10T08:49:42Z"
}
```

The MCP tool `get_portfolio` returns a markdown table. This is a deliberate distinction — REST returns data, MCP returns presentation.

### Phase 2: Rewrite vire-mcp as REST Client

**2.1 — MCP tool handlers become REST callers**

Replace the current handler pattern (service call → format → MCP response) with:

```go
// MCP handler calls REST API
func (p *MCPProxy) handlePortfolioReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    name := extractString(req, "portfolio_name")
    body := map[string]interface{}{
        "focus_signals": extractArray(req, "focus_signals"),
        "include_news":  extractBool(req, "include_news"),
    }
    resp, err := p.post(fmt.Sprintf("/api/portfolios/%s/review", name), body)
    if err != nil {
        return mcp.NewToolResultError(err.Error()), nil
    }
    return mcp.NewToolResultText(formatReviewAsMarkdown(resp)), nil
}
```

**Key insight:** The MCP layer keeps the markdown formatting logic. REST returns JSON, MCP formats it for human consumption. The formatters.go file stays in the MCP binary.

**2.2 — Transport selection via flag**

```go
func main() {
    stdio := flag.Bool("stdio", false, "Use stdio transport (for Claude Desktop)")
    flag.Parse()

    serverURL := os.Getenv("VIRE_SERVER_URL")
    if serverURL == "" {
        serverURL = "http://localhost:8500"
    }

    proxy := NewMCPProxy(serverURL)

    if *stdio {
        // Stdio transport — reads stdin, writes stdout
        server.ServeStdio(proxy.MCPServer)
    } else {
        // Streamable HTTP transport — listens on :8501
        port := os.Getenv("VIRE_MCP_PORT")
        if port == "" { port = "8501" }
        httpServer := server.NewStreamableHTTPServer(proxy.MCPServer,
            server.WithStateLess(true),
        )
        httpServer.Start(":" + port)
    }
}
```

**2.3 — File layout**

```
cmd/vire-mcp/
├── main.go          # Flag parsing, transport selection, startup
├── proxy.go         # MCPProxy struct, NewMCPProxy(), REST client
├── handlers.go      # MCP tool handlers (REST call → format → MCP response)
├── tools.go         # MCP tool definitions (unchanged from current)
├── formatters.go    # Markdown formatting (moved back from internal/app/)
└── proxy_test.go    # Tests
```

### Phase 3: Remove MCP from vire-server

**3.1 — Strip MCP from internal/app/**

- Remove `MCPServer` field from App struct
- Remove `registerTools()` method
- Remove `tools.go` (MCP tool definitions)
- Remove MCP handler functions from `handlers.go`
- Keep `formatters.go` only if REST handlers need it (likely not — REST returns JSON)
- Remove `github.com/mark3labs/mcp-go` dependency from `internal/app/`

**3.2 — vire-server uses internal/server/**

```go
// cmd/vire-server/main.go
func main() {
    a, _ := app.NewApp(configPath)
    srv := server.NewServer(a)
    srv.Start()
}
```

No more `buildMux()` in cmd/vire-server. All HTTP wiring moves to `internal/server/`.

**3.3 — Clean up internal/app/**

After MCP removal, internal/app/ becomes purely the application core:
```
internal/app/
├── app.go           # App struct, NewApp(), Close()
├── warmcache.go     # Background cache warming
├── scheduler.go     # Background price refresh
└── rebuild.go       # Schema version check
```

### Phase 4: Docker & Deploy

**4.1 — Dockerfile**

```dockerfile
# Build both binaries
RUN go build -o vire-server ./cmd/vire-server
RUN go build -o vire-mcp ./cmd/vire-mcp

COPY --from=builder /build/vire-server .
COPY --from=builder /build/vire-mcp .

EXPOSE 8500 8501
ENTRYPOINT ["./vire-server"]
```

**4.2 — docker-compose.yml**

```yaml
name: vire

services:
  vire-server:
    container_name: vire-server
    build:
      context: ..
      dockerfile: docker/Dockerfile
    entrypoint: ["./vire-server"]
    ports:
      - "${VIRE_PORT:-8500}:8500"
    volumes:
      - vire-data:/app/data
      - vire-logs:/app/logs
    healthcheck:
      test: ["CMD", "wget", "--spider", "-q", "http://localhost:8500/api/health"]
      interval: 30s
      timeout: 5s
      retries: 3
    restart: unless-stopped

  vire-mcp:
    container_name: vire-mcp
    image: vire-vire-server:latest
    entrypoint: ["./vire-mcp"]
    ports:
      - "${VIRE_MCP_PORT:-8501}:8501"
    environment:
      - VIRE_SERVER_URL=http://vire-server:8500
    depends_on:
      vire-server:
        condition: service_healthy
    restart: unless-stopped

volumes:
  vire-data:
  vire-logs:
```

**Key Docker notes:**
- Both binaries are in the same image
- vire-mcp container uses the server image but runs `./vire-mcp`
- `VIRE_SERVER_URL` uses Docker internal DNS (`http://vire-server:8500`)
- vire-mcp depends on vire-server being healthy
- Stdio proxy: `docker exec -i vire-mcp ./vire-mcp --stdio`

**4.3 — Claude Desktop config**

```json
{
  "mcpServers": {
    "vire": {
      "command": "docker",
      "args": ["exec", "-i", "vire-mcp", "./vire-mcp", "--stdio"]
    }
  }
}
```

**4.4 — Claude Code config**

```json
{
  "mcpServers": {
    "vire": {
      "url": "http://localhost:8501/mcp"
    }
  }
}
```

---

## Migration Strategy

Phases 1-4 are sequential but each phase produces a working system:

1. **After Phase 1:** vire-server has both REST and MCP. Everything works as today, plus REST API available for testing.
2. **After Phase 2:** vire-mcp works as REST client OR old embedded MCP still works. Both paths available.
3. **After Phase 3:** MCP removed from vire-server. Clean separation.
4. **After Phase 4:** Docker updated for two-service model.

Phase 1 is the largest (40 REST endpoints). Phase 2 is medium (rewrite MCP handlers as REST clients + keep formatters). Phase 3 is small (delete code). Phase 4 is config changes.

---

## Response Format: REST vs MCP

| Layer | Format | Example |
|-------|--------|---------|
| REST API | Structured JSON | `{"holdings": [...], "total_value": 460000}` |
| MCP tool | Markdown text | `## Portfolio Review\n\| Ticker \| Value \| ...` |

The MCP layer formats JSON into markdown for LLM consumption. The REST API returns raw JSON for programmatic use (web UI, external integrations, scripts).

Formatters (`formatters.go`) move from `internal/app/` to `cmd/vire-mcp/` — they're MCP presentation logic, not business logic.

---

## Estimated Effort

| Phase | Files | Effort |
|-------|-------|--------|
| Phase 1: REST API | ~5 new files in internal/server/ | Large (40 endpoints) |
| Phase 2: MCP client rewrite | ~5 files in cmd/vire-mcp/ | Medium |
| Phase 3: Strip MCP from server | Delete/simplify ~3 files | Small |
| Phase 4: Docker/deploy | ~4 config files | Small |

---

## Risk Assessment

| Risk | Mitigation |
|------|-----------|
| 40 REST endpoints is a lot of code | Many endpoints are thin (GET returns JSON from storage). Group by resource. |
| MCP formatting breaks when data format changes | REST returns typed structs, MCP formatters consume the same structs. Type safety. |
| Two containers increases deploy complexity | docker-compose handles orchestration. depends_on ensures ordering. |
| REST API needs auth eventually | Placeholder middleware — add auth header check later. |
| Stdio proxy can't reach vire-server | docker exec runs inside the vire-mcp container which has Docker DNS access to vire-server. |
