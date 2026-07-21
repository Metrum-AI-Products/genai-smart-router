#!/usr/bin/env python3
"""Fail-closed local contract for Metrum's approved EKS staging delivery.

The selected account, region, cluster, namespace, overlay, workload, and
delivery role come from a reviewed policy committed with this contract and an
independently protected AWS Systems Manager Parameter copy.  Make callers can
choose only an AWS profile, an immutable image digest, evidence location, and
the protected smoke command.  There is no production action.
"""
from __future__ import annotations

import argparse
import copy
import datetime as dt
import errno
import hashlib
import json
import os
import re
import shutil
import stat
import subprocess
import sys
import tempfile
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


REPO_ROOT = Path(__file__).resolve().parents[1]
TARGET_POLICY_PATH = REPO_ROOT / "deploy" / "aws" / "genai-smart-router-eks-staging-target.json"
SECRET_PATTERNS = (
    # Keep the key/header name for useful diagnostics while replacing the
    # complete value. This covers shell assignments and common HTTP/JSON
    # spellings, including ``Authorization: Bearer <credential>``.
    re.compile(
        r"(?im)((?:[\"']?)(?:authorization|proxy-authorization|x-api-key|api[-_]?key|access[-_]?token|refresh[-_]?token|token|password|secret(?:[-_]?access[-_]?key)?|aws_secret_access_key)(?:[\"']?)\s*[:=]\s*)(?:bearer\s+)?(?:\"[^\"]*\"|'[^']*'|[^\s,;]+)"
    ),
    re.compile(r"(?i)\bbearer\s+[a-z0-9._~+/=-]+"),
    re.compile(r"AKIA[0-9A-Z]{16}"),
)
DIGEST = re.compile(r"^[a-z0-9][a-z0-9./:_-]*@sha256:[0-9a-f]{64}$")
ECR_REPOSITORY_URI = re.compile(
    r"^([0-9]{12})\.dkr\.ecr\.([a-z0-9]+(?:-[a-z0-9]+)+)\.amazonaws\.com/"
    r"([a-z0-9](?:[a-z0-9._/-]*[a-z0-9])?)$"
)
NAME = re.compile(r"^[a-z0-9]([-a-z0-9]*[a-z0-9])?$")
PROFILE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$")
ACCOUNT_ID = re.compile(r"^[0-9]{12}$")
REGION = re.compile(r"^[a-z0-9]+(?:-[a-z0-9]+)+$")
SSM_PARAMETER_ARN = re.compile(
    r"^arn:aws:ssm:([a-z0-9]+(?:-[a-z0-9]+)+):([0-9]{12}):parameter/([A-Za-z0-9_.+/@=-]+(?:/[A-Za-z0-9_.+/@=-]+)*)$"
)
ASSUMED_ROLE_ARN = re.compile(r"^arn:aws:sts::([0-9]{12}):assumed-role/([^/]+)/[^/]+$")
NAMESPACED_KINDS = frozenset(
    {
        "Deployment",
        "Ingress",
        "NetworkPolicy",
        "PersistentVolumeClaim",
        "PodDisruptionBudget",
        "Service",
        "ServiceAccount",
    }
)
MANAGED_RESOURCE_LABEL = "app.kubernetes.io/name"
MANAGED_RESOURCE_LABEL_VALUE = "smart-llmrouter"
MANAGED_RESOURCE_TYPES = (
    "deployments",
    "ingresses",
    "networkpolicies",
    "persistentvolumeclaims",
    "poddisruptionbudgets",
    "services",
    "serviceaccounts",
)
# Kubernetes writes these fields after accepting a declarative resource. They
# do not describe the reviewed configuration and are deliberately excluded
# from the safe per-resource fingerprint below. Keep this allowlist narrow:
# other metadata, annotations, labels, and declarative fields remain bound to
# the rendered manifest and will fail closed on drift.
DYNAMIC_MANAGED_ANNOTATIONS = frozenset(
    {
        "deployment.kubernetes.io/revision",
        "kubectl.kubernetes.io/last-applied-configuration",
        "pv.kubernetes.io/bind-completed",
        "pv.kubernetes.io/bound-by-controller",
        "volume.beta.kubernetes.io/storage-provisioner",
        "volume.kubernetes.io/storage-provisioner",
    }
)
SERVICE_RUNTIME_SPEC_FIELDS = frozenset(
    {
        "clusterIP",
        "clusterIPs",
    }
)
PVC_RUNTIME_SPEC_FIELDS = frozenset({"volumeName"})
SERVICE_ACCOUNT_RUNTIME_FIELDS = frozenset({"secrets"})


def scrub(value: str) -> str:
    result = value
    for pattern in SECRET_PATTERNS:
        result = pattern.sub(
            lambda match: f"{match.group(1)}[REDACTED]" if match.lastindex else "[REDACTED]",
            result,
        )
    return result[:4000]


def fail(message: str) -> None:
    raise RuntimeError(message)


def open_protected_smoke_command_file(value: str) -> int:
    """Open an owner-only smoke script and return its bound descriptor.

    Smoke requests can require caller credentials, so command content is never
    a Make expansion or Python argument.  Opening the file once with
    ``O_NOFOLLOW`` and executing that descriptor through ``/bin/sh -s`` also
    prevents a path replacement between validation and execution.  The file
    must be a real owner-only regular file; a symlink or broader mode could
    redirect a privileged delivery invocation or expose its command content.
    """

    if not value:
        fail("EKS_SMOKE_COMMAND_FILE is required and must name a protected mode 0600 shell script")
    path = Path(value).expanduser()
    no_follow = getattr(os, "O_NOFOLLOW", 0)
    if not no_follow:
        fail("EKS_SMOKE_COMMAND_FILE cannot be opened safely on this platform")
    try:
        descriptor = os.open(
            path,
            os.O_RDONLY | no_follow | getattr(os, "O_CLOEXEC", 0),
        )
    except OSError as exc:
        if exc.errno == errno.ELOOP:
            fail("EKS_SMOKE_COMMAND_FILE must be a regular non-symlink file")
        raise RuntimeError("EKS_SMOKE_COMMAND_FILE is not an accessible protected file") from exc
    try:
        metadata = os.fstat(descriptor)
        if not stat.S_ISREG(metadata.st_mode):
            fail("EKS_SMOKE_COMMAND_FILE must be a regular non-symlink file")
        if stat.S_IMODE(metadata.st_mode) != 0o600:
            fail("EKS_SMOKE_COMMAND_FILE must have mode 0600")
        if metadata.st_uid != os.geteuid():
            fail("EKS_SMOKE_COMMAND_FILE must be owned by the invoking user")
        if metadata.st_size == 0:
            fail("EKS_SMOKE_COMMAND_FILE must not be empty")
        return descriptor
    except Exception:
        os.close(descriptor)
        raise


def command(args: list[str], env: dict[str, str], *, quiet: bool = False, raw: bool = False) -> str:
    proc = subprocess.run(args, env=env, text=True, capture_output=True)
    if proc.returncode:
        detail = scrub(proc.stderr or proc.stdout)
        fail(f"{args[0]} failed (exit {proc.returncode}): {detail}")
    return "" if quiet else (proc.stdout if raw else scrub(proc.stdout))


def canonical_json_bytes(value: object) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode("utf-8")


