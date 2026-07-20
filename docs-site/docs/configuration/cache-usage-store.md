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

Use the non-serving `router-migrate` command to inspect a durable migration ledger before an upgrade. It reports only scope/version/compatibility and safe migration state; it never returns database connection values, SQL values, request content, or credentials. The initial usage migration adopts an already initialized legacy usage database into the ledger after its schema is verified; it does not initialize an empty database or modify application tables. Follow the release-specific upgrade instructions before applying later migration definitions.

`decision_telemetry` is optional and disabled by default. When enabled, it writes normalized scalar child rows for request shape, bounded candidates, filter reasons, routing decisions, score terms, script/external policy executions, fallback transitions, cache reasons, and non-secret fingerprints.

`diagnostics` adds safe relational troubleshooting rows. `content_capture` is a separate governed feature and remains disabled until an operator enables at least one scope. Retention policy is configured under `server.retention` and defaults to dry-run.

## Rollback

Disable optional telemetry or capture by setting the relevant `enabled` flag to `false`, then restart or reload. For governed capture, also verify delete-by-request and purge authorization before rollout and after rollback.

## Related

See [Usage Reporting](../operations/usage-reporting), [Admin Browser Reports](../operations/admin-browser-reports), [PII Filtering](./pii-filtering), [Admin Authorization](./admin-authorization), and [Router Configuration](./router-config).
