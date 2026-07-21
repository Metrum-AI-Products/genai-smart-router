#!/usr/bin/env python3
"""Regression tests for EKS Make input validation."""

from __future__ import annotations

import importlib.util
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("validate_eks_make_args", ROOT / "scripts" / "validate_eks_make_args.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load EKS Make argument validator")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def test_dns_subdomain_workload_names_reach_make_validation() -> None:
    workload_name = f"{'a' * 63}.{'b' * 63}.{'c' * 63}.{'d' * 61}"
    with tempfile.TemporaryDirectory() as directory:
        original_argv = sys.argv
        sys.argv = [
            "validate_eks_make_args.py",
            "--profile", "discovery-profile",
            "--account-id", "123456789012",
            "--region", "us-east-1",
            "--cluster", "approved-cluster",
            "--namespace", "tenant-acme",
            "--linkerd-namespace", "linkerd",
            "--ingress-namespace", "gateway-system",
            "--ingress-service-account", workload_name,
            "--ingress-deployment", workload_name,
            "--linkerd-trust-domain", "mesh.example",
            "--ecr-repository", "approved-router-repository",
            "--output", str(Path(directory) / "eks-discovery.json"),
        ]
        try:
            if MODULE.main() != 0:
                raise AssertionError("valid DNS-subdomain ingress workload names failed Make validation")
        finally:
            sys.argv = original_argv
    for invalid in (f"{'a' * 64}.example", "invalid..example"):
        try:
            MODULE.validate("kubernetes-object-name", invalid)
        except ValueError:
            pass
        else:
            raise AssertionError(f"invalid Kubernetes DNS-subdomain workload name was accepted: {invalid}")


def test_non_linkerd_ingress_namespace_is_validated_and_accepted() -> None:
    with tempfile.TemporaryDirectory() as directory:
        original_argv = sys.argv
        sys.argv = [
            "validate_eks_make_args.py",
            "--profile", "discovery-profile",
            "--account-id", "123456789012",
            "--region", "us-east-1",
            "--cluster", "approved-cluster",
            "--namespace", "tenant-acme",
            "--ingress-namespace", "external-gateway",
            "--ecr-repository", "approved-router-repository",
            "--output", str(Path(directory) / "eks-discovery.json"),
        ]
        try:
            if MODULE.main() != 0:
                raise AssertionError("non-Linkerd ingress selection failed Make validation")
        finally:
            sys.argv = original_argv


if __name__ == "__main__":
    test_dns_subdomain_workload_names_reach_make_validation()
    test_non_linkerd_ingress_namespace_is_validated_and_accepted()
    print("EKS Make argument validation tests passed")
