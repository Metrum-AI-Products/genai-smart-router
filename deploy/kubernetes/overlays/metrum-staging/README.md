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

The same bootstrap identity must also own the policy-pinned, non-secret
`smartrouter-staging-runtime-attestation` ConfigMap. Before changing the
runtime Secret, delete the old attestation. After the Secret write, create a
fresh immutable ConfigMap with only `schema_version: v1`, `secret_name`,
`secret_uid`, and `secret_resource_version` in `data`, no `binaryData`, and
one same-namespace `v1` `Secret` owner reference matching the attested name
and UID. The delivery role reads that exact ConfigMap but has no Secret verbs
or other ConfigMap verbs. Do not put the attestation ConfigMap in this
Kustomize overlay: it is bootstrap-owned evidence, not workload desired state.

The RDS DSN must use TLS hostname verification and the mounted CA file, for
example `sslmode=verify-full sslrootcert=/app/config/rds-ca.pem`. Confirm the
platform wildcard certificate is available in this namespace as
`apps-metrum-ai-wildcard-tls`, or update the overlay to the platform-provided
secret name before applying.

Render, dry-run, apply, and rollback must use the root Make contract rather
than an implicit kubectl context. The approved account, region, ECR repository,
cluster, namespace, runtime Secret attestation ConfigMap, overlay,
`image_architecture`, Deployment, container, and role are pinned in
`deploy/aws/genai-smart-router-eks-staging-target.json` and must exactly match
the separately protected AWS Systems Manager Parameter named by that file.
The checked-in repository URI is only a bootstrap default: it does not prove a
live AWS/EKS approval. Reconcile the reviewed policy and protected Parameter
with a renewed least-privilege non-root session before delivery. The delivery
role may only read that one parameter; it cannot update it. With an approved
short-lived AWS identity:

```bash
make eks-preflight eks-plan \
  EKS_DELIVERY_AWS_PROFILE='genai-smart-router-eks-staging-delivery' \
  IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>'

make eks-apply-staging EKS_CONFIRM=STAGING_APPLY \
  EKS_DELIVERY_AWS_PROFILE='genai-smart-router-eks-staging-delivery' \
  IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>' \
  EKS_SUPPLY_CHAIN_DIR='tmp/eks-supply-chain'
```

The supply-chain verifier obtains the expected image architecture only from
`image_architecture` in the reviewed checked-in target policy and requires the
evidence to bind it exactly. Delivery then requires the complete policy's
canonical hash to match the protected Parameter before reconciliation. Do not
supply `EKS_IMAGE_ARCHITECTURE`, or an equivalent CI input; architecture is not
a caller-selected deployment property.

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
`tmp/eks-evidence/`; raw manifests are temporary kubectl inputs and evidence
records only their safe checksum/size. It creates a temporary kubeconfig,
rejects mutable or off-repository images (including rollback), and permits
mutation only for its fixed staging target with
`EKS_CONFIRM=STAGING_APPLY`. Production apply is intentionally unavailable.
Rollback has the same `eks-supply-chain-validate` prerequisite as apply, so
its protected evidence must bind the requested historical image digest and the
target-policy architecture before Kubernetes mutation can begin.
The overlay deliberately omits the base `Namespace` resource: namespace
creation and labels are an independently reviewed bootstrap action, and the
delivery contract rejects cluster-scoped resources or any rendered resource
outside the approved namespace.

The contract also derives a canonical identity and normalized configuration
fingerprint from the allowlisted, namespace-scoped resources bearing
`app.kubernetes.io/name=smart-llmrouter`. It verifies both before smoke and
promotion, and refuses an apply before mutation if an old label-selected
resource is no longer rendered. Kubernetes-owned runtime fields such as a
Service cluster IP and PVC binding name are excluded; routing/security specs,
labels, annotations, owner references, and finalizers are not, except the
Kubernetes-owned `volume.kubernetes.io/selected-node` annotation and
`kubernetes.io/pvc-protection` finalizer added to this overlay's
`WaitForFirstConsumer` PVC.
The separately authorized discovery workflow owns the exact
`NetworkPolicy/smart-llmrouter-discovered-ingress` companion policy. It keeps
the router label only in `spec.podSelector`, never in metadata, so the delivery
inventory does not select it. Delivery has no name-based exclusion: every
label-selected object remains fail-closed. Re-render and activate any legacy
companion policy that still carries the delivery label before its next delivery
run; the delivery contract deliberately reports that legacy object as stale.
It deliberately does **not** use Kubernetes prune: removal or renaming of a
Service, Ingress, NetworkPolicy, PVC, PodDisruptionBudget, ServiceAccount, or
Deployment requires a separately reviewed recovery/migration to remove the
stale object. The delivery role therefore needs namespace-scoped `list` access
to only those seven resource types, in addition to the existing rollout and
pod read permissions. Rollback also requires `list` access to `replicasets`:
the contract resolves the requested digest to an owned prior revision before
calling `rollout undo --to-revision`, so it never implicitly selects an
unverified immediately previous revision.

The protected policy/SSM match and reviewed change record authorize a
reconciliation; `EKS_CONFIRM=STAGING_APPLY` is only an explicit operator
acknowledgement of the mutation. Its safe before-apply configuration fingerprint
is recorded, then passed evidence requires the live configuration after apply,
smoke, and promotion to match the reviewed manifest. Investigate unexpected
pre-apply differences through change control; no caller-controlled bypass is
provided.

See `docs/EKS_STAGING_MIGRATION.md` for the full RDS, license, validation, and
rollback runbook. See `docs/EKS_STAGING_CICD.md` for the required immutable
image supply-chain evidence and protected GitHub staging-environment boundary.
