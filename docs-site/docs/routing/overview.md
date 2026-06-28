---
title: Routing
---

# Routing

GenAI Smart Router lets callers request a stable, deployment-defined model group while operators change the provider and model mix behind that group. The caller sends `model: "<group-name>"`; the router checks caller access, filters the group's targets for the request shape, applies the configured routing strategy, and forwards the request to one eligible upstream target.

Group names are deployment-defined. Names such as `fast`, `high`, `big-coder`, or `vision` may appear in examples from a reference or hosted deployment, but they are not product-required names. Callers should discover allowed groups from [`/v1/models`](../getting-started/available-models).

## Routing Pipeline

Every model request follows the same routing pipeline:

1. Authenticate the caller token and verify that the caller is allowed to request the model group.
2. Load only the targets configured under that requested group.
3. Apply model-group contracts and hard eligibility filters.
4. Filter targets for API skin, tools, structured outputs, input modalities, reasoning controls, and max-token cap behavior.
5. Run the group's routing strategy over the remaining eligible targets.
6. Retry through configured fallback targets when the selected upstream fails in a retryable way.
7. Record safe request, usage, cost, latency, routing-decision, attempt, and fallback telemetry.

The router does not select targets from another group just because they are cheaper, lower latency, or more capable. The model group is the caller-facing contract.

## Choose A Strategy

| Strategy | Use When | Notes |
| --- | --- | --- |
| Static | One exact upstream target should serve the group. | Best for smoke groups, canaries, and tightly controlled workloads. |
| Weighted | Validated targets should share traffic by configured percentages. | Common choice for conservative production mixes and gradual promotion. |
| Failover | Targets should be tried in a deterministic priority order. | Useful when one target is primary and others are fallback only. |
| Dynamic score | The router should adapt within the group using cost, latency, throughput, reliability, request shape, and validation signals. | See [Dynamic Score Routing](../configuration/dynamic-score-routing). |
| TypeScript policy | Routing policy should be deployment-local and programmable inside the router process. | See [TypeScript Routing Policy](../configuration/routing-typescript). |
| External policy | Routing policy should live in a trusted standalone service with its own deployment and observability. | See [External Routing Policy Service](../configuration/external-routing-policy). |
| Model-group contract | A group needs explicit workload requirements, quality floors, and validation gates before strategy selection. | See [Model Group Contracts](../configuration/model-group-contracts). |

## Capability Filtering

Request-shape filtering happens before the routing strategy runs. A weighted or dynamic group can contain targets with different capabilities, but each request only sees targets that satisfy its requirements.

| Request Shape | Target Requirement |
| --- | --- |
| OpenAI Chat tools | `tool_support.openai_chat` includes the required tool mode. |
| OpenAI Responses function tools | `tool_support.openai_responses` includes `function`. |
| Anthropic Messages client tools | `tool_support.anthropic_messages` includes `client_tools`. Empty tool metadata is not treated as tool-capable. |
| OpenAI Chat structured outputs | `tool_support.openai_chat` includes `structured_outputs`. |
| OpenAI Responses structured outputs | `tool_support.openai_responses` includes `structured_outputs`. |
| Image input | `input_modalities` includes `image`. |
| Explicit reasoning or thinking | Target `reasoning` metadata is compatible with the caller field. |
| Positive max-token cap | Target is not marked as unsafe for caller caps. |

If no target in the requested group satisfies the full request shape, the router returns `502 no-eligible-target` before sending an upstream request.

## Stable Group, Changing Upstreams

This example keeps the caller-visible group stable while the operator changes active upstreams and weights:

```yaml
models:
  production-general:
    strategy: weighted
    targets:
      - provider: hosted_openai_compatible
        model_ref: balanced-text
        weight: 70
      - provider: private_vllm
        model_ref: internal-coding
        weight: 20
      - provider: hosted_openai_compatible
        model_ref: low-cost-fallback
        weight: 10
```

Callers continue to request `production-general`:

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "production-general",
    "messages": [{"role": "user", "content": "Summarize this incident note."}],
    "max_tokens": 300
  }'
```

The operator can later add, remove, or reweight targets without changing the client request, as long as the group continues to satisfy its workload contract.

## Fallback Behavior

Fallback stays inside the requested group. The router can retry after retryable upstream failures such as transient network errors, selected upstream timeouts, provider overload responses, provider rate limits, provider quota or billing exhaustion, and 5xx responses. Ordinary non-retryable upstream 4xx responses, including malformed-request, policy, and authorization errors, stop fallback so the same caller payload is not replayed to another provider. Non-retryable caller errors, authentication errors, forbidden model groups, caller quota failures, and license failures stop before upstream routing.

For deterministic fallback, configure a failover-style group or a strategy-specific fallback order. For weighted or dynamic groups, keep every fallback target validated for the same API skins and workload requirements that callers depend on.

## Validation Checklist

Before exposing a group broadly:

- Call `/v1/models` with the intended caller token and confirm the group is visible only to the right callers.
- Run text requests through each supported API skin: Chat Completions, Responses, or Messages.
- Run tool, forced-tool, structured-output, image, reasoning, streaming, and low max-token cap smokes when those request shapes are in scope.
- Confirm selected targets stay inside the requested group and record usage, cost, latency, attempts, and fallback telemetry.
- Validate quality with a workload-appropriate harness such as unit tests, extraction accuracy checks, OCR targets, browser-control tasks, tool-call correctness checks, golden datasets, product acceptance tests, or an agent benchmark.
- Roll back by removing the target from `models.<group>.targets[]`, setting its weight to `0`, or changing the group to a simpler strategy.

For target metadata and onboarding requirements, see [Providers And Models](../providers-models/overview) and [Model Metadata](../reference/model-metadata).
