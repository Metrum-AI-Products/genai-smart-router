# NVIDIA local-serving overlay
#
# Profile: nvidia-local-serving, L40S fallback matrix (three GPU-owning vLLM Pods).
# KV cache (LMCache / Mooncake): intentionally omitted.
#
# Prerequisites:
# 1. Authenticated k3s kubeconfig with a NetworkPolicy-enforcing CNI.
# 2. At least three allocatable nvidia.com/gpu devices. This overlay does not
#    schedule a 20–40B primary; use a B200/H200 intent for that hardware class.
# 3. NVIDIA GPU Operator at >=v26.7.0. The checked-in gpu-operator-values.yaml
#    disables driver/toolkit for Shadeform CUDA images (AMI-owned). Prefer the
#    blueprint-rendered values file. Never enable DRA with the device plugin.
# 4. Mode-0600 runtime Secret with config.yaml, env.json, and license.json.
#
# Render a fresh blueprint:
#   metrum-genai-smartrouterctl blueprint render \
#     --intent deploy/kubernetes/intents/shadeform-nvidia-local-models.example.yaml \
#     --out /tmp/blueprint
#
# The generated serving overlay is self-contained:
#   kubectl apply -k /tmp/blueprint/overlays/nvidia-local-serving
# Install the generated Helm chart separately for the router and mount config.yaml
# from its runtime Secret; see docs/SHADEFORM_NVIDIA_LOCAL_SERVING_E2E.md.
#
# Dry-run / CI:
#   make test-k8s-nvidia-local-serving
