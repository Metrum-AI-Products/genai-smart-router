# EKS Staging Migration Runbook

This internal runbook deploys a validation-only GenAI Smart Router instance to
the Metrum EKS cluster. It does not cut over production traffic or copy usage
history. The existing Compose deployment remains authoritative until a
separately approved cutover.

## Current Staging State

The validation deployment was originally activated on 2026-07-14 with one
`f52a918-linux-amd64` router replica, the private `smartrouter-gp3` EBS-backed
state PVC, a fresh encrypted single-AZ PostgreSQL 18.3 `db.t4g.medium` RDS
instance, and a dedicated staging caller token stored in AWS Secrets Manager
as `smartrouter/staging/caller-token`. On 2026-08-06, public probes for
`/healthz`, `/readyz`, `/docs/`, and `/version` all returned HTTP 503. Treat
staging as unavailable until the repair lifecycle below completes and new
evidence supersedes that observation. The required
`genai-smart-router-eks-staging-delivery` IAM role was also absent at that
checkpoint; issue #792 tracks the reviewed IAM/EKS RBAC prerequisite.

The image remains in the Metrum ECR repository and the runtime Secret remains
Kubernetes-only. No customer traffic or EC2 usage history has moved. The
staging browser-admin Basic credential is stored separately as
`smartrouter/staging/basic-admin`; retain only its bcrypt hash in the runtime
Secret.

## Current And Target Topology

| Area | Current production | EKS staging |
| --- | --- | --- |
| Public endpoint | `https://llm-api-engg.metrum.ai` | `https://smartrouter.apps.metrum.ai` |
| Runtime | Docker Compose on EC2 | One Kubernetes Deployment replica |
| Usage database | Compose Postgres | Fresh private RDS PostgreSQL 18 |
| Caller access | Existing production callers | Dedicated staging caller only |
| Traffic authority | Production | Validation only |

## Staging Repair And Validation Lifecycle

Use this sequence for every staging repair. A prior successful deployment does
not authorize reuse of an expired identity, stale evidence, ambient kubeconfig,
or a different image digest.

### One-time authorization bootstrap

