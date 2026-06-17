#!/bin/bash
# Run on panel VPS (or anywhere with mysql client + DB access)
set -euo pipefail

APP_DIR="${1:-/www/wwwroot/panel.1clkaccess.store}"
SQL="$APP_DIR/database/migrations/002_chrome_portal_task_type.sql"
ENV_FILE="$APP_DIR/server/.env"

if [ ! -f "$ENV_FILE" ]; then
  echo "Missing $ENV_FILE"
  exit 1
fi

# shellcheck disable=SC1090
source <(grep -E '^DB_' "$ENV_FILE" | sed 's/^/export /')

echo "==> Applying migration + gfx_captureHomepage task..."
mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" < "$SQL"

echo "==> Verifying..."
mysql -h "$DB_HOST" -P "$DB_PORT" -u "$DB_USER" -p"$DB_PASS" "$DB_NAME" \
  -e "SELECT task_uid, task_name, automation_type FROM tasks WHERE task_uid='gfx_captureHomepage';"

echo "Done — refresh Automations page and search 'gfx' or 'homepage'."
