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
from copy import deepcopy
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
        "--ingress-namespace", "gateway-system",
        "--ecr-repository", "approved-router-repository",
        "--output", str(output),
    ]


def stale_discovery_report() -> str:
    return json.dumps({
        "schema_version": 1,
        "generated_at": "2026-07-20T00:00:00+00:00",
        "intent": MODULE.DISCOVERY_REPORT_INTENT,
        "selection": {},
        "aws_identity": {},
        "eks": {},
        "namespace": {},
        "cluster_resources": {},
        "linkerd": {},
        "ecr": {},
        "evidence": {},
    }) + "\n"


def test_stale_output_is_invalidated_before_first_probe(root: Path) -> None:
    output = root / "eks-discovery.json"
    output.write_text(stale_discovery_report(), encoding="utf-8")
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
    output.write_text(stale_discovery_report(), encoding="utf-8")
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
    output.write_text(stale_discovery_report(), encoding="utf-8")
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
        "EKS_INGRESS_NAMESPACE": "gateway-system",
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
    output.write_text(stale_discovery_report(), encoding="utf-8")
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


def test_oversized_report_is_rejected_before_publish(root: Path) -> None:
    output = root / "oversized-eks-discovery.json"
    # The serialized JSON is deliberately over the byte ceiling, not merely the
    # character ceiling, so a future Unicode-only report cannot bypass it.
    oversized = {"unicode": "é" * MODULE.MAX_DISCOVERY_REPORT_BYTES}
    try:
        MODULE.atomic_write_report(output, oversized)
    except MODULE.DiscoveryError as exc:
        if str(exc) != "discovery report exceeds the safe maximum size":
            raise
    else:
        raise AssertionError("oversized discovery report was published")
    if output.exists():
        raise AssertionError("oversized discovery report created a reusable output")
    if any(path.name.startswith(f".{output.name}.") and path.name != f".{output.name}.lock" for path in root.iterdir()):
        raise AssertionError("oversized discovery report created a temporary file")


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


def test_unrecognized_existing_output_is_preserved_before_probes(root: Path) -> None:
    output = root / "credentials"
    original_content = "[default]\naws_access_key_id = not-a-discovery-report\n"
    output.write_text(original_content, encoding="utf-8")
    original_argv = sys.argv
    original_which = MODULE.shutil.which
    sys.argv = discovery_argv(output)
    MODULE.shutil.which = lambda command: (_ for _ in ()).throw(AssertionError("tool probe ran before output safety validation"))  # type: ignore[method-assign]
    try:
        try:
            MODULE.main()
        except MODULE.DiscoveryError as exc:
            if str(exc) != "existing discovery report is not recognizable and will not be replaced":
                raise
        else:
            raise AssertionError("unrecognized existing output was accepted")
    finally:
        sys.argv = original_argv
        MODULE.shutil.which = original_which  # type: ignore[method-assign]
    if output.read_text(encoding="utf-8") != original_content:
        raise AssertionError("unrecognized existing output was modified or deleted")


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
        "metadata": {"name": "ingress-controller", "uid": "deployment-uid"},
        "spec": {
            "replicas": 2,
            "template": {
                "metadata": {"annotations": {"linkerd.io/inject": "enabled"}},
                "spec": {"serviceAccountName": "ingress-proxy"},
            },
        },
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