The platform-IaC owner deploys
`deploy/aws/genai-smart-router-eks-staging-identity.yaml` with one exact
federated/SSO operator-role ARN, reviews the CloudFormation change set, and
applies it with `CAPABILITY_NAMED_IAM`. That stack creates a persistent
least-privilege bootstrap role and EKS access entry. The separately authorized
cluster-bootstrap owner installs the fail-closed
`deploy/kubernetes/bootstrap/eks-staging-bootstrap-rbac-admission.yaml`, then
applies `eks-staging-bootstrap-rbac.yaml`, the separately owned named-only
admission-read `eks-staging-delivery-rbac.yaml`, and the unbound
`eks-staging-delivery-namespace-rbac.yaml` once through an explicit mode-`0600`
temporary kubeconfig. The owner creates the immutable runtime attestation and
server-side dry-runs, applies, and reads back
`eks-staging-delivery-admission.yaml` before applying
`eks-staging-delivery-rolebinding.yaml`; that final binding is the first point
at which the delivery group gains Deployment mutation authority. Thereafter an
authorized operator runs `scripts/reconcile_staging_delivery_rbac.py`, which
server-side dry-runs, force-claims drifted fields only behind the exact-spec
admission guard, and readbacks only the exact named namespace delivery `Role`
and `RoleBinding`.
The admission guard makes its required `bind` and `escalate` authorization
usable only for that exact known specification; it cannot create resources,
read Secrets, mutate workloads, or broaden itself. The cluster-bootstrap owner
still creates the immutable version-2 runtime/admission attestation and
server-side dry-runs and applies
`deploy/kubernetes/bootstrap/eks-staging-delivery-admission.yaml`. This order
keeps missing parameters and policy failures deny-by-default before a human
receives Deployment write authority. Complete commands, image-approval
requirements, and revocation behavior are in
[`deploy/aws/README.md`](../deploy/aws/README.md#deployable-staging-delivery-identity).

Every authorized operator uses their own federated source profile and a local
role profile. The local names are operator-selected and contain no credentials:

```ini
[profile <operator-delivery-profile>]
role_arn = arn:aws:iam::121701826775:role/genai-smart-router-eks-staging-delivery
source_profile = <operator-federated-profile>
region = us-east-1
role_session_name = <operator-change-id>
```

```ini
[profile <operator-bootstrap-profile>]
role_arn = arn:aws:iam::121701826775:role/genai-smart-router-eks-staging-bootstrap
source_profile = <operator-federated-profile>
region = us-east-1
role_session_name = <operator-change-id>
```

For a permanent IAM user admitted through the fixed lifecycle-operators group,
chain through the intermediary role before selecting the target role:

```ini
[profile <operator-lifecycle-profile>]
role_arn = arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-lifecycle-operator
source_profile = <operator-iam-user-profile>
region = us-east-1
role_session_name = <operator-change-id>

[profile <operator-delivery-profile>]
role_arn = arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-delivery
source_profile = <operator-lifecycle-profile>
region = us-east-1
role_session_name = <operator-change-id>

[profile <operator-bootstrap-profile>]
role_arn = arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-bootstrap
source_profile = <operator-lifecycle-profile>
region = us-east-1
role_session_name = <operator-change-id>
```

The platform-IaC owner permanently manages membership in
`genai-smart-router-eks-staging-lifecycle-operators` outside this repository.
Each member must be an IAM user under `/smart-router-lifecycle/` with the
principal tag `GenAISmartRouterLifecycle=true`; the group can only enter the
intermediary lifecycle role, which can only delegate to the three reviewed
target roles. Do not add an individual user to a target-role trust policy or
grant it EKS, Kubernetes, Secret, or direct target-role authority.

Use `<operator-bootstrap-profile>` only with
`scripts/reconcile_staging_delivery_rbac.py` after the cluster-bootstrap owner
has installed the bootstrap admission guard and binding. The normal delivery CLI
continues to use `<operator-delivery-profile>`.

The federated source role must be the exact principal trusted by the stack and
must separately allow `sts:AssumeRole` on the target roles it uses. A permanent
group member needs the fixed group policy, required IAM-user path, required
principal tag, and only the first hop to the lifecycle-operator role; that
intermediary grants the second hop to delivery, bootstrap, or image-publisher.
Identity-provider and IAM-group membership decide who may use each path; the
stack neither creates nor names individual human users.

#### Fixed lifecycle roles and source authorization

The deployment owns three fixed target roles plus an intermediary and permanent
membership group:

| Identity | Allowed lifecycle surface | Explicit boundary |
| --- | --- | --- |
| `genai-smart-router-eks-staging-lifecycle-operators` | Permanent IAM-user membership to begin a lifecycle session | May only assume the intermediary role; users must have the required IAM path |
| `genai-smart-router-eks-staging-lifecycle-operator` | Delegates a validated lifecycle session to a reviewed target role | May only assume delivery, bootstrap, or image-publisher; no EKS, Secret, ECR, or direct workload authority |
| `genai-smart-router-eks-staging-delivery` | Reviewed staging workload create, update, and patch through the checked-in delivery contract | No delete, Secret read, wildcard, cluster-wide, or admission-policy mutation authority |
| `genai-smart-router-eks-staging-bootstrap` | Exact reviewed delivery `Role` and `RoleBinding` recovery after its admission guard is installed | No workload, Secret, arbitrary RBAC, or cluster-scoped mutation authority |
| `genai-smart-router-eks-staging-image-publisher` | Immutable image publication to the reviewed staging ECR repository | No EKS, Secret, or deployment authority |

The platform-IaC owner is a separately approved non-root organization role. It
deploys the identity stack and its AWS infrastructure; it is not a runtime
delivery identity and is not named by this repository. The separately
authorized cluster-bootstrap owner creates the one-time Kubernetes admission
and RBAC objects described in the bootstrap sequence. An individual operator,
including an IAM-user-backed operator, never appears in the stack trust policy.
Instead, the organization's federated/SSO source role is the exact value
supplied as `AuthorizedOperatorRoleArn`, and its reviewed permission set must
grant only:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "sts:AssumeRole",
    "Resource": [
      "arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-delivery",
      "arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-bootstrap",
      "arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-image-publisher"
    ]
  }]
}
```

For the permanent IAM-user path, the fixed group policy grants only the first
hop:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "sts:AssumeRole",
    "Resource": "arn:aws:iam::<account-id>:role/genai-smart-router-eks-staging-lifecycle-operator"
  }]
}
```

The intermediary trust additionally requires the IAM-user path
`/smart-router-lifecycle/` and the principal tag
`GenAISmartRouterLifecycle=true`; group membership, path, and tag are all
required. The intermediary's own policy contains the exact three target-role
ARNs. Never grant a permanent IAM user a direct target-role policy, Kubernetes
permission, or runtime-secret access.

