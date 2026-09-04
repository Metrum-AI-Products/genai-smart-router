# Production promotion contract (historical)

> **Record (2026-09-01):** Metrum engineering production is authorized on Fleet
> EKS tenant `llm-api` with protected profile `metrum-production`
> (`environment: production`), alias hostnames for `llm-api-engg.metrum.ai` and
> `llm-api.metrum.ai`, and SQLite on the tenant PVC.

Executable Make-target / promotion-manifest body formerly in this file is
retired. Operators follow
[`docs/EKS_PRODUCTION_OPERATIONS.md`](EKS_PRODUCTION_OPERATIONS.md).

Maintainer self-review and routine deploy/rollback live in that runbook.
Dated cutover evidence: [`deployment.md`](../deployment.md).
