#!/usr/bin/env python3
"""Contract tests for scripts/eks_delivery.py using fake cloud executables."""
from __future__ import annotations

import os
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "eks_delivery.py"
DIGEST = "registry.example/router@sha256:" + "a" * 64


def fake_tools(directory: Path) -> None:
    log = directory / "calls.log"
    for name in ("aws", "kubectl", "kustomize"):
        body = f'''#!/bin/sh
echo "$0 $* KUBECONFIG=$KUBECONFIG" >> "{log}"
case "$0" in
  *aws) case "$*" in *get-caller-identity*) echo '{{"Account":"safe"}}' ;; *describe-cluster*) echo ACTIVE ;; esac ;;
  *kubectl) case "$*" in *"auth can-i"*) echo yes ;; esac ;;
  *kustomize) case "$*" in *build*) echo 'image: {DIGEST}' ;; esac ;;
esac
'''
        target = directory / name
        target.write_text(body)
        target.chmod(0o755)


def run(action: str, root: Path, extra: list[str] | None = None) -> subprocess.CompletedProcess[str]:
    overlay = root / "deploy" / "kubernetes" / "overlays" / "example"
    evidence = root / "tmp" / "evidence"
    command = ["python3", str(SCRIPT), action, "--aws-region", "us-east-1", "--eks-cluster", "test-cluster",
               "--k8s-namespace", "router-staging", "--kustomize-overlay", str(overlay), "--environment", "staging",
               "--evidence-dir", str(evidence)]
    if action in {"render", "plan", "apply", "smoke"}:
        command += ["--image-digest", DIGEST]
    if extra:
        command += extra
    bindir = root / "fake-bin"
    env = {**os.environ, "PATH": f"{bindir}:{os.environ['PATH']}", "KUBECONFIG": str(root / "must-not-be-used")}
    return subprocess.run(command, cwd=root, text=True, capture_output=True, env=env)


def main() -> int:
    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        shutil = __import__("shutil")
        shutil.copytree(ROOT / "deploy", root / "deploy")
        bindir = root / "fake-bin"; bindir.mkdir(); fake_tools(bindir)
        assert run("preflight", root).returncode == 0
        assert run("status", root).returncode == 0
        result = run("plan", root)
        assert result.returncode == 0, result.stderr
        calls = (bindir / "calls.log").read_text()
        assert "must-not-be-used" not in calls and "update-kubeconfig" in calls and "--dry-run=server" in calls
        assert run("apply", root).returncode != 0
        bad = run("render", root, ["--image-digest", "router:latest"])
        assert bad.returncode != 0 and "immutable" in bad.stderr
        production = subprocess.run([
            "python3", str(SCRIPT), "preflight", "--aws-region", "us-east-1", "--eks-cluster", "test-cluster",
            "--k8s-namespace", "router-staging", "--kustomize-overlay", str(root / "deploy/kubernetes/overlays/example"),
            "--environment", "production", "--evidence-dir", str(root / "tmp/production")],
            cwd=root, text=True, capture_output=True, env={**os.environ, "PATH": f"{bindir}:{os.environ['PATH']}"})
        assert production.returncode != 0 and "only ENVIRONMENT=staging" in production.stderr
        applied = run("apply", root, ["--confirm", "STAGING_APPLY"])
        assert applied.returncode == 0, applied.stderr
        smoke = run("smoke", root, ["--smoke-command", "sh -c 'echo token=leak >&2; exit 1'"])
        assert smoke.returncode != 0 and "token=leak" not in smoke.stderr
        assert "token=leak" not in (root / "tmp/evidence/evidence.json").read_text()
    print("EKS delivery contract tests passed")


if __name__ == "__main__":
    main()
