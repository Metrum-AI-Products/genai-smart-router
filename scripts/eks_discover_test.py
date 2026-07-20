#!/usr/bin/env python3
"""Regression tests for safe EKS discovery diagnostics."""

from __future__ import annotations

import importlib.util
import json
import os
import stat
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("eks_discover", ROOT / "scripts" / "eks_discover.py")
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def test_command_failure_never_copies_stderr() -> None:
    original = MODULE.subprocess.check_output
    MODULE.subprocess.check_output = lambda *args, **kwargs: (_ for _ in ()).throw(  # type: ignore[method-assign]
        subprocess.CalledProcessError(1, args[0], stderr="private-endpoint.example Authorization: Bearer test-value")
    )
    try:
        try:
            MODULE.run(["aws"])
        except MODULE.DiscoveryError as exc:
            assert str(exc) == "aws command failed"
            assert "private-endpoint" not in str(exc)
            assert "Bearer" not in str(exc)
        else:
            raise AssertionError("expected a sanitized discovery error")
    finally:
        MODULE.subprocess.check_output = original  # type: ignore[method-assign]


def discovery_argv(output: Path) -> list[str]:
    return [
        "eks_discover.py",
        "--profile", "genai-smart-router-eks-discovery",
        "--account-id", "123456789012",
        "--region", "us-east-1",
        "--cluster", "approved-cluster",
        "--namespace", "tenant-acme",
        "--ecr-repository", "approved-router-repository",
        "--output", str(output),
    ]


def test_stale_output_is_invalidated_before_first_probe(root: Path) -> None:
    output = root / "eks-discovery.json"
    output.write_text('{"schema_version": 1, "stale": true}\n', encoding="utf-8")
    original_argv = sys.argv
    original_which = MODULE.shutil.which
    original_aws_json = MODULE.aws_json
    sys.argv = discovery_argv(output)
    MODULE.shutil.which = lambda command: f"/test/{command}"  # type: ignore[method-assign]

    def fail_first_probe(args, region, profile):
        if output.exists():
            raise AssertionError("previous discovery evidence survived until the first live probe")
        raise MODULE.DiscoveryError("simulated first probe failure")

    MODULE.aws_json = fail_first_probe  # type: ignore[method-assign]
    try:
        try:
            MODULE.main()
        except MODULE.DiscoveryError as exc:
            if str(exc) != "simulated first probe failure":
                raise
        else:
            raise AssertionError("failed discovery must not report success")
    finally:
        sys.argv = original_argv
        MODULE.shutil.which = original_which  # type: ignore[method-assign]
        MODULE.aws_json = original_aws_json  # type: ignore[method-assign]
    if output.exists():
        raise AssertionError("failed discovery left stale evidence reusable at the selected path")


def test_stale_output_is_invalidated_when_tools_are_missing(root: Path) -> None:
    output = root / "eks-discovery-no-tools.json"
    output.write_text('{"schema_version": 1, "stale": true}\n', encoding="utf-8")
    original_argv = sys.argv
    original_which = MODULE.shutil.which
    sys.argv = discovery_argv(output)
    MODULE.shutil.which = lambda command: None  # type: ignore[method-assign]
    try:
        try:
            MODULE.main()
        except MODULE.DiscoveryError as exc:
            if str(exc) != "aws and kubectl are both required":
                raise
        else:
            raise AssertionError("missing discovery tools must fail")
    finally:
        sys.argv = original_argv
        MODULE.shutil.which = original_which  # type: ignore[method-assign]
    if output.exists():
        raise AssertionError("missing tools left stale evidence reusable at the selected path")


