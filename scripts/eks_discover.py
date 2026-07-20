#!/usr/bin/env python3
"""Produce a redacted, read-only EKS bootstrap inventory.

This intentionally does not implement the shared operator/CI command contract;
that belongs to issue #519.  It is the primitive those commands call.
"""

from __future__ import annotations

import argparse
import fcntl
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


class DiscoveryError(RuntimeError):
    def __init__(self, message: str, *, code: str = "") -> None:
        super().__init__(message)
        self.code = code


DNS_LABEL = re.compile(r"^[a-z0-9](?:[-a-z0-9]{0,61}[a-z0-9])?$")
TRUST_DOMAIN = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$")
DISCOVERY_ROLE_NAME = "genai-smart-router-eks-discovery"
ROOT = Path(__file__).resolve().parents[1]


def run(command: list[str], *, required: bool = True) -> str:
    try:
        return subprocess.check_output(command, text=True, stderr=subprocess.PIPE)
    except FileNotFoundError as exc:
        raise DiscoveryError(f"required command is unavailable: {command[0]}") from exc
    except subprocess.CalledProcessError as exc:
        # Never copy command stderr into a report or error: AWS and kubectl can
        # include endpoints, credentials, or request details there.  Preserve
        # only the one safe AWS classification this read-only workflow needs.
        code = "RepositoryPolicyNotFoundException" if "RepositoryPolicyNotFoundException" in (exc.stderr or "") else ""
        raise DiscoveryError(f"{command[0]} command failed", code=code) from exc


def aws_json(args: list[str], region: str, profile: str) -> dict[str, Any]:
    return json.loads(run(["aws", "--profile", profile, *args, "--region", region, "--output", "json"]))


def ecr_policy_present(repository: str, region: str, profile: str) -> bool:
    """Distinguish an absent repository policy from an inaccessible repository."""
    try:
        return bool(aws_json(["ecr", "get-repository-policy", "--repository-name", repository], region, profile).get("policyText"))
    except DiscoveryError as exc:
        if exc.code == "RepositoryPolicyNotFoundException":
            return False
        raise


def kubectl_json(kubeconfig: Path, args: list[str]) -> dict[str, Any]:
    return json.loads(run(["kubectl", "--kubeconfig", str(kubeconfig), *args, "-o", "json"]))


def item_names(payload: dict[str, Any]) -> list[str]:
    return sorted(item.get("metadata", {}).get("name", "") for item in payload.get("items", []) if item.get("metadata", {}).get("name"))


def namespace_labels(payload: dict[str, Any]) -> dict[str, str]:
    labels = payload.get("metadata", {}).get("labels", {})
    # Labels are required for injection-policy discovery. Do not include arbitrary
    # annotations, which commonly contain endpoints or secret-manager references.
    return {str(k): str(v) for k, v in sorted(labels.items())}


def linkerd_injection_annotation(payload: dict[str, Any]) -> str:
    return str(payload.get("metadata", {}).get("annotations", {}).get("linkerd.io/inject", ""))


def is_dns_subdomain(value: Any) -> bool:
    """Return whether a Kubernetes object name is a DNS subdomain."""
    return (
        isinstance(value, str)
        and 0 < len(value) <= 253
        and all(DNS_LABEL.fullmatch(label) for label in value.split("."))
    )


def linkerd_ingress_identity(namespace: str, service_account: str, control_plane_namespace: str, trust_domain: str) -> str:
    return f"{service_account}.{namespace}.serviceaccount.identity.{control_plane_namespace}.{trust_domain}"


def controller_owned_by(payload: dict[str, Any], kind: str, name: str, uid: str) -> bool:
    """Check the full controller owner reference, including UID across recreations."""
    return any(
        isinstance(owner, dict)
        and owner.get("controller") is True
        and owner.get("kind") == kind
        and owner.get("name") == name
        and owner.get("uid") == uid
        for owner in payload.get("metadata", {}).get("ownerReferences", [])
    )


def non_terminating_items(payload: dict[str, Any]) -> list[dict[str, Any]]:
    return [
        item
        for item in payload.get("items", [])
        if isinstance(item, dict) and not item.get("metadata", {}).get("deletionTimestamp")
    ]


