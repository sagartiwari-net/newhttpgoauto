#!/bin/bash
# Full automation worker setup on Mac (no Homebrew required)
set -e

REPO_URL="https://github.com/sagartiwari-net/newhttpgoauto.git"
INSTALL_DIR="${HOME}/newhttpgoauto"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# If run from /tmp (curl download), clone repo first so helper scripts are available
if [ ! -f "$SCRIPT_DIR/install-go-mac.sh" ]; then
  echo "==> [0/4] Cloning repository (helper scripts not found locally)..."
  if [ ! -d "$INSTALL_DIR/.git" ]; then
    git clone "$REPO_URL" "$INSTALL_DIR"
  fi
  SCRIPT_DIR="$INSTALL_DIR/deploy"
fi

echo "==> [1/4] Installing Go..."
bash "$SCRIPT_DIR/install-go-mac.sh"
export PATH="$HOME/.local/go/bin:$PATH"

if [ ! -d "$INSTALL_DIR/.git" ]; then
  echo "==> [2/4] Cloning repository..."
  git clone "$REPO_URL" "$INSTALL_DIR"
else
  echo "==> [2/4] Repository exists — pulling latest..."
  cd "$INSTALL_DIR"
  git pull
fi

cd "$INSTALL_DIR"

if [ ! -f server/.env ]; then
  echo "==> [3/4] Creating server/.env from template..."
  cp deploy/.env.worker.example server/.env
  echo ""
  echo "!! IMPORTANT: Edit server/.env before starting worker:"
  echo "   nano $INSTALL_DIR/server/.env"
  echo ""
  echo "   Use SSH tunnel DB settings:"
  echo "     DB_HOST=127.0.0.1"
  echo "     DB_PORT=3307"
  echo "   Copy JWT_SECRET, API_KEY, DB_PASS from panel server .env"
  echo ""
else
  echo "==> [3/4] server/.env already exists"
fi

echo "==> [4/4] Building worker..."
cd server
go build -buildvcs=false -o gohttpauto ./cmd

echo ""
echo "============================================"
echo "  Basic setup complete!"
echo "============================================"
echo ""
echo "Next — run full always-on setup (tunnel + launchd):"
echo "  bash $INSTALL_DIR/deploy/macos/setup-worker-full.sh"
echo ""
echo "Or if repo already cloned elsewhere:"
echo "  cd ~/newhttpgoauto"
echo "  bash deploy/macos/setup-worker-full.sh"
echo ""
echo "Panel: https://panel.1clkaccess.store"
echo "============================================"
