#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
BIN_DIR="$PROJECT_ROOT/bin"
SERVICE_NAME="vire-server"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

check_root() {
    if [[ $EUID -ne 0 ]]; then
        echo "This script must be run as root. Use: sudo $0"
        exit 1
    fi
}

stop_run_sh_instance() {
    PID_FILE="$BIN_DIR/vire-server.pid"
    
    if [[ -f "$PID_FILE" ]]; then
        OLD_PID=$(cat "$PID_FILE")
        if kill -0 "$OLD_PID" 2>/dev/null; then
            echo "Stopping existing vire-server (PID $OLD_PID)..."
            kill "$OLD_PID" 2>/dev/null || true
            sleep 1
        fi
        rm -f "$PID_FILE"
    fi
}



build() {
    echo "Building vire-server..."
    "$SCRIPT_DIR/build.sh"
}

verify_files() {
    echo "Verifying build output..."
    
    if [[ ! -f "$BIN_DIR/vire-server" ]]; then
        echo "  Error: $BIN_DIR/vire-server not found"
        exit 1
    fi
    
    if [[ ! -f "$BIN_DIR/vire-service.toml" ]]; then
        echo "  Error: $BIN_DIR/vire-service.toml not found"
        exit 1
    fi
    
    echo "  Binary: $BIN_DIR/vire-server"
    echo "  Config: $BIN_DIR/vire-service.toml"
}

create_service_file() {
    echo "Creating systemd service..."
    
    cat > "$SERVICE_FILE" << EOF
[Unit]
Description=Vire Server
After=network.target

[Service]
Type=simple
User=root
Environment=VIRE_CONFIG=${BIN_DIR}/vire-service.toml
ExecStart=${BIN_DIR}/vire-server
WorkingDirectory=${BIN_DIR}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    echo "  Created: $SERVICE_FILE"
}

install_service() {
    echo "Installing systemd service..."
    
    systemctl daemon-reload
    systemctl enable "$SERVICE_NAME"
    systemctl restart "$SERVICE_NAME"
    
    echo "  Service enabled and started"
}

show_status() {
    echo ""
    echo "Installation complete!"
    echo ""
    echo "Commands:"
    echo "  sudo systemctl status $SERVICE_NAME   # Check status"
    echo "  sudo systemctl restart $SERVICE_NAME  # Restart service"
    echo "  sudo systemctl stop $SERVICE_NAME     # Stop service"
    echo "  sudo journalctl -u $SERVICE_NAME -f   # View logs"
}

main() {
    check_root
    
    echo "=== Vire Server Installer ==="
    echo ""
    
    stop_run_sh_instance
    build
    verify_files
    create_service_file
    install_service
    show_status
}

main "$@"
