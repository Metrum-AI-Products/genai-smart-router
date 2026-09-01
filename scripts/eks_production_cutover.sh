#!/usr/bin/env bash
# Operator helper for Metrum EKS production cutover steps that require authenticated
# AWS/Kubernetes/DigitalOcean sessions. Does not perform login.
set -euo pipefail

INGRESS_LB="${INGRESS_LB:-llm-api.apps.metrum.ai}"
CNAME_TARGET="${CNAME_TARGET:-llm-api.apps.metrum.ai}"
PRIMARY_HOSTS=(llm-api-engg.metrum.ai llm-api.metrum.ai)

usage() {
  cat <<'EOF'
Usage: eks_production_cutover.sh <command>

Commands:
  preflight          DNS and public /readyz checks (read-only)
  verify-ingress     curl --resolve checks against ingress LB (requires INGRESS_LB_IP)
  dns-instructions   Print DigitalOcean CNAME steps (no mutation)
  compose-standdown  Print Compose stop commands for operator SSH session

Environment:
  INGRESS_LB_IP      Required for verify-ingress
EOF
}

preflight() {
  for host in "${PRIMARY_HOSTS[@]}" "${CNAME_TARGET}"; do
    echo "== ${host} =="
    dig +short "${host}" || true
    code="$(curl -fsS -o /dev/null -w '%{http_code}' "https://${host}/readyz" 2>/dev/null || echo err)"
    echo "/readyz HTTP ${code}"
  done
}

verify_ingress() {
  if [[ -z "${INGRESS_LB_IP:-}" ]]; then
    echo "INGRESS_LB_IP is required" >&2
    exit 2
  fi
  for host in "${PRIMARY_HOSTS[@]}"; do
    echo "== ${host} via ${INGRESS_LB_IP} =="
    curl -fsS --resolve "${host}:443:${INGRESS_LB_IP}" "https://${host}/readyz"
    echo
  done
}

dns_instructions() {
  cat <<EOF
Lower TTL on DigitalOcean records for:
  - llm-api-engg.metrum.ai
  - llm-api.metrum.ai
Wait for the previous TTL to expire, then replace A records with CNAME -> ${CNAME_TARGET}.
Verify with: dig +short llm-api-engg.metrum.ai
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
