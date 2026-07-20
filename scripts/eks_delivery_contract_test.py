#!/usr/bin/env python3
"""Contract tests for scripts/eks_delivery.py using fake cloud executables."""
from __future__ import annotations

import importlib.util
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "eks_delivery.py"
DIGEST = "registry.example/router@sha256:" + "a" * 64
SPEC = importlib.util.spec_from_file_location("eks_delivery", SCRIPT)
EKS_DELIVERY = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = EKS_DELIVERY
SPEC.loader.exec_module(EKS_DELIVERY)
TARGET_POLICY = json.loads((ROOT / "deploy" / "aws" / "genai-smart-router-eks-staging-target.json").read_text())


def target_value(value: dict[str, object] | None = None) -> str:
    return json.dumps(json.dumps(value or TARGET_POLICY, sort_keys=True, separators=(",", ":")))


def rendered_objects(namespace: str | None = None) -> str:
    namespace = namespace or str(TARGET_POLICY["k8s_namespace"])
    return json.dumps(
        {
            "apiVersion": "v1",
            "kind": "List",
            "items": [
                {
                    "apiVersion": "apps/v1",
                    "kind": "Deployment",
                    "metadata": {"name": "smart-llmrouter", "namespace": namespace},
                    "spec": {
                        "template": {
                            "spec": {"containers": [{"name": "router", "image": DIGEST}]}
                        }
                    },
                },
                {
                    "apiVersion": "v1",
                    "kind": "Service",
                    "metadata": {"name": "smart-llmrouter", "namespace": namespace},
                },
            ],
        }
    )


def deployment_object(digest: str = DIGEST) -> str:
    return json.dumps(
        {
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": "smart-llmrouter",
                "namespace": TARGET_POLICY["k8s_namespace"],
            },
            "spec": {"template": {"spec": {"containers": [{"name": "router", "image": digest}]}}},
        }
    )


def fake_tools(directory: Path) -> None:
    log = directory / "calls.log"
    for name in ("aws", "kubectl", "kustomize"):
        body = f'''#!/bin/sh
echo "$0 $* KUBECONFIG=$KUBECONFIG" >> "{log}"
case "$0" in
  *aws) case "$*" in
    *"ssm get-parameter"*) printf '{{"Parameter":{{"Type":"String","Value":%s}}}}\\n' "$FAKE_TARGET_POLICY_VALUE" ;;
    *get-caller-identity*) printf '{{"Account":"%s","Arn":"arn:aws:sts::%s:assumed-role/%s/test-session"}}\\n' "$FAKE_AWS_ACCOUNT" "$FAKE_AWS_ACCOUNT" "$FAKE_AWS_ROLE" ;;
    *describe-cluster*) printf '{{"cluster":{{"status":"ACTIVE","arn":"arn:aws:eks:%s:%s:cluster/%s"}}}}\\n' "$FAKE_AWS_REGION" "$FAKE_AWS_ACCOUNT" "$FAKE_EKS_CLUSTER" ;;
  esac ;;
  *kubectl) case "$*" in
    *"auth can-i"*) echo yes ;;
    *"apply --dry-run=client"*) printf '%s\\n' "$FAKE_RENDER_OBJECTS" ;;
    *"get deployment/smart-llmrouter"*) printf '%s\\n' "$FAKE_DEPLOYMENT_OBJECT" ;;
    *"rollout status"*) : ;;
    *apply*) for arg; do manifest="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}" ;;
  esac ;;
  *kustomize) case "$*" in
    *build*) printf 'secret: staging\\nimage: {DIGEST}\\n'; head -c 5000 /dev/zero | tr '\\0' x; echo ;;
  esac ;;
esac
'''
        target = directory / name
        target.write_text(body)
        target.chmod(0o755)


def run(
    action: str,
    root: Path,
    extra: list[str] | None = None,
    *,
    include_digest: bool = True,
    account: str | None = None,
    policy: dict[str, object] | None = None,
    render_objects: str | None = None,
    deployed_digest: str = DIGEST,
) -> subprocess.CompletedProcess[str]:
    evidence = root / "tmp" / "evidence"
    command = [
        "python3",
        str(SCRIPT),
        action,
        "--aws-profile",
        "test-profile",
        "--evidence-dir",
        str(evidence),
    ]
    if include_digest:
        command += ["--image-digest", DIGEST]
    if extra:
        command += extra
    bindir = root / "fake-bin"
    env = {
        **os.environ,
        "PATH": f"{bindir}:{os.environ['PATH']}",
        "KUBECONFIG": str(root / "must-not-be-used"),
        "FAKE_AWS_ACCOUNT": account or str(TARGET_POLICY["aws_account_id"]),
        "FAKE_AWS_REGION": str(TARGET_POLICY["aws_region"]),
        "FAKE_AWS_ROLE": str(TARGET_POLICY["delivery_role_name"]),
        "FAKE_EKS_CLUSTER": str(TARGET_POLICY["eks_cluster"]),
        "FAKE_TARGET_POLICY_VALUE": target_value(policy),
        "FAKE_RENDER_OBJECTS": render_objects or rendered_objects(),
        "FAKE_DEPLOYMENT_OBJECT": deployment_object(deployed_digest),
    }
    return subprocess.run(command, cwd=root, text=True, capture_output=True, env=env)


