# Docker Services

## Services

| Service | Description | External Port | Internal Port | Image |
|---------|-------------|---------------|---------------|-------|
| vire-server | Main Vire backend API | 8881 | 8080 | `vire-server:latest` |
| surrealdb | SurrealDB database | 8000 | 8000 | `surrealdb/surrealdb:v3.0.0` |

## Usage

```bash
# Start all services
docker compose up -d

# View logs
docker compose logs -f vire-server

# Stop all services
docker compose down
```

## Configuration Priority

Command-line flags (highest priority) > Config file (TOML) > Defaults

## TOML Configuration

### `config/vire-service.toml` — Server config

Server-level settings: storage paths, EODHD/Gemini API keys, logging. Also contains fallback defaults for portfolios and display_currency when running vire-server standalone (without MCP headers).

## GHCR Images

The CI workflow publishes the server image to GHCR:

- `ghcr.io/bobmcallan/vire-server`