def test_ingress_workload_requires_durable_linkerd_injection() -> None:
    deployment, _, _ = ingress_workload_payloads()
    namespace = {"metadata": {"annotations": {}}}
    evidence = MODULE.ingress_workload_injection_evidence(deployment, namespace)
    if evidence != {
        "ingress_workload_kind": "Deployment",
        "ingress_workload_name": "ingress-controller",
        "ingress_workload_injection_verified": True,
        "ingress_workload_injection_source": "deployment-template",
    }:
        raise AssertionError("ingress Deployment template injection did not produce bounded durable evidence")
    deployment, _, _ = ingress_workload_payloads()
    del deployment["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"]  # type: ignore[index]
    namespace["metadata"]["annotations"]["linkerd.io/inject"] = "enabled"  # type: ignore[index]
    evidence = MODULE.ingress_workload_injection_evidence(deployment, namespace)
    if evidence.get("ingress_workload_injection_source") != "namespace":
        raise AssertionError("namespace-level ingress injection was not accepted as durable evidence")
    deployment, _, _ = ingress_workload_payloads()
    deployment["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"] = "ingress"  # type: ignore[index]
    evidence = MODULE.ingress_workload_injection_evidence(deployment, {"metadata": {"annotations": {}}})
    if evidence.get("ingress_workload_injection_source") != "deployment-template":
        raise AssertionError("Linkerd ingress-mode template injection was not accepted for the ingress Deployment")
    deployment, _, _ = ingress_workload_payloads()
    deployment["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"] = "disabled"  # type: ignore[index]
    namespace["metadata"]["annotations"]["linkerd.io/inject"] = "enabled"  # type: ignore[index]
    try:
        MODULE.ingress_workload_injection_evidence(deployment, namespace)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress Deployment template opt-out overrode durable Linkerd injection evidence")
    deployment, _, _ = ingress_workload_payloads()
    deployment["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"] = "unexpected"  # type: ignore[index]
    try:
        MODULE.ingress_workload_injection_evidence(deployment, namespace)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress Deployment template accepted an unrecognized Linkerd injection setting")
    deployment, _, _ = ingress_workload_payloads()
    del deployment["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"]  # type: ignore[index]
    try:
        MODULE.ingress_workload_injection_evidence(deployment, {"metadata": {"annotations": {}}})
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("ingress workload without namespace or template injection was accepted")


def router_workload_payloads() -> tuple[dict[str, object], dict[str, object], dict[str, object]]:
    deployment = {
        "metadata": {"name": "smart-llmrouter", "uid": "router-deployment-uid"},
        "spec": {
            "replicas": 2,
            "template": {
                "metadata": {"annotations": {"linkerd.io/inject": "enabled"}},
                "spec": {"serviceAccountName": "router"},
            },
        },
        "status": {"availableReplicas": 2},
    }
    replica_sets = {
        "items": [{
            "metadata": {
                "name": "smart-llmrouter-abc123",
                "uid": "router-replicaset-uid",
                "ownerReferences": [{"kind": "Deployment", "name": "smart-llmrouter", "uid": "router-deployment-uid", "controller": True}],
            },
        }],
    }
    pods = {
        "items": [
            {
                "metadata": {"ownerReferences": [{"kind": "ReplicaSet", "name": "smart-llmrouter-abc123", "uid": "router-replicaset-uid", "controller": True}]},
                "spec": {
                    "serviceAccountName": "router",
                    "containers": [{
                        "name": "linkerd-proxy",
                        "env": [{"name": "_l5d_trustdomain", "value": "mesh.example"}, {
                            "name": "LINKERD2_PROXY_IDENTITY_LOCAL_NAME",
                            "value": "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.mesh.example",
                        }],
                    }],
                },
                "status": {
                    "conditions": [{"type": "Ready", "status": "True"}],
                    "containerStatuses": [{"name": "router", "ready": True}, {"name": "linkerd-proxy", "ready": True}],
                },
            },
            {
                "metadata": {"ownerReferences": [{"kind": "ReplicaSet", "name": "smart-llmrouter-abc123", "uid": "router-replicaset-uid", "controller": True}]},
                "spec": {
                    "serviceAccountName": "router",
                    "containers": [{
                        "name": "linkerd-proxy",
                        "env": [{"name": "_l5d_trustdomain", "value": "mesh.example"}, {
                            "name": "LINKERD2_PROXY_IDENTITY_LOCAL_NAME",
                            "value": "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.mesh.example",
                        }],
                    }],
                },
                "status": {
                    "conditions": [{"type": "Ready", "status": "True"}],
                    "containerStatuses": [{"name": "router", "ready": True}, {"name": "linkerd-proxy", "ready": True}],
                },
            },
        ],
    }
    return deployment, replica_sets, pods


