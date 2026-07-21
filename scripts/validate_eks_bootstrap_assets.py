#!/usr/bin/env python3
"""Static safeguards for the namespace-scoped EKS bootstrap examples."""

from __future__ import annotations

import json
import re
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
RBAC = ROOT / "deploy/kubernetes/bootstrap/tenant-provisioner-rbac.yaml"
TRUST = ROOT / "deploy/aws/github-oidc-trust-policy.example.json"
DISCOVERY_ROLE = ROOT / "deploy/aws/genai-smart-router-eks-discovery-role.example.json"
LINKERD_POLICY = ROOT / "deploy/kubernetes/bootstrap/tenant-linkerd-policy.example.yaml"
INGRESS_NETWORK_POLICY = ROOT / "deploy/kubernetes/bootstrap/tenant-ingress-network-policy.example.yaml"
BASE_NETWORK_POLICY = ROOT / "deploy/kubernetes/base/networkpolicy.yaml"
STAGING_NETWORK_POLICY_PATCH = ROOT / "deploy/kubernetes/overlays/metrum-staging/patch-networkpolicy.yaml"
MAKEFILE = ROOT / "Makefile"
PUBLIC_KUBERNETES_DOC = ROOT / "docs-site/docs/installation/kubernetes.md"
STAGING_RUNBOOK = ROOT / "docs/EKS_STAGING_MIGRATION.md"
STAGING_OVERLAY_README = ROOT / "deploy/kubernetes/overlays/metrum-staging/README.md"
DISCOVERY_NAMESPACE_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-discovery-namespace-rbac.example.yaml"
DISCOVERY_LINKERD_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-discovery-linkerd-namespace-rbac.example.yaml"
DISCOVERY_INGRESS_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-discovery-ingress-namespace-rbac.example.yaml"


