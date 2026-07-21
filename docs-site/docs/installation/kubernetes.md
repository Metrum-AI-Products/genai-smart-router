---
title: Deploy To Kubernetes
doc_type: howto
---

# Deploy To Kubernetes

Use Kubernetes when GenAI Smart Router needs to run inside a customer-managed cluster with cluster-native ingress, Secrets, external Postgres, and operational controls. This is the canonical Kubernetes installation page; post-deployment topology guidance lives in [Enterprise Deployment Patterns](../operations/deployment-patterns).

Metrum maintains Kustomize-friendly manifests as a production-oriented starting point. The manifests are examples. Review them against your cluster's ingress controller, network policy engine, storage class, registry, and secret-management process before production rollout. If you are installing from a package that does not include Kubernetes manifests, obtain the matching manifest bundle from Metrum for that release.

The base and example overlay are deployment-neutral. Choose your own hostname,
ingress class, certificate workflow, registry, database topology, storage class,
resource sizing, and caller policy. A managed deployment can maintain a private
environment-specific overlay, but those values are not product defaults and do
not belong in public manifests or package documentation.

## Prerequisites

- A Kubernetes cluster with an ingress controller and TLS automation or a separate TLS termination plan.
- A private registry image tag such as `registry.example.com/smart-llmrouter:<version>-linux-amd64`.
- External Postgres for the usage database.
- A Metrum-issued `license.json`.
- Provider credentials stored in a Kubernetes Secret or external secret manager.
- A router config reviewed for the deployment's model groups, callers, admin auth, and reporting settings.

## EKS Identity And Bootstrap

For EKS, begin with an explicitly selected account, region, cluster, namespace,
and repository. Use a short-lived federated identity to perform read-only
discovery before applying manifests; do not rely on a current kube context or
store AWS access keys in CI. Keep discovery evidence limited to safe inventory
such as versions, resource names, and policy presence—not secrets, endpoints,
certificate bodies, DSNs, or rendered Secret data.

For a selected discovery-evidence path, invalidate only a structurally
recognizable prior discovery report before live probes and publish a replacement
atomically only after all checks pass. An existing non-report file is refused
and left untouched. Do not use a report left behind by a failed or interrupted
discovery attempt; retain historical evidence under a separate timestamped path
when needed.

If deployment tooling creates a temporary source access key to establish a
short-lived session, reserve durable local recovery state before that IAM call.
An interrupted or ambiguous creation must block retries until an operator has
reconciled the dedicated source user's keys; never create a second key merely
because the first process did not return a key identifier.
The bootstrap must verify the exact approved account and discovery assumed-role
identity before it reports success. If a paired local AWS profile update cannot
publish its role-config file, it restores both prior profile files and fails
rather than leaving a new source session paired with stale role settings.
Use only absolute, non-symlink local profile files outside the repository with
non-writable-by-others parent directories; a failed post-publication verification
or cleanup restores both prior profile files (or removes newly created ones).

Use distinct roles for discovery, registry push, staging deployment, production
promotion, and workload access. GitHub Actions should use OIDC with repository
and environment claims constrained in the AWS trust policy; configure the
GitHub Environment itself to restrict deployment branches to the approved
branch, because an environment-style OIDC subject does not constrain it. Bind
deployment and tenant-provisioner identities only within their approved
namespaces; they must not read arbitrary Secrets, alter cluster roles, or modify
another tenant. Where Linkerd is used, verify its control plane, policy CRDs,
namespace injection, and identity/trust readiness before applying namespace
policy resources. Select the actual Linkerd control-plane namespace during
read-only discovery; clusters that do not use Linkerd may omit it. Review RBAC,
service-account, network-policy, quota, and
Linkerd-policy drift before overwriting it. Discovery always selects the
ingress namespace and can render its companion `NetworkPolicy` for a
non-Linkerd installation. Render each `ServerAuthorization`
only after discovery verifies the selected ingress Deployment uses the selected
service account and its controller-owned Pods are ready with a `linkerd-proxy`
sidecar, and verifies the selected router Pods also have ready `linkerd-proxy`
sidecars whose safe local identity and literal trust domain match the selected
Linkerd control plane. It also requires durable ingress and router injection
evidence from each selected Namespace or Deployment Pod template before policy
can rely on those current sidecars. A selected ingress Deployment may use
`linkerd.io/inject: ingress`; router workloads require ordinary
`linkerd.io/inject: enabled`. Discovery derives the domain from every selected ingress proxy's safe
literal trust-domain configuration and compares its safe local-identity
configuration with the selected ingress namespace and Linkerd control-plane
namespace before rendering; never apply a template with a fixed, unresolved,
or merely service-account-existence-based ingress identity. The base router
`NetworkPolicy` is egress-only so generic and non-EKS deployments retain their
deployment-owned ingress path. An EKS overlay adds the reviewed
`tenant-router-ingress-guard.example.yaml` before exposure; the same verified
report then renders its companion selected-namespace allow policy. Review,
dry-run, and apply that policy alone for non-Linkerd EKS deployment, or together
with the Linkerd authorization rather than restoring a fixed ingress namespace.

