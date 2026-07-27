---
name: nvidia-setup
description: Use this skill when installing or repairing NVIDIA GPU drivers, CUDA, Fabric Manager, or Docker GPU runtime on Ubuntu Linux servers. Trigger when the user asks to set up a GPU host, clean up an old NVIDIA stack, prepare a server for CUDA or vLLM workloads, or validate that containers can see GPUs.
---

# NVIDIA GPU Driver + CUDA Setup

Bare-metal or VM Ubuntu hosts that own NVIDIA GPUs. Do **not** use for WSL2, macOS, Windows, non-Ubuntu distros, or containers that do not own the host GPU stack.

Announce at start: `I'm using the nvidia-setup skill to configure the NVIDIA software stack.`

## Workflow

Run the bundled scripts in order. Prefer step-by-step over jumping to install.

```bash
cd /path/to/nvidia-setup/steps
./00-preflight.sh
./01-detect-gpu.sh
```

| Step | Script | Action |
| --- | --- | --- |
| 0 | `00-preflight.sh` | Verify Linux, not WSL, apt, GPU present |
| 1 | `01-detect-gpu.sh` | Collect GPU model, driver, CUDA, packages, NVSwitch topology |
| 2 | (research) | Reconfirm current driver/CUDA guidance against official NVIDIA docs |
| 3 | (confirm) | State `TARGET_*` values and reasoning before any install |
| 4 | `04-remove-old.sh` | Purge old NVIDIA/CUDA packages when switching branches or cleaning a broken stack |
| 5 | `05-install-driver.sh` | Install selected driver branch + CUDA toolkit; **reboot required** |
| 6 | `06-nvswitch.sh` | Fabric Manager / NVLSM only when topology shows NVSwitch |
| 7 | `07-docker.sh` | Install Container Toolkit from NVIDIA's own repo and configure Docker |
| 8 | `08-validate.sh` | Host `nvidia-smi`, `nvcc`, services, Docker runtime, container smoke test |

## Defaults (confirm before install)

```bash
TARGET_DRIVER=595          # production branch; use 610 only when CUDA 13.3 toolkit-corresponding driver is required
TARGET_CUDA_PKG=13-3
USE_OPEN_MODULES=true      # false for Maxwell/Pascal/Volta
NVSWITCH_PRESENT=false
NEEDS_NVLSM=false
CUDA_TEST_IMAGE="nvidia/cuda:13.3.0-base-ubuntu24.04"
PYTORCH_IMAGE=""
```

- Branches `590+` use `nvidia-driver-pinning-<branch>` + unversioned `nvidia-open` / `cuda-drivers`. Do not use old package names like `nvidia-open-595`.
- Do not treat the snapshot in this skill as fixed truth — reconfirm against official docs for the detected GPU family and intended CUDA version.

## Reference

- `references/compatibility.md` — Current branch snapshot, CUDA compatibility matrix, package naming, Container Toolkit, and GPU-family guidance
- Official docs: [Driver Installation Guide](https://docs.nvidia.com/datacenter/tesla/driver-installation-guide/), [CUDA Release Notes](https://docs.nvidia.com/cuda/cuda-toolkit-release-notes/), [CUDA Compatibility](https://docs.nvidia.com/deploy/cuda-compatibility/), [Unix Drivers](https://www.nvidia.com/en-us/drivers/unix/)
