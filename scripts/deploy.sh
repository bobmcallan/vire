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
        docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans 2>/dev/null || true
        docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down --remove-orphans 2>/dev/null || true
        if [ "$FORCE" = true ]; then
            docker image rm vire-vire-mcp 2>/dev/null || true
            docker compose -f "$COMPOSE_DIR/docker-compose.yml" build --no-cache
        else
            docker compose -f "$COMPOSE_DIR/docker-compose.yml" build
        fi
        touch "$COMPOSE_DIR/.last_build"
    else
        echo "No changes detected, skipping rebuild."
    fi

    # Ensure container is running and remove orphaned services
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d --remove-orphans
    ;;
  ghcr)
    echo "Deploying ghcr image with auto-update..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down --remove-orphans 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down --remove-orphans 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" up --pull always -d --remove-orphans
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
