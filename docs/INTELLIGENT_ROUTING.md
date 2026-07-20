# Intelligent Routing Foundation

Source-only implementation note for issue #505. This first increment adds the
licensed, validated configuration contract for `strategy: intelligent`; it is
not an enablement guide for an active decision-model selector.

An intelligent group names a catalogued decision model independently of its
serving targets. The only accepted context modes are `scalar_only` and
`redacted_text`. A decision model must be identified by `provider` and
`model_ref`, never by another model group, so configuration cannot create a
routing recursion. Time, output, retry, concurrency, cost, confidence,
fallback, and schema limits are required and bounded at config load time.

```yaml
models:
  deployment-defined-group:
    strategy: intelligent
    intelligent_routing:
      mode: shadow
      decision_model:
        provider: decision-provider
        model_ref: decision-model-reference
      timeout_ms: 250
      max_output_tokens: 128
      max_retries: 0
      max_concurrent: 4
      max_decision_cost_usd: 0.01
      confidence_threshold: 0.70
      context_mode: scalar_only
      on_error: fallback
      schema_version: v1
    targets:
      # Existing eligible serving targets remain authoritative.
      - provider: serving-provider
        model_ref: serving-model-reference
```

At this stage the router always serves the normal first eligible target and
records `intelligent:baseline-only` policy telemetry when decision telemetry is
enabled. It does **not** call the configured decision model, send request
content to another upstream, claim a recommendation, charge decision overhead,
or permit `enforce` behavior. This preserves normal target eligibility,
contracts, PII filtering, quotas, and deterministic fallbacks while later work
adds the bounded structured selector, relational overhead records, reviewed
shadow comparisons, and promotion gates.

Do not add an intelligent group to production until those later capabilities,
direct decision-model smokes, router-level shape tests, and rollout/rollback
documentation are complete. The feature requires the `intelligent_routing`
license capability in addition to ordinary routing.
