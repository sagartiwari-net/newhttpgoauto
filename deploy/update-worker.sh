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

echo "==> Refreshing launchd services (worker + tunnel + awake)..."
MACOS="$APP_DIR/deploy/macos"
PLIST_DIR="$HOME/Library/LaunchAgents"
LOG_DIR="$HOME/Library/Logs/gohttpauto"
mkdir -p "$PLIST_DIR" "$LOG_DIR" "$HOME/Desktop/screenshot/gfx"
if [ -f "$LOG_DIR/worker.log" ]; then
  mv "$LOG_DIR/worker.log" "$LOG_DIR/worker.log.$(date +%Y%m%d-%H%M%S).bak" 2>/dev/null || true
fi
touch "$LOG_DIR/worker.log"
sed "s|__HOME__|$HOME|g; s|__APP_DIR__|$APP_DIR|g" "$MACOS/com.gohttpauto.worker.plist" > "$PLIST_DIR/com.gohttpauto.worker.plist"
sed "s|__HOME__|$HOME|g; s|__APP_DIR__|$APP_DIR|g" "$MACOS/com.gohttpauto.tunnel.plist" > "$PLIST_DIR/com.gohttpauto.tunnel.plist"
sed "s|__HOME__|$HOME|g" "$MACOS/com.gohttpauto.awake.plist" > "$PLIST_DIR/com.gohttpauto.awake.plist"
chmod +x "$MACOS/wait-mysql-and-run.sh"
launchctl bootout "gui/$(id -u)/com.gohttpauto.tunnel" 2>/dev/null || true
launchctl bootout "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || true
launchctl bootout "gui/$(id -u)/com.gohttpauto.awake" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.awake.plist"
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.tunnel.plist"
launchctl bootstrap "gui/$(id -u)" "$PLIST_DIR/com.gohttpauto.worker.plist"

echo "==> Restarting worker service..."
# Stop any stray panel-mode process that binds :4011 (blocks/confuses debugging)
if PIDS=$(lsof -ti :4011 2>/dev/null); then
  echo "    Stopping old process on port 4011: $PIDS"
  kill $PIDS 2>/dev/null || true
  sleep 1
fi
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.awake" 2>/dev/null || true
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.tunnel" 2>/dev/null || true
sleep 4
launchctl kickstart -k "gui/$(id -u)/com.gohttpauto.worker" 2>/dev/null || {
  echo "!! launchctl kickstart failed — start manually:"
  echo "   cd $APP_DIR/server && ROLE=worker ./gohttpauto"
}

sleep 5
echo "==> Worker status:"
launchctl list | grep gohttpauto || true
nc -z 127.0.0.1 3307 && echo "    MySQL tunnel: OK" || echo "    MySQL tunnel: DOWN — run launchctl kickstart tunnel"
echo "==> Worker log:"
if grep -q "polling job_queue" "$LOG_DIR/worker.log" 2>/dev/null; then
  grep -E "GoHttpAuto start|ROLE|WORKER Ready|Job poller|Connected to" "$LOG_DIR/worker.log" | tail -8
  echo "    ✅ Worker mode OK"
else
  tail -15 "$LOG_DIR/worker.log" 2>/dev/null || true
  echo "    !! Worker not ready — wait 10s and run: tail -20 $LOG_DIR/worker.log"
fi

echo "==> Done. Worker updated from $(git -C "$APP_DIR" rev-parse --short HEAD)"
echo "    GFX Chrome: headless (no window). Debug visible: GFX_VISIBLE=1 in worker plist"
echo "    Failure screenshots → $HOME/Desktop/screenshot/gfx/"
echo ""
echo "    Lid-close / overnight (run once, needs Mac password):"
echo "    bash $MACOS/configure-overnight.sh"
