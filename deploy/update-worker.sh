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

echo "==> Restarting worker service..."
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || {
  echo "!! launchctl kickstart failed — start manually:"
  echo "   cd $APP_DIR/server && ./gohttpauto"
}

echo "==> Done. Worker updated from $(git -C "$APP_DIR" rev-parse --short HEAD)"