## Image And Architecture

Release Docker packages include per-architecture image tarballs:

| Node architecture | Package image tag example |
|---|---|
| `amd64` / `x86_64` | `<version>-linux-amd64` |
| `arm64` / Graviton | `<version>-linux-arm64` |

For private clusters, load the image tar into nodes or push it to a private registry:

```bash
docker load -i images/smart-llmrouter-<version>-linux-amd64.tar
docker tag smart-llmrouter:<version>-linux-amd64 registry.example.com/smart-llmrouter:<version>-linux-amd64
docker push registry.example.com/smart-llmrouter:<version>-linux-amd64
```

Mixed-architecture clusters need separate per-architecture tags or a registry-managed multi-architecture manifest. Do not use a per-architecture tarball as if it were a multi-architecture image.

## Runtime Contract

A Kubernetes deployment needs:

| Area | Required design |
|---|---|
| Namespace | A deployment-owned namespace with least-privilege RBAC. |
| Router config | A reviewed ConfigMap or mounted config artifact for `config.yaml`, depending on the customer's config-handling policy. |
| Provider keys | A Secret or external secret integration that injects provider credentials as env vars or `env.json`. |
| License | A Secret containing the Metrum-issued `license.json`, mounted at the path configured in `server.license.path`. |
| State | Durable license state and router state when the deployment design requires file-backed state. |
| Usage database | External Postgres is recommended for production Kubernetes deployments. |
| Workload | A Deployment for stateless router pods unless the state design requires a different controller. |
| Network | Service, Ingress or Gateway, TLS, and NetworkPolicy for clients, admin surfaces, database, and upstream providers or private model services. |
| Health | Readiness on `/readyz` and liveness on `/healthz`. |
| Resources | Requests and limits sized for request concurrency, streaming traffic, and admin report queries. |

Do not put provider keys, raw router tokens, token hashes, private signing material, or full production configs into public manifests, public docs, issue comments, or screenshots.

## Manifests

When the release includes Kubernetes examples, the base manifests live in:

```text
deploy/kubernetes/base/
deploy/kubernetes/overlays/example/
```

They include:

- `Namespace` and `ServiceAccount` with service account token mounting disabled;
- `ConfigMap` for non-secret router config;
- placeholder `Secret` example for provider env, license JSON, and Postgres DSN;
- `Deployment` with `/readyz` readiness/startup probes and `/healthz` liveness probe;
- `Service`, example `Ingress`, egress-focused base `NetworkPolicy`, `PersistentVolumeClaim`, and `PodDisruptionBudget`;
- an example overlay for image and ingress replacement.

The suggested layout is:

```text
namespace/
  router-config ConfigMap or mounted config artifact
  provider-keys Secret or external secret reference
  license Secret
  router Deployment
  router Service
  router Ingress or Gateway
  NetworkPolicy
```

The router container should run the packaged image tag for the target release, not `latest`. The config should point to mounted paths such as:

```yaml
server:
  listen: ":8080"
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}

state_path: /app/state/router-state.json
```

## Secrets And Config

Create deployment-owned secrets before applying the router workload. Do not commit real secret files.

