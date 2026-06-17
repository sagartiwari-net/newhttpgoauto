#!/bin/bash
# Mac worker: git pull + rebuild + restart (run on Aman's Mac)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$(dirname "$SCRIPT_DIR")"

echo "==> Pulling latest code..."
cd "$APP_DIR"
git pull origin main

echo "==> Building worker..."
cd "$APP_DIR/server"
go build -buildvcs=false -o gohttpauto ./cmd

echo "==> Refreshing worker launchd plist (GFX_VISIBLE=1 for debugging)..."
MACOS="$APP_DIR/deploy/macos"
PLIST_DIR="$HOME/Library/LaunchAgents"
mkdir -p "$PLIST_DIR" "$HOME/Library/Logs/gohttpauto" "$HOME/Desktop/screenshot/gfx"
sed "s|__HOME__|$HOME|g; s|__APP_DIR__|$APP_DIR|g" "$MACOS/com.gohttpauto.worker.plist" > "$PLIST_DIR/com.gohttpauto.worker.plist"
chmod +x "$MACOS/wait-mysql-and-run.sh"
launchctl bootout "gui/$(id -u)/com.gohttpauto.tunnel" 2>/dev/null || true
launchctl bootout "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || true
sed "s|__HOME__|$HOME|g; s|__APP_DIR__|$APP_DIR|g" "$MACOS/com.gohttpauto.tunnel.plist" > "$PLIST_DIR/com.gohttpauto.tunnel.plist"
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.tunnel.plist"
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.worker.plist"

echo "==> Restarting worker service..."
# Stop any stray panel-mode process that binds :4011 (blocks/confuses debugging)
if PIDS=$(lsof -ti :4011 2>/dev/null); then
  echo "    Stopping old process on port 4011: $PIDS"
  kill $PIDS 2>/dev/null || true
  sleep 1
fi
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.tunnel" 2>/dev/null || true
sleep 2
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || {
  echo "!! launchctl kickstart failed — start manually:"
  echo "   cd $APP_DIR/server && ROLE=worker ./gohttpauto"
}

sleep 3
echo "==> Worker log (last lines):"
tail -8 "$HOME/Library/Logs/gohttpauto/worker.log" 2>/dev/null || true
if ! grep -q "polling job_queue\|Job poller started" "$HOME/Library/Logs/gohttpauto/worker.log" 2>/dev/null; then
  echo "!! Also check: tail -20 $HOME/Library/Logs/gohttpauto/worker.err.log"
fi

echo "==> Done. Worker updated from $(git -C "$APP_DIR" rev-parse --short HEAD)"
echo "    GFX Chrome visible (GFX_VISIBLE=1) — failure screenshots → $HOME/Desktop/screenshot/gfx/"
echo "    Headless wapas: plist se GFX_VISIBLE hata dena ya 0 karna"
