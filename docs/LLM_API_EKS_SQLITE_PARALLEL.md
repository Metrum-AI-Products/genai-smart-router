# Parallel EKS SQLite instance at llm-api.apps.metrum.ai (no Compose cutover)

> **Internal execution record for the first Compose-to-EKS cutover step tracked by
> [#725](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/725).**
> This record grants no authority to repeat DNS cutover, production-profile
> mutation, Postgres usage migration, or `#518` promotion.

## Purpose

Stand up one **parallel**, **non-production** Fleet customer Router at
`https://llm-api.apps.metrum.ai` on the shared `metrum` EKS cluster. The
instance uses **SQLite** on its tenant PVC (`state_profile: sqlite-rwo-small`);
it does **not** provision dedicated RDS or copy the Compose Postgres usage
database.

The legacy Docker Compose deployment at `https://llm-api-engg.metrum.ai` and
`https://llm-api.metrum.ai` remains the **production authority** on Postgres
until a separately approved `#518` cutover.

## Target shape

| Field | Value |
| --- | --- |
| Hostname | `llm-api.apps.metrum.ai` |
| Namespace | `llm-api` (from `customer_id`) |
| Cluster | shared `metrum` EKS (non-production Fleet customer instance) |
| Lifecycle CLI | `metrum-genai-smartrouter-fleetctl customer` (SQLite path only) |
| Database | SQLite on tenant PVC; Fleet rewrites `server.usage_db` to `sqlite` + `auto-safe` at secret bind |
| Upstream contract | Compose production `env.json` keys plus `runtime-config.production-identical.yaml` provider/model-group routing (EKS `/var/lib/smart-llmrouter` paths) in a **separate** customer runtime Secret |
| Identities | new llm-api caller, browser-admin, license, and PVC — not copied Compose tokens |
| TLS | existing `*.apps.metrum.ai` wildcard; exact Host rule only after activation |
| Compose | unchanged Postgres usage and caller tokens |

## Explicit non-goals

- No `database_profile` or `--rds-admission-file` on this path.
- No logical Postgres export/import from Compose (`#507` / `#518` cutover work).
- No DigitalOcean DNS change for `llm-api-engg.metrum.ai` or `llm-api.metrum.ai`.
- No `#518` production-profile authorization.

## Operator sequence (SQLite customer path)

> **Historical (2026-08-19):** The first `llm-api` parallel deploy used manual
> Secrets Manager uploads, Python YAML assembly, and kubectl scale workarounds.
> The canonical operator path is packaged CLI `publish-runtime-bundle` /
> `customer bootstrap` with `--rewrite-paths fleet-eks`.

Use packaged Fleet binaries only. Protected refs and signed intents stay in
mode-`0600` paths outside Git.

```bash
export METRUM_FLEET_BIN_DIR=/path/to/release/bin
export FLEET_PROFILE_REF='aws-ssm:///metrum/smartrouter/profiles/staging'
export FLEET_LICENSE_REF='aws-ssm:///metrum/smartrouter/fleet/llm-api/license-request'

rtk make test-tenant-deploy-all

metrum-genai-smartrouter-fleetctl customer bootstrap \
  --customer-id llm-api \
  --profile-ref "$FLEET_PROFILE_REF" \
  --license-ref "$FLEET_LICENSE_REF" \
  --config-file /protected/runtime-config.production-identical.yaml \
  --env-file /protected/env.json \
  --rewrite-paths fleet-eks \
  --sign-with-key /protected/lifecycle_approval_private_key.b64 \
  --owner-user llm-api-admin --project llm-api-eks \
  --allow-from-config \
  --token-out ~/.local/share/metrum-fleet/llm-api/CALLER_TOKEN_ADMIN.txt \
  --model high
```

Step-by-step equivalent:

```bash
metrum-genai-smartrouter-fleetctl customer publish-runtime-bundle \
  --customer-id llm-api \
  --config-file /protected/runtime-config.production-identical.yaml \
  --env-file /protected/env.json \
  --strip-callers --rewrite-paths fleet-eks

metrum-genai-smartrouter-fleetctl customer write-manifest --customer-id llm-api \
  --profile-ref "$FLEET_PROFILE_REF" \
  --runtime-bundle-ref 'aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle' \
  --license-ref "$FLEET_LICENSE_REF"

metrum-genai-smartrouter-fleetctl customer create \
  --sign-with-key /protected/lifecycle_approval_private_key.b64

metrum-genai-smartrouter-fleetctl customer grant-caller --customer-id llm-api \
  --profile-ref "$FLEET_PROFILE_REF" \
  --runtime-bundle-ref 'aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle' \
  --license-ref "$FLEET_LICENSE_REF" \
  --owner-user llm-api-admin --project llm-api-eks --allow-from-config \
  --token-out ~/.local/share/metrum-fleet/llm-api/CALLER_TOKEN_ADMIN.txt

metrum-genai-smartrouter-fleetctl customer create \
  --sign-with-key /protected/lifecycle_approval_private_key.b64

metrum-genai-smartrouter-fleetctl customer smoke --customer-id llm-api \
  --token-file ~/.local/share/metrum-fleet/llm-api/CALLER_TOKEN_ADMIN.txt \
  --model high
```

Validate:

- `https://llm-api.apps.metrum.ai/readyz` returns 200 after activation.
- Authenticated `/v1/models` lists deployment-defined groups.
- Ordinary caller `/metrics` returns `403 metrics-forbidden`.
- Compose `https://llm-api-engg.metrum.ai/readyz` still returns 200.

Record only sanitized scalars (job id, hostname, image digest, HTTP status
buckets) in `deployment.md` and NDJSON. Never commit tokens, bundle values,
provider keys, or DSNs.

## Runtime bundle source (Compose parity)

The protected runtime bundle at
`aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle`
must carry the same upstream provider keys and active model-group routing as
Compose production, but with Fleet SQLite path rewrites:

- **`env.json`:** copied from the live Compose `env.json` snapshot under
  `~/.local/share/metrum-fleet/production-sync/` (same provider key env names).
- **`config.yaml`:** derived from
  `runtime-config.production-identical.yaml` in that directory (25 model groups,
  15 providers, EKS-safe `/var/lib/smart-llmrouter` state/logging paths). Do
  **not** reuse Compose `/app/state` paths on Fleet EKS; the tenant PVC mounts
  at `/var/lib/smart-llmrouter`.
- **Callers:** instance-specific probe token via `customer grant-caller`; never
  copy Compose `ROUTER_TOKEN*.txt` values into the bundle.
- **Size limit:** the bundle must stay within Secrets Manager's 64 KiB limit;
  use the checked-in production-identical YAML rather than the full 190 KiB
  Compose config file.

Fleet `deploy` still rewrites `server.usage_db` to SQLite + `auto-safe` at
secret bind time.

## 2026-08-19 validation (sanitized)

| Check | Result |
| --- | --- |
| Fleet job | `job-1777abc87d3bf4b6a6bd`, state `ready` |
| Hostname | `llm-api.apps.metrum.ai` |
| `/readyz` | 200 |
| `/v1/models` (probe caller) | 200, 25 groups |
| `/v1/chat/completions` group `high` | 200, exact `OK` |
| `/metrics` (ordinary caller) | 403 |
| Compose `/readyz` | 200 (unchanged Postgres authority) |

## Rollback

Application rollback for this parallel instance is a signed Fleet
`customer delete` with a job-bound delete approval. It does **not** roll back
Compose production. Retain or delete the tenant PVC per the approved retention
decision.

## Related

- [#725](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/725) — future EC2-to-EKS migration tracker
- [#518](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/518) — production promotion / cutover authority (out of scope)
- [#555](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/555) — Fleet lifecycle
- [`docs/ACME_EKS_PRODUCTION_LIKE_DEPLOY.md`](ACME_EKS_PRODUCTION_LIKE_DEPLOY.md) — completed RDS rehearsal (destroyed)
- [`docs/EKS_STAGING_MIGRATION.md`](EKS_STAGING_MIGRATION.md) — staging-only Make overlay (`smartrouter.apps.metrum.ai`)
- [`task.llm_api_eks_sqlite_parallel`](../work-items.ndjson) — tracked work item
- [`task.compose_to_eks_cutover_gates`](../work-items.ndjson) — remains blocked on `#518`
