#!/usr/bin/env bash
# Step 7: Install NVIDIA Container Toolkit from its own repository and configure Docker.
set -euo pipefail

echo "=== Configuring Docker for NVIDIA GPU ==="

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: Docker is not installed."
  echo "Install Docker from the official APT repository (not Snap) before running this step."
  exit 1
fi

echo "Adding NVIDIA Container Toolkit repository..."
sudo mkdir -p /usr/share/keyrings
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
  | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
  | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
  | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

echo "Installing nvidia-container-toolkit..."
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit

echo "Configuring Docker runtime..."
sudo nvidia-ctk runtime configure --runtime=docker

echo "Restarting Docker..."
sudo systemctl restart docker

echo
echo "Docker NVIDIA runtime:"
docker info | grep -i nvidia || echo "Not found (may need logout/login)"

echo
echo "nvidia-container-cli info:"
nvidia-container-cli info 2>/dev/null || echo "Not available"

echo
echo "=== Docker configured ==="
echo "NOTE: You may need to logout/login for Docker to pick up the NVIDIA runtime"
