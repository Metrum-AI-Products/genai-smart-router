#!/usr/bin/env python3
"""Contract tests for scripts/eks_delivery.py using fake cloud executables."""
from __future__ import annotations

import importlib.util
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "eks_delivery.py"
SPEC = importlib.util.spec_from_file_location("eks_delivery", SCRIPT)
EKS_DELIVERY = importlib.util.module_from_spec(SPEC)
assert SPEC.loader
sys.modules[SPEC.name] = EKS_DELIVERY
SPEC.loader.exec_module(EKS_DELIVERY)
TARGET_POLICY = json.loads((ROOT / "deploy" / "aws" / "genai-smart-router-eks-staging-target.json").read_text())
DIGEST = str(TARGET_POLICY["ecr_repository_uri"]) + "@sha256:" + "a" * 64
DEPLOYMENT_UID = "smart-llmrouter-deployment-uid"


def target_value(value: dict[str, object] | None = None) -> str:
    return json.dumps(json.dumps(value or TARGET_POLICY, sort_keys=True, separators=(",", ":")))


def rendered_objects(namespace: str | None = None) -> str:
    namespace = namespace or str(TARGET_POLICY["k8s_namespace"])
    labels = {"app.kubernetes.io/name": "smart-llmrouter"}
    return json.dumps(
        {
            "apiVersion": "v1",
            "kind": "List",
            "items": [
                {
                    "apiVersion": "apps/v1",
                    "kind": "Deployment",
                    "metadata": {
                        "name": "smart-llmrouter",
                        "namespace": namespace,
                        "labels": labels,
                    },
                    "spec": {
                        "template": {
                            "spec": {
                                "containers": [
                                    {
                                        "name": "router",
                                        "image": DIGEST,
                                        "ports": [
                                            {"name": "http", "containerPort": 8080}
                                        ],
                                        "readinessProbe": {
                                            "httpGet": {"path": "/readyz", "port": "http"},
                                            "periodSeconds": 10,
                                            "timeoutSeconds": 3,
                                            "failureThreshold": 3,
                                        },
                                        "livenessProbe": {
                                            "httpGet": {"path": "/healthz", "port": "http"},
                                            "periodSeconds": 30,
                                            "timeoutSeconds": 3,
                                            "failureThreshold": 3,
                                        },
                                        "startupProbe": {
                                            "httpGet": {"path": "/readyz", "port": "http"},
                                            "periodSeconds": 5,
                                            "timeoutSeconds": 3,
                                            "failureThreshold": 24,
                                        },
                                    }
                                ]
                            }
                        }
                    },
                },
                {
                    "apiVersion": "v1",
                    "kind": "Service",
                    "metadata": {
                        "name": "smart-llmrouter",
                        "namespace": namespace,
                        "labels": labels,
                    },
                    "spec": {
                        "ports": [{"port": 80, "targetPort": "http"}],
                        "selector": {"app.kubernetes.io/name": "smart-llmrouter"},
                        "type": "ClusterIP",
                    },
                },
            ],
        }
    )


def inventory_with_stale_resource() -> str:
    payload = json.loads(rendered_objects())
    payload["items"].append(
        {
            "apiVersion": "networking.k8s.io/v1",
            "kind": "Ingress",
            "metadata": {
                "name": "stale-router-ingress",
                "namespace": TARGET_POLICY["k8s_namespace"],
                "labels": {"app.kubernetes.io/name": "smart-llmrouter"},
            },
        }
    )
    return json.dumps(payload)


def inventory_with_service_configuration_drift() -> str:
    payload = json.loads(rendered_objects())
    payload["items"][1]["spec"] = {
        "ports": [{"port": 443, "targetPort": "https"}],
        "selector": {"app.kubernetes.io/name": "different-workload"},
        "type": "ClusterIP",
    }
    return json.dumps(payload)


def inventory_with_service_runtime_fields() -> str:
    payload = json.loads(rendered_objects())
    payload["items"][1]["spec"].update(
        {
            "clusterIP": "10.100.0.42",
            "clusterIPs": ["10.100.0.42"],
        }
    )
    payload["items"][1]["metadata"].update(
        {
            "resourceVersion": "12345",
            "uid": "runtime-uid",
            "annotations": {
                "kubectl.kubernetes.io/last-applied-configuration": "runtime-only"
            },
        }
    )
    return json.dumps(payload)


def inventory_with_server_defaulted_service() -> str:
    payload = json.loads(rendered_objects())
    payload["items"][1]["spec"].update(
        {
            "clusterIP": "10.100.0.42",
            "clusterIPs": ["10.100.0.42"],
            "internalTrafficPolicy": "Cluster",
            "ipFamilies": ["IPv4"],
            "ipFamilyPolicy": "SingleStack",
            "ports": [{"port": 80, "protocol": "TCP", "targetPort": "http"}],
            "sessionAffinity": "None",
        }
    )
    return json.dumps(payload)


def inventory_with_server_defaulted_deployment() -> str:
    """Return the API-defaulted nested fields omitted by the checked-in base."""

    payload = json.loads(rendered_objects())
    container = payload["items"][0]["spec"]["template"]["spec"]["containers"][0]
    container["ports"][0]["protocol"] = "TCP"
    for probe_name in ("readinessProbe", "livenessProbe", "startupProbe"):
        probe = container[probe_name]
        probe["httpGet"]["scheme"] = "HTTP"
        probe["successThreshold"] = 1
    return json.dumps(payload)


def rendered_objects_with_ingress() -> str:
    payload = json.loads(rendered_objects())
    payload["items"].append(
        {
            "apiVersion": "networking.k8s.io/v1",
            "kind": "Ingress",
            "metadata": {
                "name": "smart-llmrouter",
                "namespace": TARGET_POLICY["k8s_namespace"],
                "labels": {"app.kubernetes.io/name": "smart-llmrouter"},
            },
            "spec": {
                "rules": [
                    {
                        "host": "router.example.test",
                        "http": {
                            "paths": [
                                {
                                    "backend": {
                                        "service": {
                                            "name": "smart-llmrouter",
                                            "port": {"number": 80},
                                        }
                                    },
                                    "path": "/",
                                    "pathType": "Prefix",
                                }
                            ]
                        },
                    }
                ]
            },
        }
    )
    return json.dumps(payload)


