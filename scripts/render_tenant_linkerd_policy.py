#!/usr/bin/env python3
# Copyright 2026 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Render a tenant Linkerd policy only from verified, scrubbed discovery evidence."""

from __future__ import annotations

import argparse
import json
import os
import re
import tempfile
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_TEMPLATE = ROOT / "deploy/kubernetes/bootstrap/tenant-linkerd-policy.example.yaml"
DNS_LABEL = re.compile(r"^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$")
TRUST_DOMAIN = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$")


def linkerd_ingress_identity(namespace: str, service_account: str, control_plane_namespace: str, trust_domain: str) -> str:
    return f"{service_account}.{namespace}.serviceaccount.identity.{control_plane_namespace}.{trust_domain}"


def require_label(value: Any, name: str) -> str:
    if not isinstance(value, str) or not DNS_LABEL.fullmatch(value):
        raise ValueError(f"discovery report has invalid {name}")
    return value


def require_dns_subdomain(value: Any, name: str) -> str:
    if (
        not isinstance(value, str)
        or not value
        or len(value) > 253
        or not all(DNS_LABEL.fullmatch(label) for label in value.split("."))
    ):
        raise ValueError(f"discovery report has invalid {name}")
    return value


def require_trust_domain(value: Any) -> str:
    if not isinstance(value, str) or not TRUST_DOMAIN.fullmatch(value):
        raise ValueError("discovery report has invalid Linkerd trust domain")
    return value


def render(template: str, discovery: dict[str, Any]) -> str:
    selection = discovery.get("selection")
    linkerd = discovery.get("linkerd")
    if not isinstance(selection, dict) or not isinstance(linkerd, dict):
        raise ValueError("discovery report is missing selection or Linkerd evidence")
    if (
        linkerd.get("requested") is not True
        or linkerd.get("ingress_identity_verified") is not True
        or linkerd.get("ingress_workload_verified") is not True
        or linkerd.get("ingress_workload_mesh_ready") is not True
        or linkerd.get("ingress_workload_injection_verified") is not True
        or linkerd.get("router_workload_verified") is not True
        or linkerd.get("router_workload_mesh_ready") is not True
        or linkerd.get("router_workload_identity_verified") is not True
        or linkerd.get("router_workload_injection_verified") is not True
    ):
        raise ValueError("Linkerd ingress and router workload identity evidence was not verified by discovery")
    tenant_namespace = require_label(selection.get("namespace"), "tenant namespace")
    ingress_namespace = require_label(linkerd.get("ingress_namespace"), "ingress namespace")
    ingress_service_account = require_dns_subdomain(linkerd.get("ingress_service_account"), "ingress service account")
    control_plane_namespace = require_label(linkerd.get("control_plane_namespace"), "Linkerd control-plane namespace")
    trust_domain = require_trust_domain(linkerd.get("trust_domain"))
    expected_identity = linkerd_ingress_identity(
        ingress_namespace,
        ingress_service_account,
        control_plane_namespace,
        trust_domain,
    )
    if linkerd.get("ingress_identity") != expected_identity:
        raise ValueError("discovery report ingress identity does not match its validated namespace, service account, and trust domain")
    if template.count("__TENANT_NAMESPACE__") != 2 or template.count("__LINKERD_INGRESS_IDENTITY__") != 1:
        raise ValueError("Linkerd policy template has unexpected render placeholders")
    rendered = template.replace("__TENANT_NAMESPACE__", tenant_namespace).replace("__LINKERD_INGRESS_IDENTITY__", expected_identity)
    if "__TENANT_NAMESPACE__" in rendered or "__LINKERD_INGRESS_IDENTITY__" in rendered:
        raise ValueError("Linkerd policy rendering left an unresolved placeholder")
    return rendered


def atomic_write(path: Path, content: str) -> None:
    if not path.is_absolute() or path.resolve().is_relative_to(ROOT):
        raise ValueError("rendered Linkerd policy output must be an absolute path outside the repository")
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
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    try:
        discovery = json.loads(args.discovery_report.read_text(encoding="utf-8"))
        if not isinstance(discovery, dict):
            raise ValueError("discovery report must be an object")
        rendered = render(args.template.read_text(encoding="utf-8"), discovery)
        atomic_write(args.output, rendered)
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        parser.error(str(exc))
    print(f"wrote rendered Linkerd policy: {args.output}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
