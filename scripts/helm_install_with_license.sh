#!/usr/bin/env bash
# Issue one operator-local license, atomically refresh the runtime Secret, then install Helm.
# The signer comes from the normal protected file path or an operator Secrets Manager reference;
# neither key material nor its value enters Helm or Kubernetes.
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: helm_install_with_license.sh \
  --kubeconfig PATH --namespace NAME --release NAME --chart PATH \
  --entitlement PATH --valid-for DURATION \
  --config PATH --env-file PATH --image-repository REPOSITORY --image-tag TAG \
  [--license-key PATH] [--license-key-secret-id ID] [--runtime-secret NAME]

The entitlement must be commercially approved. A local LICENSE_SIGNING_KEY_FILE
takes precedence. Otherwise AWS Secrets Manager resolves the configured signer ID.
EOF
  exit 2
}

KUBECTL="${KUBECTL:-kubectl}"
HELM="${HELM:-helm}"
LICENSE_CLI="${LICENSE_CLI:-metrum-genai-smartrouter-license}"
AWS_CLI="${AWS_CLI:-aws}"
KUBECONFIG_PATH=""
NAMESPACE=""
RELEASE=""
CHART=""
ENTITLEMENT=""
LICENSE_KEY="${LICENSE_SIGNING_KEY_FILE:-}"
LICENSE_KEY_SECRET_ID="${LICENSE_SIGNING_KEY_SECRET_ID:-smartrouter/license/signing/metrum-license-ed25519-2026-06-prod}"
VALID_FOR=""
CONFIG=""
ENV_FILE=""
IMAGE_REPOSITORY=""
IMAGE_TAG=""
RUNTIME_SECRET="smart-llmrouter-secrets"

while (($#)); do
  case "$1" in
    --kubeconfig|--namespace|--release|--chart|--entitlement|--license-key|--license-key-secret-id|--valid-for|--config|--env-file|--image-repository|--image-tag|--runtime-secret)
      (($# >= 2)) || usage
      case "$1" in
        --kubeconfig) KUBECONFIG_PATH="$2" ;;
        --namespace) NAMESPACE="$2" ;;
        --release) RELEASE="$2" ;;
        --chart) CHART="$2" ;;
        --entitlement) ENTITLEMENT="$2" ;;
        --license-key) LICENSE_KEY="$2" ;;
        --license-key-secret-id) LICENSE_KEY_SECRET_ID="$2" ;;
        --valid-for) VALID_FOR="$2" ;;
        --config) CONFIG="$2" ;;
        --env-file) ENV_FILE="$2" ;;
        --image-repository) IMAGE_REPOSITORY="$2" ;;
        --image-tag) IMAGE_TAG="$2" ;;
        --runtime-secret) RUNTIME_SECRET="$2" ;;
      esac
      shift 2
      ;;
    --help|-h) usage ;;
    *) printf 'unsupported argument: %s\n' "$1" >&2; usage ;;
  esac
done

for required in KUBECONFIG_PATH NAMESPACE RELEASE CHART ENTITLEMENT VALID_FOR CONFIG ENV_FILE IMAGE_REPOSITORY IMAGE_TAG; do
  [[ -n "${!required}" ]] || { printf 'missing required argument: %s\n' "$required" >&2; usage; }
done
for file in "$KUBECONFIG_PATH" "$ENTITLEMENT" "$CONFIG" "$ENV_FILE"; do
  [[ -r "$file" ]] || { printf 'required file is unreadable: %s\n' "$file" >&2; exit 1; }
done

workdir="$(mktemp -d "${TMPDIR:-/tmp}/smartrouter-license-helm-XXXXXX")"
trap 'rm -rf "$workdir"' EXIT
license_path="$workdir/license.json"
if [[ -n "$LICENSE_KEY" ]]; then
  [[ -r "$LICENSE_KEY" ]] || { printf 'required file is unreadable: %s\n' "$LICENSE_KEY" >&2; exit 1; }
else
  [[ -n "$LICENSE_KEY_SECRET_ID" ]] || { printf 'license-key-secret-id is required when LICENSE_SIGNING_KEY_FILE is unset\n' >&2; exit 1; }
  LICENSE_KEY="$workdir/signing.key"
  "$AWS_CLI" secretsmanager get-secret-value \
    --secret-id "$LICENSE_KEY_SECRET_ID" \
    --query SecretString \
    --output text > "$LICENSE_KEY"
  chmod 600 "$LICENSE_KEY"
fi

"$LICENSE_CLI" issue \
  --entitlement "$ENTITLEMENT" \
  --key "$LICENSE_KEY" \
  --valid-for "$VALID_FOR" \
  --out "$license_path"

"$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" create namespace "$NAMESPACE" \
  --dry-run=client -o yaml | "$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" apply -f -

"$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" -n "$NAMESPACE" create secret generic "$RUNTIME_SECRET" \
  --from-file=config.yaml="$CONFIG" \
  --from-file=env.json="$ENV_FILE" \
  --from-file=license.json="$license_path" \
  --dry-run=client -o yaml | "$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" apply -f -

"$HELM" upgrade --install "$RELEASE" "$CHART" \
  --kubeconfig "$KUBECONFIG_PATH" \
  --namespace "$NAMESPACE" \
  --create-namespace \
  --set "image.repository=$IMAGE_REPOSITORY" \
  --set "image.tag=$IMAGE_TAG" \
  --set "config.existingSecretKey=config.yaml" \
  --set "runtimeSecret.name=$RUNTIME_SECRET"
