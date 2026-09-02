#!/usr/bin/env bash
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

# Offline CI-safe gate for the nvidia-local-serving Kubernetes profile.
# Does not contact Shadeform or require cloud authentication.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

INTENT="$ROOT/deploy/kubernetes/intents/shadeform-nvidia-local-models.example.yaml"
OVERLAY="$ROOT/deploy/kubernetes/overlays/nvidia-local-serving"
OUT="$(mktemp -d "${TMPDIR:-/tmp}/sr-k8s-nvidia-XXXXXX")"
cleanup() { rm -rf "$OUT"; }
trap cleanup EXIT

echo "==> blueprint render"
go run ./cmd/metrum-genai-smartrouterctl blueprint render --intent "$INTENT" --out "$OUT"

echo "==> assert local upstreams and no cloud URLs"
CFG="$OUT/config.yaml"
grep -q 'svc.cluster.local' "$CFG"
grep -q 'local-tiny' "$CFG"
grep -q 'local-small-chat' "$CFG"
grep -q 'local-small-coder' "$CFG"
grep -q 'LOCAL_VLLM_API_KEY' "$CFG"
if grep -E 'openrouter\.ai|api\.openai\.com|api\.anthropic\.com' "$CFG"; then
  echo "cloud upstream URLs are forbidden in this profile" >&2
  exit 1
fi

echo "==> assert kv cache omitted"
grep -q 'omitted' "$OUT/architecture.md"
grep -q 'kv_cache_enabled: false' "$OUT/inventory.yaml"
grep -q 'router_requests_gpu: false' "$OUT/inventory.yaml"

echo "==> assert serving requests GPUs"
grep -q 'nvidia.com/gpu' "$OUT/overlays/nvidia-local-serving/serving/vllm-tiny-deployment.yaml"
grep -q 'nvidia.com/gpu' "$OUT/overlays/nvidia-local-serving/serving/vllm-chat-deployment.yaml"
grep -q 'nvidia.com/gpu' "$OUT/overlays/nvidia-local-serving/serving/vllm-coder-deployment.yaml"

echo "==> assert helm chart and operator scaffold"
test -f "$OUT/charts/smart-llmrouter/Chart.yaml"
test -f "$OUT/charts/smart-llmrouter/values.yaml"
test -f "$OUT/charts/smart-llmrouter/templates/deployment.yaml"
grep -q 'nvidia-compatible' "$OUT/charts/smart-llmrouter/values.yaml"
test -f "$OUT/operator/crds/smartrouter.yaml"
test -f "$OUT/operator/crds/modelgroup.yaml"
test -f "$OUT/operator/crds/callertoken.yaml"
grep -q 'helm_chart_emitted: true' "$OUT/inventory.yaml"
grep -q 'operator_emitted: true' "$OUT/inventory.yaml"

echo "==> package unit tests"
go test ./internal/smartrouterctl ./cmd/metrum-genai-smartrouterctl -count=1

echo "==> kubectl kustomize overlay"
if command -v kubectl >/dev/null 2>&1; then
  RENDERED="$(kubectl kustomize "$OVERLAY")"
  echo "$RENDERED" | grep -q 'name: vllm-tiny'
  echo "$RENDERED" | grep -q 'name: vllm-chat'
  echo "$RENDERED" | grep -q 'name: vllm-coder'
  echo "$RENDERED" | grep -q 'nvidia.com/gpu'
  echo "$RENDERED" | python3 -c '
import sys
docs = sys.stdin.read().split("---\n")
router_gpu = False
serving_gpu = False
for doc in docs:
    if "kind: Deployment" not in doc:
        continue
    if "name: smart-llmrouter" in doc and "nvidia.com/gpu" in doc:
        if "app.kubernetes.io/component: local-serving" not in doc:
            router_gpu = True
    if "app.kubernetes.io/component: local-serving" in doc and "nvidia.com/gpu" in doc:
        serving_gpu = True
if router_gpu:
    raise SystemExit("router deployment must not request nvidia.com/gpu")
if not serving_gpu:
    raise SystemExit("serving deployments must request nvidia.com/gpu")
print("kustomize GPU ownership checks passed")
'
else
  echo "kubectl not found; skipping kustomize render check" >&2
fi

echo "OK test-k8s-nvidia-local-serving"
