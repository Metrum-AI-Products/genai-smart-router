#!/usr/bin/env python3
"""Contract tests for scripts/eks_delivery.py using fake cloud executables."""
from __future__ import annotations

import importlib.util
import json
import os
import re
import shutil
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
LINKERD_PROXY_IMAGE = "cr.l5d.io/linkerd/proxy@sha256:" + "b" * 64
LINKERD_INIT_IMAGE = "none"
DEPLOYMENT_UID = "smart-llmrouter-deployment-uid"


def target_value(value: dict[str, object] | None = None) -> str:
    return json.dumps(json.dumps(value or TARGET_POLICY, sort_keys=True, separators=(",", ":")))


def rendered_objects(namespace: str | None = None) -> str:
    namespace = namespace or str(TARGET_POLICY["k8s_namespace"])
    labels = {"app.kubernetes.io/name": "smart-llmrouter"}
    runtime_secret_name = str(TARGET_POLICY["runtime_secret_name"])
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
                                        "env": [
                                            {
                                                "name": "ROUTER_USAGE_DB_DSN",
                                                "valueFrom": {
                                                    "secretKeyRef": {
                                                        "name": runtime_secret_name,
                                                        "key": "ROUTER_USAGE_DB_DSN",
                                                    }
                                                },
                                            }
                                        ],
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
                                ],
                                "volumes": [
                                    {
                                        "name": "runtime",
                                        "secret": {"secretName": runtime_secret_name},
                                    }
                                ],
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


def inventory_with_discovered_ingress_policy() -> str:
    """Add the exact discovery-owned ingress policy to a live inventory."""

    payload = json.loads(rendered_objects())
    payload["items"].append(
        {
            "apiVersion": "networking.k8s.io/v1",
            "kind": "NetworkPolicy",
            "metadata": {
                "name": "smart-llmrouter-discovered-ingress",
                "namespace": TARGET_POLICY["k8s_namespace"],
                "labels": {"app.kubernetes.io/name": "smart-llmrouter"},
            },
            "spec": {
                "podSelector": {
                    "matchLabels": {"app.kubernetes.io/name": "smart-llmrouter"}
                },
                "policyTypes": ["Ingress"],
                "ingress": [],
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


def inventory_with_foreign_manager_service_owner_reference() -> str:
    payload = json.loads(rendered_objects())
    service = next(item for item in payload["items"] if item["kind"] == "Service")
    service["metadata"]["ownerReferences"] = [
        {
            "apiVersion": "example.test/v1",
            "kind": "ExternalManager",
            "name": "unreviewed-live-owner",
            "uid": "unreviewed-live-owner-uid",
        }
    ]
    return json.dumps(payload)


def inventory_with_foreign_manager_service_finalizer() -> str:
    payload = json.loads(rendered_objects())
    service = next(item for item in payload["items"] if item["kind"] == "Service")
    service["metadata"]["finalizers"] = ["external.example.test/unreviewed-live-finalizer"]
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
    runtime_secret_name = str(TARGET_POLICY["runtime_secret_name"])
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
                    "spec": {
                        "containers": [
                            {
                                "name": "router",
                                "image": digest,
                                "env": [
                                    {
                                        "name": "ROUTER_USAGE_DB_DSN",
                                        "valueFrom": {
                                            "secretKeyRef": {
                                                "name": runtime_secret_name,
                                                "key": "ROUTER_USAGE_DB_DSN",
                                            }
                                        },
                                    }
                                ],
                            }
                        ],
                        "volumes": [
                            {
                                "name": "runtime",
                                "secret": {"secretName": runtime_secret_name},
                            }
                        ],
                    },
                }
            },
        }
    )


def runtime_secret_attestation(
    secret_uid: str = "runtime-secret-uid-one",
    secret_resource_version: str = "101",
    *,
    attestation_uid: str = "runtime-secret-attestation-uid-one",
    attestation_resource_version: str = "201",
    secret_name: str | None = None,
    approved_router_image: str = DIGEST,
    approved_linkerd_proxy_image: str = LINKERD_PROXY_IMAGE,
    approved_linkerd_init_image: str = LINKERD_INIT_IMAGE,
    approved_pod_creator_username: str = "system:serviceaccount:kube-system:replicaset-controller",
) -> str:
    """A bootstrap-owned immutable, non-secret version attestation fixture."""

    secret_name = secret_name or str(TARGET_POLICY["runtime_secret_name"])
    return json.dumps(
        {
            "apiVersion": "v1",
            "kind": "ConfigMap",
            "metadata": {
                "name": TARGET_POLICY["runtime_secret_attestation_configmap_name"],
                "namespace": TARGET_POLICY["k8s_namespace"],
                "uid": attestation_uid,
                "resourceVersion": attestation_resource_version,
                "ownerReferences": [
                    {
                        "apiVersion": "v1",
                        "kind": "Secret",
                        "name": secret_name,
                        "uid": secret_uid,
                    }
                ],
            },
            "immutable": True,
            "data": {
                "schema_version": "v2",
                "secret_name": secret_name,
                "secret_uid": secret_uid,
                "secret_resource_version": secret_resource_version,
                "approved_router_image": approved_router_image,
                "approved_linkerd_proxy_image": approved_linkerd_proxy_image,
                "approved_linkerd_init_image": approved_linkerd_init_image,
                "approved_pod_creator_username": approved_pod_creator_username,
            },
        }
    )


