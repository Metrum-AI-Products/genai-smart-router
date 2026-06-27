# Dynamic Score Routing

`strategy: dynamic_score` is the built-in configurable routing strategy for model groups that should adapt to request shape, cost, observed performance, reliability, workload complexity, and evaluation metadata without adding custom router code.

The caller-visible model group does not change. A client requests one deployment-defined model group from `/v1/models`, and the caller token must be allowed to use that group. The router then scores only that requested group's eligible `targets[]`. A cheaper or faster target in another group is never considered. If the group has an optional [model-group contract](./model-group-contracts), its hard requirements and quality floors run before dynamic scoring.

## Configuration

```yaml
models:
  adaptive-agent:
    strategy: dynamic_score
    targets:
      - provider: baseten
        model_ref: gpt-oss-120b
        weight: 60
        tags: [validated, coding, tool_capable]
      - provider: minimax
        model_ref: m3
        weight: 25
        tags: [validated, low_cost, tool_capable]
      - provider: openai
        model_ref: gpt-5.4-nano
        weight: 5
        tags: [fallback]
    routing_policy:
      dynamic_score:
        cold_start_policy: configured_weight
        min_observations: 20
        observation_window_seconds: 600
        max_score_adjustment_percent: 70
        hard_filters:
          require_requested_api_skin: true
          require_input_modalities: true
          require_tool_support_when_tools_present: true
          require_reasoning_support_when_requested: true
          require_honors_max_tokens_when_caller_capped: true
        signals:
          request_shape: { enabled: true }
          prompt_features:
            enabled: true
            max_scan_bytes: 16384
            features: [code, diff, stack_trace, summarize, extract, security_review, tool_agent]
          complexity: { enabled: true }
          observed_performance: { enabled: true }
          cost: { enabled: true }
          evaluation_metadata: { enabled: true }
        score_terms:
          - name: cheapest_fast_enough
            when: { complexity_lte: standard }
            expression: "0.45 * cost_score + 0.25 * latency_score + 0.20 * throughput_score + 0.10 * reliability_score"
          - name: complex_quality_floor
            when: { complexity_gte: complex }
            require_tags: [validated]
            expression: "0.45 * eval_quality_score + 0.25 * reliability_score + 0.20 * latency_score + 0.10 * cost_score"
        thresholds:
          max_error_rate: 0.03
          max_timeout_rate: 0.02
          max_p95_latency_ms: 10000
```

Group names and targets are examples. Use names, upstreams, and quality contracts that match the deployment.

## How Selection Works

The router first applies the same request eligibility rules used by other strategies:

- API skin and upstream dialect must preserve the caller request.
- Tool-bearing requests require compatible tool passthrough.
- Image-bearing requests require matching input modality metadata.
- Reasoning or thinking requests require compatible `reasoning` target metadata.
- Explicit positive caller token caps, including OpenAI Chat `max_completion_tokens: 1`, skip targets marked `honors_max_tokens: false`.

`hard_filters.require_reasoning_support_when_requested` is an additional dynamic-score hard filter for deployments that want that rule declared in policy. It is not the only reasoning-routing mechanism: compatible target `reasoning` metadata is still the baseline eligibility source for ordinary weighted, failover, script, external, and dynamic-score groups. See [Reasoning Routing](./reasoning-routing) for the weighted-group pattern.

After eligibility, `dynamic_score` applies configured thresholds and score terms. Supported score names include:

| Score | Meaning |
| --- | --- |
| `cost_score` | Lower configured input/output price ranks higher. |
| `latency_score` | Lower observed p95 latency ranks higher. |
| `throughput_score` | Higher observed output tokens per second ranks higher. |
| `reliability_score` | Lower error, timeout, and fallback rates rank higher. |
| `complexity_score` | Bounded request-complexity bucket from prompt size, tools, images, output budget, and enabled prompt features. |
| `eval_quality_score` | Optional configured evaluation quality or pass-rate metadata for the target, including target `validation` metadata when present. |

Cold start is deterministic. Until `min_observations` is reached, targets are ordered by configured group-local weight. After that, score terms are blended with configured weights according to `max_score_adjustment_percent`, so operators can cap how far live signals move traffic away from the declared mix.

## Operations

The strategy uses in-memory rolling observations for latency, upstream duration, TTFB, output throughput, status, timeout class, error class, and fallback use. It does not read the usage database while routing. Historical usage tables remain useful for offline validation and reports.

Decision traces and telemetry rows are safe scalar diagnostics. They include fields such as strategy, cold-start mode, enabled signal names, request-shape buckets, selected provider/model, score bucket, observation count, candidate count, normalized reasoning fields when a caller explicitly requested reasoning or thinking, and fallback-transition rows after upstream failures. Failed attempts update the same in-memory observation store used by later dynamic-score decisions, so provider 429s, 5xxs, timeouts, decode errors, and client cancellations affect future reliability/timeout/fallback signals according to the configured scoring policy. These rows do not include raw prompts, raw images, raw tool outputs, router tokens, token hashes, provider keys, full upstream headers, or full config contents.

When decision telemetry is enabled, usage and admin reports expose safe dynamic-score buckets for operations: enabled signal names, score/value/final-score buckets, threshold/filter buckets, max-token cap filtering, max-token buckets, large input-token buckets, and quota/admission reason buckets. Daily rollups preserve those buckets in normalized rows so operators can keep commercial reporting after raw request-level detail expires.

Roll out on a deployment-defined test group with interchangeable validated targets before enabling broad production traffic. Test simple text, code/debug prompts, tool requests, forced tool requests, image requests where supported, structured-output requests where supported, reasoning or thinking requests where supported, and low explicit max-token caps. Roll back by changing the group strategy to `weighted`, removing unsafe reasoning metadata, or disabling strict thresholds and score terms.
