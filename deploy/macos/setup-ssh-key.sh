#!/bin/bash
# One-time: generate SSH key for VPS tunnel (no password prompt at boot)
# Prefer: bash deploy/macos/setup-worker-full.sh  (does everything)
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

KEY="$HOME/.ssh/gohttpauto_vps"
VPS="root@74.208.99.161"

if [ ! -f "$KEY" ]; then
  echo "==> Generating SSH key at $KEY"
  ssh-keygen -t ed25519 -f "$KEY" -N "" -C "gohttpauto-mac-tunnel"
fi

echo ""
echo "==> Copy this key to the VPS (enter VPS password once):"
echo "    ssh-copy-id -i $KEY $VPS"
echo ""
echo "Then run full setup from repo root:"
echo "    cd $APP_DIR"
echo "    bash deploy/macos/setup-worker-full.sh"
