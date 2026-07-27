#!/usr/bin/env bash
# Step 4: Remove old NVIDIA and CUDA packages before a clean reinstall.

set -euo pipefail

echo "=== Removing Old NVIDIA/CUDA Packages ==="

echo "Stopping NVIDIA services if present..."
sudo systemctl stop nvidia-fabricmanager 2>/dev/null || true
sudo systemctl stop nvidia-persistenced 2>/dev/null || true

echo "Purging packages..."
sudo apt-get remove --purge -y \
  'nvidia-driver-*' 'nvidia-open-*' 'nvidia-utils-*' \
  'nvidia-fabricmanager-*' nvidia-container-toolkit \
  nvidia-container-runtime nvidia-docker2 \
  'libnvidia-*' 'cuda-*' 'libcuda*' nvlsm 2>/dev/null || true

sudo apt-get autoremove -y
sudo apt-get autoclean

echo "Cleaning repository files..."
sudo rm -f /etc/apt/sources.list.d/cuda*.list
sudo rm -f /etc/apt/sources.list.d/nvidia*.list
sudo apt-get update

echo
echo "Remaining nvidia/cuda packages:"
dpkg -l | grep -E "nvidia|cuda" | grep -v "^rc" || echo "Clean - no packages remaining"

echo "=== Removal complete ==="
