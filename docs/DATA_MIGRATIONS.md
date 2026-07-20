# Data migration framework

`router-migrate` is the non-serving entrypoint for inspecting durable-data migration state. It opens the selected usage database without starting the router and emits only safe scalar metadata.

```sh
router-migrate --driver=sqlite --db=usage.sqlite --action=status
router-migrate --driver=postgres --dsn="$ROUTER_USAGE_DSN" --action=verify --json
```

The framework bootstraps two relational tables: `schema_migration_ledger` and `schema_migration_locks`. A checked-in migration manifest supplies a monotonic ID, scope, release, checksum, schema/data version, transaction and maintenance class, rollback class, and apply/verify handlers. An applied ID whose checksum no longer matches the binary, or an unknown future ID, is incompatible and fails closed. The runner allows only transactional online migrations; maintenance and non-transactional definitions require an explicit operator workflow rather than a serving-process startup action.

The lock table is a durable, scope-level single-runner lease for SQLite and PostgreSQL. SQLite remains a single-node option: schedule migrations during a maintenance window, take a file backup, run `PRAGMA integrity_check`, and ensure no other router process owns the database. PostgreSQL operators should use a backup/restore-tested deployment job and retain the ledger as audit evidence.

The initial usage manifest has one immutable, transaction-safe baseline adoption (`2026071901`, schema version `1`, data version `0`). It verifies that every current relational usage table already exists and has scalar columns, then writes its checksum to the ledger. It executes no schema-changing DDL and cannot initialize an empty database: the legacy initializer remains temporarily responsible for fresh installs. Run `router-migrate --action=plan` first, back up the database, then run `--action=apply` only against an already initialized usage database during a controlled maintenance window. `--action=verify` rechecks applied postconditions without changing application tables. Future schema and data changes must add immutable definitions, use expand/migrate/validate/contract phases, and state the package/config/database rollback class in release notes.

`server.usage_db.migration_policy` controls serving-process behavior. `legacy-auto-migrate` is the backward-compatible default and is temporary while the fresh-install manifest is incomplete. `validate` and `deployment-job` never apply application-schema DDL at startup; both require a current, compatible ledger and fail startup otherwise. `auto-safe` applies only checked-in transactional online definitions, then verifies the current ledger; it cannot initialize an empty database with the present adoption-only baseline. For PostgreSQL production, use a non-serving deployment job and `deployment-job` (or `validate` after that job); do not rely on a serving replica to perform a migration.

The ledger intentionally excludes SQL values, DSNs, raw configuration, request content, images, tool payloads, tokens, hashes, provider credentials, and payment data.
