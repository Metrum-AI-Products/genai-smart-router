#!/usr/bin/env bash
# Step 0: Pre-flight checks for Ubuntu GPU hosts.

set -euo pipefail

echo "=== Pre-flight System Checks ==="

if [ "$(uname -s)" != "Linux" ]; then
  echo "ERROR: This workflow only supports Linux hosts."
  exit 1
fi

if grep -qiE "(microsoft|wsl)" /proc/sys/kernel/osrelease 2>/dev/null; then
  echo "ERROR: WSL detected."
  echo "Install the NVIDIA driver on the Windows host instead of inside WSL."
  exit 1
fi

if ! command -v apt-get >/dev/null 2>&1; then
  echo "ERROR: apt-get not found."
  echo "This public skill currently targets Ubuntu systems."
  exit 1
fi

if [ -r /etc/os-release ]; then
  # shellcheck disable=SC1091
  . /etc/os-release
  if [ "${ID:-}" != "ubuntu" ]; then
    echo "ERROR: Unsupported distro: ${ID:-unknown}"
    echo "This public skill currently targets Ubuntu 22.04/24.04 style installs."
    exit 1
  fi
fi

echo "[OK] Linux host detected: $(uname -s) $(uname -r)"

if command -v nvidia-smi >/dev/null 2>&1; then
  echo "[OK] nvidia-smi found - NVIDIA driver already installed"
  nvidia-smi --query-gpu=name,driver_version,memory.total --format=csv,noheader 2>/dev/null || true
elif command -v lspci >/dev/null 2>&1 && lspci | grep -qi nvidia; then
  echo "[OK] NVIDIA GPU detected via lspci"
  lspci | grep -i nvidia
else
  echo "ERROR: No NVIDIA GPU hardware detected on this system."
  echo "Verify with one of:"
  echo "  nvidia-smi"
  echo "  lspci | grep -i nvidia"
  exit 1
fi

echo "=== Pre-flight checks passed ==="