def proxy_environment_value(container: dict[str, Any], name: str) -> str | None:
    """Return exactly one literal safe proxy environment value, if present."""
    environment = container.get("env")
    if not isinstance(environment, list):
        return None
    values = [entry.get("value") for entry in environment if isinstance(entry, dict) and entry.get("name") == name]
    return values[0] if len(values) == 1 and isinstance(values[0], str) else None


def proxy_local_identity_matches(
    container: dict[str, Any],
    ingress_namespace: str,
    service_account: str,
    control_plane_namespace: str,
    trust_domain: str,
) -> bool:
    """Require the proxy's safe local-identity value to match observed trust config.

    Linkerd injects this non-secret value into every proxy. Discovery derives
    the trust domain from the separate literal `_l5d_trustdomain` value, then
    requires this identity template to agree with it. Trust anchors,
    certificates, tokens, and arbitrary environment values remain outside
    discovery scope.
    """
    local_identity = proxy_environment_value(container, "LINKERD2_PROXY_IDENTITY_LOCAL_NAME")
    if local_identity is None:
        return False
    expected_identity = linkerd_ingress_identity(
        ingress_namespace,
        service_account,
        control_plane_namespace,
        trust_domain,
    )
    expected_injected_template = (
        f"$(_pod_sa).$(_pod_ns).serviceaccount.identity.{control_plane_namespace}.{trust_domain}"
    )
    return local_identity in (expected_identity, expected_injected_template)


def ready_linkerd_ingress_pod(
    payload: dict[str, Any],
    ingress_namespace: str,
    service_account: str,
    control_plane_namespace: str,
) -> str | None:
    spec = payload.get("spec", {})
    status = payload.get("status", {})
    if not isinstance(spec, dict) or not isinstance(status, dict) or spec.get("serviceAccountName") != service_account:
        return None
    ready = any(
        isinstance(condition, dict) and condition.get("type") == "Ready" and condition.get("status") == "True"
        for condition in status.get("conditions", [])
    )
    proxy_ready = any(
        isinstance(container, dict) and container.get("name") == "linkerd-proxy" and container.get("ready") is True
        for container in status.get("containerStatuses", [])
    )
    proxy_containers = [
        container
        for container in spec.get("containers", [])
        if isinstance(container, dict) and container.get("name") == "linkerd-proxy"
    ]
    if not ready or not proxy_ready or len(proxy_containers) != 1:
        return None
    observed_trust_domain = proxy_environment_value(proxy_containers[0], "_l5d_trustdomain")
    if not isinstance(observed_trust_domain, str) or not TRUST_DOMAIN.fullmatch(observed_trust_domain):
        return None
    if not proxy_local_identity_matches(
        proxy_containers[0],
        ingress_namespace,
        service_account,
        control_plane_namespace,
        observed_trust_domain,
    ):
        return None
    return observed_trust_domain


