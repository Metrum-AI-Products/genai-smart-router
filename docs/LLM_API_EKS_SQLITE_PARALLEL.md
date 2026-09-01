# Metrum Fleet production tenant (llm-api)

> **Updated 2026-09-01:** This tenant is Metrum engineering **production**, not a
> parallel validation instance. Production hostnames
> `llm-api-engg.metrum.ai` and `llm-api.metrum.ai` CNAME to the Fleet primary
> hostname `llm-api.apps.metrum.ai`.

Canonical operations: **`docs/EKS_PRODUCTION_OPERATIONS.md`**.

## Tenant shape

| Field | Value |
| --- | --- |
| Hostnames | `llm-api.apps.metrum.ai`, `llm-api-engg.metrum.ai`, `llm-api.metrum.ai` |
| Namespace | `llm-api` |
| Cluster | shared `metrum` EKS (`us-east-1`) |
| Lifecycle CLI | `metrum-genai-smartrouter-fleetctl customer` (SQLite path) |
| Database | SQLite on tenant PVC |
| Runtime bundle | `aws-secretsmanager:///smartrouter/fleet/customers/llm-api/runtime-bundle` |
| Profile | protected SSM production profile with `approved_alias_hostnames` |

## Operator sequence

Use the copy-paste block in
[`docs/CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md`](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md#metrum-operator-quick-reference-sqlite-fleet-customers)
with `--customer-id llm-api` and the protected production refs.

Production caller tokens are carried by `token_sha256` in the runtime bundle
config (hashes only in the bundle; raw tokens never stored in git).

## Rollback

DNS rollback: restore DigitalOcean A records to the retained EC2 Compose host and
restart Compose router/Caddy. Application rollback on EKS:
`customer delete --sign-with-key` only when decommissioning the tenant entirely.

## Related

- [`docs/EKS_PRODUCTION_OPERATIONS.md`](EKS_PRODUCTION_OPERATIONS.md) — current runbook
- Historical: [`docs/EKS_STAGING_MIGRATION.md`](EKS_STAGING_MIGRATION.md),
  [`docs/LLM_API_ENGG_EKS_CUTOVER_PLAN.md`](LLM_API_ENGG_EKS_CUTOVER_PLAN.md)
