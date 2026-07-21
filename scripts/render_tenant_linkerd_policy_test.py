#!/usr/bin/env python3
"""Regression tests for validated Linkerd policy rendering."""

from __future__ import annotations

import importlib.util
import stat
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("render_tenant_linkerd_policy", ROOT / "scripts/render_tenant_linkerd_policy.py")
if SPEC is None or SPEC.loader is None:
    raise SystemExit("cannot load Linkerd renderer")
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
            "router_workload_verified": True,
            "router_workload_mesh_ready": True,
            "router_workload_identity_verified": True,
            "ingress_namespace": "gateway-system",
            "ingress_service_account": "gateway.proxy",
            "control_plane_namespace": "linkerd-control",
            "trust_domain": "mesh.example",
            "ingress_identity": "gateway.proxy.gateway-system.serviceaccount.identity.linkerd-control.mesh.example",
        },
    }


def main() -> int:
    template = (ROOT / "deploy/kubernetes/bootstrap/tenant-linkerd-policy.example.yaml").read_text(encoding="utf-8")
    rendered = MODULE.render(template, discovery())
    if "__TENANT_NAMESPACE__" in rendered or "__LINKERD_INGRESS_IDENTITY__" in rendered:
        raise AssertionError("rendered policy retained a placeholder")
    if "namespace: tenant-acme" not in rendered:
        raise AssertionError("renderer did not derive the tenant namespace from discovery")
    expected = "gateway.proxy.gateway-system.serviceaccount.identity.linkerd-control.mesh.example"
    if expected not in rendered:
        raise AssertionError("renderer did not derive the Linkerd identity from validated discovery values")
    invalid = discovery()
    invalid_linkerd = invalid["linkerd"]
    assert isinstance(invalid_linkerd, dict)
    invalid_linkerd["ingress_identity"] = "wrong.identity"
    try:
        MODULE.render(template, invalid)
    except ValueError:
        pass
    else:
        raise AssertionError("renderer accepted an identity not derived from validated discovery values")
    invalid_mesh = discovery()
    invalid_mesh_linkerd = invalid_mesh["linkerd"]
    assert isinstance(invalid_mesh_linkerd, dict)
    invalid_mesh_linkerd["ingress_workload_mesh_ready"] = False
    try:
        MODULE.render(template, invalid_mesh)
    except ValueError:
        pass
    else:
        raise AssertionError("renderer accepted identity evidence without a ready meshed ingress workload")
    invalid_router = discovery()
    invalid_router_linkerd = invalid_router["linkerd"]
    assert isinstance(invalid_router_linkerd, dict)
    invalid_router_linkerd["router_workload_mesh_ready"] = False
    try:
        MODULE.render(template, invalid_router)
    except ValueError:
        pass
    else:
        raise AssertionError("renderer accepted identity evidence without a ready meshed router workload")
    invalid_router_identity = discovery()
    invalid_router_identity_linkerd = invalid_router_identity["linkerd"]
    assert isinstance(invalid_router_identity_linkerd, dict)
    invalid_router_identity_linkerd["router_workload_identity_verified"] = False
    try:
        MODULE.render(template, invalid_router_identity)
    except ValueError:
        pass
    else:
        raise AssertionError("renderer accepted router evidence without a verified Linkerd identity and trust domain")
    invalid_control_plane = discovery()
    invalid_control_plane_linkerd = invalid_control_plane["linkerd"]
    assert isinstance(invalid_control_plane_linkerd, dict)
    invalid_control_plane_linkerd["control_plane_namespace"] = "different-linkerd"
    try:
        MODULE.render(template, invalid_control_plane)
    except ValueError:
        pass
    else:
        raise AssertionError("renderer accepted an identity that does not match its verified Linkerd control-plane namespace")
    with tempfile.TemporaryDirectory() as directory:
        output = Path(directory) / "tenant-linkerd-policy.yaml"
        MODULE.atomic_write(output, rendered)
        if output.read_text(encoding="utf-8") != rendered:
            raise AssertionError("renderer did not atomically persist the rendered policy")
        if stat.S_IMODE(output.stat().st_mode) != 0o600:
            raise AssertionError("rendered policy must be written with mode 0600")
    print("Linkerd policy render tests passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
