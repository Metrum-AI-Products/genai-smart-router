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

Before applying it, create the `smartrouter-staging-runtime` Secret in the
`smart-llmrouter-staging` namespace from ignored local files. The mounted
`config.yaml` must be derived from the live production config but must use a
dedicated staging caller, the EKS-bound license, `/app/state` paths, and the
new RDS DSN. Keep all files and the DSN outside Git.

```bash
kubectl create namespace smart-llmrouter-staging --dry-run=client -o yaml | kubectl apply -f -
kubectl -n smart-llmrouter-staging create secret generic smartrouter-staging-runtime \
  --from-file=config.yaml=/secure/staging/config.yaml \
  --from-file=env.json=/secure/staging/env.json \
  --from-file=license.json=/secure/staging/license.json \
  --from-file=rds-ca.pem=/secure/staging/rds-ca.pem \
  --from-file=router.ts=/secure/staging/router.ts \
  --from-literal=ROUTER_USAGE_DB_DSN="$(cat /secure/staging/rds-dsn)"
```

The RDS DSN must use TLS hostname verification and the mounted CA file, for
example `sslmode=verify-full sslrootcert=/app/config/rds-ca.pem`. Confirm the
platform wildcard certificate is available in this namespace as
`apps-metrum-ai-wildcard-tls`, or update the overlay to the platform-provided
secret name before applying.

Render and validate before deployment:

```bash
kubectl kustomize deploy/kubernetes/overlays/metrum-staging >/tmp/smartrouter-staging.yaml
kubectl apply --dry-run=server -f /tmp/smartrouter-staging.yaml
kubectl apply -f /tmp/smartrouter-staging.yaml
```

The overlay's base NetworkPolicy intentionally denies ingress at this point.
After the router Deployment is Ready, rerun the approved staging discovery and
render the companion policy from that evidence; it is not checked into this
overlay because the ingress namespace is deployment-specific. Activate it only
through the selection-bound Make target, which verifies the discovery account
and EKS endpoint against the named deployment profile and explicit kubeconfig/
context rather than an ambient `kubectl` context. For Linkerd staging, it
dry-runs both artifacts, then applies the Linkerd policy before the namespace
allow policy. Discovery evidence is valid for 15 minutes only; rerun it if the
window expires before activation:

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

See `docs/EKS_STAGING_MIGRATION.md` for the full RDS, license, validation, and
rollback runbook.
