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

1. Obtain Kubernetes RBAC for the deployment identity: namespace creation,
   Secret/PVC/Service/Ingress/Deployment/NetworkPolicy management, pod logs,
   and port-forwarding. EKS authentication alone is insufficient.
2. Confirm the `nginx` ingress class, selected ingress namespace, and the
   namespace-local wildcard certificate Secret named
   `apps-metrum-ai-wildcard-tls`. Render the discovery-derived ingress
   NetworkPolicy for that namespace after the router Pods are Ready. When
   Linkerd is selected, prove ready router and ingress proxies whose safe local
   identity and trust-domain fields match the selected control plane, then render
   and activate its policy with the NetworkPolicy; do not restore a fixed ingress
   namespace in the overlay.
3. Create `deploy/kubernetes/overlays/metrum-staging/storageclass.yaml` with a
   cluster-admin identity before applying the overlay. It defines the internal
   `smartrouter-gp3` class using this EKS cluster's Auto Mode EBS CSI driver;
   the existing `gp2` class points at an unavailable legacy provisioner.
4. Preserve the configured license fingerprint when preparing the staging
   config, then validate the existing signed license from the staging pod. If
   the license policy or commercial terms require a distinct staging binding,
   obtain a replacement Metrum-issued license before exposing the endpoint.
5. Push an immutable router image to the registry used by EKS, then replace
   `replace-with-immutable-image-tag` in the overlay through a local,
   reviewed image patch or Kustomize image override.

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

1. Create the `smart-llmrouter-staging` namespace, then create the runtime
   Secret and make the wildcard certificate Secret available in that namespace.
2. Render `deploy/kubernetes/overlays/metrum-staging` and run a server-side
   dry run.
3. Apply the overlay and wait for the router Deployment to become ready.
4. Re-run the explicit-target discovery after the router Pods are Ready, then
   render and activate the companion ingress NetworkPolicy through the
   selection-bound target. It verifies the discovery account and selected EKS
   endpoint against the named deployment AWS profile and explicit kubeconfig/
   context; do not use an ambient `kubectl` context. For the selected Linkerd
   deployment, it server-side dry-runs and applies its `Server` and
   `ServerAuthorization` before the namespace allow policy. Discovery evidence
   is valid for 15 minutes only, so rerun it immediately if that window expires
   before activation:

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
6. Validate `/healthz`, `/readyz`, `/docs/`, and `/version` through both the
   Service and `https://smartrouter.apps.metrum.ai`.
7. With the dedicated staging caller, validate `/v1/models`, OpenAI Chat,
   OpenAI Responses, Anthropic Messages, streaming, and representative
   validated tool/image request shapes for each intended staging group.
8. Verify fresh usage rows and authorized admin reports against RDS. Verify a
   normal caller receives `403 metrics-forbidden` from `/metrics`.
9. Record safe image version, RDS major version, readiness result, smoke
   results, and rollback evidence in `deployment.md`.

## Rollback And Deferred Cutover

If staging fails, preserve the RDS and state PVC snapshot for diagnosis, then
remove the staging Ingress and Deployment. EC2 traffic and data remain
unaffected.

A future production cutover requires separate approval and a new runbook
section covering an EC2 write freeze, logical Postgres export/import, row and
report reconciliation, final state handling, DNS transition, client acceptance,
and an explicit rollback window. Do not make either public production hostname
point at EKS before that process passes.