def ingress_workload_evidence(
    deployment: dict[str, Any],
    replica_sets: dict[str, Any],
    pods: dict[str, Any],
    deployment_name: str,
    ingress_namespace: str,
    service_account: str,
    control_plane_namespace: str,
    trust_domain: str,
) -> dict[str, int | bool | str]:
    """Fail closed unless the selected ingress Deployment really presents the mesh identity.

    Service-account existence alone is insufficient: a stale or incorrectly
    selected account would otherwise authorize an identity that no ingress pod
    can present. We inspect only the selected Deployment and its controller
    owned Pods, then retain safe scalar readiness evidence in the report.
    """
    deployment_metadata = deployment.get("metadata", {})
    deployment_uid = deployment_metadata.get("uid") if isinstance(deployment_metadata, dict) else None
    if not isinstance(deployment_uid, str) or not deployment_uid:
        raise DiscoveryError("selected ingress deployment has no stable controller UID")
    deployment_spec = deployment.get("spec", {})
    deployment_status = deployment.get("status", {})
    template = deployment_spec.get("template", {}) if isinstance(deployment_spec, dict) else {}
    template_spec = template.get("spec", {}) if isinstance(template, dict) else {}
    if not isinstance(template_spec, dict) or template_spec.get("serviceAccountName") != service_account:
        raise DiscoveryError("selected ingress deployment does not use the selected service account")
    desired = deployment_spec.get("replicas", 1) if isinstance(deployment_spec, dict) else 1
    available = deployment_status.get("availableReplicas", 0) if isinstance(deployment_status, dict) else 0
    if isinstance(desired, bool) or not isinstance(desired, int) or desired < 1:
        raise DiscoveryError("selected ingress deployment does not declare a positive replica count")
    if isinstance(available, bool) or not isinstance(available, int) or available < desired:
        raise DiscoveryError("selected ingress deployment is not fully available")
    replica_sets_by_name = {
        str(item.get("metadata", {}).get("name")): str(item.get("metadata", {}).get("uid"))
        for item in non_terminating_items(replica_sets)
        if controller_owned_by(item, "Deployment", deployment_name, deployment_uid)
        and isinstance(item.get("metadata", {}).get("name"), str)
        and isinstance(item.get("metadata", {}).get("uid"), str)
    }
    if not replica_sets_by_name:
        raise DiscoveryError("selected ingress deployment has no controller-owned ReplicaSet")
    workload_pods = [
        item
        for item in non_terminating_items(pods)
        if any(controller_owned_by(item, "ReplicaSet", replica_set_name, replica_set_uid) for replica_set_name, replica_set_uid in replica_sets_by_name.items())
    ]
    if len(workload_pods) < desired:
        raise DiscoveryError("selected ingress deployment has fewer running workload pods than desired replicas")
    observed_trust_domains = [
        ready_linkerd_ingress_pod(
            pod,
            ingress_namespace,
            service_account,
            control_plane_namespace,
        )
        for pod in workload_pods
    ]
    if any(domain is None for domain in observed_trust_domains):
        raise DiscoveryError(
            "selected ingress workload is not Linkerd-injected, identity-configured, and ready with the selected service account"
        )
    actual_trust_domains = {domain for domain in observed_trust_domains if domain is not None}
    if len(actual_trust_domains) != 1:
        raise DiscoveryError("selected ingress workload has inconsistent Linkerd proxy trust-domain evidence")
    if actual_trust_domains != {trust_domain}:
        raise DiscoveryError("selected Linkerd trust domain does not match the ready ingress proxy identity evidence")
    return {
        "ingress_workload_kind": "Deployment",
        "ingress_workload_name": deployment_name,
        "ingress_workload_desired_replicas": desired,
        "ingress_workload_ready_pods": len(workload_pods),
        "ingress_workload_verified": True,
        "ingress_workload_mesh_ready": True,
    }


def validate_discovery_identity(identity: dict[str, Any], account_id: str) -> tuple[str, str]:
    """Require the approved account and discovery role without echoing identity data."""
    actual_account = str(identity.get("Account", ""))
    if actual_account != account_id:
        raise DiscoveryError("AWS identity account does not match the explicitly approved account")
    actual_arn = str(identity.get("Arn", ""))
    expected_arn_prefix = f"arn:aws:sts::{account_id}:assumed-role/{DISCOVERY_ROLE_NAME}/"
    if not actual_arn.startswith(expected_arn_prefix):
        raise DiscoveryError("AWS identity is not the expected discovery role")
    return actual_account, actual_arn


def validate_output_path(path: Path) -> None:
    """Reject unsafe direct-script output paths before touching prior evidence."""
    if not path.is_absolute() or path.parent == Path("/"):
        raise DiscoveryError("discovery report output must be an absolute non-root file path")
    if path.is_symlink() or path.is_dir():
        raise DiscoveryError("discovery report output must be a non-symlink file path")
    try:
        if path.resolve().is_relative_to(ROOT.resolve()):
            raise DiscoveryError("discovery report output must be outside the repository")
        path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    except (OSError, RuntimeError) as exc:
        raise DiscoveryError("discovery report output path could not be prepared safely") from exc


