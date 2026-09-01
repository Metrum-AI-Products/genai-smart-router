#!/usr/bin/env bash
# Issue an operator-local self-managed license, refresh the runtime Secret, then install Helm.
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: helm_install_with_license.sh \
  --kubeconfig PATH --namespace NAME --release NAME --chart PATH \
  --entitlement PATH --valid-for DURATION \
  --config PATH --env-file PATH --image-repository REPOSITORY --image-tag TAG \
  [--license-key PATH] [--license-public-key PATH] [--runtime-secret NAME]

The wrapper creates a mode-0600 Ed25519 keypair on first use. It uploads only
the public key and issued license; the private key remains on the operator host.
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
LICENSE_KEY="${LICENSE_SIGNING_KEY_FILE:-${HOME}/.local/state/genai-smart-router/license.key}"
LICENSE_PUBLIC_KEY="${LICENSE_SIGNING_PUBLIC_KEY_FILE:-}"
VALID_FOR=""
CONFIG=""
ENV_FILE=""
IMAGE_REPOSITORY=""
IMAGE_TAG=""
RUNTIME_SECRET="smart-llmrouter-secrets"

while (($#)); do
  case "$1" in
    --kubeconfig|--namespace|--release|--chart|--entitlement|--license-key|--license-public-key|--valid-for|--config|--env-file|--image-repository|--image-tag|--runtime-secret)
      (($# >= 2)) || usage
      case "$1" in
        --kubeconfig) KUBECONFIG_PATH="$2" ;;
        --namespace) NAMESPACE="$2" ;;
        --release) RELEASE="$2" ;;
        --chart) CHART="$2" ;;
        --entitlement) ENTITLEMENT="$2" ;;
        --license-key) LICENSE_KEY="$2" ;;
        --license-public-key) LICENSE_PUBLIC_KEY="$2" ;;
        --valid-for) VALID_FOR="$2" ;;
        --config) CONFIG="$2" ;;
        --env-file) ENV_FILE="$2" ;;
        --image-repository) IMAGE_REPOSITORY="$2" ;;
        --image-tag) IMAGE_TAG="$2" ;;
        --runtime-secret) RUNTIME_SECRET="$2" ;;
      esac
      shift 2 ;;
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
if [[ -z "$LICENSE_PUBLIC_KEY" ]]; then LICENSE_PUBLIC_KEY="${LICENSE_KEY}.pub"; fi
if [[ ! -e "$LICENSE_KEY" && ! -e "$LICENSE_PUBLIC_KEY" ]]; then
  "$LICENSE_CLI" generate-keypair --private-key-out "$LICENSE_KEY" --public-key-out "$LICENSE_PUBLIC_KEY"
elif [[ ! -r "$LICENSE_KEY" || ! -r "$LICENSE_PUBLIC_KEY" ]]; then
  printf 'license keypair is incomplete or unreadable; provide both --license-key and --license-public-key\n' >&2
  exit 1
fi

workdir="$(mktemp -d "${TMPDIR:-/tmp}/smartrouter-license-helm-XXXXXX")"
trap 'rm -rf "$workdir"' EXIT
license_path="$workdir/license.json"

"$LICENSE_CLI" issue \
  --entitlement "$ENTITLEMENT" \
  --key "$LICENSE_KEY" \
  --public-key "$LICENSE_PUBLIC_KEY" \
  --allow-unknown-runtime-key \
  --valid-for "$VALID_FOR" \
  --out "$license_path"

"$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" create namespace "$NAMESPACE" --dry-run=client -o yaml | "$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" apply -f -
"$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" -n "$NAMESPACE" create secret generic "$RUNTIME_SECRET" \
  --from-file=config.yaml="$CONFIG" \
  --from-file=env.json="$ENV_FILE" \
  --from-file=license.json="$license_path" \
  --from-file=license.pub="$LICENSE_PUBLIC_KEY" \
  --dry-run=client -o yaml | "$KUBECTL" --kubeconfig "$KUBECONFIG_PATH" apply -f -
"$HELM" upgrade --install "$RELEASE" "$CHART" \
  --kubeconfig "$KUBECONFIG_PATH" --namespace "$NAMESPACE" --create-namespace \
  --set "image.repository=$IMAGE_REPOSITORY" --set "image.tag=$IMAGE_TAG" \
  --set "config.existingSecretKey=config.yaml" --set "runtimeSecret.name=$RUNTIME_SECRET"
