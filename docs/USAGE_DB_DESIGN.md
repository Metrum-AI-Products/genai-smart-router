# Usage DB And Reporting Design

This document is for maintainers. The external solution brief should describe capabilities without exposing implementation details.

## Storage

- Usage persistence is implemented with GORM.
- Supported drivers are `sqlite` and `postgres`.
- Local/default config uses SQLite at `server.usage_db.path`.
- Docker Compose production uses Postgres via `server.usage_db.driver: postgres` and `server.usage_db.dsn`.
- SQLite usage database files are created/chmodded `0600`; keep the containing state directory private and do not loosen permissions on WAL or SHM sidecars.
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
- `request_traffic_shape_events`: one scalar row per evaluated caller/server traffic-shaping bucket, with scope, bucket, decision, cost, retry-after, queue wait, estimated input tokens, reserved output tokens, and total reserved tokens.
- `request_upstream_shape_events`: one scalar row per provider/model/target shaping decision, including scope, provider, model reference or label, dialect, bucket, decision, retry-after milliseconds, estimated input tokens, reserved output tokens, total reserved tokens, and safe backoff reason.
- `request_errors`: one scalar terminal error row per failed request for fast incident queries.

These tables are keyed by `request_id`. They must not store raw prompts, image payloads, bearer tokens, provider keys, token hashes, full upstream headers, raw Retry-After headers, or unsanitized provider response bodies.

Usage rollups are generated from stored `request_usage` rows and also follow the scalar relational rule:

- `usage_rollup_runs`: one row per generated hourly, daily, or monthly rollup window, with draft/finalized status, UTC source window, source table name, source request count, source min/max request timestamp, deterministic source checksum, aggregate row counts, decision-bucket row count, router version/commit, safe error text, and generation/finalization timestamps.
- `usage_rollup_hourly`: scalar aggregate rows per UTC hour and reporting dimension for recent operational trend reporting.
- `usage_rollup_daily`: scalar aggregate rows per UTC day and reporting dimension in the rollup window, keyed to `usage_rollup_runs`.
- `usage_rollup_monthly_billing`: scalar aggregate rows per UTC calendar month and reporting dimension for **customer internal chargeback and optional enterprise contract true-up summaries**. This is **not** a Metrum product billing ledger; Metrum commercial billing is handled outside the router by Metrum finance.
- `usage_rollup_decision_buckets`: scalar aggregate rows per rollup bucket, model group, strategy, bucket kind, bucket name, and optional secondary bucket. It preserves report-critical decision buckets after raw request/decision detail retention, including max-token buckets, input-token buckets, quota/admission reason buckets, enabled dynamic-score signal names, dynamic score buckets, and threshold/filter buckets.
- `usage_rollup_audit_events`: scalar audit rows for rollup create, draft regeneration, and finalization events with a safe summary.

Rollup dimensions include caller ID/user/project/environment, token ID, client, inbound dialect, requested model, resolved group, routing strategy, upstream provider/model/dialect, status class, stream flag, cache outcome, image-input flag, PII-filter flag, contract bucket, target-validation status, and optional savings baseline ID. Measures include request/success/error/cache/fallback/attempt counts, input/output/total tokens, input image count, input image tokens, request-time calculated and upstream-reported cost sums, optional baseline input/output/total costs and savings, latency/duration sums/counts/maxima, throughput sums/counts/minima/maxima, and cache snapshot sums/maxima.

Draft rollups for the same exact type/window may be regenerated idempotently. Finalized rollup windows are immutable through the rollup generator, and new rollup runs are rejected if their type/window overlaps an existing finalized window. Retention uses finalized daily rollup metadata as a prerequisite before `usage_detail` delete batches can run and does not mutate finalized rollup rows.

Commercial retention tables also follow the scalar relational rule:

- `retention_policy_versions`: active config-derived retention policy versions with a scalar policy hash, dry-run flag, default batch size, and activation timestamps.
- `retention_policy_rules`: one row per data class rule, with data class, enabled flag, retention days, batch size, and the `usage_detail` finalized-rollup prerequisite.
- `retention_jobs`: one row per status or run job with policy version, mode, status, requested-by, dry-run flag, and timestamps.
- `retention_job_table_results`: per-job/per-table counts for candidate rows, legal-hold skipped rows, eligible rows, blocked rows, deleted rows, cutoff timestamp, and status.
- `legal_holds`: active or released holds keyed by hold ID, data class, optional request ID, timestamp range, reason, subject, creator/releaser, and timestamps.
- `legal_hold_audit_events`: scalar audit rows for hold create/release/update workflows.

The current retention foundation initializes policy rows from `server.retention`, records dry-run counts for all known classes, and can delete one configured batch for `usage_diagnostics` (`request_attempts`, `request_trace_events`, `request_traffic_shape_events`, `request_upstream_shape_events`, `request_errors`) and rollup-gated `usage_detail` (`request_usage`) when `dry_run: false`. Legal holds are checked by `data_class`, optional `request_id`, and timestamp range when counting skipped rows and selecting delete batches; request-scoped legal holds on child data classes also block `usage_detail` parent deletion so cascade rules cannot remove held child telemetry. Decision telemetry child rows are counted through their parent `request_usage.ts`. It does not archive content, schedule jobs, delete unsupported classes through the generic runner, or provide a full admin UI/API for hold lifecycle.

