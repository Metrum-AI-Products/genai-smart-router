# Shadeform NVIDIA local-serving e2e

**Audience:** operators with a Shadeform GPU Kubernetes instance  
**Date:** 2026-08-31  
**Profile:** `nvidia-local-serving`  
**KV cache:** off (LMCache / Mooncake not installed)

This runbook validates GenAI Smart Router against **in-cluster** OpenAI-compatible
models on a Shadeform GPU node. Cloud/cluster authentication is a prerequisite.
Do not run Shadeform login, kubeconfig bootstrap, or credential copy as part of
the numbered steps below.

## Preconditions

1. `kubectl` already targets the Shadeform cluster (`kubectl get nodes` succeeds).
2. Nodes expose allocatable `nvidia.com/gpu` **or** you will install the NVIDIA
   GPU Operator at floor `v26.7.0` using
   [`deploy/kubernetes/overlays/nvidia-local-serving/gpu-operator-values.yaml`](../deploy/kubernetes/overlays/nvidia-local-serving/gpu-operator-values.yaml).
3. When the Shadeform node image already owns drivers/toolkit, set
   `driver.enabled=false` and `toolkit.enabled=false` in those values (same rule
   as EKS accelerated AMIs).
4. A Metrum-issued `license.json` and a mode-`0600` runtime Secret plan exist.
5. Router image is loaded or pushed to a registry the cluster can pull.

If `kubectl get nodes` fails, stop with: authenticate first, then retry.

## Offline dry-run (CI-safe)

```bash
make test-k8s-nvidia-local-serving
```

This renders the stack intent, asserts local Service DNS in `config.yaml`,
asserts the router Deployment does not request GPUs, and runs
`kubectl kustomize` on the overlay. It does not contact Shadeform.

## Live steps

### 1. Confirm GPU capacity

```bash
kubectl get nodes -o json | jq -r '
  .items[] | "\(.metadata.name) allocatable_gpu=\(.status.allocatable["nvidia.com/gpu"] // "0")"
'
```

### 2. Install NVIDIA GPU Operator (when AMI does not own drivers)

```bash
helm repo add nvidia https://helm.ngc.nvidia.com/nvidia
helm repo update
helm upgrade --install gpu-operator nvidia/gpu-operator \
  --namespace gpu-operator --create-namespace \
  --version v26.7.0 \
  -f deploy/kubernetes/overlays/nvidia-local-serving/gpu-operator-values.yaml
kubectl -n gpu-operator rollout status deploy/gpu-operator
```

Do **not** install LMCache or Mooncake for this test.

### 3. Render blueprint and prepare runtime Secret

```bash
metrum-genai-smartrouterctl blueprint render \
  --intent deploy/kubernetes/intents/shadeform-nvidia-local-models.example.yaml \
  --out /tmp/shadeform-blueprint

# Start from generated config; replace the placeholder caller hash:
metrum-genai-smartrouterctl callers generate \
  --owner-user local-operator --project local --env dev \
  --allow local-chat,local-coder \
  --token-out /tmp/shadeform-caller.token \
  --config /tmp/shadeform-blueprint/config.yaml --write

# Build Secret locally (never commit). Keys: config.yaml, env.json, license.json.
# env.json must define LOCAL_VLLM_API_KEY (even a local opaque value).
```

Confirm generated providers use only `*.svc.cluster.local` URLs.

### 4. Apply serving + router overlay

```bash
kubectl apply -k deploy/kubernetes/overlays/nvidia-local-serving
# Mount the Secret created above into the router Deployment per base/secret.example.yaml.
kubectl -n smart-llmrouter rollout status deploy/smart-llmrouter
kubectl -n smart-llmrouter rollout status deploy/vllm-chat
kubectl -n smart-llmrouter rollout status deploy/vllm-coder
```

### 5. Direct serving smokes

```bash
kubectl -n smart-llmrouter port-forward svc/vllm-chat 18000:8000 &
curl -fsS http://127.0.0.1:18000/v1/models
curl -fsS http://127.0.0.1:18000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"local-chat","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16}'

kubectl -n smart-llmrouter port-forward svc/vllm-coder 18001:8000 &
curl -fsS http://127.0.0.1:18001/v1/models
curl -fsS http://127.0.0.1:18001/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"local-coder","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16}'
```

### 6. Router smokes (local upstreams only)

```bash
kubectl -n smart-llmrouter port-forward svc/smart-llmrouter 18080:80 &
TOKEN=$(cat /tmp/shadeform-caller.token)
curl -fsS http://127.0.0.1:18080/readyz
curl -fsS -H "Authorization: Bearer ${TOKEN}" http://127.0.0.1:18080/v1/models
curl -fsS -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  http://127.0.0.1:18080/v1/chat/completions \
  -d '{"model":"local-chat","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":false}'
curl -fsS -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" \
  http://127.0.0.1:18080/v1/chat/completions \
  -d '{"model":"local-coder","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":false}'
```

Record only safe scalars: HTTP status, model group, request id, selected
provider/model names. Do not retain prompts, tokens, or Secret contents.

### 7. SQLite backup / restore CLI round-trip

Stop or drain the router so SQLite is exclusive, then:

```bash
metrum-genai-smartrouterctl usage backup \
  --config /tmp/shadeform-blueprint/config.yaml \
  --out /tmp/usage-backup.sqlite \
  --confirm-offline

metrum-genai-smartrouterctl usage restore \
  --config /tmp/shadeform-blueprint/config.yaml \
  --from /tmp/usage-backup.sqlite \
  --confirm-offline

# Then run router-migrate --action=verify-serving before serving again.
```

## Pass criteria

| Check | Expected |
|---|---|
| GPU Operator / allocatable GPUs | At least one `nvidia.com/gpu` |
| Serving Deployments | `vllm-chat` and `vllm-coder` Ready |
| Router Deployment | Ready, **no** `nvidia.com/gpu` request |
| Router config providers | Only cluster Service DNS |
| `/v1/models` | `local-chat` and `local-coder` only for the test caller |
| Chat smokes | HTTP 2xx for both groups |
| KV cache | Not installed |
| Usage backup | `integrity_ok: true` |

## Cleanup

```bash
kubectl delete -k deploy/kubernetes/overlays/nvidia-local-serving
# Optionally remove gpu-operator release if it was installed only for this test.
rm -f /tmp/shadeform-caller.token
```

Do not commit kubeconfigs, Shadeform API keys, runtime Secrets, or token files.
