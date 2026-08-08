#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

if [[ -n "${COMPOSE_E2E_WORKDIR:-}" ]]; then
  WORKDIR="$COMPOSE_E2E_WORKDIR"
  OWNED_WORKDIR=0
else
  WORKDIR="$(mktemp -d)"
  OWNED_WORKDIR=1
fi
E2E_VOLUME_CLEANUP_MARKER="$WORKDIR/.smart-llmrouter-compose-e2e-disposable"
TOKEN="${ROUTER_TOKEN:-rtr_compose_live_e2e_local}"
GROUP="${COMPOSE_E2E_GROUP:-compose-live}"
MODEL="${COMPOSE_E2E_MODEL:-deepseek/deepseek-v4-flash:nitro}"
CODEX_TOOL_GROUP="${COMPOSE_E2E_CODEX_TOOL_GROUP:-agent-tools-smoke}"
CODEX_TOOL_MODEL="${COMPOSE_E2E_CODEX_TOOL_MODEL:-MiniMax-M3}"
CLAUDE_TOOL_GROUP="${COMPOSE_E2E_CLAUDE_TOOL_GROUP:-claude-tools-smoke}"
CLAUDE_TOOL_MODEL="${COMPOSE_E2E_CLAUDE_TOOL_MODEL:-MiniMax-M3}"
HTTP_PORT="${COMPOSE_E2E_HTTP_PORT:-18080}"
BASE_URL="http://127.0.0.1:${HTTP_PORT}"
IMAGE_TAG="${COMPOSE_E2E_IMAGE_TAG:-compose-e2e}"
KEEP_WORKDIR="${KEEP_LIVE_E2E_WORKDIR:-0}"
PERMISSIONS_IMAGE="${COMPOSE_E2E_PERMISSIONS_IMAGE:-alpine:3.20}"
HOST_UID="$(id -u)"
HOST_GID="$(id -g)"

umask 077

# A volume deletion is allowed only for a runner-created disposable test
# directory with an explicit opt-in. The default cleanup intentionally leaves
# volumes behind rather than guessing that an operator's Compose project is
# empty or non-production.
if [[ "$OWNED_WORKDIR" == "1" ]]; then
  : >"$E2E_VOLUME_CLEANUP_MARKER"
fi

scrub_retained_secrets() {
  if command -v docker >/dev/null 2>&1 && [[ -d "$WORKDIR" ]]; then
    docker run --rm --network none \
      --mount "type=bind,source=${WORKDIR},target=/work" \
      "$PERMISSIONS_IMAGE" \
      sh -ceu 'rm -f /work/config/env.json /work/.env' >/dev/null 2>&1 || true
  else
    rm -f "$WORKDIR/config/env.json" "$WORKDIR/.env" >/dev/null 2>&1 || true
  fi
}

reset_config_permissions() {
  if command -v docker >/dev/null 2>&1 && [[ -d "$WORKDIR/config" ]]; then
    docker run --rm --network none \
      --mount "type=bind,source=${WORKDIR}/config,target=/config" \
      "$PERMISSIONS_IMAGE" \
      sh -ceu 'chown -R "$1:$2" /config; chmod -R u+rwX /config' sh "$HOST_UID" "$HOST_GID" >/dev/null 2>&1 || true
  fi
}

reset_runtime_permissions() {
  if command -v docker >/dev/null 2>&1 && [[ -d "$WORKDIR/state" && -d "$WORKDIR/logs" ]]; then
    docker run --rm --network none \
      --mount "type=bind,source=${WORKDIR}/state,target=/state" \
      --mount "type=bind,source=${WORKDIR}/logs,target=/logs" \
      "$PERMISSIONS_IMAGE" \
      sh -ceu 'chown -R "$1:$2" /state /logs; chmod -R u+rwX /state /logs' sh "$HOST_UID" "$HOST_GID" >/dev/null 2>&1 || true
  fi
}

