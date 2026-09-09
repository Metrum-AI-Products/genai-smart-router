#!/usr/bin/env bash
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EXAMPLE_DIR="$ROOT/examples/harbor-algotune-pca"

CASE_ID="${CASE_ID:-}"
if [[ -z "$CASE_ID" ]]; then
  latest="$(find "$EXAMPLE_DIR/generated" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort | tail -n 1 || true)"
  if [[ -n "$latest" ]]; then
    CASE_ID="$(basename "$latest")"
  else
    CASE_ID="case-$(date -u +%Y%m%dT%H%M%SZ)"
  fi
fi

ROUTER_BASE_URL="${ROUTER_BASE_URL:-http://127.0.0.1:18080}"
HARBOR_TASK="${HARBOR_TASK:-aider/polyglot_python_two-bucket}"
AGENTS="${AGENTS:-codex,claude-code}"
MODEL_GROUPS="${MODEL_GROUPS:-default,fast,small,medium,high,big-coder}"
TOKEN_ENV_FILE="${TOKEN_ENV_FILE:-$EXAMPLE_DIR/generated/$CASE_ID/tokens.env}"
HARBOR_ROUTER_TOKEN="${HARBOR_ROUTER_TOKEN:-}"
RUN_DIR="${RUN_DIR:-$EXAMPLE_DIR/runs/$CASE_ID}"
DRY_RUN="${DRY_RUN:-0}"
DISABLE_VERIFICATION="${DISABLE_VERIFICATION:-0}"
HARBOR_ARTIFACTS="${HARBOR_ARTIFACTS:-/app/two_bucket.py}"
EXTRA_INSTRUCTION_PATHS="${EXTRA_INSTRUCTION_PATHS:-$EXAMPLE_DIR/two-bucket-verification.md}"
HARBOR_BIN="${HARBOR_BIN:-harbor}"

if [[ ! -f "$TOKEN_ENV_FILE" && -z "$HARBOR_ROUTER_TOKEN" ]]; then
  echo "Token env file not found: $TOKEN_ENV_FILE" >&2
  echo "Set HARBOR_ROUTER_TOKEN for a reusable Harbor caller, or run ./generate_tokens.sh and register generated callers in the router config first." >&2
  exit 2
fi

if [[ "$DRY_RUN" != "1" ]] && ! command -v "$HARBOR_BIN" >/dev/null 2>&1; then
  echo "harbor CLI not found. Install it with: uv tool install harbor" >&2
  exit 2
fi

