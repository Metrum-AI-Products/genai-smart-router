#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

WORKDIR="${COMPOSE_E2E_WORKDIR:-$(mktemp -d)}"
TOKEN="${ROUTER_TOKEN:-rtr_compose_live_e2e_local}"
GROUP="${COMPOSE_E2E_GROUP:-compose-live}"
MODEL="${COMPOSE_E2E_MODEL:-qwen/qwen3.7-max:nitro}"
CODEX_TOOL_GROUP="${COMPOSE_E2E_CODEX_TOOL_GROUP:-agent-tools-smoke}"
CODEX_TOOL_MODEL="${COMPOSE_E2E_CODEX_TOOL_MODEL:-MiniMax-M3}"
CLAUDE_TOOL_GROUP="${COMPOSE_E2E_CLAUDE_TOOL_GROUP:-claude-tools-smoke}"
CLAUDE_TOOL_MODEL="${COMPOSE_E2E_CLAUDE_TOOL_MODEL:-MiniMax-M3}"
HTTP_PORT="${COMPOSE_E2E_HTTP_PORT:-18080}"
BASE_URL="http://127.0.0.1:${HTTP_PORT}"
IMAGE_TAG="${COMPOSE_E2E_IMAGE_TAG:-compose-e2e}"
KEEP_WORKDIR="${KEEP_LIVE_E2E_WORKDIR:-0}"

cleanup() {
  if [[ "$KEEP_WORKDIR" == "1" ]]; then
    echo "compose live e2e workdir: $WORKDIR" >&2
    return
  fi
  if [[ -f "$WORKDIR/docker-compose.yml" ]]; then
    (cd "$WORKDIR" && docker compose down -v) >/dev/null 2>&1 || true
  fi
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

mkdir -p "$WORKDIR/config/scripts" "$WORKDIR/state" "$WORKDIR/logs"
chmod 0755 "$WORKDIR" "$WORKDIR/config" "$WORKDIR/config/scripts"
chmod 0777 "$WORKDIR/state" "$WORKDIR/logs"
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
chmod 0644 "$WORKDIR/config/config.yaml" "$WORKDIR/config/env.json" "$WORKDIR/config/scripts/router.ts"

cat >"$WORKDIR/.env" <<ENV
SMART_LLMROUTER_VERSION=${IMAGE_TAG}
ROUTER_HOSTNAME=:80
CADDY_EMAIL=engg@metrum.ai
CADDY_HTTP_PORT=${HTTP_PORT}
CADDY_HTTPS_PORT=18443
ENV

docker build -t "smart-llmrouter:${IMAGE_TAG}" "$ROOT"
(cd "$WORKDIR" && docker compose up -d)

for _ in $(seq 1 120); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done
curl -fsS "$BASE_URL/healthz" >/dev/null
curl -fsS "$BASE_URL/v1/models" -H "Authorization: Bearer ${TOKEN}" >/dev/null

curl -fsS "$BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer ${TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"model\":\"${GROUP}\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply with exactly: router compose ok\"}],\"temperature\":0}" \
  | grep -qi "router compose ok"

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
      "Create a file named claude_tool_smoke.txt in the current directory containing exactly claude-tool-ok, then run cat claude_tool_smoke.txt, then finish with the single line claude-tool-ok." \
      >"$WORKDIR/claude-tool-smoke.out" 2>"$WORKDIR/claude-tool-smoke.err"
)
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
METRUM_ROUTER_KEY="$TOKEN" \
  timeout 240 codex exec --ignore-user-config --ephemeral \
    --ignore-rules \
    --skip-git-repo-check \
    --dangerously-bypass-approvals-and-sandbox \
    -C "$CODEX_TOOL_WORK" \
    -c "model=\"${CODEX_TOOL_GROUP}\"" \
    -c 'model_provider="metrum-router"' \
    -c 'model_providers.metrum-router.name="Metrum Router"' \
    -c "model_providers.metrum-router.base_url=\"${BASE_URL}/v1\"" \
    -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
    -c 'model_providers.metrum-router.wire_api="responses"' \
    "Create a file named codex_tool_smoke.txt in the current directory containing exactly codex-tool-ok, then run cat codex_tool_smoke.txt, then finish with the single line codex-tool-ok." \
    >"$WORKDIR/codex-tool-smoke.out" 2>"$WORKDIR/codex-tool-smoke.err"
grep -qx "codex-tool-ok" "$CODEX_TOOL_WORK/codex_tool_smoke.txt"
grep -q "codex-tool-ok" "$WORKDIR/codex-tool-smoke.out"

echo "compose live e2e: ok"