def replica_sets_object(
    digest: str = DIGEST,
    revision: int = 1,
    owner_name: str = "smart-llmrouter",
    owner_uid: str = DEPLOYMENT_UID,
    template_marker: str = "template-one",
) -> str:
    runtime_secret_name = str(TARGET_POLICY["runtime_secret_name"])
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
                            "metadata": {
                                "annotations": {
                                    "example.metrum.ai/config-revision": template_marker
                                },
                                # Controller-owned, not an approved workload
                                # setting; the production fingerprint removes
                                # only this label.
                                "labels": {"pod-template-hash": "historical-revision"},
                            },
                            "spec": {
                                "containers": [
                                    {
                                        "name": "router",
                                        "image": digest,
                                        "env": [
                                            {
                                                "name": "ROUTER_USAGE_DB_DSN",
                                                "valueFrom": {
                                                    "secretKeyRef": {
                                                        "name": runtime_secret_name,
                                                        "key": "ROUTER_USAGE_DB_DSN",
                                                    }
                                                },
                                            }
                                        ],
                                    }
                                ],
                                "volumes": [
                                    {
                                        "name": "runtime",
                                        "secret": {"secretName": runtime_secret_name},
                                    }
                                ],
                            }
                        }
                    },
                }
            ],
        }
    )

def admission_policy_objects() -> str:
    """Minimal fake objects used to prove exact live/checked-in spec matching."""

    items: list[dict[str, object]] = []
    for name, api_groups, resource in (
        (EKS_DELIVERY.DELIVERY_ADMISSION_NAMES[0], ["apps"], "deployments"),
        (EKS_DELIVERY.DELIVERY_ADMISSION_NAMES[1], [""], "pods"),
    ):
        items.extend(
            (
                {
                    "apiVersion": "admissionregistration.k8s.io/v1",
                    "kind": "ValidatingAdmissionPolicy",
                    "metadata": {"name": name},
                    "spec": {
                        "failurePolicy": "Fail",
                        "matchConstraints": {
                            "resourceRules": [
                                {
                                    "apiGroups": api_groups,
                                    "apiVersions": ["v1"],
                                    "operations": ["CREATE", "UPDATE"],
                                    "resources": [resource],
                                    "scope": "Namespaced",
                                }
                            ]
                        },
                        "validations": [{"expression": "object.metadata.name != ''"}],
                    },
                },
                {
                    "apiVersion": "admissionregistration.k8s.io/v1",
                    "kind": "ValidatingAdmissionPolicyBinding",
                    "metadata": {"name": name},
                    "spec": {
                        "policyName": name,
                        "validationActions": ["Deny", "Audit"],
                        "paramRef": {
                            "name": TARGET_POLICY[
                                "runtime_secret_attestation_configmap_name"
                            ],
                            "namespace": TARGET_POLICY["k8s_namespace"],
                            "parameterNotFoundAction": "Deny",
                        },
                    },
                },
            )
        )
    return json.dumps({"apiVersion": "v1", "kind": "List", "items": items})


def admission_environment_variables(payload: str) -> dict[str, str]:
    environment: dict[str, str] = {}
    for item in json.loads(payload)["items"]:
        kind = "POLICY" if item["kind"] == "ValidatingAdmissionPolicy" else "BINDING"
        name = item["metadata"]["name"].split("-")[-1].upper()
        environment[f"FAKE_LIVE_ADMISSION_{kind}_{name}"] = json.dumps(item)
    return environment


def fake_admission_environment() -> dict[str, str]:
    payload = admission_policy_objects()
    return {
        "FAKE_ADMISSION_RBAC": "no",
        "FAKE_ADMISSION_POLICY_OBJECTS": payload,
        **admission_environment_variables(payload),
    }

