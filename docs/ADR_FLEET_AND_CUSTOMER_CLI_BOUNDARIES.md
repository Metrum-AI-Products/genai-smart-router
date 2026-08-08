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

## Dedicated RDS boundary

A protected non-production profile may select `database_mode: dedicated-rds`.
The plan contains only deterministic database identity and profile scalars—no
DSN, hostname, port, credentials, secret reference, or database response.
The normalized lifecycle records retries and ownership-safe retention exactly
as for the PVC path. The fake adapter is the executable contract. The typed
live EKS adapter rejects RDS mutation with `rds_live_admission_required` until
independent non-production RDS adapter, credential-binding, activation, and
security review evidence is accepted. Production profiles remain rejected
until #518.

## Consequences

- A customer cannot accidentally use a support CLI to provision or activate an
  environment.
- A Fleet operator cannot obtain Router/provider credentials or DSNs from
  plans, registries, statuses, or error output.
- Compose-to-EKS cutover remains an externally gated operation; this ADR grants
  no production, DNS, secret, credential, or migration mutation authority.
