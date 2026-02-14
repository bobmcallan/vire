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
        # Update build timestamp and contributor in .version file
        sed -i "s/^build:.*/build: $BUILD_TS/" "$PROJECT_DIR/.version"
        CONTRIBUTOR=$(git config user.email 2>/dev/null || echo "unknown")
        sed -i "s/^contributor:.*/contributor: $CONTRIBUTOR/" "$PROJECT_DIR/.version"
        grep -q "^contributor:" "$PROJECT_DIR/.version" || echo "contributor: $CONTRIBUTOR" >> "$PROJECT_DIR/.version"
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
        echo "Building vire v$VERSION (commit: $GIT_COMMIT)..."
        # Stop any ghcr container first (different compose file)
        docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down 2>/dev/null || true
        # Build new image while old containers keep running
        if [ "$FORCE" = true ]; then
            docker image rm vire-server:latest 2>/dev/null || true
            docker compose -f "$COMPOSE_DIR/docker-compose.yml" build --no-cache
        else
            docker compose -f "$COMPOSE_DIR/docker-compose.yml" build
        fi
        touch "$COMPOSE_DIR/.last_build"
        echo " Image vire-server:latest built "
    else
        echo "No changes detected, skipping rebuild."
    fi

    # Ensure config files exist for volume mounts
    if [ ! -f "$COMPOSE_DIR/vire.toml" ]; then
        echo "Creating default vire.toml from docker/vire.toml.docker..."
        cp "$COMPOSE_DIR/vire.toml.docker" "$COMPOSE_DIR/vire.toml"
    fi
    # Start or recreate container with latest image (zero-downtime swap).
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" up -d --force-recreate
    ;;
  ghcr)
    echo "Deploying ghcr image with auto-update..."
    # Stop any local-build container first (different compose file)
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down 2>/dev/null || true
    # Pull new image and swap container in one step
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" up --pull always -d --force-recreate
    ;;
  down)
    echo "Stopping all vire containers..."
    docker compose -f "$COMPOSE_DIR/docker-compose.yml" down 2>/dev/null || true
    docker compose -f "$COMPOSE_DIR/docker-compose.ghcr.yml" down 2>/dev/null || true
    ;;
  prune)
    echo "Pruning stopped containers, dangling images, and unused volumes..."
    docker container prune -f
    docker image prune -f
    docker volume prune -f
    echo "Prune complete."
    ;;
  *)
    echo "Usage: ./scripts/deploy.sh [local|ghcr|down|prune] [--force]"
    exit 1
    ;;
esac

sleep 2
docker ps --filter "name=vire" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}"
echo ""
echo "Logs: docker logs -f vire-server"
echo "Health: curl http://localhost:4242/api/health"
