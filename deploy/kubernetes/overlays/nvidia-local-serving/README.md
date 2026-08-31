# NVIDIA local-serving overlay
#
# Profile: nvidia-local-serving (see deploy/kubernetes/intents/shadeform-nvidia-local-models.example.yaml)
# KV cache (LMCache / Mooncake): intentionally omitted
#
# Prerequisites:
# 1. Authenticated kubeconfig (login is never a product CLI step).
# 2. NVIDIA GPU Operator installed with gpu-operator-values.yaml (or AMI-owned drivers).
# 3. Runtime Secret with config.yaml (start from config.example.yaml), env.json, license.json.
#
# Render a fresh blueprint:
#   metrum-genai-smartrouterctl blueprint render \
#     --intent deploy/kubernetes/intents/shadeform-nvidia-local-models.example.yaml \
#     --out /tmp/blueprint
#
# Apply (after GPU Operator and Secret exist):
#   kubectl apply -k deploy/kubernetes/overlays/nvidia-local-serving
#
# Dry-run / CI:
#   make test-k8s-nvidia-local-serving