The organization attaches that policy to the source role or its permission set,
not to an individual user and not to the router roles themselves. Before any
target action, prove the intended assumed role with
`aws sts get-caller-identity`; an `AccessDenied` result from `AssumeRole` is a
source-authorization defect, not a reason to use root credentials or widen the
delivery role.

The role profile may use any local AWS profile name—including `-`, `_`, `.`,
`@`, `+`, `=`, and `,`—but never accept credentials as flags or configuration
content. Before lifecycle work, the operator runs `aws sso login` or the
organization's equivalent federated login for the source profile, then verifies:


```bash
aws sts get-caller-identity \
  --profile <operator-delivery-profile> \
  --query '{Account:Account,Arn:Arn}' \
  --output json
```

#### Headless and noninteractive source authentication

The delivery CLI does not require a local browser. It delegates source
authentication to the operator-selected AWS profile, so use one of these
approved source-profile mechanisms before invoking a delivery target:

- **Headless Identity Center device authorization:** run
  `aws sso login --use-device-code --profile <operator-federated-profile>`.
  Complete the short-lived authorization from a separate browser, without
  copying device codes into evidence or chat.
- **Organization credential process:** configure
  `credential_process = <approved-command>` in the federated source profile.
  The command must emit short-lived AWS credentials only to the AWS CLI process;
  do not write credentials, tokens, or session caches into this repository,
  casts, or change records.
- **Approved workload federation:** use an organization-issued workload profile
  that resolves to the exact trusted source role. Its trust and source-role
  `sts:AssumeRole` grant require the same review as an Identity Center
  assignment.

The source role is the authorization boundary. A developer's ambient IAM-user
credentials cannot mint an Identity Center session or substitute for the
trusted source role, even when they can administer bootstrap infrastructure.

The account must be `121701826775` and the ARN must have
`assumed-role/genai-smart-router-eks-staging-delivery/` as its role/session
path. Record only the pass/fail classification and change ID; do not copy cache
files, access keys, session tokens, SSO device codes, MFA values, or the full
identity response into tickets, casts, or shared logs.

1. **Open the change record.** Record the incident or validation issue, intended
   immutable image digest, previous known-good digest and pod-template
   fingerprint, runtime Secret attestation reference, RDS/PVC retention
   decision, rollback owner, and evidence directory. Never put credentials,
   tokens, Secret data, DSNs, license payloads, prompts, or full configuration
   in the record.
2. **Establish the approved identity.** Authenticate the operator's federated
   source profile, then use any local AWS profile that assumes
   `genai-smart-router-eks-staging-delivery`. Verify the exact account and
   assumed-role name. The role, protected target Parameter, EKS access entry,
   and namespace RBAC must already exist through reviewed infrastructure-as-code.
   Do not substitute `default`, a direct IAM-user profile, account root, or the
   operator's current kubeconfig. Authorization belongs to the federated role
   and its identity-provider membership, never to a hardcoded human username.
3. **Capture the pre-repair baseline.** Record UTC time and only safe scalar
   results for public `/healthz`, `/readyz`, `/docs/`, and `/version`. Record
   workload readiness class, restart-count bucket, desired/available replica
   counts, current image digest, PVC phase, and RDS availability/TLS class
   through approved read-only paths. Keep raw Pod logs, events, configuration,
   and database output outside asciinema and shared evidence.
4. **Run protected preflight and plan.** Use the exact approved digest and a new
   private evidence directory:

   ```bash
   make eks-preflight eks-plan \
     EKS_DELIVERY_AWS_PROFILE='<operator-delivery-profile>' \
     IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>' \
     EKS_EVIDENCE_DIR='/protected/evidence/<change-id>/preflight'
   ```

   Stop if the target-policy hashes differ; the admission policy/binding differs
   from the checked-in contract; the role can mutate admission state; the
   version-2 attestation is stale or does not approve the requested digest;
   supply-chain evidence is missing; namespace permissions are broader than the
   contract; live inventory contains an unmanaged field/object; or RDS/PVC
   ownership is uncertain.
5. **Classify the failure before mutation.** Separate image/config/license,
   database/TLS/migration, PVC, scheduling, ingress/Linkerd/NetworkPolicy, and
   upstream activation failures. Preserve RDS and PVC by default. A Secret or
   license repair belongs to the privileged bootstrap owner; delete the old
   attestation before changing the Secret or approved router/Linkerd images,
   then create the fresh immutable version-2 attestation and reconcile the
   admission policy before delivery. The delivery identity must never read or
   mutate the Secret, attestation, admission policy, or binding.
