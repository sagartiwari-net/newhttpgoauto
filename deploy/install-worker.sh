#!/bin/bash
# Run on automation Mac after git pull
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_DIR="$(dirname "$SCRIPT_DIR")"
cd "$APP_DIR/server"

if [ ! -f .env ]; then
  cp ../deploy/.env.worker.example .env
  echo "!! Edit server/.env with DB credentials (use SSH tunnel if needed)"
  exit 1
fi

go build -o gohttpauto ./cmd
echo "==> Worker built. Start with:"
echo "    cd $APP_DIR/server && ./gohttpauto"
echo "    (Keep SSH tunnel open if using DB_PORT=3307)"
