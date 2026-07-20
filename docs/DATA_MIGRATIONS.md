# Data migration framework

`router-migrate` is the non-serving entrypoint for inspecting durable-data migration state. It opens the selected usage database without starting the router and emits only safe scalar metadata.

```sh
router-migrate --driver=sqlite --db=usage.sqlite --action=status
router-migrate --driver=postgres --dsn="$ROUTER_USAGE_DSN" --action=verify --json
```

The framework bootstraps two relational tables: `schema_migration_ledger` and `schema_migration_locks`. A checked-in migration manifest supplies a monotonic ID, scope, release, checksum, schema/data version, transaction and maintenance class, rollback class, and apply/verify handlers. An applied ID whose checksum no longer matches the binary, or an unknown future ID, is incompatible and fails closed. The runner allows only transactional online migrations; maintenance and non-transactional definitions require an explicit operator workflow rather than a serving-process startup action.

The lock table is a durable, scope-level single-runner lease for SQLite and PostgreSQL. SQLite remains a single-node option: schedule migrations during a maintenance window, take a file backup, run `PRAGMA integrity_check`, and ensure no other router process owns the database. PostgreSQL operators should use a backup/restore-tested deployment job and retain the ledger as audit evidence.

This framework is the migration contract foundation. The existing usage schema is still initialized by the legacy store path in this release, so `router-migrate` currently reports version `0` and has no conversion definitions. Do not use `--action=apply` as a substitute for an upgrade procedure until a release ships reviewed definitions for that scope. Future schema and data changes must add immutable definitions, use expand/migrate/validate/contract phases, and state the package/config/database rollback class in release notes.

The ledger intentionally excludes SQL values, DSNs, raw configuration, request content, images, tool payloads, tokens, hashes, provider credentials, and payment data.
