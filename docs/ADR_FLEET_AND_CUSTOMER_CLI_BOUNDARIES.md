# ADR: Separate fleet lifecycle authority from customer operations

- **Status:** Accepted, 2026-08-08
- **Scope:** #555 lifecycle packages and customer-local Router operations.

## Decision

`metrum-fleetctl` is the only Fleet authority. It owns the one #555 lifecycle
registry and the deterministic `plan`, idempotent `deploy`, exact-job `status`,
and separately approved `delete` vocabulary. Its reference-only manifests,
protected profiles, ownership labels, activation evidence, and retention
approvals are the only deployment authority. The legacy
`metrum-smartrouterctl` binary exists for one release only; it exits after
reporting the rename and performs no operation.

`smartrouterctl` is customer-local and ships in both binary and Router Docker
packages. It can validate or safely compare local config, generate a caller
token into a new mode-`0600` file exactly once, inspect safe config/license/
model status, and print aggregate usage. It cannot invoke AWS, EKS, RDS, DNS,
Fleet registries, cross-customer operations, config activation, key rotation,
or license signing.

Fleet binaries are included only in binary tarballs. Customer Docker images
contain `smartrouterctl`, never `metrum-fleetctl` or the compatibility binary.

## Runtime bundle boundary

The manifest carries exactly one `runtime_bundle_ref`, an
`aws-ssm:///` or `aws-secretsmanager:///` reference without query data. It
never carries `config.yaml`, `env.json`, provider credentials, or any other
runtime value. The typed resolver reads the protected value only in memory and
accepts a single JSON object with exactly two string fields: `config.yaml`
(a YAML mapping) and `env.json` (a JSON string map). Malformed payloads,
unknown fields, empty values, and raw secret-shaped manifest content fail with
safe generic errors.

The adapter writes those two exact keys to the owned `router-runtime` Secret
and mounts it read-only at `/app/config`, the Router image's startup path.
`router-license` remains a distinct one-key `license.json` Secret and mount.
Neither protected bundle values nor references enter plans, lifecycle records,
statuses, or error output.

## Dedicated RDS boundary

The default deployment path is SQLite state with one Router container and one
replica; it neither provisions nor binds RDS. A protected non-production
profile may permit an explicit approved `database_profile` manifest branch for
`database_mode: dedicated-rds`. That plan contains only deterministic database
identity and profile scalars—no DSN, hostname, port, credentials, secret
reference, or database response.
The normalized lifecycle records retries and ownership-safe retention exactly
as for the PVC path. `tenant_deployment_rds.go` provides the typed AWS RDS
adapter for a private, encrypted PostgreSQL instance with ownership tags,
RDS-Proxy-disabled policy, final-snapshot deletion, and scalar-only evidence.
It is unattached by the default EKS constructor. The existing `deploy` and
`delete` verbs can attach it only by consuming an externally issued,
mode-`0600`, time-bounded non-production disposable-E2E admission, signed by
the approved profile's `lifecycle_approval_public_key`, and bound to the exact
profile, deterministic job/intent, namespace, database profile, and manifest
digest. `metrum-fleetctl` never creates, updates, emits, or persists that
admission or signing material. Invalid, unsigned, stale, or out-of-scope
records fail before registry or AWS/EKS clients are opened. Production profiles
remain rejected until #518.

## Review authority

The security/operations review needs one qualified reviewer. A
single-maintainer deployment may self-review, and the reviewer may be the
person who implemented the change. The external disposable-E2E admission
allows only the first bounded E2E after #818 preflight; it is neither a second
review nor a production authorization. Once the E2E evidence exists, the
single review is recorded before the authorized production-like non-production
rehearsal and cites the approved profile/intent IDs, immutable digest,
config/license revisions, passing suite and disposable-E2E results, isolation
and secret-handling checks, and the retention/rollback decision, per [Recorded
security and operations
review](CUSTOMER_INSTANCE_OPERATIONS_RUNBOOK.md#recorded-security-and-operations-review).
Separate reviewers or additional scoped EKS roles may be used when more than
one qualified person is available or a customer contract demands separation of
duties. Production cutover authority remains #518's.

## Consequences

- A customer cannot accidentally use a support CLI to provision or activate an
  environment.
- A Fleet operator cannot obtain Router/provider credentials or DSNs from
  plans, registries, statuses, or error output.
- Compose-to-EKS cutover remains an externally gated operation; this ADR grants
  no production, DNS, secret, credential, or migration mutation authority.
