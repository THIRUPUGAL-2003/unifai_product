#!/bin/sh
# Recreate Docker stack dynamically using .env configuration.
set -e

cd "$(dirname "$0")/../.."

if [ -f .env ]; then
  set -a
  . ./.env
  set +a
fi

CONTAINER="${CONTAINER_NAME:?set CONTAINER_NAME in .env}"
PORT="${APP_PORT:?set APP_PORT in .env}"
P_PORT="${PROXY_PORT:?set PROXY_PORT in .env}"
NET_PROXY="${NETWORK_PROXY_CONTAINER_NAME:-raksha_browser_ai_proxy}"

echo "Removing old containers..."
# Primary = CONTAINER_NAME from .env. Network-proxy + legacy names cleaned once.
docker rm -f "${CONTAINER}" "${NET_PROXY}" raksha_browser_proxy raksha_broswer_proxy 2>/dev/null || true

echo "Starting updated stack..."
docker compose up -d

echo "Waiting for ${CONTAINER}..."
sleep 5

if docker exec "${CONTAINER}" wget -q -O /dev/null "http://127.0.0.1:${PORT}/health" 2>/dev/null; then
  echo "OK: ${CONTAINER} healthy on :${PORT}"
else
  echo "WARN: health check failed — run: docker logs ${CONTAINER} --tail 50"
fi

echo "Done."
echo "  Raksha dashboard: http://localhost:${PORT}"
echo "  Lab browser proxy: http://localhost:${P_PORT} (profile: network-proxy)"
