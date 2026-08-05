---
title: Cache And Usage Store
doc_type: reference
---

# Cache And Usage Store

The cache, usage database, decision telemetry, diagnostics, content capture, and retention settings determine what the router stores about requests. These features must remain safe for operations, reports, and audits without persisting secrets or raw governed content unless content capture is explicitly enabled.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth.

```yaml title="config.example.yaml"
server:
  default_model_group: default
  cache:
    enabled: true
    max_bytes: 134217728
    default_ttl: 15m
  usage_db:
    enabled: true
    driver: sqlite
    path: usage.sqlite
    migration_policy: deployment-job
  decision_telemetry:
    enabled: false
    max_candidates: 64
    max_filter_reasons: 256
    record_candidates: true
    record_cache_reasons: true
  diagnostics:
    enabled: true
    retention_days: 30
    store_sanitized_upstream_errors: true
    max_error_bytes: 2048
  content_capture:
    enabled: false
    retention_days: 30
    capture_request: false
    capture_response: false
    capture_tool_calls: false
    capture_images: false
    capture_upstream_errors: false
    capture_headers_allowlist: []
    redact_before_storage: true
    redaction_patterns: []
    max_capture_bytes: 65536
    encryption:
      enabled: false
      kms_key_id: ""
```

`store_sanitized_upstream_errors` defaults to `true` when diagnostics are enabled. It writes only bounded allowlisted provider error fields such as code, type, param, provider request ID, and categorized message values; set it to `false` only when you want to suppress those provider detail rows.

## Schema

`server.cache` covers eligible deterministic unary responses. Tool-bearing agent requests and image-bearing requests bypass response caching.

`usage_db` enables relational usage persistence. Usage rows store request-time cost inputs, calculated costs, upstream-reported billed costs where available, latency, throughput, cache status, attempts, fallbacks, caller/project dimensions, and provider/model/dialect dimensions.

Use the non-serving `router-migrate` command to inspect a durable migration ledger before an upgrade. It reports only scope/version/compatibility and safe migration state; it never returns database connection values, SQL values, request content, or credentials. The shared migration contract binds each checked-in migration’s reviewed metadata and handler identity, and stores only normalized scalar audit, job, and checkpoint records. Data conversions are checkpointed non-serving jobs: they can be cancelled only at a checkpoint, require an explicit recovery-evidence reference before retry, and advance a data version only after their checked-in validation succeeds. Router startup and request paths never start them. Follow the release-specific upgrade instructions before applying later migration definitions.

`migration_policy: deployment-job` is the serving default. Apply and verify migrations with the non-serving `router-migrate` deployment job, then configure `migration_policy: deployment-job` (or `validate`). Those policies do not apply application-schema changes at router startup and fail closed unless the ledger is current and compatible. `auto-safe` is limited to checked-in transactional online migrations for reviewed small/single-node deployments.

For PostgreSQL, supply the migration connection only through the deployment environment (for example, `--dsn-env=ROUTER_USAGE_DB_DSN`); do not place a DSN in a shell command, ticket, or operator output. Package rollback never runs a reverse migration. If the reviewed migration contract requires restoration, restore the approved pre-migration backup before deploying an older package.

`decision_telemetry` is optional and disabled by default. When enabled, it writes normalized scalar child rows for request shape, bounded candidates, filter reasons, routing decisions, score terms, script/external policy executions, fallback transitions, cache reasons, and non-secret fingerprints.

`diagnostics` adds safe relational troubleshooting rows. `content_capture` is a separate governed feature and remains disabled until an operator enables at least one scope. Retention policy is configured under `server.retention` and defaults to dry-run.

## Rollback

Disable optional telemetry or capture by setting the relevant `enabled` flag to `false`, then restart or reload. For governed capture, also verify delete-by-request and purge authorization before rollout and after rollback.

## Related

See [Usage Reporting](../operations/usage-reporting), [Admin Browser Reports](../operations/admin-browser-reports), [PII Filtering](./pii-filtering), [Admin Authorization](./admin-authorization), and [Router Configuration](./router-config).
