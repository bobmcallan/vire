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

# Build flags for self-contained binary
# CGO_ENABLED=0: Static linking, no C dependencies
# -ldflags="-s -w": Strip debug info and symbol table

echo "Building $OUTPUT_NAME..."

if [[ "$VERBOSE" == "true" ]]; then
    CGO_ENABLED=0 go build -v -ldflags="-s -w" -o "$BIN_DIR/$OUTPUT_NAME" "$MAIN_PKG"
else
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BIN_DIR/$OUTPUT_NAME" "$MAIN_PKG"
fi

# Copy configuration files for self-contained deployment
echo "Copying configuration files..."
cp "$PROJECT_ROOT/config/vire.toml" "$BIN_DIR/vire.toml"

if [[ -f "$PROJECT_ROOT/config/.env" ]]; then
    cp "$PROJECT_ROOT/config/.env" "$BIN_DIR/.env"
    echo "  - .env"
fi
echo "  - vire.toml"

# Show result
SIZE=$(du -h "$BIN_DIR/$OUTPUT_NAME" | cut -f1)
echo ""
echo "Built self-contained deployment:"
echo "  $BIN_DIR/"
echo "  ├── $OUTPUT_NAME ($SIZE)"
echo "  ├── vire.toml"
[[ -f "$BIN_DIR/.env" ]] && echo "  └── .env"