def inventory_with_foreign_manager_ingress_annotation() -> str:
    payload = json.loads(rendered_objects_with_ingress())
    ingress = next(item for item in payload["items"] if item["kind"] == "Ingress")
    ingress["metadata"]["annotations"] = {
        "external.example.test/managed": "unreviewed-live-drift"
    }
    return json.dumps(payload)


def inventory_with_foreign_manager_ingress_label() -> str:
    payload = json.loads(rendered_objects_with_ingress())
    ingress = next(item for item in payload["items"] if item["kind"] == "Ingress")
    ingress["metadata"]["labels"]["external.example.test/managed"] = "unreviewed-live-drift"
    return json.dumps(payload)


def all_managed_configuration_objects() -> list[dict[str, object]]:
    namespace = str(TARGET_POLICY["k8s_namespace"])
    labels = {"app.kubernetes.io/name": "smart-llmrouter"}

    def resource(kind: str, name: str, **fields: object) -> dict[str, object]:
        return {
            "apiVersion": "v1",
            "kind": kind,
            "metadata": {"name": name, "namespace": namespace, "labels": labels},
            **fields,
        }

    return [
        resource(
            "Deployment",
            "smart-llmrouter",
            spec={"replicas": 1, "template": {"spec": {"containers": []}}},
        ),
        resource(
            "Service",
            "smart-llmrouter",
            spec={
                "healthCheckNodePort": 32456,
                "ipFamilies": ["IPv4"],
                "ipFamilyPolicy": "SingleStack",
                "ports": [{"port": 80, "targetPort": "http"}],
                "selector": labels,
            },
        ),
        resource(
            "Ingress",
            "smart-llmrouter",
            spec={"rules": [{"host": "router.example.test"}]},
        ),
        resource(
            "NetworkPolicy",
            "smart-llmrouter-restrict",
            spec={"egress": [{"ports": [{"port": 443}]}], "policyTypes": ["Egress"]},
        ),
        resource(
            "PersistentVolumeClaim",
            "smart-llmrouter-state",
            spec={"accessModes": ["ReadWriteOnce"], "resources": {"requests": {"storage": "5Gi"}}},
        ),
        resource(
            "PodDisruptionBudget",
            "smart-llmrouter",
            spec={"minAvailable": 1, "selector": {"matchLabels": labels}},
        ),
        resource("ServiceAccount", "smart-llmrouter", automountServiceAccountToken=False),
    ]


def rendered_manifest(configuration_marker: str = "configuration-one") -> str:
    return f"image: {DIGEST}\nsafe-rendered-configuration: {configuration_marker}\n" + "x" * 5000


def deployment_object(
    digest: str = DIGEST,
    template_marker: str = "template-one",
    generation: int = 1,
    observed_generation: int | None = None,
    revision: int = 2,
) -> str:
    return json.dumps(
        {
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": "smart-llmrouter",
                "namespace": TARGET_POLICY["k8s_namespace"],
                "generation": generation,
                "uid": DEPLOYMENT_UID,
                "annotations": {"deployment.kubernetes.io/revision": str(revision)},
            },
            "status": {"observedGeneration": observed_generation if observed_generation is not None else generation},
            "spec": {
                "template": {
                    "metadata": {"annotations": {"example.metrum.ai/config-revision": template_marker}},
                    "spec": {"containers": [{"name": "router", "image": digest}]},
                }
            },
        }
    )


def replica_sets_object(
    digest: str = DIGEST,
    revision: int = 1,
    owner_name: str = "smart-llmrouter",
    owner_uid: str = DEPLOYMENT_UID,
) -> str:
    return json.dumps(
        {
            "apiVersion": "v1",
            "kind": "List",
            "items": [
                {
                    "apiVersion": "apps/v1",
                    "kind": "ReplicaSet",
                    "metadata": {
                        "name": "smart-llmrouter-rollback-candidate",
                        "namespace": TARGET_POLICY["k8s_namespace"],
                        "annotations": {"deployment.kubernetes.io/revision": str(revision)},
                        "ownerReferences": [
                            {
                                "apiVersion": "apps/v1",
                                "kind": "Deployment",
                                "name": owner_name,
                                "uid": owner_uid,
                                "controller": True,
                            }
                        ],
                    },
                    "spec": {
                        "template": {
                            "spec": {
                                "containers": [{"name": "router", "image": digest}]
                            }
                        }
                    },
                }
            ],
        }
    )