def test_router_workload_requires_ready_identity_matched_pods() -> None:
    deployment, replica_sets, pods = router_workload_payloads()
    evidence = MODULE.router_workload_evidence(deployment, replica_sets, pods, "gateway-system", "linkerd", "mesh.example")
    if evidence != {
        "router_workload_verified": True,
        "router_workload_mesh_ready": True,
        "router_workload_identity_verified": True,
        "router_workload_ready_pods": 2,
    }:
        raise AssertionError("ready identity-matched router Pods did not produce bounded scalar evidence")
    pods["items"][0]["status"]["containerStatuses"][1]["ready"] = False  # type: ignore[index]
    try:
        MODULE.router_workload_evidence(deployment, replica_sets, pods, "gateway-system", "linkerd", "mesh.example")
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router workload verification accepted a Pod without a ready Linkerd proxy")
    deployment, replica_sets, pods = router_workload_payloads()
    for pod in pods["items"]:  # type: ignore[index]
        pod["spec"]["containers"][0]["env"][0]["value"] = "wrong.example"  # type: ignore[index]
        pod["spec"]["containers"][0]["env"][1]["value"] = "$(_pod_sa).$(_pod_ns).serviceaccount.identity.linkerd.wrong.example"  # type: ignore[index]
    try:
        MODULE.router_workload_evidence(deployment, replica_sets, pods, "gateway-system", "linkerd", "mesh.example")
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router workload verification accepted a proxy from a different Linkerd trust domain")
    deployment, replica_sets, pods = router_workload_payloads()
    for pod in pods["items"]:  # type: ignore[index]
        pod["spec"]["containers"][0]["env"][1]["value"] = "$(_pod_sa).$(_pod_ns).serviceaccount.identity.other-linkerd.mesh.example"  # type: ignore[index]
    try:
        MODULE.router_workload_evidence(deployment, replica_sets, pods, "gateway-system", "linkerd", "mesh.example")
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router workload verification accepted a proxy from a different Linkerd control plane")
    deployment, replica_sets, pods = router_workload_payloads()
    decoy = deepcopy(pods["items"][0])  # type: ignore[index]
    decoy["metadata"]["ownerReferences"][0]["uid"] = "decoy-replicaset-uid"  # type: ignore[index]
    pods["items"].append(decoy)  # type: ignore[index]
    try:
        MODULE.router_workload_evidence(deployment, replica_sets, pods, "gateway-system", "linkerd", "mesh.example")
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router workload verification accepted a label-selected Pod not owned by the selected Deployment")
    deployment, replica_sets, pods = router_workload_payloads()
    dotted_service_account = "router.runtime"
    deployment["spec"]["template"]["spec"]["serviceAccountName"] = dotted_service_account  # type: ignore[index]
    for pod in pods["items"]:  # type: ignore[index]
        pod["spec"]["serviceAccountName"] = dotted_service_account  # type: ignore[index]
        pod["spec"]["containers"][0]["env"][1]["value"] = f"{dotted_service_account}.gateway-system.serviceaccount.identity.linkerd.mesh.example"  # type: ignore[index]
    dotted_evidence = MODULE.router_workload_evidence(deployment, replica_sets, pods, "gateway-system", "linkerd", "mesh.example")
    if dotted_evidence.get("router_workload_identity_verified") is not True:
        raise AssertionError("router workload rejected a valid DNS-subdomain service-account name")


def router_deployment_payloads() -> tuple[dict[str, object], dict[str, object]]:
    deployment, _, _ = router_workload_payloads()
    deployments = {"items": [deployment]}
    namespace = {"metadata": {"annotations": {}}}
    return deployments, namespace


