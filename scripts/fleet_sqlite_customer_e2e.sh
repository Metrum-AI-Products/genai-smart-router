#!/usr/bin/env bash
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

# Operator-only gated live E2E for SQLite Fleet customer bootstrap.
# Requires protected staging refs and operator IAM; skipped when unset.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="${METRUM_FLEET_BIN_DIR:-${ROOT}/dist/bin}"
FLEETCTL="${BIN_DIR}/metrum-genai-smartrouter-fleetctl"
FLEET_SIGN="${BIN_DIR}/metrum-genai-smartrouter-fleet-sign"

if [[ -z "${FLEET_SQLITE_E2E_PROFILE_REF:-}" || -z "${FLEET_SQLITE_E2E_LICENSE_REF:-}" ]]; then
  echo "skip: set FLEET_SQLITE_E2E_PROFILE_REF and FLEET_SQLITE_E2E_LICENSE_REF for live SQLite customer E2E" >&2
  exit 0
fi
if [[ -z "${FLEET_SQLITE_E2E_CONFIG_FILE:-}" || -z "${FLEET_SQLITE_E2E_ENV_FILE:-}" ]]; then
  echo "skip: set FLEET_SQLITE_E2E_CONFIG_FILE and FLEET_SQLITE_E2E_ENV_FILE" >&2
  exit 0
fi
if [[ -z "${FLEET_SQLITE_E2E_SIGN_KEY:-}" ]]; then
  echo "skip: set FLEET_SQLITE_E2E_SIGN_KEY (mode-0600 lifecycle approval key)" >&2
  exit 0
fi
for bin in "$FLEETCTL" "$FLEET_SIGN"; do
  if [[ ! -x "$bin" ]]; then
    echo "missing packaged binary: $bin (build with make package or set METRUM_FLEET_BIN_DIR)" >&2
    exit 1
  fi
done

CUSTOMER_ID="${FLEET_SQLITE_E2E_CUSTOMER_ID:-auto-e2e-$(date -u +%Y%m%d%H%M%S)}"
TOKEN_OUT="${FLEET_SQLITE_E2E_TOKEN_OUT:-${HOME}/.local/share/metrum-fleet/${CUSTOMER_ID}/CALLER_TOKEN_E2E.txt}"
MODEL="${FLEET_SQLITE_E2E_MODEL:-high}"
export METRUM_FLEET_BIN_DIR="$BIN_DIR"

echo "live SQLite customer E2E customer_id=${CUSTOMER_ID}"

"$FLEETCTL" customer bootstrap \
  --customer-id "$CUSTOMER_ID" \
  --profile-ref "$FLEET_SQLITE_E2E_PROFILE_REF" \
  --license-ref "$FLEET_SQLITE_E2E_LICENSE_REF" \
  --config-file "$FLEET_SQLITE_E2E_CONFIG_FILE" \
  --env-file "$FLEET_SQLITE_E2E_ENV_FILE" \
  --rewrite-paths fleet-eks \
  --sign-with-key "$FLEET_SQLITE_E2E_SIGN_KEY" \
  --owner-user "${FLEET_SQLITE_E2E_OWNER_USER:-e2e-admin}" \
  --project "${FLEET_SQLITE_E2E_PROJECT:-e2e}" \
  --token-out "$TOKEN_OUT" \
  --model "$MODEL"

HOST="https://${CUSTOMER_ID}.apps.example.test"
curl -fsS "${HOST}/readyz" >/dev/null
MODELS=$(curl -fsS -H "Authorization: Bearer $(tr -d '\n' <"$TOKEN_OUT")" "${HOST}/v1/models")
echo "$MODELS" | grep -q '"data"'
METRICS_CODE=$(curl -s -o /dev/null -w '%{http_code}' "${HOST}/metrics")
if [[ "$METRICS_CODE" != "403" ]]; then
  echo "expected /metrics 403 for ordinary caller, got ${METRICS_CODE}" >&2
  exit 1
fi

echo "live SQLite customer E2E passed: customer_id=${CUSTOMER_ID} readyz=200 metrics=${METRICS_CODE}"

if [[ "${FLEET_SQLITE_E2E_SKIP_DELETE:-}" != "1" ]]; then
  if [[ -z "${FLEET_SQLITE_E2E_DELETE_CONFIRM:-}" ]]; then
    echo "note: set FLEET_SQLITE_E2E_DELETE_CONFIRM for signed cleanup" >&2
  else
    "$FLEETCTL" customer delete \
      --customer-id "$CUSTOMER_ID" \
      --confirm-file "$FLEET_SQLITE_E2E_DELETE_CONFIRM"
  fi
fi
