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

echo "==> Refreshing worker launchd plist..."
MACOS="$APP_DIR/deploy/macos"
PLIST_DIR="$HOME/Library/LaunchAgents"
sed "s|__HOME__|$HOME|g; s|__APP_DIR__|$APP_DIR|g" "$MACOS/com.gohttpauto.worker.plist" > "$PLIST_DIR/com.gohttpauto.worker.plist"
launchctl bootout "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.worker.plist"

echo "==> Restarting worker service..."
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || {
  echo "!! launchctl kickstart failed — start manually:"
  echo "   cd $APP_DIR/server && ./gohttpauto"
}

echo "==> Done. Worker updated from $(git -C "$APP_DIR" rev-parse --short HEAD)"
if grep -q '<key>GFX_VISIBLE</key>' "$PLIST_DIR/com.gohttpauto.worker.plist" 2>/dev/null; then
  echo "   GFX_VISIBLE=1 (Chrome window will show during GFX tasks)"
fi
