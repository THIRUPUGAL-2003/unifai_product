#!/bin/sh
# Wire AI Guard Bot → Ollama. All hosts come from .env (CONTAINER_NAME / OLLAMA_URL).
# Run:  sh tools/dev/server_browser_ai_ollama_fix.sh
if [ -f .env ]; then
  set -a
  . ./.env
  set +a
elif [ -f "$(dirname "$0")/../../.env" ]; then
  set -a
  . "$(dirname "$0")/../../.env"
  set +a
fi

OLLAMA_URL="${BROWSER_AI_OLLAMA_URL:-${OLLAMA_URL:?set OLLAMA_URL (or BROWSER_AI_OLLAMA_URL) in .env}}"
TARGET_CONTAINER="${CONTAINER_NAME:?set CONTAINER_NAME in .env}"

echo "=== 1) Ollama health (host) ==="
if ! curl -sf "${OLLAMA_URL}/api/tags" >/dev/null; then
  echo "FAIL: Ollama not reachable at ${OLLAMA_URL}"
  echo "      Fix 1Panel Ollama app first, then re-run."
  exit 1
fi
echo "OK: Ollama at ${OLLAMA_URL}"

echo ""
echo "=== 2) Find Raksha backend container (${TARGET_CONTAINER}) ==="
CID="$(docker ps -q -f "name=^/${TARGET_CONTAINER}$" 2>/dev/null | head -1)"
if [ -z "$CID" ]; then
  CID="$(docker ps -q -f "name=${TARGET_CONTAINER}" 2>/dev/null | head -1)"
fi

if [ -z "$CID" ]; then
  echo "WARN: Container ${TARGET_CONTAINER} not running. Listing:"
  docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}' | grep -E "${APP_PORT:?set APP_PORT in .env}|raksha|raksha" || docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
  echo ""
  echo "Set CONTAINER_NAME in .env and start: docker compose up -d"
  exit 1
fi

CNAME="$(docker inspect -f '{{.Name}}' "$CID" | sed 's#^/##')"
echo "Found: $CNAME ($CID)"

echo ""
echo "=== 3) Test Ollama from inside backend container ==="
# Same URL from .env — must be reachable from inside the container (e.g. host.docker.internal:…).
DOCKER_OLLAMA="${OLLAMA_URL}"
if docker exec "$CID" wget -q -O - "${DOCKER_OLLAMA}/api/tags" 2>/dev/null | head -c 120; then
  echo ""
  echo "OK: container can reach Ollama at ${DOCKER_OLLAMA}"
  OLLAMA_FOR_CONTAINER="$DOCKER_OLLAMA"
else
  echo "FAIL: backend container cannot reach Ollama at ${DOCKER_OLLAMA}"
  echo "Set BROWSER_AI_OLLAMA_URL / OLLAMA_URL to a URL reachable from the container (see .env)."
  exit 1
fi

echo ""
echo "=== 4) Set env + restart backend ==="
COMPOSE_DIR=""
for d in /opt/1panel/apps/raksha_tech/raksha_tech /opt/1panel/apps/raksha/raksha "$(dirname "$0")/../.."; do
  if [ -f "$d/docker-compose.yml" ]; then
    COMPOSE_DIR="$d"
    break
  fi
done

if [ -n "$COMPOSE_DIR" ] && [ -f "$COMPOSE_DIR/.env" ]; then
  echo "Patching $COMPOSE_DIR/.env"
  if grep -q '^BROWSER_AI_OLLAMA_URL=' "$COMPOSE_DIR/.env" 2>/dev/null; then
    sed -i "s|^BROWSER_AI_OLLAMA_URL=.*|BROWSER_AI_OLLAMA_URL=${OLLAMA_FOR_CONTAINER}|" "$COMPOSE_DIR/.env"
  else
    echo "BROWSER_AI_OLLAMA_URL=${OLLAMA_FOR_CONTAINER}" >>"$COMPOSE_DIR/.env"
  fi
  (cd "$COMPOSE_DIR" && docker compose up -d --force-recreate)
  echo "Restarted via docker compose in $COMPOSE_DIR"
else
  echo "No compose .env found — recreate container with env (manual 1Panel step):"
  echo "  BROWSER_AI_OLLAMA_URL=${OLLAMA_FOR_CONTAINER}"
  docker restart "$CID"
  echo "Restarted $CNAME (env must be set in 1Panel for persistent fix)."
fi

sleep 4
echo ""
echo "=== 5) Backend health ==="
APP_PORT="${APP_PORT:?set APP_PORT in .env}"
if docker exec "$CID" wget -q -O /dev/null "http://127.0.0.1:${APP_PORT}/health" 2>/dev/null; then
  echo "OK: Raksha healthy on :${APP_PORT}"
else
  echo "WARN: health check failed — docker logs $CNAME --tail 30"
fi

echo ""
echo "Done. Test in dashboard: Browser AI → Rules → AI Guard Bot → Test"