def test_router_workload_requires_durable_linkerd_injection() -> None:
    deployments, namespace = router_deployment_payloads()
    evidence = MODULE.router_workload_injection_evidence(MODULE.selected_router_deployment(deployments), namespace)
    if evidence != {
        "router_workload_kind": "Deployment",
        "router_workload_name": "smart-llmrouter",
        "router_workload_injection_verified": True,
        "router_workload_injection_source": "deployment-template",
    }:
        raise AssertionError("router Deployment template injection did not produce bounded durable evidence")
    deployments, namespace = router_deployment_payloads()
    del deployments["items"][0]["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"]  # type: ignore[index]
    namespace["metadata"]["annotations"]["linkerd.io/inject"] = "enabled"  # type: ignore[index]
    evidence = MODULE.router_workload_injection_evidence(MODULE.selected_router_deployment(deployments), namespace)
    if evidence.get("router_workload_injection_source") != "namespace":
        raise AssertionError("namespace-level Linkerd injection was not accepted as durable evidence")
    deployments, namespace = router_deployment_payloads()
    deployments["items"][0]["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"] = "disabled"  # type: ignore[index]
    namespace["metadata"]["annotations"]["linkerd.io/inject"] = "enabled"  # type: ignore[index]
    try:
        MODULE.router_workload_injection_evidence(MODULE.selected_router_deployment(deployments), namespace)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router Deployment template opt-out overrode durable Linkerd injection evidence")
    deployments, namespace = router_deployment_payloads()
    deployments["items"][0]["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"] = "unexpected"  # type: ignore[index]
    namespace["metadata"]["annotations"]["linkerd.io/inject"] = "enabled"  # type: ignore[index]
    try:
        MODULE.router_workload_injection_evidence(MODULE.selected_router_deployment(deployments), namespace)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router Deployment template accepted an unrecognized Linkerd injection setting")
    deployments, namespace = router_deployment_payloads()
    deployments["items"][0]["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"] = "ingress"  # type: ignore[index]
    namespace["metadata"]["annotations"]["linkerd.io/inject"] = "enabled"  # type: ignore[index]
    try:
        MODULE.router_workload_injection_evidence(MODULE.selected_router_deployment(deployments), namespace)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router Deployment accepted ingress-only Linkerd injection mode")
    deployments, namespace = router_deployment_payloads()
    del deployments["items"][0]["spec"]["template"]["metadata"]["annotations"]["linkerd.io/inject"]  # type: ignore[index]
    try:
        MODULE.router_workload_injection_evidence(MODULE.selected_router_deployment(deployments), namespace)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("router workload without namespace or template injection was accepted")
    deployments, namespace = router_deployment_payloads()
    deployments["items"].append(deployments["items"][0].copy())  # type: ignore[index]
    try:
        MODULE.selected_router_deployment(deployments)
    except MODULE.DiscoveryError:
        pass
    else:
        raise AssertionError("multiple selected router Deployments were accepted as one durable workload")


def test_namespace_report_evidence_is_scrubbed() -> None:
    sentinel = "https://private.example/secret-like-value"
    namespace = {
        "metadata": {
            "labels": {"example.com/customer-note": sentinel},
            "annotations": {"linkerd.io/inject": sentinel},
        },
    }
    evidence = MODULE.namespace_discovery_evidence(namespace)
    if evidence != {"linkerd_injection_annotation_state": "other"}:
        raise AssertionError("namespace report evidence did not normalize an unknown Linkerd injection annotation")
    serialized = json.dumps({"namespace": evidence, "linkerd": {"namespace_injection_annotation_state": evidence["linkerd_injection_annotation_state"]}})
    if sentinel in serialized or "customer-note" in serialized:
        raise AssertionError("scrubbed discovery evidence retained raw namespace annotation or label data")


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
        test_oversized_report_is_rejected_before_publish(root)
        test_unsafe_output_paths_are_rejected_without_deletion(root)
        test_unrecognized_existing_output_is_preserved_before_probes(root)
        test_output_lock_is_exclusive(root)
        test_ingress_workload_requires_actual_meshed_service_account()
        test_ingress_workload_requires_durable_linkerd_injection()
        test_router_workload_requires_ready_identity_matched_pods()
        test_router_workload_requires_durable_linkerd_injection()
        test_namespace_report_evidence_is_scrubbed()
        test_linkerd_workload_names_accept_dns_subdomains()
    print("EKS discovery diagnostic safeguards passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