def fsync_parent(path: Path) -> None:
    descriptor = os.open(path.parent, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
    try:
        os.fsync(descriptor)
    finally:
        os.close(descriptor)


@contextmanager
def output_lock(path: Path):
    """Serialize local discovery runs that target one evidence file."""
    lock_path = path.with_name(f".{path.name}.lock")
    descriptor: int | None = None
    try:
        descriptor = os.open(lock_path, os.O_WRONLY | os.O_CREAT | getattr(os, "O_NOFOLLOW", 0), 0o600)
        os.fchmod(descriptor, 0o600)
        fcntl.flock(descriptor, fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError as exc:
        if descriptor is not None:
            os.close(descriptor)
        raise DiscoveryError("another discovery run is already updating this output path") from exc
    except OSError as exc:
        if descriptor is not None:
            os.close(descriptor)
        raise DiscoveryError("discovery report output lock could not be acquired safely") from exc
    try:
        yield
    finally:
        try:
            fcntl.flock(descriptor, fcntl.LOCK_UN)
        finally:
            os.close(descriptor)


def invalidate_output(path: Path) -> None:
    """Remove evidence before live probes so a failed run cannot look current."""
    try:
        path.unlink(missing_ok=True)
        fsync_parent(path)
    except OSError as exc:
        raise DiscoveryError("previous discovery report could not be invalidated safely") from exc


def atomic_write_report(path: Path, report: dict[str, Any]) -> None:
    """Publish only a complete, durable, non-secret discovery report."""
    temporary: Path | None = None
    published = False
    try:
        serialized = json.dumps(report, indent=2, sort_keys=True) + "\n"
        descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
        temporary = Path(temporary_name)
        with os.fdopen(descriptor, "w", encoding="utf-8") as file:
            os.fchmod(file.fileno(), 0o600)
            file.write(serialized)
            file.flush()
            os.fsync(file.fileno())
        os.replace(temporary, path)
        published = True
        fsync_parent(path)
    except (OSError, TypeError) as exc:
        if published:
            try:
                path.unlink(missing_ok=True)
                fsync_parent(path)
            except OSError:
                pass
        raise DiscoveryError("discovery report could not be published safely") from exc
    finally:
        if temporary is not None and temporary.exists():
            temporary.unlink()


def main() -> int:
    parser = argparse.ArgumentParser(description="Read-only, redacted EKS bootstrap discovery")
    parser.add_argument("--profile", required=True, help="validated AWS CLI profile for the discovery role")
    parser.add_argument("--account-id", required=True, help="approved 12-digit AWS account ID")
    parser.add_argument("--region", required=True)
    parser.add_argument("--cluster", required=True)
    parser.add_argument("--namespace", required=True)
    parser.add_argument(
        "--linkerd-namespace",
        help="optional Linkerd control-plane namespace; when set, require the policy CRDs and template API versions",
    )
    parser.add_argument("--ingress-namespace", help="ingress namespace required with Linkerd policy discovery")
    parser.add_argument("--ingress-service-account", help="ingress service account required with Linkerd policy discovery")
    parser.add_argument("--ingress-deployment", help="ingress Deployment required with Linkerd policy discovery")
    parser.add_argument("--linkerd-trust-domain", help="explicit Linkerd trust domain required with Linkerd policy discovery")
    parser.add_argument("--ecr-repository", required=True)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()

    if not args.account_id.isdigit() or len(args.account_id) != 12:
        parser.error("--account-id must be an explicit 12-digit account ID")
    linkerd_identity_inputs = (args.ingress_namespace, args.ingress_service_account, args.ingress_deployment, args.linkerd_trust_domain)
    if any(linkerd_identity_inputs) and not all(linkerd_identity_inputs):
        parser.error("--ingress-namespace, --ingress-service-account, --ingress-deployment, and --linkerd-trust-domain must be supplied together")
    if args.linkerd_namespace and not all(linkerd_identity_inputs):
        parser.error("Linkerd discovery requires the explicit ingress namespace, service account, deployment, and trust domain")
    if not args.linkerd_namespace and any(linkerd_identity_inputs):
        parser.error("ingress identity is valid only when Linkerd discovery is selected")
    if args.linkerd_namespace and (
        not DNS_LABEL.fullmatch(args.linkerd_namespace)
        or not DNS_LABEL.fullmatch(args.ingress_namespace)
        or not is_dns_subdomain(args.ingress_service_account)
        or not is_dns_subdomain(args.ingress_deployment)
        or not TRUST_DOMAIN.fullmatch(args.linkerd_trust_domain)
    ):
        parser.error("invalid Linkerd ingress namespace, service account, deployment, or trust domain")
    validate_output_path(args.output)

    with output_lock(args.output):
        # Invalidate prior evidence before the first AWS/Kubernetes probe. A
        # nonzero run therefore cannot leave a prior success reusable.
        invalidate_output(args.output)
        if shutil.which("aws") is None or shutil.which("kubectl") is None:
            raise DiscoveryError("aws and kubectl are both required")
        identity = aws_json(["sts", "get-caller-identity"], args.region, args.profile)
        actual_account, actual_arn = validate_discovery_identity(identity, args.account_id)

        cluster = aws_json(["eks", "describe-cluster", "--name", args.cluster], args.region, args.profile).get("cluster", {})
        if not cluster:
            raise DiscoveryError("explicit EKS cluster was not found")

        with tempfile.TemporaryDirectory(prefix="smartrouter-eks-discovery-") as temp_dir:
            kubeconfig = Path(temp_dir) / "kubeconfig"
            # Never read or update the operator's default kubeconfig/current-context.
            run([
                "aws", "--profile", args.profile, "eks", "update-kubeconfig", "--name", args.cluster, "--region", args.region,
                "--kubeconfig", str(kubeconfig), "--alias", "discovery-target",
            ])
            context = "discovery-target"
            namespace = kubectl_json(kubeconfig, ["--context", context, "get", "namespace", args.namespace])
            linkerd: dict[str, Any] = {
                "requested": bool(args.linkerd_namespace),
                "control_plane_namespace": args.linkerd_namespace,
                "control_plane_namespace_present": False,
                "policy_api_resources": [],
                "policy_crd_served_versions": {},
                "namespace_injection_labels": {k: v for k, v in namespace_labels(namespace).items() if "linkerd.io" in k},
                "namespace_injection_annotation": linkerd_injection_annotation(namespace),
                "identity_service_accounts": [],
                "ingress_namespace": args.ingress_namespace,
                "ingress_service_account": args.ingress_service_account,
                "ingress_deployment": args.ingress_deployment,
                "trust_domain": args.linkerd_trust_domain,
                "ingress_identity": "",
                "ingress_identity_verified": False,
                "ingress_workload_verified": False,
                "ingress_workload_mesh_ready": False,
                "trust_identity_config_payload_read": False,
                "trust_domain_source": "explicit operator selection; Linkerd trust configuration payload not read",
            }
            if args.linkerd_namespace:
                api_resources = run(["kubectl", "--kubeconfig", str(kubeconfig), "--context", context, "api-resources", "--api-group=policy.linkerd.io", "-o", "name"]).splitlines()
                required_linkerd_resources = {"servers.policy.linkerd.io", "serverauthorizations.policy.linkerd.io"}
                if not required_linkerd_resources.issubset(set(api_resources)):
                    raise DiscoveryError("requested Linkerd policy CRDs required for namespace bootstrap are unavailable or incompatible")
                linkerd_namespace = kubectl_json(kubeconfig, ["--context", context, "get", "namespace", args.linkerd_namespace])
                linkerd_service_accounts = kubectl_json(kubeconfig, ["--context", context, "-n", args.linkerd_namespace, "get", "serviceaccounts"])
                ingress_service_accounts = kubectl_json(kubeconfig, ["--context", context, "-n", args.ingress_namespace, "get", "serviceaccounts"])
                if args.ingress_service_account not in item_names(ingress_service_accounts):
                    raise DiscoveryError("selected ingress service account is unavailable for Linkerd policy identity")
                ingress_deployment = kubectl_json(kubeconfig, ["--context", context, "-n", args.ingress_namespace, "get", "deployment", args.ingress_deployment])
                ingress_replica_sets = kubectl_json(kubeconfig, ["--context", context, "-n", args.ingress_namespace, "get", "replicasets"])
                ingress_pods = kubectl_json(kubeconfig, ["--context", context, "-n", args.ingress_namespace, "get", "pods"])
                workload_evidence = ingress_workload_evidence(
                    ingress_deployment,
                    ingress_replica_sets,
                    ingress_pods,
                    args.ingress_deployment,
                    args.ingress_namespace,
                    args.ingress_service_account,
                    args.linkerd_namespace,
                    args.linkerd_trust_domain,
                )
                ingress_identity = linkerd_ingress_identity(
                    args.ingress_namespace,
                    args.ingress_service_account,
                    args.linkerd_namespace,
                    args.linkerd_trust_domain,
                )
                linkerd_crd_versions: dict[str, list[str]] = {}
                for crd in sorted(required_linkerd_resources):
                    crd_payload = kubectl_json(kubeconfig, ["--context", context, "get", "customresourcedefinition", crd])
                    linkerd_crd_versions[crd] = sorted(version.get("name", "") for version in crd_payload.get("spec", {}).get("versions", []) if version.get("served"))
                required_versions = {"servers.policy.linkerd.io": "v1beta3", "serverauthorizations.policy.linkerd.io": "v1beta1"}
                for crd, version in required_versions.items():
                    if version not in linkerd_crd_versions[crd]:
                        raise DiscoveryError(f"Linkerd {crd} does not serve required {version} API")
                linkerd.update({
                    "control_plane_namespace_present": bool(linkerd_namespace.get("metadata", {}).get("name")),
                    "policy_api_resources": sorted(api_resources),
                    "policy_crd_served_versions": linkerd_crd_versions,
                    "identity_service_accounts": item_names(linkerd_service_accounts),
                    "ingress_identity": ingress_identity,
                    "ingress_identity_verified": True,
                    **workload_evidence,
                })

            report: dict[str, Any] = {
                "schema_version": 1,
                "generated_at": datetime.now(timezone.utc).isoformat(),
                "intent": "read-only bootstrap discovery; no secret, endpoint, certificate, DSN, or policy payload values",
                "selection": {"account_id": args.account_id, "region": args.region, "cluster": args.cluster, "namespace": args.namespace, "ecr_repository": args.ecr_repository},
                "aws_identity": {"account_id": actual_account, "principal_type": actual_arn.split(":")[5].split("/")[0]},
                "eks": {
                    "version": cluster.get("version"), "platform_version": cluster.get("platformVersion"),
                    "authentication_mode": cluster.get("accessConfig", {}).get("authenticationMode"),
                    "endpoint_public_access": cluster.get("resourcesVpcConfig", {}).get("endpointPublicAccess"),
                    "endpoint_private_access": cluster.get("resourcesVpcConfig", {}).get("endpointPrivateAccess"),
                    "logging_types": sorted({log_type for log in cluster.get("logging", {}).get("clusterLogging", []) if log.get("enabled") for log_type in log.get("types", [])}),
                    "access_entry_principals": sorted(str(entry).split(":")[-1] for entry in aws_json(["eks", "list-access-entries", "--cluster-name", args.cluster], args.region, args.profile).get("accessEntries", [])),
                    "managed_nodegroups": sorted(aws_json(["eks", "list-nodegroups", "--cluster-name", args.cluster], args.region, args.profile).get("nodegroups", [])),
                    "auto_mode_capabilities": sorted(key for key, value in cluster.get("computeConfig", {}).items() if value is True),
                },
                "namespace": {"name": args.namespace, "labels": namespace_labels(namespace), "service_accounts": item_names(kubectl_json(kubeconfig, ["--context", context, "-n", args.namespace, "get", "serviceaccounts"])), "network_policies": item_names(kubectl_json(kubeconfig, ["--context", context, "-n", args.namespace, "get", "networkpolicies"])), "rds_connectivity_boundary": "review namespace NetworkPolicy names and approved private database boundary outside this report", "router_resources": {}},
                "cluster_resources": {"ingress_classes": item_names(kubectl_json(kubeconfig, ["--context", context, "get", "ingressclasses"])), "storage_classes": item_names(kubectl_json(kubeconfig, ["--context", context, "get", "storageclasses"]))},
                "linkerd": linkerd,
                "ecr": {},
                "evidence": {"secrets_read": False, "secret_data_read": False, "config_payloads_read": False, "drift_requires_review": True},
            }
            for kind, plural in (("deployments", "deployments"), ("services", "services"), ("persistent_volume_claims", "persistentvolumeclaims"), ("ingresses", "ingresses")):
                report["namespace"]["router_resources"][kind] = item_names(kubectl_json(kubeconfig, ["--context", context, "-n", args.namespace, "get", plural, "-l", "app.kubernetes.io/name=smart-llmrouter"]))

            repository = aws_json(["ecr", "describe-repositories", "--repository-names", args.ecr_repository], args.region, args.profile)["repositories"][0]
            report["ecr"] = {"image_tag_mutability": repository.get("imageTagMutability"), "encryption_type": repository.get("encryptionConfiguration", {}).get("encryptionType"), "repository_policy_present": ecr_policy_present(args.ecr_repository, args.region, args.profile), "image_scan_on_push": repository.get("imageScanningConfiguration", {}).get("scanOnPush"), "image_scan_visibility_checked": False}

        atomic_write_report(args.output, report)
    print(f"wrote sanitized discovery report: {args.output}")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except DiscoveryError as exc:
        print(f"EKS discovery failed safely: {exc}", file=sys.stderr)
        raise SystemExit(2)
