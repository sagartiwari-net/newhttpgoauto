#!/bin/bash
# VPS panel: git pull + rebuild + restart (run as root on panel server)
set -e

APP_DIR="/www/wwwroot/panel.1clkaccess.store"

echo "==> Pulling latest code..."
cd "$APP_DIR"
git pull origin main

echo "==> Building Go panel..."
cd "$APP_DIR/server"
go build -buildvcs=false -o gohttpauto ./cmd

echo "==> Building dashboard..."
cd "$APP_DIR/dashboard"
npm ci
npm run build

echo "==> Restarting panel service..."
systemctl restart gohttpauto

echo "==> Done. Panel updated from $(git -C "$APP_DIR" rev-parse --short HEAD)"
systemctl status gohttpauto --no-pager || true
