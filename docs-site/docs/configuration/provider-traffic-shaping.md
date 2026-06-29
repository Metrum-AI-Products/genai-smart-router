---
title: Provider Traffic Shaping
doc_type: reference
---

# Provider Traffic Shaping

Provider traffic shaping protects shared upstream capacity across all caller keys. It complements caller rate and quota policy: caller limits run first, while provider shaping decides whether a selected upstream target can be used right now.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth.

```yaml title="config.example.yaml"
providers:
  baseten:
    traffic_shape:
      enabled: false
      request_start_per_sec: 10
      request_burst: 30
      input_tokens_per_sec: 500000
      input_token_burst: 1500000
      total_reserved_tokens_per_sec: 750000
      total_reserved_token_burst: 2000000
      upstream_429_backoff:
        enabled: true
        min_backoff_ms: 1000
        max_backoff_ms: 60000
        multiplier: 2.0
        honor_retry_after: true
      upstream_quota_backoff:
        enabled: true
        min_backoff_ms: 30000
        max_backoff_ms: 300000
        multiplier: 2.0
        honor_retry_after: true
```

## Schema

`traffic_shape` can be declared on provider, provider-model, or target entries. Active scopes are cumulative: a request must pass every configured scope before the router calls upstream. Cache hits do not consume shared provider capacity.

`request_start_per_sec` limits upstream request starts. `input_tokens_per_sec` uses the router's input-token estimate. `total_reserved_tokens_per_sec` uses estimated input plus caller output-cap reservation. Adaptive backoff starts when an upstream attempt is classified as rate-limited or quota-exhausted, with `Retry-After` honored only when configured and bounded.

Shape decisions are stored as safe scalar telemetry without request bodies, upstream response bodies, provider keys, router tokens, or token hashes.

## Rollback

Disable the provider, model, or target `traffic_shape` block, restart or reload, then compare upstream 429/quota attempts and caller latency. If only queueing or backoff is too aggressive, lower that subsection while preserving the basic request-start buckets.

## Related

See [Caller Traffic Shaping](./caller-traffic-shaping), [Cache And Usage Store](./cache-usage-store), and [Router Configuration](./router-config).