```bash
kubectl create namespace smart-llmrouter

kubectl -n smart-llmrouter create secret generic smart-llmrouter-secrets \
  --from-literal=ROUTER_USAGE_DB_DSN='postgres://llmrouter:replace-with-password@postgres.example.internal:5432/llmrouter?sslmode=require' \
  --from-file=env.json=./env.json \
  --from-file=license.json=./license.json
```

The router config is mounted at `/app/config/config.yaml`. Provider keys are mounted at `/app/config/env.json`. The signed license is mounted read-only at `/app/config/license.json`. Durable license and router state are written under `/app/state`.

When a deployment configuration includes caller token hashes, browser-admin
credentials, or other sensitive deployment values, store the entire runtime
`config.yaml` in a Kubernetes Secret rather than a ConfigMap. Mount it beside
`env.json` so the router's adjacent-file loading behavior remains intact. Keep
the database DSN and any private CA material in the same protected runtime
secret or equivalent secret-manager integration; do not render those values
into checked-in manifests.

For automated delivery evidence, bind a runtime Secret by its Kubernetes UID
and `resourceVersion`, never by its data or a captured content checksum. A
replacement or update must invalidate prior apply/smoke proof and require a
fresh reviewed rollout and smoke before promotion. Do **not** grant the
delivery identity any Secret verb: Kubernetes cannot authorize a
metadata-only Secret `get`. Instead, have a separate secret-bootstrap identity
publish a policy-pinned, immutable, non-secret ConfigMap containing only a
schema version, Secret name, UID, and resourceVersion, with a same-namespace
Secret owner reference matching the attested UID. Grant delivery only
name-scoped `get` on that ConfigMap; do not allow any other ConfigMap or
Secret verb. Keep
the attestation outside the workload Kustomize inventory. Delete it before a
Secret mutation and recreate it only after bootstrap has read the new Secret
metadata, so a partial rotation fails closed.

For managed PostgreSQL, use TLS with hostname verification. Mount the
provider's CA bundle when the container trust store does not already contain
the required root, and reference that file from the DSN. Validate the database
connection from the router pod before publishing the Ingress.

For production, use an external secret manager or sealed-secret workflow if that is the cluster standard. Keep raw provider keys, router tokens, token hashes, license files, and DSNs out of tickets, screenshots, and public docs.

## Usage Database And State

Use external Postgres for production usage reporting:

```yaml
server:
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}
```

The example uses a PVC for file-backed router and license state. Back up the PVC or move state to a deployment-approved durable store if that is supported by your router version. Keep the initial deployment at one replica unless the state, quota, and license behavior has been validated for the chosen scaling design.

## Deploy

The base kustomization intentionally does not apply `secret.example.yaml`; create `smart-llmrouter-secrets` through your secret-management process first, then render and inspect the manifests before applying them:

```bash
kubectl kustomize deploy/kubernetes/overlays/example > /tmp/smart-llmrouter.yaml
kubectl apply --dry-run=server -f /tmp/smart-llmrouter.yaml
kubectl apply -f /tmp/smart-llmrouter.yaml
```

If server-side dry-run is unavailable, use client-side dry-run as a syntax check:

```bash
kubectl apply --dry-run=client -f /tmp/smart-llmrouter.yaml
```