6. **Apply only reviewed desired state.** After reviewing the plan and
   supply-chain bundle, run:

   ```bash
   make eks-apply-staging \
     EKS_CONFIRM=STAGING_APPLY \
     EKS_DELIVERY_AWS_PROFILE='<operator-delivery-profile>' \
     IMAGE_DIGEST='<same-approved-digest>' \
     EKS_SUPPLY_CHAIN_DIR='/protected/evidence/<change-id>/supply-chain' \
     EKS_EVIDENCE_DIR='/protected/evidence/<change-id>/delivery'
   ```

   The apply may reconcile only fields represented by the reviewed manifest.
   Do not hand-patch the Deployment, delete the Pod to hide a configuration
   failure, or broaden RBAC/NetworkPolicy to make the check pass.
7. **Restore the selected network boundary.** Rerun explicit-target discovery
   after Pods are Ready, render the selected ingress and optional Linkerd
   policies, validate them against the exact kubeconfig/context, then apply
   them with the separate network-policy confirmation described below.
8. **Run the protected smoke.** Use an owner-only mode-`0600` script referenced
   by `EKS_SMOKE_COMMAND_FILE`. Validate Service and public paths for health,
   readiness, docs, and version; then validate authenticated `/v1/models`,
   OpenAI Chat, OpenAI Responses, Anthropic Messages, promised streaming/tool/
   image shapes, ordinary-caller `/metrics` denial, authorized metrics, and
   fresh relational usage/report rows. Record request IDs and safe scalar
   outcomes only.
9. **Capture post-repair evidence.** Record a sanitized asciinema session that
   contains commands without credentials and bounded scalar results without
   model output or raw payloads. Parse the cast for credential/token/Secret/
   DSN/webhook patterns before upload, retain a mode-`0600` protected copy and
   SHA-256, and treat an anonymous expiring URL as presentation evidence only.
10. **Review and announce.** Run `eks-promotion-plan` only after the accepted
    apply and smoke evidence match the same digest, target, attestation, live
    generation, and configuration fingerprints. Update the issue and
    `deployment.md`; post only the endpoint, safe version/digest, high-signal
    checks, evidence URL/checksum, and rollback state to Google Workspace.
11. **Rollback on failed acceptance.** Do not announce success or enable
    ordinary traffic. Preserve RDS/PVC, then use `eks-rollback-staging` with the
    reviewed prior digest and exact prior pod-template SHA-256. Repeat
    preflight, network, smoke, evidence, and announcement steps after rollback.
12. **Clean up authority and temporary data.** Delete temporary kubeconfigs,
    smoke scripts, unprotected cast copies, source access keys, and expired
    session profiles. Keep only intentional protected evidence and approved
    timestamped backups. Confirm Compose production health remains unchanged.

The existing router host is `ubuntu@100.30.225.66`, using
`~/.ssh/chetan-jun-2026.pem`. Its Compose directory is
`/opt/smart-llmrouter/compose`. Access it only for safe deployment status,
backup, configuration-summary, or later cutover work. Do not print or copy its
provider keys, caller tokens, token hashes, or complete config.

## Prerequisites

Before any automated or repeated EKS access, follow
[`EKS_IDENTITY_BOOTSTRAP.md`](./EKS_IDENTITY_BOOTSTRAP.md) to reauthenticate,
produce a redacted explicit-target discovery report, and review identity,
Linkerd, and namespace bootstrap prerequisites. This staging runbook does not
authorize a production cutover.

1. Obtain separate Kubernetes identities: a privileged bootstrap identity for
   namespace/Secret/admission setup and a least-privilege delivery identity for
   reviewed workload resources. The delivery identity has no Secret or delete
   verbs. It reads only the named non-secret attestation ConfigMap and the exact
   admission policy/binding described below; EKS authentication alone is
   insufficient.
2. Confirm the `nginx` ingress class, selected ingress namespace, and the
   namespace-local wildcard certificate Secret named
   `apps-metrum-ai-wildcard-tls`. Render the discovery-derived ingress
   NetworkPolicy for that namespace after the router Pods are Ready. The staging
   overlay's `networkpolicy-ingress-guard.yaml` denies ingress until that companion policy is
   activated; the generic base remains usable without an ingress policy on
   non-EKS clusters. When Linkerd is selected, retain the router Deployment
   template's `linkerd.io/inject: enabled` setting and prove ready router and
   ingress proxies whose safe local identity and trust-domain fields match the
   selected control plane, then render and activate its policy with the
   NetworkPolicy; do not restore a fixed ingress namespace in the overlay.
   The activation helper snapshots the named kubeconfig and exact rendered
   policy bytes privately before target validation and apply, so do not replace
   those caller files during an activation run.
