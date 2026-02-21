# Vire

Portfolio Compliance Engine — rules-based MCP service for Australian equities.

Vire connects to Claude (via [MCP](https://modelcontextprotocol.io/)) to provide real-time portfolio compliance checks, stock analysis, technical indicators, and company filings intelligence. It aggregates data from EODHD, Navexa, ASX announcements, and uses Google Gemini for AI-powered summaries.

> **Disclaimer:** Vire is an information tool, not a financial adviser. All output reflects rules-based indicator computations, not personal financial advice. Users should consult a licensed financial adviser before making investment decisions.

## Features

- **Portfolio Compliance** — Sync holdings from Navexa, analyse positions with compliance status classifications
- **Portfolio Strategy** — Define and store investment strategies per portfolio with devil's advocate validation
- **Real-Time Quotes** — Live price quotes for stocks, forex pairs, and commodities via EODHD
- **Stock Analysis** — Real-time price, fundamentals, technical indicators, company releases with extracted financials, and structured company timeline for any ASX/US ticker
- **Technical Indicators** — SMA, RSI, MACD, volume, regime detection, relative strength, support/resistance
- **Company Filings Intelligence** — ASX announcement scraping, PDF extraction, and Gemini-powered financial analysis
- **News Intelligence** — AI-summarised news sentiment per ticker
- **Strategy Scanner** — Scan for tickers matching strategy entry criteria
- **Stock Screening** — Screen stocks by quantitative filters with consistent returns and credible news support
- **Report Generation** — Fast portfolio reports using core market data (EOD + fundamentals); detailed analysis (filings, AI) collected in background
- **Stock Index** — Shared cross-user ticker registry with per-component freshness tracking, auto-populated from portfolio syncs
- **Job Queue** — Priority-based background data collection with 9 discrete job types, configurable concurrency, admin API, and real-time WebSocket monitoring

## MCP Tools

### Market Data

| Tool | Description |
|------|-------------|
| `get_quote` | Real-time price quote for any ticker — stocks (BHP.AU), forex (AUDUSD.FOREX), commodities (XAUUSD.FOREX). Returns OHLCV, change%, and previous close. |
| `get_stock_data` | Real-time price, fundamentals, indicators, company releases (per-filing extracted financials), company timeline, and news for a ticker |
| `compute_indicators` | Compute technical indicators for tickers |
| `strategy_scanner` | Scan for tickers matching strategy entry criteria |
| `stock_screen` | Screen stocks by quantitative filters: low P/E, consistent returns |

### Portfolio

| Tool | Description |
|------|-------------|
| `portfolio_compliance` | Full portfolio analysis with real-time prices, compliance status classifications, company releases, and company timeline per holding |
| `get_portfolio` | Get current portfolio holdings — tickers, names, values, weights, and gains |
| `get_portfolio_stock` | Get portfolio position data for a single holding — position details, trade history, dividends, and returns. Supports `force_refresh` to re-sync from Navexa |
| `list_portfolios` | List available portfolios |
| `set_default_portfolio` | Set or view the default portfolio |

### Reports

| Tool | Description |
|------|-------------|
| `generate_report` | Generate portfolio report (fast — core market data only, detailed data collected in background) |
| `get_summary` | Get cached portfolio summary |
| `list_reports` | List available reports with timestamps |

### Strategy

| Tool | Description |
|------|-------------|
| `get_strategy_template` | Field reference with valid values, guidance tables, and examples |
| `set_portfolio_strategy` | Create or update a portfolio strategy (merge semantics) |
| `get_portfolio_strategy` | View the strategy document as formatted markdown |
| `delete_portfolio_strategy` | Delete a portfolio strategy |

### Plan

| Tool | Description |
|------|-------------|
| `get_portfolio_plan` | Get the current investment plan for a portfolio |
| `set_portfolio_plan` | Set or update the investment plan |
| `add_plan_item` | Add a single action item to a portfolio plan |
| `update_plan_item` | Update an existing plan item by ID (merge semantics) |
| `remove_plan_item` | Remove a plan item by ID |
| `check_plan_status` | Evaluate plan status: checks event triggers and deadline expiry |

### System

| Tool | Description |
|------|-------------|
| `get_version` | Server version and status |
| `get_config` | List all configuration settings |
| `get_diagnostics` | Server diagnostics: uptime, recent logs, per-request traces via correlation ID |

## Architecture

Vire is transitioning to a two-service architecture:

| Service | Repo | External Port | Internal Port | Role |
|---------|------|---------------|---------------|------|
| **vire-server** | `vire` | `:8501` | `:8080` | Backend API — market data, portfolio analysis, compliance, storage |
| **vire-portal** | `vire-portal` | `:8080` | `:8080` | User-facing — UI, OAuth 2.1, MCP proxy |

**Target state:** The portal fetches the tool catalog from vire-server (`GET /api/mcp/tools`), dynamically registers MCP tools, and proxies tool calls with `X-Vire-User-ID`. The server resolves user preferences (portfolios, display currency, navexa key) internally from the user profile. No hardcoded tool definitions in the portal.

```
Claude / MCP Client
  │
  │  POST /mcp (OAuth 2.1 authenticated)
  ▼
vire-portal (:8080)
  │  Dynamic tool registration from catalog
  │  Per-user header: X-Vire-User-ID
  │  Proxies tool calls to vire-server
  ▼
vire-server (:8501)
     REST API, background job queue, admin WebSocket
     GET /api/mcp/tools → tool catalog for portal bootstrap
     User storage: profiles, preferences, credentials
     Per-request context: resolves portfolios, currency, navexa key from user profile
     Stock index → shared ticker registry with freshness tracking
     Job queue → priority-based background data collection (watcher + processor pool)
```

**vire-server endpoints (`:8501`):**

| Endpoint | Method | Description |
|----------|--------|-------------|
| **System** | | |
| `/api/health` | GET | Health check — `{"status":"ok"}` |
| `/api/version` | GET | Version info |
| `/api/config` | GET | Runtime configuration and resolved settings |
| `/api/diagnostics` | GET | Uptime, recent logs, per-request traces via `?correlation_id=` |
| `/api/mcp/tools` | GET | Tool catalog for dynamic MCP registration |
| `/api/shutdown` | POST | Graceful shutdown (dev mode only, disabled in production) |
| **Users** | | |
| `/api/users` | POST | Create user (bcrypt password, returns username/email/role). 409 if exists. |
| `/api/users/upsert` | POST | Create or update user (merge semantics). Password required for new users. |
| `/api/users/check/{username}` | GET | Check username availability — `{available: true/false}` |
| `/api/users/{id}` | GET | Get user profile (passwords and keys masked) |
| `/api/users/{id}` | PUT | Update user fields (merge semantics, supports password change) |
| `/api/users/{id}` | DELETE | Delete user and associated KV entries |
| **Auth** | | |
| `/api/auth/login` | POST | Verify credentials (bcrypt compare), returns JWT + user profile |
| `/api/auth/password-reset` | POST | Reset a user's password (requires username + new_password) |
| `/api/auth/oauth` | POST | Exchange OAuth provider code for JWT — supports `dev`, `google`, `github` |
| `/api/auth/validate` | POST | Validate JWT from `Authorization: Bearer` header, returns user profile |
| `/api/auth/login/google` | GET | Redirect to Google OAuth consent screen (`?callback=` for portal return URL) |
| `/api/auth/login/github` | GET | Redirect to GitHub OAuth consent screen (`?callback=` for portal return URL) |
| `/api/auth/callback/google` | GET | Google OAuth callback — exchanges code, signs JWT, redirects with `?token=` |
| `/api/auth/callback/github` | GET | GitHub OAuth callback — exchanges code, signs JWT, redirects with `?token=` |
| **Portfolios** | | |
| `/api/portfolios` | GET | List portfolios |
| `/api/portfolios/default` | GET/PUT | Get or set the default portfolio |
| `/api/portfolios/{name}` | GET | Portfolio holdings |
| `/api/portfolios/{name}/stock/{ticker}` | GET | Single holding position data |
| `/api/portfolios/{name}/review` | POST | Portfolio compliance review |
| `/api/portfolios/{name}/sync` | POST | Sync holdings from Navexa |
| `/api/portfolios/{name}/rebuild` | POST | Full rebuild of portfolio data |
| `/api/portfolios/{name}/strategy` | GET/PUT/DELETE | Portfolio strategy (merge semantics on PUT) |
| `/api/portfolios/{name}/plan` | GET/PUT | Portfolio investment plan |
| `/api/portfolios/{name}/plan/items` | POST | Add plan item |
| `/api/portfolios/{name}/plan/items/{id}` | PUT/DELETE | Update or remove plan item |
| `/api/portfolios/{name}/plan/status` | GET | Check plan status (triggers, deadlines) |
| `/api/portfolios/{name}/watchlist` | GET | Portfolio watchlist |
| `/api/portfolios/{name}/watchlist/items` | POST | Add watchlist item |
| `/api/portfolios/{name}/watchlist/items/{ticker}` | PUT/DELETE | Update or remove watchlist item |
| `/api/portfolios/{name}/report` | POST | Generate portfolio report |
| `/api/portfolios/{name}/summary` | GET | Cached portfolio summary |
| `/api/portfolios/{name}/tickers` | GET | List tickers in portfolio |
| `/api/portfolios/{name}/snapshot` | POST | Save portfolio snapshot |
| `/api/portfolios/{name}/history` | GET | Portfolio snapshot history |
| `/api/portfolios/{name}/reports/{ticker}` | GET | Per-ticker report |
| **Market Data** | | |
| `/api/market/quote/{ticker}` | GET | Real-time price quote (OHLCV + change%) |
| `/api/market/stocks/{ticker}` | GET | Stock data with fundamentals, signals, filings, timeline, quality assessment |
| `/api/market/stocks/{ticker}/filing-summaries` | GET | Filing summaries with quality assessment for a ticker |
| `/api/market/signals` | POST | Compute technical indicators |
| `/api/market/collect` | POST | Trigger market data collection for tickers |
| **Screening** | | |
| `/api/screen` | POST | Stock screen by quantitative filters |
| `/api/screen/snipe` | POST | Strategy scanner |
| `/api/screen/funnel` | POST | Multi-stage screening funnel |
| **Jobs** | | |
| `/api/jobs/status` | GET | Legacy job run status (enabled flag + last run info) |
| **Admin** (requires `role: admin`) | | |
| `/api/admin/jobs` | GET | List jobs with optional `?ticker=`, `?status=pending`, `?limit=` filters |
| `/api/admin/jobs/queue` | GET | List pending jobs ordered by priority with count |
| `/api/admin/jobs/enqueue` | POST | Manually enqueue a job (`{job_type, ticker, priority}`) |
| `/api/admin/jobs/{id}/priority` | PUT | Set job priority (number or `"top"` to push to front) |
| `/api/admin/jobs/{id}/cancel` | POST | Cancel a pending or running job |
| `/api/admin/stock-index` | GET | List all tracked stocks with freshness timestamps |
| `/api/admin/stock-index` | POST | Add or upsert a stock to the index (`{ticker, code, exchange, name}`) |
| `/api/admin/ws/jobs` | GET | WebSocket for real-time job queue events |
| **Other** | | |
| `/api/strategies/template` | GET | Strategy field reference with valid values |
| `/api/searches` | GET | List saved searches |
| `/api/searches/{id}` | GET | Get saved search by ID |
| `/api/reports` | GET | List available reports |

### Dynamic Tool Catalog

vire-server exposes `GET /api/mcp/tools` — a machine-readable catalog of all MCP tools with their HTTP mappings. This is the bootstrap mechanism for vire-portal: the portal fetches the catalog on startup, builds MCP tool schemas from it, and registers a generic proxy handler for each tool. When vire-server adds a new tool, it appears in the catalog automatically — no portal code changes needed.

**Catalog schema:**

Each entry describes one MCP tool and how to call it as an HTTP request:

```json
{
  "name": "portfolio_compliance",
  "description": "Review a portfolio for signals, overnight movement, and actionable observations.",
  "method": "POST",
  "path": "/api/portfolios/{portfolio_name}/review",
  "params": [
    {
      "name": "portfolio_name",
      "type": "string",
      "description": "Name of the portfolio to review.",
      "required": false,
      "in": "path",
      "default_from": "user_config.default_portfolio"
    },
    {
      "name": "focus_signals",
      "type": "array",
      "description": "Signal types to focus on: sma, rsi, volume, pbas, vli, regime, trend",
      "required": false,
      "in": "body"
    }
  ]
}
```

**Parameter fields:**

| Field | Description |
|-------|-------------|
| `name` | Parameter name — matches the HTTP body key, path placeholder, or query param |
| `type` | `string`, `number`, `boolean`, `array`, `object` |
| `description` | Human-readable description (used in MCP tool schema) |
| `required` | Whether the parameter must be provided |
| `in` | Where the parameter goes: `path` (URL template), `query` (query string), `body` (JSON body) |
| `default_from` | Optional. Dot-path into user config for default value (e.g., `user_config.default_portfolio`) |

**How the portal uses it:**

1. **Startup:** `GET /api/mcp/tools` → receives array of tool definitions
2. **Register:** For each tool, build an `mcp-go` tool schema from the catalog entry and register it with a generic handler
3. **Handle calls:** When Claude calls a tool, the generic handler:
   - Resolves `path` params by substituting `{param}` placeholders from the request (or from `default_from` user config)
   - Builds a JSON body from `body` params
   - Appends `query` params to the URL
   - Sends the HTTP request to vire-server with `X-Vire-*` headers for user context
   - Returns the response as an MCP tool result
4. **Refresh:** Optionally re-fetch the catalog on a timer or admin trigger to pick up new tools without restart

This design means the portal contains zero tool-specific logic. All tool definitions, parameter schemas, and HTTP routing live in vire-server's catalog.

## Background Processing

Vire uses a stock index and priority job queue to decouple "which stocks to track" (user-driven) from "how to collect data" (system-driven).

### Stock Index

The `stock_index` table is a shared, user-agnostic registry of all tickers the system tracks. It contains no user data — just ticker metadata and per-component freshness timestamps:

```
stock_index:BHP.AU
  ticker: "BHP.AU", code: "BHP", exchange: "AU", name: "BHP Group Limited"
  source: "portfolio"                    # how it was added
  eod_collected_at: 2026-02-21T10:00:00  # per-component timestamps
  fundamentals_collected_at: ...
  filings_collected_at: ...
  news_collected_at: ...
  filing_summaries_collected_at: ...
  timeline_collected_at: ...
  signals_collected_at: ...
```

Stocks are automatically upserted when:
- A user syncs a portfolio (all holdings added with `source: "portfolio"`)
- An admin manually adds a ticker via `POST /api/admin/stock-index`

### Job Queue

The `job_queue` table is a persistent, priority-ordered work queue. Most jobs target a single ticker; bulk jobs target an exchange:

| Job Type | Description | Default Priority |
|----------|-------------|-----------------|
| `collect_eod` | Fetch EOD price bars (incremental, single ticker) | 10 |
| `collect_eod_bulk` | Fetch last-day EOD bars for all tickers on an exchange via bulk API | 10 |
| `compute_signals` | Compute technical indicators | 9 |
| `collect_fundamentals` | Fetch fundamental data | 8 |
| `collect_news` | Fetch news articles | 7 |
| `collect_filings` | Fetch + download filing PDFs | 5 |
| `collect_news_intel` | AI news intelligence summary | 4 |
| `collect_filing_summaries` | AI filing extraction | 3 |
| `collect_timeline` | Generate company timeline | 2 |

Higher priority = processed first. New stocks get elevated priority (15). Jobs can be pushed to the top of the queue via the admin API.

### Pipeline

```
Portfolio Sync ──► Stock Index ──► Watcher ──► Job Queue ──► Processors
  (user action)     (upsert)      (1m scan)   (priority)   (concurrent)
```

1. **User syncs portfolio** → tickers upserted to stock index with zero collection timestamps
2. **Watcher** (runs every `watcher_interval`, default 1m) scans the stock index, checks each ticker's freshness timestamps against TTLs, and enqueues jobs for stale components (with deduplication). EOD jobs are grouped per-exchange: tickers with stale EOD data are batched into one `collect_eod_bulk` job per exchange rather than individual `collect_eod` jobs per ticker
3. **Processor pool** (`max_concurrent` workers, default 5) dequeues jobs by priority and executes them via the corresponding MarketService method
4. On completion, the stock index timestamps are updated. Failed jobs are retried up to `max_retries` times

### Configuration

```toml
[jobmanager]
enabled = true
watcher_interval = "1m"    # how often to scan the stock index
max_concurrent = 5         # concurrent job processors
max_retries = 3            # retry attempts per failed job
purge_after = "24h"        # remove completed jobs older than this
```

### Admin Monitoring

The admin API (`/api/admin/jobs/*`) provides queue inspection, priority management, and manual job enqueue. A WebSocket endpoint (`/api/admin/ws/jobs`) streams real-time job events (`job_queued`, `job_started`, `job_completed`, `job_failed`) for live monitoring.

## Prerequisites

- **Go 1.21+** — for local development (`./scripts/run.sh`)
- **SurrealDB v3.0+** — required for storage ([surrealdb.com](https://surrealdb.com)); can run via Docker or native binary
- **Docker** — for running SurrealDB and optional container deployments
- API keys for:
  - **EODHD** — stock prices and fundamentals ([eodhd.com](https://eodhd.com))
  - **Google Gemini** — AI analysis ([aistudio.google.com](https://aistudio.google.com)) *(optional, enables filings + news intelligence)*
  - **Navexa** — portfolio sync ([navexa.com.au](https://navexa.com.au)) *(per-user, injected by vire-portal via `X-Vire-Navexa-Key` header)*

## Deployment

### Quick Start (Local)

```bash
# 1. Start SurrealDB (if not already running)
docker run -d --name surrealdb -p 8000:8000 surrealdb/surrealdb:v3.0.0 start --user root --pass root

# 2. Copy and edit the config file with your API keys
cp config/vire-service.toml.example config/vire-service.toml
# Edit config/vire-service.toml — add your EODHD and Gemini API keys
# Note: Navexa API key is NOT stored in config — it is injected per-user via vire-portal

# 3. Build and run
./scripts/run.sh start
```

### Run Script (Local Dev)

| Command | Description |
|---------|-------------|
| `./scripts/run.sh start` | Build and run vire-server as a background process |
| `./scripts/run.sh stop` | Graceful shutdown via HTTP endpoint, fallback to SIGTERM |
| `./scripts/run.sh restart` | Stop and start |
| `./scripts/run.sh status` | Show PID and version info |
| `sudo ./scripts/service-uninstall.sh` | Remove legacy systemd services (vire-server + vire-portal) |

### Verify

```bash
curl http://localhost:8501/api/health    # {"status":"ok"}
curl http://localhost:8501/api/version   # {"version":"0.3.0",...}
./scripts/run.sh status                  # Local: PID and version
docker logs vire-server                  # Docker: container logs
```

### MCP Client Setup

MCP is served by [vire-portal](https://github.com/bobmcallan/vire-portal) which proxies tool calls to vire-server.

#### Claude Code (Direct)

With the portal running, add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "vire": {
      "type": "http",
      "url": "http://localhost:8080/mcp"
    }
  }
}
```

#### Claude Code (Docker Compose)

Start vire-server and SurrealDB with Docker Compose:

```bash
cd docker
docker compose up -d
```

Then add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "vire": {
      "type": "http",
      "url": "http://localhost:8881/mcp"
    }
  }
}
```

#### Claude Desktop (Windows/macOS)

Claude Desktop requires stdio transport. Use `npx mcp-remote` to bridge the HTTP endpoint. Add to your Claude Desktop config (`%APPDATA%\Claude\claude_desktop_config.json` on Windows, `~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "vire": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "http://localhost:8080/mcp"]
    }
  }
}
```

This uses [mcp-remote](https://www.npmjs.com/package/mcp-remote) to connect Claude Desktop's stdio transport to the portal's HTTP endpoint. Requires Node.js installed locally.

## Configuration

### Config Files

| File | Contains | Consumed by |
|------|----------|-------------|
| `config/vire-service.toml` | Server settings, SurrealDB connection, EODHD/Gemini keys, fallback defaults | `vire-server` |

### SurrealDB Configuration

The `[storage]` section in `vire-service.toml` configures the SurrealDB connection:

```toml
[storage]
address   = "ws://localhost:8000/rpc"
namespace = "vire"
database  = "vire"
username  = "root"
password  = "root"
data_path = "data/market"   # local path for generated files (charts)
```

| Field | Description | Default |
|-------|-------------|---------|
| `address` | SurrealDB WebSocket RPC endpoint | `ws://localhost:8000/rpc` |
| `namespace` | SurrealDB namespace | `vire` |
| `database` | SurrealDB database name | `vire` |
| `username` | SurrealDB auth username | `root` |
| `password` | SurrealDB auth password | `root` |
| `data_path` | Local directory for generated files (charts, raw data) | `data/market` |

Environment variable overrides: `VIRE_DATA_PATH` overrides `data_path`. SurrealDB must be running and reachable at the configured address before starting vire-server.

### API Keys

**Server-side keys** (EODHD, Gemini) can be provided two ways:

**Option 1: Config files** (recommended)

Copy `config/vire-service.toml.example` to `config/vire-service.toml` (local) or `config/vire-service.toml.docker` (Docker) and add your EODHD and Gemini keys. These files are gitignored so keys never enter the repo.

**Option 2: Environment variables**

Set `EODHD_API_KEY` and `GEMINI_API_KEY` in the server environment. Env vars take priority over config file values.

**Per-user context** is resolved from the user profile stored in vire-server. The portal sends only `X-Vire-User-ID`; the middleware resolves all preferences from the user profile (portfolios, display currency, navexa key). Individual headers are available for direct API use and override profile values:

| Header | Purpose |
|--------|---------|
| `X-Vire-User-ID` | User identifier — resolves all preferences from the stored user profile |
| `X-Vire-Portfolios` | Override portfolios (comma-separated) |
| `X-Vire-Display-Currency` | Override display currency (AUD/USD) |
| `X-Vire-Navexa-Key` | Override Navexa API key |

User preferences (`display_currency`, `portfolios`, `navexa_key`) are stored as per-user KV entries in the InternalStore and set via `PUT /api/users/{id}`. They are resolved automatically when `X-Vire-User-ID` is present. At least one source must resolve a navexa key for Navexa-dependent endpoints (`/api/portfolios/{name}/sync`, `/api/portfolios/{name}/rebuild`). The `[clients.navexa]` config section provides only `base_url`, `rate_limit`, and `timeout` -- no `api_key`.

## Portfolio Strategy

Vire lets you define an investment strategy per portfolio — covering risk appetite, target returns, position sizing, sector preferences, and more. The strategy is a living document stored alongside your portfolio data with automatic versioning. Once set, it influences all analysis:

| Analysis | Strategy Effect |
|----------|----------------|
| Portfolio compliance | RSI thresholds adjusted by risk level (conservative: 65/35, moderate: 70/30, aggressive: 80/25). Position size alerts when holdings exceed strategy limits. |
| Strategy scanner | Excluded sectors filtered out. Conservative strategies penalise high-volatility candidates. |
| Stock screen | Default P/E cap adjusted by risk level (conservative: 15, moderate: 20, aggressive: 25). Conservative strategies boost dividend payers. |
| AI summaries | Risk level, target return, and account type included as context in Gemini prompts. |

The strategy also includes a devil's advocate: when you save a strategy, Vire checks for unrealistic goals (e.g. 25% annual returns with conservative risk) and internal contradictions (e.g. a sector in both preferred and excluded lists). Warnings are returned but never block the save.

### How it works in Claude Code vs Claude Desktop

The strategy system is built entirely on MCP tools, so it works identically in both Claude Code (CLI) and Claude Desktop — no skills, CLAUDE.md files, or CLI-specific features are required.

**Claude Desktop** — Use the 4 strategy MCP tools directly in conversation:

1. Ask Claude to call `get_strategy_template` to see available fields and valid values
2. Discuss your investment goals — Claude builds the JSON from the conversation
3. Claude calls `set_portfolio_strategy` with the structured fields
4. Review the devil's advocate warnings and adjust if needed
5. Call `get_portfolio_strategy` at any time to view your strategy as markdown

**Claude Code** — Same MCP tools are available, plus an optional `/strategy` skill that provides guided workflows:

- `/strategy SMSF` — View the strategy for a portfolio
- `/strategy SMSF build` — Interactive strategy-building conversation
- `/strategy SMSF update` — Update specific fields
- `/strategy template` — Show the field reference

In both clients, the strategy uses merge semantics for updates — only include the fields you want to change. Unspecified fields keep their current values. When updating nested objects (e.g. `risk_appetite`), include all sub-fields you want to keep, as nested objects are replaced atomically.

The strategy is stored per portfolio as a `UserRecord` in the UserDataStore (subject: `strategy`) with automatic version incrementing.

## Storage

Vire uses SurrealDB for all persistent storage, with a file-based layer for generated assets:

| Area | Backend | Contents |
|------|---------|----------|
| **InternalStore** | SurrealDB | User accounts (`InternalUser`), per-user config KV (`UserKeyValue`), system KV (schema version) |
| **UserDataStore** | SurrealDB | All user domain data via generic `UserRecord` (portfolios, strategies, plans, watchlists, reports, searches) |
| **MarketDataStorage** | SurrealDB | Market data (EOD prices, fundamentals) |
| **SignalStorage** | SurrealDB | Technical signals per ticker |
| **StockIndexStore** | SurrealDB | Shared stock index — all tracked tickers with per-component freshness timestamps |
| **JobQueueStore** | SurrealDB | Persistent priority job queue for background data collection |
| **Generated files** | Local filesystem | Charts and raw data files (`data_path` in config) |

### InternalStore

Stores user accounts (`InternalUser`: user_id, email, password_hash, role, created_at, modified_at) and per-user key-value config (`UserKeyValue`: user_id, key, value, version, datetime). User preferences like `display_currency`, `portfolios`, and `navexa_key` are stored as KV entries, not as fields on the user model. The `InternalStore` interface provides `GetUser`, `SaveUser`, `DeleteUser`, `ListUsers`, `GetUserKV`, `SetUserKV`, `DeleteUserKV`, `ListUserKV`, `GetSystemKV`, `SetSystemKV`.

### UserDataStore

All user domain data uses a single generic record type: `UserRecord` (user_id, subject, key, value, version, datetime). Subjects include `portfolio`, `strategy`, `plan`, `watchlist`, `report`, `search`. Services marshal/unmarshal domain types to/from the `value` field as JSON. The `UserDataStore` interface provides `Get`, `Put`, `Delete`, `List`, `Query`, `DeleteBySubject`.

### MarketDataStorage / SignalStorage

Market data (EOD prices, fundamentals) and technical signals are stored in SurrealDB tables. Implements `MarketDataStorage` and `SignalStorage` interfaces.

### Running SurrealDB

**Docker (recommended):**

```bash
docker run -d --name surrealdb -p 8000:8000 surrealdb/surrealdb:v3.0.0 start --user root --pass root
```

**Verify SurrealDB is running:**

```bash
curl -s http://localhost:8000/health    # Should return OK
```

SurrealDB data persists inside the container by default. For durable storage, mount a volume:

```bash
docker run -d --name surrealdb -p 8000:8000 \
  -v surrealdb-data:/data \
  surrealdb/surrealdb:v3.0.0 start --user root --pass root file:/data/vire.db
```

### Test Infrastructure

The test suite uses a Docker Compose stack (`tests/docker/docker-compose.yml`) that starts SurrealDB and vire-server together:

```bash
cd tests/docker
docker compose up --build
```

This starts SurrealDB with a health check and waits for it to be healthy before starting vire-server.

## Development

```bash
# Build and run locally (builds binary, starts in background)
./scripts/run.sh start

# Stop / restart / check status
./scripts/run.sh stop
./scripts/run.sh restart
./scripts/run.sh status

# Build binary only (output: bin/vire-server)
./scripts/build.sh

# Run unit tests
go test ./internal/...

# Run integration tests (requires Docker)
go test ./tests/api/... -v -timeout 300s

# Run portfolio workflow test (requires Navexa API key)
# Copy tests/docker/.env.example to tests/docker/.env and fill in values
go test ./tests/api/... -run TestPortfolioWorkflow -v -timeout 300s
```

## Releasing

Push a version tag to trigger the GitHub Actions workflow:

```bash
git tag v0.3.0
git push origin v0.3.0
```

This builds and pushes `ghcr.io/bobmcallan/vire-server` with the version tag and `:latest` to GHCR. Pull images with `docker pull ghcr.io/bobmcallan/vire-server:latest`.

You can also trigger a build manually from the Actions tab using "Run workflow".

## License

Private repository.
