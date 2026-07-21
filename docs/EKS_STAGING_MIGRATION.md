# EKS Staging Migration Runbook

This internal runbook deploys a validation-only GenAI Smart Router instance to
the Metrum EKS cluster. It does not cut over production traffic or copy usage
history. The existing Compose deployment remains authoritative until a
separately approved cutover.

## Current Staging State

The validation deployment is live as of 2026-07-14. It uses one
`f52a918-linux-amd64` router replica, the private `smartrouter-gp3` EBS-backed
state PVC, a fresh encrypted single-AZ PostgreSQL 18.3 `db.t4g.medium` RDS
instance, and a dedicated staging caller token stored in AWS Secrets Manager
as `smartrouter/staging/caller-token`. The image is in the Metrum ECR
repository and the runtime Secret remains Kubernetes-only. No customer traffic
or EC2 usage history has moved. The staging browser-admin Basic credential is
stored separately as `smartrouter/staging/basic-admin`; retain only its bcrypt
hash in the runtime Secret.

## Current And Target Topology

| Area | Current production | EKS staging |
| --- | --- | --- |
| Public endpoint | `https://llm-api-engg.metrum.ai` | `https://smartrouter.apps.metrum.ai` |
| Runtime | Docker Compose on EC2 | One Kubernetes Deployment replica |
| Usage database | Compose Postgres | Fresh private RDS PostgreSQL 18 |
| Caller access | Existing production callers | Dedicated staging caller only |
| Traffic authority | Production | Validation only |

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
   namespace/Secret setup and a least-privilege delivery identity for the
   reviewed workload resources. The delivery identity must not have any Secret
   verbs; it reads only the pinned non-secret attestation ConfigMap described
   below. EKS authentication alone is insufficient.
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
   Kustomize source image name, Deployment, container, and delivery role. The checked-in
   repository URI is a bootstrap default, not proof of an approved live AWS/EKS
   target. First renew a least-privilege non-root session and explicitly
   reconcile the approved target. Then provision the exact JSON as a
   standard (not SecureString) AWS Systems Manager Parameter at the ARN in
   that file through reviewed infrastructure-as-code. The staging delivery
   role receives only `ssm:GetParameter` for that one parameter (see
   `deploy/aws/genai-smart-router-eks-staging-delivery-parameter-read-policy.example.json`);
   a separate platform configuration role owns `ssm:PutParameter`. The
   delivery contract reads both copies and fails unless their canonical JSON
   hashes match. The current schema is version 5; update the reviewed file and
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

A separate, privileged Secret-bootstrap identity owns both the runtime Secret
and the policy-pinned non-secret ConfigMap
`smartrouter-staging-runtime-attestation`; the delivery identity gets
name-scoped `get` only on that ConfigMap and no other ConfigMap or Secret
verbs. Before
any Secret mutation, bootstrap deletes the existing attestation. After the
Secret write succeeds, bootstrap reads its metadata and creates a fresh
`immutable: true` ConfigMap with exactly these non-secret `data` keys:
`schema_version: v1`, `secret_name`, `secret_uid`, and
`secret_resource_version`. It must have no `binaryData` and exactly one
same-namespace `v1` `Secret` owner reference whose name and UID match the
attested values. A missing, deleting, mutable, malformed, or mismatched
attestation blocks preflight and delivery. The contract records only the
attested Secret UID/resourceVersion plus the attestation ConfigMap
UID/resourceVersion, never Secret contents or raw ConfigMap data. A Secret or
attestation replacement/update invalidates apply/smoke evidence; rerun the
reviewed apply and protected smoke before a promotion plan can pass. Keep this
ConfigMap outside the Kustomize delivery inventory: it is bootstrap evidence,
not workload desired state.

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
account, region, ECR repository, cluster, namespace, overlay, or environment as caller-set
variables. Those values come from the reviewed target policy and its
independently protected Parameter copy. It creates a temporary per-invocation
kubeconfig, so it neither reads nor changes the operator's default kubectl
context. See `make eks-help` for the complete target list and required inputs.

1. Create the `smart-llmrouter-staging` namespace, then create the runtime
   Secret and make the wildcard certificate Secret available in that namespace.
2. With a short-lived, approved AWS identity, run the read-only contract:

   ```bash
   make eks-preflight eks-plan \
     EKS_DELIVERY_AWS_PROFILE='genai-smart-router-eks-staging-delivery' \
     IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>'
   ```

   Inspect the scrubbed JSON and Markdown in `tmp/eks-evidence/`. The raw
   manifest exists only in a per-invocation temporary directory for kubectl;
   evidence retains only its checksum, a digest-normalized non-secret rendered
   configuration fingerprint, and safe scalar metadata. A missing session,
   protected policy, matching account/role, namespace RBAC, digest, namespace
   match, or dry-run failure stops before mutation.

   The delivery role must also have namespace-scoped `list` permission only on
   `deployments`, `ingresses`, `networkpolicies`,
   `persistentvolumeclaims`, `poddisruptionbudgets`, `services`, and
   `serviceaccounts`, plus name-scoped `get` permission only for the approved
   runtime Secret attestation ConfigMap. It must have no `get`, `list`,
   `watch`, `create`, `update`, `patch`, `delete`, or `deletecollection`
   permissions for Secrets, and no `get` on other ConfigMaps, `list`, `watch`,
   `create`, `update`, `patch`, `delete`, or `deletecollection` permission for
   ConfigMaps. The delivery
   contract builds its expected inventory from
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
     EKS_DELIVERY_AWS_PROFILE='genai-smart-router-eks-staging-delivery' \
     IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>'
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
       EKS_DELIVERY_AWS_PROFILE='genai-smart-router-eks-staging-delivery' \
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
  EKS_DELIVERY_AWS_PROFILE='genai-smart-router-eks-staging-delivery' \
  IMAGE_DIGEST='<approved-ecr-repository>@sha256:<64-hex>' \
  ROLLBACK_POD_TEMPLATE_SHA256='<approved prior live_pod_template_sha256>'
```

It resolves only a matching owned ReplicaSet revision whose named router
container has that exact digest **and** whose complete normalized pod template
matches the approved SHA-256 before mutating; it then verifies the restored
Deployment against the same hash. This binds Secret references, service
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