3. Create `deploy/kubernetes/overlays/metrum-staging/storageclass.yaml` with a
   cluster-admin identity before applying the overlay. It defines the internal
   `smartrouter-gp3` class using this EKS cluster's Auto Mode EBS CSI driver;
   the existing `gp2` class points at an unavailable legacy provisioner.
4. Preserve the configured license fingerprint when preparing the staging
   config, then validate the existing signed license from the staging pod. If
   the license policy or commercial terms require a distinct staging binding,
   obtain a replacement Metrum-issued license before exposing the endpoint.
5. Push an immutable router image only to the ECR repository named by the
   protected staging target policy, then replace
   `replace-with-immutable-image-tag` in the overlay through a local,
   reviewed image patch or Kustomize image override. The Make contract rejects
   every digest reference outside that exact account- and region-bound
   repository before it selects the cluster, renders, or applies a manifest.
6. Establish the protected staging delivery target before invoking a Make
   target. `deploy/aws/genai-smart-router-eks-staging-target.json` is the
   reviewed canonical target: account, region, ECR repository, cluster,
   namespace, runtime Secret, runtime Secret attestation ConfigMap, overlay,
   Kustomize source image name, `image_architecture`, Deployment, container,
   and delivery role. The checked-in repository URI is a bootstrap default, not
   proof of an approved live AWS/EKS target. First renew a
   least-privilege non-root session and explicitly
   reconcile the approved target. Then provision the exact JSON as a
   standard (not SecureString) AWS Systems Manager Parameter at the ARN in
   that file through reviewed infrastructure-as-code. The staging delivery
   role receives only `ssm:GetParameter` for that one parameter (see
   `deploy/aws/genai-smart-router-eks-staging-delivery-parameter-read-policy.example.json`);
   a separate platform configuration role owns `ssm:PutParameter`. The
   delivery contract reads both copies and fails unless their canonical JSON
   hashes match. The current schema is version 6; update the reviewed file and
   protected Parameter through the same approved infrastructure change. The
   `kustomize_router_image_name` field is the full tagless source image in the
   overlay, not the ECR destination; the contract replaces exactly that name
   with the approved immutable digest. A
   version or value mismatch fails closed. See `deploy/aws/README.md` for the
   required protected-policy reconciliation.

## RDS Provisioning

Create an RDS instance with these fixed requirements:

- engine: PostgreSQL 18.3;
- class: `db.t4g.medium`;
- availability: single AZ;
- storage encryption and automated backups enabled;
- public access disabled;
- DB subnet group spanning the EKS VPC private subnets;
- a dedicated security group allowing TCP 5432 only from the EKS workload/node
  security group;
- TLS hostname verification using the mounted RDS CA bundle.

Create a new empty database and dedicated least-privilege application user.
The router creates its relational usage schema on startup through GORM. Do not
restore the current EC2 Postgres dump in this staging phase.

## Runtime Secret And Config

Create `smartrouter-staging-runtime` from ignored local files as documented in
the overlay README. It contains `config.yaml`, `env.json`, `license.json`,
`rds-ca.pem`, `router.ts`, and `ROUTER_USAGE_DB_DSN`.

The staging configuration starts from the production routing/provider/model
configuration, but it must:

- set `server.usage_db` to PostgreSQL and use `${ROUTER_USAGE_DB_DSN}`;
- use `/app/state/router-state.json` and
  `/app/state/license-state.json`;
- use the EKS-bound license at `/app/config/license.json`;
- trust only verified ingress-proxy CIDRs for caller-IP and Basic-auth HTTPS
  forwarding. The current nginx ingress pod range is `192.168.0.0/16`; the
  legacy Compose-only `172.18.0.0/16` range is not valid for this deployment;
- contain only the dedicated staging user, project, membership, caller hash,
  and non-production caller token;
- retain provider API key references in the ignored `env.json` without
  changing active provider/model metadata.

Never create the Secret from a tracked file, a shell history containing a raw
DSN, or a rendered manifest committed to the repository.

The delivery contract permits only this approved runtime Secret in the router
Pod's Secret volume, projected-volume, `env`, `envFrom`, and init-container
sources. The delivery identity must have **no** Secret verbs: Kubernetes RBAC
does not support a metadata-only Secret `get`, and JSONPath would filter only
after the full credential-bearing Secret had been authorized and returned.

