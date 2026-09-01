# EKS production discovery (2026-09-01)

Historical note: captured during the Compose-to-Fleet production cutover. Re-run
these checks before any DNS or TLS change; values drift with cluster upgrades.

## DNS (read-only probes)

| Hostname | Record type | Target |
| --- | --- | --- |
| `llm-api-engg.metrum.ai` | A | `54.84.22.33` (EC2 Compose, pre-cutover) |
| `llm-api.metrum.ai` | A | same EC2 host (pre-cutover) |
| `llm-api.apps.metrum.ai` | CNAME | `k8s-ingressn-ingressn-*.elb.us-east-1.amazonaws.com` → `52.3.128.72` |

Both public endpoints returned HTTP 200 on `/readyz` before cutover (parallel
validation instance on EKS, Compose authority on EC2).

## Cluster access

Authenticate to AWS and the `metrum` EKS cluster out of band before operator
steps. `kubectl` against the cluster requires a current kubeconfig; the lifecycle
user alone is insufficient.

Discovery checklist (run after auth):

```bash
rtk kubectl get svc -A -l app.kubernetes.io/name=ingress-nginx
rtk kubectl -n ingress-nginx get pods -o wide
rtk kubectl get clusterissuer,certificate -A 2>/dev/null || true
rtk kubectl -n llm-api get ingress router -o yaml
rtk kubectl -n llm-api get secret
```

Record:

- ingress-nginx external hostname and namespace
- pod CIDR for `server.client_ip.trusted_proxy_cidrs` (expect `192.168.0.0/16`)
- whether cert-manager issuers exist for `metrum.ai` SAN certificates
- namespace-local TLS Secrets (`apps-metrum-ai-wildcard-tls`, production alias cert)

## Compose usage database

Production Compose uses Postgres via `ROUTER_USAGE_DB_DSN` in `compose/.env`
when `docker-compose.postgres-localhost.yml` is active. Archive with
`scripts/archive_compose_usage.sh` before decommission; EKS production remains
SQLite on the Fleet tenant PVC.

## TLS path decision

| Path | When to use |
| --- | --- |
| cert-manager DNS-01 | Cluster has issuer + DNS API access for `metrum.ai` |
| cert-manager HTTP-01 | After DNS points at ingress (short cert gap possible) |
| External Secret | No cert-manager or no automated DNS; manual renewal required |

The existing `*.apps.metrum.ai` wildcard does **not** cover
`llm-api-engg.metrum.ai` or `llm-api.metrum.ai`.