cleanup() {
  if [[ -f "$WORKDIR/docker-compose.yml" ]]; then
    (cd "$WORKDIR" && docker compose down) >/dev/null 2>&1 || true
    if [[ "$OWNED_WORKDIR" == "1" && "${COMPOSE_E2E_ALLOW_VOLUME_CLEANUP:-0}" == "1" && -f "$E2E_VOLUME_CLEANUP_MARKER" ]]; then
      (cd "$WORKDIR" && docker compose down -v) >/dev/null 2>&1 || true
    fi
  fi
  if [[ "$KEEP_WORKDIR" == "1" ]]; then
    scrub_retained_secrets
    reset_config_permissions
    reset_runtime_permissions
    echo "compose live e2e workdir: $WORKDIR" >&2
    return
  fi
  reset_config_permissions
  reset_runtime_permissions
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

load_project_env() {
  python3 - "$ROOT/env.json" <<'PY'
import json
import shlex
import sys
from pathlib import Path

path = Path(sys.argv[1])
if not path.exists():
    raise SystemExit(0)
data = json.loads(path.read_text())
for key, value in data.items():
    if isinstance(value, str):
        print(f"export {key}={shlex.quote(value)}")
PY
}

eval "$(load_project_env)"

if [[ -z "${OPENROUTER_API_KEY:-}" ]]; then
  echo "OPENROUTER_API_KEY must be present in env.json or environment" >&2
  exit 2
fi
if [[ -z "${MINIMAX_API_KEY:-}" ]]; then
  echo "MINIMAX_API_KEY must be present in env.json or environment for the Codex and Claude tool smokes" >&2
  exit 2
fi
if ! command -v codex >/dev/null 2>&1; then
  echo "codex must be installed on the operator or CI machine for Compose E2E tool smokes" >&2
  exit 2
fi
if ! command -v claude >/dev/null 2>&1; then
  echo "claude must be installed on the operator or CI machine for Compose E2E tool smokes" >&2
  exit 2
fi
if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for compose live e2e" >&2
  exit 2
fi

protect_compose_config_for_router() {
  chmod 0700 "$WORKDIR/config" "$WORKDIR/config/scripts"
  chmod 0600 "$WORKDIR/config/config.yaml" "$WORKDIR/config/env.json" "$WORKDIR/config/scripts/router.ts"
  docker run --rm --network none \
    --mount "type=bind,source=${WORKDIR}/config,target=/config" \
    "$PERMISSIONS_IMAGE" \
    sh -ceu 'chown -R 65532:65532 /config; find /config -type d -exec chmod 0700 {} +; find /config -type f -exec chmod 0600 {} +'
}

mkdir -p "$WORKDIR/config/scripts" "$WORKDIR/state" "$WORKDIR/logs"
chmod 0700 "$WORKDIR" "$WORKDIR/config" "$WORKDIR/config/scripts" "$WORKDIR/state" "$WORKDIR/logs"
docker run --rm --network none \
  --mount "type=bind,source=${WORKDIR}/state,target=/state" \
  --mount "type=bind,source=${WORKDIR}/logs,target=/logs" \
  "$PERMISSIONS_IMAGE" \
  sh -ceu 'chown 65532:65532 /state /logs; chmod 0700 /state /logs'
cp deploy/docker-compose.yml "$WORKDIR/docker-compose.yml"
cp deploy/Caddyfile.compose "$WORKDIR/Caddyfile.compose"
cp scripts/router.ts "$WORKDIR/config/scripts/router.ts"

python3 - "$TOKEN" "$WORKDIR" "$GROUP" "$MODEL" "$CODEX_TOOL_GROUP" "$CODEX_TOOL_MODEL" "$CLAUDE_TOOL_GROUP" "$CLAUDE_TOOL_MODEL" <<'PY'
import hashlib
import json
import sys
from pathlib import Path

token, work, group, model, codex_group, codex_model, claude_group, claude_model = sys.argv[1], Path(sys.argv[2]), sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6], sys.argv[7], sys.argv[8]
token_hash = hashlib.sha256(token.encode()).hexdigest()
(work / "config/env.json").write_text(json.dumps({"OPENROUTER_API_KEY": "", "MINIMAX_API_KEY": ""}, indent=2))
(work / "config/config.yaml").write_text(f"""server:
  listen: ":8080"
  cache:
    enabled: true
    max_bytes: 1048576
    default_ttl: 10m
  usage_db:
    enabled: true
    driver: sqlite
    path: /app/state/usage.sqlite
    migration_policy: deployment-job
  logging:
    path: /app/logs/requests.jsonl
state_path: /app/state/router-state.json
providers:
  minimax:
    base_url: https://api.minimax.io/v1
    dialect: openai-responses
    api_key: ${{MINIMAX_API_KEY}}
    api_key_env: MINIMAX_API_KEY
    key_id: minimax-compose-live
  openrouter:
    base_url: https://openrouter.ai/api/v1
    dialect: openai-chat
    api_key: ${{OPENROUTER_API_KEY}}
    api_key_env: OPENROUTER_API_KEY
    key_id: openrouter-compose-live
  minimax_anthropic:
    base_url: https://api.minimax.io/anthropic
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${{MINIMAX_API_KEY}}
    api_key_env: MINIMAX_API_KEY
    key_id: minimax-anthropic-compose-live
models:
  {group}:
    strategy: static
    targets:
      - {{ provider: openrouter, model: "{model}" }}
  {codex_group}:
    strategy: static
    targets:
      - {{ provider: minimax, model: "{codex_model}" }}
  {claude_group}:
    strategy: static
    targets:
      - {{ provider: minimax_anthropic, model: "{claude_model}" }}
callers:
  - id: compose-live
    token_sha256: "{token_hash}"
    token_id: "rtr_compose_live_e2e"
    allow: ["{group}", "{codex_group}", "{claude_group}"]
    rate: {{ rpm: 120, tpm: 200000, concurrent: 4 }}
    quota:
      day: {{ requests: 1000, tokens: 2000000 }}
      month: {{ tokens: 10000000 }}
    key: {{ lifetime_tokens: 10000000, soft_pct: 90, on_exhaust: disable }}
""")
PY

