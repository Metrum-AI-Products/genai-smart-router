# NVIDIA Compatibility Notes

This reference captures the public guidance that informed the bundled workflow. Reconfirm on the official NVIDIA docs before changing a production host.

The bundled scripts in this repository target Ubuntu hosts, not generic Debian-family systems.

## Official docs to consult

- NVIDIA data center driver index:
  `https://docs.nvidia.com/datacenter/tesla/index.html`
- NVIDIA driver installation guide:
  `https://docs.nvidia.com/datacenter/tesla/driver-installation-guide/`
- CUDA forward compatibility:
  `https://docs.nvidia.com/deploy/cuda-compatibility/`
- CUDA Linux installation guide:
  `https://docs.nvidia.com/cuda/cuda-installation-guide-linux/`
- Container Toolkit install guide:
  `https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html`

## Current branch snapshot

Checked against NVIDIA documentation on **July 19, 2026**:

- NVIDIA Unix driver archive lists Linux x86_64/aarch64 production branch `595.84`.
- NVIDIA Unix driver archive lists Linux x86_64/aarch64 new-feature branch `610.43.03`.
- NVIDIA data center driver index currently lists R580 data-center release notes, with older branches still present.
- CUDA Toolkit release notes for CUDA 13.3 Update 1 list `610.43.02` as the Linux driver version associated with the toolkit.
- CUDA 13.4 Developer Preview is available and unbundles the Linux driver (R616, >=616.00).
- Container Toolkit latest version: `1.18.2`.

Implication:

- Start production Ubuntu installs at `TARGET_DRIVER=595` unless current docs or the workload require a newer new-feature branch.
- Use `TARGET_DRIVER=610` only when the host/workload specifically needs the CUDA 13.3 toolkit-corresponding driver or another feature unavailable in production branch 595.
- Do not infer that every CUDA 13.3 workload requires 610; CUDA 13.x minor-version compatibility allows 580+ drivers, but newer toolkit features can require newer drivers.

## CUDA compatibility guidance

NVIDIA's forward compatibility guidance currently indicates:

- CUDA 13.x minor-version compatibility requires `580+`
- CUDA 12.x minor-version compatibility requires `525+` and `<580`
- CUDA 11.x minor-version compatibility requires `450+` and `<525`

CUDA Toolkit 13.3 release notes list toolkit-corresponding Linux driver versions:

| CUDA Toolkit | Corresponding Linux Driver |
|---|---|
| CUDA 13.4 Developer Preview | N/A (unbundled; R616 >=616.00) |
| CUDA 13.3 Update 1 | >=610.43.02 |
| CUDA 13.3 GA | >=610.43.02 |
| CUDA 13.2 Update 1 | >=595.58.03 |
| CUDA 13.1 Update 1 | >=590.48.01 |
| CUDA 13.0 Update 2 | >=580.95.05 |

Do not select the CUDA package independently from the driver branch.

Starting with CUDA 13.4 Developer Preview, the Linux driver is no longer bundled with the CUDA Toolkit. You must install the separately-released NVIDIA Developer Driver (R616, >=616.00) to use 13.4 features or newly enabled platforms. Existing CUDA 13.x applications continue to run on drivers >=580 under CUDA minor-version compatibility.

## Kernel module guidance

From the NVIDIA driver installation guide:

- Proprietary kernel modules are required for older Maxwell, Pascal, and Volta GPUs.
- Open kernel modules are supported on Turing and newer GPUs.
- Starting with the 560 driver series, open kernel modules are the default and suggested installation.

Practical rule for this skill:

- Maxwell / Pascal / Volta: use proprietary
- Turing and newer: prefer open unless the environment has a documented reason not to

## CUDA package guidance

The CUDA installation guide recommends `cuda-toolkit` meta packages for generic installs.

For pinned installs, distro repositories commonly expose versioned toolkit packages such as:

- `cuda-toolkit-13-3`
- `cuda-toolkit-13-2`
- `cuda-toolkit-13-1`
- `cuda-toolkit-13-0`

The bundled install script accepts `TARGET_CUDA_PKG` so you can pin the exact package line you researched. The default is `13-3`.

## Ubuntu driver package guidance

NVIDIA changed Ubuntu driver package naming for branch 590 and later:

- Install `nvidia-driver-pinning-<branch>` to lock a branch such as `595`.
- Then install the unversioned package:
  - `nvidia-open` for Turing and newer GPUs using open kernel modules.
  - `cuda-drivers` for proprietary modules or legacy GPU families.
- Do not use old branch-suffixed driver package names such as `nvidia-open-595` for branches `590+`.

For branches older than 590, the historical package names remain relevant:

- `nvidia-open-<branch>`
- `nvidia-driver-<branch>`

Ubuntu 22.04, 24.04, and **26.04** are all supported platforms in the latest kernel module tables.

## NVSwitch and services

Use Fabric Manager only when the platform actually needs it. Common cases:

- PCIe-only single-GPU or multi-GPU hosts: usually no Fabric Manager
- HGX / NVSwitch systems: Fabric Manager is typically required
- NVL systems that explicitly require NVLSM: install `nvlsm` in addition to Fabric Manager

Match Fabric Manager to the selected driver branch:

- For `590+` Ubuntu packages, use unversioned `nvidia-fabricmanager` with the branch pinning package already installed.
- For older branches, use branch-suffixed packages such as `nvidia-fabricmanager-580`.

## Container Toolkit guidance

The NVIDIA Container Toolkit is installed from its own repository (`stable/deb`), not from the CUDA repo. The step 7 script in this skill handles GPG key and apt source setup correctly.

Latest release as of July 2026: **1.18.2**.

Install command (for version pinning):

```bash
sudo apt-get install -y \
  nvidia-container-toolkit=1.18.2-1 \
  nvidia-container-toolkit-base=1.18.2-1 \
  libnvidia-container-tools=1.18.2-1 \
  libnvidia-container1=1.18.2-1
```

## Validation image guidance

The skill defaults to a lightweight CUDA image:

```bash
CUDA_TEST_IMAGE="nvidia/cuda:13.3.0-base-ubuntu24.04"
```

If you want a framework-level validation, set `PYTORCH_IMAGE` to a current image that matches the host CUDA stack before running the validation step.