def fake_tools(directory: Path) -> None:
    log = directory / "calls.log"
    for name in ("aws", "kubectl", "kustomize"):
        body = f'''#!/bin/sh
echo "$0 $* KUBECONFIG=$KUBECONFIG" >> "{log}"
case "$0" in
  *aws) case "$*" in
    *"ssm get-parameter"*) printf '{{"Parameter":{{"Type":"String","Value":%s}}}}\\n' "$FAKE_TARGET_POLICY_VALUE" ;;
    *get-caller-identity*) printf '{{"Account":"%s","Arn":"arn:aws:sts::%s:assumed-role/%s/test-session"}}\\n' "$FAKE_AWS_ACCOUNT" "$FAKE_AWS_ACCOUNT" "$FAKE_AWS_ROLE" ;;
    *describe-cluster*) printf '{{"cluster":{{"status":"ACTIVE","arn":"arn:aws:eks:%s:%s:cluster/%s"}}}}\\n' "$FAKE_AWS_REGION" "$FAKE_AWS_ACCOUNT" "$FAKE_EKS_CLUSTER" ;;
  esac ;;
  *kubectl) case "$*" in
    *"auth can-i"*) echo yes ;;
    *"apply --dry-run=client"*) printf '%s\\n' "$FAKE_RENDER_OBJECTS" ;;
    *"--dry-run=server"*" -o json"*) previous=''; for arg; do if [ "$previous" = '-f' ]; then manifest="$arg"; break; fi; previous="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}"; printf '%s\\n' "$FAKE_SERVER_NORMALIZED_OBJECTS" ;;
    *"apply --server-side --dry-run=server"*) previous=''; for arg; do if [ "$previous" = '-f' ]; then manifest="$arg"; break; fi; previous="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}" ;;
    *"get deployments,ingresses,networkpolicies,persistentvolumeclaims,poddisruptionbudgets,services,serviceaccounts"*) cat "$FAKE_LIVE_INVENTORY_FILE" ;;
    *"get replicasets"*) printf '%s\\n' "$FAKE_REPLICA_SETS" ;;
    *"get deployment/smart-llmrouter"*) printf '%s\\n' "$FAKE_DEPLOYMENT_OBJECT" ;;
    *"rollout status"*) : ;;
    *apply*) for arg; do manifest="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}"; printf '%s\\n' "$FAKE_APPLIED_LIVE_MANAGED_OBJECTS" > "$FAKE_LIVE_INVENTORY_FILE" ;;
  esac ;;
  *kustomize) case "$*" in
    *build*) printf '%s\\n' "$FAKE_RENDERED_MANIFEST" ;;
  esac ;;
esac
'''
        target = directory / name
        target.write_text(body)
        target.chmod(0o755)


def protected_smoke_script(root: Path, name: str, body: str, mode: int = 0o600) -> Path:
    """Write a mode-controlled shell source without placing it in CLI argv."""

    script = root / name
    script.write_text(body, encoding="utf-8")
    script.chmod(mode)
    return script


def run(
    action: str,
    root: Path,
    extra: list[str] | None = None,
    *,
    include_digest: bool = True,
    image_digest: str = DIGEST,
    account: str | None = None,
    policy: dict[str, object] | None = None,
    render_objects: str | None = None,
    server_normalized_inventory: str | None = None,
    live_inventory: str | None = None,
    live_inventory_after_apply: str | None = None,
    rendered_manifest_value: str | None = None,
    deployed_digest: str = DIGEST,
    deployed_template_marker: str = "template-one",
    deployed_generation: int = 1,
    deployed_observed_generation: int | None = None,
    deployed_revision: int = 2,
    replica_sets: str | None = None,
    confirm_smoke: bool = True,
) -> subprocess.CompletedProcess[str]:
    evidence = root / "tmp" / "evidence"
    command = [
        "python3",
        str(SCRIPT),
        action,
        "--aws-profile",
        "test-profile",
        "--evidence-dir",
        str(evidence),
    ]
    if include_digest:
        command += ["--image-digest", image_digest]
    if action == "smoke" and confirm_smoke:
        command += ["--confirm", "STAGING_APPLY"]
    if extra:
        command += extra
    bindir = root / "fake-bin"
    live_inventory_file = root / "tmp" / "live-managed-inventory.json"
    live_inventory_file.parent.mkdir(parents=True, exist_ok=True)
    live_inventory_file.write_text(live_inventory or rendered_objects())
    env = {
        **os.environ,
        "PATH": f"{bindir}:{os.environ['PATH']}",
        "KUBECONFIG": str(root / "must-not-be-used"),
        "FAKE_AWS_ACCOUNT": account or str(TARGET_POLICY["aws_account_id"]),
        "FAKE_AWS_REGION": str(TARGET_POLICY["aws_region"]),
        "FAKE_AWS_ROLE": str(TARGET_POLICY["delivery_role_name"]),
        "FAKE_EKS_CLUSTER": str(TARGET_POLICY["eks_cluster"]),
        "FAKE_TARGET_POLICY_VALUE": target_value(policy),
        "FAKE_RENDER_OBJECTS": render_objects or rendered_objects(),
        "FAKE_SERVER_NORMALIZED_OBJECTS": server_normalized_inventory or render_objects or rendered_objects(),
        "FAKE_LIVE_INVENTORY_FILE": str(live_inventory_file),
        "FAKE_APPLIED_LIVE_MANAGED_OBJECTS": live_inventory_after_apply
        or live_inventory
        or rendered_objects(),
        "FAKE_RENDERED_MANIFEST": rendered_manifest_value or rendered_manifest(),
        "FAKE_DEPLOYMENT_OBJECT": deployment_object(
            deployed_digest,
            deployed_template_marker,
            deployed_generation,
            deployed_observed_generation,
            deployed_revision,
        ),
        "FAKE_REPLICA_SETS": replica_sets or replica_sets_object(),
    }
    return subprocess.run(command, cwd=root, text=True, capture_output=True, env=env)