For automated staging or production delivery, do not treat CI or Make
variables as approval for an account, cluster, namespace, or overlay. Keep a
reviewed environment target policy in an independently controlled store, give
the delivery identity read-only access to that exact policy, verify its account
and role before creating a kubeconfig, and reject any rendered resource outside
the approved namespace. Bind smoke and promotion evidence to the exact
immutable image digest from the exact approved registry/repository, a
digest-normalized fingerprint of the validated rendered configuration, live
Deployment pod-template/generation state, attested runtime Secret
UID/resourceVersion, and the immutable attestation ConfigMap identity.
Recheck that state before
promotion so a rollback or replacement cannot reuse stale smoke evidence. For
automated delivery, use a deployment-owned label to derive an allowlisted
namespace inventory for the router Deployment, Service, Ingress, NetworkPolicy,
PVC, PodDisruptionBudget, and ServiceAccount. Bind both resource identity and a
normalized declarative-configuration fingerprint derived from the isolated
client-rendered manifest. Treat server-side dry-run output only as a candidate
live object to validate against that desired fingerprint; never let fields
preserved by another field manager become expected configuration. Exclude only
documented Kubernetes runtime allocations, not routing or security settings,
and reject unexpected live labels, annotations, owner references, finalizers,
or spec fields before a mutation. For a `WaitForFirstConsumer` PVC, document
and normalize only the Kubernetes-owned selected-node annotation and
PVC-protection finalizer; do not broadly ignore PVC metadata. Verify both
before smoke and promotion. Do
not use a broad prune: a
removed or renamed resource should fail delivery until it is removed through a
separately approved migration. Treat rollback as an artifact deployment too:
require an explicitly approved immutable digest and full pod-template SHA-256
from approved release evidence, resolve a matching owned historical revision
before undoing, and verify the restored workload's complete template, digest,
and rollout identity before recording rollback success.

For a credentialed deployment smoke, keep the command in an owner-only
mode-`0600` shell script or equivalent protected CI file, and pass only its
path to the runner. Treat an arbitrary smoke script as a potentially mutating
staging action: it requires the same explicit confirmation and protected target
boundary as apply and rollback. The EKS runner opens the validated non-symlink
file once and executes that bound descriptor, so a later path replacement
cannot alter the command. Smoke evidence must bind to a passed apply that
predates it; any later apply invalidates the smoke for promotion. Do not
interpolate command content, router tokens, or
authorization headers into Make recipes, command arguments, CI logs, or
evidence files. Capture diagnostic output only in the protected runner.

Check rollout:

```bash
kubectl -n smart-llmrouter rollout status deploy/smart-llmrouter
kubectl -n smart-llmrouter get pods,svc,ingress
```

The generic base `NetworkPolicy` does not constrain ingress, so non-EKS users
must apply their own reviewed client/ingress policy before exposure. For EKS,
copy the reviewed `tenant-router-ingress-guard.example.yaml` into the
deployment-owned overlay before exposure; it denies ingress until discovery
selects the real ingress namespace. After the rollout is Ready, rerun the
explicit-target discovery so Linkerd mode can also verify durable ingress/router
injection and router Pods, then use the matching deployment bundle to render and activate
that companion policy. Do not substitute a fixed namespace in the overlay or
use an ambient `kubectl` context. The activation target verifies the discovery
account and selected EKS endpoint against the named AWS profile and explicit
kubeconfig/context snapshot before every server-side dry-run or apply, and uses
private copies of the exact rendered policy bytes rather than rereading caller
paths. It accepts only
discovery evidence generated within the preceding 15 minutes; rerun discovery
after that window rather than applying a stale ingress or Linkerd identity.

For a non-Linkerd deployment:

```bash
make eks-render-ingress-network-policy \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml

make eks-validate-tenant-network-policies \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_POLICY_AWS_PROFILE=<approved-deployment-profile> \
  EKS_POLICY_KUBECONFIG=/secure/kubeconfigs/approved-cluster.yaml \
  EKS_POLICY_CONTEXT=<approved-deployment-context> \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml

make eks-apply-tenant-network-policies \
  EKS_POLICY_APPLY_CONFIRM=apply \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_POLICY_AWS_PROFILE=<approved-deployment-profile> \
  EKS_POLICY_KUBECONFIG=/secure/kubeconfigs/approved-cluster.yaml \
  EKS_POLICY_CONTEXT=<approved-deployment-context> \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml
```

For a Linkerd deployment, render both reviewed artifacts instead. The same
target dry-runs both, then applies the Linkerd `Server` and
`ServerAuthorization` before the ingress allow policy:

