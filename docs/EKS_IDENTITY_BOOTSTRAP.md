# EKS Identity, Discovery, And Bootstrap

This internal runbook establishes the safe prerequisites for EKS delivery. It
does not deploy the router, modify AWS, or replace the validation-only staging
overlay. The shared operator and CI command wrapper is tracked separately; this
document defines the guarantees it must preserve.

## Reauthenticate And Discover

Use an authorized short-lived federated AWS session. Do not copy credentials or
fall back to static keys. Supply every selection explicitly; the discovery
primitive never selects a current kube context, first account, or first cluster.

### Local Discovery Profile

Create the `genai-smart-router-eks-discovery` role from
`deploy/aws/genai-smart-router-eks-discovery-role.example.json` through
reviewed infrastructure-as-code. Its trust must name the approved MFA-protected
non-root operator explicitly. The current approved local identity name is
`smartrouter`; do not use an email address or account-root ARN as an IAM
principal. Account-root credentials cannot assume an AWS role and must never
be retained as a discovery source profile. The discovery policy is read-only:
it cannot create EKS resources, change an access entry, or create a Kubernetes
namespace.

After the platform administrator creates the `smartrouter` console login,
enrolls MFA, and grants only `sts:AssumeRole`, use a bounded local session; do
not retain a long-lived access key:

```ini
[profile smartrouter]
region = <approved-region>

[profile genai-smart-router-eks-discovery]
role_arn = arn:aws:iam::<ACCOUNT_ID>:role/genai-smart-router-eks-discovery
source_profile = smartrouter
region = <approved-region>
```

Run the canonical bootstrap target. Before it creates a one-time source key,
it atomically reserves the mode-0600 recovery-record path. It then exchanges
the key with the macOS Keychain MFA seed for a one-hour STS session, verifies
the discovery-role identity, and deletes the source key even when bootstrap
fails:

```bash
make eks-session-bootstrap \
  EKS_ACCOUNT_ID=123456789012 \
  EKS_REGION=us-east-1 \
  EKS_MFA_SERIAL=arn:aws:iam::123456789012:mfa/smartrouter \
  EKS_MFA_KEYCHAIN_SERVICE=your-local-mfa-keychain-service \
  EKS_CLEANUP_RECORD=/secure/local/eks-source-key-cleanup.json
```

Verify the resulting identity is an `assumed-role/genai-smart-router-eks-discovery`
session before running discovery. EKS access entries plus namespace-scoped
Kubernetes RBAC remain separately required for `kubectl` reads.

If AWS_CONFIG_FILE or AWS_SHARED_CREDENTIALS_FILE is set, bootstrap writes the
session and role profiles to those exact selected files (otherwise the standard
~/.aws files) and pins its verification command to them. It ignores ambient AWS
credentials, profile selection, and web-identity overrides while performing
that verification, so it cannot validate an older same-named profile. Keep the
selected local files protected and distinct.

Deletion is retried three times. If AWS remains unavailable, bootstrap fails
with a mode-0600 recovery record containing only safe state: either an exact
temporary access-key ID after a known create response, or a pre-create
reservation when the process was killed or the create result is ambiguous.
It never records the secret key. An existing recovery record blocks all new
source-key creation; the bootstrap tool never overwrites an unresolved record.

### Reconcile A Stale Recovery Record

Do not rerun bootstrap or delete a recovery record blindly. First inspect its
safe state locally:

```bash
make eks-session-recovery-status \
  EKS_CLEANUP_RECORD=/secure/local/eks-source-key-cleanup.json
```

If it reports `access_key_created`, use the approved non-root admin identity to
delete exactly the recorded key ID, verify deletion, then remove the recovery
record and retry. If it reports `reserved_before_create`, no key ID can be
safely inferred: the AWS create request may have succeeded after the client was
killed or disconnected. Using the approved admin identity, list access keys for
the dedicated source user, delete any unexpected bootstrap key, and verify that
no temporary source key remains before removing the reservation and retrying.
The dedicated `smartrouter` source user should not retain normal access keys,
which makes that reconciliation bounded and reviewable. Never create another
key until this reconciliation is complete.

```bash
# Read-only reconciliation inventory; it never reveals secret access-key material.
aws iam list-access-keys --profile <approved-admin-profile> --user-name smartrouter --output json

# After review, delete only an unexpected temporary key ID or the ID reported
# by access_key_created. Then re-run the list command and remove the local
# recovery record only when no temporary source key remains.
aws iam delete-access-key --profile <approved-admin-profile> --user-name smartrouter --access-key-id <temporary-key-id>
```

Use the Make targets to prevent implicit target selection and root-profile
access. They validate identifiers; `make eks-discover` then starts the locked
read-only discovery script, which invalidates the selected evidence path before
it rechecks the expected assumed role or makes any live probe. Keep the
evidence output outside this repository:

```bash
make eks-discover \
  EKS_AWS_PROFILE=genai-smart-router-eks-discovery \
  EKS_ACCOUNT_ID=123456789012 \
  EKS_REGION=us-east-1 \
  EKS_CLUSTER=approved-cluster \
  EKS_NAMESPACE=tenant-approved-customer \
  EKS_LINKERD_NAMESPACE=linkerd \
  EKS_INGRESS_NAMESPACE=ingress-nginx \
  EKS_INGRESS_SERVICE_ACCOUNT=ingress-nginx \
  EKS_LINKERD_TRUST_DOMAIN=cluster.local \
  EKS_ECR_REPOSITORY=approved-router-repository \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json
```