def main() -> int:
    scrubbed = EKS_DELIVERY.scrub(
        '{"Authorization":"Bearer abc123","X-Api-Key":"key456","AWS_SECRET_ACCESS_KEY":"secret789"}'
    )
    for leaked in ("abc123", "key456", "secret789"):
        assert leaked not in scrubbed
    assert "abc123" not in EKS_DELIVERY.scrub("Authorization: Bearer abc123")

    target = EKS_DELIVERY.parse_target_policy(TARGET_POLICY)
    expected_resources = all_managed_configuration_objects()
    expected_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
        expected_resources, target, source="configuration fingerprint fixture"
    )
    expected_identities = EKS_DELIVERY.managed_resource_identities(expected_inventory)
    expected_spec_fingerprint = EKS_DELIVERY.managed_resource_spec_fingerprint(expected_inventory)
    for kind, mutate in (
        ("Service", lambda resource: resource["spec"].update({"selector": {"unexpected": "true"}})),
        ("Ingress", lambda resource: resource["spec"].update({"rules": [{"host": "wrong.example.test"}]})),
        ("NetworkPolicy", lambda resource: resource["spec"].update({"egress": [{"to": [{"ipBlock": {"cidr": "0.0.0.0/0"}}]}]})),
        ("PersistentVolumeClaim", lambda resource: resource["spec"].update({"accessModes": ["ReadWriteMany"]})),
        ("PodDisruptionBudget", lambda resource: resource["spec"].update({"minAvailable": 0})),
        ("ServiceAccount", lambda resource: resource.update({"automountServiceAccountToken": True})),
    ):
        changed_resources = json.loads(json.dumps(expected_resources))
        changed = next(item for item in changed_resources if item["kind"] == kind)
        mutate(changed)
        changed_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
            changed_resources, target, source="configuration drift fixture"
        )
        assert EKS_DELIVERY.managed_resource_identities(changed_inventory) == expected_identities
        assert (
            EKS_DELIVERY.managed_resource_spec_fingerprint(changed_inventory)
            != expected_spec_fingerprint
        )

    for field, value in (
        ("healthCheckNodePort", 32457),
        ("ipFamilies", ["IPv6"]),
        ("ipFamilyPolicy", "RequireDualStack"),
    ):
        changed_resources = json.loads(json.dumps(expected_resources))
        service = next(item for item in changed_resources if item["kind"] == "Service")
        service["spec"][field] = value
        changed_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
            changed_resources, target, source="Service networking configuration drift fixture"
        )
        assert (
            EKS_DELIVERY.managed_resource_spec_fingerprint(changed_inventory)
            != expected_spec_fingerprint
        )

    runtime_only_resources = json.loads(json.dumps(expected_resources))
    runtime_service = next(item for item in runtime_only_resources if item["kind"] == "Service")
    runtime_service["metadata"].update(
        {
            "resourceVersion": "12345",
            "uid": "runtime-uid",
            "annotations": {
                "kubectl.kubernetes.io/last-applied-configuration": "runtime-only"
            },
        }
    )
    runtime_service["spec"].update(
        {
            "clusterIP": "10.100.0.42",
            "clusterIPs": ["10.100.0.42"],
        }
    )
    runtime_pvc = next(item for item in runtime_only_resources if item["kind"] == "PersistentVolumeClaim")
    runtime_pvc["spec"]["volumeName"] = "pvc-runtime-volume"
    runtime_service_account = next(item for item in runtime_only_resources if item["kind"] == "ServiceAccount")
    runtime_service_account["secrets"] = [{"name": "runtime-token-secret"}]
    runtime_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
        runtime_only_resources, target, source="runtime normalization fixture"
    )
    assert EKS_DELIVERY.managed_resource_identities(runtime_inventory) == expected_identities
    assert (
        EKS_DELIVERY.managed_resource_spec_fingerprint(runtime_inventory)
        == expected_spec_fingerprint
    )

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        bindir = root / "fake-bin"
        bindir.mkdir()
        fake_tools(bindir)

        assert run("preflight", root).returncode == 0
        result = run("plan", root)
        assert result.returncode == 0, result.stderr
        calls = (bindir / "calls.log").read_text()
        assert "must-not-be-used" not in calls and "update-kubeconfig" in calls and "--dry-run=server" in calls
        assert calls.index("ssm get-parameter") < calls.index("describe-cluster")
        manifest_size = re.search(r"manifest-bytes=\s*(\d+)", calls)
        assert manifest_size and int(manifest_size.group(1)) > 4000

        # Server-side dry-run/live JSON includes API-defaulted nested container
        # and probe fields that are absent from the reviewed manifest. Exact
        # defaults normalize; non-default values remain fail-closed.
        server_defaulted_deployment = inventory_with_server_defaulted_deployment()
        nested_default_plan = run(
            "plan", root, server_normalized_inventory=server_defaulted_deployment
        )
        assert nested_default_plan.returncode == 0, nested_default_plan.stderr
        nondefault_nested_deployment = json.loads(server_defaulted_deployment)
        nondefault_nested_deployment["items"][0]["spec"]["template"]["spec"]["containers"][0][
            "readinessProbe"
        ]["httpGet"]["scheme"] = "HTTPS"
        nested_drift_plan = run(
            "plan", root, server_normalized_inventory=json.dumps(nondefault_nested_deployment)
        )
        assert (
            nested_drift_plan.returncode != 0
            and "server-side manifest validation" in nested_drift_plan.stderr
        )

        before_unapproved_image = (bindir / "calls.log").read_text()
        unapproved_image = (
            "121701826775.dkr.ecr.us-east-1.amazonaws.com/other-repository@sha256:"
            + "b" * 64
        )
        rejected_image = run("plan", root, image_digest=unapproved_image)
        assert (
            rejected_image.returncode != 0
            and "approved staging ECR repository" in rejected_image.stderr
        )
        rejected_image_calls = (bindir / "calls.log").read_text()[len(before_unapproved_image):]
        assert "describe-cluster" not in rejected_image_calls
        assert "update-kubeconfig" not in rejected_image_calls
        assert "kustomize" not in rejected_image_calls

        before_unapproved_rollback = (bindir / "calls.log").read_text()
        rejected_rollback = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            image_digest=unapproved_image,
        )
        assert (
            rejected_rollback.returncode != 0
            and "approved staging ECR repository" in rejected_rollback.stderr
        )
        rejected_rollback_calls = (bindir / "calls.log").read_text()[
            len(before_unapproved_rollback):
        ]
        assert "describe-cluster" not in rejected_rollback_calls
        assert "rollout undo" not in rejected_rollback_calls

        no_digest_rollback = run(
            "rollback", root, ["--confirm", "STAGING_APPLY"], include_digest=False
        )
        assert no_digest_rollback.returncode != 0 and "IMAGE_DIGEST" in no_digest_rollback.stderr

        before_unmatched_rollback = (bindir / "calls.log").read_text()
        unmatched_rollback = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            replica_sets=replica_sets_object(
                digest=str(TARGET_POLICY["ecr_repository_uri"]) + "@sha256:" + "c" * 64
            ),
        )
        assert (
            unmatched_rollback.returncode != 0
            and "no prior Deployment revision" in unmatched_rollback.stderr
        )
        unmatched_rollback_calls = (bindir / "calls.log").read_text()[
            len(before_unmatched_rollback):
        ]
        assert "rollout undo" not in unmatched_rollback_calls

        before_wrong_owner_rollback = (bindir / "calls.log").read_text()
        wrong_owner_rollback = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            replica_sets=replica_sets_object(owner_uid="stale-deployment-uid"),
        )
        assert (
            wrong_owner_rollback.returncode != 0
            and "no prior Deployment revision" in wrong_owner_rollback.stderr
        )
        wrong_owner_rollback_calls = (bindir / "calls.log").read_text()[
            len(before_wrong_owner_rollback):
        ]
        assert "rollout undo" not in wrong_owner_rollback_calls

        unexpected_rollback = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            deployed_digest=str(TARGET_POLICY["ecr_repository_uri"]) + "@sha256:" + "b" * 64,
        )
        assert unexpected_rollback.returncode != 0 and "does not use" in unexpected_rollback.stderr

        before_rollback = (bindir / "calls.log").read_text()
        rollback = run("rollback", root, ["--confirm", "STAGING_APPLY"])
        assert rollback.returncode == 0, rollback.stderr
        rollback_calls = (bindir / "calls.log").read_text()[len(before_rollback):]
        assert "rollout undo deployment/smart-llmrouter" in rollback_calls
        assert "--to-revision 1" in rollback_calls
        rollback_evidence = json.loads((root / "tmp/evidence/evidence-rollback.json").read_text())
        assert rollback_evidence["outcome"] == "passed"
        assert rollback_evidence["image_digest"] == DIGEST
        assert rollback_evidence["rollback_target_revision"] == 1
        assert rollback_evidence["live_deployment_generation"] == 1
        assert any(
            event["name"] == "live_deployment" and event["phase"] == "after_rollback"
            for event in rollback_evidence["events"]
        )

        before_wrong_account = (bindir / "calls.log").read_text()
        wrong_account = run("preflight", root, account="999999999999")
        assert wrong_account.returncode != 0 and "approved staging account" in wrong_account.stderr
        assert "describe-cluster" not in (bindir / "calls.log").read_text()[len(before_wrong_account):]

        policy_drift = dict(TARGET_POLICY)
        policy_drift["eks_cluster"] = "unapproved-cluster"
        drift = run("preflight", root, policy=policy_drift)
        assert drift.returncode != 0 and "does not match" in drift.stderr

        malformed_ecr_policy = dict(TARGET_POLICY)
        malformed_ecr_policy["ecr_repository_uri"] = (
            "121701826775.dkr.ecr.us-west-2.amazonaws.com/smart-llmrouter"
        )
        malformed_ecr = run("preflight", root, policy=malformed_ecr_policy)
        assert malformed_ecr.returncode != 0 and "ECR repository" in malformed_ecr.stderr

        wrong_namespace = run("plan", root, render_objects=rendered_objects("another-namespace"))
        assert wrong_namespace.returncode != 0 and "outside the approved namespace" in wrong_namespace.stderr

        unlabeled_objects = json.loads(rendered_objects())
        del unlabeled_objects["items"][1]["metadata"]["labels"]
        unlabeled = run("plan", root, render_objects=json.dumps(unlabeled_objects))
        assert unlabeled.returncode != 0 and "managed-resource label" in unlabeled.stderr

        assert run("apply", root).returncode != 0
        before_stale_apply = (bindir / "calls.log").read_text()
        stale_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            live_inventory=inventory_with_stale_resource(),
        )
        assert stale_apply.returncode != 0 and "stale resources" in stale_apply.stderr
        stale_apply_calls = (bindir / "calls.log").read_text()[len(before_stale_apply):]
        assert "apply --server-side -f" not in stale_apply_calls
        assert "--prune" not in stale_apply_calls

        drift_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            live_inventory=inventory_with_service_configuration_drift(),
        )
        assert drift_apply.returncode != 0 and "configuration" in drift_apply.stderr

        reconciled_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            live_inventory=inventory_with_service_configuration_drift(),
            live_inventory_after_apply=rendered_objects(),
        )
        assert reconciled_apply.returncode == 0, reconciled_apply.stderr
        reconciled_evidence = json.loads((root / "tmp/evidence/evidence-apply.json").read_text())
        before_apply_event = next(
            event
            for event in reconciled_evidence["events"]
            if event["name"] == "managed_inventory" and event["phase"] == "before_apply"
        )
        after_apply_event = next(
            event
            for event in reconciled_evidence["events"]
            if event["name"] == "managed_inventory" and event["phase"] == "after_apply"
        )
        assert before_apply_event["expected_spec_sha256"] != before_apply_event["live_spec_sha256"]
        assert after_apply_event["expected_spec_sha256"] == after_apply_event["live_spec_sha256"]

        server_defaulted_inventory = inventory_with_server_defaulted_service()
        server_defaulted_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            live_inventory=server_defaulted_inventory,
            live_inventory_after_apply=server_defaulted_inventory,
        )
        assert server_defaulted_apply.returncode == 0, server_defaulted_apply.stderr
        applied = run("apply", root, ["--confirm", "STAGING_APPLY"])
        assert applied.returncode == 0, applied.stderr

        promotion = run("promotion-plan", root)
        assert promotion.returncode != 0 and "smoke evidence" in promotion.stderr
        no_digest = run("promotion-plan", root, include_digest=False)
        assert no_digest.returncode != 0 and "IMAGE_DIGEST" in no_digest.stderr

        passing_smoke_script = protected_smoke_script(root, "passing-smoke.sh", "exit 0\n")
        assert passing_smoke_script.stat().st_mode & 0o777 == 0o600

        unconfirmed_smoke_marker = root / "unconfirmed-smoke-command-ran"
        unconfirmed_smoke_script = protected_smoke_script(
            root, "unconfirmed-smoke.sh", f"touch '{unconfirmed_smoke_marker}'\n"
        )
        before_unconfirmed_smoke = (bindir / "calls.log").read_text()
        unconfirmed_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(unconfirmed_smoke_script)],
            confirm_smoke=False,
        )
        assert (
            unconfirmed_smoke.returncode != 0
            and "EKS_CONFIRM=STAGING_APPLY" in unconfirmed_smoke.stderr
            and not unconfirmed_smoke_marker.exists()
        )
        unconfirmed_smoke_calls = (bindir / "calls.log").read_text()[
            len(before_unconfirmed_smoke):
        ]
        assert "describe-cluster" not in unconfirmed_smoke_calls

        server_defaulted_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(passing_smoke_script)],
            server_normalized_inventory=server_defaulted_inventory,
            live_inventory=server_defaulted_inventory,
        )
        assert server_defaulted_smoke.returncode == 0, server_defaulted_smoke.stderr
        client_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
            json.loads(rendered_objects())["items"],
            target,
            source="client baseline fixture",
        )
        server_defaulted_smoke_evidence = json.loads(
            (root / "tmp/evidence/evidence-smoke.json").read_text()
        )
        assert (
            server_defaulted_smoke_evidence["managed_resource_spec_sha256"]
            == EKS_DELIVERY.managed_resource_spec_fingerprint(client_inventory)
        )

        ingress_rendered = rendered_objects_with_ingress()
        foreign_manager_ingress = inventory_with_foreign_manager_ingress_annotation()
        foreign_manager_dry_run_marker = root / "foreign-manager-dry-run-script-ran"
        foreign_manager_dry_run_script = protected_smoke_script(
            root,
            "foreign-manager-dry-run.sh",
            f"touch '{foreign_manager_dry_run_marker}'\n",
        )
        foreign_manager_dry_run = run(
            "smoke",
            root,
            ["--smoke-command-file", str(foreign_manager_dry_run_script)],
            render_objects=ingress_rendered,
            server_normalized_inventory=foreign_manager_ingress,
            live_inventory=ingress_rendered,
        )
        assert (
            foreign_manager_dry_run.returncode != 0
            and "server-side manifest validation" in foreign_manager_dry_run.stderr
        )
        assert not foreign_manager_dry_run_marker.exists()

        foreign_manager_marker = root / "foreign-manager-smoke-command-ran"
        foreign_manager_script = protected_smoke_script(
            root, "foreign-manager-smoke.sh", f"touch '{foreign_manager_marker}'\n"
        )
        foreign_manager_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(foreign_manager_script)],
            render_objects=ingress_rendered,
            server_normalized_inventory=ingress_rendered,
            live_inventory=foreign_manager_ingress,
        )
        assert (
            foreign_manager_smoke.returncode != 0
            and "configuration" in foreign_manager_smoke.stderr
        )
        assert not foreign_manager_marker.exists()

        foreign_manager_label_marker = root / "foreign-manager-label-smoke-command-ran"
        foreign_manager_label_script = protected_smoke_script(
            root,
            "foreign-manager-label-smoke.sh",
            f"touch '{foreign_manager_label_marker}'\n",
        )
        foreign_manager_label_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(foreign_manager_label_script)],
            render_objects=ingress_rendered,
            server_normalized_inventory=ingress_rendered,
            live_inventory=inventory_with_foreign_manager_ingress_label(),
        )
        assert (
            foreign_manager_label_smoke.returncode != 0
            and "configuration" in foreign_manager_label_smoke.stderr
        )
        assert not foreign_manager_label_marker.exists()

        before_foreign_manager_apply = (bindir / "calls.log").read_text()
        foreign_manager_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            render_objects=ingress_rendered,
            server_normalized_inventory=ingress_rendered,
            live_inventory=foreign_manager_ingress,
        )
        assert (
            foreign_manager_apply.returncode != 0
            and "fields absent from the reviewed" in foreign_manager_apply.stderr
        )
        foreign_manager_apply_calls = (bindir / "calls.log").read_text()[
            len(before_foreign_manager_apply):
        ]
        assert "apply --server-side -f" not in foreign_manager_apply_calls
        reapplied_after_foreign_manager_rejection = run(
            "apply", root, ["--confirm", "STAGING_APPLY"]
        )
        assert (
            reapplied_after_foreign_manager_rejection.returncode == 0
        ), reapplied_after_foreign_manager_rejection.stderr

        stale_smoke_marker = root / "stale-smoke-command-ran"
        stale_smoke_script = protected_smoke_script(
            root, "stale-smoke.sh", f"touch '{stale_smoke_marker}'\n"
        )
        stale_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(stale_smoke_script)],
            live_inventory=inventory_with_stale_resource(),
        )
        assert stale_smoke.returncode != 0 and "managed-resource inventory" in stale_smoke.stderr
        assert not stale_smoke_marker.exists()

        failing_smoke_script = protected_smoke_script(
            root,
            "failing-smoke.sh",
            "printf '%s\\n' 'Authorization: Bearer abc123 X-Api-Key=key456 AWS_SECRET_ACCESS_KEY=secret789' >&2\nexit 1\n",
        )
        smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(failing_smoke_script)],
        )
        assert smoke.returncode != 0
        for leaked in ("abc123", "key456", "secret789"):
            assert leaked not in str(smoke.args)
            assert leaked not in smoke.stderr and leaked not in (root / "tmp/evidence/evidence.json").read_text()

        insecure_smoke_script = protected_smoke_script(
            root, "insecure-smoke.sh", "exit 0\n", mode=0o644
        )
        insecure_smoke = run(
            "smoke", root, ["--smoke-command-file", str(insecure_smoke_script)]
        )
        assert insecure_smoke.returncode != 0 and "mode 0600" in insecure_smoke.stderr

        # Bind the checked file descriptor before execution.  Replacing the
        # path after validation must not substitute a different smoke script.
        descriptor_smoke_script = protected_smoke_script(
            root, "descriptor-smoke.sh", "exit 0\n"
        )
        descriptor = EKS_DELIVERY.open_protected_smoke_command_file(
            str(descriptor_smoke_script)
        )
        try:
            replacement = protected_smoke_script(
                root, "descriptor-smoke-replacement.sh", "exit 37\n"
            )
            replacement.replace(descriptor_smoke_script)
            descriptor_smoke = subprocess.run(
                ["/bin/sh", "-s"],
                stdin=descriptor,
                text=True,
                capture_output=True,
            )
            assert descriptor_smoke.returncode == 0, descriptor_smoke.stderr
        finally:
            os.close(descriptor)

        marker = root / "smoke-command-ran"
        marker_smoke_script = protected_smoke_script(
            root, "marker-smoke.sh", f"touch '{marker}'\n"
        )
        stale = run(
            "smoke",
            root,
            ["--smoke-command-file", str(marker_smoke_script)],
            deployed_digest="registry.example/router@sha256:" + "b" * 64,
        )
        assert stale.returncode != 0 and "does not use" in stale.stderr and not marker.exists()

        not_observed = run(
            "smoke",
            root,
            ["--smoke-command-file", str(marker_smoke_script)],
            deployed_observed_generation=0,
        )
        assert not_observed.returncode != 0 and "not fully observed" in not_observed.stderr and not marker.exists()

        runtime_only_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(passing_smoke_script)],
            live_inventory=inventory_with_service_runtime_fields(),
        )
        assert runtime_only_smoke.returncode == 0, runtime_only_smoke.stderr

        configuration_marker = root / "configuration-drift-smoke-command-ran"
        configuration_drift_smoke = run(
            "smoke",
            root,
            [
                "--smoke-command-file",
                str(
                    protected_smoke_script(
                        root,
                        "configuration-drift-smoke.sh",
                        f"touch '{configuration_marker}'\n",
                    )
                ),
            ],
            live_inventory=inventory_with_service_configuration_drift(),
        )
        assert (
            configuration_drift_smoke.returncode != 0
            and "configuration" in configuration_drift_smoke.stderr
        )
        assert not configuration_marker.exists()

        smoke_passed = run(
            "smoke", root, ["--smoke-command-file", str(passing_smoke_script)]
        )
        assert smoke_passed.returncode == 0, smoke_passed.stderr

        # A later apply changes the accepted evidence record even when its
        # resource fingerprints are identical. Promotion must reject the
        # earlier smoke until a new smoke binds to that latest apply.
        apply_after_smoke = run("apply", root, ["--confirm", "STAGING_APPLY"])
        assert apply_after_smoke.returncode == 0, apply_after_smoke.stderr
        stale_smoke_evidence = run("promotion-plan", root)
        assert (
            stale_smoke_evidence.returncode != 0
            and (
                "smoke evidence must postdate" in stale_smoke_evidence.stderr
                or "not bound to the current accepted apply" in stale_smoke_evidence.stderr
            )
        )
        smoke_after_apply = run(
            "smoke", root, ["--smoke-command-file", str(passing_smoke_script)]
        )
        assert smoke_after_apply.returncode == 0, smoke_after_apply.stderr

        make_smoke_script = protected_smoke_script(
            root,
            "make-smoke.sh",
            "private_header='Authorization: Bearer make-smoke-token'\nexit 0\n",
        )
        make_smoke = subprocess.run(
            [
                "make",
                "eks-smoke-staging",
                "EKS_AWS_PROFILE=test-profile",
                "EKS_CONFIRM=STAGING_APPLY",
                f"IMAGE_DIGEST={DIGEST}",
                f"EKS_EVIDENCE_DIR={root / 'tmp/evidence'}",
            ],
            cwd=ROOT,
            text=True,
            capture_output=True,
            env={
                **os.environ,
                "PATH": f"{bindir}:{os.environ['PATH']}",
                "KUBECONFIG": str(root / "must-not-be-used"),
                "EKS_SMOKE_COMMAND_FILE": str(make_smoke_script),
                "FAKE_AWS_ACCOUNT": str(TARGET_POLICY["aws_account_id"]),
                "FAKE_AWS_REGION": str(TARGET_POLICY["aws_region"]),
                "FAKE_AWS_ROLE": str(TARGET_POLICY["delivery_role_name"]),
                "FAKE_EKS_CLUSTER": str(TARGET_POLICY["eks_cluster"]),
                "FAKE_TARGET_POLICY_VALUE": target_value(),
                "FAKE_RENDER_OBJECTS": rendered_objects(),
                "FAKE_SERVER_NORMALIZED_OBJECTS": rendered_objects(),
                "FAKE_LIVE_INVENTORY_FILE": str(root / "tmp/live-managed-inventory.json"),
                "FAKE_APPLIED_LIVE_MANAGED_OBJECTS": rendered_objects(),
                "FAKE_RENDERED_MANIFEST": rendered_manifest(),
                "FAKE_DEPLOYMENT_OBJECT": deployment_object(),
            },
        )
        assert make_smoke.returncode == 0, make_smoke.stderr
        assert "make-smoke-token" not in make_smoke.stdout and "make-smoke-token" not in make_smoke.stderr

        apply_evidence = json.loads((root / "tmp/evidence/evidence-apply.json").read_text())
        smoke_evidence = json.loads((root / "tmp/evidence/evidence-smoke.json").read_text())
        for evidence in (apply_evidence, smoke_evidence):
            assert re.fullmatch(r"[0-9a-f]{64}", str(evidence["configuration_fingerprint"]))
            assert evidence["configuration_fingerprint"] != evidence["rendered_manifest_sha256"]
            assert evidence["managed_resource_count"] == evidence["live_managed_resource_count"] == 2, evidence
            assert re.fullmatch(r"[0-9a-f]{64}", str(evidence["managed_resource_inventory_sha256"]))
            assert evidence["managed_resource_inventory_sha256"] == evidence["live_managed_resource_inventory_sha256"]
            assert re.fullmatch(r"[0-9a-f]{64}", str(evidence["managed_resource_spec_sha256"]))
            assert evidence["managed_resource_spec_sha256"] == evidence["live_managed_resource_spec_sha256"]
            assert re.fullmatch(r"[0-9a-f]{64}", str(evidence["live_pod_template_sha256"]))
            assert evidence["live_deployment_generation"] == evidence["live_deployment_observed_generation"] == 1
            assert "different-workload" not in json.dumps(evidence)
        promotion = run("promotion-plan", root)
        assert promotion.returncode == 0, promotion.stderr

        apply_evidence_path = root / "tmp/evidence/evidence-apply.json"
        original_apply_evidence = apply_evidence_path.read_text()
        failed_apply_evidence = json.loads(original_apply_evidence)
        failed_apply_evidence["outcome"] = "failed"
        apply_evidence_path.write_text(json.dumps(failed_apply_evidence))
        release_live_inventory = root / "tmp/evidence/release-live-managed-inventory.json"
        release_live_inventory.write_text(rendered_objects())
        release_evidence = subprocess.run(
            [
                "make",
                "eks-release-evidence",
                "EKS_AWS_PROFILE=test-profile",
                f"IMAGE_DIGEST={DIGEST}",
                f"EKS_EVIDENCE_DIR={root / 'tmp/evidence'}",
            ],
            cwd=ROOT,
            text=True,
            capture_output=True,
            env={
                **os.environ,
                "PATH": f"{bindir}:{os.environ['PATH']}",
                "KUBECONFIG": str(root / "must-not-be-used"),
                "FAKE_AWS_ACCOUNT": str(TARGET_POLICY["aws_account_id"]),
                "FAKE_AWS_REGION": str(TARGET_POLICY["aws_region"]),
                "FAKE_AWS_ROLE": str(TARGET_POLICY["delivery_role_name"]),
                "FAKE_EKS_CLUSTER": str(TARGET_POLICY["eks_cluster"]),
                "FAKE_TARGET_POLICY_VALUE": target_value(),
                "FAKE_RENDER_OBJECTS": rendered_objects(),
                "FAKE_SERVER_NORMALIZED_OBJECTS": rendered_objects(),
                "FAKE_LIVE_INVENTORY_FILE": str(release_live_inventory),
                "FAKE_APPLIED_LIVE_MANAGED_OBJECTS": rendered_objects(),
                "FAKE_RENDERED_MANIFEST": rendered_manifest(),
                "FAKE_DEPLOYMENT_OBJECT": deployment_object(),
            },
        )
        assert release_evidence.returncode != 0
        assert "Safe evidence:" not in release_evidence.stdout
        apply_evidence_path.write_text(original_apply_evidence)

        stale_promotion = run(
            "promotion-plan", root, live_inventory=inventory_with_stale_resource()
        )
        assert stale_promotion.returncode != 0 and "managed-resource inventory" in stale_promotion.stderr

        configuration_drift_promotion = run(
            "promotion-plan", root, live_inventory=inventory_with_service_configuration_drift()
        )
        assert (
            configuration_drift_promotion.returncode != 0
            and "configuration" in configuration_drift_promotion.stderr
        )

        configuration_drift = run(
            "promotion-plan",
            root,
            rendered_manifest_value=rendered_manifest("configuration-two"),
        )
        assert configuration_drift.returncode != 0 and "configuration_fingerprint" in configuration_drift.stderr

        rollout_drift = run(
            "promotion-plan",
            root,
            deployed_template_marker="template-two",
            deployed_generation=2,
        )
        assert rollout_drift.returncode != 0 and "live_deployment_generation" in rollout_drift.stderr

        make_dry_run = subprocess.run(["make", "-n", "eks-preflight"], cwd=ROOT, text=True, capture_output=True)
        assert make_dry_run.returncode == 0, make_dry_run.stderr
        assert "--eks-cluster" not in make_dry_run.stdout and "--approved-eks-cluster" not in make_dry_run.stdout
        make_smoke_dry_run = subprocess.run(
            ["make", "-n", "eks-smoke-staging"],
            cwd=ROOT,
            text=True,
            capture_output=True,
            env={
                **os.environ,
                "EKS_SMOKE_COMMAND_FILE": "/secure/ci/smoke-script.sh",
            },
        )
        assert make_smoke_dry_run.returncode == 0, make_smoke_dry_run.stderr
        assert "--smoke-command-file" in make_smoke_dry_run.stdout
        assert "EKS_SMOKE_COMMAND_FILE" in make_smoke_dry_run.stdout
        assert "Authorization:" not in make_smoke_dry_run.stdout
        legacy_target = subprocess.run(
            [
                "python3",
                str(SCRIPT),
                "preflight",
                "--aws-profile",
                "test-profile",
                "--evidence-dir",
                str(root / "tmp" / "legacy"),
                "--eks-cluster",
                "unapproved-cluster",
            ],
            text=True,
            capture_output=True,
        )
        assert legacy_target.returncode != 0 and "unrecognized arguments" in legacy_target.stderr
        legacy_smoke_command = run("smoke", root, ["--smoke-command", "exit 0"])
        assert (
            legacy_smoke_command.returncode != 0
            and "unrecognized arguments" in legacy_smoke_command.stderr
        )
    print("EKS delivery contract tests passed")


if __name__ == "__main__":
    raise SystemExit(main())
