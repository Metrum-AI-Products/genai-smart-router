# Usage DB And Reporting Design

This document is for maintainers. The external solution brief should describe capabilities without exposing implementation details.

## Storage

- Usage persistence is implemented with GORM.
- Supported drivers are `sqlite` and `postgres`.
- Local/default config uses SQLite at `server.usage_db.path`.
- Docker Compose production uses Postgres via `server.usage_db.driver: postgres` and `server.usage_db.dsn`.
- The packaged compose service uses the official `postgres:18-bookworm` image and publishes host port `15432` by default.

## Relational Schema Rule

The entire usage DB schema must remain relational-only:

- Do not use JSON, JSONB, array, or packed multi-value text columns.
- Store request usage as scalar columns on `request_usage`.
- If a future feature needs one-to-many data, add a child table with scalar columns and a foreign key to `request_usage`.
- Keep schema tests that inspect the actual DB column types.

## Request Metrics

Each request row stores:

- caller, token, client, requested model group, resolved target provider/model, status, cache state, attempts, fallback state, and token counts.
- `caller_ip`: source IP derived from `X-Forwarded-For`, `X-Real-IP`, or the direct remote address.
- `upstream_duration_ms`: router-observed upstream provider/fallback call duration.
- `downstream_duration_ms`: router-to-caller response write duration.
- upstream/downstream output-token/sec and total-token/sec.
- cache snapshot: enabled state, item count, occupied bytes, max bytes, and occupancy percentage.
- request-time pricing: input/output dollars per million tokens, pricing source/update date, and calculated input/output/total USD cost.

For cache hits, upstream duration and upstream TPS are absent because no provider call occurs. Downstream duration and downstream TPS are still measured.

Cost columns are stored as scalar values on each row. Reports must sum stored cost values; they must not recalculate historical cost from current provider catalog pricing.

## Durability

Durable across container restarts when volumes are preserved:

- usage DB rows.
- JSONL request logs.
- per-request timing, TPS, and cache snapshot fields.

Not durable across router restarts:

- in-memory response cache contents.
- in-process Prometheus counters and gauges.

## Production Reset

For this schema change, production should start from a clean usage database.

For Postgres compose deployments:

1. Stop the stack.
2. Back up the Postgres data volume or dump the database.
3. Remove the Postgres data volume.
4. Start the stack so GORM creates the clean relational schema.
5. Verify `/readyz`, generate a fresh usage report, and keep the backup until the deployment is accepted.

Do not delete JSONL request logs unless explicitly requested.