`make eks-identity-check` is available for an identity-only preflight. Neither
Make target creates an AWS, EKS, or Kubernetes resource.

```bash
python3 scripts/eks_discover.py \
  --profile genai-smart-router-eks-discovery \
  --account-id 123456789012 \
  --region us-east-1 \
  --cluster approved-cluster \
  --namespace approved-namespace \
  --linkerd-namespace linkerd \
  --ingress-namespace ingress-nginx \
  --ingress-service-account ingress-nginx \
  --linkerd-trust-domain cluster.local \
  --ecr-repository approved-router-repository \
  --output /secure/evidence/eks-discovery.json
```

`EKS_LINKERD_NAMESPACE` / `--linkerd-namespace` is optional. Omit it for a
cluster that does not use Linkerd. When selected, it is the explicit
control-plane namespace (not a product default), and discovery requires the
Linkerd policy CRDs and the `v1beta3` `Server` plus `v1beta1`
`ServerAuthorization` APIs used by the checked-in template. Select the actual
ingress namespace, service account, and deployment trust domain as well;
discovery proves that the selected service account exists, records the derived
Linkerd identity, and only then reports Linkerd readiness. It deliberately does
not read Linkerd trust configuration payloads; supply the current trust domain
from the approved mesh deployment configuration and review the derived identity
before rendering.

The command creates a temporary kubeconfig and removes it on exit. It reads no
Kubernetes Secrets or ConfigMap payloads, and writes a machine-readable report
containing only safe names, booleans, versions, labels needed for Linkerd
injection, and policy presence. An expired session, wrong account, missing
namespace RBAC, inaccessible ECR, or missing EKS access fails before producing
a success report. When Linkerd was selected, missing Linkerd API/RBAC also
fails before success; otherwise the report records Linkerd as not requested.
Discovery locally serializes use of one output path, invalidates its previous
report before its discovery-role check or any other live probe, and atomically
publishes only a fully successful replacement. A failed role check, probe, or
publish therefore leaves no reusable success report at that selected path.
Preserve historical evidence under a different timestamped path before a rerun.
Keep evidence outside Git.

Review the report before bootstrap: EKS version/auth mode/access entries,
endpoint exposure, audit logging, ingress/storage names, namespace labels and
network policies, router resource names, ECR immutability/encryption/policy
presence, and Linkerd control-plane/policy API/identity service-account
inventory. It deliberately excludes endpoint values, trust-root bodies,
certificates, policy/config payloads, DSNs, and secrets.

## Identity Boundaries

Create distinct short-lived identities for read-only discovery, ECR build/push,
staging reconciliation, production promotion, and the router workload. GitHub
OIDC trust pins the repository/environment subject. The GitHub `staging`
environment must separately restrict deployment branches to the approved branch
before it grants an OIDC token. Production uses a distinct environment-gated
role. Prefer EKS access entries plus
namespace-scoped RBAC; document a dated migration plan if `aws-auth` is still
required.

Discovery requires a separate reviewed cluster-level read binding for exactly
the approved discovery identity: get Namespaces and CRDs, and get/list
IngressClasses and StorageClasses. Do not add those permissions to the tenant
provisioner Role or grant wildcard cluster administration.

It also requires the reviewed namespace-scoped read-only binding in
`deploy/kubernetes/bootstrap/eks-discovery-namespace-rbac.example.yaml` for
the selected tenant namespace: ServiceAccounts, NetworkPolicies, Deployments,
Services, PVCs, and Ingresses. If Linkerd discovery is selected, apply the
separate `eks-discovery-linkerd-namespace-rbac.example.yaml` in the explicit
Linkerd control-plane namespace and
`eks-discovery-ingress-namespace-rbac.example.yaml` in the selected ingress
namespace. All templates bind the EKS access entry's configured Kubernetes
group, not its IAM principal ARN; none grants Secret or ConfigMap reads or any
write verb.

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
modify cluster-wide Linkerd policy, create/mutate Deployments, or create/mutate
service accounts. Router workload manifests use a distinct release identity and
must be constrained by admission policy to approved service accounts and Secret
references. Verify with `kubectl auth can-i` for allowed resources and explicit
denials for `secrets`, `deployments`, `serviceaccounts`, `clusterroles`, other
namespaces, and `pods/exec`.

Before using `tenant-linkerd-policy.example.yaml`, select the Linkerd
control-plane namespace during discovery and verify the discovered
Linkerd control plane, `policy.linkerd.io` CRDs/version, namespace injection
labels, and trust/identity readiness. The template uses `v1beta3` for `Server`
and `v1beta1` for `ServerAuthorization`, the separately served standard CRDs;
discovery must still confirm both versions and the ingress identity against the
cluster before rendering. The template deliberately contains unresolved
namespace and ingress-identity placeholders. Render it only from the successful
scrubbed discovery report, not from hand-copied values:

```bash
make eks-render-linkerd-policy \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml
```

The renderer derives `service-account.namespace.serviceaccount.identity.linkerd.trust-domain`
from the verified report and rejects mismatched identities or unrendered
placeholders. Review and server-side dry-run the resulting outside-repository
artifact before any apply.

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
