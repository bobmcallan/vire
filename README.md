# Vire

AI-powered portfolio analysis and market intelligence MCP server for Australian equities.

Vire connects to Claude (via [MCP](https://modelcontextprotocol.io/)) to provide real-time portfolio reviews, stock analysis, technical signals, and company filings intelligence. It aggregates data from EODHD, Navexa, ASX announcements, and uses Google Gemini for AI-powered summaries.

## Features

- **Portfolio Review** — Sync holdings from Navexa, analyse positions with buy/sell/hold recommendations
- **Portfolio Strategy** — Define and store investment strategies per portfolio with devil's advocate validation
- **Real-Time Quotes** — Live price quotes for stocks, forex pairs, and commodities via EODHD
- **Stock Analysis** — Real-time price, fundamentals, technical signals, and AI-generated filings intelligence for any ASX/US ticker
- **Technical Signals** — SMA, RSI, MACD, volume, regime detection, relative strength, support/resistance
- **Company Filings Intelligence** — ASX announcement scraping, PDF extraction, and Gemini-powered financial analysis
- **News Intelligence** — AI-summarised news sentiment per ticker
- **Market Snipe** — Scan for turnaround opportunities matching technical criteria
- **Stock Screening** — Quality-value stock screening with consistent returns and credible news support
- **Report Generation** — Cached per-ticker and portfolio summary reports

## MCP Tools

### Market Data

| Tool | Description |
|------|-------------|
| `get_quote` | Real-time price quote for any ticker — stocks (BHP.AU), forex (AUDUSD.FOREX), commodities (XAUUSD.FOREX). Returns OHLCV, change%, and previous close. |
| `get_stock_data` | Real-time price, fundamentals, signals, filings, and news for a ticker |
| `detect_signals` | Compute technical signals for tickers |
| `collect_market_data` | Pre-fetch and cache market data |
| `market_snipe` | Scan for turnaround buy opportunities |
| `stock_screen` | Screen for quality-value stocks with low P/E and consistent returns |

### Portfolio

| Tool | Description |
|------|-------------|
| `portfolio_review` | Full portfolio analysis with real-time prices and buy/sell/hold recommendations |
| `sync_portfolio` | Sync holdings from Navexa |
| `list_portfolios` | List available portfolios |
| `set_default_portfolio` | Set or view the default portfolio |
| `get_portfolio_snapshot` | Reconstruct portfolio state as of a historical date |
| `get_portfolio_history` | Daily portfolio value history for a date range |

### Reports

| Tool | Description |
|------|-------------|
| `generate_report` | Generate full portfolio report (slow — refreshes all data) |
| `generate_ticker_report` | Regenerate report for a single ticker |
| `get_summary` | Get cached portfolio summary |
| `get_ticker_report` | Get cached per-ticker report |
| `list_reports` | List available reports with timestamps |
| `list_tickers` | List tickers in a portfolio report |

### Strategy

| Tool | Description |
|------|-------------|
| `get_strategy_template` | Field reference with valid values, guidance tables, and examples |
| `set_portfolio_strategy` | Create or update a portfolio strategy (merge semantics) |
| `get_portfolio_strategy` | View the strategy document as formatted markdown |
| `delete_portfolio_strategy` | Delete a portfolio strategy |

### System

| Tool | Description |
|------|-------------|
| `get_version` | Server version and status |
| `get_config` | List all configuration settings |
| `get_diagnostics` | Server diagnostics: uptime, recent logs, per-request traces via correlation ID |

## Architecture

Vire uses a two-binary architecture:

| Binary | Port | Role | Location |
|--------|------|------|----------|
| `vire-server` | `:4242` | REST API server — services, storage, warm cache, scheduler | `cmd/vire-server/` |
| `vire-mcp` | `:4243` | MCP-to-REST translator — Streamable HTTP (default) or `--stdio` | `cmd/vire-mcp/` |

The server runs continuously and exposes a pure REST API (`/api/*`). The MCP binary is a thin translator that receives MCP tool calls, forwards them as REST requests to vire-server, and formats JSON responses as markdown for LLM consumption.

**vire-server endpoints (`:4242`):**

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/health` | GET | Health check — `{"status":"ok"}` |
| `/api/version` | GET | Version info |
| `/api/portfolios` | GET | List portfolios |
| `/api/portfolios/{name}/review` | POST | Portfolio review |
| `/api/market/quote/{ticker}` | GET | Real-time price quote (OHLCV + change%) |
| `/api/market/stocks/{ticker}` | GET | Stock data |
| `/api/*` | various | 40+ REST endpoints |

**vire-mcp endpoints (`:4243`):**

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/mcp` | POST | MCP over Streamable HTTP (JSON-RPC) |

## Prerequisites

- Docker
- API keys for:
  - **EODHD** — stock prices and fundamentals ([eodhd.com](https://eodhd.com))
  - **Navexa** — portfolio sync ([navexa.com.au](https://navexa.com.au)) *(optional)*
  - **Google Gemini** — AI analysis ([aistudio.google.com](https://aistudio.google.com)) *(optional, enables filings + news intelligence)*

## Deployment

### Quick Start (Local Build)

```bash
# 1. Copy and edit the config file with your API keys
cp config/vire.toml.example docker/vire.toml
# Edit docker/vire.toml — add your EODHD, Navexa, and Gemini API keys

# 2. Deploy
./scripts/deploy.sh local
```

### Quick Start (GHCR — recommended)

Pull pre-built images from GitHub Container Registry with automatic updates via Watchtower:

```bash
# 1. Copy and edit the config file with your API keys
cp config/vire.toml.example docker/vire.toml
# Edit docker/vire.toml — add your EODHD, Navexa, and Gemini API keys

# 2. Deploy from GHCR
./scripts/deploy.sh ghcr
```

This uses `docker/docker-compose.ghcr.yml` which pulls separate images per service and includes a Watchtower sidecar that polls for new images every 2 minutes. When you push a new tag, containers auto-update.

```yaml
services:
  vire-server:    # REST API on :4242
    image: ghcr.io/bobmcallan/vire-server:latest

  vire-mcp:       # MCP Streamable HTTP on :4243
    image: ghcr.io/bobmcallan/vire-mcp:latest
    environment:
      - VIRE_SERVER_URL=http://vire-server:4242

  watchtower:     # Auto-update on new GHCR pushes
    image: containrrr/watchtower
```

### Deploy Script Modes

| Mode | Description |
|------|-------------|
| `local` | Build from per-service Dockerfiles and deploy |
| `ghcr` (recommended) | Deploy from `ghcr.io/bobmcallan/vire-server:latest` and `ghcr.io/bobmcallan/vire-mcp:latest` with Watchtower auto-update |
| `down` | Stop all vire containers |
| `prune` | Remove stopped containers, dangling images, and unused volumes |

### Verify

```bash
curl http://localhost:4242/api/health    # {"status":"ok"}
curl http://localhost:4242/api/version   # {"version":"0.3.0",...}
docker logs vire-server                  # REST API server logs
docker logs vire-mcp                     # MCP translator logs
```

### Claude Code

Claude Code connects to the MCP HTTP service directly. With the containers running, add via the CLI:

```bash
claude mcp add --transport http --url http://localhost:4243/mcp vire
```

Or add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "vire": {
      "type": "http",
      "url": "http://localhost:4243/mcp"
    }
  }
}
```

### Claude Desktop

Claude Desktop uses stdio transport. Each session spins up an ephemeral container that connects to the running `vire-server` via the Docker network.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows):

```json
{
  "mcpServers": {
    "vire": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "--network", "vire_default",
        "-e", "VIRE_SERVER_URL=http://vire-server:4242",
        "vire-mcp:latest",
        "--stdio"
      ]
    }
  }
}
```

Each Desktop session creates an isolated container (`--rm` auto-cleans on exit). The `--network vire_default` flag joins the compose network so the stdio proxy can reach `vire-server`. The `VIRE_SERVER_URL` env var configures the REST API URL. No `--entrypoint` override is needed since the `vire-mcp` image defaults to `./vire-mcp`.

**How the transports differ:**

| Client | Transport | Connection |
|--------|-----------|------------|
| Claude Code | Streamable HTTP | Direct to `vire-mcp` container on `:4243` |
| Claude Desktop | stdio | Ephemeral `docker run` container per session, proxies to `vire-server` on `:4242` |

## Configuration

API keys can be provided two ways:

**Option 1: Config file** (recommended)

Copy `config/vire.toml.example` to `docker/vire.toml` and add your keys. The `deploy` script mounts this into the container at runtime. The file is gitignored so keys never enter the repo.

**Option 2: Environment variables**

Set `EODHD_API_KEY`, `NAVEXA_API_KEY`, and `GEMINI_API_KEY` in your environment. These take priority over config file values.

## Portfolio Strategy

Vire lets you define an investment strategy per portfolio — covering risk appetite, target returns, position sizing, sector preferences, and more. The strategy is a living document stored alongside your portfolio data with automatic versioning. Once set, it influences all analysis:

| Analysis | Strategy Effect |
|----------|----------------|
| Portfolio review | RSI thresholds adjusted by risk level (conservative: 65/35, moderate: 70/30, aggressive: 80/25). Position size alerts when holdings exceed strategy limits. |
| Market snipe | Excluded sectors filtered out. Conservative strategies penalise high-volatility candidates. |
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

The strategy is stored per portfolio as versioned JSON files with automatic backup retention.

## Storage

Vire uses file-based JSON storage. All data is stored as human-readable JSON files under the `data/` directory, organised by type:

```
data/
├── portfolios/    # Portfolio holdings (synced from Navexa)
├── market/        # EOD prices, fundamentals, news, filings per ticker
├── signals/       # Computed technical signals per ticker
├── reports/       # Cached portfolio and ticker reports
├── strategies/    # Investment strategy documents per portfolio
├── plans/         # Action plans per portfolio
├── watchlists/    # Stock watchlists per portfolio
├── searches/      # Screen/snipe/funnel search history
└── kv/            # Key-value settings (default portfolio, etc.)
```

### Versioning

Each write creates a backup of the previous version. Configure the number of retained versions in `vire.toml`:

```toml
[storage.file]
path = "data"
versions = 5       # keep 5 previous versions per file (0 to disable)
```

Version files are stored alongside the primary file with `.v1` (most recent backup) through `.v{N}` suffixes. Writes are atomic (temp file + rename) to prevent corruption.

### Data Portability

All data is plain JSON -- you can inspect, back up, or edit files directly. The directory structure is compatible with cloud storage mounts (e.g. GCS FUSE) since there are no exclusive locks or binary formats.

## Development

```bash
# Build both binaries
go build ./cmd/vire-server/
go build ./cmd/vire-mcp/

# Run HTTP server locally
EODHD_API_KEY=xxx ./vire-server

# Run stdio proxy (connects to running server)
VIRE_SERVER_URL=http://localhost:4242 ./vire-mcp

# Deploy local build
./scripts/deploy.sh local

# Deploy ghcr with auto-update
./scripts/deploy.sh ghcr

# Run tests
go test ./...
```

## Releasing

Push a version tag to trigger the GitHub Actions workflow:

```bash
git tag v0.3.0
git push origin v0.3.0
```

This builds and pushes both `ghcr.io/bobmcallan/vire-server` and `ghcr.io/bobmcallan/vire-mcp` with the version tag and `:latest` to GHCR.

You can also trigger a build manually from the Actions tab using "Run workflow".

## License

Private repository.
