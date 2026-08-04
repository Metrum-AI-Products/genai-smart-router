# Multi-environment deployment CLI safe contract

`metrum-smartrouterctl` currently implements only the ADR-0012 safe contract
slice. Its local SQLite registry is a relational inventory of non-secret
tenant, router-instance, dedicated-RDS allocation, schema-state, and local
quota-admission records. It does not store credentials, DSNs, endpoints,
tokens, secret references, or runtime configuration content.

The CLI is shipped in binary tarballs. It is not included in the standard
Docker or Docker Compose image; Docker-based operators run it from an extracted
binary package on a separate trusted administration host.

The currently available safe commands are:

```text
metrum-smartrouterctl register ...
metrum-smartrouterctl observe-schema ...
metrum-smartrouterctl quota-reserve ... --mock-quota-limit ...
metrum-smartrouterctl status ...
```

`register` accepts a deployment-defined stage label and records one isolated
router instance plus one explicit dedicated RDS allocation ID. It requires
separate `--schema-version` and `--current-schema-version` values, so the
current version is never inferred from expected deployment metadata. Both
versions must be between 0 and 2147483647. Registration rejects every placement
other than `dedicated_instance` and every RDS Proxy mode other than `disabled`.
It derives neither an allocation name nor a database address from a tenant
name. Repeating an unchanged registration remains idempotent after an
observation and preserves the stored current version and observation time;
immutable contract conflicts still fail closed.

`observe-schema` selects exactly one registered tenant and stage, records the
explicit `--current-schema-version`, and assigns a server-generated UTC
observation time. It updates only the local current-version observation;
expected version and desired release digest remain immutable. Missing,
ambiguous, malformed, and out-of-range observations fail closed. The command
does not contact the router database, a deployment endpoint, Kubernetes, AWS,
RDS, DNS, a credential store, or any adapter.

It opens only an existing registry and never initializes a missing registry as
a side effect of a failed observation.

`quota-reserve` is a fake-adapter-only local admission reservation. It verifies
the scalar quota snapshot plus configured headroom and records an idempotent
local hold. It is not an AWS reservation; a later approved provisioner must
recheck and reconcile capacity immediately before resource creation.

The required `--reservation` value is a bounded opaque idempotency key in
`rsv-<lowercase-canonical-uuid>` form, for example
`rsv-018f4f47-7d2b-7e2a-9c35-6b8c85f71942`. Token-like, credential-like, URL,
path, uppercase, malformed, and unbounded values are rejected before any quota
adapter call or quota-record write. An exact retry of a pre-existing bounded
opaque legacy reservation row remains readable and idempotent; that
compatibility path never creates a new hold, and another identifier cannot
bypass the active hold.

`status` is bounded (1–100 rows), stable by tenant ID, and registry-only. It
reports `schema_version_mismatch` after registration when the independently
observed current version differs from expected. A later `observe-schema` call
can return the status to `current`. Status remains read-only and makes no live
health, Kubernetes, RDS, DNS, or credential calls.

`deploy`, `promote`, and `rollback` fail closed. Live resource creation,
updates, deletion, configuration changes, and RDS Proxy remain disabled while
the endpoint, encryption, TLS, durability/backup, HA, network/subnet/security
group, and DNS policies require human approval. Cleanup remains a separately
confirmed future operation.

## Validation and future rollout

Run the registry unit and CLI contract tests before changing the safe contract:

```bash
rtk go test ./internal/router ./cmd/metrum-smartrouterctl
```

Exercise the shipped workflow as `register` with separate expected/current
versions, `status`, `observe-schema`, and `status` again. Rollback may remove
the observation command, but must not rewrite previously recorded observations.

Do not connect a live AWS, Kubernetes, DNS, RDS, Service Quotas, or RDS Proxy
adapter until the policy gates have been signed and the implementation receives
separate architecture and security review. This registry is intentionally
separate from router usage persistence and issue #507's forward-only migration
work.
