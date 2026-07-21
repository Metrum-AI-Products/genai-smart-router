#!/usr/bin/env python3
"""Regression tests for Makefile build metadata handling."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
MALICIOUS_VERSION = "x; id >/tmp/smart-llmrouter-make-poc #"
MALICIOUS_EKS_INPUT = '"; id >/tmp/smart-llmrouter-eks-make-poc; echo "'


def run_make(*args: str) -> subprocess.CompletedProcess[str]:
    env = os.environ.copy()
    env.update(
        {
            "VERSION": "v1.2.3-4-gabcdef0-dirty",
            "COMMIT": "abcdef0",
            "BUILD_DATE": "2026-06-28T00:00:00Z",
            "GOOS": "linux",
            "GOARCH": "amd64",
            "PKG_NAME": "smart-llmrouter",
            "DIST_DIR": "dist",
            "IMAGE_NAME": "smart-llmrouter",
            "IMAGE_TAG": "v1.2.3-linux-amd64",
        }
    )
    return subprocess.run(
        ["make", *args],
        cwd=ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def main() -> int:
    good = run_make("validate-build-metadata")
    require(good.returncode == 0, f"normal metadata rejected:\n{good.stdout}\n{good.stderr}")

    bad = run_make("validate-build-metadata", f"VERSION={MALICIOUS_VERSION}")
    require(bad.returncode != 0, "malicious VERSION was accepted")
    require("unsupported characters" in bad.stderr, f"missing validation error: {bad.stderr}")

    package_dry_run = run_make("-n", "package-one-no-docs", f"VERSION={MALICIOUS_VERSION}")
    docker_dry_run = run_make("-n", "package-docker-one-no-docs", f"VERSION={MALICIOUS_VERSION}")
    combined = package_dry_run.stdout + package_dry_run.stderr + docker_dry_run.stdout + docker_dry_run.stderr
    require("id >/tmp/smart-llmrouter-make-poc" not in combined, f"dry-run exposed executable payload:\n{combined}")
    require("$VERSION" in combined or "${VERSION}" in combined, "dry-run should defer metadata to shell environment expansion")

    eks_profile_dry_run = run_make(
        "-n", "eks-preflight", f"EKS_AWS_PROFILE={MALICIOUS_EKS_INPUT}"
    )
    eks_evidence_dry_run = run_make(
        "-n", "eks-release-evidence", f"EKS_EVIDENCE_DIR={MALICIOUS_EKS_INPUT}"
    )
    eks_combined = (
        eks_profile_dry_run.stdout
        + eks_profile_dry_run.stderr
        + eks_evidence_dry_run.stdout
        + eks_evidence_dry_run.stderr
    )
    require(
        "id >/tmp/smart-llmrouter-eks-make-poc" not in eks_combined,
        f"EKS dry-run exposed executable payload:\n{eks_combined}",
    )
    require(
        "$EKS_AWS_PROFILE" in eks_combined or "${EKS_AWS_PROFILE}" in eks_combined,
        "EKS profile must defer expansion to the recipe shell",
    )
    require(
        "$EKS_EVIDENCE_DIR" in eks_combined or "${EKS_EVIDENCE_DIR}" in eks_combined,
        "EKS evidence directory must defer expansion to the recipe shell",
    )

    default_dry_run = run_make("-n")
    require(default_dry_run.returncode == 0, f"bare make dry-run failed:\n{default_dry_run.stderr}")
    require("go test ./..." in default_dry_run.stdout, "bare make must retain the test target")

    print("Makefile security self-test passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