@dataclass(frozen=True)
class TargetPolicy:
    aws_account_id: str
    aws_region: str
    container_name: str
    delivery_role_name: str
    deployment_name: str
    ecr_repository_uri: str
    eks_cluster: str
    k8s_namespace: str
    kustomize_overlay: Path
    ssm_parameter_arn: str
    raw: dict[str, object]

    @property
    def sha256(self) -> str:
        return hashlib.sha256(canonical_json_bytes(self.raw)).hexdigest()


@dataclass(frozen=True)
class LiveDeploymentIdentity:
    """Safe, stable identity for the live rollout tested by staging evidence."""

    generation: int
    observed_generation: int
    pod_template_sha256: str


@dataclass(frozen=True, order=True)
class ManagedResource:
    """A safe identity for one allowlisted, label-selected staging resource."""

    kind: str
    name: str


@dataclass(frozen=True, order=True)
class ManagedResourceConfiguration:
    """Safe identity plus a normalized declarative-configuration digest."""

    resource: ManagedResource
    configuration_sha256: str
    # This exists only while the local delivery process is running. It lets the
    # live-read path strip a very small, reviewed set of API-owned defaults
    # only when the reviewed client-rendered manifest omitted them. It is never
    # logged, placed in evidence, or used for ordering/hashing this value.
    normalized_configuration: dict[str, object] = field(compare=False, repr=False)


def inventory_fingerprint(inventory: tuple[ManagedResource, ...]) -> str:
    """Return a stable, non-sensitive digest for an allowlisted inventory."""

    return hashlib.sha256(
        canonical_json_bytes(
            [{"kind": resource.kind, "name": resource.name} for resource in inventory]
        )
    ).hexdigest()


def managed_resource_spec_fingerprint(
    inventory: tuple[ManagedResourceConfiguration, ...],
) -> str:
    """Return a stable digest of each managed resource's normalized config."""

    return hashlib.sha256(
        canonical_json_bytes(
            [
                {
                    "configuration_sha256": item.configuration_sha256,
                    "kind": item.resource.kind,
                    "name": item.resource.name,
                }
                for item in inventory
            ]
        )
    ).hexdigest()


def managed_resource_identities(
    inventory: tuple[ManagedResourceConfiguration, ...],
) -> tuple[ManagedResource, ...]:
    return tuple(item.resource for item in inventory)


def parse_target_policy(value: object) -> TargetPolicy:
    if not isinstance(value, dict):
        fail("approved staging target policy is not a JSON object")
    required = {
        "schema_version",
        "environment",
        "aws_account_id",
        "aws_region",
        "ssm_parameter_arn",
        "ecr_repository_uri",
        "eks_cluster",
        "k8s_namespace",
        "kustomize_overlay",
        "deployment_name",
        "container_name",
        "delivery_role_name",
    }
    if set(value) != required:
        fail("approved staging target policy has an unexpected schema")
    if value.get("schema_version") != 2 or value.get("environment") != "staging":
        fail("approved staging target policy is not a supported staging policy")

    def string(name: str) -> str:
        item = value.get(name)
        if not isinstance(item, str) or not item:
            fail(f"approved staging target policy has invalid {name}")
        return item

    account_id = string("aws_account_id")
    region = string("aws_region")
    cluster = string("eks_cluster")
    ecr_repository_uri = string("ecr_repository_uri")
    namespace = string("k8s_namespace")
    deployment = string("deployment_name")
    container = string("container_name")
    role = string("delivery_role_name")
    parameter_arn = string("ssm_parameter_arn")
    overlay_value = string("kustomize_overlay")
    if not ACCOUNT_ID.fullmatch(account_id) or not REGION.fullmatch(region):
        fail("approved staging target policy has invalid AWS account or region")
    if not all(NAME.fullmatch(item) for item in (namespace, deployment, container)):
        fail("approved staging target policy has invalid Kubernetes identifiers")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,99}", cluster):
        fail("approved staging target policy has invalid EKS cluster")
    if not re.fullmatch(r"[A-Za-z0-9+=,.@_-]{1,64}", role):
        fail("approved staging target policy has invalid delivery role")
    ecr_match = ECR_REPOSITORY_URI.fullmatch(ecr_repository_uri)
    if (
        not ecr_match
        or ecr_match.group(1) != account_id
        or ecr_match.group(2) != region
    ):
        fail("approved staging target policy ECR repository is not pinned to its account and region")
    parameter_match = SSM_PARAMETER_ARN.fullmatch(parameter_arn)
    if not parameter_match or parameter_match.group(1) != region or parameter_match.group(2) != account_id:
        fail("approved staging target policy SSM parameter is not pinned to its account and region")
    overlay = Path(overlay_value)
    if overlay.is_absolute():
        fail("approved staging target policy overlay must be repository-relative")
    resolved_overlay = (REPO_ROOT / overlay).resolve()
    if not resolved_overlay.is_relative_to(REPO_ROOT) or not (resolved_overlay / "kustomization.yaml").is_file():
        fail("approved staging target policy overlay is not a checked-in kustomization")
    # Construct a normalized mapping before hashing so whitespace in the SSM
    # parameter cannot mask a policy drift.
    normalized: dict[str, object] = {
        "aws_account_id": account_id,
        "aws_region": region,
        "container_name": container,
        "delivery_role_name": role,
        "deployment_name": deployment,
        "ecr_repository_uri": ecr_repository_uri,
        "environment": "staging",
        "eks_cluster": cluster,
        "k8s_namespace": namespace,
        "kustomize_overlay": overlay.as_posix(),
        "schema_version": 2,
        "ssm_parameter_arn": parameter_arn,
    }
    return TargetPolicy(
        aws_account_id=account_id,
        aws_region=region,
        container_name=container,
        delivery_role_name=role,
        deployment_name=deployment,
        ecr_repository_uri=ecr_repository_uri,
        eks_cluster=cluster,
        k8s_namespace=namespace,
        kustomize_overlay=resolved_overlay,
        ssm_parameter_arn=parameter_arn,
        raw=normalized,
    )


def read_checked_in_target_policy() -> TargetPolicy:
    try:
        return parse_target_policy(json.loads(TARGET_POLICY_PATH.read_text(encoding="utf-8")))
    except FileNotFoundError as exc:
        raise RuntimeError("checked-in approved staging target policy is missing") from exc
    except json.JSONDecodeError as exc:
        raise RuntimeError("checked-in approved staging target policy is invalid JSON") from exc


