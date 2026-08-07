#!/usr/bin/env python3
"""Reconcile the one reviewed staging delivery Role and RoleBinding safely."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import stat
import subprocess
import sys
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
NAMESPACE = "smart-llmrouter-staging"
ROLE_NAME = "genai-smart-router-eks-staging-delivery"
MANIFESTS = (
    ROOT / "deploy/kubernetes/bootstrap/eks-staging-delivery-namespace-rbac.yaml",
    ROOT / "deploy/kubernetes/bootstrap/eks-staging-delivery-rolebinding.yaml",
)
FIELD_MANAGER = "genai-smart-router-eks-staging-bootstrap"
CONFIRMATION = "RECOVER_STAGING_DELIVERY_RBAC"
GUARD_NAME = "genai-smart-router-eks-staging-bootstrap-rbac"
GUARD_POLICY_DIGEST = "33477d2999148bd1831b9a8d2f818029d89529d076af638e5c500b0f4f4fd213"
GUARD_BINDING_DIGEST = "07a7b140864e4af6137a60d84aabc8a9e79ff0c24a86ab9b81090997625dd749"

ROLE_RULES: list[dict[str, Any]] = [
    {"apiGroups": [""], "resources": ["pods"], "verbs": ["get", "list", "watch"]},
    {
        "apiGroups": [""],
        "resources": ["services", "serviceaccounts", "persistentvolumeclaims"],
        "verbs": ["get", "list", "watch", "create", "update", "patch"],
    },
    {
        "apiGroups": ["apps"],
        "resources": ["deployments"],
        "verbs": ["get", "list", "watch", "create", "update", "patch"],
    },
    {"apiGroups": ["apps"], "resources": ["replicasets"], "verbs": ["get", "list", "watch"]},
    {
        "apiGroups": ["networking.k8s.io"],
        "resources": ["ingresses", "networkpolicies"],
        "verbs": ["get", "list", "watch", "create", "update", "patch"],
    },
    {
        "apiGroups": ["policy"],
        "resources": ["poddisruptionbudgets"],
        "verbs": ["get", "list", "watch", "create", "update", "patch"],
    },
    {
        "apiGroups": [""],
        "resources": ["configmaps"],
        "resourceNames": ["smartrouter-staging-runtime-attestation"],
        "verbs": ["get"],
    },
]
LABELS = {
    "app.kubernetes.io/name": "smart-llmrouter",
    "app.kubernetes.io/component": "delivery-identity",
}


class RecoveryError(RuntimeError):
    pass


def run(command: list[str]) -> str:
    try:
        completed = subprocess.run(command, check=True, capture_output=True, text=True)
    except FileNotFoundError as exc:
        raise RecoveryError(f"required executable is unavailable: {command[0]}") from exc
    except subprocess.CalledProcessError as exc:
        # kubectl diagnostics can include kubeconfig paths, endpoint details, or
        # API payload fragments. This CLI records only a bounded failure class.
        raise RecoveryError(f"{command[0]} command failed") from exc
    return completed.stdout


def safe_kubeconfig(path_text: str) -> Path:
    if not path_text or os.pathsep in path_text:
        raise RecoveryError("KUBECONFIG must name one explicit kubeconfig file")
    path = Path(path_text).expanduser().resolve()
    try:
        mode = stat.S_IMODE(path.stat().st_mode)
    except OSError as exc:
        raise RecoveryError("explicit kubeconfig is unavailable") from exc
    if mode & 0o077:
        raise RecoveryError("explicit kubeconfig must not be group- or world-accessible")
    return path


def kubectl(kubeconfig: Path, args: list[str]) -> str:
    return run(["kubectl", "--kubeconfig", str(kubeconfig), *args])


def allowed(kubeconfig: Path, verb: str, resource: str) -> bool:
    return kubectl(kubeconfig, ["auth", "can-i", verb, resource, "-n", NAMESPACE]).strip() == "yes"


def expected_role(payload: dict[str, Any]) -> bool:
    metadata = payload.get("metadata")
    return (
        isinstance(metadata, dict)
        and metadata.get("name") == ROLE_NAME
        and metadata.get("namespace") == NAMESPACE
        and metadata.get("labels") == LABELS
        and payload.get("rules") == ROLE_RULES
    )


def expected_rolebinding(payload: dict[str, Any]) -> bool:
    metadata = payload.get("metadata")
    return (
        isinstance(metadata, dict)
        and metadata.get("name") == ROLE_NAME
        and metadata.get("namespace") == NAMESPACE
        and metadata.get("labels") == LABELS
        and payload.get("subjects")
        == [{"apiGroup": "rbac.authorization.k8s.io", "kind": "Group", "name": ROLE_NAME}]
        and payload.get("roleRef")
        == {"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": ROLE_NAME}
    )


def readback(kubeconfig: Path) -> dict[str, bool]:
    states: dict[str, bool] = {}
    for resource, validator in (("role", expected_role), ("rolebinding", expected_rolebinding)):
        try:
            payload = json.loads(kubectl(kubeconfig, ["get", resource, ROLE_NAME, "-n", NAMESPACE, "-o", "json"]))
            states[resource] = validator(payload)
        except (RecoveryError, json.JSONDecodeError):
            states[resource] = False
    return states

def expected_guard(payload: dict[str, Any], *, digest: str) -> bool:
    metadata = payload.get("metadata")
    spec = payload.get("spec")
    if not isinstance(metadata, dict) or metadata.get("name") != GUARD_NAME or not isinstance(spec, dict):
        return False
    canonical_spec = json.dumps(spec, sort_keys=True, separators=(",", ":")).encode()
    return hashlib.sha256(canonical_spec).hexdigest() == digest


def assert_admission_guard(kubeconfig: Path) -> None:
    resources = (
        ("validatingadmissionpolicy", GUARD_POLICY_DIGEST),
        ("validatingadmissionpolicybinding", GUARD_BINDING_DIGEST),
    )
    for resource, digest in resources:
        try:
            payload = json.loads(kubectl(kubeconfig, ["get", resource, GUARD_NAME, "-o", "json"]))
        except (RecoveryError, json.JSONDecodeError) as exc:
            raise RecoveryError("reviewed bootstrap admission guard is unavailable") from exc
        if not expected_guard(payload, digest=digest):
            raise RecoveryError("reviewed bootstrap admission guard does not match its exact specification")


def assert_authorized(kubeconfig: Path) -> None:
    checks = (
        ("get", f"role/{ROLE_NAME}"),
        ("patch", f"role/{ROLE_NAME}"),
        ("update", f"role/{ROLE_NAME}"),
        ("escalate", f"role/{ROLE_NAME}"),
        ("bind", f"role/{ROLE_NAME}"),
        ("get", f"rolebinding/{ROLE_NAME}"),
        ("patch", f"rolebinding/{ROLE_NAME}"),
        ("update", f"rolebinding/{ROLE_NAME}"),
    )
    denied = [f"{verb} {resource}" for verb, resource in checks if not allowed(kubeconfig, verb, resource)]
    if denied:
        raise RecoveryError("bootstrap authorization is incomplete for the exact delivery RBAC objects")


def apply(kubeconfig: Path, *, dry_run: bool) -> None:
    command = [
        "apply",
        "--server-side",
        "--force-conflicts",
        f"--field-manager={FIELD_MANAGER}",
        "--validate=true",
        *[value for manifest in MANIFESTS for value in ("-f", str(manifest))],
    ]
    if dry_run:
        command.insert(3, "--dry-run=server")
    kubectl(kubeconfig, command)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    parser.add_argument("--kubeconfig", required=True)
    parser.add_argument("--confirm", required=True)
    args = parser.parse_args(argv)
    if args.confirm != CONFIRMATION:
        raise RecoveryError("confirmation must equal RECOVER_STAGING_DELIVERY_RBAC")
    kubeconfig = safe_kubeconfig(args.kubeconfig)
    assert_admission_guard(kubeconfig)
    assert_authorized(kubeconfig)
    apply(kubeconfig, dry_run=True)
    try:
        apply(kubeconfig, dry_run=False)
    except RecoveryError:
        states = readback(kubeconfig)
        print(json.dumps({"action": "recover-staging-delivery-rbac", "outcome": "not-reconciled", "objects": states}, sort_keys=True))
        return 1
    states = readback(kubeconfig)
    if not all(states.values()):
        print(json.dumps({"action": "recover-staging-delivery-rbac", "outcome": "not-reconciled", "objects": states}, sort_keys=True))
        return 1
    print(json.dumps({"action": "recover-staging-delivery-rbac", "outcome": "reconciled", "objects": states}, sort_keys=True))
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except RecoveryError as exc:
        print(f"staging delivery RBAC recovery failed: {exc}", file=sys.stderr)
        raise SystemExit(2)
