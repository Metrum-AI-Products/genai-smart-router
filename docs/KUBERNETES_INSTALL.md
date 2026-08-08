# Kubernetes Deployment Bootstrap

The checked-in generic manifests provide a one-router SQLite installation for a fresh PVC. They use one replica, `Recreate`, and `ReadWriteOnce` storage so `/app/state/usage.sqlite` has exactly one active writer. The base includes no database DSN, credentials, or TCP/5432 egress.

## Fresh-PVC Bootstrap

Set the same immutable image in `deploy/kubernetes/overlays/sqlite-bootstrap/job.yaml` and `deploy/kubernetes/overlays/example/patch-image.yaml`. Use this path only when `smart-llmrouter-state` does not already exist; existing databases use the reviewed upgrade migration runbook.

```bash
kubectl apply -k deploy/kubernetes/overlays/sqlite-bootstrap
kubectl -n smart-llmrouter wait --for=condition=complete job/smart-llmrouter-sqlite-bootstrap --timeout=10m
kubectl -n smart-llmrouter logs job/smart-llmrouter-sqlite-bootstrap -c migration-verify-serving
kubectl -n smart-llmrouter delete job smart-llmrouter-sqlite-bootstrap
kubectl apply -k deploy/kubernetes/overlays/example
kubectl -n smart-llmrouter rollout status deployment/smart-llmrouter --timeout=10m
```

Never use `delete -k` for the bootstrap overlay: it can remove shared resources or the PVC. The bootstrap Job serializes `version`, `plan`, `apply`, zero-row `resume`, and `verify-serving` while no serving Deployment mounts the claim. `verify-serving` fails unless the migration ledger is current/compatible and every bound data job is validated.

For multi-replica or externally managed database deployments, select PostgreSQL explicitly: provide a deployment-owned DSN Secret, configure `server.usage_db.driver: postgres` with that DSN, and add narrowly scoped database egress. Do not share SQLite storage between router replicas.

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