A separate, privileged Secret-bootstrap identity owns the runtime Secret, the
non-secret `smartrouter-staging-runtime-attestation` ConfigMap, and the
cluster-scoped staging delivery admission policy/binding. The delivery identity
gets name-scoped `get` only on those non-secret objects and cannot mutate them.
Before any Secret or approved-image change, bootstrap deletes the existing
attestation. After the reviewed inputs are ready, bootstrap creates a fresh
`immutable: true` ConfigMap with exactly these non-secret `data` keys:
`schema_version: v2`, `secret_name`, `secret_uid`,
`secret_resource_version`, `approved_router_image`,
`approved_linkerd_proxy_image`, `approved_linkerd_init_image`, and the exact
`approved_pod_creator_username` observed for the ReplicaSet controller. The
router image is an approved immutable ECR digest. Linkerd images are the exact
injector-owned values; `approved_linkerd_init_image: none` is valid only when
Linkerd CNI removes the `linkerd-init` initializer—the native `linkerd-proxy`
sidecar remains required. The ConfigMap has no `binaryData` and exactly one
same-namespace `v1` `Secret` owner reference matching the attested name and UID.

Bootstrap server-side dry-runs and applies
`deploy/kubernetes/bootstrap/eks-staging-delivery-admission.yaml` before
delivery RBAC. The native ConfigMap parameter uses
`parameterNotFoundAction: Deny`; `failurePolicy: Fail` and `Deny` actions keep
missing parameters, CEL failures, unapproved images, unsafe process/host
settings, extra containers, and protected-volume access fail-closed. Preflight
compares the live policy and binding to the checked-in specs and proves the
delivery role cannot mutate either. It also requires the requested digest to
equal `approved_router_image`.

A missing, deleting, mutable, malformed, or mismatched attestation blocks
preflight and delivery. The contract records only safe attested
UID/resource-version fields and admission-spec hashes, never Secret contents or
raw ConfigMap data. A Secret, attestation, policy, binding, or approved-image
change invalidates apply/smoke evidence; rerun reviewed apply and protected
smoke before promotion. Keep the ConfigMap and admission objects outside the
Kustomize workload inventory: they are privileged bootstrap state.

## Outcome-Calibrated Routing Validation

The staging namespace may run the separate `outcome-calibrated-policy` service
when validating `strategy: external`. It is a ClusterIP-only trusted policy
service, not router product code. The service receives the router's
`include_request: true` policy payload, calls the configured OpenAI embeddings
endpoint using a Kubernetes Secret, and accepts the router only with a separate
request-authentication header. Its audit endpoint has a different secret and is
accessed only through an operator port-forward for evidence collection.

Keep the deployment-owned dataset, real candidate responses, reviewed outcomes,
generated profile, temporary caller token, audit token, and evidence output in
an ignored protected directory. The reusable sequence is: dispatch each
case/candidate with calibration overrides, run the objective verifier, generate
the promotable profile, redeploy without overrides, then run fresh requests with
a unique run ID. Retain the router request IDs, selected policy targets, and
verifier result as validation evidence. Remove the policy Deployment, Service,
NetworkPolicy, ConfigMaps, and policy Secret when the staging validation ends;
the router group must also be removed from the runtime Secret before treating
the staging configuration as a general baseline.

## Deployment And Validation

The root Makefile is the canonical EKS operator and CI interface. It accepts a
short-lived AWS profile and immutable image digest, but never accepts an
account, region, ECR repository, cluster, namespace, overlay, environment, or
`image_architecture` as caller-set variables. Those values come from the
reviewed checked-in target policy; delivery requires that complete policy's
canonical hash to match its independently protected Parameter copy. The
supply-chain verifier validates the image-bound release-binding statement, its
raw SBOM/provenance/scan hashes, and signature-verifier result against the
policy-selected `image_architecture`. It creates a
temporary per-invocation kubeconfig, so it neither reads nor changes the
operator's default kubectl context. See `make eks-help` for the complete target
list and required inputs.

For signed-image evidence, protected-environment prerequisites, and the future
GitHub Actions release boundary, see `docs/EKS_STAGING_CICD.md`. This runbook
continues to own runtime-secret, RDS, license, smoke, and rollback procedures.

1. Create the `smart-llmrouter-staging` namespace, then create the runtime
   Secret and make the wildcard certificate Secret available in that namespace.
