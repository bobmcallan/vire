# Vire

AI-powered portfolio analysis and market intelligence MCP server for Australian equities.

Vire connects to Claude (via [MCP](https://modelcontextprotocol.io/)) to provide real-time portfolio reviews, stock analysis, technical signals, and company filings intelligence. It aggregates data from EODHD, Navexa, ASX announcements, and uses Google Gemini for AI-powered summaries.

## Features

- **Portfolio Review** — Sync holdings from Navexa, analyse positions with buy/sell/hold recommendations
- **Stock Analysis** — Price, fundamentals, technical signals, and AI-generated filings intelligence for any ASX/US ticker
- **Technical Signals** — SMA, RSI, MACD, volume, regime detection, relative strength, support/resistance
- **Company Filings Intelligence** — ASX announcement scraping, PDF extraction, and Gemini-powered financial analysis
- **News Intelligence** — AI-summarised news sentiment per ticker
- **Market Snipe** — Scan for turnaround opportunities matching technical criteria
- **Report Generation** — Cached per-ticker and portfolio summary reports

## MCP Tools

| Tool | Description |
|------|-------------|
| `get_version` | Server version and status |
| `get_stock_data` | Price, fundamentals, signals, filings, and news for a ticker |
| `detect_signals` | Compute technical signals for tickers |
| `collect_market_data` | Pre-fetch and cache market data |
| `portfolio_review` | Full portfolio analysis with recommendations |
| `sync_portfolio` | Sync holdings from Navexa |
| `list_portfolios` | List available portfolios |
| `market_snipe` | Scan for buy opportunities |
| `generate_report` | Generate full portfolio report |
| `generate_ticker_report` | Regenerate report for a single ticker |
| `get_summary` | Get portfolio summary markdown |
| `get_ticker_report` | Get per-ticker report markdown |
| `list_reports` | List available reports |
| `list_tickers` | List tickers in a portfolio report |

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
        "-e", "EODHD_API_KEY",
        "-e", "NAVEXA_API_KEY",
        "-e", "GEMINI_API_KEY",
        "ghcr.io/bobmcallan/vire-mcp:latest",
        "--stdio"
      ]
    }
  }
}
```

The `-e VAR` syntax forwards environment variables from your host. Set them in your shell profile or use a tool like `direnv`.

For local development builds, see `docker/claude_desktop_config.local.json`.

### Data Persistence

Each mode uses a separate Docker named volume because BadgerDB only supports single-process access:

| Mode | Volume | Container | Image |
|------|--------|-----------|-------|
| Claude Code (dev) | `vire-data` | `vire-mcp` (long-running via compose) | Local build |
| Claude Desktop (prod) | `vire-desktop-data` | Transient (managed by Desktop) | GHCR release |

Both volumes store BadgerDB cache and downloaded filings. Data survives container restarts and image upgrades. Both modes can run simultaneously.

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