def main() -> int:
    scrubbed = EKS_DELIVERY.scrub(
        '{"Authorization":"Bearer abc123","X-Api-Key":"key456","AWS_SECRET_ACCESS_KEY":"secret789"}'
    )
    for leaked in ("abc123", "key456", "secret789"):
        assert leaked not in scrubbed
    assert "abc123" not in EKS_DELIVERY.scrub("Authorization: Bearer abc123")

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        bindir = root / "fake-bin"
        bindir.mkdir()
        fake_tools(bindir)

        assert run("preflight", root).returncode == 0
        result = run("plan", root)
        assert result.returncode == 0, result.stderr
        calls = (bindir / "calls.log").read_text()
        assert "must-not-be-used" not in calls and "update-kubeconfig" in calls and "--dry-run=server" in calls
        assert calls.index("ssm get-parameter") < calls.index("describe-cluster")
        manifest_size = re.search(r"manifest-bytes=\s*(\d+)", calls)
        assert manifest_size and int(manifest_size.group(1)) > 4000

        before_wrong_account = (bindir / "calls.log").read_text()
        wrong_account = run("preflight", root, account="999999999999")
        assert wrong_account.returncode != 0 and "approved staging account" in wrong_account.stderr
        assert "describe-cluster" not in (bindir / "calls.log").read_text()[len(before_wrong_account):]

        policy_drift = dict(TARGET_POLICY)
        policy_drift["eks_cluster"] = "unapproved-cluster"
        drift = run("preflight", root, policy=policy_drift)
        assert drift.returncode != 0 and "does not match" in drift.stderr

        wrong_namespace = run("plan", root, render_objects=rendered_objects("another-namespace"))
        assert wrong_namespace.returncode != 0 and "outside the approved namespace" in wrong_namespace.stderr

        assert run("apply", root).returncode != 0
        applied = run("apply", root, ["--confirm", "STAGING_APPLY"])
        assert applied.returncode == 0, applied.stderr

        promotion = run("promotion-plan", root)
        assert promotion.returncode != 0 and "smoke evidence" in promotion.stderr
        no_digest = run("promotion-plan", root, include_digest=False)
        assert no_digest.returncode != 0 and "IMAGE_DIGEST" in no_digest.stderr

        smoke = run(
            "smoke",
            root,
            [
                "--smoke-command",
                "sh -c 'echo Authorization: Bearer abc123 X-Api-Key=key456 AWS_SECRET_ACCESS_KEY=secret789 >&2; exit 1'",
            ],
        )
        assert smoke.returncode != 0
        for leaked in ("abc123", "key456", "secret789"):
            assert leaked not in smoke.stderr and leaked not in (root / "tmp/evidence/evidence.json").read_text()

        marker = root / "smoke-command-ran"
        stale = run(
            "smoke",
            root,
            ["--smoke-command", f"touch {marker}"],
            deployed_digest="registry.example/router@sha256:" + "b" * 64,
        )
        assert stale.returncode != 0 and "does not use" in stale.stderr and not marker.exists()

        smoke_passed = run("smoke", root, ["--smoke-command", "sh -c 'exit 0'"])
        assert smoke_passed.returncode == 0, smoke_passed.stderr
        promotion = run("promotion-plan", root)
        assert promotion.returncode == 0, promotion.stderr

        make_dry_run = subprocess.run(["make", "-n", "eks-preflight"], cwd=ROOT, text=True, capture_output=True)
        assert make_dry_run.returncode == 0, make_dry_run.stderr
        assert "--eks-cluster" not in make_dry_run.stdout and "--approved-eks-cluster" not in make_dry_run.stdout
        legacy_target = subprocess.run(
            [
                "python3",
                str(SCRIPT),
                "preflight",
                "--aws-profile",
                "test-profile",
                "--evidence-dir",
                str(root / "tmp" / "legacy"),
                "--eks-cluster",
                "unapproved-cluster",
            ],
            text=True,
            capture_output=True,
        )
        assert legacy_target.returncode != 0 and "unrecognized arguments" in legacy_target.stderr
    print("EKS delivery contract tests passed")


if __name__ == "__main__":
    raise SystemExit(main())