2. With a short-lived, approved AWS identity, run the read-only contract:

   ```bash
   make eks-preflight eks-plan \
     EKS_DELIVERY_AWS_PROFILE='<operator-delivery-profile>' \
     IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>'
   ```

   Inspect the scrubbed JSON and Markdown in `tmp/eks-evidence/`. The raw
   manifest exists only in a per-invocation temporary directory for kubectl;
   evidence retains only its checksum, a digest-normalized non-secret rendered
   configuration fingerprint, and safe scalar metadata. A missing session,
   protected policy, matching account/role, namespace RBAC, digest, namespace
   match, or dry-run failure stops before mutation.

   The delivery role has namespace-scoped `list` permission only on
   `deployments`, `ingresses`, `networkpolicies`,
   `persistentvolumeclaims`, `poddisruptionbudgets`, `services`, and
   `serviceaccounts`; name-scoped `get` on the approved runtime attestation
   ConfigMap; and name-scoped `get` on the exact admission policy and binding.
   It has no Secret or resource-deletion verbs, no other ConfigMap access, and
   no admission-policy mutation/list/watch authority. Preflight probes every
   denied surface before render or apply.
   The delivery contract builds its expected inventory from
   the isolated client-rendered manifest, not from Server-Side Apply output.
   Server-side dry-run is an acceptance/field-ownership check only: its object
   output is compared as a candidate live object and never becomes the expected
   inventory because it can include fields preserved from another manager. Live
   objects are compared with that desired baseline after removing only
   documented API-owned runtime fields (for example Service cluster allocation
   and a PVC binding name) and exact, omitted Kubernetes defaults.
   Routing, security, labels, annotations, owner references, finalizers,
   admission additions, and all other declared resource settings must exactly
   match the reviewed manifest, except the Kubernetes-owned
   `volume.kubernetes.io/selected-node` annotation and
   `kubernetes.io/pvc-protection` finalizer on the reviewed
   `WaitForFirstConsumer` state PVC. A new
   admission-managed field must be represented in the reviewed manifest or it
   will fail closed rather than being absorbed into the expected fingerprint.
   It does not use broad prune. If an old managed object is absent from a new
   manifest (for example, a removed or renamed Ingress, Service, or
   NetworkPolicy), apply stops before mutation and the object must be removed
   through a separate reviewed recovery/migration.

   Rollback additionally needs namespace-scoped `list` permission for
   `replicasets`. Before undoing anything, the contract resolves the highest
   prior ReplicaSet revision owned by the approved Deployment that already uses
   the requested immutable digest. If there is no matching prior revision, it
   stops before mutation rather than letting `kubectl rollout undo` select an
   arbitrary immediately previous revision.

   A reviewed change record is the desired-state reconciliation authorization;
   `EKS_CONFIRM=STAGING_APPLY` is only an explicit operator acknowledgement of
   the mutation, never a substitute for the protected policy/SSM boundary. The
   apply may repair a changed value at a field already represented by the
   reviewed manifest, but it stops before mutation when live state contains an
   additional label, annotation, owner reference, finalizer, spec field, or
   list item. The contract records safe before-apply identity/configuration
   fingerprints, then refuses passed
   apply, smoke, or promotion evidence unless the live objects exactly match
   the reviewed configuration. Treat an unexpected pre-apply fingerprint
   difference as an audit/change-control signal; do not add a caller-controlled
   bypass for it.
3. Apply only after review, using the explicit staging confirmation:

   ```bash
   make eks-apply-staging EKS_CONFIRM=STAGING_APPLY \
     EKS_DELIVERY_AWS_PROFILE='<operator-delivery-profile>' \
     IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>' \
     EKS_SUPPLY_CHAIN_DIR='tmp/eks-supply-chain'
   ```

4. Re-run the explicit-target discovery after the router Pods are Ready, then
   render and activate the companion ingress NetworkPolicy through the
   selection-bound target. It verifies the discovery account and selected EKS
   endpoint against the named deployment AWS profile and explicit kubeconfig/
   context; do not use an ambient `kubectl` context. For the selected Linkerd
   deployment, it requires durable namespace or Deployment-template Linkerd
   injection evidence, then server-side dry-runs and applies its `Server` and
   `ServerAuthorization` before the namespace allow policy. Discovery evidence
   is valid for 15 minutes only, so rerun it immediately if that window expires
   before activation. The exact companion
   `NetworkPolicy/smart-llmrouter-discovered-ingress` is discovery-owned rather
   than overlay-owned. It retains the router label only in `spec.podSelector`,
   not metadata, so delivery never selects it; every label-selected resource
   remains fail-closed. Re-render and activate any legacy companion policy that
   still carries the delivery label before its next delivery run; delivery
   deliberately reports that object as stale:

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
   `make eks-render-ingress-network-policy`, then the same validate/apply
   targets without `EKS_LINKERD_POLICY_OUTPUT`.