if [[ -f "$TOKEN_ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  . "$TOKEN_ENV_FILE"
  set +a
fi

mkdir -p "$RUN_DIR"
chmod 0700 "$RUN_DIR"
RESULTS="$RUN_DIR/results.tsv"
printf 'agent\tmodel_group\tstatus\texit_code\telapsed_seconds\treward\terrors\tjob_result\tlog\n' >"$RESULTS"

IFS=',' read -r -a agent_list <<<"$AGENTS"
IFS=',' read -r -a group_list <<<"$MODEL_GROUPS"

env_name() {
  local raw="$1"
  raw="${raw//-/_}"
  raw="${raw//[^A-Za-z0-9_]/_}"
  printf '%s' "${raw^^}"
}

run_one() {
  local agent="$1"
  local group="$2"
  local token_var="ROUTER_TOKEN_$(env_name "${agent}_${group}")"
  local token="${!token_var:-$HARBOR_ROUTER_TOKEN}"
  local log="$RUN_DIR/${agent}-${group}.log"
  local harbor_env="$RUN_DIR/${agent}-${group}.env"
  local start end elapsed code status reward errors job_result

  if [[ -z "$token" ]]; then
    echo "Missing $token_var in $TOKEN_ENV_FILE and HARBOR_ROUTER_TOKEN is not set" >&2
    return 2
  fi

  echo "Running Harbor task=$HARBOR_TASK agent=$agent model_group=$group"
  umask 077
  if [[ "$agent" == "codex" ]]; then
    {
      printf 'METRUM_ROUTER_KEY=%q\n' "$token"
      printf 'METRUM_ROUTER_BASE_URL=%q\n' "$ROUTER_BASE_URL/v1"
    } >"$harbor_env"
  else
    {
      printf 'ANTHROPIC_BASE_URL=%q\n' "$ROUTER_BASE_URL"
      printf 'ANTHROPIC_AUTH_TOKEN=%q\n' "$token"
    } >"$harbor_env"
  fi
  chmod 0600 "$harbor_env"

  if [[ "$DRY_RUN" == "1" ]]; then
    printf 'DRY RUN: %s run -t %q -m %q --env-file %q %s --n-concurrent 1 --yes\n' "$HARBOR_BIN" "$HARBOR_TASK" "$group" "$harbor_env" "$agent" | tee "$log"
    printf '%s\t%s\tdry-run\t0\t0\t\t\t\t%s\n' "$agent" "$group" "$log" >>"$RESULTS"
    return 0
  fi

  start="$(date +%s)"
  set +e
  artifact_args=()
  IFS=',' read -r -a artifact_list <<<"$HARBOR_ARTIFACTS"
  for artifact in "${artifact_list[@]}"; do
    artifact="$(printf '%s' "$artifact" | xargs)"
    [[ -n "$artifact" ]] || continue
    artifact_args+=(--artifact "$artifact")
  done
  extra_instruction_args=()
  IFS=',' read -r -a extra_instruction_list <<<"$EXTRA_INSTRUCTION_PATHS"
  for extra_instruction in "${extra_instruction_list[@]}"; do
    extra_instruction="$(printf '%s' "$extra_instruction" | xargs)"
    [[ -n "$extra_instruction" ]] || continue
    extra_instruction_args+=(--extra-instruction-path "$extra_instruction")
  done
  if [[ "$agent" == "codex" ]]; then
    harbor_args=()
    if [[ "$DISABLE_VERIFICATION" == "1" ]]; then
      harbor_args+=(--disable-verification)
    fi
    PYTHONPATH="$EXAMPLE_DIR${PYTHONPATH:+:$PYTHONPATH}" \
    "$HARBOR_BIN" run \
      -t "$HARBOR_TASK" \
      -m "$group" \
      --agent-import-path "metrum_codex_agent:MetrumCodex" \
      --env-file "$harbor_env" \
      --n-concurrent 1 \
      --yes \
      "${artifact_args[@]}" \
      "${extra_instruction_args[@]}" \
      "${harbor_args[@]}" >"$log" 2>&1
  else
    harbor_args=()
    if [[ "$DISABLE_VERIFICATION" == "1" ]]; then
      harbor_args+=(--disable-verification)
    fi
    env -u ANTHROPIC_API_KEY \
      "$HARBOR_BIN" run \
        -t "$HARBOR_TASK" \
        -m "$group" \
        -a "$agent" \
        --env-file "$harbor_env" \
        --n-concurrent 1 \
        --yes \
        "${artifact_args[@]}" \
        "${extra_instruction_args[@]}" \
        "${harbor_args[@]}" >"$log" 2>&1
  fi
  code=$?
  set -e
  end="$(date +%s)"
  elapsed=$((end - start))
  reward=""
  errors=""
  job_result=""
  if [[ -f "$log" ]]; then
    job_result="$(awk '/Results written to jobs\// {print $4}' "$log" | tail -n 1)"
  fi
  if [[ -n "$job_result" && -f "$job_result" ]]; then
    read -r reward errors < <(python3 - "$job_result" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as f:
    result = json.load(f)

stats = result.get("stats") or {}
errors = int(stats.get("n_errored_trials") or 0)
rewards = []
for eval_result in (stats.get("evals") or {}).values():
    for metric in eval_result.get("metrics") or []:
        if "mean" in metric and metric["mean"] is not None:
            rewards.append(float(metric["mean"]))
reward = min(rewards) if rewards else 0.0
print(f"{reward:g} {errors}")
PY
)
  fi
  if [[ "$code" == "0" && "${errors:-1}" == "0" && "${reward:-0}" == "1" ]]; then
    status="ok"
  else
    status="failed"
    if [[ "$code" == "0" ]]; then
      code=3
    fi
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$agent" "$group" "$status" "$code" "$elapsed" "$reward" "$errors" "$job_result" "$log" >>"$RESULTS"
  return "$code"
}

failures=0
for agent in "${agent_list[@]}"; do
  agent="$(printf '%s' "$agent" | xargs)"
  [[ -n "$agent" ]] || continue
  for group in "${group_list[@]}"; do
    group="$(printf '%s' "$group" | xargs)"
    [[ -n "$group" ]] || continue
    if ! run_one "$agent" "$group"; then
      failures=$((failures + 1))
    fi
  done
done

echo "Results: $RESULTS"
exit "$failures"
