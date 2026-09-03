#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Regression tests for discovery-bound ingress NetworkPolicy rendering."""

from __future__ import annotations

import importlib.util
import json
import os
import stat
import subprocess
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("render_tenant_ingress_network_policy", ROOT / "scripts" / "render_tenant_ingress_network_policy.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load ingress NetworkPolicy renderer")
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def discovery() -> dict[str, object]:
    return {
        "selection": {"namespace": "tenant-acme", "ingress_namespace": "gateway-system"},
        "linkerd": {
            "requested": True,
            "ingress_identity_verified": True,
            "ingress_workload_verified": True,
            "ingress_workload_mesh_ready": True,
            "ingress_workload_injection_verified": True,
            "router_workload_verified": True,
            "router_workload_mesh_ready": True,
            "router_workload_identity_verified": True,
            "router_workload_injection_verified": True,
            "ingress_namespace": "gateway-system",
        },
    }


def expect_rejected(callback) -> None:
    try:
        callback()
    except ValueError:
        return
    raise AssertionError("expected ingress NetworkPolicy rendering to fail closed")


def assert_make_renders_both_policies() -> None:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        report = discovery()
        linkerd = report["linkerd"]
        assert isinstance(linkerd, dict)
        linkerd.update(
            {
                "ingress_service_account": "gateway-proxy",
                "control_plane_namespace": "linkerd-control",
                "trust_domain": "mesh.example",
                "ingress_identity": "gateway-proxy.gateway-system.serviceaccount.identity.linkerd-control.mesh.example",
            }
        )
        discovery_report = root / "eks-discovery.json"
        ingress_policy = root / "tenant-ingress-network-policy.yaml"
        linkerd_policy = root / "tenant-linkerd-policy.yaml"
        discovery_report.write_text(json.dumps(report), encoding="utf-8")
        environment = {
            **os.environ,
            "EKS_DISCOVERY_OUTPUT": str(discovery_report),
            "EKS_INGRESS_NETWORK_POLICY_OUTPUT": str(ingress_policy),
            "EKS_LINKERD_POLICY_OUTPUT": str(linkerd_policy),
        }
        completed = subprocess.run(
            ["make", "eks-render-linkerd-policy"],
            cwd=ROOT,
            env=environment,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        if completed.returncode:
            raise AssertionError(f"Make did not render both discovery-bound policies: {completed.stderr}")
        if not ingress_policy.exists() or not linkerd_policy.exists():
            raise AssertionError("Make did not produce both required policy artifacts")
        environment.pop("EKS_INGRESS_NETWORK_POLICY_OUTPUT", None)
        failed = subprocess.run(
            ["make", "eks-render-linkerd-policy"],
            cwd=ROOT,
            env=environment,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        if failed.returncode == 0 or "EKS_INGRESS_NETWORK_POLICY_OUTPUT is required" not in failed.stderr:
            raise AssertionError("Make allowed Linkerd rendering without the companion ingress policy output")


def assert_make_renders_non_linkerd_ingress_policy() -> None:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        report = {"selection": {"namespace": "tenant-acme", "ingress_namespace": "external-gateway"}, "linkerd": {"requested": False}}
        discovery_report = root / "eks-discovery.json"
        ingress_policy = root / "tenant-ingress-network-policy.yaml"
        discovery_report.write_text(json.dumps(report), encoding="utf-8")
        completed = subprocess.run(
            ["make", "eks-render-ingress-network-policy"],
            cwd=ROOT,
            env={
                **os.environ,
                "EKS_DISCOVERY_OUTPUT": str(discovery_report),
                "EKS_INGRESS_NETWORK_POLICY_OUTPUT": str(ingress_policy),
            },
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=False,
        )
        if completed.returncode or "kubernetes.io/metadata.name: external-gateway" not in ingress_policy.read_text(encoding="utf-8"):
            raise AssertionError(f"Make did not render the non-Linkerd ingress policy: {completed.stderr}")


def main() -> int:
    template = (ROOT / "deploy/kubernetes/bootstrap/tenant-ingress-network-policy.example.yaml").read_text(encoding="utf-8")
    generic_base = (ROOT / "deploy/kubernetes/base/networkpolicy.yaml").read_text(encoding="utf-8")
    if "\n    - Ingress\n" in generic_base or "\n  ingress:" in generic_base:
        raise AssertionError("generic base NetworkPolicy must remain ingress-neutral for non-EKS deployments")
    eks_guard = (ROOT / "deploy/kubernetes/bootstrap/tenant-router-ingress-guard.example.yaml").read_text(encoding="utf-8")
    rendered = MODULE.render(template, discovery(), eks_guard)
    if "namespace: tenant-acme" not in rendered or "kubernetes.io/metadata.name: gateway-system" not in rendered:
        raise AssertionError("renderer did not bind the NetworkPolicy to verified discovery namespaces")
    metadata, spec = rendered.split("\nspec:\n", 1)
    delivery_label = "app.kubernetes.io/name: smart-llmrouter"
    if (
        delivery_label in metadata
        or delivery_label not in spec
        or rendered.count(delivery_label) != 1
    ):
        raise AssertionError(
            "discovery-owned ingress policy must keep the delivery label only in spec.podSelector"
        )
    if "ingress-nginx" in rendered:
        raise AssertionError("rendered NetworkPolicy retained a fixed ingress namespace")
    expect_rejected(lambda: MODULE.render(template, discovery(), generic_base))
    invalid = discovery()
    invalid_linkerd = invalid["linkerd"]
    assert isinstance(invalid_linkerd, dict)
    invalid_linkerd["ingress_workload_mesh_ready"] = False
    expect_rejected(lambda: MODULE.render(template, invalid, eks_guard))
    invalid_ingress_injection = discovery()
    invalid_ingress_injection_linkerd = invalid_ingress_injection["linkerd"]
    assert isinstance(invalid_ingress_injection_linkerd, dict)
    invalid_ingress_injection_linkerd["ingress_workload_injection_verified"] = False
    expect_rejected(lambda: MODULE.render(template, invalid_ingress_injection, eks_guard))
    invalid_router = discovery()
    invalid_router_linkerd = invalid_router["linkerd"]
    assert isinstance(invalid_router_linkerd, dict)
    invalid_router_linkerd["router_workload_mesh_ready"] = False
    expect_rejected(lambda: MODULE.render(template, invalid_router, eks_guard))
    invalid_router_identity = discovery()
    invalid_router_identity_linkerd = invalid_router_identity["linkerd"]
    assert isinstance(invalid_router_identity_linkerd, dict)
    invalid_router_identity_linkerd["router_workload_identity_verified"] = False
    expect_rejected(lambda: MODULE.render(template, invalid_router_identity, eks_guard))
    invalid_router_injection = discovery()
    invalid_router_injection_linkerd = invalid_router_injection["linkerd"]
    assert isinstance(invalid_router_injection_linkerd, dict)
    invalid_router_injection_linkerd["router_workload_injection_verified"] = False
    expect_rejected(lambda: MODULE.render(template, invalid_router_injection, eks_guard))
    non_linkerd = {"selection": {"namespace": "tenant-acme", "ingress_namespace": "external-gateway"}, "linkerd": {"requested": False}}
    non_linkerd_rendered = MODULE.render(template, non_linkerd, eks_guard)
    if "kubernetes.io/metadata.name: external-gateway" not in non_linkerd_rendered:
        raise AssertionError("renderer rejected or changed the explicit non-Linkerd ingress namespace")
    expect_rejected(lambda: MODULE.render(template, discovery(), eks_guard.replace("  ingress: []", "  ingress:\n    - from: []")))
    expect_rejected(lambda: MODULE.render(template, discovery(), eks_guard.replace("smart-llmrouter-restrict-ingress", "wrong-ingress-guard")))
    with tempfile.TemporaryDirectory() as directory:
        output = Path(directory) / "tenant-ingress-network-policy.yaml"
        MODULE.atomic_write(output, rendered)
        if output.read_text(encoding="utf-8") != rendered:
            raise AssertionError("renderer did not atomically persist the ingress NetworkPolicy")
        if stat.S_IMODE(output.stat().st_mode) != 0o600:
            raise AssertionError("rendered ingress NetworkPolicy must be written with mode 0600")
    assert_make_renders_both_policies()
    assert_make_renders_non_linkerd_ingress_policy()
    print("Ingress NetworkPolicy render tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
