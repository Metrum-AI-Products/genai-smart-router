# ACME production-like EKS deployment result (no Compose cutover)

> **HISTORICAL — internal execution record only (2026-08-10).** Do not repeat
> CLI sequences from this page; use the canonical
> [Customer Instance Operations Runbook](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md#metrum-operator-quick-reference-sqlite-fleet-customers)
> for current SQLite Fleet customers.

> **Internal execution record for issue [#869](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/869).**
> This execution record grants no authority to repeat EKS/RDS, DNS, runtime-secret, production, or Compose-to-EKS mutation.

## Completed outcome

On 2026-08-10, Fleet deployed, validated, and then authorizedly destroyed one
isolated, production-like ACME Router on the shared `metrum` EKS cluster.
`https://acme.apps.metrum.ai` served only after activation; the exact ACME
namespace, dedicated RDS, PVC, runtime bundle, license binding, and Ingress
belonged to the Fleet job. The job finished `deleted` with
`database_state=deleted`.

The test bundle contained the protected production-equivalent upstream routing
contract, including all 25 deployed model groups. It was bound only through a
separate runtime Secret and protected `runtime_bundle_ref`; no values entered
Git, NDJSON, command arguments, logs, or this record.

`https://llm-api-engg.metrum.ai` was then still Compose production (unchanged by
this ACME rehearsal). As of 2026-09-01, Metrum production is Fleet EKS; see
[EKS production operations](EKS_PRODUCTION_OPERATIONS.md). This record is not
authorization to repeat the ACME disposable path.

## Target shape

| Field | Value |
| --- | --- |
| Hostname | `acme.apps.metrum.ai` |
| Cluster | shared `metrum` EKS (non-production customer-like instance) |
| Lifecycle CLI | `metrum-fleetctl` only |
| Image | clean reviewed `origin/main` pinned as `repository@sha256:...` |
| Upstream | production-equivalent provider keys + model groups in a **separate** ACME Secret/bundle |
| Identities | new ACME caller, browser-admin, license, and PVC |
| TLS | existing `*.apps.metrum.ai` wildcard; exact Host rule only after activation |
| Database | SQLite single-writer default; dedicated private RDS only with an explicit approved `database_profile` |
| Compute | Optional manifesto `compute_profile`; default `t3a.medium` from protected `approved_compute_profiles` (K8s scheduling only) |

## Completed gate and smoke evidence

1. **`#818` repair** — completed 2026-08-09. The delivery Role/RoleBinding
   reconciliation passed protected preflight with deletion verbs denied.
2. **Disposable EKS/RDS E2E** — completed with the external, signed,
   mode-`0600` admission; it exercised failure/retry and confirmed cleanup.
3. **Recorded security and operations review** — completed as permitted
   single-maintainer self-review for this non-production target.
4. **ACME lifecycle** — Fleet derived `namespace=acme` from `customer_id`,
   created the dedicated RDS, bound `ROUTER_USAGE_DB_DSN` only through the
   runtime environment, activated the Router, then published the exact Host.
5. **Smokes and isolation** — cluster and public-host `/readyz` passed;
   `/v1/models` returned 25 groups including `default` and `big-coder`;
   ordinary caller `/metrics` returned `403`; owned-resource isolation passed.
6. **Cleanup** — the separately authorized delete completed, including
   `database_state=deleted`. Compose production remained untouched.

The sanitized execution outcome is recorded in
[#869](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/869#issuecomment-5245520939).
Metrum production operations after the 2026-09-01 cutover:
[EKS production operations](EKS_PRODUCTION_OPERATIONS.md).

## Placeholder artifacts (safe)

Checked-in placeholders contain no credentials, DSNs, tokens, or production host secrets:

- Signed reference-only intent shape: `metrum.ai/smartrouter-deployment-intent/v1` under protected storage
- RDS admission JSON shape: [`testdata/acme-eks/rds-admission.placeholder.json`](../testdata/acme-eks/rds-admission.placeholder.json)

Real protected files live only under mode-`0600` paths outside the repository (for example `/protected/acme/`).

## Historical command shape

The following commands illustrate the protected reference-only lifecycle shape
used by the completed rehearsal; they are not an authorization to repeat it:

```bash
rtk ./bin/metrum-fleetctl plan \
  --intent /protected/acme/acme2-deploy-intent.json \
  --output json

rtk ./bin/metrum-fleetctl deploy \
  --intent /protected/acme/acme2-deploy-intent.json \
  --rds-admission-file /protected/acme/rds-admission.json \
  --output json
```

## Related

- Issue `#869`, epic `#555`, preflight `#818` (historical ACME disposable path)
- PR `#868` scoped RDS admission
- Current Metrum production: [EKS production operations](EKS_PRODUCTION_OPERATIONS.md)
- [Customer instance operations runbook](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md)
- [Fleet CLI contract](MULTI_ENVIRONMENT_DEPLOYMENT_CLI.md)
