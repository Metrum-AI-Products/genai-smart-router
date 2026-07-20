# Metrum EKS Staging Overlay

This is an internal Metrum-managed deployment overlay, not a product default or
release-package deployment contract. It deploys the validation-only router at
`smartrouter.apps.metrum.ai`. It does not move production traffic, import the
EC2 usage database, or expose ordinary production caller keys.

Use `deploy/kubernetes/base/` and `deploy/kubernetes/overlays/example/` for
generic product deployments. Copy and adapt an overlay for each customer or
managed environment; do not treat this hostname, ingress class, StorageClass,
RDS topology, ECR path, or caller policy as product defaults.

Before applying the namespace-scoped overlay, a cluster administrator must
create the deployment-specific `smartrouter-gp3` StorageClass from
`storageclass.yaml`. It uses the Metrum cluster's EKS Auto Mode EBS CSI driver;
other deployments must select their own storage provisioner and class.

Before applying it, use the separately approved namespace/Secret bootstrap
procedure to create `smartrouter-staging-runtime` from ignored secure files.
That privileged bootstrap is deliberately outside this Make delivery contract;
never place DSNs, token values, provider keys, or runtime Secret creation
commands in this repository or shell history. The mounted `config.yaml` must
use a dedicated staging caller, the EKS-bound license, `/app/state` paths, and
the new RDS DSN.

The RDS DSN must use TLS hostname verification and the mounted CA file, for
example `sslmode=verify-full sslrootcert=/app/config/rds-ca.pem`. Confirm the
platform wildcard certificate is available in this namespace as
`apps-metrum-ai-wildcard-tls`, or update the overlay to the platform-provided
secret name before applying.

Render, dry-run, apply, and rollback must use the root Make contract rather
than an implicit kubectl context. With an approved short-lived AWS identity:

```bash
make eks-preflight eks-plan \
  AWS_REGION='<approved-region>' EKS_CLUSTER='<approved-cluster>' \
  K8S_NAMESPACE='smart-llmrouter-staging' \
  KUSTOMIZE_OVERLAY='deploy/kubernetes/overlays/metrum-staging' \
  ENVIRONMENT='staging' \
  IMAGE_DIGEST='registry.example/smart-llmrouter@sha256:<64-hex>'

make eks-apply-staging EKS_CONFIRM=STAGING_APPLY \
  AWS_REGION='<approved-region>' EKS_CLUSTER='<approved-cluster>' \
  K8S_NAMESPACE='smart-llmrouter-staging' \
  KUSTOMIZE_OVERLAY='deploy/kubernetes/overlays/metrum-staging' \
  ENVIRONMENT='staging' \
  IMAGE_DIGEST='registry.example/smart-llmrouter@sha256:<64-hex>'
```

The overlay's `networkpolicy-ingress-guard.yaml` intentionally denies ingress
at this point; the generic base policy remains ingress-neutral for non-EKS
deployments.
After the router Deployment is Ready, rerun the approved staging discovery and
render the companion policy from that evidence; it is not checked into this
overlay because the ingress namespace is deployment-specific. Activate it only
through the selection-bound Make target, which verifies the discovery account
and EKS endpoint against the named deployment profile and explicit kubeconfig/
context rather than an ambient `kubectl` context. It snapshots that kubeconfig
and the exact renderer-generated policy bytes privately before validation, so
it never applies a caller artifact after that path changes. For Linkerd staging, it
dry-runs both artifacts, then applies the Linkerd policy before the namespace
allow policy. The staging Deployment template explicitly requests Linkerd
injection so future rollouts remain meshed; do not remove that annotation while
Linkerd policy is selected. Discovery evidence is valid for 15 minutes only;
rerun it if the window expires before activation:

```bash
make eks-render-linkerd-policy \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml

make eks-validate-tenant-network-policies \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_POLICY_AWS_PROFILE=<approved-staging-deployment-profile> \
  EKS_POLICY_KUBECONFIG=/secure/kubeconfigs/approved-staging.yaml \
  EKS_POLICY_CONTEXT=<approved-staging-context> \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml

make eks-apply-tenant-network-policies \
  EKS_POLICY_APPLY_CONFIRM=apply \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_POLICY_AWS_PROFILE=<approved-staging-deployment-profile> \
  EKS_POLICY_KUBECONFIG=/secure/kubeconfigs/approved-staging.yaml \
  EKS_POLICY_CONTEXT=<approved-staging-context> \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml
```

If Linkerd is intentionally not selected, use
`make eks-render-ingress-network-policy`, then the same validate/apply targets
without `EKS_LINKERD_POLICY_OUTPUT`.

The Make contract writes only scrubbed JSON and Markdown evidence below
`tmp/eks-evidence/`, creates a temporary kubeconfig, rejects mutable images,
and permits mutation only for `ENVIRONMENT=staging` with
`EKS_CONFIRM=STAGING_APPLY`. Production apply is intentionally unavailable.

See `docs/EKS_STAGING_MIGRATION.md` for the full RDS, license, validation, and
rollback runbook.
