#!/usr/bin/env python3
"""Static safeguards for the namespace-scoped EKS bootstrap examples."""

from __future__ import annotations

import json
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RBAC = ROOT / "deploy/kubernetes/bootstrap/tenant-provisioner-rbac.yaml"
TRUST = ROOT / "deploy/aws/github-oidc-trust-policy.example.json"


def main() -> int:
    content = RBAC.read_text(encoding="utf-8")
    forbidden = ("resources: [\"secrets\"]", "kind: ClusterRole", "kind: ClusterRoleBinding", "pods/exec", "verbs: [\"*\"]")
    for value in forbidden:
        if value in content:
            raise SystemExit(f"unsafe tenant provisioner permission: {value}")
    if "apiGroups: [\"policy.linkerd.io\"]" not in content or "serverauthorizations" not in content:
        raise SystemExit("tenant provisioner must retain namespace-scoped Linkerd policy support")

    policy = json.loads(TRUST.read_text(encoding="utf-8"))
    condition = policy["Statement"][0]["Condition"]["StringEquals"]
    subject = condition.get("token.actions.githubusercontent.com:sub", "")
    if not subject.startswith("repo:<OWNER>/<REPOSITORY>:environment:") or "*" in subject:
        raise SystemExit("OIDC subject must be an exact repository environment subject")
    if condition.get("token.actions.githubusercontent.com:aud") != "sts.amazonaws.com":
        raise SystemExit("OIDC audience must be sts.amazonaws.com")
    print("EKS bootstrap asset safeguards passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
