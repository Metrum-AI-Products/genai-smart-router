#!/usr/bin/env bash
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EXAMPLE_DIR="$ROOT/examples/harbor-algotune-pca"

CASE_ID="${CASE_ID:-case-$(date -u +%Y%m%dT%H%M%SZ)}"
PROJECT="${PROJECT:-harbor-algotune-pca}"
AGENTS="${AGENTS:-codex,claude-code}"
MODEL_GROUPS="${MODEL_GROUPS:-default,fast,small,medium,high,big-coder}"
OUT_DIR="${OUT_DIR:-$EXAMPLE_DIR/generated/$CASE_ID}"
TOKEN_GEN_BIN="${ROUTER_TOKEN_GEN_BIN:-$ROOT/router-token-gen}"

if [[ ! -x "$TOKEN_GEN_BIN" ]]; then
  if command -v go >/dev/null 2>&1; then
    TOKEN_GEN_BIN=(go run "$ROOT/cmd/metrum-router-token-gen")
  else
    echo "router-token-gen not found at $TOKEN_GEN_BIN and go is not available" >&2
    exit 2
  fi
else
  TOKEN_GEN_BIN=("$TOKEN_GEN_BIN")
fi

mkdir -p "$OUT_DIR"
TOKENS_ENV="$OUT_DIR/tokens.env"
CALLERS_YAML="$OUT_DIR/callers.yaml"
MANIFEST="$OUT_DIR/manifest.tsv"

umask 077
: >"$TOKENS_ENV"
: >"$CALLERS_YAML"
printf 'agent\tmodel_group\ttoken_id\tenv_var\n' >"$MANIFEST"

IFS=',' read -r -a agent_list <<<"$AGENTS"
IFS=',' read -r -a group_list <<<"$MODEL_GROUPS"

env_name() {
  local raw="$1"
  raw="${raw//-/_}"
  raw="${raw//[^A-Za-z0-9_]/_}"
  printf '%s' "${raw^^}"
}

for agent in "${agent_list[@]}"; do
  agent="$(printf '%s' "$agent" | xargs)"
  [[ -n "$agent" ]] || continue
  for group in "${group_list[@]}"; do
    group="$(printf '%s' "$group" | xargs)"
    [[ -n "$group" ]] || continue
    user="${agent}-${group}"
    key="${CASE_ID}-${agent}-${group}"
    token_file="$OUT_DIR/${agent}-${group}.yaml"
    "${TOKEN_GEN_BIN[@]}" generate \
      --user "$user" \
      --project "$PROJECT" \
      --env "$CASE_ID" \
      --key "$key" \
      --allow "$group" \
      --format yaml >"$token_file"

    token="$(uv run python - "$token_file" <<'PY'
from pathlib import Path
import sys
for line in Path(sys.argv[1]).read_text().splitlines():
    if line.startswith("token: "):
        print(line.split(": ", 1)[1].strip("'\""))
        break
PY
)"
    token_id="$(uv run python - "$token_file" <<'PY'
from pathlib import Path
import sys
for line in Path(sys.argv[1]).read_text().splitlines():
    if line.startswith("token_id: "):
        print(line.split(": ", 1)[1].strip("'\""))
        break
PY
)"
    var="ROUTER_TOKEN_$(env_name "${agent}_${group}")"
    printf 'export %s=%q\n' "$var" "$token" >>"$TOKENS_ENV"
    printf '%s\t%s\t%s\t%s\n' "$agent" "$group" "$token_id" "$var" >>"$MANIFEST"

    uv run python - "$token_file" "$CALLERS_YAML" <<'PY'
from pathlib import Path
import sys

src = Path(sys.argv[1]).read_text().splitlines()
dst = Path(sys.argv[2])
capture = False
lines = []
for line in src:
    if line == "callers:":
        capture = True
        continue
    if capture:
        lines.append(line)
if lines:
    if dst.stat().st_size == 0:
        dst.write_text("callers:\n")
    with dst.open("a") as f:
        for line in lines:
            f.write(line + "\n")
PY
  done
done

cat <<EOF
Generated Harbor case-study tokens.

Case ID: $CASE_ID
Directory: $OUT_DIR
Token env file: $TOKENS_ENV
Caller config bundle: $CALLERS_YAML
Manifest: $MANIFEST

Register the caller blocks from callers.yaml in the router config before running the case study.
The token env file contains raw bearer tokens and must not be committed.
EOF
