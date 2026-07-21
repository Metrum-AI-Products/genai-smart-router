#!/usr/bin/env python3
"""Regression tests for selection-bound tenant policy activation."""

from __future__ import annotations

import importlib.util
import json
import os
import subprocess
import tempfile
from datetime import datetime, timedelta, timezone
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "apply_tenant_network_policies",
    ROOT / "scripts" / "apply_tenant_network_policies.py",
)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def discovery(*, linkerd: bool = False) -> dict[str, object]:
    selection = {
        "account_id": "123456789012",
        "region": "us-east-1",
        "cluster": "approved-cluster",
        "namespace": "tenant-acme",
        "ingress_namespace": "gateway-system",
        "ecr_repository": "approved-router-repository",
    }
    linkerd_evidence: dict[str, object] = {"requested": linkerd}
    if linkerd:
        linkerd_evidence.update(
            {
                "ingress_identity_verified": True,
                "ingress_workload_verified": True,
                "ingress_workload_mesh_ready": True,
                "ingress_workload_injection_verified": True,
                "router_workload_verified": True,
                "router_workload_mesh_ready": True,
                "router_workload_identity_verified": True,
                "router_workload_injection_verified": True,
                "ingress_namespace": "gateway-system",
                "ingress_service_account": "gateway-proxy",
                "control_plane_namespace": "linkerd-control",
                "trust_domain": "mesh.example",
                "ingress_identity": "gateway-proxy.gateway-system.serviceaccount.identity.linkerd-control.mesh.example",
            }
        )
    return {
        "schema_version": 1,
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "intent": MODULE.DISCOVERY_REPORT_INTENT,
        "selection": selection,
        "linkerd": linkerd_evidence,
    }


def write_artifacts(root: Path, report: dict[str, object], *, linkerd: bool = False) -> tuple[Path, Path, Path | None, Path]:
    report_path = root / "eks-discovery.json"
    kubeconfig = root / "approved-kubeconfig"
    ingress_policy = root / "tenant-ingress-network-policy.yaml"
    report_path.write_text(json.dumps(report), encoding="utf-8")
    kubeconfig.write_text("apiVersion: v1\nkind: Config\n", encoding="utf-8")
    ingress_policy.write_text(
        MODULE.INGRESS_RENDERER.render(
            MODULE.INGRESS_RENDERER.DEFAULT_TEMPLATE.read_text(encoding="utf-8"),
            report,
            MODULE.INGRESS_RENDERER.EKS_INGRESS_GUARD_POLICY.read_text(encoding="utf-8"),
        ),
        encoding="utf-8",
    )
    linkerd_policy: Path | None = None
    if linkerd:
        linkerd_policy = root / "tenant-linkerd-policy.yaml"
        linkerd_policy.write_text(
            MODULE.LINKERD_RENDERER.render(
                MODULE.LINKERD_RENDERER.DEFAULT_TEMPLATE.read_text(encoding="utf-8"),
                report,
            ),
            encoding="utf-8",
        )
    return report_path, kubeconfig, linkerd_policy, ingress_policy


def fake_target_run(
    calls: list[list[str]],
    *,
    account_id: str = "123456789012",
    kube_endpoint: str = "https://approved.example",
):
    def fake_run(command: list[str]) -> str:
        calls.append(command)
        if command[0] == "aws" and "sts" in command:
            return json.dumps({"Account": account_id})
        if command[0] == "aws" and "eks" in command:
            return json.dumps({"cluster": {"endpoint": "https://approved.example"}})
        if command[0] == "kubectl" and "config" in command:
            return json.dumps({"clusters": [{"cluster": {"server": kube_endpoint}}]})
        if command[0] == "kubectl" and "apply" in command:
            return "networkpolicy/smart-llmrouter-discovered-ingress configured\n"
        raise AssertionError(f"unexpected command shape: {command!r}")

    return fake_run


def expect_failure(callback, expected: str) -> None:
    try:
        callback()
    except MODULE.PolicyActivationError as exc:
        if str(exc) != expected:
            raise AssertionError(f"expected {expected!r}, got {str(exc)!r}") from exc
    else:
        raise AssertionError("expected activation to fail closed")


