#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_DIR="$PROJECT_DIR/docker"

# Compose file combinations
BASE="-f $COMPOSE_DIR/docker-compose.yml"
GHCR="$BASE -f $COMPOSE_DIR/docker-compose.ghcr.yml"

MODE="${1:-ghcr}"

case "$MODE" in
  ghcr)
    echo "Deploying ghcr image with auto-update..."
    docker compose $GHCR up --pull always -d --force-recreate
    sleep 2
    docker ps --filter "name=vire" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
    echo ""
    echo "Logs: docker logs -f vire-server"
    echo "Health: curl http://localhost:4242/api/health"
    ;;
  down)
    echo "Stopping vire docker containers..."
    docker compose $BASE down
    ;;
  prune)
    echo "Pruning stopped containers, dangling images, and unused volumes..."
    docker container prune -f
    docker image prune -f
    docker volume prune -f
    echo "Prune complete."
    ;;
  *)
    echo "Usage: ./scripts/deploy.sh [ghcr|down|prune]"
    exit 1
    ;;
esac
