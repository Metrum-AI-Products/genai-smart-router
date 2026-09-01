#!/usr/bin/env bash
# Offline CI-safe gate for the nvidia-llmd-compat Kubernetes profile.
# Does not contact Shadeform, install llm-d, or require cloud authentication.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

INTENT="$ROOT/deploy/kubernetes/intents/shadeform-nvidia-llmd-compat.example.yaml"
OUT="$(mktemp -d "${TMPDIR:-/tmp}/sr-k8s-llmd-XXXXXX")"
cleanup() { rm -rf "$OUT"; }
trap cleanup EXIT

echo "==> blueprint render"
go run ./cmd/metrum-genai-smartrouterctl blueprint render --intent "$INTENT" --out "$OUT"

echo "==> assert local llm-d upstream and no cloud URLs"
CFG="$OUT/config.yaml"
grep -q 'svc.cluster.local' "$CFG"
grep -q 'local-llmd-chat' "$CFG"
grep -q 'LOCAL_VLLM_API_KEY' "$CFG"
if grep -E 'openrouter\.ai|api\.openai\.com|api\.anthropic\.com' "$CFG"; then
  echo "cloud upstream URLs are forbidden in this profile" >&2
  exit 1
fi

echo "==> assert llm-d artifacts"
test -f "$OUT/overlays/nvidia-llmd-compat/llm-d/helm-values.generated.yaml"
test -f "$OUT/overlays/nvidia-llmd-compat/llm-d/install.example.sh"
grep -q 'llm-d-local-epp' "$OUT/overlays/nvidia-llmd-compat/llm-d/install.example.sh"
grep -q 'llm-d-local-epp' "$OUT/config.yaml"
grep -q 'app: vllm-llmd-backend' "$OUT/overlays/nvidia-llmd-compat/llm-d/helm-values.generated.yaml"

echo "==> assert model server requests GPU; no llm-d frontend Deployment"
grep -q 'nvidia.com/gpu' "$OUT/overlays/nvidia-llmd-compat/serving/vllm-llmd-backend-deployment.yaml"
if test -f "$OUT/overlays/nvidia-llmd-compat/serving/llm-d-frontend-deployment.yaml"; then
  echo "llm-d frontend must not emit a vLLM Deployment" >&2
  exit 1
fi

echo "==> assert kv cache omitted"
grep -q 'kv_cache_enabled: false' "$OUT/inventory.yaml"
grep -q 'router_requests_gpu: false' "$OUT/inventory.yaml"

echo "==> package unit tests"
go test ./internal/smartrouterctl ./cmd/metrum-genai-smartrouterctl -count=1 -run 'TestRenderBlueprintNvidia|TestNvidia|TestLoadIntent|TestRenderBlueprintNvidiaLLMD'

echo "OK test-k8s-nvidia-llmd-compat"
