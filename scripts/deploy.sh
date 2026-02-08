#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_DIR="$SCRIPT_DIR/docker"
MODE="${1:-local}"

case "$MODE" in
  local)
    echo "Deploying local build on port 4242..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" build
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d
    ;;
  ghcr)
    echo "Deploying ghcr image with auto-update on port 4242..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" up --pull always -d
    ;;
  down)
    echo "Stopping all vire containers..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down 2>/dev/null || true
    ;;
  *)
    echo "Usage: ./deploy [local|ghcr|down]"
    echo ""
    echo "  local  (default) Build and deploy from local Dockerfile"
    echo "  ghcr   Deploy ghcr.io/bobmcallan/vire-mcp:latest with Watchtower auto-update"
    echo "  down   Stop all vire containers"
    exit 1
    ;;
esac

echo "Done. Waiting for health check..."
sleep 3
docker ps --filter "name=vire" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
