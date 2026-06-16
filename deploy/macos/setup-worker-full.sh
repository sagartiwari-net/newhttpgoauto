#!/bin/bash
# One-shot worker setup on Aman's Mac — run from anywhere:
#   bash deploy/macos/setup-worker-full.sh
# Or after clone:
#   cd ~/newhttpgoauto && bash deploy/macos/setup-worker-full.sh
set -euo pipefail

REPO_URL="https://github.com/sagartiwari-net/newhttpgoauto.git"
INSTALL_DIR="${1:-${GOHTTPAUTO_DIR:-$HOME/newhttpgoauto}}"
KEY="$HOME/.ssh/gohttpauto_vps"
VPS="root@74.208.99.161"

echo "============================================"
echo "  GoHttpAuto — Mac Worker Setup"
echo "============================================"
echo "Install dir: $INSTALL_DIR"
echo ""

# ── 1. Repo ──────────────────────────────────────────────────────────────────
if [ ! -d "$INSTALL_DIR/.git" ]; then
  echo "==> [1/6] Cloning repository..."
  git clone "$REPO_URL" "$INSTALL_DIR"
else
  echo "==> [1/6] Repository found — pulling latest..."
  git -C "$INSTALL_DIR" pull origin main
fi

DEPLOY="$INSTALL_DIR/deploy"
MACOS="$DEPLOY/macos"

# ── 2. Go ────────────────────────────────────────────────────────────────────
echo "==> [2/6] Installing Go (if needed)..."
bash "$DEPLOY/install-go-mac.sh"
export PATH="$HOME/.local/go/bin:$PATH"

# ── 3. server/.env ───────────────────────────────────────────────────────────
ENV_FILE="$INSTALL_DIR/server/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo "==> [3/6] Creating server/.env from template..."
  cp "$DEPLOY/.env.worker.example" "$ENV_FILE"
  # macOS + Linux sed
  if sed --version 2>/dev/null | grep -q GNU; then
    sed -i 's/^DB_HOST=.*/DB_HOST=127.0.0.1/' "$ENV_FILE"
    sed -i 's/^DB_PORT=.*/DB_PORT=3307/' "$ENV_FILE"
  else
    sed -i '' 's/^DB_HOST=.*/DB_HOST=127.0.0.1/' "$ENV_FILE"
    sed -i '' 's/^DB_PORT=.*/DB_PORT=3307/' "$ENV_FILE"
  fi
  echo ""
  echo "!! IMPORTANT: Edit these values in server/.env before worker can run:"
  echo "   nano $ENV_FILE"
  echo ""
  echo "   Change:"
  echo "     DB_PASS=YOUR_MYSQL_PASSWORD   (from panel VPS server/.env)"
  echo "     JWT_SECRET=SAME_AS_PANEL_SERVER"
  echo "     API_KEY=SAME_AS_PANEL"
  echo ""
  read -r -p "Press Enter after you have saved server/.env ... "
else
  echo "==> [3/6] server/.env already exists"
fi

if grep -q 'YOUR_MYSQL_PASSWORD\|SAME_AS_PANEL' "$ENV_FILE" 2>/dev/null; then
  echo ""
  echo "!! server/.env still has placeholder values (DB_PASS / JWT_SECRET / API_KEY)."
  echo "   Edit: nano $ENV_FILE"
  read -r -p "Press Enter after fixing placeholders ... "
fi

# ── 4. SSH key for DB tunnel ─────────────────────────────────────────────────
echo "==> [4/6] SSH key for VPS tunnel..."
if [ ! -f "$KEY" ]; then
  ssh-keygen -t ed25519 -f "$KEY" -N "" -C "gohttpauto-mac-tunnel"
  echo "Created: $KEY"
else
  echo "Key already exists: $KEY"
fi

# ── 5. Copy key to VPS (one time) ──────────────────────────────────────────
echo "==> [5/6] Authorizing SSH key on VPS..."
if ssh -i "$KEY" -o BatchMode=yes -o ConnectTimeout=8 -o StrictHostKeyChecking=accept-new "$VPS" exit 2>/dev/null; then
  echo "SSH key already authorized on VPS."
else
  echo "Enter VPS root password when prompted (only once):"
  ssh-copy-id -i "$KEY" "$VPS"
fi

# ── 6. Build + launchd services ──────────────────────────────────────────────
echo "==> [6/6] Installing always-on tunnel + worker services..."
bash "$MACOS/install-always-on.sh" "$INSTALL_DIR"

echo ""
echo "============================================"
echo "  Setup complete!"
echo "============================================"
echo ""
echo "Check logs:"
echo "  tail -f $HOME/Library/Logs/gohttpauto/worker.log"
echo "  tail -f $HOME/Library/Logs/gohttpauto/tunnel.log"
echo ""
echo "Panel: https://panel.1clkaccess.store"
echo "============================================"
