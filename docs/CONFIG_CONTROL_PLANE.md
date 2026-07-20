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

The database enforces the provider-header boundary on both inserts and
updates. Only `HTTP-Referer`, `User-Agent`, and `X-Title` metadata headers are
representable; standard or provider-specific credential headers are rejected.
Each allowed header name is unique case-insensitively for a provider, so a
configuration set cannot contain conflicting casing variants. Values in these
metadata fields must remain non-secret. Use `api_key_env` or a
deployment-managed secret reference for every upstream credential.

The read path independently rejects case-insensitive duplicate provider-header
names. That guard remains active when a database has only the phase-2 prefix or
an unavailable migration ledger, so header-map iteration can never select an
ambiguous upstream value. Phase 3 verifies the named unique expression index
semantically on both SQLite and PostgreSQL: it must be unique and contain
exactly `config_set_id`, `provider_name`, and `LOWER(header_name)` in that
order.

`LoadActiveConfigFromDB` is deliberately read-only. It loads only a validated
active set and reuses `Config.Validate`; missing active state and invalid
relational data fail closed. Runtime DB mode, YAML import/export, write APIs,
activation/rollback actions, PostgREST, Caddy, secret persistence, and hot
reload remain later issue #7 phases.

For migration tests, use `ConfigControlPlaneMigrationRunner` with a dedicated
database. Do not point it at a production usage database until a reviewed
deployment migration and backup/restore procedure are available. The initial
table creation is an online migration, but the provider-header hardening
phases are transactional **maintenance** migrations: PostgreSQL must lock the
existing header table while adding the allowlist constraint and building the
unique expression index, while SQLite rebuilds that table. `ApplyPending`
stops before those phases. A non-serving deployment job must first apply the
online prefix, take the approved backup, then explicitly invoke
`ApplyMaintenancePending` in a scheduled maintenance window and finish with
`Verify`; it must never be run by router startup.
