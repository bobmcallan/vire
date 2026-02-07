# Vire

AI-powered portfolio analysis and market intelligence MCP server for Australian equities.

Vire connects to Claude (via [MCP](https://modelcontextprotocol.io/)) to provide real-time portfolio reviews, stock analysis, technical signals, and company filings intelligence. It aggregates data from EODHD, Navexa, ASX announcements, and uses Google Gemini for AI-powered summaries.

## Features

- **Portfolio Review** — Sync holdings from Navexa, analyse positions with buy/sell/hold recommendations
- **Portfolio Strategy** — Define and store investment strategies per portfolio with devil's advocate validation
- **Stock Analysis** — Price, fundamentals, technical signals, and AI-generated filings intelligence for any ASX/US ticker
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
| `get_stock_data` | Price, fundamentals, signals, filings, and news for a ticker |
| `detect_signals` | Compute technical signals for tickers |
| `collect_market_data` | Pre-fetch and cache market data |
| `market_snipe` | Scan for turnaround buy opportunities |
| `stock_screen` | Screen for quality-value stocks with low P/E and consistent returns |

### Portfolio

| Tool | Description |
|------|-------------|
| `portfolio_review` | Full portfolio analysis with buy/sell/hold recommendations |
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

## Prerequisites

- Docker
- API keys for:
  - **EODHD** — stock prices and fundamentals ([eodhd.com](https://eodhd.com))
  - **Navexa** — portfolio sync ([navexa.com.au](https://navexa.com.au)) *(optional)*
  - **Google Gemini** — AI analysis ([aistudio.google.com](https://aistudio.google.com)) *(optional, enables filings + news intelligence)*

## Deployment

### Quick Start

```bash
# 1. Copy and edit the config file with your API keys
cp config/vire.toml.example docker/vire.toml
# Edit docker/vire.toml — add your EODHD, Navexa, and Gemini API keys

# 2. Build and start
docker compose -f docker/docker-compose.yml build
docker compose -f docker/docker-compose.yml up -d
```

### Claude Code (SSE transport)

Claude Code connects over HTTP SSE. With the server running, add to your project's `.mcp.json`:

```json
{
  "mcpServers": {
    "vire": {
      "type": "sse",
      "url": "http://localhost:4242/sse"
    }
  }
}
```

### Claude Desktop (stdio transport)

Claude Desktop uses stdio transport with the GHCR release image. This is the production deployment.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS, `%APPDATA%\Claude\claude_desktop_config.json` on Windows). See `docker/claude_desktop_config.ghcr.json`:

```json
{
  "mcpServers": {
    "vire": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "vire-desktop-data:/app/data",
        "-e", "EODHD_API_KEY=your_eodhd_api_key (from eodhd.com)",
        "-e", "NAVEXA_API_KEY=your_navexa_api_key (from navexa.com.au)",
        "-e", "GEMINI_API_KEY=your_gemini_api_key (from aistudio.google.com)",
        "ghcr.io/bobmcallan/vire-mcp:latest",
        "--stdio"
      ]
    }
  }
}
```

Each `-e` flag must use the `VARNAME=value` format to pass the key inline. Alternatively, you can use `-e VARNAME` (without a value) to forward an environment variable already set on your host — set them in your shell profile or use a tool like `direnv`.

For local development builds, see `docker/claude_desktop_config.local.json`.

### Data Persistence

Each mode uses a separate Docker named volume because BadgerDB only supports single-process access:

| Mode | Volume | Container | Image |
|------|--------|-----------|-------|
| Claude Code (dev) | `vire-data` | `vire-mcp` (long-running via compose) | Local build |
| Claude Desktop (prod) | `vire-desktop-data` | Transient (managed by Desktop) | GHCR release |

Both volumes store BadgerDB cache and downloaded filings. Data survives container restarts and image upgrades. Both modes can run simultaneously.

## Portfolio Strategy

Vire lets you define an investment strategy per portfolio — covering risk appetite, target returns, position sizing, sector preferences, and more. The strategy is a living document stored in BadgerDB alongside your portfolio data. Once set, it influences all analysis:

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

The strategy is stored per portfolio in BadgerDB with automatic versioning. Both Claude Code and Claude Desktop share the same data volume, so a strategy set in one client is immediately available in the other.

## Configuration

API keys can be provided two ways:

**Option 1: Config file** (default for local builds)

Copy `config/vire.toml.example` to `docker/vire.toml` and add your keys. The Dockerfile copies this into the image at build time. The file is gitignored so keys never enter the repo.

**Option 2: Environment variables**

Set `EODHD_API_KEY`, `NAVEXA_API_KEY`, and `GEMINI_API_KEY` in your environment. These take priority over config file values. Required for the GHCR image (which ships without keys).

The `docker-compose.yml` passes all three env vars through to the container automatically.

## Development

```bash
# Build locally
go build ./cmd/vire-mcp/

# Run locally (SSE mode)
EODHD_API_KEY=xxx ./vire-mcp

# Run locally (stdio mode)
echo '{"jsonrpc":"2.0","id":1,"method":"initialize",...}' | ./vire-mcp --stdio

# Rebuild Docker image
docker compose -f docker/docker-compose.yml build

# Run tests
go test ./...
```

## Releasing

Push a version tag to trigger the GitHub Actions workflow:

```bash
git tag v0.3.0
git push origin v0.3.0
```

This builds and pushes `ghcr.io/bobmcallan/vire-mcp:v0.3.0` and `:latest` to GHCR.

You can also trigger a build manually from the Actions tab using "Run workflow".

## License

Private repository.
