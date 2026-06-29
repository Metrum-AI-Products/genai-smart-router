# Kubernetes Deployment Bootstrap

This package does not include turnkey Kubernetes manifests or a Helm chart. Kubernetes is a supported deployment pattern when the operating team supplies reviewed manifests that follow the same runtime contract as the binary and Docker Compose packages.

Use the embedded `/docs/` site after startup for the full Kubernetes deployment planning guide.

## Required Kubernetes Objects

A Kubernetes deployment normally needs:

- a namespace owned by the platform team;
- a ConfigMap for reviewed router config when policy allows config in ConfigMaps;
- Secrets for provider keys, caller-token hashes or generated config fragments, browser-admin credentials, and the issued `license.json`;
- an external Postgres database or a separately managed in-cluster database;
- a Deployment for stateless router pods plus persistent state only where required by the deployment design;
- a Service and Ingress or Gateway with TLS;
- readiness and liveness probes for `/readyz` and `/healthz`;
- resource requests and limits sized for expected concurrent requests;
- NetworkPolicy that permits only required clients, admin paths, database access, and upstream providers or private model services.

## Bootstrap Validation

Before sending production traffic:

```bash
kubectl -n <namespace> rollout status deploy/<router-deployment>
kubectl -n <namespace> get pods,svc,ingress

curl -fsS https://router.example.com/readyz
curl -fsS https://router.example.com/docs/
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" \
  https://router.example.com/v1/models
```

Run one small Chat, Responses, or Anthropic Messages smoke for each client API shape the deployment exposes.

## Upgrade And Rollback

Use normal Kubernetes rollout controls with an immutable image tag and a reviewed config or Secret change. Back up config, license state, and usage database state before rollout. Roll back with `kubectl rollout undo` only when the previous image and configuration are still available and compatible with the current database state.

For packaged manifests or Helm support, track the Kubernetes packaging implementation issue for the release you are deploying.
