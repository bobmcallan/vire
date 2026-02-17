# Docker Services

## Services

| Service | Description | External Port | Internal Port | Image |
|---------|-------------|---------------|---------------|-------|
| vire-server | Main Vire backend API | 8500 | 8080 | `vire-server:latest` |

## Usage

```bash
# Start all services
./scripts/deploy.sh

# View logs
docker logs -f vire-server

# Stop all services
./scripts/deploy.sh down
```

## Configuration Priority

Command-line flags (highest priority) > Config file (TOML) > Defaults

## TOML Configuration

### `docker/vire.toml` — Server config

Server-level settings: storage paths, EODHD/Gemini API keys, logging. Also contains fallback defaults for portfolios and display_currency when running vire-server standalone (without MCP headers).

## GHCR Images

The CI workflow publishes the server image to GHCR:

- `ghcr.io/bobmcallan/vire-server`

Deploy from GHCR with:

```bash
./scripts/deploy.sh ghcr
```
