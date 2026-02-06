#!/bin/bash
set -euo pipefail

# Build script for Vire MCP server
# Outputs a self-contained binary to ./bin

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"

# Build configuration
OUTPUT_NAME="vire-mcp"
MAIN_PKG="./cmd/vire-mcp"
MODULE="github.com/bobmccarthy/vire/internal/common"

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
    # Update build timestamp in .version file
    sed -i "s/^build:.*/build: $BUILD_TS/" "$VERSION_FILE"
fi
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

LDFLAGS="-s -w -X '$MODULE.Version=$VERSION' -X '$MODULE.Build=$BUILD_TS' -X '$MODULE.GitCommit=$GIT_COMMIT'"

echo "Building $OUTPUT_NAME v$VERSION (commit: $GIT_COMMIT)..."

if [[ "$VERBOSE" == "true" ]]; then
    CGO_ENABLED=0 go build -v -ldflags="$LDFLAGS" -o "$BIN_DIR/$OUTPUT_NAME" "$MAIN_PKG"
else
    CGO_ENABLED=0 go build -ldflags="$LDFLAGS" -o "$BIN_DIR/$OUTPUT_NAME" "$MAIN_PKG"
fi

# Copy configuration files for self-contained deployment
echo "Copying configuration files..."
cp "$PROJECT_ROOT/config/vire.toml" "$BIN_DIR/vire.toml"
cp "$PROJECT_ROOT/.version" "$BIN_DIR/.version"

if [[ -f "$PROJECT_ROOT/config/.env" ]]; then
    cp "$PROJECT_ROOT/config/.env" "$BIN_DIR/.env"
    echo "  - .env"
fi
echo "  - vire.toml"
echo "  - .version"

# Show result
SIZE=$(du -h "$BIN_DIR/$OUTPUT_NAME" | cut -f1)
echo ""
echo "Built self-contained deployment:"
echo "  $BIN_DIR/"
echo "  ├── $OUTPUT_NAME ($SIZE)"
echo "  ├── vire.toml"
echo "  ├── .version"
[[ -f "$BIN_DIR/.env" ]] && echo "  └── .env" || true