python3 - "$WORKDIR/config/env.json" <<'PY'
import json
import os
import sys
from pathlib import Path

path = Path(sys.argv[1])
data = json.loads(path.read_text())
data["OPENROUTER_API_KEY"] = os.environ["OPENROUTER_API_KEY"]
data["MINIMAX_API_KEY"] = os.environ["MINIMAX_API_KEY"]
path.write_text(json.dumps(data, indent=2) + "\n")
PY
protect_compose_config_for_router

cat >"$WORKDIR/.env" <<ENV
SMART_LLMROUTER_VERSION=${IMAGE_TAG}
ROUTER_HOSTNAME=:80
CADDY_EMAIL=engg@metrum.ai
CADDY_HTTP_PORT=${HTTP_PORT}
CADDY_HTTPS_PORT=18443
ENV
chmod 0600 "$WORKDIR/.env"


docker buildx build --load -t "smart-llmrouter:${IMAGE_TAG}" "$ROOT"
(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-migrate router --version)
(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-migrate router --action=plan --driver=sqlite --db=/app/state/usage.sqlite --json)
(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-migrate router --action=apply --driver=sqlite --db=/app/state/usage.sqlite --json)
(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-migrate router --action=resume --job=historical-usage-validation-v1 --checkpoint-ordinal=0 --driver=sqlite --db=/app/state/usage.sqlite --json)
(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-migrate router --action=verify-serving --driver=sqlite --db=/app/state/usage.sqlite --json)
MIGRATION_STATUS="$(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-migrate router --action=status --driver=sqlite --db=/app/state/usage.sqlite --json)"
python3 - "$MIGRATION_STATUS" <<'PY'
import json
import sys

status = json.loads(sys.argv[1])
if status.get("State") != "current" or status.get("Compatible") is not True:
    raise SystemExit(f"migration status is not serving-compatible: {status!r}")
jobs = status.get("Jobs") or []
if not any(job.get("Key") == "historical-usage-validation-v1" and job.get("State") == "validated" for job in jobs):
    raise SystemExit(f"historical validation job is not validated: {jobs!r}")
PY
(cd "$WORKDIR" && docker compose up -d)

for _ in $(seq 1 120); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS "$BASE_URL/healthz" >/dev/null
curl -fsS "$BASE_URL/v1/models" -H "Authorization: Bearer ${TOKEN}" >/dev/null

CHAT_HEADERS="$WORKDIR/chat.headers"
CHAT_BODY="$WORKDIR/chat.json"
curl -fsS "$BASE_URL/v1/chat/completions" \
  -D "$CHAT_HEADERS" \
  -o "$CHAT_BODY" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"${GROUP}\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with exactly: router compose ok\"}],\"temperature\":0}"
grep -qi "router compose ok" "$CHAT_BODY"
REQUEST_ID="$(python3 - "$CHAT_HEADERS" <<'PY'
from pathlib import Path
import sys

for line in Path(sys.argv[1]).read_text().splitlines():
    if line.lower().startswith("x-request-id:"):
        print(line.split(":", 1)[1].strip())
        break
else:
    raise SystemExit("Chat smoke did not return X-Request-Id")
PY
)"

env -u ANTHROPIC_API_KEY \
  ANTHROPIC_BASE_URL="$BASE_URL" \
  ANTHROPIC_AUTH_TOKEN="$TOKEN" \
  ANTHROPIC_MODEL="$GROUP" \
  timeout 180 claude --bare --print --model "$GROUP" "Reply with exactly: router compose claude ok" \
  | grep -qx "router compose claude ok"

