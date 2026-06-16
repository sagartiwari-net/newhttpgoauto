#!/bin/bash
# Run on VPS as root after git clone to /www/wwwroot/panel.1clkaccess.store
set -e

APP_DIR="/www/wwwroot/panel.1clkaccess.store"
cd "$APP_DIR"

echo "==> Building dashboard..."
cd dashboard
npm ci
npm run build
cd ..

echo "==> Building Go server..."
cd server
if [ ! -f .env ]; then
  cp ../deploy/.env.panel.example .env
  echo "!! Created server/.env from template — edit passwords before starting"
fi
if ! command -v go >/dev/null 2>&1; then
  echo "==> Go not found — installing Go 1.22..."
  bash ../deploy/install-go.sh
  export PATH=$PATH:/usr/local/go/bin
fi
go build -o gohttpauto ./cmd
cd ..

echo "==> Installing systemd service..."
cp deploy/gohttpauto.service /etc/systemd/system/gohttpauto.service
systemctl daemon-reload
systemctl enable gohttpauto
systemctl restart gohttpauto

echo "==> Done. Configure nginx reverse proxy to 127.0.0.1:4010"
echo "    See deploy/nginx-panel.conf.example"
systemctl status gohttpauto --no-pager || true
