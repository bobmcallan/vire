#!/bin/bash
set -euo pipefail

SERVICE_NAME_SERVER="vire-server"
SERVICE_NAME_PORTAL="vire-portal"
SERVICE_FILE_SERVER="/etc/systemd/system/${SERVICE_NAME_SERVER}.service"
SERVICE_FILE_PORTAL="/etc/systemd/system/${SERVICE_NAME_PORTAL}.service"

check_root() {
    if [[ $EUID -ne 0 ]]; then
        echo "This script must be run as root. Use: sudo $0"
        exit 1
    fi
}

uninstall_service() {
    local name="$1"
    local file="$2"

    if [[ ! -f "$file" ]]; then
        echo "  $name: not installed (no service file)"
        return
    fi

    echo "  $name: stopping..."
    systemctl stop "$name" 2>/dev/null || true

    echo "  $name: disabling..."
    systemctl disable "$name" 2>/dev/null || true

    echo "  $name: removing $file"
    rm -f "$file"

    echo "  $name: removed"
}

main() {
    check_root

    echo "=== Vire Service Uninstaller ==="
    echo ""

    uninstall_service "$SERVICE_NAME_SERVER" "$SERVICE_FILE_SERVER"
    echo ""
    uninstall_service "$SERVICE_NAME_PORTAL" "$SERVICE_FILE_PORTAL"

    echo ""
    echo "Reloading systemd..."
    systemctl daemon-reload
    systemctl reset-failed 2>/dev/null || true

    echo ""
    echo "Done. Both services have been stopped, disabled, and removed."
    echo "Docker containers are unaffected."
}

main "$@"
