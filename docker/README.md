# Docker Services

## Services

| Service | Description | Port | Image |
|---------|-------------|------|-------|
| vire-server | Main Vire backend API | 4242 | `vire-server:latest` |
| vire-mcp | MCP server (Streamable HTTP) | 4243 | `vire-mcp:latest` |

Each service has its own Dockerfile (`Dockerfile.server`, `Dockerfile.mcp`) and builds only its own binary.

## Usage

```bash
# Start all services
./scripts/deploy.sh

# View logs
docker logs -f vire-server
docker logs -f vire-mcp

# Stop all services
./scripts/deploy.sh down
```

## Configuration Priority

Command-line flags (highest priority) > Config file (TOML) > Defaults

## MCP Server Modes

### HTTP Mode (default)

```bash
./scripts/deploy.sh
```

Access: `http://localhost:4243/mcp`

### Stdio Mode (for Claude Desktop)

```bash
docker-compose run --rm vire-mcp -stdio
```

## TOML Configuration

### `docker/vire.toml` — Server config

Server-level settings: storage paths, EODHD/Gemini API keys, logging. Also contains fallback defaults for portfolios and display_currency when running vire-server standalone (without MCP headers).

### `docker/vire-mcp.toml` — MCP + user config

MCP connection settings and per-user context. The MCP proxy reads `[user]` and `[navexa]` sections and injects them as `X-Vire-*` headers on every request to vire-server.

```toml
[server]
name = "Vire-MCP"
port = "4243"
server_url = "http://vire-server:4242"

[user]
portfolios = ["SMSF"]
display_currency = "AUD"

[navexa]
api_key = "your-navexa-api-key"
```

| TOML Field | Header | Description |
|------------|--------|-------------|
| `user.portfolios` | `X-Vire-Portfolios` | Comma-separated portfolio names |
| `user.display_currency` | `X-Vire-Display-Currency` | `AUD` or `USD` |
| `navexa.api_key` | `X-Vire-Navexa-Key` | Navexa API key |

Environment variable overrides: `VIRE_DEFAULT_PORTFOLIO`, `VIRE_DISPLAY_CURRENCY`, `NAVEXA_API_KEY`.

## Claude Desktop Configuration

Add to `%APPDATA%\Claude\claude_desktop_config.json` (Windows) or `~/Library/Application Support/Claude/claude_desktop_config.json` (macOS):

```json
{
  "mcpServers": {
    "vire": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "--network", "vire_default",
        "-e", "VIRE_SERVER_URL=http://vire-server:4242",
        "-e", "VIRE_DEFAULT_PORTFOLIO=SMSF",
        "-e", "VIRE_DISPLAY_CURRENCY=AUD",
        "-e", "NAVEXA_API_KEY=your-navexa-api-key",
        "vire-mcp:latest",
        "--stdio"
      ]
    }
  }
}
```

User context is passed as `-e` env vars, which override any values in the image's baked-in `vire-mcp.toml`.

## ChatGPT Desktop Configuration

```json
{
  "type": "http",
  "url": "http://localhost:4243/mcp"
}
```

## GHCR Images

The CI workflow publishes separate images to GHCR:

- `ghcr.io/bobmcallan/vire-server`
- `ghcr.io/bobmcallan/vire-mcp`

Deploy from GHCR with:

```bash
./scripts/deploy.sh ghcr
```

## Notes

- **MCP service** is a pass-through proxy to Vire backend API
- Configuration is done via `docker/vire-mcp.toml`
