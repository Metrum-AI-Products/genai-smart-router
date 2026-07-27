#!/usr/bin/env bash
# Step 6: Install NVSwitch-related services when the platform requires them.

set -euo pipefail

TARGET_DRIVER="${TARGET_DRIVER:-595}"
NVSWITCH_PRESENT="${NVSWITCH_PRESENT:-false}"
NEEDS_NVLSM="${NEEDS_NVLSM:-false}"

if [ "${NVSWITCH_PRESENT}" != "true" ]; then
  echo "=== NVSwitch not detected, skipping Step 6 ==="
  exit 0
fi

echo "=== Installing NVSwitch Services ==="
echo "Driver branch: ${TARGET_DRIVER}"
echo "NVLSM needed: ${NEEDS_NVLSM}"

echo "Installing NVIDIA Fabric Manager..."
if [ "${TARGET_DRIVER}" -ge 590 ] 2>/dev/null; then
  sudo apt-get install -y nvidia-fabricmanager
else
  sudo apt-get install -y "nvidia-fabricmanager-${TARGET_DRIVER}"
fi

if [ "${NEEDS_NVLSM}" = "true" ]; then
  echo "Installing NVLink Subnet Manager..."
  sudo apt-get install -y nvlsm
fi

echo "Starting Fabric Manager..."
sudo systemctl enable --now nvidia-fabricmanager

echo
echo "Fabric Manager status:"
systemctl status nvidia-fabricmanager --no-pager || true

echo "=== NVSwitch services installed ==="