CLAUDE_WORK="$WORKDIR/claude-tool-work"
mkdir -p "$CLAUDE_WORK"
(
  cd "$CLAUDE_WORK"
  env -u ANTHROPIC_API_KEY \
    ANTHROPIC_BASE_URL="$BASE_URL" \
    ANTHROPIC_AUTH_TOKEN="$TOKEN" \
    ANTHROPIC_MODEL="$CLAUDE_TOOL_GROUP" \
    timeout 240 claude --bare --print --model "$CLAUDE_TOOL_GROUP" \
      --permission-mode bypassPermissions \
      --allowedTools "Write,Bash" \
      "Create a file named claude_tool_smoke.txt in the current directory containing exactly claude-tool-ok, then run cat claude_tool_smoke.txt, then finish with the single line claude-tool-ok."
) >"$WORKDIR/claude-tool-smoke.out" 2>"$WORKDIR/claude-tool-smoke.err"
grep -qx "claude-tool-ok" "$CLAUDE_WORK/claude_tool_smoke.txt"
grep -q "claude-tool-ok" "$WORKDIR/claude-tool-smoke.out"

CODEX_WORK="$WORKDIR/codex-work"
mkdir -p "$CODEX_WORK"
METRUM_ROUTER_KEY="$TOKEN" \
  timeout 180 codex exec --ignore-user-config --ephemeral \
    --ignore-rules \
    --skip-git-repo-check \
    -C "$CODEX_WORK" \
    -c "model=\"${GROUP}\"" \
    -c 'model_provider="metrum-router"' \
    -c 'model_providers.metrum-router.name="Metrum Router"' \
    -c "model_providers.metrum-router.base_url=\"${BASE_URL}/v1\"" \
    -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
    -c 'model_providers.metrum-router.wire_api="responses"' \
    "Reply with exactly: router compose codex ok" </dev/null \
  | grep -qi "router compose codex ok"

CODEX_TOOL_WORK="$WORKDIR/codex-tool-work"
mkdir -p "$CODEX_TOOL_WORK"
(
  cd "$CODEX_TOOL_WORK"
  METRUM_ROUTER_KEY="$TOKEN" \
    timeout 240 codex exec --ignore-user-config --ephemeral \
      --ignore-rules \
      --skip-git-repo-check \
      --sandbox workspace-write \
      -C "$CODEX_TOOL_WORK" \
      -c "model=\"${CODEX_TOOL_GROUP}\"" \
      -c 'model_provider="metrum-router"' \
      -c 'model_providers.metrum-router.name="Metrum Router"' \
      -c "model_providers.metrum-router.base_url=\"${BASE_URL}/v1\"" \
      -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
      -c 'model_providers.metrum-router.wire_api="responses"' \
      "Create a file named codex_tool_smoke.txt in the current directory containing exactly codex-tool-ok, then run cat codex_tool_smoke.txt, then finish with the single line codex-tool-ok." </dev/null
) >"$WORKDIR/codex-tool-smoke.out" 2>"$WORKDIR/codex-tool-smoke.err"
grep -qx "codex-tool-ok" "$CODEX_TOOL_WORK/codex_tool_smoke.txt"
grep -q "codex-tool-ok" "$WORKDIR/codex-tool-smoke.out"

(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-usage-report router --driver=sqlite --db=/app/state/usage.sqlite --since=24h --out=/app/logs/usage-e2e.md)
grep -q "$REQUEST_ID" "$WORKDIR/logs/usage-e2e.md"
(cd "$WORKDIR" && docker compose up -d --force-recreate router)
for _ in $(seq 1 120); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS "$BASE_URL/healthz" >/dev/null
(cd "$WORKDIR" && docker compose run --rm --no-deps --entrypoint /app/bin/router-usage-report router --driver=sqlite --db=/app/state/usage.sqlite --since=24h --out=/app/logs/usage-e2e.md)
grep -q "$REQUEST_ID" "$WORKDIR/logs/usage-e2e.md"
python3 - "$WORKDIR" <<'PY'
import os
import stat
import sys
from pathlib import Path

work = Path(sys.argv[1])
for path in (work / "state", work / "logs"):
    if stat.S_IMODE(path.stat().st_mode) & 0o077:
        raise SystemExit(f"{path} has group or other permissions")
for path in (work / "state" / "usage.sqlite", *(work / "state").glob("usage.sqlite-*")):
    mode = stat.S_IMODE(path.stat().st_mode)
    if not path.is_file() or mode & 0o077:
        raise SystemExit(f"{path} must be a private regular SQLite file")
PY
if (cd "$WORKDIR" && docker compose ps --services | grep -qx postgres); then
  echo "SQLite Compose E2E must not start PostgreSQL" >&2
  exit 1
fi

echo "compose live e2e: ok"
