#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
COMPOSE_DIR="$PROJECT_DIR/docker"

# Parse arguments
MODE="${1:-local}"
FORCE=false
shift || true
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=true ;;
    esac
done

case "$MODE" in
  local)
    # Extract version info
    VERSION="dev"
    BUILD_TS=$(date +"%m-%d-%H-%M-%S")
    if [ -f "$PROJECT_DIR/.version" ]; then
        VERSION=$(grep "^version:" "$PROJECT_DIR/.version" | sed 's/version:\s*//' | tr -d ' ')
        sed -i "s/^build:.*/build: $BUILD_TS/" "$PROJECT_DIR/.version"
    fi
    GIT_COMMIT=$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")
    export VERSION BUILD=$BUILD_TS GIT_COMMIT

    # Smart rebuild check
    NEEDS_REBUILD=false
    if [ "$FORCE" = true ]; then
        NEEDS_REBUILD=true
    elif [ ! -f "$COMPOSE_DIR/.last_build" ]; then
        NEEDS_REBUILD=true
    else
        if find "$PROJECT_DIR" -name "*.go" -newer "$COMPOSE_DIR/.last_build" 2>/dev/null | grep -q . || \
           [ "$PROJECT_DIR/go.mod" -nt "$COMPOSE_DIR/.last_build" ] || \
           [ "$PROJECT_DIR/go.sum" -nt "$COMPOSE_DIR/.last_build" ]; then
            NEEDS_REBUILD=true
        fi
    fi

    if [ "$NEEDS_REBUILD" = true ]; then
        echo "Building vire-mcp v$VERSION (commit: $GIT_COMMIT)..."
        # Stop any ghcr container first (different compose file)
        docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down --remove-orphans 2>/dev/null || true
        # Build the new image while the old container keeps running
        if [ "$FORCE" = true ]; then
            docker image rm vire-vire-mcp 2>/dev/null || true
            docker compose -f "$COMPOSE_DIR/docker-compose.yml" build --no-cache
        else
            docker compose -f "$COMPOSE_DIR/docker-compose.yml" build
        fi
        touch "$COMPOSE_DIR/.last_build"
        echo " Image vire-vire-mcp Built "
    else
        echo "No changes detected, skipping rebuild."
    fi

    # Start or recreate container with the latest image (zero-downtime swap).
    # compose up --force-recreate replaces the running container in one step:
    # new container starts, then old one is removed.
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d --force-recreate --remove-orphans
    ;;
  ghcr)
    echo "Deploying ghcr image with auto-update..."
    # Stop any local-build container first (different compose file)
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans 2>/dev/null || true
    # Pull new image and swap container in one step
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" up --pull always -d --force-recreate --remove-orphans
    ;;
  down)
    echo "Stopping all vire containers..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down --remove-orphans 2>/dev/null || true
    ;;
  *)
    echo "Usage: ./scripts/deploy.sh [local|ghcr|down] [--force]"
    echo ""
    echo "  local  (default) Build and deploy from local Dockerfile"
    echo "  ghcr   Deploy ghcr.io/bobmcallan/vire-mcp:latest with Watchtower auto-update"
    echo "  down   Stop all vire containers"
    echo ""
    echo "Options:"
    echo "  --force  Force clean rebuild (removes cached image)"
    exit 1
    ;;
esac

echo "Done."
sleep 2
docker ps --filter "name=vire" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
echo ""
echo "Logs: docker logs -f vire-mcp"
echo "Health: curl http://localhost:4242/api/health"
echo "MCP: http://localhost:4242/mcp"