def test_make_discover_invalidates_before_identity_failure(root: Path) -> None:
    """The canonical Make target must enter the script lock before its role probe."""
    output = root / "make-discovery" / "eks-discovery.json"
    output.parent.mkdir()
    output.write_text('{"schema_version": 1, "stale": true}\n', encoding="utf-8")
    command_directory = root / "commands"
    command_directory.mkdir()
    for command in ("aws", "kubectl"):
        path = command_directory / command
        path.write_text("#!/bin/sh\nexit 1\n", encoding="utf-8")
        path.chmod(0o700)
    environment = {
        **os.environ,
        "PATH": f"{command_directory}:{os.environ.get('PATH', '')}",
        "EKS_AWS_PROFILE": "genai-smart-router-eks-discovery",
        "EKS_ACCOUNT_ID": "123456789012",
        "EKS_REGION": "us-east-1",
        "EKS_CLUSTER": "approved-cluster",
        "EKS_NAMESPACE": "tenant-acme",
        "EKS_LINKERD_NAMESPACE": "",
        "EKS_INGRESS_NAMESPACE": "",
        "EKS_INGRESS_SERVICE_ACCOUNT": "",
        "EKS_INGRESS_DEPLOYMENT": "",
        "EKS_LINKERD_TRUST_DOMAIN": "",
        "EKS_ECR_REPOSITORY": "approved-router-repository",
        "EKS_DISCOVERY_OUTPUT": str(output),
    }
    completed = subprocess.run(
        ["make", "eks-discover"],
        cwd=ROOT,
        env=environment,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode == 0:
        raise AssertionError("simulated identity failure unexpectedly completed discovery")
    if "EKS discovery failed safely: aws command failed" not in completed.stderr:
        raise AssertionError("Make discovery did not reach the locked discovery identity check")
    if output.exists():
        raise AssertionError("Make identity failure left stale evidence reusable at the selected path")


def test_atomic_report_publish(root: Path) -> None:
    output = root / "eks-discovery.json"
    MODULE.invalidate_output(output)
    report = {"schema_version": 1, "selection": {"cluster": "approved-cluster"}}
    MODULE.atomic_write_report(output, report)
    if json.loads(output.read_text(encoding="utf-8")) != report:
        raise AssertionError("discovery report publication changed the report content")
    if stat.S_IMODE(output.stat().st_mode) != 0o600:
        raise AssertionError("discovery report must be published with mode 0600")
    if any(path.name.startswith(f".{output.name}.") and path.name != f".{output.name}.lock" for path in root.iterdir()):
        raise AssertionError("discovery report temporary file was not removed")


def test_failed_publish_leaves_no_report(root: Path) -> None:
    output = root / "eks-discovery.json"
    output.write_text('{"schema_version": 1, "stale": true}\n', encoding="utf-8")
    MODULE.invalidate_output(output)
    original_replace = MODULE.os.replace
    MODULE.os.replace = lambda source, destination: (_ for _ in ()).throw(OSError("simulated publish failure"))  # type: ignore[method-assign]
    try:
        try:
            MODULE.atomic_write_report(output, {"schema_version": 1})
        except MODULE.DiscoveryError as exc:
            if str(exc) != "discovery report could not be published safely":
                raise
        else:
            raise AssertionError("simulated publish failure must fail closed")
    finally:
        MODULE.os.replace = original_replace  # type: ignore[method-assign]
    if output.exists():
        raise AssertionError("failed report publication left a reusable report")
    if any(path.name.startswith(f".{output.name}.") and path.name != f".{output.name}.lock" for path in root.iterdir()):
        raise AssertionError("failed report publication left a temporary file")


def test_unsafe_output_paths_are_rejected_without_deletion(root: Path) -> None:
    repository_file = ROOT / "scripts" / "eks_discover.py"
    original_repository_content = repository_file.read_text(encoding="utf-8")
    unsafe_paths = [repository_file]
    directory = root / "output-directory"
    directory.mkdir()
    unsafe_paths.append(directory)
    target = root / "target.json"
    target.write_text("do not delete\n", encoding="utf-8")
    symlink = root / "output-link.json"
    symlink.symlink_to(target)
    unsafe_paths.append(symlink)
    for path in unsafe_paths:
        try:
            MODULE.validate_output_path(path)
        except MODULE.DiscoveryError:
            pass
        else:
            raise AssertionError(f"unsafe direct discovery output path was accepted: {path}")
    if repository_file.read_text(encoding="utf-8") != original_repository_content:
        raise AssertionError("unsafe discovery path validation modified repository content")
    if not directory.is_dir() or target.read_text(encoding="utf-8") != "do not delete\n" or not symlink.is_symlink():
        raise AssertionError("unsafe discovery path validation deleted an existing target")


def test_output_lock_is_exclusive(root: Path) -> None:
    output = root / "eks-discovery.json"
    MODULE.validate_output_path(output)
    with MODULE.output_lock(output):
        try:
            with MODULE.output_lock(output):
                pass
        except MODULE.DiscoveryError:
            pass
        else:
            raise AssertionError("a second local discovery run acquired the same output lock")


def ingress_workload_payloads() -> tuple[dict[str, object], dict[str, object], dict[str, object]]:
    deployment = {
        "metadata": {"uid": "deployment-uid"},
        "spec": {"replicas": 2, "template": {"spec": {"serviceAccountName": "ingress-proxy"}}},
        "status": {"availableReplicas": 2},
    }
    replica_sets = {
        "items": [{
            "metadata": {
                "name": "ingress-controller-abc123",
                "uid": "replicaset-uid",
                "ownerReferences": [{"kind": "Deployment", "name": "ingress-controller", "uid": "deployment-uid", "controller": True}],
            },
        }],
    }
    pods = {
        "items": [
            {
                "metadata": {"ownerReferences": [{"kind": "ReplicaSet", "name": "ingress-controller-abc123", "uid": "replicaset-uid", "controller": True}]},
                "spec": {
                    "serviceAccountName": "ingress-proxy",
                    "containers": [{
                        "name": "linkerd-proxy",
                        "env": [{
                            "name": "_l5d_trustdomain",
                            "value": "mesh.example",
                        }, {
                            "name": "LINKERD2_PROXY_IDENTITY_LOCAL_NAME",
                            "value": "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.mesh.example",
                        }],
                    }],
                },
                "status": {
                    "conditions": [{"type": "Ready", "status": "True"}],
                    "containerStatuses": [{"name": "controller", "ready": True}, {"name": "linkerd-proxy", "ready": True}],
                },
            },
            {
                "metadata": {"ownerReferences": [{"kind": "ReplicaSet", "name": "ingress-controller-abc123", "uid": "replicaset-uid", "controller": True}]},
                "spec": {
                    "serviceAccountName": "ingress-proxy",
                    "containers": [{
                        "name": "linkerd-proxy",
                        "env": [{
                            "name": "_l5d_trustdomain",
                            "value": "mesh.example",
                        }, {
                            "name": "LINKERD2_PROXY_IDENTITY_LOCAL_NAME",
                            "value": "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.mesh.example",
                        }],
                    }],
                },
                "status": {
                    "conditions": [{"type": "Ready", "status": "True"}],
                    "containerStatuses": [{"name": "controller", "ready": True}, {"name": "linkerd-proxy", "ready": True}],
                },
            },
        ],
    }
    return deployment, replica_sets, pods


def test_ingress_workload_requires_actual_meshed_service_account() -> None:
    deployment, replica_sets, pods = ingress_workload_payloads()
    evidence = MODULE.ingress_workload_evidence(
        deployment,
        replica_sets,
        pods,
        "ingress-controller",
        "gateway-system",
        "ingress-proxy",
        "linkerd",
        "mesh.example",
    )
    if evidence != {
        "ingress_workload_kind": "Deployment",
        "ingress_workload_name": "ingress-controller",
        "ingress_workload_desired_replicas": 2,
        "ingress_workload_ready_pods": 2,
        "ingress_workload_verified": True,
        "ingress_workload_mesh_ready": True,
    }:
        raise AssertionError("ready Linkerd ingress deployment did not produce bounded scalar evidence")
    deployment["spec"]["template"]["spec"]["serviceAccountName"] = "different-service-account"  # type: ignore[index]
    try:
        MODULE.ingress_workload_evidence(
            deployment,
            replica_sets,
            pods,
            "ingress-controller",
            "gateway-system",
            "ingress-proxy",
            "linkerd",
            "mesh.example",
        )
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress identity verification accepted a deployment with a different service account")
    deployment, replica_sets, pods = ingress_workload_payloads()
    pods["items"][0]["status"]["containerStatuses"][1]["ready"] = False  # type: ignore[index]
    try:
        MODULE.ingress_workload_evidence(
            deployment,
            replica_sets,
            pods,
            "ingress-controller",
            "gateway-system",
            "ingress-proxy",
            "linkerd",
            "mesh.example",
        )
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress identity verification accepted a Pod without a ready Linkerd proxy")
    deployment, replica_sets, pods = ingress_workload_payloads()
    for pod in pods["items"]:  # type: ignore[index]
        pod["spec"]["containers"][0]["env"][0]["value"] = "wrong.example"  # type: ignore[index]
        pod["spec"]["containers"][0]["env"][1]["value"] = "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.wrong.example"  # type: ignore[index]
    try:
        MODULE.ingress_workload_evidence(
            deployment,
            replica_sets,
            pods,
            "ingress-controller",
            "gateway-system",
            "ingress-proxy",
            "linkerd",
            "mesh.example",
        )
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress identity verification accepted a Pod with a mismatched Linkerd trust domain")
    deployment, replica_sets, pods = ingress_workload_payloads()
    pods["items"][0]["spec"]["containers"][0]["env"][0]["value"] = "different.example"  # type: ignore[index]
    pods["items"][0]["spec"]["containers"][0]["env"][1]["value"] = "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.different.example"  # type: ignore[index]
    try:
        MODULE.ingress_workload_evidence(
            deployment,
            replica_sets,
            pods,
            "ingress-controller",
            "gateway-system",
            "ingress-proxy",
            "linkerd",
            "mesh.example",
        )
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress identity verification accepted inconsistent Linkerd trust-domain evidence")


def test_linkerd_workload_names_accept_dns_subdomains() -> None:
    maximum_length_name = f"{'a' * 63}.{'b' * 63}.{'c' * 63}.{'d' * 61}"
    if not MODULE.is_dns_subdomain(maximum_length_name):
        raise AssertionError("Kubernetes DNS-subdomain workload name at the 253-character limit was rejected")
    if MODULE.is_dns_subdomain(f"{'a' * 64}.example") or MODULE.is_dns_subdomain("invalid..example"):
        raise AssertionError("invalid Kubernetes DNS-subdomain workload name was accepted")


def main() -> int:
    test_command_failure_never_copies_stderr()
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        test_stale_output_is_invalidated_before_first_probe(root)
        test_stale_output_is_invalidated_when_tools_are_missing(root)
        test_make_discover_invalidates_before_identity_failure(root)
        test_atomic_report_publish(root)
        test_failed_publish_leaves_no_report(root)
        test_unsafe_output_paths_are_rejected_without_deletion(root)
        test_output_lock_is_exclusive(root)
        test_ingress_workload_requires_actual_meshed_service_account()
        test_linkerd_workload_names_accept_dns_subdomains()
    print("EKS discovery diagnostic safeguards passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
