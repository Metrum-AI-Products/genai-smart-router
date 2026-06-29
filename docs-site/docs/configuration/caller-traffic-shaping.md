---
title: Caller Traffic Shaping
doc_type: reference
---

# Caller Traffic Shaping

Caller traffic shaping smooths short bursts before upstream calls. It complements hard controls such as `rpm`, `tpm`, `concurrent`, quotas, budgets, and license checks; it does not replace them.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth.

```yaml title="config.example.yaml"
server:
  traffic_shape:
    enabled: false
    default_caller:
      request_start_per_sec: 0
      request_burst: 0
      input_tokens_per_sec: 0
      input_token_burst: 0
      output_reservation_tokens_per_sec: 0
      output_reservation_token_burst: 0
      total_reserved_tokens_per_sec: 0
      total_reserved_token_burst: 0
      queue:
        enabled: false
        max_wait_ms: 0
        max_depth: 0
```

## Schema

Server defaults are disabled unless `server.traffic_shape.enabled: true`. Caller-level `callers[].traffic_shape` overrides the server default, and `enabled: false` opts a caller out of an enabled default.

Request-start shaping applies after authentication, model-group allow-list checks, and token estimation. It applies to all allowed requests, including cache hits, before the active concurrency slot is acquired. Input, output-reservation, and total-reserved token shaping apply only to cache misses that would otherwise call upstream. Bucket state is in memory and resets on router restart.

Shaping rejections return `429 traffic-shaped` with `Retry-After` when a retry time is known and a safe bucket label.

## Rollback

Disable a caller or server default `traffic_shape.enabled`, or set only `queue.enabled: false` to preserve fail-fast bucket checks. Compare traffic-shape usage fields, upstream 429 attempts, and user latency before broadening a shaping policy.

## Related

See [Provider Traffic Shaping](./provider-traffic-shaping), [Usage Reporting](../operations/usage-reporting), and [Router Configuration](./router-config).