class Delivery:
    def __init__(self, args: argparse.Namespace):
        self.args = args
        self.smoke_command_fd: int | None = None
        self._validate_inputs()
        self.evidence_dir = Path(args.evidence_dir).resolve()
        self.evidence_dir.mkdir(parents=True, exist_ok=True)
        self.target: TargetPolicy | None = None
        self.managed_inventory: tuple[ManagedResourceConfiguration, ...] = ()
        self.evidence: dict[str, object] = {
            "timestamp": dt.datetime.now(dt.timezone.utc).isoformat(),
            "environment": "staging",
            "image_digest": args.image_digest or None,
            "action": args.action,
            "events": [],
        }

    def _validate_inputs(self) -> None:
        if not PROFILE.fullmatch(self.args.aws_profile):
            fail("EKS_AWS_PROFILE must be a documented AWS profile identifier")
        if self.args.action in {
            "render",
            "plan",
            "apply",
            "rollback",
            "smoke",
            "promotion-plan",
        }:
            if not self.args.image_digest or not DIGEST.fullmatch(self.args.image_digest):
                fail("IMAGE_DIGEST must be a lower-case immutable image@sha256:<64 hex> reference")
        if self.args.action in {"apply", "rollback"} and self.args.confirm != "STAGING_APPLY":
            fail("mutating action requires EKS_CONFIRM=STAGING_APPLY")
        if self.args.action == "smoke":
            self.smoke_command_fd = open_protected_smoke_command_file(self.args.smoke_command_file)

    def event(self, name: str, **fields: object) -> None:
        self.evidence["events"].append({"name": name, **fields})

    def write_evidence(self, outcome: str, error: str | None = None) -> None:
        self.evidence["outcome"] = outcome
        if error:
            self.evidence["error"] = scrub(error)
        payload = json.dumps(self.evidence, indent=2, sort_keys=True) + "\n"
        # Keep immutable per-action records so a later smoke cannot overwrite
        # the apply proof needed by promotion. evidence.json remains a concise
        # convenience pointer to the most recent result.
        (self.evidence_dir / f"evidence-{self.args.action}.json").write_text(payload, encoding="utf-8")
        (self.evidence_dir / "evidence.json").write_text(payload, encoding="utf-8")
        markdown = [f"# EKS {self.args.action} evidence", "", f"Outcome: **{outcome}**", ""]
        for key in (
            "environment",
            "aws_account_id",
            "aws_region",
            "ecr_repository_uri",
            "eks_cluster",
            "k8s_namespace",
            "target_policy_sha256",
            "image_digest",
            "configuration_fingerprint",
            "managed_resource_count",
            "managed_resource_inventory_sha256",
            "managed_resource_spec_sha256",
            "live_managed_resource_count",
            "live_managed_resource_inventory_sha256",
            "live_managed_resource_spec_sha256",
            "rollback_target_revision",
            "live_deployment_generation",
            "live_deployment_observed_generation",
            "live_pod_template_sha256",
        ):
            if key in self.evidence:
                markdown.append(f"- {key}: `{self.evidence[key]}`")
        if error:
            markdown.extend(["", "## Safe failure", "", "```text", scrub(error), "```"])
        summary = "\n".join(markdown) + "\n"
        (self.evidence_dir / f"summary-{self.args.action}.md").write_text(summary, encoding="utf-8")
        (self.evidence_dir / "summary.md").write_text(summary, encoding="utf-8")

    def env_with_kubeconfig(self) -> tuple[dict[str, str], tempfile.TemporaryDirectory[str]]:
        temp = tempfile.TemporaryDirectory(prefix="smartrouter-eks-")
        env = os.environ.copy()
        env["KUBECONFIG"] = str(Path(temp.name) / "kubeconfig")
        return env, temp

    def aws_json(self, arguments: list[str], env: dict[str, str], target: TargetPolicy) -> dict[str, Any]:
        raw = command(
            ["aws", "--profile", self.args.aws_profile, "--region", target.aws_region, *arguments, "--output", "json"],
            env,
            raw=True,
        )
        try:
            parsed = json.loads(raw)
        except json.JSONDecodeError as exc:
            raise RuntimeError("AWS command returned invalid JSON") from exc
        if not isinstance(parsed, dict):
            fail("AWS command returned an unexpected response")
        return parsed

    def load_target_policy(self, env: dict[str, str]) -> TargetPolicy:
        checked_in = read_checked_in_target_policy()
        response = self.aws_json(
            ["ssm", "get-parameter", "--name", checked_in.ssm_parameter_arn],
            env,
            checked_in,
        )
        parameter = response.get("Parameter")
        if not isinstance(parameter, dict) or parameter.get("Type") != "String" or not isinstance(parameter.get("Value"), str):
            fail("approved staging target policy parameter is missing or has an unsafe type")
        try:
            protected = parse_target_policy(json.loads(parameter["Value"]))
        except json.JSONDecodeError as exc:
            raise RuntimeError("approved staging target policy parameter is invalid JSON") from exc
        if protected.sha256 != checked_in.sha256:
            fail("protected staging target policy does not match the checked-in reviewed policy")
        self.target = checked_in
        self.evidence.update(
            {
                "aws_account_id": checked_in.aws_account_id,
                "aws_region": checked_in.aws_region,
                "ecr_repository_uri": checked_in.ecr_repository_uri,
                "eks_cluster": checked_in.eks_cluster,
                "k8s_namespace": checked_in.k8s_namespace,
                "kustomize_overlay": checked_in.kustomize_overlay.relative_to(REPO_ROOT).as_posix(),
                "deployment_name": checked_in.deployment_name,
                "container_name": checked_in.container_name,
                "target_policy_arn": checked_in.ssm_parameter_arn,
                "target_policy_sha256": checked_in.sha256,
            }
        )
        return checked_in

    def verify_approved_image_repository(self, target: TargetPolicy) -> None:
        """Bind the requested immutable artifact to the protected ECR repo."""

        if self.args.action not in {
            "render",
            "plan",
            "apply",
            "rollback",
            "smoke",
            "promotion-plan",
        }:
            return
        expected_prefix = f"{target.ecr_repository_uri}@sha256:"
        if not self.args.image_digest.startswith(expected_prefix):
            fail("IMAGE_DIGEST does not reference the approved staging ECR repository")
        self.event("image_repository", result="approved_ecr_repository_verified")

    def preflight(self, env: dict[str, str]) -> TargetPolicy:
        for tool in ("aws", "kubectl", "kustomize"):
            if not shutil.which(tool):
                fail(f"required executable not found: {tool}")
        # The external policy is read before any EKS selection. It must match
        # the reviewed copy in Git exactly; the delivery role gets only
        # ssm:GetParameter on this one Parameter, never PutParameter.
        target = self.load_target_policy(env)
        # Validate the immutable artifact reference before selecting a cluster,
        # creating kubeconfig, rendering, or applying any manifest. A caller
        # cannot redirect a staging pod with mounted runtime secrets to another
        # registry or repository merely by providing a digest-pinned image.
        self.verify_approved_image_repository(target)
        identity = self.aws_json(["sts", "get-caller-identity"], env, target)
        actual_account = str(identity.get("Account", ""))
        assumed_role = ASSUMED_ROLE_ARN.fullmatch(str(identity.get("Arn", "")))
        if actual_account != target.aws_account_id or not assumed_role or assumed_role.group(1) != target.aws_account_id:
            fail("AWS identity does not match the approved staging account")
        if assumed_role.group(2) != target.delivery_role_name:
            fail("AWS identity does not use the approved staging delivery role")
        cluster_payload = self.aws_json(["eks", "describe-cluster", "--name", target.eks_cluster], env, target)
        cluster = cluster_payload.get("cluster")
        expected_cluster_arn = f"arn:aws:eks:{target.aws_region}:{target.aws_account_id}:cluster/{target.eks_cluster}"
        if not isinstance(cluster, dict) or cluster.get("status") != "ACTIVE" or cluster.get("arn") != expected_cluster_arn:
            fail("approved EKS cluster is unavailable or does not match the protected target")
        command(
            [
                "aws",
                "--profile",
                self.args.aws_profile,
                "--region",
                target.aws_region,
                "eks",
                "update-kubeconfig",
                "--name",
                target.eks_cluster,
                "--kubeconfig",
                env["KUBECONFIG"],
                "--alias",
                "smartrouter-staging-delivery",
            ],
            env,
            quiet=True,
        )
        allowed = command(
            ["kubectl", "auth", "can-i", "get", "pods", "-n", target.k8s_namespace], env
        ).strip()
        if allowed != "yes":
            fail("Kubernetes RBAC does not allow get pods in the approved namespace")
        for resource_type in MANAGED_RESOURCE_TYPES:
            list_allowed = command(
                ["kubectl", "auth", "can-i", "list", resource_type, "-n", target.k8s_namespace], env
            ).strip()
            if list_allowed != "yes":
                fail(
                    "Kubernetes RBAC does not allow list "
                    f"{resource_type} in the approved namespace"
                )
        if self.args.action == "rollback":
            replica_sets_allowed = command(
                ["kubectl", "auth", "can-i", "list", "replicasets", "-n", target.k8s_namespace],
                env,
            ).strip()
            if replica_sets_allowed != "yes":
                fail("Kubernetes RBAC does not allow list replicasets in the approved namespace")
        self.event(
            "preflight",
            aws_account_id=actual_account,
            aws_principal_type="assumed-role",
            cluster_status="ACTIVE",
            rbac_get_pods=allowed,
            rbac_list_managed_resource_types=len(MANAGED_RESOURCE_TYPES),
            rbac_list_replicasets=self.args.action != "rollback" or replica_sets_allowed == "yes",
        )
        return target

    @staticmethod
    def _json_objects(
        value: str, *, source: str, allow_empty: bool = False
    ) -> list[dict[str, Any]]:
        decoder = json.JSONDecoder()
        objects: list[dict[str, Any]] = []
        index = 0
        while index < len(value):
            while index < len(value) and value[index].isspace():
                index += 1
            if index == len(value):
                break
            try:
                payload, index = decoder.raw_decode(value, index)
            except json.JSONDecodeError as exc:
                raise RuntimeError(f"{source} returned invalid JSON") from exc
            if not isinstance(payload, dict):
                fail(f"{source} returned an invalid resource")
            if payload.get("kind") == "List":
                items = payload.get("items")
                if not isinstance(items, list):
                    fail(f"{source} returned an invalid resource list")
                count_before = len(objects)
                objects.extend(item for item in items if isinstance(item, dict))
                if len(objects) - count_before != len(items):
                    fail(f"{source} returned an invalid resource list")
            else:
                objects.append(payload)
        if not objects and not allow_empty:
            fail(f"{source} contains no Kubernetes resources")
        return objects

    @staticmethod
    def normalized_managed_resource(
        resource: dict[str, Any], identity: ManagedResource, *, source: str
    ) -> dict[str, object]:
        """Return the declarative, security-relevant configuration only.

        The API server adds resource/version/status fields and a small set of
        allocation values after apply. Those are intentionally removed before
        hashing. Everything else in the manifest, including labels,
        annotations, selectors, ingress rules, network policy, PVC settings,
        disruption policy, and ServiceAccount controls, remains in the hash.
        The function never emits the configuration; evidence records only its
        SHA-256 aggregate.
        """

        metadata = resource.get("metadata")
        if not isinstance(metadata, dict):
            fail(f"{source} resource has invalid metadata")

        def string_map(name: str) -> dict[str, str]:
            value = metadata.get(name)
            if value is None:
                return {}
            if not isinstance(value, dict) or any(
                not isinstance(key, str) or not isinstance(item, str)
                for key, item in value.items()
            ):
                fail(f"{source} resource has invalid metadata.{name}")
            return dict(value)

        annotations = {
            key: value
            for key, value in string_map("annotations").items()
            if key not in DYNAMIC_MANAGED_ANNOTATIONS
        }
        labels = string_map("labels")
        normalized_metadata: dict[str, object] = {
            "name": identity.name,
            "namespace": metadata.get("namespace"),
        }
        if labels:
            normalized_metadata["labels"] = labels
        if annotations:
            normalized_metadata["annotations"] = annotations

        normalized: dict[str, object] = {}
        for key, value in resource.items():
            if key in {"metadata", "status"}:
                continue
            normalized[key] = value
        normalized["metadata"] = normalized_metadata

        spec = normalized.get("spec")
        if isinstance(spec, dict):
            normalized_spec = dict(spec)
            if identity.kind == "Service":
                for field in SERVICE_RUNTIME_SPEC_FIELDS:
                    normalized_spec.pop(field, None)
            elif identity.kind == "PersistentVolumeClaim":
                for field in PVC_RUNTIME_SPEC_FIELDS:
                    normalized_spec.pop(field, None)
            if normalized_spec:
                normalized["spec"] = normalized_spec
            else:
                normalized.pop("spec", None)
        elif spec is not None:
            fail(f"{source} resource has invalid spec")

        if identity.kind == "ServiceAccount":
            for field in SERVICE_ACCOUNT_RUNTIME_FIELDS:
                normalized.pop(field, None)
        return normalized

    @staticmethod
    def _mapping_at(value: dict[str, object], path: tuple[str, ...]) -> dict[str, object] | None:
        current: object = value
        for segment in path:
            if not isinstance(current, dict):
                return None
            current = current.get(segment)
        return current if isinstance(current, dict) else None

    @classmethod
    def _drop_defaulted_mapping_field(
        cls,
        live: dict[str, object],
        expected: dict[str, object],
        path: tuple[str, ...],
        default: object,
    ) -> None:
        """Drop one exact API default only when the manifest omitted it.

        This is deliberately not a generic "ignore unknown fields" helper.
        Any live label, annotation, or spec field that is not an exact entry in
        this small reviewed list remains in the hash and therefore fails
        closed.  The expected value always comes from client-rendered manifest
        content, never from a server-side apply response.
        """

        live_parent = cls._mapping_at(live, path[:-1])
        expected_parent = cls._mapping_at(expected, path[:-1])
        field_name = path[-1]
        if (
            live_parent is not None
            and expected_parent is not None
            and field_name not in expected_parent
            and live_parent.get(field_name) == default
        ):
            live_parent.pop(field_name)

    @classmethod
    def normalized_live_managed_resource(
        cls,
        resource: dict[str, Any],
        identity: ManagedResource,
        expected: dict[str, object],
        *,
        source: str,
    ) -> dict[str, object]:
        """Normalize a live resource against the isolated desired baseline.

        `kubectl apply --server-side --dry-run=server` is intentionally not
        used as that baseline: Server-Side Apply can return fields preserved
        from other field managers.  The reviewed client-rendered manifest is
        authoritative.  This function removes only exact Kubernetes API
        defaults that were absent from that manifest; all other additions and
        changes, including foreign-manager labels/annotations, remain visible
        to the configuration fingerprint.
        """

        normalized = copy.deepcopy(
            cls.normalized_managed_resource(resource, identity, source=source)
        )

        if identity.kind == "Service":
            for path, default in (
                (("spec", "internalTrafficPolicy"), "Cluster"),
                (("spec", "ipFamilyPolicy"), "SingleStack"),
                (("spec", "sessionAffinity"), "None"),
            ):
                cls._drop_defaulted_mapping_field(normalized, expected, path, default)
            live_spec = cls._mapping_at(normalized, ("spec",))
            expected_spec = cls._mapping_at(expected, ("spec",))
            if (
                live_spec is not None
                and expected_spec is not None
                and "ipFamilies" not in expected_spec
                and live_spec.get("ipFamilies") in (["IPv4"], ["IPv6"])
            ):
                # The allocation family is chosen by the cluster when omitted.
                # An explicit manifest value stays in the fingerprint.
                live_spec.pop("ipFamilies")
            live_ports = live_spec.get("ports") if live_spec is not None else None
            expected_ports = expected_spec.get("ports") if expected_spec is not None else None
            if (
                isinstance(live_ports, list)
                and isinstance(expected_ports, list)
                and len(live_ports) == len(expected_ports)
            ):
                for live_port, expected_port in zip(live_ports, expected_ports):
                    if (
                        isinstance(live_port, dict)
                        and isinstance(expected_port, dict)
                        and "protocol" not in expected_port
                        and live_port.get("protocol") == "TCP"
                    ):
                        live_port.pop("protocol")
        elif identity.kind == "Deployment":
            for path, default in (
                (("spec", "progressDeadlineSeconds"), 600),
                (("spec", "revisionHistoryLimit"), 10),
                (("spec", "paused"), False),
                (("spec", "template", "spec", "dnsPolicy"), "ClusterFirst"),
                (("spec", "template", "spec", "enableServiceLinks"), True),
                (("spec", "template", "spec", "hostIPC"), False),
                (("spec", "template", "spec", "hostNetwork"), False),
                (("spec", "template", "spec", "hostPID"), False),
                (("spec", "template", "spec", "preemptionPolicy"), "PreemptLowerPriority"),
                (("spec", "template", "spec", "priority"), 0),
                (("spec", "template", "spec", "restartPolicy"), "Always"),
                (("spec", "template", "spec", "schedulerName"), "default-scheduler"),
                (("spec", "template", "spec", "terminationGracePeriodSeconds"), 30),
            ):
                cls._drop_defaulted_mapping_field(normalized, expected, path, default)
            live_spec = cls._mapping_at(normalized, ("spec",))
            expected_spec = cls._mapping_at(expected, ("spec",))
            default_strategy = {
                "rollingUpdate": {"maxSurge": "25%", "maxUnavailable": "25%"},
                "type": "RollingUpdate",
            }
            if (
                live_spec is not None
                and expected_spec is not None
                and "strategy" not in expected_spec
                and live_spec.get("strategy") == default_strategy
            ):
                live_spec.pop("strategy")
            live_pod_spec = cls._mapping_at(normalized, ("spec", "template", "spec"))
            expected_pod_spec = cls._mapping_at(expected, ("spec", "template", "spec"))
            live_containers = live_pod_spec.get("containers") if live_pod_spec is not None else None
            expected_containers = (
                expected_pod_spec.get("containers") if expected_pod_spec is not None else None
            )
            if isinstance(live_containers, list) and isinstance(expected_containers, list):
                expected_by_name = {
                    item.get("name"): item
                    for item in expected_containers
                    if isinstance(item, dict) and isinstance(item.get("name"), str)
                }
                for live_container in live_containers:
                    if not isinstance(live_container, dict):
                        continue
                    expected_container = expected_by_name.get(live_container.get("name"))
                    if not isinstance(expected_container, dict):
                        continue
                    for field_name, default in (
                        ("terminationMessagePath", "/dev/termination-log"),
                        ("terminationMessagePolicy", "File"),
                    ):
                        if (
                            field_name not in expected_container
                            and live_container.get(field_name) == default
                        ):
                            live_container.pop(field_name)
        elif identity.kind == "PersistentVolumeClaim":
            cls._drop_defaulted_mapping_field(
                normalized, expected, ("spec", "volumeMode"), "Filesystem"
            )
        elif identity.kind == "PodDisruptionBudget":
            cls._drop_defaulted_mapping_field(
                normalized, expected, ("spec", "unhealthyPodEvictionPolicy"), "IfHealthyBudget"
            )
        elif identity.kind == "NetworkPolicy":
            live_spec = cls._mapping_at(normalized, ("spec",))
            expected_spec = cls._mapping_at(expected, ("spec",))
            if (
                live_spec is not None
                and expected_spec is not None
                and "policyTypes" not in expected_spec
            ):
                default_policy_types: list[str] = []
                if "ingress" in expected_spec or "egress" not in expected_spec:
                    default_policy_types.append("Ingress")
                if "egress" in expected_spec:
                    default_policy_types.append("Egress")
                if live_spec.get("policyTypes") == default_policy_types:
                    live_spec.pop("policyTypes")

        return normalized

    @staticmethod
    def first_unreviewed_live_configuration_path(
        live: object, expected: object, path: str = "$"
    ) -> str | None:
        """Return the first live-only configuration path, if any.

        Before an apply, changed values at a field already represented by the
        reviewed manifest may be reconciled. A new label, annotation, spec
        field, or list item is different: Server-Side Apply may preserve it
        under another manager, so the delivery contract rejects it rather than
        letting the field become an implicit part of the deployment.
        """

        if isinstance(live, dict):
            if not isinstance(expected, dict):
                return path
            for key in sorted(live):
                if key not in expected:
                    return f"{path}.{key}"
                nested = Delivery.first_unreviewed_live_configuration_path(
                    live[key], expected[key], f"{path}.{key}"
                )
                if nested is not None:
                    return nested
            return None
        if isinstance(live, list):
            if not isinstance(expected, list):
                return path
            if len(live) > len(expected):
                return path
            for index, value in enumerate(live):
                nested = Delivery.first_unreviewed_live_configuration_path(
                    value, expected[index], f"{path}[{index}]"
                )
                if nested is not None:
                    return nested
        return None

    @classmethod
    def inventory_from_resources(
        cls,
        resources: list[dict[str, Any]],
        target: TargetPolicy,
        *,
        source: str,
        expected_configurations: dict[ManagedResource, dict[str, object]] | None = None,
    ) -> tuple[ManagedResourceConfiguration, ...]:
        inventory: list[ManagedResourceConfiguration] = []
        seen: set[ManagedResource] = set()
        for resource in resources:
            kind = resource.get("kind")
            metadata = resource.get("metadata")
            if kind not in NAMESPACED_KINDS or not isinstance(metadata, dict):
                fail(f"{source} contains a forbidden cluster-scoped or unsupported resource")
            name = metadata.get("name")
            if metadata.get("namespace") != target.k8s_namespace or not isinstance(name, str) or not NAME.fullmatch(name):
                fail(f"{source} contains a resource outside the approved namespace")
            labels = metadata.get("labels")
            if not isinstance(labels, dict) or labels.get(MANAGED_RESOURCE_LABEL) != MANAGED_RESOURCE_LABEL_VALUE:
                fail(f"{source} resource is missing the required managed-resource label")
            identity = ManagedResource(kind=kind, name=name)
            if identity in seen:
                fail(f"{source} contains a duplicate managed resource")
            seen.add(identity)
            normalized = cls.normalized_managed_resource(resource, identity, source=source)
            if expected_configurations is not None and identity in expected_configurations:
                normalized = cls.normalized_live_managed_resource(
                    resource,
                    identity,
                    expected_configurations[identity],
                    source=source,
                )
            inventory.append(
                ManagedResourceConfiguration(
                    resource=identity,
                    configuration_sha256=hashlib.sha256(canonical_json_bytes(normalized)).hexdigest(),
                    normalized_configuration=normalized,
                )
            )
        return tuple(sorted(inventory))

    def validate_rendered_manifest(
        self, manifest: Path, env: dict[str, str], target: TargetPolicy
    ) -> tuple[ManagedResourceConfiguration, ...]:
        # Client-side decoding reads the exact temporary manifest without making
        # an API mutation. Only the narrow namespaced workload surface is
        # permitted; namespace creation and Secrets are separately bootstrapped.
        decoded = command(
            ["kubectl", "apply", "--dry-run=client", "--validate=true", "-f", str(manifest), "-o", "json"],
            env,
            raw=True,
        )
        resources = self._json_objects(decoded, source="kubectl client validation")
        inventory = self.inventory_from_resources(resources, target, source="rendered manifest")
        deployments = 0
        for resource in resources:
            kind = resource.get("kind")
            metadata = resource.get("metadata")
            assert isinstance(metadata, dict)
            if kind == "Deployment":
                deployments += 1
                if metadata.get("name") != target.deployment_name:
                    fail("rendered manifest contains an unapproved Deployment")
                containers = (
                    resource.get("spec", {})
                    if isinstance(resource.get("spec"), dict)
                    else {}
                )
                containers = containers.get("template", {}) if isinstance(containers.get("template"), dict) else {}
                containers = containers.get("spec", {}) if isinstance(containers.get("spec"), dict) else {}
                containers = containers.get("containers") if isinstance(containers.get("containers"), list) else []
                router_container = next(
                    (item for item in containers if isinstance(item, dict) and item.get("name") == target.container_name),
                    None,
                )
                if not router_container or router_container.get("image") != self.args.image_digest:
                    fail("rendered Deployment does not pin the approved router container to IMAGE_DIGEST")
        if deployments != 1:
            fail("rendered manifest must contain exactly one approved router Deployment")
        return inventory

    def verify_server_side_manifest(
        self, manifest: Path, env: dict[str, str], target: TargetPolicy
    ) -> None:
        """Verify server acceptance without using its object output as desired state.

        Server-Side Apply dry-run is useful for detecting API validation and
        field-ownership conflicts before a mutation. It is not a source of
        truth for the desired fingerprint because the API may merge fields
        owned by other managers into its response. The client-rendered
        inventory remains authoritative; dry-run output is treated only as a
        candidate live object that must already match it after narrow API
        default normalization.
        """

        payload = command(
            [
                "kubectl",
                "apply",
                "--server-side",
                "--dry-run=server",
                "--validate=true",
                "-f",
                str(manifest),
                "-o",
                "json",
            ],
            env,
            raw=True,
        )
        resources = self._json_objects(payload, source="kubectl server-side manifest validation")
        expected_configurations = {
            item.resource: item.normalized_configuration for item in self.managed_inventory
        }
        candidate_inventory = self.inventory_from_resources(
            resources,
            target,
            source="kubectl server-side manifest validation",
            expected_configurations=expected_configurations,
        )
        if managed_resource_identities(candidate_inventory) != managed_resource_identities(
            self.managed_inventory
        ):
            fail("server-side manifest validation changed the managed-resource inventory")
        if managed_resource_spec_fingerprint(candidate_inventory) != managed_resource_spec_fingerprint(
            self.managed_inventory
        ):
            fail(
                "server-side manifest validation contains configuration absent from the reviewed staging manifest"
            )
        self.event("server_side_manifest_validation", result="passed")

    def render(self, env: dict[str, str], temporary_dir: Path, target: TargetPolicy) -> Path:
        source_root = target.kustomize_overlay.parents[2]
        with tempfile.TemporaryDirectory(prefix="smartrouter-kustomize-") as temporary:
            copied_root = Path(temporary) / source_root.name
            shutil.copytree(source_root, copied_root)
            relative_overlay = target.kustomize_overlay.relative_to(source_root)
            copied_overlay = copied_root / relative_overlay
            # kustomize edit operates on the process working directory; run it only
            # in the copied overlay so a render cannot modify tracked manifests.
            proc = subprocess.run(
                ["kustomize", "edit", "set", "image", f"smart-llmrouter={self.args.image_digest}"],
                cwd=copied_overlay,
                env=env,
                text=True,
                capture_output=True,
            )
            if proc.returncode:
                fail(f"kustomize image override failed: {scrub(proc.stderr)}")
            # This is an operational input, not an evidence/log value. Keep it
            # byte-for-byte for kubectl, but only record its checksum below.
            rendered = command(["kustomize", "build", str(copied_overlay)], env, raw=True)
        output = temporary_dir / "rendered.yaml"
        output.write_bytes(rendered.encode("utf-8"))
        self.managed_inventory = self.validate_rendered_manifest(output, env, target)
        if self.args.action in {"plan", "apply", "smoke", "promotion-plan"}:
            self.verify_server_side_manifest(output, env, target)
        # The manifest is only a temporary kubectl input. Evidence retains its
        # checksum, never its contents. The configuration fingerprint replaces
        # the exact router image digest with a fixed marker before hashing so
        # it captures deployment configuration independently of the promoted
        # artifact; IMAGE_DIGEST is bound separately.
        rendered_bytes = rendered.encode("utf-8")
        rendered_manifest_sha256 = hashlib.sha256(rendered_bytes).hexdigest()
        configuration_bytes = rendered_bytes.replace(
            self.args.image_digest.encode("utf-8"), b"<ROUTER_IMAGE_DIGEST>"
        )
        self.evidence["rendered_manifest_sha256"] = rendered_manifest_sha256
        self.evidence["configuration_fingerprint"] = hashlib.sha256(configuration_bytes).hexdigest()
        self.evidence["rendered_manifest_bytes"] = len(rendered_bytes)
        self.evidence["managed_resource_count"] = len(self.managed_inventory)
        self.evidence["managed_resource_inventory_sha256"] = inventory_fingerprint(
            managed_resource_identities(self.managed_inventory)
        )
        self.evidence["managed_resource_spec_sha256"] = managed_resource_spec_fingerprint(
            self.managed_inventory
        )
        self.event(
            "render",
            artifact="temporary-manifest",
            managed_resource_count=len(self.managed_inventory),
            managed_resource_inventory_sha256=inventory_fingerprint(
                managed_resource_identities(self.managed_inventory)
            ),
            managed_resource_spec_sha256=managed_resource_spec_fingerprint(self.managed_inventory),
        )
        return output

    def live_managed_inventory(
        self, env: dict[str, str], target: TargetPolicy
    ) -> tuple[ManagedResourceConfiguration, ...]:
        payload = command(
            [
                "kubectl",
                "get",
                ",".join(MANAGED_RESOURCE_TYPES),
                "-n",
                target.k8s_namespace,
                "-l",
                f"{MANAGED_RESOURCE_LABEL}={MANAGED_RESOURCE_LABEL_VALUE}",
                "-o",
                "json",
            ],
            env,
            raw=True,
        )
        resources = self._json_objects(
            payload, source="live managed-resource inventory", allow_empty=True
        )
        expected_configurations = {
            item.resource: item.normalized_configuration for item in self.managed_inventory
        }
        return self.inventory_from_resources(
            resources,
            target,
            source="live managed-resource inventory",
            expected_configurations=expected_configurations,
        )

    def record_live_managed_inventory(
        self, live_inventory: tuple[ManagedResourceConfiguration, ...], phase: str, result: str
    ) -> None:
        expected_fingerprint = inventory_fingerprint(managed_resource_identities(self.managed_inventory))
        live_fingerprint = inventory_fingerprint(managed_resource_identities(live_inventory))
        expected_spec_fingerprint = managed_resource_spec_fingerprint(self.managed_inventory)
        live_spec_fingerprint = managed_resource_spec_fingerprint(live_inventory)
        self.evidence.update(
            {
                "managed_resource_count": len(self.managed_inventory),
                "managed_resource_inventory_sha256": expected_fingerprint,
                "managed_resource_spec_sha256": expected_spec_fingerprint,
                "live_managed_resource_count": len(live_inventory),
                "live_managed_resource_inventory_sha256": live_fingerprint,
                "live_managed_resource_spec_sha256": live_spec_fingerprint,
            }
        )
        self.event(
            "managed_inventory",
            phase=phase,
            result=result,
            expected_count=len(self.managed_inventory),
            expected_sha256=expected_fingerprint,
            expected_spec_sha256=expected_spec_fingerprint,
            live_count=len(live_inventory),
            live_sha256=live_fingerprint,
            live_spec_sha256=live_spec_fingerprint,
        )

    def verify_no_stale_managed_resources(
        self, env: dict[str, str], target: TargetPolicy
    ) -> None:
        if not self.managed_inventory:
            fail("rendered managed-resource inventory is unavailable")
        live_inventory = self.live_managed_inventory(env, target)
        expected = set(managed_resource_identities(self.managed_inventory))
        stale = tuple(
            item.resource for item in live_inventory if item.resource not in expected
        )
        if stale:
            self.record_live_managed_inventory(live_inventory, "before_apply", "stale_resources")
            fail("live managed-resource inventory contains stale resources not in the rendered staging manifest")
        expected_configurations = {
            item.resource: item.normalized_configuration for item in self.managed_inventory
        }
        for item in live_inventory:
            unexpected_path = self.first_unreviewed_live_configuration_path(
                item.normalized_configuration,
                expected_configurations[item.resource],
            )
            if unexpected_path is not None:
                self.record_live_managed_inventory(live_inventory, "before_apply", "unreviewed_fields")
                fail(
                    "live managed-resource configuration contains fields absent from the reviewed staging manifest"
                )
        self.record_live_managed_inventory(live_inventory, "before_apply", "no_stale_resources")

    def verify_managed_inventory(
        self, env: dict[str, str], target: TargetPolicy, phase: str
    ) -> None:
        if not self.managed_inventory:
            fail("rendered managed-resource inventory is unavailable")
        live_inventory = self.live_managed_inventory(env, target)
        if managed_resource_identities(live_inventory) != managed_resource_identities(
            self.managed_inventory
        ):
            self.record_live_managed_inventory(live_inventory, phase, "mismatch")
            fail("live managed-resource inventory does not match the rendered staging manifest")
        if managed_resource_spec_fingerprint(live_inventory) != managed_resource_spec_fingerprint(
            self.managed_inventory
        ):
            self.record_live_managed_inventory(live_inventory, phase, "configuration_mismatch")
            fail("live managed-resource configuration does not match the rendered staging manifest")
        self.record_live_managed_inventory(live_inventory, phase, "matched")

    def verify_live_deployment(self, env: dict[str, str], target: TargetPolicy, phase: str) -> LiveDeploymentIdentity:
        command(
            [
                "kubectl",
                "rollout",
                "status",
                f"deployment/{target.deployment_name}",
                "-n",
                target.k8s_namespace,
                "--timeout=5m",
            ],
            env,
            quiet=True,
        )
        payload = command(
            [
                "kubectl",
                "get",
                f"deployment/{target.deployment_name}",
                "-n",
                target.k8s_namespace,
                "-o",
                "json",
            ],
            env,
            raw=True,
        )
        try:
            deployment = json.loads(payload)
        except json.JSONDecodeError as exc:
            raise RuntimeError("live deployment lookup returned invalid JSON") from exc
        if not isinstance(deployment, dict):
            fail("live deployment lookup returned an invalid resource")
        metadata = deployment.get("metadata")
        if not isinstance(metadata, dict) or metadata.get("name") != target.deployment_name or metadata.get("namespace") != target.k8s_namespace:
            fail("live deployment lookup does not match the approved target")
        generation = metadata.get("generation")
        status = deployment.get("status") if isinstance(deployment.get("status"), dict) else {}
        observed_generation = status.get("observedGeneration")
        if type(generation) is not int or generation < 1 or type(observed_generation) is not int or observed_generation != generation:
            fail("live deployment generation is not fully observed after rollout")
        spec = deployment.get("spec") if isinstance(deployment.get("spec"), dict) else {}
        template = spec.get("template") if isinstance(spec.get("template"), dict) else {}
        if not template:
            fail("live deployment lookup has no pod template")
        pod_spec = template.get("spec") if isinstance(template.get("spec"), dict) else {}
        containers = pod_spec.get("containers") if isinstance(pod_spec.get("containers"), list) else []
        router_container = next(
            (item for item in containers if isinstance(item, dict) and item.get("name") == target.container_name),
            None,
        )
        if not router_container or router_container.get("image") != self.args.image_digest:
            fail("live router Deployment does not use the requested immutable IMAGE_DIGEST")
        identity = LiveDeploymentIdentity(
            generation=generation,
            observed_generation=observed_generation,
            pod_template_sha256=hashlib.sha256(canonical_json_bytes(template)).hexdigest(),
        )
        self.evidence.update(
            {
                "live_deployment_generation": identity.generation,
                "live_deployment_observed_generation": identity.observed_generation,
                "live_pod_template_sha256": identity.pod_template_sha256,
            }
        )
        self.event(
            "live_deployment",
            phase=phase,
            result="digest-rollout-and-pod-template-verified",
            generation=identity.generation,
            observed_generation=identity.observed_generation,
            pod_template_sha256=identity.pod_template_sha256,
        )
        return identity

    def rollback_revision_for_digest(self, env: dict[str, str], target: TargetPolicy) -> int:
        """Find a prior deployment revision that already uses IMAGE_DIGEST.

        ``kubectl rollout undo`` otherwise selects whichever revision happens
        to be immediately previous. Resolve the requested immutable artifact
        before any mutation and use its explicit revision instead.
        """

        deployment_payload = command(
            [
                "kubectl",
                "get",
                f"deployment/{target.deployment_name}",
                "-n",
                target.k8s_namespace,
                "-o",
                "json",
            ],
            env,
            raw=True,
        )
        try:
            deployment = json.loads(deployment_payload)
        except json.JSONDecodeError as exc:
            raise RuntimeError("rollback deployment lookup returned invalid JSON") from exc
        if not isinstance(deployment, dict):
            fail("rollback deployment lookup returned an invalid resource")
        metadata = deployment.get("metadata")
        if (
            not isinstance(metadata, dict)
            or metadata.get("name") != target.deployment_name
            or metadata.get("namespace") != target.k8s_namespace
        ):
            fail("rollback deployment lookup does not match the approved target")
        deployment_uid = metadata.get("uid")
        if not isinstance(deployment_uid, str) or not deployment_uid:
            fail("rollback deployment lookup has no stable Deployment UID")
        annotations = metadata.get("annotations")
        revision_value = annotations.get("deployment.kubernetes.io/revision") if isinstance(annotations, dict) else None
        if not isinstance(revision_value, str) or not revision_value.isdigit() or int(revision_value) < 2:
            fail("rollback requires a current Deployment revision with prior history")
        current_revision = int(revision_value)

        replica_set_payload = command(
            [
                "kubectl",
                "get",
                "replicasets",
                "-n",
                target.k8s_namespace,
                "-l",
                f"{MANAGED_RESOURCE_LABEL}={MANAGED_RESOURCE_LABEL_VALUE}",
                "-o",
                "json",
            ],
            env,
            raw=True,
        )
        replica_sets = self._json_objects(
            replica_set_payload, source="rollback ReplicaSet lookup", allow_empty=True
        )
        candidates: list[int] = []
        for replica_set in replica_sets:
            if replica_set.get("kind") != "ReplicaSet":
                fail("rollback ReplicaSet lookup returned an invalid resource")
            replica_metadata = replica_set.get("metadata")
            if (
                not isinstance(replica_metadata, dict)
                or replica_metadata.get("namespace") != target.k8s_namespace
            ):
                fail("rollback ReplicaSet lookup returned a resource outside the approved namespace")
            owners = replica_metadata.get("ownerReferences")
            if not isinstance(owners, list) or not any(
                isinstance(owner, dict)
                and owner.get("kind") == "Deployment"
                and owner.get("name") == target.deployment_name
                and owner.get("uid") == deployment_uid
                and owner.get("controller") is True
                for owner in owners
            ):
                continue
            replica_annotations = replica_metadata.get("annotations")
            replica_revision = (
                replica_annotations.get("deployment.kubernetes.io/revision")
                if isinstance(replica_annotations, dict)
                else None
            )
            if (
                not isinstance(replica_revision, str)
                or not replica_revision.isdigit()
                or int(replica_revision) < 1
            ):
                fail("rollback ReplicaSet has no valid Deployment revision")
            revision = int(replica_revision)
            if revision >= current_revision:
                continue
            spec = replica_set.get("spec") if isinstance(replica_set.get("spec"), dict) else {}
            template = spec.get("template") if isinstance(spec.get("template"), dict) else {}
            pod_spec = template.get("spec") if isinstance(template.get("spec"), dict) else {}
            containers = pod_spec.get("containers") if isinstance(pod_spec.get("containers"), list) else []
            router_container = next(
                (
                    item
                    for item in containers
                    if isinstance(item, dict) and item.get("name") == target.container_name
                ),
                None,
            )
            if router_container and router_container.get("image") == self.args.image_digest:
                candidates.append(revision)
        if not candidates:
            fail("no prior Deployment revision uses the requested approved IMAGE_DIGEST")
        revision = max(candidates)
        self.evidence["rollback_target_revision"] = revision
        self.event("rollback_target", revision=revision, result="approved_digest_revision_resolved")
        return revision

    def run(self) -> None:
        env, temp = self.env_with_kubeconfig()
        try:
            target = self.preflight(env)
            manifest = self.render(env, Path(temp.name), target) if self.args.action in {"render", "plan", "apply", "smoke", "promotion-plan"} else None
            if self.args.action == "status":
                status = command(
                    [
                        "kubectl",
                        "get",
                        "deployment,pods,service,ingress,pdb,networkpolicy,pvc",
                        "-n",
                        target.k8s_namespace,
                        "-o",
                        "name",
                    ],
                    env,
                )
                self.event("rollout_status", resources=scrub(status))
            elif self.args.action == "plan":
                self.event("server_side_dry_run", result="passed")
            elif self.args.action == "apply":
                # Never use broad prune. If a previous overlay left an
                # allowlisted, label-selected object behind, stop before this
                # mutating apply and require an explicit reviewed recovery.
                self.verify_no_stale_managed_resources(env, target)
                command(["kubectl", "apply", "--server-side", "-f", str(manifest)], env, quiet=True)
                self.verify_managed_inventory(env, target, "after_apply")
                self.verify_live_deployment(env, target, "after_apply")
                self.event("apply", result="passed")
            elif self.args.action == "rollback":
                rollback_revision = self.rollback_revision_for_digest(env, target)
                command(
                    [
                        "kubectl",
                        "rollout",
                        "undo",
                        f"deployment/{target.deployment_name}",
                        "-n",
                        target.k8s_namespace,
                        "--to-revision",
                        str(rollback_revision),
                    ],
                    env,
                    quiet=True,
                )
                self.verify_live_deployment(env, target, "after_rollback")
                self.event("rollback", result="passed")
            elif self.args.action == "smoke":
                if self.smoke_command_fd is None:
                    fail("EKS_SMOKE_COMMAND_FILE is required")
                self.verify_managed_inventory(env, target, "before_smoke")
                before_smoke = self.verify_live_deployment(env, target, "before_smoke")
                proc = subprocess.run(
                    ["/bin/sh", "-s"],
                    stdin=self.smoke_command_fd,
                    env=env,
                    text=True,
                    capture_output=True,
                )
                if proc.returncode:
                    # Do not persist or print output from a credential-bearing
                    # protected smoke script.  Its private runner/CI log is the
                    # diagnostic source; contract evidence records only the
                    # exit status through this redacted error.
                    fail(f"protected staging smoke failed (exit {proc.returncode}); output is not recorded")
                self.verify_managed_inventory(env, target, "after_smoke")
                after_smoke = self.verify_live_deployment(env, target, "after_smoke")
                if before_smoke != after_smoke:
                    fail("live router Deployment changed during the protected staging smoke")
                self.event("smoke", result="passed")
            elif self.args.action == "promotion-plan":
                # Evidence alone cannot prove the staging workload was not
                # rolled back or replaced after the smoke. Re-check the live
                # rollout and named router container before it can support a
                # promotion decision.
                self.verify_managed_inventory(env, target, "promotion_plan")
                self.verify_live_deployment(env, target, "promotion_plan")
                records: dict[str, dict[str, object]] = {}
                for action, required_event in (("apply", "apply"), ("smoke", "smoke")):
                    evidence_file = self.evidence_dir / f"evidence-{action}.json"
                    if not evidence_file.is_file():
                        fail(f"passed staging {action} evidence is required before a promotion plan")
                    record = json.loads(evidence_file.read_text(encoding="utf-8"))
                    events = record.get("events", [])
                    if record.get("action") != action or record.get("outcome") != "passed" or not any(
                        isinstance(event, dict) and event.get("name") == required_event for event in events
                    ):
                        fail(f"staging {action} evidence is incomplete or not passed")
                    records[action] = record
                for key in (
                    "environment",
                    "aws_account_id",
                    "aws_region",
                    "ecr_repository_uri",
                    "eks_cluster",
                    "k8s_namespace",
                    "kustomize_overlay",
                    "deployment_name",
                    "container_name",
                    "target_policy_arn",
                    "target_policy_sha256",
                    "image_digest",
                    "configuration_fingerprint",
                    "managed_resource_count",
                    "managed_resource_inventory_sha256",
                    "managed_resource_spec_sha256",
                    "live_managed_resource_count",
                    "live_managed_resource_inventory_sha256",
                    "live_managed_resource_spec_sha256",
                    "live_deployment_generation",
                    "live_deployment_observed_generation",
                    "live_pod_template_sha256",
                ):
                    if records["apply"].get(key) != records["smoke"].get(key) or records["apply"].get(key) != self.evidence.get(key):
                        fail(f"apply and smoke evidence disagree on {key}")
                self.event("promotion_plan", result="review_required_no_production_apply")
            self.write_evidence("passed")
        except Exception as exc:
            self.write_evidence("failed", str(exc))
            raise
        finally:
            temp.cleanup()
            if self.smoke_command_fd is not None:
                os.close(self.smoke_command_fd)
                self.smoke_command_fd = None


def parser() -> argparse.ArgumentParser:
    # Do not let the removed --smoke-command form abbreviate the protected
    # --smoke-command-file argument and reintroduce command content in argv.
    p = argparse.ArgumentParser(description=__doc__, allow_abbrev=False)
    p.add_argument("action", choices=("preflight", "status", "render", "plan", "apply", "rollback", "smoke", "promotion-plan"))
    p.add_argument("--aws-profile", required=True)
    p.add_argument("--image-digest", default="")
    p.add_argument("--confirm", default="")
    p.add_argument("--evidence-dir", required=True)
    p.add_argument("--smoke-command-file", default="")
    return p


def main() -> int:
    args = parser().parse_args()
    try:
        Delivery(args).run()
        print(json.dumps({"action": args.action, "outcome": "passed", "evidence_dir": args.evidence_dir}))
        return 0
    except Exception as exc:
        print(f"EKS delivery contract failed: {scrub(str(exc))}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
