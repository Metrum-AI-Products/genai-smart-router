#!/usr/bin/env python3
# Copyright 2006 Metrum AI
# SPDX-License-Identifier: Apache-2.0

"""Validate and activate discovery-bound tenant network policies safely.

The discovery workflow deliberately uses a temporary kubeconfig and the
``discovery-target`` context.  Deployment policy activation must not fall back
to an operator's ambient kubeconfig/current context after that verification.
This helper binds the rendered artifacts to the selected AWS account and EKS
cluster before it invokes server-side dry-run or apply with an explicit
kubeconfig and context.
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import re
import stat
import subprocess
import sys
import tempfile
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any


ROOT = Path(__file__).resolve().parents[1]
DISCOVERY_REPORT_INTENT = "read-only bootstrap discovery; no secret, endpoint, certificate, DSN, or policy payload values"
MAX_DISCOVERY_REPORT_BYTES = 1_000_000
MAX_RENDERED_POLICY_BYTES = 64 * 1024
MAX_KUBECONFIG_BYTES = 1_000_000
# Policy artifacts are rendered only after the router workload is Ready. A
# short fixed lifetime keeps the discovered ingress/Linkerd identity from being
# reused after ordinary rollout or controller drift.
MAX_DISCOVERY_EVIDENCE_AGE_SECONDS = 900
ACCOUNT_ID = re.compile(r"^[0-9]{12}$")
REGION = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)+$")
CLUSTER = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$")
DNS_LABEL = re.compile(r"^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$")
PROFILE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")
CONTEXT = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.:/@-]{0,253}$")


class PolicyActivationError(RuntimeError):
    """A deliberately scrubbed activation error."""


class Selection:
    """The bounded, explicit deployment target copied from discovery evidence."""

    __slots__ = ("account_id", "region", "cluster", "namespace")

    def __init__(self, account_id: str, region: str, cluster: str, namespace: str) -> None:
        self.account_id = account_id
        self.region = region
        self.cluster = cluster
        self.namespace = namespace


def load_script_module(name: str, path: Path):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"cannot load required {name} renderer")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


INGRESS_RENDERER = load_script_module(
    "render_tenant_ingress_network_policy_for_activation",
    ROOT / "scripts" / "render_tenant_ingress_network_policy.py",
)
LINKERD_RENDERER = load_script_module(
    "render_tenant_linkerd_policy_for_activation",
    ROOT / "scripts" / "render_tenant_linkerd_policy.py",
)


def read_safe_file(path: Path, label: str, *, max_bytes: int) -> bytes:
    """Read one bounded, regular, non-repository input without path TOCTOU."""
    try:
        if not path.is_absolute() or path.is_symlink():
            raise PolicyActivationError(f"{label} must be an existing absolute non-symlink file")
        descriptor = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
    except PolicyActivationError:
        raise
    except OSError as exc:
        raise PolicyActivationError(f"{label} could not be inspected safely") from exc
    try:
        metadata = os.fstat(descriptor)
        if not stat.S_ISREG(metadata.st_mode):
            raise PolicyActivationError(f"{label} must be a regular file")
        if metadata.st_size > max_bytes:
            raise PolicyActivationError(f"{label} exceeds the safe maximum size")
        if path.resolve().is_relative_to(ROOT.resolve()):
            raise PolicyActivationError(f"{label} must remain outside the repository")
        with os.fdopen(descriptor, "rb") as file:
            descriptor = -1
            content = file.read(max_bytes + 1)
        if len(content) > max_bytes:
            raise PolicyActivationError(f"{label} exceeds the safe maximum size")
        return content
    except PolicyActivationError:
        raise
    except (OSError, RuntimeError) as exc:
        raise PolicyActivationError(f"{label} could not be inspected safely") from exc
    finally:
        if descriptor >= 0:
            os.close(descriptor)


def write_private_snapshot(directory: Path, name: str, content: bytes) -> Path:
    """Write a 0600 immutable activation input that kubectl can safely consume."""
    path = directory / name
    try:
        descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(descriptor, "wb") as file:
            os.fchmod(file.fileno(), 0o600)
            file.write(content)
            file.flush()
            os.fsync(file.fileno())
    except OSError as exc:
        raise PolicyActivationError("private activation input could not be prepared safely") from exc
    return path


def read_discovery_report(path: Path) -> dict[str, Any]:
    try:
        report = json.loads(read_safe_file(path, "discovery report", max_bytes=MAX_DISCOVERY_REPORT_BYTES).decode("utf-8"))
    except (UnicodeError, json.JSONDecodeError) as exc:
        raise PolicyActivationError("discovery report is not a recognized JSON document") from exc
    if not isinstance(report, dict) or report.get("schema_version") != 1 or report.get("intent") != DISCOVERY_REPORT_INTENT:
        raise PolicyActivationError("discovery report is not a recognized successful discovery result")
    return report


def require_fresh_discovery_evidence(report: dict[str, Any], *, now: datetime | None = None) -> None:
    """Reject missing, future, or expired discovery evidence before any target probe."""
    generated_at = report.get("generated_at")
    if not isinstance(generated_at, str):
        raise PolicyActivationError("discovery report is missing its generation timestamp")
    try:
        observed = datetime.fromisoformat(generated_at.replace("Z", "+00:00"))
    except ValueError as exc:
        raise PolicyActivationError("discovery report has an invalid generation timestamp") from exc
    if observed.tzinfo is None or observed.utcoffset() is None:
        raise PolicyActivationError("discovery report generation timestamp must include a timezone")
    current = now or datetime.now(timezone.utc)
    age = current - observed.astimezone(timezone.utc)
    if age < timedelta(0):
        raise PolicyActivationError("discovery report generation timestamp is in the future")
    if age > timedelta(seconds=MAX_DISCOVERY_EVIDENCE_AGE_SECONDS):
        raise PolicyActivationError("discovery report is too old; rerun discovery before policy activation")


def required_string(value: Any, pattern: re.Pattern[str], label: str) -> str:
    if not isinstance(value, str) or not pattern.fullmatch(value):
        raise PolicyActivationError(f"discovery report has an invalid {label}")
    return value


def selection_from_report(report: dict[str, Any]) -> Selection:
    selection = report.get("selection")
    if not isinstance(selection, dict):
        raise PolicyActivationError("discovery report is missing its explicit target selection")
    return Selection(
        account_id=required_string(selection.get("account_id"), ACCOUNT_ID, "AWS account selection"),
        region=required_string(selection.get("region"), REGION, "AWS region selection"),
        cluster=required_string(selection.get("cluster"), CLUSTER, "EKS cluster selection"),
        namespace=required_string(selection.get("namespace"), DNS_LABEL, "tenant namespace selection"),
    )


def read_exact_rendered_policy(path: Path, expected: str, label: str) -> None:
    if len(expected.encode("utf-8")) > MAX_RENDERED_POLICY_BYTES:
        raise PolicyActivationError(f"{label} exceeds the safe maximum size")
    try:
        actual = read_safe_file(path, label, max_bytes=MAX_RENDERED_POLICY_BYTES).decode("utf-8")
    except UnicodeError as exc:
        raise PolicyActivationError(f"{label} could not be read safely") from exc
    if actual != expected:
        raise PolicyActivationError(f"{label} does not exactly match the selected discovery evidence")


def validate_rendered_policies(report: dict[str, Any], ingress_policy: Path, linkerd_policy: Path | None) -> list[tuple[str, str]]:
    """Accept only the exact artifacts generated from the supplied report.

    Re-rendering here prevents a same-named policy or an appended YAML document
    from reaching the cluster.  It also ensures the target tenant namespace is
    the namespace that discovery recorded.
    """
    try:
        ingress_expected = INGRESS_RENDERER.render(
            INGRESS_RENDERER.DEFAULT_TEMPLATE.read_text(encoding="utf-8"),
            report,
            INGRESS_RENDERER.EKS_INGRESS_GUARD_POLICY.read_text(encoding="utf-8"),
        )
    except (OSError, ValueError) as exc:
        raise PolicyActivationError("discovery evidence cannot safely render the ingress policy") from exc
    read_exact_rendered_policy(ingress_policy, ingress_expected, "rendered ingress NetworkPolicy")

    linkerd = report.get("linkerd")
    if not isinstance(linkerd, dict) or not isinstance(linkerd.get("requested"), bool):
        raise PolicyActivationError("discovery report has an invalid Linkerd selection state")
    if linkerd["requested"] is False:
        if linkerd_policy is not None:
            raise PolicyActivationError("a Linkerd policy was supplied for a non-Linkerd discovery selection")
        return [("tenant-ingress-network-policy.yaml", ingress_expected)]
    if linkerd_policy is None:
        raise PolicyActivationError("Linkerd discovery requires the rendered Linkerd policy")
    try:
        linkerd_expected = LINKERD_RENDERER.render(
            LINKERD_RENDERER.DEFAULT_TEMPLATE.read_text(encoding="utf-8"),
            report,
        )
    except (OSError, ValueError) as exc:
        raise PolicyActivationError("discovery evidence cannot safely render the Linkerd policy") from exc
    read_exact_rendered_policy(linkerd_policy, linkerd_expected, "rendered Linkerd policy")
    # Preserve Linkerd-before-NetworkPolicy ordering for both dry-run and apply.
    return [
        ("tenant-linkerd-policy.yaml", linkerd_expected),
        ("tenant-ingress-network-policy.yaml", ingress_expected),
    ]


def run(command: list[str]) -> str:
    try:
        return subprocess.check_output(command, text=True, stderr=subprocess.PIPE)
    except FileNotFoundError as exc:
        raise PolicyActivationError(f"required command is unavailable: {command[0]}") from exc
    except subprocess.CalledProcessError as exc:
        # Do not include stderr: AWS and kubectl can include endpoint, auth, or
        # request data there. Callers get only a safe command classification.
        raise PolicyActivationError(f"{command[0]} command failed") from exc


def aws_json(args: list[str], selection: Selection, profile: str) -> dict[str, Any]:
    try:
        payload = json.loads(run(["aws", "--profile", profile, *args, "--region", selection.region, "--output", "json"]))
    except json.JSONDecodeError as exc:
        raise PolicyActivationError("AWS returned an unrecognized target response") from exc
    if not isinstance(payload, dict):
        raise PolicyActivationError("AWS returned an unrecognized target response")
    return payload


def explicit_context_endpoint(kubeconfig: Path, context: str) -> str:
    try:
        payload = json.loads(
            run([
                "kubectl",
                "--kubeconfig",
                str(kubeconfig),
                "--context",
                context,
                "config",
                "view",
                "--minify",
                "-o",
                "json",
            ])
        )
    except json.JSONDecodeError as exc:
        raise PolicyActivationError("explicit kubeconfig context could not be verified") from exc
    clusters = payload.get("clusters") if isinstance(payload, dict) else None
    if not isinstance(clusters, list) or len(clusters) != 1 or not isinstance(clusters[0], dict):
        raise PolicyActivationError("explicit kubeconfig context could not be verified")
    cluster = clusters[0].get("cluster")
    endpoint = cluster.get("server") if isinstance(cluster, dict) else None
    if not isinstance(endpoint, str) or not endpoint.startswith("https://"):
        raise PolicyActivationError("explicit kubeconfig context could not be verified")
    return endpoint.rstrip("/")


def validate_selected_target(selection: Selection, profile: str, kubeconfig: Path, context: str) -> None:
    if not PROFILE.fullmatch(profile):
        raise PolicyActivationError("AWS profile has an invalid format")
    if not CONTEXT.fullmatch(context):
        raise PolicyActivationError("Kubernetes context has an invalid format")
    identity = aws_json(["sts", "get-caller-identity"], selection, profile)
    if identity.get("Account") != selection.account_id:
        raise PolicyActivationError("AWS profile account does not match the discovery selection")
    cluster = aws_json(["eks", "describe-cluster", "--name", selection.cluster], selection, profile).get("cluster")
    endpoint = cluster.get("endpoint") if isinstance(cluster, dict) else None
    if not isinstance(endpoint, str) or not endpoint.startswith("https://"):
        raise PolicyActivationError("selected EKS cluster could not be verified")
    if explicit_context_endpoint(kubeconfig, context) != endpoint.rstrip("/"):
        raise PolicyActivationError("explicit kubeconfig context does not match the selected EKS cluster")


def kubectl_apply(kubeconfig: Path, context: str, policy: Path, *, dry_run: bool) -> None:
    command = ["kubectl", "--kubeconfig", str(kubeconfig), "--context", context, "apply"]
    if dry_run:
        command.append("--dry-run=server")
    command.extend(["-f", str(policy)])
    run(command)


def activate(
    discovery_report: Path,
    profile: str,
    kubeconfig: Path,
    context: str,
    ingress_policy: Path,
    linkerd_policy: Path | None,
    *,
    apply: bool,
    now: datetime | None = None,
) -> None:
    report = read_discovery_report(discovery_report)
    require_fresh_discovery_evidence(report, now=now)
    selection = selection_from_report(report)
    policies = validate_rendered_policies(report, ingress_policy, linkerd_policy)
    kubeconfig_bytes = read_safe_file(kubeconfig, "Kubernetes kubeconfig", max_bytes=MAX_KUBECONFIG_BYTES)
    with tempfile.TemporaryDirectory(prefix="smartrouter-tenant-policy-") as temporary_directory:
        temporary_root = Path(temporary_directory)
        kubeconfig_snapshot = write_private_snapshot(temporary_root, "kubeconfig", kubeconfig_bytes)
        policy_snapshots = [
            write_private_snapshot(temporary_root, name, content.encode("utf-8"))
            for name, content in policies
        ]
        validate_selected_target(selection, profile, kubeconfig_snapshot, context)
        for policy_snapshot in policy_snapshots:
            kubectl_apply(kubeconfig_snapshot, context, policy_snapshot, dry_run=True)
        if apply:
            for policy_snapshot in policy_snapshots:
                kubectl_apply(kubeconfig_snapshot, context, policy_snapshot, dry_run=False)


def parser() -> argparse.ArgumentParser:
    result = argparse.ArgumentParser(description=__doc__)
    result.add_argument("--discovery-report", required=True, type=Path)
    result.add_argument("--profile", required=True, help="approved deployment AWS profile with sts:GetCallerIdentity and eks:DescribeCluster")
    result.add_argument("--kubeconfig", required=True, type=Path, help="explicit deployment kubeconfig outside the repository")
    result.add_argument("--context", required=True, help="explicit kubeconfig context; ambient current-context is never used")
    result.add_argument("--ingress-policy", required=True, type=Path)
    result.add_argument("--linkerd-policy", type=Path)
    result.add_argument("--apply", action="store_true", help="apply only after target verification and server-side dry-run")
    return result


def main() -> int:
    args = parser().parse_args()
    try:
        activate(
            args.discovery_report,
            args.profile,
            args.kubeconfig,
            args.context,
            args.ingress_policy,
            args.linkerd_policy,
            apply=args.apply,
        )
    except PolicyActivationError as exc:
        print(f"EKS tenant policy activation failed safely: {exc}", file=sys.stderr)
        return 2
    if args.apply:
        print("verified selected EKS target, server-side validated, and applied tenant network policies")
    else:
        print("verified selected EKS target and server-side validated tenant network policies")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
