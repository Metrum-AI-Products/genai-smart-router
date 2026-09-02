#!/usr/bin/env python3
# Copyright 2026 Metrum AI, Inc.
# SPDX-License-Identifier: Apache-2.0

"""Validate explicit, non-secret inputs for the read-only EKS Make targets."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


PATTERNS = {
    "profile": re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$"),
    "account-id": re.compile(r"^[0-9]{12}$"),
    "region": re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)+$"),
    "cluster": re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$"),
    "namespace": re.compile(r"^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$"),
    "trust-domain": re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$"),
    "ecr-repository": re.compile(r"^[A-Za-z0-9][A-Za-z0-9._/-]{0,255}$"),
}


def is_kubernetes_dns_subdomain(value: str | None) -> bool:
    """Accept Kubernetes object names, which may be dotted DNS subdomains."""
    label = PATTERNS["namespace"]
    return bool(value) and len(value) <= 253 and all(label.fullmatch(part) for part in value.split("."))


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser()
    result.add_argument("--identity-only", action="store_true")
    result.add_argument("--profile", required=True)
    result.add_argument("--account-id", required=True)
    result.add_argument("--region", required=True)
    result.add_argument("--cluster")
    result.add_argument("--namespace")
    result.add_argument("--linkerd-namespace")
    result.add_argument("--ingress-namespace")
    result.add_argument("--ingress-service-account")
    result.add_argument("--ingress-deployment")
    result.add_argument("--linkerd-trust-domain")
    result.add_argument("--ecr-repository")
    result.add_argument("--output")
    return result


def validate(name: str, value: str | None) -> None:
    valid = is_kubernetes_dns_subdomain(value) if name == "kubernetes-object-name" else bool(value and PATTERNS[name].fullmatch(value))
    if not valid:
        raise ValueError(f"invalid {name}; use the documented explicit identifier format")


def main() -> int:
    args = parser().parse_args()
    try:
        for name, value in (("profile", args.profile), ("account-id", args.account_id), ("region", args.region)):
            validate(name, value)
        if not args.identity_only:
            for name, value in (("cluster", args.cluster), ("namespace", args.namespace), ("ecr-repository", args.ecr_repository)):
                validate(name, value)
            if args.linkerd_namespace:
                validate("namespace", args.linkerd_namespace)
            validate("namespace", args.ingress_namespace)
            linkerd_identity_inputs = (args.ingress_service_account, args.ingress_deployment, args.linkerd_trust_domain)
            if any(linkerd_identity_inputs) and not all(linkerd_identity_inputs):
                raise ValueError("ingress service account, deployment, and Linkerd trust domain must be supplied together")
            if args.linkerd_namespace:
                validate("kubernetes-object-name", args.ingress_service_account)
                validate("kubernetes-object-name", args.ingress_deployment)
                validate("trust-domain", args.linkerd_trust_domain)
            elif any(linkerd_identity_inputs):
                raise ValueError("ingress identity is only valid when Linkerd discovery is selected")
            output = Path(args.output or "")
            if not output.is_absolute() or output.parent == Path("/"):
                raise ValueError("output must be an explicit absolute path outside the repository")
            repository_root = Path(__file__).resolve().parents[1]
            if output.resolve().is_relative_to(repository_root):
                raise ValueError("output must be outside the repository")
    except ValueError as error:
        print(f"EKS Make input error: {error}", file=sys.stderr)
        return 2
    print("EKS Make inputs validated")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
