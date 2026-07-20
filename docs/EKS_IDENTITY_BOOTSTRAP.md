# EKS Identity, Discovery, And Bootstrap

This internal runbook establishes the safe prerequisites for EKS delivery. It
does not deploy the router, modify AWS, or replace the validation-only staging
overlay. The shared operator and CI command wrapper is tracked separately; this
document defines the guarantees it must preserve.

## Reauthenticate And Discover

Use an authorized short-lived federated AWS session. Do not copy credentials or
fall back to static keys. Supply every selection explicitly; the discovery
primitive never selects a current kube context, first account, or first cluster.

```bash
python3 scripts/eks_discover.py \
  --account-id 123456789012 \
  --region us-east-1 \
  --cluster approved-cluster \
  --namespace approved-namespace \
  --ecr-repository approved-router-repository \
  --output /secure/evidence/eks-discovery.json
```

The command creates a temporary kubeconfig and removes it on exit. It reads no
Kubernetes Secrets or ConfigMap payloads, and writes a machine-readable report
containing only safe names, booleans, versions, labels needed for Linkerd
injection, and policy presence. An expired session, wrong account, missing
namespace RBAC, unavailable Linkerd API, inaccessible ECR, or missing EKS
access fails before producing a success report. Keep evidence outside Git.

Review the report before bootstrap: EKS version/auth mode/access entries,
endpoint exposure, audit logging, ingress/storage names, namespace labels and
network policies, router resource names, ECR immutability/encryption/policy
presence, and Linkerd control-plane/policy API/identity service-account
inventory. It deliberately excludes endpoint values, trust-root bodies,
certificates, policy/config payloads, DSNs, and secrets.

## Identity Boundaries

Create distinct short-lived identities for read-only discovery, ECR build/push,
staging reconciliation, production promotion, and the router workload. GitHub
OIDC trust must pin repository, branch, and environment subject claims; reject
other repositories, tags, pull requests, branches, and environments. Production
uses a distinct environment-gated role. Prefer EKS access entries plus
namespace-scoped RBAC; document a dated migration plan if `aws-auth` is still
required.

The router workload uses EKS Pod Identity or IRSA only after discovery confirms
cluster support. Its AWS policy may read only approved secret references and
write only required CloudWatch telemetry. It must not inherit node credentials.
Break-glass access uses a separately audited, MFA/federated, time-bounded role;
it is never a CI role.

## Tenant And Linkerd Bootstrap

`deploy/kubernetes/bootstrap/tenant-provisioner-rbac.yaml` and
`tenant-bootstrap-resources.example.yaml` are reviewed per-namespace templates.
A platform administrator binds/applies them only in an approved tenant namespace.
The provisioner cannot read Secrets, bind roles, operate in another namespace,
or modify cluster-wide Linkerd policy. Verify with `kubectl auth can-i` for
allowed resources and explicit denials for `secrets`, `clusterroles`, other
namespaces, and `pods/exec`.

Before using `tenant-linkerd-policy.example.yaml`, verify the discovered
Linkerd control plane, `policy.linkerd.io` CRDs/version, namespace injection
labels, and trust/identity readiness. The template is intentionally an example:
confirm its Server/ServerAuthorization API version and ingress identity against
the cluster; do not infer either from this repository.

## Idempotence, Drift, And Rollback

Render assets, server-side dry-run them, then apply only after review. Record
safe evidence: actor/workflow, selected account/region/cluster/namespace,
policy version/digest, resource names, validation result, and Linkerd injection
and policy API state. Do not record resource payloads, secrets, endpoint values,
or configuration contents. Detect RBAC, service-account, NetworkPolicy,
ResourceQuota/LimitRange, and Linkerd Server/authorization drift; require human
review before overwriting security-sensitive drift.

Rollback removes only the reviewed namespace-scoped resources or restores their
previous reviewed manifests. Do not delete a tenant namespace, Linkerd control
plane resource, Secret, PVC, or shared ingress/certificate dependency as part of
bootstrap rollback.
