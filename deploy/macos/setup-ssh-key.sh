#!/bin/bash
# One-time: generate SSH key for VPS tunnel (no password prompt at boot)
set -e

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
echo "Test tunnel:"
echo "    ssh -i $KEY -N -L 3307:127.0.0.1:3306 $VPS"
echo ""
echo "Then run: bash deploy/macos/install-always-on.sh"
