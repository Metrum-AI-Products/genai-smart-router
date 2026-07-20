#!/usr/bin/env python3
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
        "selection": {"namespace": "tenant-acme"},
        "linkerd": {
            "requested": True,
            "ingress_identity_verified": True,
            "ingress_workload_verified": True,
            "ingress_workload_mesh_ready": True,
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


def main() -> int:
    template = (ROOT / "deploy/kubernetes/bootstrap/tenant-ingress-network-policy.example.yaml").read_text(encoding="utf-8")
    base = (ROOT / "deploy/kubernetes/base/networkpolicy.yaml").read_text(encoding="utf-8")
    rendered = MODULE.render(template, discovery(), base)
    if "namespace: tenant-acme" not in rendered or "kubernetes.io/metadata.name: gateway-system" not in rendered:
        raise AssertionError("renderer did not bind the NetworkPolicy to verified discovery namespaces")
    if "ingress-nginx" in rendered:
        raise AssertionError("rendered NetworkPolicy retained a fixed ingress namespace")
    invalid = discovery()
    invalid_linkerd = invalid["linkerd"]
    assert isinstance(invalid_linkerd, dict)
    invalid_linkerd["ingress_workload_mesh_ready"] = False
    expect_rejected(lambda: MODULE.render(template, invalid, base))
    expect_rejected(lambda: MODULE.render(template, discovery(), base.replace("  ingress: []", "  ingress:\n    - from: []")))
    expect_rejected(lambda: MODULE.render(template, discovery(), base + "\n  ingress:\n    - from: []\n"))
    with tempfile.TemporaryDirectory() as directory:
        output = Path(directory) / "tenant-ingress-network-policy.yaml"
        MODULE.atomic_write(output, rendered)
        if output.read_text(encoding="utf-8") != rendered:
            raise AssertionError("renderer did not atomically persist the ingress NetworkPolicy")
        if stat.S_IMODE(output.stat().st_mode) != 0o600:
            raise AssertionError("rendered ingress NetworkPolicy must be written with mode 0600")
    assert_make_renders_both_policies()
    print("Ingress NetworkPolicy render tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
