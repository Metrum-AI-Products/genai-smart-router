#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Render a selected-ingress NetworkPolicy from verified EKS discovery."""

from __future__ import annotations

import argparse
import json
import os
import re
import tempfile
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_TEMPLATE = ROOT / "deploy/kubernetes/bootstrap/tenant-ingress-network-policy.example.yaml"
EKS_INGRESS_GUARD_POLICY = ROOT / "deploy/kubernetes/bootstrap/tenant-router-ingress-guard.example.yaml"
DNS_LABEL = re.compile(r"^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$")


def require_label(value: object, label: str) -> str:
    if not isinstance(value, str) or not DNS_LABEL.fullmatch(value):
        raise ValueError(f"discovery report has an invalid {label}")
    return value


def validated_namespaces(discovery: dict[str, Any]) -> tuple[str, str]:
    selection = discovery.get("selection")
    linkerd = discovery.get("linkerd")
    if not isinstance(selection, dict) or not isinstance(linkerd, dict):
        raise ValueError("discovery report is missing selection or Linkerd evidence")
    tenant_namespace = require_label(selection.get("namespace"), "tenant namespace")
    ingress_namespace = require_label(selection.get("ingress_namespace"), "ingress namespace")
    if linkerd.get("requested") is True:
        if (
            linkerd.get("ingress_identity_verified") is not True
            or linkerd.get("ingress_workload_verified") is not True
            or linkerd.get("ingress_workload_mesh_ready") is not True
            or linkerd.get("ingress_workload_injection_verified") is not True
            or linkerd.get("router_workload_verified") is not True
            or linkerd.get("router_workload_mesh_ready") is not True
            or linkerd.get("router_workload_identity_verified") is not True
            or linkerd.get("router_workload_injection_verified") is not True
            or linkerd.get("ingress_namespace") != ingress_namespace
        ):
            raise ValueError("Linkerd ingress and router workload identity evidence was not verified by discovery")
    elif linkerd.get("requested") is not False:
        raise ValueError("discovery report has an invalid Linkerd selection state")
    return tenant_namespace, ingress_namespace


def validate_eks_ingress_guard_policy(content: str) -> None:
    """Require the EKS-only guard to deny ingress pending selected-policy activation."""
    if "metadata:\n  name: smart-llmrouter-restrict-ingress\n" not in content:
        raise ValueError("EKS ingress guard is not the reviewed smart-router policy")
    if "    - Ingress\n" not in content or len(re.findall(r"(?m)^  ingress:", content)) != 1 or not re.search(r"(?m)^  ingress:\s*\[\]\s*$", content):
        raise ValueError("EKS ingress guard must deny ingress until the selected namespace policy is rendered")


def render(template: str, discovery: dict[str, Any], eks_ingress_guard_policy: str) -> str:
    tenant_namespace, ingress_namespace = validated_namespaces(discovery)
    validate_eks_ingress_guard_policy(eks_ingress_guard_policy)
    if template.count("__TENANT_NAMESPACE__") != 1 or template.count("__INGRESS_NAMESPACE__") != 1:
        raise ValueError("ingress NetworkPolicy template has unexpected render placeholders")
    if "ingress-nginx" in template:
        raise ValueError("ingress NetworkPolicy template must not retain a fixed ingress namespace")
    rendered = template.replace("__TENANT_NAMESPACE__", tenant_namespace).replace("__INGRESS_NAMESPACE__", ingress_namespace)
    if "__TENANT_NAMESPACE__" in rendered or "__INGRESS_NAMESPACE__" in rendered:
        raise ValueError("ingress NetworkPolicy rendering left an unresolved placeholder")
    return rendered


def atomic_write(path: Path, content: str) -> None:
    if not path.is_absolute() or path.resolve().is_relative_to(ROOT):
        raise ValueError("rendered ingress NetworkPolicy output must be an absolute path outside the repository")
    path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8") as file:
            os.fchmod(file.fileno(), 0o600)
            file.write(content)
            file.flush()
            os.fsync(file.fileno())
        os.replace(temporary, path)
    finally:
        if temporary.exists():
            temporary.unlink()


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--discovery-report", required=True, type=Path)
    parser.add_argument("--template", type=Path, default=DEFAULT_TEMPLATE)
    parser.add_argument("--eks-ingress-guard-policy", type=Path, default=EKS_INGRESS_GUARD_POLICY)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    try:
        discovery = json.loads(args.discovery_report.read_text(encoding="utf-8"))
        if not isinstance(discovery, dict):
            raise ValueError("discovery report must be an object")
        rendered = render(
            args.template.read_text(encoding="utf-8"),
            discovery,
            args.eks_ingress_guard_policy.read_text(encoding="utf-8"),
        )
        atomic_write(args.output, rendered)
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        parser.error(str(exc))
    print(f"wrote rendered ingress NetworkPolicy: {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
