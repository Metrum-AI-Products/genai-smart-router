#!/usr/bin/env bash
# Issue one operator-local license, atomically refresh the runtime Secret, then install Helm.
# The signing key is referenced by name only and never enters Helm or Kubernetes.
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: helm_install_with_license.sh \
  --kubeconfig PATH --namespace NAME --release NAME --chart PATH \
  --entitlement PATH --valid-for DURATION \
  --config PATH --env-file PATH --image-repository REPOSITORY --image-tag TAG \
  [--license-key PATH] [--runtime-secret NAME]

The entitlement must be commercially approved. The signing-key path defaults to
LICENSE_SIGNING_KEY_FILE, matching the normal operator issuance flow.
EOF
  exit 2
}

KUBECTL="${KUBECTL:-kubectl}"
HELM="${HELM:-helm}"
LICENSE_CLI="${LICENSE_CLI:-metrum-genai-smartrouter-license}"
KUBECONFIG_PATH=""
NAMESPACE=""
RELEASE=""
CHART=""
ENTITLEMENT=""
LICENSE_KEY="${LICENSE_SIGNING_KEY_FILE:-}"
VALID_FOR=""
CONFIG=""
ENV_FILE=""
IMAGE_REPOSITORY=""
IMAGE_TAG=""
RUNTIME_SECRET="smart-llmrouter-secrets"

while (($#)); do
  case "$1" in
    --kubeconfig|--namespace|--release|--chart|--entitlement|--license-key|--valid-for|--config|--env-file|--image-repository|--image-tag|--runtime-secret)
      (($# >= 2)) || usage
      case "$1" in
        --kubeconfig) KUBECONFIG_PATH="$2" ;;
        --namespace) NAMESPACE="$2" ;;
        --release) RELEASE="$2" ;;
        --chart) CHART="$2" ;;
        --entitlement) ENTITLEMENT="$2" ;;
        --license-key) LICENSE_KEY="$2" ;;
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
for file in "$KUBECONFIG_PATH" "$ENTITLEMENT" "$CONFIG" "$ENV_FILE" "$LICENSE_KEY"; do
  [[ -r "$file" ]] || { printf 'required file is unreadable: %s\n' "$file" >&2; exit 1; }
done

workdir="$(mktemp -d "${TMPDIR:-/tmp}/smartrouter-license-helm-XXXXXX")"
trap 'rm -rf "$workdir"' EXIT
license_path="$workdir/license.json"

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
