# ACME production-like EKS deploy (no Compose cutover)

> **Internal plan for issue [#869](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/869).**
> This document grants no AWS, EKS, RDS, DNS, production, runtime-secret, or Compose-to-EKS mutation authority until the fail-closed prerequisites below pass.

## Decision

Deploy one isolated licensed customer-like Router for **ACME** at
`https://acme.apps.metrum.ai` on the shared `metrum` EKS cluster. Reuse the
current Compose production **upstream provider credential values and
provider/model-group routing contract** through a separate ACME runtime Secret
and protected `runtime_bundle_ref`. Keep
`https://llm-api-engg.metrum.ai` as the unchanged Compose production authority.

This is **not** `#518` production cutover.

Tracked work item: `task.acme_eks_production_like_deploy`.

## Target shape

| Field | Value |
| --- | --- |
| Hostname | `acme.apps.metrum.ai` |
| Cluster | shared `metrum` EKS (non-production customer-like instance) |
| Lifecycle CLI | `metrum-fleetctl` only |
| Image | clean reviewed `origin/main` pinned as `repository@sha256:...` |
| Upstream | production-equivalent provider keys + model groups in a **separate** ACME Secret/bundle |
| Identities | new ACME caller, browser-admin, license, PVC, dedicated RDS |
| TLS | existing `*.apps.metrum.ai` wildcard; exact Host rule only after activation |
| Database | dedicated private RDS (`#555` default) unless an explicit SQLite exception is recorded |

## Fail-closed sequence

1. **`#818` repair** — remove forbidden Deployment `delete` from the staging delivery Role/RoleBinding; obtain passing least-privilege delivery preflight under the approved delivery identity.
2. **Disposable EKS/RDS E2E** — deterministic `metrum-fleetctl plan`, externally issued mode-`0600` `--rds-admission-file` with `action: disposable-e2e`, then packaged disposable E2E including failure/retry and confirmed cleanup (PR `#868`).
3. **Recorded single-reviewer review** — checklist in [Recorded security and operations review](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md#recorded-security-and-operations-review); self-review by the implementing maintainer is permitted for this non-production target.
4. **ACME plan** — reference-only intent + protected profile + protected runtime bundle; no secrets in Git, NDJSON, argv, logs, or chat.
5. **ACME deploy** — `metrum-fleetctl deploy` with a fresh scoped admission bound to the ACME job/intent/namespace/manifest digest; activation before Host publish.
6. **Smoke** — `/readyz`, `/v1/models`, Chat/Responses/Messages as exposed, Codex/Claude Code for the coding group, ordinary-caller `/metrics` → `403 metrics-forbidden`, sanitized usage evidence.
7. **Retention** — record keep-or-destroy for namespace/RDS/PVC/Secrets/Host; Compose production untouched.

`#518` remains the sole authority for any later production-profile or Compose cutover work.

## Placeholder artifacts (safe)

Checked-in placeholders contain no credentials, DSNs, tokens, or production host secrets:

- Reference-only intent shape: [`testdata/acme-eks/manifest.placeholder.yaml`](../testdata/acme-eks/manifest.placeholder.yaml)
- RDS admission JSON shape: [`testdata/acme-eks/rds-admission.placeholder.json`](../testdata/acme-eks/rds-admission.placeholder.json)

Real protected files live only under mode-`0600` paths outside the repository (for example `/protected/acme/`).

## Operator commands (after gates pass)

```bash
# Read-only plan (no mutation)
rtk ./bin/metrum-fleetctl plan \
  --profile-ref <approved-nonproduction-profile-ref> \
  --manifest /protected/acme/intent.yaml \
  --intent-id <unique-acme-intent-id> \
  --output json

# Deploy only with a still-valid scoped admission
rtk ./bin/metrum-fleetctl deploy \
  --profile-ref <approved-nonproduction-profile-ref> \
  --manifest /protected/acme/intent.yaml \
  --intent-id <same-unique-acme-intent-id> \
  --rds-admission-file /protected/acme/rds-admission.json \
  --output json
```

## Open human decisions

Record answers in the protected approval channel; store only safe non-secret
identifiers in NDJSON evidence:

1. `#818` preflight owner session and passing evidence location (sanitized)?
2. Confirm production upstream credential reuse into a separate ACME Secret?
3. Dedicated RDS vs explicit SQLite exception?
4. Admission `issuer_role` alias for ACME?
5. Post-smoke retain vs destroy schedule and cleanup authority?

## Related

- Issue `#869`, epic `#555`, preflight `#818`, cutover `#518` (out of scope)
- PR `#868` scoped RDS admission
- `task.customer_eks_deploy_cli`, `task.compose_to_eks_cutover_gates`
- [Customer instance operations runbook](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md)
- [Fleet CLI contract](MULTI_ENVIRONMENT_DEPLOYMENT_CLI.md)