5. Verify RDS TLS connectivity and that the fresh database contains the router
   schema. A failed migration or license check must keep `/readyz` unhealthy.
6. A protected smoke script is arbitrary operator-supplied code, so treat it
   as a potentially mutating staging action—not a read-only command. It uses
   the same approved delivery identity and therefore requires the same explicit
   `EKS_CONFIRM=STAGING_APPLY` acknowledgement as apply and rollback. Create an
   owner-only, non-symlink mode-`0600` POSIX shell script in the protected
   local/CI workspace, then invoke `make eks-smoke-staging` with the same
   approved profile and immutable digest plus its path in the inherited
   `EKS_SMOKE_COMMAND_FILE` environment variable:

   ```bash
   chmod 600 /secure/ci/smartrouter-staging-smoke.sh
   EKS_SMOKE_COMMAND_FILE=/secure/ci/smartrouter-staging-smoke.sh \
     make eks-smoke-staging \
       EKS_CONFIRM=STAGING_APPLY \
       EKS_DELIVERY_AWS_PROFILE='<operator-delivery-profile>' \
       IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>'
   ```

   The Make recipe is non-echoed and passes only the script path to the
   delivery process. The process opens that exact non-symlink file once with a
   no-follow descriptor and invokes `/bin/sh -s` from the descriptor, so a
   later path replacement cannot change the validated script. It never reads,
   prints, hashes, or stores command content. Smoke stdout/stderr is not copied
   into evidence on failure. Keep credentials in the secure runner/secret
   source rather than an inline command, and use that private runner log for
   diagnostics; do not put a caller credential in Make variables, command
   lines, evidence, or shell history.
   Validate `/healthz`, `/readyz`, `/docs/`, and `/version` through both the
   Service and `https://smartrouter.apps.metrum.ai`.
7. Run `make eks-promotion-plan IMAGE_DIGEST='<the same immutable digest>'`
   only after review. It remains read-only and requires both passed
   `evidence-apply.json` and `evidence-smoke.json` for that exact protected
   target, digest, rendered configuration fingerprint, live Deployment
   pod-template/generation state, attested runtime Secret UID/resourceVersion,
   immutable attestation ConfigMap name/UID/resourceVersion, exact
   label-selected managed-resource identity, and normalized configuration
   fingerprints. A smoke must accept the exact passed apply
   evidence first; a later apply invalidates it, so promotion can use only a
   smoke that ran after its accepted apply. It rechecks the current rollout and
   resource configuration before producing review-only evidence and cannot
   apply to production.
8. With the dedicated staging caller, validate `/v1/models`, OpenAI Chat,
   OpenAI Responses, Anthropic Messages, streaming, and representative
   validated tool/image request shapes for each intended staging group.
9. Verify fresh usage rows and authorized admin reports against RDS. Verify a
   normal caller receives `403 metrics-forbidden` from `/metrics`.
10. Record safe image version, RDS major version, readiness result, smoke
   results, and rollback evidence in `deployment.md`.

## Rollback And Deferred Cutover

If staging fails, preserve the RDS and state PVC snapshot for diagnosis, then
run the rollback with the explicitly reviewed, approved-repository immutable
digest and the approved full pod-template SHA-256 from the prior release's
safe apply evidence (`live_pod_template_sha256`):

```bash
make eks-rollback-staging EKS_CONFIRM=STAGING_APPLY \
  EKS_DELIVERY_AWS_PROFILE='<operator-delivery-profile>' \
  IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>' \
  EKS_SUPPLY_CHAIN_DIR='tmp/eks-supply-chain' \
  ROLLBACK_POD_TEMPLATE_SHA256='<approved prior live_pod_template_sha256>'
```

The same supply-chain validation prerequisite runs before rollback delivery;
the protected evidence bundle must bind this exact historical image digest and
the architecture from the approved target policy. It resolves only a matching
owned ReplicaSet revision whose named router container has that exact digest
**and** whose complete normalized pod template matches the approved SHA-256
before mutating; it then verifies the restored Deployment against the same
hash. This binds Secret references, service
account, security settings, volumes, sidecars, and init containers—not merely
the router image. A mutable, off-repository, missing-history, template-mismatch,
or unexpected restored image fails closed. Removal of the Ingress or Deployment
remains a separate approved recovery action. EC2 traffic and data remain
unaffected.

A future production cutover requires separate approval and a new runbook
section covering an EC2 write freeze, logical Postgres export/import, row and
report reconciliation, final state handling, DNS transition, client acceptance,
and an explicit rollback window. Do not make either public production hostname
point at EKS before that process passes.
