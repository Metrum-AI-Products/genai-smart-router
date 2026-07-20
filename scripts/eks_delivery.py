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
import datetime as dt
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass
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


def inventory_fingerprint(inventory: tuple[ManagedResource, ...]) -> str:
    """Return a stable, non-sensitive digest for an allowlisted inventory."""

    return hashlib.sha256(
        canonical_json_bytes(
            [{"kind": resource.kind, "name": resource.name} for resource in inventory]
        )
    ).hexdigest()


def parse_target_policy(value: object) -> TargetPolicy:
    if not isinstance(value, dict):
        fail("approved staging target policy is not a JSON object")
    required = {
        "schema_version",
        "environment",
        "aws_account_id",
        "aws_region",
        "ssm_parameter_arn",
        "eks_cluster",
        "k8s_namespace",
        "kustomize_overlay",
        "deployment_name",
        "container_name",
        "delivery_role_name",
    }
    if set(value) != required:
        fail("approved staging target policy has an unexpected schema")
    if value.get("schema_version") != 1 or value.get("environment") != "staging":
        fail("approved staging target policy is not a supported staging policy")

    def string(name: str) -> str:
        item = value.get(name)
        if not isinstance(item, str) or not item:
            fail(f"approved staging target policy has invalid {name}")
        return item

    account_id = string("aws_account_id")
    region = string("aws_region")
    cluster = string("eks_cluster")
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
        "environment": "staging",
        "eks_cluster": cluster,
        "k8s_namespace": namespace,
        "kustomize_overlay": overlay.as_posix(),
        "schema_version": 1,
        "ssm_parameter_arn": parameter_arn,
    }
    return TargetPolicy(
        aws_account_id=account_id,
        aws_region=region,
        container_name=container,
        delivery_role_name=role,
        deployment_name=deployment,
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
        self._validate_inputs()
        self.evidence_dir = Path(args.evidence_dir).resolve()
        self.evidence_dir.mkdir(parents=True, exist_ok=True)
        self.target: TargetPolicy | None = None
        self.managed_inventory: tuple[ManagedResource, ...] = ()
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
        if self.args.action in {"render", "plan", "apply", "smoke", "promotion-plan"}:
            if not self.args.image_digest or not DIGEST.fullmatch(self.args.image_digest):
                fail("IMAGE_DIGEST must be a lower-case immutable image@sha256:<64 hex> reference")
        if self.args.action in {"apply", "rollback"} and self.args.confirm != "STAGING_APPLY":
            fail("mutating action requires EKS_CONFIRM=STAGING_APPLY")

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
            "eks_cluster",
            "k8s_namespace",
            "target_policy_sha256",
            "image_digest",
            "configuration_fingerprint",
            "managed_resource_count",
            "managed_resource_inventory_sha256",
            "live_managed_resource_count",
            "live_managed_resource_inventory_sha256",
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

    def preflight(self, env: dict[str, str]) -> TargetPolicy:
        for tool in ("aws", "kubectl", "kustomize"):
            if not shutil.which(tool):
                fail(f"required executable not found: {tool}")
        # The external policy is read before any EKS selection. It must match
        # the reviewed copy in Git exactly; the delivery role gets only
        # ssm:GetParameter on this one Parameter, never PutParameter.
        target = self.load_target_policy(env)
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
        self.event(
            "preflight",
            aws_account_id=actual_account,
            aws_principal_type="assumed-role",
            cluster_status="ACTIVE",
            rbac_get_pods=allowed,
            rbac_list_managed_resource_types=len(MANAGED_RESOURCE_TYPES),
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
    def inventory_from_resources(
        resources: list[dict[str, Any]], target: TargetPolicy, *, source: str
    ) -> tuple[ManagedResource, ...]:
        inventory: list[ManagedResource] = []
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
            inventory.append(identity)
        return tuple(sorted(inventory))

    def validate_rendered_manifest(
        self, manifest: Path, env: dict[str, str], target: TargetPolicy
    ) -> tuple[ManagedResource, ...]:
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
        self.evidence["managed_resource_inventory_sha256"] = inventory_fingerprint(self.managed_inventory)
        self.event(
            "render",
            artifact="temporary-manifest",
            managed_resource_count=len(self.managed_inventory),
            managed_resource_inventory_sha256=inventory_fingerprint(self.managed_inventory),
        )
        return output

    def live_managed_inventory(
        self, env: dict[str, str], target: TargetPolicy
    ) -> tuple[ManagedResource, ...]:
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
        return self.inventory_from_resources(
            resources, target, source="live managed-resource inventory"
        )

    def record_live_managed_inventory(
        self, live_inventory: tuple[ManagedResource, ...], phase: str, result: str
    ) -> None:
        expected_fingerprint = inventory_fingerprint(self.managed_inventory)
        live_fingerprint = inventory_fingerprint(live_inventory)
        self.evidence.update(
            {
                "managed_resource_count": len(self.managed_inventory),
                "managed_resource_inventory_sha256": expected_fingerprint,
                "live_managed_resource_count": len(live_inventory),
                "live_managed_resource_inventory_sha256": live_fingerprint,
            }
        )
        self.event(
            "managed_inventory",
            phase=phase,
            result=result,
            expected_count=len(self.managed_inventory),
            expected_sha256=expected_fingerprint,
            live_count=len(live_inventory),
            live_sha256=live_fingerprint,
        )

    def verify_no_stale_managed_resources(
        self, env: dict[str, str], target: TargetPolicy
    ) -> None:
        if not self.managed_inventory:
            fail("rendered managed-resource inventory is unavailable")
        live_inventory = self.live_managed_inventory(env, target)
        expected = set(self.managed_inventory)
        stale = tuple(resource for resource in live_inventory if resource not in expected)
        if stale:
            self.record_live_managed_inventory(live_inventory, "before_apply", "stale_resources")
            fail("live managed-resource inventory contains stale resources not in the rendered staging manifest")
        self.record_live_managed_inventory(live_inventory, "before_apply", "no_stale_resources")

    def verify_managed_inventory(
        self, env: dict[str, str], target: TargetPolicy, phase: str
    ) -> None:
        if not self.managed_inventory:
            fail("rendered managed-resource inventory is unavailable")
        live_inventory = self.live_managed_inventory(env, target)
        if live_inventory != self.managed_inventory:
            self.record_live_managed_inventory(live_inventory, phase, "mismatch")
            fail("live managed-resource inventory does not match the rendered staging manifest")
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
                command(["kubectl", "apply", "--server-side", "--dry-run=server", "-f", str(manifest)], env, quiet=True)
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
                command(["kubectl", "rollout", "undo", f"deployment/{target.deployment_name}", "-n", target.k8s_namespace], env, quiet=True)
                command(["kubectl", "rollout", "status", f"deployment/{target.deployment_name}", "-n", target.k8s_namespace, "--timeout=5m"], env, quiet=True)
                self.event("rollback", result="passed")
            elif self.args.action == "smoke":
                if not self.args.smoke_command:
                    fail("EKS_SMOKE_COMMAND is required and is never logged")
                self.verify_managed_inventory(env, target, "before_smoke")
                before_smoke = self.verify_live_deployment(env, target, "before_smoke")
                proc = subprocess.run(self.args.smoke_command, shell=True, env=env, text=True, capture_output=True)
                if proc.returncode:
                    fail(f"protected staging smoke failed (exit {proc.returncode}): {scrub(proc.stderr or proc.stdout)}")
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
                    "live_managed_resource_count",
                    "live_managed_resource_inventory_sha256",
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


def parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("action", choices=("preflight", "status", "render", "plan", "apply", "rollback", "smoke", "promotion-plan"))
    p.add_argument("--aws-profile", required=True)
    p.add_argument("--image-digest", default="")
    p.add_argument("--confirm", default="")
    p.add_argument("--evidence-dir", required=True)
    p.add_argument("--smoke-command", default="")
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
