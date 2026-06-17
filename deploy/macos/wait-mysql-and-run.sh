#!/bin/bash
# Wait for SSH MySQL tunnel (127.0.0.1:3307) then exec the worker binary.
set -e
PORT="${GOHTTPAUTO_DB_PORT:-3307}"
HOST="${GOHTTPAUTO_DB_HOST:-127.0.0.1}"

for i in $(seq 1 90); do
  if nc -z "$HOST" "$PORT" 2>/dev/null; then
    exec "$@"
  fi
  sleep 1
done
echo "MySQL tunnel not ready at ${HOST}:${PORT} after 90s" >&2
exit 1
