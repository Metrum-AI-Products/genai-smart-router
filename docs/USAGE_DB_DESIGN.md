# Usage DB And Reporting Design

This document is for maintainers. The external solution brief should describe capabilities without exposing implementation details.

## Storage

- Usage persistence is implemented with GORM.
- Supported drivers are `sqlite` and `postgres`.
- Local/default config uses SQLite at `server.usage_db.path`.
- Docker Compose production uses Postgres via `server.usage_db.driver: postgres` and `server.usage_db.dsn`.
- The packaged compose service uses the official `postgres:18-bookworm` image and listens only on the internal Docker network by default. Host access requires the explicit localhost-only compose override.

## Relational Schema Rule

The entire usage DB schema must remain relational-only:

- Do not use JSON, JSONB, array, or packed multi-value text columns.
- Store request usage as scalar columns on `request_usage`.
- If a future feature needs one-to-many data, add a child table with scalar columns and a foreign key to `request_usage`.
- Keep schema tests that inspect the actual DB column types.

Diagnostic child tables are part of the usage DB and follow the same rule:

- `request_attempts`: one scalar row per upstream attempt, including provider/model, status, timing, timeout/cancel flags, retryability, and sanitized error class/message.
- `request_trace_events`: ordered scalar router events such as request accepted, cache decision, upstream attempt, fallback, timeout, and terminal failure.
- `request_errors`: one scalar terminal error row per failed request for fast incident queries.

These tables are keyed by `request_id`. They must not store raw prompts, image payloads, bearer tokens, provider keys, token hashes, full upstream headers, or unsanitized provider response bodies.

Usage rollups are generated from stored `request_usage` rows and also follow the scalar relational rule:

- `usage_rollup_runs`: one row per generated daily rollup window, with draft/finalized status, UTC source window, source table name, source request count, daily row count, and generation timestamps.
- `usage_rollup_daily`: scalar aggregate rows per UTC day and reporting dimension in the rollup window, keyed to `usage_rollup_runs`. Dimensions include caller ID/user/project/environment, token ID, client, inbound dialect, requested model, resolved group, routing strategy, upstream provider/model/dialect, status class, stream flag, cache outcome, image-input flag, PII-filter flag, contract bucket, and target-validation status. Measures include request/error/cache/fallback/attempt counts, input/output/total tokens, input image count, input image tokens, request-time calculated and upstream-reported cost sums, latency/duration sums/counts/maxima, throughput sums/counts, and cache snapshot sums/maxima.

Draft rollups for the same exact window may be regenerated idempotently. Finalized rollup windows are immutable, and new rollup runs are rejected if their window overlaps an existing finalized daily window. Raw request purge and legal hold behavior are not part of this first rollup foundation.

Normalized decision telemetry is an optional first-slice diagnostic feature under `server.decision_telemetry`. It is disabled by default and writes only safe scalar child rows:

- `request_decision_shape_features`: one row per safe request-shape feature such as caller dialect, stream flag, tool count, image count, structured-output flag, max-token flag, and cacheability.
- `request_target_candidates`: one row per bounded group target candidate with provider, model, dialect, configured weight, tool-only flag, eligibility flag, and selected flag.
- `request_target_filter_reasons`: one row per bounded candidate filter bucket, such as `tool-only-target`, `tool-support`, `dialect-tool-passthrough`, `structured-output-support`, `max-tokens-honored`, or `contract-*`.
- `request_routing_decisions`: one row per selected routing decision with strategy, selected candidate index, provider, model, dialect, fallback count, and optional safe class label.
- `request_cache_reasons`: one row per cache decision bucket, such as `cache-hit`, `cache-miss`, `cache-request-no-cache`, `cache-tool-request`, `cache-image-request`, `cache-structured-output`, `cache-streaming`, or `cache-temperature`.

Decision telemetry must not store prompt text, image URLs or bytes, tool schemas, tool outputs, bearer tokens, provider keys, token hashes, full config, or routing script raw request mirrors. Keep new reason names stable, lowercase, and safe for reports.

Governed content-capture tables are separate from diagnostics and also follow the relational-only rule:

- `request_content_captures`: redacted request, response, and upstream-error content rows with scalar request/route metadata, retention timestamp, redaction counts, and truncation flags.
- `request_content_headers`: allowlisted captured header values keyed to a capture row; authorization, API-key, token, secret, cookie, and key-like headers must be rejected before storage.
- `request_content_audit_events`: read/delete/purge audit events with actor caller metadata, action, request ID, affected row count, and reason.

Content-capture rows are keyed by `request_id` so administrators can join them to `request_usage`. This is an explicit opt-in enterprise feature; default usage and diagnostics behavior remains metadata-only.

Casbin policy lifecycle tables also live in the usage DB and follow the same relational-only rule:

- `authz_policy_sets`: one row per versioned policy set with status, creator, and created/activated/retired timestamps.
- `authz_policy_rules`: one scalar row per Casbin `p` rule with sequence, subject or role, domain, object, action, and effect.
- `authz_role_links`: one scalar row per Casbin `g` role link with sequence, subject, role, and domain.
- `authz_policy_audit_events`: create, activation, rollback, and validation-failure audit events with safe actor, request, action, and summary fields.

These tables must not store raw router tokens, token hashes, provider keys, password material, prompts, images, tool outputs, or full config. Policy activation validates rows before status changes, and startup in DB policy mode fails closed when no single valid active policy set exists.

## Request Metrics

Each request row stores:

- caller, token, client, requested model group, resolved target provider/model, status, cache state, attempts, fallback state, and token counts.
- `caller_ip`: source IP derived from `X-Forwarded-For`, `X-Real-IP`, or the direct remote address.
- `upstream_duration_ms`: router-observed upstream provider/fallback call duration.
- `downstream_duration_ms`: router-to-caller response write duration.
- upstream/downstream output-token/sec and total-token/sec.
- cache snapshot: enabled state, item count, occupied bytes, max bytes, and occupancy percentage.
- request-time pricing: input/output dollars per million tokens, pricing source/update date, and calculated input/output/total USD cost.
- PII-filter metadata: `pii_filter_applied`, `pii_filter_mode`, `pii_filter_replacements`, and `pii_filter_rule_count`; never raw matched values or placeholder mappings.
- diagnostic traceability: child rows keyed by request ID for upstream attempts, trace events, and terminal errors.
- optional decision telemetry traceability: child rows keyed by request ID for request-shape features, candidate eligibility, filter buckets, routing decisions, and cache reason buckets when `server.decision_telemetry.enabled: true`.
- optional governed content-capture traceability: separate content rows keyed by request ID only when `server.content_capture.enabled` and a capture scope are configured.

For cache hits, upstream duration and upstream TPS are absent because no provider call occurs. Downstream duration and downstream TPS are still measured.

Cost columns are stored as scalar values on each row. Reports must sum stored cost values; they must not recalculate historical cost from current provider catalog pricing.

## Durability

Durable across container restarts when volumes are preserved:

- usage DB rows.
- JSONL request logs.
- per-request timing, TPS, and cache snapshot fields.
- diagnostic attempt, trace, and terminal error rows when diagnostics are enabled.
- decision telemetry rows when `server.decision_telemetry.enabled: true`.
- content-capture rows and content-capture audit rows when governed content capture is enabled.
- authz policy sets, policy rows, role links, and policy audit rows when DB-backed authorization is enabled.
- usage rollup run and daily aggregate rows after an operator generates them.

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
