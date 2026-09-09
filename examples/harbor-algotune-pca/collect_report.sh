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
    echo "CASE_ID is required when no generated case directory exists" >&2
    exit 2
  fi
fi

PROJECT="${PROJECT:-harbor-algotune-pca}"
REPORT_DIR="${REPORT_DIR:-$EXAMPLE_DIR/reports/$CASE_ID}"
REPORT_BIN="${ROUTER_USAGE_REPORT_BIN:-$ROOT/router-usage-report}"
DRIVER="${ROUTER_USAGE_DB_DRIVER:-sqlite}"
DB_PATH="${ROUTER_USAGE_DB_PATH:-usage.sqlite}"
DSN="${ROUTER_USAGE_DB_DSN:-}"
FROM="${FROM:-}"
TO="${TO:-}"
SINCE="${SINCE:-24h}"
case_environment="$(printf '%s' "$CASE_ID" | tr '[:upper:]' '[:lower:]')"
ENVIRONMENT="${CALLER_ENVIRONMENT:-$case_environment}"

if [[ ! -x "$REPORT_BIN" ]]; then
  if command -v go >/dev/null 2>&1; then
    REPORT_BIN=(go run "$ROOT/cmd/router-usage-report")
  else
    echo "router-usage-report not found at $REPORT_BIN and go is not available" >&2
    exit 2
  fi
else
  REPORT_BIN=("$REPORT_BIN")
fi

mkdir -p "$REPORT_DIR"

args=(
  --driver "$DRIVER"
  --caller-project "$PROJECT"
  --caller-environment "$ENVIRONMENT"
  --out "$REPORT_DIR/usage.md"
)
if [[ "$DRIVER" == "postgres" || "$DRIVER" == "postgresql" ]]; then
  args+=(--dsn "$DSN")
else
  args+=(--db "$DB_PATH")
fi
if [[ -n "$FROM" ]]; then
  args+=(--from "$FROM")
  [[ -z "$TO" ]] || args+=(--to "$TO")
else
  args+=(--since "$SINCE")
fi

"${REPORT_BIN[@]}" "${args[@]}"

{
  echo "# Harbor Agentic Coding Case Study"
  echo
  echo "- Case ID: \`$CASE_ID\`"
  echo "- Caller project: \`$PROJECT\`"
  echo "- Caller environment: \`$ENVIRONMENT\`"
  echo "- Usage report: [usage.md](usage.md)"
  if [[ -f "$EXAMPLE_DIR/runs/$CASE_ID/results.tsv" ]]; then
    echo
    echo "## Harbor Run Matrix"
    echo
    echo "| Agent | Model Group | Status | Exit Code | Elapsed Seconds | Reward | Errors | Job Result | Log |"
    echo "|---|---|---|---:|---:|---:|---:|---|---|"
    tail -n +2 "$EXAMPLE_DIR/runs/$CASE_ID/results.tsv" | while IFS=$'\t' read -r agent group status code elapsed reward errors job_result log; do
      rel_log="$log"
      echo "| $agent | $group | $status | $code | $elapsed | $reward | $errors | \`$job_result\` | \`$rel_log\` |"
    done
  fi
  echo
  echo "## Router Usage"
  echo
  cat "$REPORT_DIR/usage.md"
} >"$REPORT_DIR/case-study.md"

echo "Wrote $REPORT_DIR/case-study.md"