Normalized decision telemetry is an optional first-slice diagnostic feature under `server.decision_telemetry`. It is disabled by default and writes only safe scalar child rows:

- `request_decision_shape_features`: one row per safe request-shape feature such as caller dialect, stream flag, tool count, image count, structured-output flag, max-token flag, max-token bucket, input-token bucket, and cacheability.
- `request_target_candidates`: one row per bounded group target candidate with provider, model, dialect, configured weight, tool-only flag, scalar capability flags, validation status/age bucket, eligibility flag, and selected flag.
- `request_target_filter_reasons`: one row per bounded candidate filter bucket, such as `tool-only-target`, `tool-support`, `dialect-tool-passthrough`, `structured-output-support`, `max-tokens-honored`, or `contract-*`.
- `request_routing_decisions`: one row per selected routing decision with strategy, selected candidate index, provider, model, dialect, fallback count, and optional safe class label.
- `request_routing_signals`: one row per safe routing signal used by a strategy, such as enabled `dynamic_score` signals and scalar policy knobs.
- `request_dynamic_score_terms`: one row per bounded candidate/rank/term/score contribution or simple-strategy ranking explanation. `dynamic_score` rows include scalar value/contribution/final-score buckets; simple `static`, `failover`, `weighted`, `latency`, `cost`, `script`, and `external` rows use stable configured-order, configured-weight, configured-rank, or policy-output terms without random seeds or raw policy payloads.
- `request_policy_executions`: one row per script/external policy execution, including fail-closed errors before target selection and configured external fallback executions. Rows store strategy, policy kind, outcome, safe error class/message, duration, eligible/all target counts, selected candidate index when any, fallback count, and terminal error type.
- `request_fallback_transitions`: one row per upstream failure that moves to a fallback target, with failed/fallback candidate indexes, provider/model/dialect labels, fallback reason/error class, retryable flag, and whether that fallback attempt succeeded.
- `request_cache_reasons`: one row per cache decision bucket, such as `cache-hit`, `cache-miss`, `cache-request-no-cache`, `cache-tool-request`, `cache-image-request`, `cache-structured-output`, `cache-streaming`, or `cache-temperature`.

Request usage rows also store non-secret reproducibility fields: router version/build date, routing config fingerprint, model-group config fingerprint, routing-policy fingerprint, and pricing-catalog fingerprint. Fingerprints are SHA-256 hashes of redacted canonical routing metadata. They exclude provider API keys, router token hashes, raw prompts, raw policy request/response bodies, policy headers, raw URLs, script paths, and full config contents; script policy changes are represented by a script-content hash only when the file is available.

Decision telemetry must not store prompt text, image URLs or bytes, tool schemas, tool outputs, bearer tokens, provider keys, token hashes, full config, policy request/response JSON, or routing script raw request mirrors. Keep new reason names stable, lowercase, and safe for reports.

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
- traffic-shaping metadata: `traffic_shape_applied`, `traffic_shape_decision`, `traffic_shape_scope`, `traffic_shape_bucket`, retry-after, queue-wait, estimated input tokens, reserved output tokens, total reserved tokens, and child rows in `request_traffic_shape_events` for each evaluated bucket.
- optional decision telemetry traceability: child rows keyed by request ID for request-shape features, candidate eligibility/capability metadata, filter buckets, routing decisions, routing signals, score/ranking term rows, policy execution rows, fallback transition rows, and cache reason buckets when `server.decision_telemetry.enabled: true`.
- derived report buckets: max-token bucket, input-token bucket, admission reason, enabled dynamic-score signal names, score buckets, and threshold buckets. Multi-value decision buckets are kept as child/rollup rows, never arrays or packed JSON.
- optional governed content-capture traceability: separate content rows keyed by request ID only when `server.content_capture.enabled` and a capture scope are configured.

For cache hits, upstream duration and upstream TPS are absent because no provider call occurs. Downstream duration and downstream TPS are still measured.

Cost columns are stored as scalar values on each row. Reports must sum stored cost values; they must not recalculate historical cost from current provider catalog pricing.

## Durability

Durable across container restarts when volumes are preserved:

- usage DB rows.
- JSONL request logs.
- per-request timing, TPS, and cache snapshot fields.
- diagnostic attempt, trace, and terminal error rows when diagnostics are enabled.
- traffic-shaping request rows and per-bucket event rows for admitted, queued, and rejected shaped requests.
- decision telemetry rows when `server.decision_telemetry.enabled: true`.
- content-capture rows and content-capture audit rows when governed content capture is enabled.
- authz policy sets, policy rows, role links, and policy audit rows when DB-backed authorization is enabled.
- usage rollup run, daily aggregate rows, and daily decision-bucket rollup rows after an operator generates them.
- retention policy, job result, legal hold, and legal hold audit rows after an operator runs dry-run retention status.

Not durable across router restarts:

- in-memory response cache contents.
- in-memory traffic-shaping token buckets and queue depths.
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
