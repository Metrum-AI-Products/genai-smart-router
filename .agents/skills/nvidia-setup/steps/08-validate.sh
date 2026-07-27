#!/usr/bin/env bash
# Step 8: Validate the installed NVIDIA stack on the host and in Docker.

set -euo pipefail

CUDA_TEST_IMAGE="${CUDA_TEST_IMAGE:-nvidia/cuda:13.3.0-base-ubuntu24.04}"
PYTORCH_IMAGE="${PYTORCH_IMAGE:-}"

echo "=== Validating NVIDIA Stack ==="

echo "1. Driver (nvidia-smi):"
nvidia-smi

echo
echo "2. CUDA (nvcc):"
nvcc --version 2>/dev/null || echo "nvcc not in PATH (may need reboot or shell refresh)"

echo
echo "3. Services:"
echo "  - nvidia-fabricmanager: $(systemctl is-active nvidia-fabricmanager 2>/dev/null || echo 'N/A')"
echo "  - nvidia-persistenced: $(systemctl is-active nvidia-persistenced 2>/dev/null || echo 'N/A')"

echo
echo "4. Docker runtime visibility:"
docker info | grep -i nvidia && echo "  [OK] Docker sees NVIDIA runtime" || echo "  [?] Check Docker config"

echo
echo "5. CUDA container smoke test (${CUDA_TEST_IMAGE}):"
docker run --rm --gpus all "${CUDA_TEST_IMAGE}" nvidia-smi

echo
echo "6. Optional framework validation:"
if [ -n "${PYTORCH_IMAGE}" ]; then
  echo "Running ${PYTORCH_IMAGE}"
  docker run --rm --gpus all --entrypoint python3 \
    "${PYTORCH_IMAGE}" \
    -c "import torch; print(f'CUDA: {torch.cuda.is_available()}'); print(f'GPUs: {torch.cuda.device_count()}')"
else
  echo "Skipped. Set PYTORCH_IMAGE to a current CUDA-matched image to run a framework-level check."
fi

echo
echo "=== Validation complete ==="
