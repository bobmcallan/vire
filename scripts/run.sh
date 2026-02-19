#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG_DIR="$PROJECT_DIR/config"
BIN_DIR="$PROJECT_DIR/bin"
PID_FILE="$BIN_DIR/vire-server.pid"

# Read port from environment or default
PORT="${VIRE_PORT:-8501}"

stop_server() {
    if [ ! -f "$PID_FILE" ]; then
        echo "No PID file found."
        return 0
    fi

    OLD_PID=$(cat "$PID_FILE")

    # Try graceful HTTP shutdown first
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Requesting graceful shutdown..."
        if curl -sf -X POST "http://localhost:$PORT/api/shutdown" --max-time 5 > /dev/null 2>&1; then
            # Wait for process to exit
            for i in $(seq 1 10); do
                if ! kill -0 "$OLD_PID" 2>/dev/null; then
                    echo "Server stopped gracefully."
                    rm -f "$PID_FILE"
                    return 0
                fi
                sleep 0.5
            done
        fi

        # Fallback to SIGTERM
        echo "Sending SIGTERM..."
        kill "$OLD_PID" 2>/dev/null || true
        sleep 2
    fi

    rm -f "$PID_FILE"
}

case "${1:-start}" in
  start)
    # Stop existing instance
    stop_server

    # Ensure config exists
    if [ ! -f "$CONFIG_DIR/vire-service.toml" ]; then
        echo "Creating default vire-service.toml from config/vire-service.toml.example..."
        cp "$CONFIG_DIR/vire-service.toml.example" "$CONFIG_DIR/vire-service.toml"
    fi

    # Extract version info
    VERSION="dev"
    BUILD_TS=$(date +"%m-%d-%H-%M-%S")
    if [ -f "$PROJECT_DIR/.version" ]; then
        VERSION=$(grep "^version:" "$PROJECT_DIR/.version" | sed 's/version:\s*//' | tr -d ' ')
    fi
    GIT_COMMIT=$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || echo "unknown")

    # Build
    LDFLAGS="-s -w \
        -X 'github.com/bobmcallan/vire/internal/common.Version=${VERSION}' \
        -X 'github.com/bobmcallan/vire/internal/common.Build=${BUILD_TS}' \
        -X 'github.com/bobmcallan/vire/internal/common.GitCommit=${GIT_COMMIT}'"

    echo "Building vire-server v$VERSION (commit: $GIT_COMMIT)..."
    cd "$PROJECT_DIR"
    go build -ldflags="$LDFLAGS" -o "$BIN_DIR/vire-server" ./cmd/vire-server

    # Copy config alongside binary
    cp "$CONFIG_DIR/vire-service.toml" "$BIN_DIR/vire-service.toml"

    # Start detached
    "$BIN_DIR/vire-server" > /dev/null 2>&1 &
    SERVER_PID=$!
    echo "$SERVER_PID" > "$PID_FILE"

    sleep 1
    if kill -0 "$SERVER_PID" 2>/dev/null; then
        echo "vire-server v$VERSION running on :$PORT (PID $SERVER_PID)"
        echo "  http://localhost:$PORT/api/health"
        echo "  Stop: ./scripts/run.sh stop"
    else
        echo "vire-server failed to start"
        rm -f "$PID_FILE"
        exit 1
    fi
    ;;
  stop)
    stop_server
    ;;
  restart)
    stop_server
    exec "$0" start
    ;;
  status)
    if [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
        echo "vire-server running on :$PORT (PID $(cat "$PID_FILE"))"
        curl -sf "http://localhost:$PORT/api/version" 2>/dev/null || true
    else
        echo "vire-server not running"
        rm -f "$PID_FILE" 2>/dev/null
    fi
    ;;
  *)
    echo "Usage: ./scripts/run.sh [start|stop|restart|status]"
    exit 1
    ;;
esac
