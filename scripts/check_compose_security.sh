#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

VALID_ENV="$TMP_DIR/compose.env"
cat >"$VALID_ENV" <<'ENV'
SMART_LLMROUTER_VERSION=test-secure-linux-amd64
ROUTER_HOSTNAME=router.example.com
CADDY_EMAIL=ops@example.com
CADDY_HTTP_PORT=8080
CADDY_HTTPS_PORT=8443
POSTGRES_DB=llmrouter
POSTGRES_USER=llmrouter
POSTGRES_PASSWORD=test-secure-compose-password-42
ROUTER_USAGE_DB_DSN=host=postgres port=5432 user=llmrouter password=test-secure-compose-password-42 dbname=llmrouter sslmode=disable TimeZone=UTC
ENV

if env \
  -u SMART_LLMROUTER_VERSION \
  -u ROUTER_USAGE_DB_DSN \
  -u POSTGRES_PASSWORD \
  docker compose --env-file /dev/null -f "$ROOT_DIR/deploy/docker-compose.yml" config >"$TMP_DIR/missing.out" 2>&1; then
  echo "expected docker compose config to fail when required env vars are missing" >&2
  exit 1
fi

DEFAULT_JSON="$TMP_DIR/default.json"
docker compose --env-file "$VALID_ENV" -f "$ROOT_DIR/deploy/docker-compose.yml" config --format json >"$DEFAULT_JSON"

python3 - "$DEFAULT_JSON" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    cfg = json.load(f)

postgres = cfg["services"]["postgres"]
ports = postgres.get("ports") or []
if ports:
    raise SystemExit(f"default postgres service must not publish host ports: {ports!r}")

router_image = cfg["services"]["router"]["image"]
if router_image.endswith(":latest"):
    raise SystemExit(f"router image must not resolve to latest: {router_image}")

env = postgres.get("environment") or {}
if env.get("POSTGRES_PASSWORD") in {"", "llmrouter", "change-this-password", "replace-with-strong-random-db-password"}:
    raise SystemExit("postgres password resolved to an unsafe example value")
PY

LOCALHOST_JSON="$TMP_DIR/localhost.json"
docker compose \
  --env-file "$VALID_ENV" \
  -f "$ROOT_DIR/deploy/docker-compose.yml" \
  -f "$ROOT_DIR/deploy/docker-compose.postgres-localhost.yml" \
  config --format json >"$LOCALHOST_JSON"

python3 - "$LOCALHOST_JSON" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as f:
    cfg = json.load(f)

ports = cfg["services"]["postgres"].get("ports") or []
if len(ports) != 1:
    raise SystemExit(f"expected one explicit localhost postgres port, got {ports!r}")

port = ports[0]
host_ip = port.get("host_ip")
published = str(port.get("published"))
target = str(port.get("target"))
if host_ip != "127.0.0.1" or published != "15432" or target != "5432":
    raise SystemExit(f"postgres override must bind 127.0.0.1:15432->5432, got {port!r}")
PY

echo "compose security checks: ok"
