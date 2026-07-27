#!/usr/bin/env bash
# Step 1: Detect current NVIDIA GPU state and host details.

set -euo pipefail

echo "=== GPU Hardware Detection ==="

if command -v nvidia-smi >/dev/null 2>&1; then
  echo "Querying nvidia-smi..."
  nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader 2>/dev/null || true
else
  echo "nvidia-smi not available; falling back to lspci"
  lspci | grep -i nvidia || true
fi

echo
echo "Current nvidia-smi output:"
nvidia-smi 2>/dev/null || echo "nvidia-smi not available"

echo
echo "CUDA compiler:"
nvcc --version 2>/dev/null || echo "nvcc not available"

echo
echo "Installed nvidia/cuda packages:"
dpkg -l | grep -E "nvidia|cuda" | grep -v "^rc" || echo "None"

echo
echo "NVSwitch topology:"
nvidia-smi topo -m 2>/dev/null | grep -i nvswitch || echo "No NVSwitch reported"

echo
echo "Kernel: $(uname -r)"
echo "OS:"
lsb_release -a 2>/dev/null || cat /etc/os-release
