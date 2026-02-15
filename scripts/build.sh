#!/bin/bash
set -euo pipefail

# Build script for Vire server binary
# Outputs self-contained binary to ./bin

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"
CONFIG_DIR="$PROJECT_ROOT/config"

MODULE="github.com/bobmcallan/vire/internal/common"

# Parse arguments
VERBOSE=false
CLEAN=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--verbose) VERBOSE=true; shift ;;
        -c|--clean) CLEAN=true; shift ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  -v, --verbose  Show verbose build output"
            echo "  -c, --clean    Clean bin directory before building"
            echo "  -h, --help     Show this help message"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

cd "$PROJECT_ROOT"

# Clean if requested
if [[ "$CLEAN" == "true" ]]; then
    echo "Cleaning $BIN_DIR..."
    rm -rf "$BIN_DIR"
fi

# Create output directory
mkdir -p "$BIN_DIR"

# Extract version info
VERSION="dev"
BUILD_TS="unknown"
VERSION_FILE="$PROJECT_ROOT/.version"
if [[ -f "$VERSION_FILE" ]]; then
    VERSION=$(grep "^version:" "$VERSION_FILE" | sed 's/version:\s*//' | tr -d ' ')
    BUILD_TS=$(date +"%m-%d-%H-%M-%S")
    # Update build timestamp and contributor in .version file
    sed -i "s/^build:.*/build: $BUILD_TS/" "$VERSION_FILE"
    CONTRIBUTOR=$(git config user.email 2>/dev/null || echo "unknown")
    sed -i "s/^contributor:.*/contributor: $CONTRIBUTOR/" "$VERSION_FILE"
    grep -q "^contributor:" "$VERSION_FILE" || echo "contributor: $CONTRIBUTOR" >> "$VERSION_FILE"
fi
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS="-s -w -X '$MODULE.Version=$VERSION' -X '$MODULE.Build=$BUILD_TS' -X '$MODULE.GitCommit=$GIT_COMMIT'"

echo "Building vire v$VERSION (commit: $GIT_COMMIT)..."

# Build vire-server
echo "  Building vire-server..."
if [[ "$VERBOSE" == "true" ]]; then
    CGO_ENABLED=0 go build -v -ldflags="$LDFLAGS" -o "$BIN_DIR/vire-server" "./cmd/vire-server"
else
    CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o "$BIN_DIR/vire-server" "./cmd/vire-server"
fi

# Copy configuration for self-contained deployment
echo "Copying configuration..."
if [[ -f "$CONFIG_DIR/vire-service.toml" ]]; then
    cp "$CONFIG_DIR/vire-service.toml" "$BIN_DIR/vire-service.toml"
else
    cp "$CONFIG_DIR/vire.toml.example" "$BIN_DIR/vire-service.toml"
fi

# Show result
SERVER_SIZE=$(du -h "$BIN_DIR/vire-server" | cut -f1)
echo ""
echo "Built self-contained deployment:"
echo "  $BIN_DIR/"
echo "  ├── vire-server ($SERVER_SIZE)"
echo "  └── vire-service.toml"