def main() -> int:
    content = RBAC.read_text(encoding="utf-8")
    forbidden = ("resources: [\"secrets\"]", "resources: [\"deployments\"]", "resources: [\"serviceaccounts\"]", "kind: ClusterRole", "kind: ClusterRoleBinding", "pods/exec", "verbs: [\"*\"]")
    for value in forbidden:
        if value in content:
            raise SystemExit(f"unsafe tenant provisioner permission: {value}")
    if "apiGroups: [\"policy.linkerd.io\"]" not in content or "serverauthorizations" not in content:
        raise SystemExit("tenant provisioner must retain namespace-scoped Linkerd policy support")

    linkerd = LINKERD_POLICY.read_text(encoding="utf-8")
    if "kind: Server\n" not in linkerd or "apiVersion: policy.linkerd.io/v1beta3\nkind: Server\n" not in linkerd:
        raise SystemExit("Linkerd Server template must use the served v1beta3 API")
    if "apiVersion: policy.linkerd.io/v1beta1\nkind: ServerAuthorization\n" not in linkerd:
        raise SystemExit("Linkerd ServerAuthorization template must use the separately served v1beta1 API")
    if linkerd.count("__TENANT_NAMESPACE__") != 2 or linkerd.count("__LINKERD_INGRESS_IDENTITY__") != 1:
        raise SystemExit("Linkerd policy must retain exactly the validated render placeholders")
    if "ingress-nginx.ingress-nginx.serviceaccount.identity.linkerd.cluster.local" in linkerd:
        raise SystemExit("Linkerd policy must not retain a fixed ingress identity")

    base_network_policy = BASE_NETWORK_POLICY.read_text(encoding="utf-8")
    if len(re.findall(r"(?m)^  ingress:", base_network_policy)) != 1 or not re.search(r"(?m)^  ingress:\s*\[\]\s*$", base_network_policy):
        raise SystemExit("base NetworkPolicy must deny ingress until discovery renders an allow policy")
    staging_network_policy_patch = STAGING_NETWORK_POLICY_PATCH.read_text(encoding="utf-8")
    if re.search(r"(?m)^  ingress:", staging_network_policy_patch) or "ingress-nginx" in staging_network_policy_patch:
        raise SystemExit("staging NetworkPolicy patch must not restore a fixed ingress namespace")

    ingress_network_policy = INGRESS_NETWORK_POLICY.read_text(encoding="utf-8")
    if "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\n" not in ingress_network_policy:
        raise SystemExit("ingress NetworkPolicy template must use the standard NetworkPolicy API")
    if ingress_network_policy.count("__TENANT_NAMESPACE__") != 1 or ingress_network_policy.count("__INGRESS_NAMESPACE__") != 1:
        raise SystemExit("ingress NetworkPolicy must retain exactly the validated render placeholders")
    if "kubernetes.io/metadata.name: __INGRESS_NAMESPACE__" not in ingress_network_policy or "ingress-nginx" in ingress_network_policy:
        raise SystemExit("ingress NetworkPolicy template must derive its namespace from discovery")

    makefile = MAKEFILE.read_text(encoding="utf-8")
    if "eks-render-linkerd-policy: eks-render-ingress-network-policy" not in makefile or "scripts/render_tenant_ingress_network_policy.py" not in makefile:
        raise SystemExit("Linkerd policy rendering must require the discovery-derived ingress NetworkPolicy")

    for path in (PUBLIC_KUBERNETES_DOC, STAGING_RUNBOOK, STAGING_OVERLAY_README):
        deployment_path = path.read_text(encoding="utf-8")
        if "make eks-render-ingress-network-policy" not in deployment_path:
            raise SystemExit(f"{path.name} must document the non-Linkerd ingress policy render path")
        if "kubectl apply -f /secure/evidence/tenant-ingress-network-policy.yaml" not in deployment_path:
            raise SystemExit(f"{path.name} must document applying the rendered ingress policy")

    for path, required_resources in (
        (DISCOVERY_NAMESPACE_RBAC, ("serviceaccounts", "networkpolicies", "deployments", "services", "persistentvolumeclaims", "ingresses", "pods")),
        (DISCOVERY_LINKERD_RBAC, ("serviceaccounts",)),
        (DISCOVERY_INGRESS_RBAC, ("serviceaccounts", "deployments", "replicasets", "pods")),
    ):
        content = path.read_text(encoding="utf-8")
        if "kind: Role\n" not in content or "kind: RoleBinding\n" not in content or "kind: Group\n" not in content:
            raise SystemExit(f"{path.name} must bind an EKS Kubernetes group with namespace-scoped RBAC")
        if any(value in content for value in ('verbs: [\"*\"]', '\"secrets\"', '\"configmaps\"')):
            raise SystemExit(f"{path.name} must not grant broad or sensitive reads")
        if any(resource not in content for resource in required_resources):
            raise SystemExit(f"{path.name} lacks a required discovery read")

    discovery = json.loads(DISCOVERY_ROLE.read_text(encoding="utf-8"))["InlinePolicy"]["Statement"]
    eks = next(statement for statement in discovery if statement["Sid"] == "EksReadOnlyDiscovery")
    if "eks:ListClusters" in eks["Action"] or eks["Resource"] == "*":
        raise SystemExit("discovery EKS access must be scoped to the approved cluster")
    ecr = next(statement for statement in discovery if statement["Sid"] == "EcrReadOnlyDiscovery")
    if ecr["Resource"] == "*":
        raise SystemExit("discovery ECR access must be scoped to the approved repository")

    policy = json.loads(TRUST.read_text(encoding="utf-8"))
    condition = policy["Statement"][0]["Condition"]["StringEquals"]
    subject = condition.get("token.actions.githubusercontent.com:sub", "")
    if not subject.startswith("repo:<OWNER>/<REPOSITORY>:environment:") or "*" in subject:
        raise SystemExit("OIDC subject must be an exact repository environment subject")
    if condition.get("token.actions.githubusercontent.com:aud") != "sts.amazonaws.com":
        raise SystemExit("OIDC audience must be sts.amazonaws.com")
    print("EKS bootstrap asset safeguards passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