def test_explicit_context_is_bound_and_server_dry_run_only(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    calls: list[list[str]] = []
    original_run = MODULE.run
    MODULE.run = fake_target_run(calls)  # type: ignore[method-assign]
    try:
        MODULE.activate(
            report_path,
            "approved-profile",
            kubeconfig,
            "approved-deployment-context",
            ingress_policy,
            linkerd_policy,
            apply=False,
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]
    context_reads = [command for command in calls if command[0] == "kubectl" and "config" in command]
    if len(context_reads) != 1 or context_reads[0][:2] != ["kubectl", "--kubeconfig"] or context_reads[0][3:5] != ["--context", "approved-deployment-context"]:
        raise AssertionError("activation did not inspect a private kubeconfig snapshot with the explicit context")
    kubeconfig_snapshot = Path(context_reads[0][2])
    if kubeconfig_snapshot == kubeconfig or kubeconfig_snapshot.name != "kubeconfig":
        raise AssertionError("activation did not replace the mutable kubeconfig with its private snapshot")
    apply_calls = [command for command in calls if command[0] == "kubectl" and "apply" in command]
    if len(apply_calls) != 1:
        raise AssertionError("non-Linkerd activation did not invoke exactly one policy validation")
    command = apply_calls[0]
    if command[:5] != ["kubectl", "--kubeconfig", str(kubeconfig_snapshot), "--context", "approved-deployment-context"]:
        raise AssertionError("policy validation did not reuse the verified kubeconfig snapshot")
    if "--dry-run=server" not in command or command[-2] != "-f" or Path(command[-1]) == ingress_policy or Path(command[-1]).name != "tenant-ingress-network-policy.yaml":
        raise AssertionError("default activation must server-side dry-run only the private discovery-bound policy snapshot")


def test_cluster_context_mismatch_prevents_any_apply(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    calls: list[list[str]] = []
    original_run = MODULE.run
    MODULE.run = fake_target_run(calls, kube_endpoint="https://different.example")  # type: ignore[method-assign]
    try:
        expect_failure(
            lambda: MODULE.activate(
                report_path,
                "approved-profile",
                kubeconfig,
                "approved-deployment-context",
                ingress_policy,
                linkerd_policy,
                apply=True,
            ),
            "explicit kubeconfig context does not match the selected EKS cluster",
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]
    if any(command[0] == "kubectl" and "apply" in command for command in calls):
        raise AssertionError("a mismatched explicit context reached kubectl apply")


def test_account_mismatch_prevents_any_apply(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    calls: list[list[str]] = []
    original_run = MODULE.run
    MODULE.run = fake_target_run(calls, account_id="210987654321")  # type: ignore[method-assign]
    try:
        expect_failure(
            lambda: MODULE.activate(
                report_path,
                "approved-profile",
                kubeconfig,
                "approved-deployment-context",
                ingress_policy,
                linkerd_policy,
                apply=True,
            ),
            "AWS profile account does not match the discovery selection",
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]
    if any(command[0] == "kubectl" and "apply" in command for command in calls):
        raise AssertionError("an account-mismatched deployment profile reached kubectl apply")


def test_stale_discovery_evidence_prevents_any_target_command(root: Path) -> None:
    now = datetime(2026, 7, 21, tzinfo=timezone.utc)
    report = discovery()
    report["generated_at"] = (now - timedelta(seconds=MODULE.MAX_DISCOVERY_EVIDENCE_AGE_SECONDS + 1)).isoformat()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    original_run = MODULE.run
    MODULE.run = lambda command: (_ for _ in ()).throw(AssertionError("stale evidence must not reach target commands"))  # type: ignore[method-assign]
    try:
        expect_failure(
            lambda: MODULE.activate(
                report_path,
                "approved-profile",
                kubeconfig,
                "approved-deployment-context",
                ingress_policy,
                linkerd_policy,
                apply=True,
                now=now,
            ),
            "discovery report is too old; rerun discovery before policy activation",
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]


def test_linkerd_policy_order_requires_dry_run_before_apply(root: Path) -> None:
    report = discovery(linkerd=True)
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report, linkerd=True)
    assert linkerd_policy is not None
    calls: list[list[str]] = []
    original_run = MODULE.run
    MODULE.run = fake_target_run(calls)  # type: ignore[method-assign]
    try:
        MODULE.activate(
            report_path,
            "approved-profile",
            kubeconfig,
            "approved-deployment-context",
            ingress_policy,
            linkerd_policy,
            apply=True,
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]
    apply_calls = [command for command in calls if command[0] == "kubectl" and "apply" in command]
    if len(apply_calls) != 4:
        raise AssertionError("Linkerd activation must dry-run and apply both discovery-bound policies")
    expected = [
        (True, "tenant-linkerd-policy.yaml"),
        (True, "tenant-ingress-network-policy.yaml"),
        (False, "tenant-linkerd-policy.yaml"),
        (False, "tenant-ingress-network-policy.yaml"),
    ]
    actual = [("--dry-run=server" in command, Path(command[-1]).name) for command in apply_calls]
    if actual != expected or any(Path(command[-1]) in {linkerd_policy, ingress_policy} for command in apply_calls):
        raise AssertionError("Linkerd policy activation did not preserve dry-run and policy ordering")


def test_tampered_policy_is_rejected_before_target_commands(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    ingress_policy.write_text(ingress_policy.read_text(encoding="utf-8") + "\n# tampered\n", encoding="utf-8")
    original_run = MODULE.run
    MODULE.run = lambda command: (_ for _ in ()).throw(AssertionError("target command must not run for tampered policy"))  # type: ignore[method-assign]
    try:
        expect_failure(
            lambda: MODULE.activate(
                report_path,
                "approved-profile",
                kubeconfig,
                "approved-deployment-context",
                ingress_policy,
                linkerd_policy,
                apply=False,
            ),
            "rendered ingress NetworkPolicy does not exactly match the selected discovery evidence",
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]


def test_oversized_policy_is_rejected_before_target_commands(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    ingress_policy.write_bytes(b"x" * (MODULE.MAX_RENDERED_POLICY_BYTES + 1))
    original_run = MODULE.run
    MODULE.run = lambda command: (_ for _ in ()).throw(AssertionError("target command must not run for an oversized policy"))  # type: ignore[method-assign]
    try:
        expect_failure(
            lambda: MODULE.activate(
                report_path,
                "approved-profile",
                kubeconfig,
                "approved-deployment-context",
                ingress_policy,
                linkerd_policy,
                apply=False,
            ),
            "rendered ingress NetworkPolicy exceeds the safe maximum size",
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]


def test_oversized_kubeconfig_is_rejected_before_target_commands(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    kubeconfig.write_bytes(b"x" * (MODULE.MAX_KUBECONFIG_BYTES + 1))
    original_run = MODULE.run
    MODULE.run = lambda command: (_ for _ in ()).throw(AssertionError("target command must not run for an oversized kubeconfig"))  # type: ignore[method-assign]
    try:
        expect_failure(
            lambda: MODULE.activate(
                report_path,
                "approved-profile",
                kubeconfig,
                "approved-deployment-context",
                ingress_policy,
                linkerd_policy,
                apply=False,
            ),
            "Kubernetes kubeconfig exceeds the safe maximum size",
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]


def test_activation_uses_immutable_kubeconfig_and_policy_snapshots(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    expected_policy = ingress_policy.read_text(encoding="utf-8")
    calls: list[list[str]] = []

    def run(command: list[str]) -> str:
        calls.append(command)
        if command[0] == "aws" and "sts" in command:
            return json.dumps({"Account": "123456789012"})
        if command[0] == "aws" and "eks" in command:
            return json.dumps({"cluster": {"endpoint": "https://approved.example"}})
        if command[0] == "kubectl" and "config" in command:
            kubeconfig.write_text("apiVersion: v1\nclusters: changed-after-snapshot\n", encoding="utf-8")
            ingress_policy.write_text("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: tampered\n", encoding="utf-8")
            return json.dumps({"clusters": [{"cluster": {"server": "https://approved.example"}}]})
        if command[0] == "kubectl" and "apply" in command:
            snapshot_kubeconfig = Path(command[2])
            snapshot_policy = Path(command[-1])
            if snapshot_kubeconfig == kubeconfig or snapshot_policy == ingress_policy:
                raise AssertionError("activation passed a mutable caller path to kubectl")
            if snapshot_policy.read_text(encoding="utf-8") != expected_policy:
                raise AssertionError("activation did not apply the renderer-generated policy snapshot")
            return "networkpolicy/smart-llmrouter-discovered-ingress configured\n"
        raise AssertionError(f"unexpected command shape: {command!r}")

    original_run = MODULE.run
    MODULE.run = run  # type: ignore[method-assign]
    try:
        MODULE.activate(
            report_path,
            "approved-profile",
            kubeconfig,
            "approved-deployment-context",
            ingress_policy,
            linkerd_policy,
            apply=True,
        )
    finally:
        MODULE.run = original_run  # type: ignore[method-assign]
    if len([command for command in calls if command[0] == "kubectl" and "apply" in command]) != 2:
        raise AssertionError("snapshot regression did not execute both dry-run and apply")


def test_make_apply_requires_explicit_confirmation(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    completed = subprocess.run(
        ["make", "eks-apply-tenant-network-policies"],
        cwd=ROOT,
        env={
            **os.environ,
            "EKS_DISCOVERY_OUTPUT": str(report_path),
            "EKS_POLICY_AWS_PROFILE": "approved-profile",
            "EKS_POLICY_KUBECONFIG": str(kubeconfig),
            "EKS_POLICY_CONTEXT": "approved-deployment-context",
            "EKS_INGRESS_NETWORK_POLICY_OUTPUT": str(ingress_policy),
            "EKS_LINKERD_POLICY_OUTPUT": "",
        },
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode == 0 or "EKS_POLICY_APPLY_CONFIRM=apply is required" not in completed.stderr:
        raise AssertionError("Make allowed policy mutation without the explicit confirmation gate")


def test_make_validation_binds_the_explicit_context(root: Path) -> None:
    report = discovery()
    report_path, kubeconfig, linkerd_policy, ingress_policy = write_artifacts(root, report)
    assert linkerd_policy is None
    command_directory = root / "commands"
    command_directory.mkdir()
    command_log = root / "commands.log"
    aws = command_directory / "aws"
    kubectl = command_directory / "kubectl"
    aws.write_text(
        "#!/bin/sh\n"
        "case \" $* \" in\n"
        "  *\" sts get-caller-identity \"*) printf '%s\\n' '{\"Account\":\"123456789012\"}' ;;\n"
        "  *\" eks describe-cluster \"*) printf '%s\\n' '{\"cluster\":{\"endpoint\":\"https://approved.example\"}}' ;;\n"
        "  *) exit 64 ;;\n"
        "esac\n",
        encoding="utf-8",
    )
    kubectl.write_text(
        "#!/bin/sh\n"
        "printf '%s\\n' \"$*\" >> \"$TEST_COMMAND_LOG\"\n"
        "case \" $* \" in\n"
        "  *\" config view \"*) printf '%s\\n' '{\"clusters\":[{\"cluster\":{\"server\":\"https://approved.example\"}}]}' ;;\n"
        "  *\" apply \"*) printf '%s\\n' 'networkpolicy/smart-llmrouter-discovered-ingress configured' ;;\n"
        "  *) exit 64 ;;\n"
        "esac\n",
        encoding="utf-8",
    )
    aws.chmod(0o700)
    kubectl.chmod(0o700)
    completed = subprocess.run(
        ["make", "eks-validate-tenant-network-policies"],
        cwd=ROOT,
        env={
            **os.environ,
            "PATH": f"{command_directory}:{os.environ.get('PATH', '')}",
            "TEST_COMMAND_LOG": str(command_log),
            "EKS_DISCOVERY_OUTPUT": str(report_path),
            "EKS_POLICY_AWS_PROFILE": "approved-profile",
            "EKS_POLICY_KUBECONFIG": str(kubeconfig),
            "EKS_POLICY_CONTEXT": "approved-deployment-context",
            "EKS_INGRESS_NETWORK_POLICY_OUTPUT": str(ingress_policy),
            "EKS_LINKERD_POLICY_OUTPUT": "",
        },
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if completed.returncode:
        raise AssertionError(f"Make validation did not complete with only mock commands: {completed.stderr}")
    commands = command_log.read_text(encoding="utf-8").splitlines()
    if len(commands) != 2:
        raise AssertionError("Make validation did not invoke exactly the context check and policy dry-run")
    if "--context approved-deployment-context config view" not in commands[0] or str(kubeconfig) in commands[0]:
        raise AssertionError("Make validation did not bind the context inspection to a private kubeconfig snapshot")
    if "--context approved-deployment-context apply --dry-run=server -f " not in commands[1] or str(ingress_policy) in commands[1]:
        raise AssertionError("Make validation did not dry-run a private policy snapshot")


def main() -> int:
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        test_explicit_context_is_bound_and_server_dry_run_only(root)
        test_cluster_context_mismatch_prevents_any_apply(root)
        test_account_mismatch_prevents_any_apply(root)
        test_stale_discovery_evidence_prevents_any_target_command(root)
        test_linkerd_policy_order_requires_dry_run_before_apply(root)
        test_tampered_policy_is_rejected_before_target_commands(root)
        test_oversized_policy_is_rejected_before_target_commands(root)
        test_oversized_kubeconfig_is_rejected_before_target_commands(root)
        test_activation_uses_immutable_kubeconfig_and_policy_snapshots(root)
        test_make_apply_requires_explicit_confirmation(root)
        test_make_validation_binds_the_explicit_context(root)
    print("Tenant network policy activation safeguards passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
