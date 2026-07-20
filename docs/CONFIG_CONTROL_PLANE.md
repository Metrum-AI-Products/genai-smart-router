# Configuration control-plane foundation

Phase 1 introduces a migration-scoped, relational configuration projection for
future managed deployments. It is not enabled by router startup and does not
replace `config.yaml`; YAML remains the only production configuration source
in this phase.

The `router_config` migration scope creates scalar, foreign-keyed records for:

- versioned configuration-set metadata and one active set per runtime scope;
- server settings, providers, provider headers and model catalog metadata;
- model groups and ordered weighted targets; and
- caller identities, token hashes, rate limits, and allowed groups.

Raw provider credentials and raw caller tokens have no column in this schema.
Provider credentials remain environment/secret references; caller rows retain
only an existing SHA-256 token hash and public token ID.

`LoadActiveConfigFromDB` is deliberately read-only. It loads only a validated
active set and reuses `Config.Validate`; missing active state and invalid
relational data fail closed. Runtime DB mode, YAML import/export, write APIs,
activation/rollback actions, PostgREST, Caddy, secret persistence, and hot
reload remain later issue #7 phases.

For migration tests, use `ConfigControlPlaneMigrationRunner` with a dedicated
database. Do not point it at a production usage database until a reviewed
deployment migration and backup/restore procedure are available.
