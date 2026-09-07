#!/usr/bin/env bash
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

# Operator helper for EKS production cutover steps that require authenticated
# AWS/Kubernetes/DNS sessions. Does not perform login. All hostnames must be
# supplied via environment variables; there are no live infrastructure defaults.
set -euo pipefail

require_env() {
  local name="$1"
  if [[ -z "${!name:-}" ]]; then
    echo "${name} is required" >&2
    exit 2
  fi
}

primary_hosts() {
  require_env PRIMARY_HOSTS
  # shellcheck disable=SC2206
  PRIMARY_HOSTS_ARR=(${PRIMARY_HOSTS})
  if [[ ${#PRIMARY_HOSTS_ARR[@]} -eq 0 ]]; then
    echo "PRIMARY_HOSTS must list one or more hostnames" >&2
    exit 2
  fi
}

usage() {
  cat <<'EOF'
Usage: eks_production_cutover.sh <command>

Commands:
  preflight          DNS and public /readyz checks (read-only)
  verify-ingress     curl --resolve checks against ingress LB (requires INGRESS_LB_IP)
  dns-instructions   Print DNS CNAME steps (no mutation)
  compose-standdown  Print Compose stop commands for operator SSH session

Environment (required for hostname-dependent commands):
  PRIMARY_HOSTS      Space-separated primary public hostnames
  CNAME_TARGET       CNAME target hostname
  INGRESS_LB         Ingress load-balancer hostname (optional alias of CNAME_TARGET)
  INGRESS_LB_IP      Required for verify-ingress
EOF
}

preflight() {
  primary_hosts
  require_env CNAME_TARGET
  local hosts=("${PRIMARY_HOSTS_ARR[@]}" "${CNAME_TARGET}")
  if [[ -n "${INGRESS_LB:-}" ]]; then
    hosts+=("${INGRESS_LB}")
  fi
  for host in "${hosts[@]}"; do
    echo "== ${host} =="
    dig +short "${host}" || true
    code="$(curl -fsS -o /dev/null -w '%{http_code}' "https://${host}/readyz" 2>/dev/null || echo err)"
    echo "/readyz HTTP ${code}"
  done
}

verify_ingress() {
  primary_hosts
  require_env INGRESS_LB_IP
  for host in "${PRIMARY_HOSTS_ARR[@]}"; do
    echo "== ${host} via ${INGRESS_LB_IP} =="
    curl -fsS --resolve "${host}:443:${INGRESS_LB_IP}" "https://${host}/readyz"
    echo
  done
}

dns_instructions() {
  primary_hosts
  require_env CNAME_TARGET
  cat <<EOF
Lower TTL on DNS records for:
$(printf '  - %s\n' "${PRIMARY_HOSTS_ARR[@]}")
Wait for the previous TTL to expire, then replace A records with CNAME -> ${CNAME_TARGET}.
Verify with: dig +short ${PRIMARY_HOSTS_ARR[0]}
EOF
}

compose_standdown() {
  cat <<'EOF'
On the Compose host (after EKS acceptance):
  cd /opt/smart-llmrouter/compose
  sudo docker compose stop router caddy
If stop protection prevents instance stop, leave the instance running but stopped containers.
Rollback: restore A records, sudo docker compose start router caddy
EOF
}

cmd="${1:-}"
case "${cmd}" in
  preflight) preflight ;;
  verify-ingress) verify_ingress ;;
  dns-instructions) dns_instructions ;;
  compose-standdown) compose_standdown ;;
  -h|--help|"") usage ;;
  *) echo "unknown command: ${cmd}" >&2; usage; exit 2 ;;
esac
