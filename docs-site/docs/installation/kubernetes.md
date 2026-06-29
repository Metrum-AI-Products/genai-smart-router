---
title: Deploy To Kubernetes
---

# Deploy To Kubernetes

Kubernetes is a deployment pattern for GenAI Smart Router, but release packages do not currently include turnkey Kubernetes manifests or a Helm chart. A Kubernetes installation should be built from reviewed deployment-owned manifests that follow the same runtime contract as the binary and Docker Compose packages.

Track the Kubernetes packaging implementation issue for your release when packaged manifests or Helm support are required. Until then, treat the checklist below as the administrator plan for customer-owned manifests.

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

## Suggested Layout

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

## Smoke Tests

After rollout:

```bash
kubectl -n <namespace> rollout status deploy/<router-deployment>
kubectl -n <namespace> get pods,svc,ingress

export ROUTER_BASE_URL="https://router.example.com"
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

## Upgrade And Rollback

Use immutable image tags and reviewed config changes. Before rollout, back up or snapshot config, license state, usage database state, and any deployment-owned state volume. During rollout, watch `/readyz`, pod restarts, upstream error rates, request latency, and admin report health.

Rollback requires both the previous image and the previous compatible config or Secret set. Use the platform's rollout controls only after confirming database migrations, license state, and config changes are compatible with the previous release.

Related pages:

- [Deployment Artifacts](./deployment-artifacts)
- [Package Validation And Security Checks](./package-validation)
- [Router Configuration](../configuration/router-config)
- [License-Protected Deployments](../operations/license-protected-deployments)
