# Docker Services

## Services

| Service | Description | Port |
|---------|-------------|------|
| vire-server | Main Vire backend API | 4242 |
| vire-mcp | MCP server (Streamable HTTP) | 4243 |

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

Create `docker/vire-mcp.toml` (only MCP-specific settings - no analysis/storage):

```toml
# Vire MCP Server Configuration
# MCP-specific settings only - no analysis, storage, or other config

[server]
name = "Vire-MCP"
port = "4243"

# Backend API connection
server_url = "http://vire-server:4242"
```

## ChatGPT Desktop Configuration

```json
{
  "type": "http",
  "url": "http://localhost:4243/mcp"
}
```

## Notes

- **MCP service** is a pass-through proxy to Vire backend API
- Configuration is done via `docker/vire-mcp.toml`