```bash
make eks-render-linkerd-policy \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml

make eks-validate-tenant-network-policies \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_POLICY_AWS_PROFILE=<approved-deployment-profile> \
  EKS_POLICY_KUBECONFIG=/secure/kubeconfigs/approved-cluster.yaml \
  EKS_POLICY_CONTEXT=<approved-deployment-context> \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml

make eks-apply-tenant-network-policies \
  EKS_POLICY_APPLY_CONFIRM=apply \
  EKS_DISCOVERY_OUTPUT=/secure/evidence/eks-discovery.json \
  EKS_POLICY_AWS_PROFILE=<approved-deployment-profile> \
  EKS_POLICY_KUBECONFIG=/secure/kubeconfigs/approved-cluster.yaml \
  EKS_POLICY_CONTEXT=<approved-deployment-context> \
  EKS_INGRESS_NETWORK_POLICY_OUTPUT=/secure/evidence/tenant-ingress-network-policy.yaml \
  EKS_LINKERD_POLICY_OUTPUT=/secure/evidence/tenant-linkerd-policy.yaml
```

## Network Policy

The generic base policy limits egress for HTTPS, DNS, and an example private
Postgres CIDR but intentionally leaves ingress to the deployment. On EKS, add
the reviewed deny-ingress guard to the deployment-owned overlay before exposure;
the discovery-derived companion policy then becomes the selected ingress allow.
Update the deployment for:

- approved provider endpoints or private upstream ranges;
- external Postgres address ranges;
- internal observability endpoints if required.

Network policy enforcement depends on the cluster CNI. Validate both allowed and denied flows in staging.

## Smoke Tests

Port-forward for an internal smoke before exposing ingress:

```bash
kubectl -n smart-llmrouter port-forward svc/smart-llmrouter 18080:80
curl -fsS http://127.0.0.1:18080/readyz
curl -fsS http://127.0.0.1:18080/docs/
curl -fsS http://127.0.0.1:18080/version
```

Then test with a deployment-issued caller token:

```bash
kubectl -n <namespace> rollout status deploy/<router-deployment>
kubectl -n <namespace> get pods,svc,ingress

export ROUTER_BASE_URL="https://<router-host>"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS "$ROUTER_BASE_URL/readyz"
curl -fsS "$ROUTER_BASE_URL/docs/"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Run one small request through each client API shape that callers use:

- OpenAI Chat for `/v1/chat/completions`;
- OpenAI Responses or Codex CLI when enabled;
- Anthropic Messages or Claude Code when enabled;
- tool-call and image smokes for groups that advertise those capabilities.

When admin reports are enabled, verify browser-admin authentication and authorization separately. Ordinary caller tokens must not access `/admin/reports/` or `/metrics`.

## Upgrade And Rollback

Use immutable image tags and reviewed config changes. Before rollout:

1. Back up the router ConfigMap, Secret references, PVC or state backup, and usage database.
2. Push the new per-architecture image tag to the private registry.
3. Update the overlay image patch.
4. Run `kubectl apply --dry-run=server`.
5. Apply and wait for rollout.
6. Smoke `/readyz`, `/docs/`, `/v1/models`, one chat request, and admin reports if enabled.

Rollback uses the previous image tag and previous ConfigMap/Secret versions:

```bash
kubectl -n smart-llmrouter rollout undo deploy/smart-llmrouter
kubectl -n smart-llmrouter rollout status deploy/smart-llmrouter
```

If a database migration or config change caused the failure, restore from the pre-upgrade backup before resuming traffic.

## Troubleshooting

- Pod not ready: check license path, Postgres DSN, provider env file mount, and `/readyz` logs.
- `CrashLoopBackOff`: run `kubectl logs deploy/smart-llmrouter` and verify the mounted config parses.
- `/v1/models` empty: confirm the caller token allow-list and model group config.
- Provider errors: validate cluster egress, provider keys, and direct upstream smokes.
- Admin reports unavailable: confirm `server.admin_reports.enabled`, browser-admin auth, authorization grants, and usage DB connectivity.

The router never needs raw provider keys or router tokens in support screenshots. Share request IDs, status codes, sanitized logs, and safe configuration summaries instead.

Related pages:

- [Deployment Artifacts](./deployment-artifacts)
- [Package Validation And Security Checks](./package-validation)
- [Router Configuration](../configuration/router-config)
- [License-Protected Deployments](../operations/license-protected-deployments)
