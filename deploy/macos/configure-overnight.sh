#!/bin/bash
# Keep Aman's Mac awake for overnight GFX worker (run once on the worker Mac).
# Usage: bash deploy/macos/configure-overnight.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HOME_DIR="$HOME"
PLIST_DIR="$HOME_DIR/Library/LaunchAgents"
LOG_DIR="$HOME_DIR/Library/Logs/gohttpauto"

echo "============================================"
echo "  GoHttpAuto — Overnight / lid-close setup"
echo "============================================"
echo ""

mkdir -p "$LOG_DIR" "$PLIST_DIR"

install_plist() {
  local src="$1" name="$2"
  sed "s|__HOME__|$HOME_DIR|g" "$src" > "$PLIST_DIR/$name"
}

echo "==> Installing caffeinate service (prevents idle sleep)..."
install_plist "$SCRIPT_DIR/com.gohttpauto.awake.plist" "com.gohttpauto.awake.plist"
launchctl bootout "gui/$(id -u)/com.gohttpauto.awake" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.awake.plist"

echo ""
echo "==> macOS power settings (needs Mac login password)..."
echo "    This reduces auto-sleep while charger is connected."
sudo pmset -c sleep 0
sudo pmset -c disksleep 0
sudo pmset -c displaysleep 10
sudo pmset -b sleep 15
sudo pmset -a tcpkeepalive 1
sudo pmset -a powernap 0

echo ""
echo "Current power settings:"
pmset -g custom

echo ""
echo "============================================"
echo "  Done!"
echo "============================================"
echo ""
echo "IMPORTANT for overnight GFX worker on MacBook:"
echo ""
echo "  1. Charger MUST stay plugged in."
echo "  2. Lid CLOSED without external monitor — Mac may still sleep"
echo "     (Apple hardware limit). Best options:"
echo "       • Lid OPEN + display auto-off (recommended)"
echo "       • OR lid closed + charger + external monitor"
echo "  3. Do NOT log out — user session must stay active."
echo "  4. Wi‑Fi: disable 'Ask to join networks' only; stay on stable Wi‑Fi."
echo ""
echo "Check worker is running:"
echo "  launchctl list | grep gohttpauto"
echo "  tail -f $LOG_DIR/worker.log"
echo ""
echo "Morning check on panel:"
echo "  https://panel.1clkaccess.store/logs"
echo "  https://panel.1clkaccess.store/queue  (worker should be online)"
echo "============================================"
