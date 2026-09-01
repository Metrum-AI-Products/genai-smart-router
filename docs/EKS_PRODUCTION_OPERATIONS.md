# Metrum EKS production operations

GenAI Smart Router production for Metrum engineering runs on the Fleet-managed
SQLite tenant `llm-api` in the shared `metrum` EKS cluster (`us-east-1`).

| Surface | Hostname |
| --- | --- |
| Primary Fleet hostname | `https://llm-api.apps.metrum.ai` |
| Production aliases | `https://llm-api-engg.metrum.ai`, `https://llm-api.metrum.ai` |

Docker Compose remains a **customer deployment option** documented in
`docs/DOCKER_COMPOSE_INSTALL.md` and `docs/DOCKER_DEPLOYMENT.md`. It is no longer
the Metrum production path.

## Authority and rollback

- **Writer:** one Fleet tenant (`customer_id=llm-api`), replica=1, SQLite on PVC.
- **Rollback window:** restore DigitalOcean A records to the retained EC2 Compose
  host and `docker compose start router caddy`. No reverse usage migration.
- **Usage history:** Compose Postgres archives are forensics-only; EKS SQLite
  starts fresh at cutover.

## Prerequisites

Authenticate to AWS, EKS, and DigitalOcean **before** operator commands. Login is
never part of numbered rollout steps.

```bash
rtk aws sts get-caller-identity
rtk kubectl config current-context   # must be metrum cluster
```

See `docs/EKS_PRODUCTION_DISCOVERY.md` for DNS and cluster discovery checks.

## Protected profile

Store the live profile in SSM (example fixture:
`deploy/release/fixtures/metrum-production-profile.example.yaml`):

- `environment: production`
- `hostname_suffix: apps.metrum.ai`
- `approved_alias_hostnames` for `llm-api-engg.metrum.ai` and `llm-api.metrum.ai`
- `tls_secret_name: apps-metrum-ai-wildcard-tls` for the primary hostname
- per-alias `tls_secret_name: llm-api-metrum-ai-tls`

Fleet derives the primary hostname as `llm-api.apps.metrum.ai` and reconciles a
multi-host Ingress after activation.

## Runtime bundle

Protected bundle:
`aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle`

Must contain:

- `config.yaml` with EKS paths (`/var/lib/smart-llmrouter`), trusted proxy
  `192.168.0.0/16`, production caller `token_sha256` values (not raw tokens),
  and browser-admin bcrypt hash.
- `env.json` with upstream provider keys (same env names as former Compose).

Validate a local config snapshot:

```bash
rtk python3 scripts/prepare_fleet_production_bundle.py /protected/runtime-config.production-identical.yaml
```

Deploy bundle updates with a new signed immutable intent:

```bash
export FLEET_PROFILE_REF='aws-ssm:///metrum/smartrouter/profiles/production'
export FLEET_RUNTIME_BUNDLE_REF='aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle'
# customer update-config / deploy per docs/CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md
```

## TLS for production aliases

1. Create namespace Secret `llm-api-metrum-ai-tls` in `llm-api` covering both
   `llm-api-engg.metrum.ai` and `llm-api.metrum.ai` (cert-manager DNS-01 preferred).
2. Pre-verify before DNS flip:

```bash
export INGRESS_LB_IP="$(dig +short k8s-ingressn-ingressn-*.elb.us-east-1.amazonaws.com | head -1)"
rtk bash scripts/eks_production_cutover.sh verify-ingress
```

3. Redeploy or resume Fleet so `EnableHostname` reconciles Ingress rules/TLS.

## Cutover sequence

1. **Archive Compose usage** (forensics):

```bash
rtk bash scripts/archive_compose_usage.sh --remote ubuntu@HOST --install-root /opt/smart-llmrouter
```

2. **Lower TTL** on DigitalOcean A records; wait for expiry.
3. **CNAME** both production hostnames to `llm-api.apps.metrum.ai`.
4. **Acceptance** on production hostnames (not only the apps hostname):

```bash
rtk bash scripts/eks_production_cutover.sh preflight
curl -fsS https://llm-api-engg.metrum.ai/readyz
curl -fsS https://llm-api-engg.metrum.ai/version
# authenticated /v1/models, /metrics 403 for ordinary callers
# Codex CLI + Claude Code CLI smokes per AGENTS.md
```

5. **Compose standdown** (containers only; instance may stay up if stop-protected):

```bash
rtk bash scripts/eks_production_cutover.sh compose-standdown
```

## Routine operations

| Task | Command surface |
| --- | --- |
| Status | `metrum-genai-smartrouter-fleetctl customer status --customer-id llm-api` |
| Smoke | `metrum-genai-smartrouter-fleetctl customer smoke --customer-id llm-api` |
| Config update | signed intent + `customer update-config` |
| Image refresh | new digest in profile + deploy |
| Rollback DNS | restore A records + restart Compose |

## Production promotion record

Maintainer self-review authorizing Metrum production on Fleet (2026-09-01 UTC):

- Profile: `metrum-production` (`environment: production`)
- Tenant: `llm-api` / namespace `llm-api`
- Alias hostnames: `llm-api-engg.metrum.ai`, `llm-api.metrum.ai`
- Database: SQLite on tenant PVC (Postgres deferred)
- Code gate: `environment: production` accepted by Fleet profile validator
- Ingress: multi-host reconciliation in `EnableHostname`
- Evidence: `rtk go test ./internal/router/...`, `rtk make test-tenant-deploy-all`

See `docs/PRODUCTION_PROMOTION_CONTRACT.md` for the repository-side contract.

## Related

- `docs/CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md` — Fleet operator quick reference
- `docs/EKS_PRODUCTION_DISCOVERY.md` — discovery checklist
- Historical: `docs/EKS_STAGING_MIGRATION.md`, `docs/LLM_API_ENGG_EKS_CUTOVER_PLAN.md`
