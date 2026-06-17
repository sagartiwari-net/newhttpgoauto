#!/bin/bash
# Install always-on tunnel + worker on Aman's Mac (run via SSH as amantiwari)
set -e

APP_DIR="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
HOME_DIR="$HOME"
PLIST_DIR="$HOME_DIR/Library/LaunchAgents"
LOG_DIR="$HOME_DIR/Library/Logs/gohttpauto"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> App dir: $APP_DIR"
mkdir -p "$LOG_DIR" "$PLIST_DIR"

# Build worker binary
export PATH="$HOME/.local/go/bin:$PATH"
cd "$APP_DIR/server"
go build -buildvcs=false -o gohttpauto ./cmd
chmod +x gohttpauto

install_plist() {
  local src="$1" name="$2"
  sed "s|__HOME__|$HOME_DIR|g; s|__APP_DIR__|$APP_DIR|g" "$src" > "$PLIST_DIR/$name"
}

install_plist "$SCRIPT_DIR/com.gohttpauto.tunnel.plist" "com.gohttpauto.tunnel.plist"
install_plist "$SCRIPT_DIR/com.gohttpauto.worker.plist" "com.gohttpauto.worker.plist"
install_plist "$SCRIPT_DIR/com.gohttpauto.awake.plist" "com.gohttpauto.awake.plist"
chmod +x "$SCRIPT_DIR/wait-mysql-and-run.sh"

# Reload services
launchctl bootout "gui/$(id -u)/com.gohttpauto.tunnel" 2>/dev/null || true
launchctl bootout "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || true
launchctl bootout "gui/$(id -u)/com.gohttpauto.awake" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.tunnel.plist"
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.worker.plist"
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.awake.plist"

echo ""
echo "============================================"
echo "  Always-on services installed!"
echo "============================================"
echo "Logs:"
echo "  tail -f $LOG_DIR/worker.log"
echo "  tail -f $LOG_DIR/tunnel.log"
echo "  tail -f $LOG_DIR/awake.log"
echo ""
echo "Overnight (lid close / no sleep): bash $SCRIPT_DIR/configure-overnight.sh"
echo ""
echo "IMPORTANT: Copy SSH key to VPS (one time, needs VPS password):"
echo "  ssh-copy-id -i $HOME/.ssh/gohttpauto_vps root@74.208.99.161"
echo "============================================"
