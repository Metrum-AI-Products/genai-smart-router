#!/usr/bin/env bash
# Step 5: Install NVIDIA driver and a pinned CUDA toolkit package line.

set -euo pipefail

TARGET_DRIVER="${TARGET_DRIVER:-595}"
TARGET_CUDA_PKG="${TARGET_CUDA_PKG:-13-3}"
USE_OPEN_MODULES="${USE_OPEN_MODULES:-true}"

echo "=== Installing Driver + CUDA ==="
echo "Driver branch: ${TARGET_DRIVER}"
echo "CUDA package: ${TARGET_CUDA_PKG}"
echo "Use open modules: ${USE_OPEN_MODULES}"

if [ -r /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
fi

if [ "${ID:-}" != "ubuntu" ]; then
  echo "ERROR: Unsupported distro: ${ID:-unknown}"
  echo "This install script currently targets Ubuntu repository paths."
  exit 1
fi

OS_VERSION="$(printf '%s' "${VERSION_ID:-}" | tr -d '.')"
if [ -z "${OS_VERSION}" ]; then
  echo "ERROR: Unable to determine Ubuntu VERSION_ID from /etc/os-release."
  exit 1
fi
ARCH="$(dpkg --print-architecture)"

echo
echo "OS: Ubuntu ${OS_VERSION}, Arch: ${ARCH}"

echo "Adding NVIDIA repository..."
wget -q "https://developer.download.nvidia.com/compute/cuda/repos/ubuntu${OS_VERSION}/${ARCH}/cuda-keyring_1.1-1_all.deb"
sudo dpkg -i cuda-keyring_1.1-1_all.deb
rm -f cuda-keyring_1.1-1_all.deb
sudo apt-get update

echo "Installing driver..."
if [ "${TARGET_DRIVER}" -ge 590 ] 2>/dev/null; then
  echo "Installing Ubuntu 590+ branch pinning package..."
  sudo apt-get install -y --purge "nvidia-driver-pinning-${TARGET_DRIVER}"
  if [ "${USE_OPEN_MODULES}" = "true" ]; then
    sudo apt-get install -y nvidia-open
  else
    sudo apt-get install -y cuda-drivers
  fi
else
  if [ "${USE_OPEN_MODULES}" = "true" ]; then
    sudo apt-get install -y "nvidia-open-${TARGET_DRIVER}"
  else
    sudo apt-get install -y "nvidia-driver-${TARGET_DRIVER}"
  fi
fi

echo "Installing CUDA toolkit..."
if ! sudo apt-get install -y "cuda-toolkit-${TARGET_CUDA_PKG}"; then
  echo "cuda-toolkit-${TARGET_CUDA_PKG} not found; trying cuda-${TARGET_CUDA_PKG}"
  sudo apt-get install -y "cuda-${TARGET_CUDA_PKG}"
fi

echo
echo ">>> REBOOT REQUIRED - continue from Step 6 after reboot <<<"
echo "After reboot, verify with: nvidia-smi"
