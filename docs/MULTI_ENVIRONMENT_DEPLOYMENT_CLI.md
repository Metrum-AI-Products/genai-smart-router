# Multi-environment deployment CLI safe contract

`metrum-smartrouterctl` currently implements only the ADR-0012 safe contract
slice. Its local SQLite registry is a relational inventory of non-secret
tenant, router-instance, dedicated-RDS allocation, schema-state, and local
quota-admission records. It does not store credentials, DSNs, endpoints,
tokens, secret references, or runtime configuration content.

The currently available safe commands are:

```text
metrum-smartrouterctl register ...
metrum-smartrouterctl quota-reserve ... --mock-quota-limit ...
metrum-smartrouterctl status ...
```

`register` accepts a deployment-defined stage label and records one isolated
router instance plus one explicit dedicated RDS allocation ID. It rejects every
placement other than `dedicated_instance` and every RDS Proxy mode other than
`disabled`. It derives neither an allocation name nor a database address from a
tenant name.

`quota-reserve` is a fake-adapter-only local admission reservation. It verifies
the scalar quota snapshot plus configured headroom and records an idempotent
local hold. It is not an AWS reservation; a later approved provisioner must
recheck and reconcile capacity immediately before resource creation.

`status` is bounded (1–100 rows), stable by tenant ID, and registry-only. It
reports schema-version drift and the still-open policy gates, but makes no live
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

Do not connect a live AWS, Kubernetes, DNS, RDS, Service Quotas, or RDS Proxy
adapter until the policy gates have been signed and the implementation receives
separate architecture and security review. This registry is intentionally
separate from router usage persistence and issue #507's forward-only migration
work.
