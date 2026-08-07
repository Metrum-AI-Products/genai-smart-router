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
EKS_INGRESS_GUARD = ROOT / "deploy/kubernetes/bootstrap/tenant-router-ingress-guard.example.yaml"
LINKERD_INJECTION_PATCH = ROOT / "deploy/kubernetes/bootstrap/tenant-router-linkerd-injection-patch.example.yaml"
POLICY_ACTIVATOR = ROOT / "scripts/apply_tenant_network_policies.py"
INGRESS_RENDERER = ROOT / "scripts/render_tenant_ingress_network_policy.py"
DISCOVERY_SCRIPT = ROOT / "scripts/eks_discover.py"
BASE_NETWORK_POLICY = ROOT / "deploy/kubernetes/base/networkpolicy.yaml"
STAGING_NETWORK_POLICY_PATCH = ROOT / "deploy/kubernetes/overlays/metrum-staging/patch-networkpolicy.yaml"
STAGING_INGRESS_GUARD = ROOT / "deploy/kubernetes/overlays/metrum-staging/networkpolicy-ingress-guard.yaml"
STAGING_DEPLOYMENT_PATCH = ROOT / "deploy/kubernetes/overlays/metrum-staging/patch-deployment.yaml"
STAGING_KUSTOMIZATION = ROOT / "deploy/kubernetes/overlays/metrum-staging/kustomization.yaml"
MAKEFILE = ROOT / "Makefile"
PUBLIC_KUBERNETES_DOC = ROOT / "docs-site/docs/installation/kubernetes.md"
STAGING_RUNBOOK = ROOT / "docs/EKS_STAGING_MIGRATION.md"
STAGING_OVERLAY_README = ROOT / "deploy/kubernetes/overlays/metrum-staging/README.md"
IDENTITY_BOOTSTRAP = ROOT / "docs/EKS_IDENTITY_BOOTSTRAP.md"
DISCOVERY_NAMESPACE_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-discovery-namespace-rbac.example.yaml"
DISCOVERY_LINKERD_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-discovery-linkerd-namespace-rbac.example.yaml"
DISCOVERY_INGRESS_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-discovery-ingress-namespace-rbac.example.yaml"
STAGING_IDENTITY_STACK = ROOT / "deploy/aws/genai-smart-router-eks-staging-identity.yaml"
STAGING_DELIVERY_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-staging-delivery-rbac.yaml"
STAGING_DELIVERY_NAMESPACE_RBAC = (
    ROOT / "deploy/kubernetes/bootstrap/eks-staging-delivery-namespace-rbac.yaml"
)
STAGING_DELIVERY_ROLEBINDING = (
    ROOT / "deploy/kubernetes/bootstrap/eks-staging-delivery-rolebinding.yaml"
)
STAGING_BOOTSTRAP_RBAC = ROOT / "deploy/kubernetes/bootstrap/eks-staging-bootstrap-rbac.yaml"
STAGING_BOOTSTRAP_ADMISSION = (
    ROOT / "deploy/kubernetes/bootstrap/eks-staging-bootstrap-rbac-admission.yaml"
)
STAGING_DELIVERY_ADMISSION = (
    ROOT / "deploy/kubernetes/bootstrap/eks-staging-delivery-admission.yaml"
)


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
    if "    - Egress\n" not in base_network_policy or "    - Ingress\n" in base_network_policy or re.search(r"(?m)^  ingress:", base_network_policy):
        raise SystemExit("generic base NetworkPolicy must remain egress-only for non-EKS deployment")
    eks_ingress_guard = EKS_INGRESS_GUARD.read_text(encoding="utf-8")
    if "metadata:\n  name: smart-llmrouter-restrict-ingress\n" not in eks_ingress_guard or "    - Ingress\n" not in eks_ingress_guard or not re.search(r"(?m)^  ingress:\s*\[\]\s*$", eks_ingress_guard):
        raise SystemExit("EKS ingress guard must deny ingress until discovery renders an allow policy")
    staging_network_policy_patch = STAGING_NETWORK_POLICY_PATCH.read_text(encoding="utf-8")
    if "    - Egress\n" not in staging_network_policy_patch or "    - Ingress\n" in staging_network_policy_patch or re.search(r"(?m)^  ingress:", staging_network_policy_patch):
        raise SystemExit("staging egress NetworkPolicy patch must not create a generic ingress policy")
    staging_ingress_guard = STAGING_INGRESS_GUARD.read_text(encoding="utf-8")
    if "metadata:\n  name: smart-llmrouter-restrict-ingress\n" not in staging_ingress_guard or "    - Ingress\n" not in staging_ingress_guard or not re.search(r"(?m)^  ingress:\s*\[\]\s*$", staging_ingress_guard) or "ingress-nginx" in staging_ingress_guard:
        raise SystemExit("staging must include a fixed-free deny-ingress EKS guard")
    if "networkpolicy-ingress-guard.yaml" not in STAGING_KUSTOMIZATION.read_text(encoding="utf-8"):
        raise SystemExit("staging EKS overlay must include the deny-ingress guard")
    for path in (LINKERD_INJECTION_PATCH, STAGING_DEPLOYMENT_PATCH):
        if "linkerd.io/inject: enabled" not in path.read_text(encoding="utf-8"):
            raise SystemExit(f"{path.name} must retain durable Linkerd Pod-template injection")

    ingress_network_policy = INGRESS_NETWORK_POLICY.read_text(encoding="utf-8")
    if "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\n" not in ingress_network_policy:
        raise SystemExit("ingress NetworkPolicy template must use the standard NetworkPolicy API")
    if ingress_network_policy.count("__TENANT_NAMESPACE__") != 1 or ingress_network_policy.count("__INGRESS_NAMESPACE__") != 1:
        raise SystemExit("ingress NetworkPolicy must retain exactly the validated render placeholders")
    if "kubernetes.io/metadata.name: __INGRESS_NAMESPACE__" not in ingress_network_policy or "ingress-nginx" in ingress_network_policy:
        raise SystemExit("ingress NetworkPolicy template must derive its namespace from discovery")
    ingress_metadata, ingress_spec = ingress_network_policy.split("\nspec:\n", 1)
    delivery_label = "app.kubernetes.io/name: smart-llmrouter"
    if (
        delivery_label in ingress_metadata
        or delivery_label not in ingress_spec
        or ingress_network_policy.count(delivery_label) != 1
    ):
        raise SystemExit("discovery-owned ingress NetworkPolicy must keep the delivery label only in spec.podSelector")

    makefile = MAKEFILE.read_text(encoding="utf-8")
    if "eks-render-linkerd-policy: eks-render-ingress-network-policy" not in makefile or "scripts/render_tenant_ingress_network_policy.py" not in makefile:
        raise SystemExit("Linkerd policy rendering must require the discovery-derived ingress NetworkPolicy")
    if "eks-validate-tenant-network-policies" not in makefile or "eks-apply-tenant-network-policies" not in makefile or "scripts/apply_tenant_network_policies.py" not in makefile:
        raise SystemExit("tenant policy activation must use the selection-bound validation and apply targets")
    activator = POLICY_ACTIVATOR.read_text(encoding="utf-8")
    for required in ("--kubeconfig", "--context", "get-caller-identity", "describe-cluster", "--dry-run=server", "MAX_DISCOVERY_EVIDENCE_AGE_SECONDS", "EKS_INGRESS_GUARD_POLICY"):
        if required not in activator:
            raise SystemExit("tenant policy activation must bind explicit Kubernetes context to the discovered AWS target")
    ingress_renderer = INGRESS_RENDERER.read_text(encoding="utf-8")
    if "EKS_INGRESS_GUARD_POLICY" not in ingress_renderer or "router_workload_injection_verified" not in ingress_renderer or "ingress_workload_injection_verified" not in ingress_renderer:
        raise SystemExit("selected-ingress rendering must require the EKS guard and durable workload injection evidence")
    discovery_script = DISCOVERY_SCRIPT.read_text(encoding="utf-8")
    if "router_workload_injection_evidence" not in discovery_script or "ingress_workload_injection_evidence" not in discovery_script:
        raise SystemExit("Linkerd discovery must verify durable ingress and router injection before policy rendering")

    for path in (PUBLIC_KUBERNETES_DOC, STAGING_RUNBOOK, STAGING_OVERLAY_README, IDENTITY_BOOTSTRAP):
        deployment_path = path.read_text(encoding="utf-8")
        if "make eks-render-ingress-network-policy" not in deployment_path:
            raise SystemExit(f"{path.name} must document the non-Linkerd ingress policy render path")
        if "make eks-validate-tenant-network-policies" not in deployment_path or "make eks-apply-tenant-network-policies" not in deployment_path:
            raise SystemExit(f"{path.name} must document selection-bound rendered ingress policy activation")
        if "15 minutes" not in deployment_path:
            raise SystemExit(f"{path.name} must document the short discovery-evidence lifetime before policy activation")
        if "kubectl apply -f /secure/evidence/tenant-ingress-network-policy.yaml" in deployment_path:
            raise SystemExit(f"{path.name} must not apply the rendered ingress policy through an ambient kubectl context")

    for path, required_resources in (
        (DISCOVERY_NAMESPACE_RBAC, ("serviceaccounts", "networkpolicies", "deployments", "replicasets", "services", "persistentvolumeclaims", "ingresses", "pods")),
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
    if set(eks["Action"]) != {"eks:DescribeCluster", "eks:ListAccessEntries", "eks:ListNodegroups"} or eks["Resource"] == "*":
        raise SystemExit("discovery EKS access must be exactly the required reads on the approved cluster")
    ecr = next(statement for statement in discovery if statement["Sid"] == "EcrReadOnlyDiscovery")
    if set(ecr["Action"]) != {"ecr:DescribeRepositories", "ecr:GetRepositoryPolicy"} or ecr["Resource"] == "*":
        raise SystemExit("discovery ECR access must be exactly the required reads on the approved repository")

    staging_identity = STAGING_IDENTITY_STACK.read_text(encoding="utf-8")
    for required in (
        "RoleName: genai-smart-router-eks-staging-delivery",
        "RoleName: genai-smart-router-eks-staging-bootstrap",
        "RoleName: genai-smart-router-eks-staging-lifecycle-operator",
        "GroupName: genai-smart-router-eks-staging-lifecycle-operators",
        "AuthorizedOperatorRoleArn:",
        "AllowedPattern: ^arn:(aws|aws-us-gov|aws-cn):iam::[0-9]{12}:role/",
        "AWS: !Ref AuthorizedOperatorRoleArn",
        "Action: eks:DescribeCluster",
        "Action: ssm:GetParameter",
        "Name: /metrum/genai-smart-router/staging-delivery-target",
        "Type: AWS::EKS::AccessEntry",
        "- genai-smart-router-eks-staging-delivery",
        "- genai-smart-router-eks-staging-bootstrap",
    ):
        if required not in staging_identity:
            raise SystemExit(f"staging identity stack lacks required boundary: {required}")
    for required in (
        "Type: AWS::ECR::Repository",
        "RepositoryName: smart-llmrouter",
        "ImageTagMutability: IMMUTABLE",
        "ScanOnPush: true",
        "RoleName: genai-smart-router-eks-staging-image-publisher",
        "PolicyName: GenAISmartRouterEKSStagingImagePublisher",
        "Action: ecr:GetAuthorizationToken",
        "Resource: !GetAtt SmartRouterStagingImageRepository.Arn",
    ):
        if required not in staging_identity:
            raise SystemExit(f"staging image publisher lacks required boundary: {required}")
    access_entry_tags = re.search(
        r"StagingDeliveryAccessEntry:\n(?:.*\n)*?      Tags:\n"
        r"        - Key: application\n"
        r"          Value: genai-smart-router\n"
        r"        - Key: purpose\n"
        r"          Value: eks-staging-delivery\n"
        r"        - Key: environment\n"
        r"          Value: non-production\n",
        staging_identity,
    )
    if access_entry_tags is None:
        raise SystemExit(
            "staging delivery AccessEntry tags must use the CloudFormation tag array schema"
        )
    for forbidden_value in (
        "Action: \"*\"",
        "eks:AssociateAccessPolicy",
        "secretsmanager:",
        "iam:PassRole",
        "Default: smartrouter",
        "AWS::IAM::UserPolicy",
    ):
        if forbidden_value in staging_identity:
            raise SystemExit(f"staging identity stack contains forbidden authority: {forbidden_value}")
    if len(re.findall(r"^\s+Action: sts:AssumeRole$", staging_identity, re.MULTILINE)) != 10:
        raise SystemExit(
            "staging identity stack must grant the fixed group only entry to the "
            "lifecycle operator role and retain direct federated plus delegated "
            "target-role access"
        )
    lifecycle_operator_match = re.search(
        r"(?ms)^  StagingLifecycleOperatorRole:\n(?P<body>.*?)(?=^  \w|\Z)",
        staging_identity,
    )
    if lifecycle_operator_match is None:
        raise SystemExit("staging identity stack lacks the permanent lifecycle operator role")
    lifecycle_operator = lifecycle_operator_match.group("body")
    for required in (
        "arn:${AWS::Partition}:iam::${AWS::AccountId}:root",
        "aws:PrincipalArn:",
        "user/smart-router-lifecycle/*",
        "aws:PrincipalTag/GenAISmartRouterLifecycle: \"true\"",
        "Resource:",
        "role/genai-smart-router-eks-staging-delivery",
        "role/genai-smart-router-eks-staging-bootstrap",
        "role/genai-smart-router-eks-staging-image-publisher",
    ):
        if required not in lifecycle_operator:
            raise SystemExit(f"lifecycle operator role lacks required boundary: {required}")
    if any(value in lifecycle_operator for value in ("secretsmanager:", "eks:", "ecr:", 'Resource: "*"')):
        raise SystemExit("lifecycle operator role may only delegate to the reviewed target roles")
    lifecycle_group_match = re.search(
        r"(?ms)^  LifecycleOperatorGroup:\n(?P<body>.*?)(?=^  \w|\Z)",
        staging_identity,
    )
    for target_name in ("StagingDeliveryRole", "StagingBootstrapRole", "StagingImagePublisherRole"):
        target_match = re.search(
            rf"(?ms)^  {target_name}:\n(?P<body>.*?)(?=^  \w|\Z)",
            staging_identity,
        )
        if target_match is None:
            raise SystemExit(f"staging identity stack lacks {target_name}")
        target_role = target_match.group("body")
        if (
            "DependsOn: StagingLifecycleOperatorRole" not in target_role
            or "AWS: !Ref AuthorizedOperatorRoleArn" not in target_role
            or "role/genai-smart-router-eks-staging-lifecycle-operator" not in target_role
            or "!GetAtt StagingLifecycleOperatorRole.Arn" in target_role
            or "arn:${AWS::Partition}:iam::${AWS::AccountId}:root" in target_role
        ):
            raise SystemExit(
                f"{target_name} must trust only the exact federated source and "
                "the fixed lifecycle operator role without a dependency cycle"
            )
    if lifecycle_group_match is None:
        raise SystemExit("staging identity stack lacks the permanent lifecycle operator group")
    lifecycle_group = lifecycle_group_match.group("body")
    if (
        "DependsOn: StagingLifecycleOperatorRole" not in lifecycle_group
        or "role/genai-smart-router-eks-staging-lifecycle-operator" not in lifecycle_group
        or "!GetAtt StagingLifecycleOperatorRole.Arn" in lifecycle_group
        or any(value in lifecycle_group for value in ("Action: iam:", "Action: eks:", "Action: ecr:", "Action: secretsmanager:", 'Resource: "*"'))
    ):
        raise SystemExit("lifecycle operator group may only enter the lifecycle operator role without a dependency cycle")
    if staging_identity.count('Resource: "*"') != 1 or "Action: ecr:GetAuthorizationToken" not in staging_identity:
        raise SystemExit("only the required ECR authorization token action may use wildcard resource scope")
    bootstrap_role_match = re.search(
        r"(?ms)^  StagingBootstrapRole:\n(?P<body>.*?)(?=^  \w|\Z)",
        staging_identity,
    )
    if bootstrap_role_match is None:
        raise SystemExit("staging identity stack lacks the reusable bootstrap role")
    bootstrap_role = bootstrap_role_match.group("body")
    if bootstrap_role.count("Action: eks:DescribeCluster") != 1 or any(
        value in bootstrap_role
        for value in ("ssm:", "ecr:", "eks:AssociateAccessPolicy", 'Resource: "*"')
    ):
        raise SystemExit("staging bootstrap role must only describe the approved cluster")


    staging_bootstrap_rbac = STAGING_BOOTSTRAP_RBAC.read_text(encoding="utf-8")
    for required in (
        "kind: Role\n",
        "kind: RoleBinding\n",
        "namespace: smart-llmrouter-staging",
        "kind: Group\n",
        "name: genai-smart-router-eks-staging-bootstrap",
        'resources: ["roles"]',
        'resources: ["rolebindings"]',
        'resourceNames: ["genai-smart-router-eks-staging-delivery"]',
        'verbs: ["bind", "escalate", "get", "patch", "update"]',
    ):
        if required not in staging_bootstrap_rbac:
            raise SystemExit(f"staging bootstrap RBAC lacks required boundary: {required}")
    for forbidden_value in (
        '"secrets"',
        '"deployments"',
        "pods/log",
        "pods/exec",
        "kind: ClusterRole",
        "kind: ClusterRoleBinding",
        'verbs: ["*"]',
        '"create"',
        '"delete"',
        "deletecollection",
    ):
        if forbidden_value in staging_bootstrap_rbac:
            raise SystemExit(f"staging bootstrap RBAC contains forbidden authority: {forbidden_value}")

    staging_bootstrap_admission = STAGING_BOOTSTRAP_ADMISSION.read_text(encoding="utf-8")
    for required in (
        "kind: ValidatingAdmissionPolicy\n",
        "kind: ValidatingAdmissionPolicyBinding\n",
        "failurePolicy: Fail",
        "'genai-smart-router-eks-staging-bootstrap' in request.userInfo.groups",
        "object.rules.size() == 7",
        "object.subjects.size() == 1",
        "The bootstrap role may reconcile only the exact reviewed staging delivery RBAC objects.",
        "validationActions: [Deny, Audit]",
        "resources: [\"roles\", \"rolebindings\"]",
        "kubernetes.io/metadata.name: smart-llmrouter-staging",
    ):
        if required not in staging_bootstrap_admission:
            raise SystemExit(f"staging bootstrap admission lacks required boundary: {required}")
    for forbidden_value in ('resources: ["*"]', 'operations: ["*"]', "failurePolicy: Ignore"):
        if forbidden_value in staging_bootstrap_admission:
            raise SystemExit(f"staging bootstrap admission contains forbidden boundary: {forbidden_value}")

    staging_delivery_namespace_rbac = (
        STAGING_DELIVERY_NAMESPACE_RBAC.read_text(encoding="utf-8")
        + STAGING_DELIVERY_ROLEBINDING.read_text(encoding="utf-8")
    )
    for required in (
        "kind: Role\n",
        "kind: RoleBinding\n",
        "namespace: smart-llmrouter-staging",
        "kind: Group\n",
        "name: genai-smart-router-eks-staging-delivery",
        'resourceNames: ["smartrouter-staging-runtime-attestation"]',
        'resources: ["deployments"]',
        'resources: ["replicasets"]',
        'resources: ["ingresses", "networkpolicies"]',
        'resources: ["poddisruptionbudgets"]',
        'resources: ["services", "serviceaccounts", "persistentvolumeclaims"]',
    ):
        if required not in staging_delivery_namespace_rbac:
            raise SystemExit(f"staging delivery namespace RBAC lacks required boundary: {required}")
    if "kind: ClusterRole" in staging_delivery_namespace_rbac or "kind: ClusterRoleBinding" in staging_delivery_namespace_rbac:
        raise SystemExit("scoped recovery manifest must not include cluster-scoped RBAC")
    configmap_rule = re.search(
        r'  - apiGroups: \[""\]\n    resources: \["configmaps"\]\n'
        r'    resourceNames: \["smartrouter-staging-runtime-attestation"\]\n'
        r'    verbs: \["get"\]\n',
        staging_delivery_namespace_rbac,
    )
    if configmap_rule is None or staging_delivery_namespace_rbac.count('resources: ["configmaps"]') != 1:
        raise SystemExit("staging delivery namespace RBAC must permit only named attestation ConfigMap get")

    staging_delivery_rbac = STAGING_DELIVERY_RBAC.read_text(encoding="utf-8")
    for required in (
        "kind: ClusterRole\n",
        "kind: ClusterRoleBinding\n",
        "genai-smart-router-eks-staging-bootstrap-rbac",
        "genai-smart-router-eks-staging-linkerd-pod",
        'resources: ["validatingadmissionpolicies"]',
        'resources: ["validatingadmissionpolicybindings"]',
    ):
        if required not in staging_delivery_rbac:
            raise SystemExit(f"staging delivery admission-read RBAC lacks required boundary: {required}")
    if "kind: Role\n" in staging_delivery_rbac or "kind: RoleBinding\n" in staging_delivery_rbac:
        raise SystemExit("delivery admission-read RBAC must not include namespace recovery objects")
    for forbidden_value in ('"secrets"', "pods/log", "pods/exec", 'verbs: ["*"]', '"delete"', "deletecollection"):
        if forbidden_value in staging_delivery_rbac:
            raise SystemExit(f"staging delivery admission-read RBAC contains forbidden authority: {forbidden_value}")

    staging_delivery_admission = STAGING_DELIVERY_ADMISSION.read_text(encoding="utf-8")
    for required in (
        "kind: ValidatingAdmissionPolicy\n",
        "kind: ValidatingAdmissionPolicyBinding\n",
        "failurePolicy: Fail",
        "  validations:\n",
        "kind: ConfigMap",
        "object.metadata.name == 'smart-llmrouter'",
        "'genai-smart-router-eks-staging-delivery' in request.userInfo.groups",
        "kubernetes.io/metadata.name: smart-llmrouter-staging",
        "c.image == params.data.approved_router_image",
        "c.image == params.data.approved_linkerd_proxy_image",
        "c.image == params.data.approved_linkerd_init_image",
        "capabilities.add == ['NET_ADMIN', 'NET_RAW']",
        "validationActions: [Deny, Audit]",
        "name: smartrouter-staging-runtime-attestation",
        "namespace: smart-llmrouter-staging",
        "parameterNotFoundAction: Deny",
    ):
        if required not in staging_delivery_admission:
            raise SystemExit(
                f"staging delivery admission policy lacks required boundary: {required}"
            )
    if "name: resource-name" in staging_delivery_admission:
        raise SystemExit(
            "delivery admission must evaluate every delivery-role Deployment; "
            "the exact-name validation denies alternate workload names"
        )
    for forbidden_value in ('resources: ["*"]', 'operations: ["*"]', "failurePolicy: Ignore"):
        if forbidden_value in staging_delivery_admission:
            raise SystemExit(
                f"staging delivery admission policy contains forbidden boundary: {forbidden_value}"
            )
    if staging_delivery_admission.count("kind: ValidatingAdmissionPolicy\n") != 2:
        raise SystemExit("staging delivery admission policies must protect Deployment and injected Pod stages")
    if staging_delivery_admission.count("kind: ValidatingAdmissionPolicyBinding\n") != 2:
        raise SystemExit("staging delivery admission policy bindings must be unique")

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