def fake_tools(directory: Path) -> None:
    log = directory / "calls.log"
    for name in ("aws", "kubectl", "kustomize"):
        body = f'''#!/bin/sh
echo "$0 $* KUBECONFIG=$KUBECONFIG" >> "{log}"
emit_can_i() {{
  printf '%s\n' "$1"
  if test "$1" = no && test "${{FAKE_KUBECTL_CAN_I_NO_EXIT:-0}}" = 1; then
    return 1
  fi
}}
case "$0" in
  *aws) case "$*" in
    *"ssm get-parameter"*) printf '{{"Parameter":{{"Type":"String","Value":%s}}}}\\n' "$FAKE_TARGET_POLICY_VALUE" ;;
    *get-caller-identity*) printf '{{"Account":"%s","Arn":"arn:aws:sts::%s:assumed-role/%s/test-session"}}\\n' "$FAKE_AWS_ACCOUNT" "$FAKE_AWS_ACCOUNT" "$FAKE_AWS_ROLE" ;;
    *describe-cluster*) printf '{{"cluster":{{"status":"ACTIVE","arn":"arn:aws:eks:%s:%s:cluster/%s"}}}}\\n' "$FAKE_AWS_REGION" "$FAKE_AWS_ACCOUNT" "$FAKE_EKS_CLUSTER" ;;
  esac ;;
  *kubectl) case "$*" in
    *"auth can-i get validatingadmissionpolicy/"*) emit_can_i yes ;;
    *"auth can-i get validatingadmissionpolicybinding/"*) emit_can_i yes ;;
    *"auth can-i"*validatingadmission*) emit_can_i "$FAKE_ADMISSION_RBAC" ;;
    *"auth can-i"*secret*) emit_can_i "$FAKE_SECRET_RBAC" ;;
    *"auth can-i get configmap/$FAKE_RUNTIME_SECRET_ATTESTATION_NAME"*) emit_can_i yes ;;
    *"auth can-i"*configmap*) emit_can_i "$FAKE_CONFIGMAP_RBAC" ;;
    *"auth can-i delete "*|*"auth can-i deletecollection "*) emit_can_i "${{FAKE_MANAGED_DELETE_RBAC:-no}}" ;;
    *"auth can-i"*) emit_can_i yes ;;
    *"create --dry-run=client"*"eks-staging-delivery-admission.yaml"*) printf '%s\n' "$FAKE_ADMISSION_POLICY_OBJECTS" ;;
    *"apply --dry-run=client"*) printf '%s\\n' "$FAKE_RENDER_OBJECTS" ;;
    *"--dry-run=server"*" -o json"*) previous=''; for arg; do if [ "$previous" = '-f' ]; then manifest="$arg"; break; fi; previous="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}"; printf '%s\\n' "$FAKE_SERVER_NORMALIZED_OBJECTS" ;;
    *"apply --server-side --dry-run=server"*) previous=''; for arg; do if [ "$previous" = '-f' ]; then manifest="$arg"; break; fi; previous="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}" ;;
    *"get validatingadmissionpolicy/genai-smart-router-eks-staging-linkerd-pod"*) printf '%s\n' "$FAKE_LIVE_ADMISSION_POLICY_POD" ;;
    *"get validatingadmissionpolicy/genai-smart-router-eks-staging-delivery"*) printf '%s\n' "$FAKE_LIVE_ADMISSION_POLICY_DELIVERY" ;;
    *"get validatingadmissionpolicybinding/genai-smart-router-eks-staging-linkerd-pod"*) printf '%s\n' "$FAKE_LIVE_ADMISSION_BINDING_POD" ;;
    *"get validatingadmissionpolicybinding/genai-smart-router-eks-staging-delivery"*) printf '%s\n' "$FAKE_LIVE_ADMISSION_BINDING_DELIVERY" ;;
    *"get deployments,ingresses,networkpolicies,persistentvolumeclaims,poddisruptionbudgets,services,serviceaccounts"*) cat "$FAKE_LIVE_INVENTORY_FILE" ;;
    *"get replicasets"*) printf '%s\\n' "$FAKE_REPLICA_SETS" ;;
    *"get secret"*) echo "unexpected Secret read" >&2; exit 46 ;;
    *"get configmap $FAKE_RUNTIME_SECRET_ATTESTATION_NAME"*) if test -n "$FAKE_RUNTIME_SECRET_ATTESTATION_FILE"; then cat "$FAKE_RUNTIME_SECRET_ATTESTATION_FILE"; else printf '%s\n' "$FAKE_RUNTIME_SECRET_ATTESTATION"; fi ;;
    *"get deployment/smart-llmrouter"*) printf '%s\\n' "$FAKE_DEPLOYMENT_OBJECT" ;;
    *"rollout status"*) : ;;
    *apply*) for arg; do manifest="$arg"; done; printf 'manifest-bytes=' >> "{log}"; wc -c < "$manifest" >> "{log}"; printf '%s\\n' "$FAKE_APPLIED_LIVE_MANAGED_OBJECTS" > "$FAKE_LIVE_INVENTORY_FILE"; if test -n "$FAKE_RUNTIME_SECRET_ATTESTATION_AFTER_APPLY"; then printf '%s\\n' "$FAKE_RUNTIME_SECRET_ATTESTATION_AFTER_APPLY" > "$FAKE_RUNTIME_SECRET_ATTESTATION_FILE"; fi ;;
  esac ;;
  *kustomize) case "$*" in
    "edit set image $FAKE_KUSTOMIZE_SOURCE_IMAGE=$FAKE_IMAGE_DIGEST") : > "$FAKE_KUSTOMIZE_EDIT_MARKER" ;;
    *"edit set image"*) echo "unexpected kustomize image override" >&2; exit 44 ;;
    *build*) if test ! -f "$FAKE_KUSTOMIZE_EDIT_MARKER"; then echo "missing exact kustomize image override" >&2; exit 45; fi; printf '%s\\n' "$FAKE_RENDERED_MANIFEST" ;;
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
    runtime_secret_uid: str = "runtime-secret-uid-one",
    runtime_secret_resource_version: str = "101",
    runtime_secret_attestation_uid: str = "runtime-secret-attestation-uid-one",
    runtime_secret_attestation_resource_version: str = "201",
    runtime_secret_attested_name: str | None = None,
    runtime_secret_attestation_payload: str | None = None,
    runtime_secret_attestation_after_apply_payload: str | None = None,
    secret_rbac: str = "no",
    configmap_rbac: str = "no",
    admission_rbac: str = "no",
    managed_delete_rbac: str = "no",
    kubectl_can_i_no_exit: bool = False,
    live_admission_objects: str | None = None,
    replica_sets: str | None = None,
    rollback_pod_template_sha256: str | None = None,
    include_rollback_pod_template_sha256: bool = True,
    confirm_smoke: bool = True,
    include_promotion_evidence: bool = True,
    promotion_evidence_dir: Path | None = None,
    aws_profile: str = "test-profile",
    evidence_dir: Path | None = None,
) -> subprocess.CompletedProcess[str]:
    evidence = evidence_dir or root / "tmp" / "evidence"
    promotion_evidence = root / "tmp" / "promotion-evidence"
    replica_sets_payload = replica_sets or replica_sets_object()
    expected_admission_payload = admission_policy_objects()
    live_admission_payload = json.loads(
        live_admission_objects or expected_admission_payload
    )
    command = [
        "python3",
        str(SCRIPT),
        action,
        "--aws-profile",
        aws_profile,
        "--evidence-dir",
        str(evidence),
    ]
    if action == "promotion-plan" and include_promotion_evidence:
        command += [
            "--promotion-evidence-dir",
            str(promotion_evidence_dir or promotion_evidence),
        ]
    if include_digest:
        command += ["--image-digest", image_digest]
    if action == "smoke" and confirm_smoke:
        command += ["--confirm", "STAGING_APPLY"]
    if action == "rollback" and include_rollback_pod_template_sha256:
        candidate_template = json.loads(replica_sets_payload)["items"][0]["spec"]["template"]
        command += [
            "--rollback-pod-template-sha256",
            rollback_pod_template_sha256
            or EKS_DELIVERY.pod_template_sha256(candidate_template),
        ]
    if extra:
        command += extra
    bindir = root / "fake-bin"
    live_inventory_file = root / "tmp" / "live-managed-inventory.json"
    runtime_secret_attestation_file = root / "tmp" / "runtime-secret-attestation.json"
    live_inventory_file.parent.mkdir(parents=True, exist_ok=True)
    live_inventory_file.write_text(live_inventory or rendered_objects())
    attestation_payload = runtime_secret_attestation_payload or runtime_secret_attestation(
        runtime_secret_uid,
        runtime_secret_resource_version,
        attestation_uid=runtime_secret_attestation_uid,
        attestation_resource_version=runtime_secret_attestation_resource_version,
        secret_name=runtime_secret_attested_name,
    )
    runtime_secret_attestation_file.write_text(attestation_payload)
    env = {
        **os.environ,
        "PATH": f"{bindir}:{os.environ['PATH']}",
        "KUBECONFIG": str(root / "must-not-be-used"),
        "FAKE_AWS_ACCOUNT": account or str(TARGET_POLICY["aws_account_id"]),
        "FAKE_AWS_REGION": str(TARGET_POLICY["aws_region"]),
        "FAKE_AWS_ROLE": str(TARGET_POLICY["delivery_role_name"]),
        "FAKE_EKS_CLUSTER": str(TARGET_POLICY["eks_cluster"]),
        "FAKE_SECRET_RBAC": secret_rbac,
        "FAKE_CONFIGMAP_RBAC": configmap_rbac,
        "FAKE_ADMISSION_RBAC": admission_rbac,
        "FAKE_MANAGED_DELETE_RBAC": managed_delete_rbac,
        "FAKE_KUBECTL_CAN_I_NO_EXIT": "1" if kubectl_can_i_no_exit else "0",
        "FAKE_ADMISSION_POLICY_OBJECTS": expected_admission_payload,
        **admission_environment_variables(json.dumps(live_admission_payload)),
        "FAKE_TARGET_POLICY_VALUE": target_value(policy),
        "FAKE_RENDER_OBJECTS": render_objects or rendered_objects(),
        "FAKE_SERVER_NORMALIZED_OBJECTS": server_normalized_inventory or render_objects or rendered_objects(),
        "FAKE_LIVE_INVENTORY_FILE": str(live_inventory_file),
        "FAKE_APPLIED_LIVE_MANAGED_OBJECTS": live_inventory_after_apply
        or live_inventory
        or rendered_objects(),
        "FAKE_RENDERED_MANIFEST": rendered_manifest_value or rendered_manifest(),
        "FAKE_IMAGE_DIGEST": image_digest,
        "FAKE_KUSTOMIZE_EDIT_MARKER": str(root / "tmp" / "kustomize-image-overridden"),
        "FAKE_KUSTOMIZE_SOURCE_IMAGE": str(TARGET_POLICY["kustomize_router_image_name"]),
        "FAKE_DEPLOYMENT_OBJECT": deployment_object(
            deployed_digest,
            deployed_template_marker,
            deployed_generation,
            deployed_observed_generation,
            deployed_revision,
        ),
        "FAKE_RUNTIME_SECRET_ATTESTATION_NAME": str(
            TARGET_POLICY["runtime_secret_attestation_configmap_name"]
        ),
        "FAKE_RUNTIME_SECRET_ATTESTATION": attestation_payload,
        "FAKE_RUNTIME_SECRET_ATTESTATION_FILE": str(runtime_secret_attestation_file),
        "FAKE_RUNTIME_SECRET_ATTESTATION_AFTER_APPLY": runtime_secret_attestation_after_apply_payload
        or "",
        "FAKE_REPLICA_SETS": replica_sets_payload,
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
    assert target.image_architecture == TARGET_POLICY["image_architecture"]
    staging_deployment_patch = (target.kustomize_overlay / "patch-deployment.yaml").read_text(
        encoding="utf-8"
    )
    assert (
        f"image: {target.kustomize_router_image_name}:replace-with-immutable-image-tag"
        in staging_deployment_patch
    )
    expected_resources = all_managed_configuration_objects()
    expected_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
        expected_resources, target, source="configuration fingerprint fixture"
    )
    expected_identities = EKS_DELIVERY.managed_resource_identities(expected_inventory)
    expected_spec_fingerprint = EKS_DELIVERY.managed_resource_spec_fingerprint(expected_inventory)
    invalid_source_image_policy = dict(TARGET_POLICY)
    invalid_source_image_policy["kustomize_router_image_name"] = "smart-llmrouter:mutable"
    try:
        EKS_DELIVERY.parse_target_policy(invalid_source_image_policy)
    except RuntimeError as exc:
        assert "Kustomize source image" in str(exc)
    else:
        raise AssertionError("target policy accepted a tagged Kustomize source image")

    invalid_architecture_policy = dict(TARGET_POLICY)
    invalid_architecture_policy["image_architecture"] = "linux/s390x"
    try:
        EKS_DELIVERY.parse_target_policy(invalid_architecture_policy)
    except RuntimeError as exc:
        assert "image architecture" in str(exc)
    else:
        raise AssertionError("target policy accepted an unsupported image architecture")

    missing_attestation_policy = dict(TARGET_POLICY)
    del missing_attestation_policy["runtime_secret_attestation_configmap_name"]
    try:
        EKS_DELIVERY.parse_target_policy(missing_attestation_policy)
    except RuntimeError as exc:
        assert "unexpected schema" in str(exc)
    else:
        raise AssertionError("target policy accepted a missing Secret attestation ConfigMap")

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

    for metadata_name, metadata_value in (
        (
            "ownerReferences",
            [
                {
                    "apiVersion": "example.test/v1",
                    "kind": "ExternalManager",
                    "name": "reviewed-owner",
                    "uid": "reviewed-owner-uid",
                }
            ],
        ),
        ("finalizers", ["external.example.test/reviewed-finalizer"]),
        ("deletionTimestamp", "2026-07-20T00:00:00Z"),
    ):
        changed_resources = json.loads(json.dumps(expected_resources))
        service = next(item for item in changed_resources if item["kind"] == "Service")
        service["metadata"][metadata_name] = metadata_value
        changed_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
            changed_resources, target, source="metadata configuration drift fixture"
        )
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
            "creationTimestamp": "2026-07-20T00:00:00Z",
            "generation": 7,
            "managedFields": [{"manager": "kube-controller-manager"}],
            "resourceVersion": "12345",
            "selfLink": "/api/v1/namespaces/staging/services/smart-llmrouter",
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
    runtime_pvc["metadata"]["annotations"] = {
        "volume.kubernetes.io/selected-node": "ip-10-0-0-10"
    }
    runtime_pvc["metadata"]["finalizers"] = ["kubernetes.io/pvc-protection"]
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

    unexpected_pvc_metadata = json.loads(json.dumps(expected_resources))
    unexpected_pvc = next(
        item for item in unexpected_pvc_metadata if item["kind"] == "PersistentVolumeClaim"
    )
    unexpected_pvc["metadata"]["annotations"] = {
        "external.example.test/unreviewed": "true"
    }
    unexpected_pvc["metadata"]["finalizers"] = [
        "kubernetes.io/pvc-protection",
        "external.example.test/unreviewed-finalizer",
    ]
    unexpected_pvc_inventory = EKS_DELIVERY.Delivery.inventory_from_resources(
        unexpected_pvc_metadata, target, source="unreviewed PVC metadata fixture"
    )
    assert (
        EKS_DELIVERY.managed_resource_spec_fingerprint(unexpected_pvc_inventory)
        != expected_spec_fingerprint
    )

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        bindir = root / "fake-bin"
        bindir.mkdir()
        fake_tools(bindir)

        assert run("preflight", root, kubectl_can_i_no_exit=True).returncode == 0
        result = run("plan", root)
        assert result.returncode == 0, result.stderr

        unapproved_runtime_secret = json.loads(rendered_objects())
        deployment = next(
            item for item in unapproved_runtime_secret["items"] if item["kind"] == "Deployment"
        )
        deployment["spec"]["template"]["spec"]["volumes"][0]["secret"][
            "secretName"
        ] = "another-runtime-secret"
        rejected_runtime_secret = run(
            "plan", root, render_objects=json.dumps(unapproved_runtime_secret)
        )
        assert (
            rejected_runtime_secret.returncode != 0
            and "approved runtime Secret" in rejected_runtime_secret.stderr
        )
        calls = (bindir / "calls.log").read_text()
        expected_kustomize_override = (
            "kustomize edit set image "
            f"{TARGET_POLICY['kustomize_router_image_name']}={DIGEST}"
        )
        assert expected_kustomize_override in calls
        assert f"kustomize edit set image smart-llmrouter={DIGEST}" not in calls
        assert "must-not-be-used" not in calls and "update-kubeconfig" in calls and "--dry-run=server" in calls
        assert calls.index("ssm get-parameter") < calls.index("describe-cluster")
        manifest_size = re.search(r"manifest-bytes=\s*(\d+)", calls)
        assert manifest_size and int(manifest_size.group(1)) > 4000
        assert (
            f"auth can-i get secret/{TARGET_POLICY['runtime_secret_name']}" in calls
        )
        assert f"get secret {TARGET_POLICY['runtime_secret_name']}" not in calls
        assert (
            "get configmap "
            f"{TARGET_POLICY['runtime_secret_attestation_configmap_name']}" in calls
        )

        inherited_secret_access = run("preflight", root, secret_rbac="yes")
        assert (
            inherited_secret_access.returncode != 0
            and "forbidden Secret permission" in inherited_secret_access.stderr
        )
        inherited_configmap_write_access = run(
            "preflight", root, configmap_rbac="yes"
        )
        assert (
            inherited_configmap_write_access.returncode != 0
            and "forbidden ConfigMap permission"
            in inherited_configmap_write_access.stderr
        )
        inherited_admission_write_access = run(
            "preflight", root, admission_rbac="yes"
        )
        assert (
            inherited_admission_write_access.returncode != 0
            and "forbidden delivery admission-policy mutation"
            in inherited_admission_write_access.stderr
        )
        inherited_managed_delete_access = run(
            "preflight", root, managed_delete_rbac="yes"
        )
        assert (
            inherited_managed_delete_access.returncode != 0
            and "forbidden delete authority" in inherited_managed_delete_access.stderr
        )

        weakened_admission = json.loads(admission_policy_objects())
        weakened_admission["items"][0]["spec"]["failurePolicy"] = "Ignore"
        mismatched_admission = run(
            "preflight",
            root,
            live_admission_objects=json.dumps(weakened_admission),
        )
        assert (
            mismatched_admission.returncode != 0
            and "differs from the reviewed contract" in mismatched_admission.stderr
        )

        # The delivery contract fails before a mutating apply if the
        # bootstrap-owned attestation is not a strictly safe immutable
        # ConfigMap. The fake kubectl rejects every actual Secret read above.
        for mutation_name, mutate in (
            ("non-immutable", lambda item: item.update({"immutable": False})),
            ("binary data", lambda item: item.update({"binaryData": {"x": "eA=="}})),
            ("extra data", lambda item: item["data"].update({"unexpected": "value"})),
            ("wrong owner", lambda item: item["metadata"].update({"ownerReferences": []})),
            ("deletion in progress", lambda item: item["metadata"].update({"deletionTimestamp": "2026-07-20T00:00:00Z"})),
            ("wrong secret name", lambda item: item["data"].update({"secret_name": "wrong-secret"})),
            ("legacy schema", lambda item: item["data"].update({"schema_version": "v1"})),
            ("mutable router image", lambda item: item["data"].update({"approved_router_image": "registry.example/router:latest"})),
            ("unsafe proxy image", lambda item: item["data"].update({"approved_linkerd_proxy_image": "bad image"})),
            ("unsafe init image", lambda item: item["data"].update({"approved_linkerd_init_image": "bad image"})),
        ):
            invalid_attestation = json.loads(runtime_secret_attestation())
            mutate(invalid_attestation)
            before_invalid_apply = (bindir / "calls.log").read_text()
            invalid_apply = run(
                "apply",
                root,
                ["--confirm", "STAGING_APPLY"],
                runtime_secret_attestation_payload=json.dumps(invalid_attestation),
            )
            assert invalid_apply.returncode != 0, mutation_name
            assert "attestation" in invalid_apply.stderr, mutation_name
            invalid_apply_calls = (bindir / "calls.log").read_text()[
                len(before_invalid_apply):
            ]
            assert "apply --server-side -f" not in invalid_apply_calls, mutation_name

        unapproved_digest = str(TARGET_POLICY["ecr_repository_uri"]) + "@sha256:" + "c" * 64
        digest_mismatch = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            runtime_secret_attestation_payload=runtime_secret_attestation(
                approved_router_image=unapproved_digest
            ),
        )
        assert (
            digest_mismatch.returncode != 0
            and "not bootstrap-approved" in digest_mismatch.stderr
        )

        replacement_attestation = json.loads(runtime_secret_attestation())
        replacement_attestation["metadata"]["resourceVersion"] = "202"
        before_changed_attestation_apply = (bindir / "calls.log").read_text()
        changed_attestation_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            runtime_secret_attestation_after_apply_payload=json.dumps(
                replacement_attestation
            ),
        )
        assert (
            changed_attestation_apply.returncode != 0
            and "attestation changed during staging apply" in changed_attestation_apply.stderr
        )
        changed_attestation_apply_calls = (bindir / "calls.log").read_text()[
            len(before_changed_attestation_apply):
        ]
        assert "apply --server-side -f" in changed_attestation_apply_calls

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

        before_missing_rollback_template = (bindir / "calls.log").read_text()
        missing_rollback_template = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            include_rollback_pod_template_sha256=False,
        )
        assert (
            missing_rollback_template.returncode != 0
            and "ROLLBACK_POD_TEMPLATE_SHA256" in missing_rollback_template.stderr
        )
        assert "rollout undo" not in (bindir / "calls.log").read_text()[
            len(before_missing_rollback_template):
        ]

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

        # A same-digest historical ReplicaSet must still match the explicitly
        # approved complete pod template before rollback can mutate anything.
        approved_template = json.loads(replica_sets_object())["items"][0]["spec"]["template"]
        stale_template = replica_sets_object(template_marker="stale-template")
        before_stale_template_rollback = (bindir / "calls.log").read_text()
        stale_template_rollback = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            replica_sets=stale_template,
            rollback_pod_template_sha256=EKS_DELIVERY.pod_template_sha256(approved_template),
        )
        assert (
            stale_template_rollback.returncode != 0
            and "approved IMAGE_DIGEST and pod template" in stale_template_rollback.stderr
        )
        assert "rollout undo" not in (bindir / "calls.log").read_text()[
            len(before_stale_template_rollback):
        ]

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

        template_mismatch_rollback = run(
            "rollback",
            root,
            ["--confirm", "STAGING_APPLY"],
            deployed_template_marker="template-two",
        )
        assert (
            template_mismatch_rollback.returncode != 0
            and "approved rollback pod template" in template_mismatch_rollback.stderr
        )

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
        assert rollback_evidence["rollback_target_pod_template_sha256"] == EKS_DELIVERY.pod_template_sha256(
            json.loads(replica_sets_object())["items"][0]["spec"]["template"]
        )
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

        label_selected_discovered_ingress = inventory_with_discovered_ingress_policy()
        label_selected_discovered_ingress_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            live_inventory=label_selected_discovered_ingress,
        )
        assert (
            label_selected_discovered_ingress_apply.returncode != 0
            and "stale resources" in label_selected_discovered_ingress_apply.stderr
        )

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
        safe_promotion_path = (
            root
            / "tmp"
            / "promotion-evidence"
            / EKS_DELIVERY.PROMOTION_PLAN_SAFE_EVIDENCE_FILE
        )
        assert not safe_promotion_path.exists()
        missing_promotion_evidence = run(
            "promotion-plan", root, include_promotion_evidence=False
        )
        assert (
            missing_promotion_evidence.returncode != 0
            and "promotion-evidence-dir" in missing_promotion_evidence.stderr
        )
        overlapping_promotion_evidence = run(
            "promotion-plan",
            root,
            promotion_evidence_dir=root / "tmp" / "evidence",
        )
        assert (
            overlapping_promotion_evidence.returncode != 0
            and "must be separate" in overlapping_promotion_evidence.stderr
        )
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

        foreign_manager_finalizer_marker = root / "foreign-manager-finalizer-smoke-command-ran"
        foreign_manager_finalizer_script = protected_smoke_script(
            root,
            "foreign-manager-finalizer-smoke.sh",
            f"touch '{foreign_manager_finalizer_marker}'\n",
        )
        foreign_manager_finalizer_smoke = run(
            "smoke",
            root,
            ["--smoke-command-file", str(foreign_manager_finalizer_script)],
            live_inventory=inventory_with_foreign_manager_service_finalizer(),
        )
        assert (
            foreign_manager_finalizer_smoke.returncode != 0
            and "configuration" in foreign_manager_finalizer_smoke.stderr
        )
        assert not foreign_manager_finalizer_marker.exists()

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

        before_foreign_owner_reference_apply = (bindir / "calls.log").read_text()
        foreign_owner_reference_apply = run(
            "apply",
            root,
            ["--confirm", "STAGING_APPLY"],
            live_inventory=inventory_with_foreign_manager_service_owner_reference(),
        )
        assert (
            foreign_owner_reference_apply.returncode != 0
            and "fields absent from the reviewed" in foreign_owner_reference_apply.stderr
        )
        foreign_owner_reference_apply_calls = (bindir / "calls.log").read_text()[
            len(before_foreign_owner_reference_apply):
        ]
        assert "apply --server-side -f" not in foreign_owner_reference_apply_calls
        reapplied_after_owner_reference_rejection = run(
            "apply", root, ["--confirm", "STAGING_APPLY"]
        )
        assert (
            reapplied_after_owner_reference_rejection.returncode == 0
        ), reapplied_after_owner_reference_rejection.stderr

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
                "EKS_DELIVERY_AWS_PROFILE=test-profile",
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
                "FAKE_SECRET_RBAC": "no",
                "FAKE_CONFIGMAP_RBAC": "no",
                "FAKE_TARGET_POLICY_VALUE": target_value(),
                "FAKE_RENDER_OBJECTS": rendered_objects(),
                "FAKE_SERVER_NORMALIZED_OBJECTS": rendered_objects(),
                "FAKE_LIVE_INVENTORY_FILE": str(root / "tmp/live-managed-inventory.json"),
                "FAKE_APPLIED_LIVE_MANAGED_OBJECTS": rendered_objects(),
                "FAKE_RENDERED_MANIFEST": rendered_manifest(),
                "FAKE_IMAGE_DIGEST": DIGEST,
                "FAKE_KUSTOMIZE_EDIT_MARKER": str(root / "tmp" / "kustomize-image-overridden"),
                "FAKE_KUSTOMIZE_SOURCE_IMAGE": str(TARGET_POLICY["kustomize_router_image_name"]),
                "FAKE_DEPLOYMENT_OBJECT": deployment_object(),
                "FAKE_RUNTIME_SECRET_ATTESTATION_NAME": str(
                    TARGET_POLICY["runtime_secret_attestation_configmap_name"]
                ),
                "FAKE_RUNTIME_SECRET_ATTESTATION": runtime_secret_attestation(),
                **fake_admission_environment(),
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
            assert evidence["image_architecture"] == TARGET_POLICY["image_architecture"]
            assert evidence["runtime_secret_name"] == TARGET_POLICY["runtime_secret_name"]
            assert (
                evidence["runtime_secret_attestation_configmap_name"]
                == TARGET_POLICY["runtime_secret_attestation_configmap_name"]
            )
            assert evidence["runtime_secret_uid"] == "runtime-secret-uid-one"
            assert evidence["runtime_secret_resource_version"] == "101"
            assert evidence["runtime_secret_attestation_uid"] == "runtime-secret-attestation-uid-one"
            assert evidence["runtime_secret_attestation_resource_version"] == "201"
            assert "different-workload" not in json.dumps(evidence)
        secret_shaped_evidence_dir = root / "tmp" / "evidence-token=redacted-test-material"
        shutil.copytree(root / "tmp" / "evidence", secret_shaped_evidence_dir)
        secret_path_promotion = run(
            "promotion-plan", root, evidence_dir=secret_shaped_evidence_dir
        )
        secret_path_output = secret_path_promotion.stdout + secret_path_promotion.stderr
        assert secret_path_promotion.returncode == 0, secret_path_promotion.stderr
        assert json.loads(secret_path_promotion.stdout) == {
            "action": "promotion-plan",
            "outcome": "passed",
        }
        assert str(secret_shaped_evidence_dir) not in secret_path_output
        assert "redacted-test-material" not in secret_path_output
        promotion = run("promotion-plan", root)
        assert promotion.returncode == 0, promotion.stderr
        raw_promotion = json.loads(
            (root / "tmp/evidence/evidence-promotion-plan.json").read_text()
        )
        safe_promotion = json.loads(safe_promotion_path.read_text())
        assert set(safe_promotion) == EKS_DELIVERY.PROMOTION_PLAN_SAFE_EVIDENCE_FIELDS
        assert safe_promotion == {
            "schema_version": 1,
            "outcome": "passed",
            "timestamp": raw_promotion["timestamp"],
            "environment": "staging",
            "action": "promotion-plan",
            "image_digest": DIGEST,
            "configuration_fingerprint": raw_promotion["configuration_fingerprint"],
            "promotion_plan_result": "review_required_no_production_apply",
        }
        assert "events" in raw_promotion and "events" not in safe_promotion
        assert raw_promotion["aws_account_id"] == TARGET_POLICY["aws_account_id"]
        assert "aws_account_id" not in safe_promotion

        invalid_after_success = run("promotion-plan", root, include_digest=False)
        assert (
            invalid_after_success.returncode != 0
            and "IMAGE_DIGEST" in invalid_after_success.stderr
        )
        assert not safe_promotion_path.exists()
        replacement_promotion = run("promotion-plan", root)
        assert replacement_promotion.returncode == 0, replacement_promotion.stderr
        assert safe_promotion_path.exists()
        authorized_profile = run(
            "promotion-plan", root, aws_profile="authorized.user@example.com"
        )
        assert authorized_profile.returncode == 0, authorized_profile.stderr
        assert safe_promotion_path.exists()
        invalid_profile_after_success = run(
            "promotion-plan", root, aws_profile="invalid profile"
        )
        assert (
            invalid_profile_after_success.returncode != 0
            and "aws-profile" in invalid_profile_after_success.stderr
        )
        assert not safe_promotion_path.exists()
        replacement_promotion = run("promotion-plan", root)
        assert replacement_promotion.returncode == 0, replacement_promotion.stderr
        assert safe_promotion_path.exists()

        # A raw evidence path can become unreadable after a prior successful
        # promotion-plan. The next attempt must revoke the former safe handoff
        # before trying to resolve that raw path, without leaking it in the
        # generic protected-input error.
        looping_evidence_name = "raw-evidence-sk_test_aaaaaaaaaaaaaaaaaaaa"
        looping_evidence_dir = root / "tmp" / looping_evidence_name
        looping_evidence_dir.symlink_to(looping_evidence_name)
        looping_promotion = run(
            "promotion-plan", root, evidence_dir=looping_evidence_dir
        )
        looping_output = looping_promotion.stdout + looping_promotion.stderr
        assert looping_promotion.returncode != 0
        assert not safe_promotion_path.exists()
        assert "Traceback" not in looping_output
        assert str(looping_evidence_dir) not in looping_output
        assert looping_evidence_name not in looping_output

        replacement_promotion = run("promotion-plan", root)
        assert replacement_promotion.returncode == 0, replacement_promotion.stderr
        assert safe_promotion_path.exists()

        finalizer_drift_promotion = run(
            "promotion-plan", root, live_inventory=inventory_with_foreign_manager_service_finalizer()
        )
        assert (
            finalizer_drift_promotion.returncode != 0
            and "configuration" in finalizer_drift_promotion.stderr
        )
        assert not safe_promotion_path.exists()

        # A runtime Secret update does not change the Deployment template. Its
        # resourceVersion must nevertheless invalidate prior apply/smoke proof
        # before a promotion decision can be emitted.
        runtime_secret_drift = run(
            "promotion-plan", root, runtime_secret_resource_version="102"
        )
        assert (
            runtime_secret_drift.returncode != 0
            and "runtime_secret_resource_version" in runtime_secret_drift.stderr
        )

        # Replacing the immutable non-secret attestation (or otherwise changing
        # its API version) must also invalidate prior apply/smoke evidence.
        runtime_secret_attestation_drift = run(
            "promotion-plan", root, runtime_secret_attestation_resource_version="202"
        )
        assert (
            runtime_secret_attestation_drift.returncode != 0
            and "runtime_secret_attestation_resource_version"
            in runtime_secret_attestation_drift.stderr
        )

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
                "EKS_DELIVERY_AWS_PROFILE=test-profile",
                f"IMAGE_DIGEST={DIGEST}",
                f"EKS_EVIDENCE_DIR={root / 'tmp/evidence'}",
                f"EKS_PROMOTION_EVIDENCE_DIR={root / 'tmp/promotion-evidence'}",
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
                "FAKE_SECRET_RBAC": "no",
                "FAKE_CONFIGMAP_RBAC": "no",
                "FAKE_TARGET_POLICY_VALUE": target_value(),
                "FAKE_RENDER_OBJECTS": rendered_objects(),
                "FAKE_SERVER_NORMALIZED_OBJECTS": rendered_objects(),
                "FAKE_LIVE_INVENTORY_FILE": str(release_live_inventory),
                "FAKE_APPLIED_LIVE_MANAGED_OBJECTS": rendered_objects(),
                "FAKE_RENDERED_MANIFEST": rendered_manifest(),
                "FAKE_IMAGE_DIGEST": DIGEST,
                "FAKE_KUSTOMIZE_EDIT_MARKER": str(root / "tmp" / "kustomize-image-overridden"),
                "FAKE_KUSTOMIZE_SOURCE_IMAGE": str(TARGET_POLICY["kustomize_router_image_name"]),
                "FAKE_DEPLOYMENT_OBJECT": deployment_object(),
                "FAKE_RUNTIME_SECRET_ATTESTATION_NAME": str(
                    TARGET_POLICY["runtime_secret_attestation_configmap_name"]
                ),
                "FAKE_RUNTIME_SECRET_ATTESTATION": runtime_secret_attestation(),
                **fake_admission_environment(),
            },
        )
        assert release_evidence.returncode != 0
        assert "Safe promotion handoff:" not in release_evidence.stdout
        assert not safe_promotion_path.exists()
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
        make_rollback_dry_run = subprocess.run(
            ["make", "-n", "eks-rollback-staging"], cwd=ROOT, text=True, capture_output=True
        )
        assert make_rollback_dry_run.returncode == 0, make_rollback_dry_run.stderr
        rollback_validate = "scripts/validate_staging_supply_chain.py"
        rollback_delivery = "scripts/eks_delivery.py rollback"
        assert rollback_validate in make_rollback_dry_run.stdout
        assert rollback_delivery in make_rollback_dry_run.stdout
        assert (
            make_rollback_dry_run.stdout.index(rollback_validate)
            < make_rollback_dry_run.stdout.index(rollback_delivery)
        )
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
