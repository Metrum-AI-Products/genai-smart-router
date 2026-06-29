---
title: Kubernetes Deployment
---

# Kubernetes Deployment

Use Kubernetes when GenAI Smart Router needs to run inside a customer-managed cluster with cluster-native ingress, Secrets, external Postgres, and operational controls. Metrum maintains raw Kustomize-friendly manifests under `deploy/kubernetes/` as a production-oriented starting point.

The manifests are examples. Review them against your cluster's ingress controller, network policy engine, storage class, registry, and secret-management process before production rollout. If you are installing from a binary or Docker Compose release package that does not include `deploy/kubernetes/`, obtain the matching Kubernetes manifest bundle from Metrum or from the reviewed source artifact for that release.

## Prerequisites

- A Kubernetes cluster with an ingress controller and TLS automation or a separate TLS termination plan.
- A private registry image tag such as `registry.example.com/smart-llmrouter:<version>-linux-amd64`.
- External Postgres for the usage database.
- A Metrum-issued `license.json`.
- Provider credentials stored in a Kubernetes Secret or external secret manager.
- A router config reviewed for the deployment's model groups, callers, admin auth, and reporting settings.

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

## Manifests

The base manifests live in:

```text
deploy/kubernetes/base/
deploy/kubernetes/overlays/example/
```

They include:

- `Namespace` and `ServiceAccount` with service account token mounting disabled;
- `ConfigMap` for non-secret router config;
- placeholder `Secret` example for provider env, license JSON, and Postgres DSN;
- `Deployment` with `/readyz` readiness/startup probes and `/healthz` liveness probe;
- `Service`, example `Ingress`, `NetworkPolicy`, `PersistentVolumeClaim`, and `PodDisruptionBudget`;
- an example overlay for image and ingress replacement.

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

Check rollout:

```bash
kubectl -n smart-llmrouter rollout status deploy/smart-llmrouter
kubectl -n smart-llmrouter get pods,svc,ingress
```

## Network Policy

The example policy allows ingress from a placeholder ingress controller namespace, HTTPS egress for external model providers, DNS egress, and Postgres egress to an example private CIDR. Update it for:

- the actual ingress controller namespace labels;
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
export ROUTER_BASE_URL="https://llm-api.example.com"
export ROUTER_TOKEN="replace-with-router-token"

curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

When admin reports are enabled, verify browser-admin authentication and authorization separately. Ordinary caller tokens must not access `/admin/reports/` or `/metrics`.

## Upgrade And Rollback

Before an upgrade:

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
- Admin reports unavailable: confirm `server.admin_reports.enabled`, browser-admin auth, Casbin grants, and usage DB connectivity.

The router never needs raw provider keys or router tokens in support screenshots. Share request IDs, status codes, sanitized logs, and safe configuration summaries instead.
