#!/bin/bash
# Keep worker Mac awake — including lid closed on charger (run once on the worker Mac).
# Usage: bash deploy/macos/configure-overnight.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
HOME_DIR="$HOME"
PLIST_DIR="$HOME_DIR/Library/LaunchAgents"
LOG_DIR="$HOME_DIR/Library/Logs/gohttpauto"

echo "============================================"
echo "  GoHttpAuto — Lid-close / always-on setup"
echo "============================================"
echo ""

mkdir -p "$LOG_DIR" "$PLIST_DIR"

install_plist() {
  local src="$1" name="$2"
  if [[ "$src" == *"worker"* ]] || [[ "$src" == *"tunnel"* ]]; then
    sed "s|__HOME__|$HOME_DIR|g; s|__APP_DIR__|$APP_DIR|g" "$src" > "$PLIST_DIR/$name"
  else
    sed "s|__HOME__|$HOME_DIR|g" "$src" > "$PLIST_DIR/$name"
  fi
}

echo "==> Installing caffeinate service (prevents idle + lid-close sleep)..."
install_plist "$SCRIPT_DIR/com.gohttpauto.awake.plist" "com.gohttpauto.awake.plist"
launchctl bootout "gui/$(id -u)/com.gohttpauto.awake" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.awake.plist"
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.awake" 2>/dev/null || true

echo ""
echo "==> macOS power settings (needs Mac login password)..."
echo "    Keeps system awake on charger even when lid is closed."
sudo pmset -a sleep 0
sudo pmset -a disksleep 0
sudo pmset -a disablesleep 1
sudo pmset -a standby 0
sudo pmset -a autopoweroff 0
sudo pmset -a hibernatemode 0
sudo pmset -a proximitywake 0
sudo pmset -a powernap 0
sudo pmset -a tcpkeepalive 1
sudo pmset -a displaysleep 10

echo ""
echo "Current power settings:"
pmset -g custom

echo ""
echo "============================================"
echo "  Done!"
echo "============================================"
echo ""
echo "Requirements for 24/7 worker on MacBook:"
echo ""
echo "  1. Charger MUST stay plugged in (disablesleep drains battery if unplugged)."
echo "  2. Do NOT log out — user session must stay active."
echo "  3. Wi‑Fi: stay on stable network; disable auto-switch."
echo "  4. Chrome runs headless — no visible browser window."
echo ""
echo "Verify services:"
echo "  launchctl list | grep gohttpauto"
echo "  tail -f $LOG_DIR/worker.log"
echo ""
echo "Revert sleep settings later (optional):"
echo "  sudo pmset -a disablesleep 0"
echo "  sudo pmset -a sleep 10"
echo "============================================"
