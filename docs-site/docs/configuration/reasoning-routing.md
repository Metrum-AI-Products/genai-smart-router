---
title: Reasoning Routing
doc_type: howto
---

# Reasoning Routing

Reasoning routing lets callers request explicit reasoning or thinking controls while the router keeps provider credentials, model selection, budgets, and usage accounting server-side. The caller still requests one deployment-defined model group. The router filters that group's targets to the ones that can safely preserve the requested reasoning shape, then runs the group's configured strategy on the remaining targets.

Use this when a group should handle both ordinary requests and explicit reasoning requests. Ordinary requests can continue using the full ordinary eligible target mix. Requests with OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, or Anthropic Messages `thinking` use only compatible reasoning targets inside the same requested group.

## Configure Metadata

Reasoning metadata is eligibility metadata. Add it only after direct upstream and router-level smokes pass for the exact provider, model ID, dialect, and API skin. A model name or provider marketing page is not enough.

Minimal provider catalog example:

```yaml
providers:
  baseten:
    dialect: openai-chat
    models:
      glm-5-2:
        model: zai-org/GLM-5.2
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum
```

Use `control: effort_enum` for upstreams that accept levels such as `low`, `medium`, and `high`. Use `control: token_budget` for Anthropic-style thinking budgets. Add compatibility fields only when the exact upstream behavior is known:

```yaml
reasoning:
  supported: true
  mode: opt_in
  control: token_budget
  min_budget_tokens: 2048
  max_budget_tokens: 24576
  budget_must_be_less_than_max_tokens: true
  rejects_max_tokens: true
  rejects_temperature: true
  rejects_top_p: true
  supports_summaries: true
```

## Weighted Group Pattern

A mixed weighted group can prioritize validated reasoning targets only when the caller asks for reasoning. This keeps ordinary traffic on the configured weighted mix while preventing explicit reasoning controls from being dropped or sent to incompatible targets.

```yaml
models:
  coding:
    strategy: weighted
    targets:
      - provider: baseten
        model_ref: gpt-oss-120b
        weight: 60
      - provider: baseten
        model_ref: glm-5-2
        weight: 20
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum
      - provider: provider_without_reasoning
        model_ref: low-cost-text
        weight: 20
```

In this example, `coding` is only a sample group name. Ordinary compatible requests can use all ordinary eligible targets. A request that includes `reasoning_effort`, Responses `reasoning`, or Messages `thinking` can use only the target with compatible `reasoning` metadata. If several targets remain, the weighted strategy still applies to those remaining targets.

If no compatible target remains, the router returns `502 no-eligible-target` before sending an upstream request. The response requirements include `reasoning`.

## Caller Examples

Use the base URL and model groups issued by the deployment administrator. Public examples should use placeholder hosts and placeholder router tokens only.

OpenAI Chat Completions with `reasoning_effort`:

```bash
curl https://your-router.example.com/v1/chat/completions \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "coding",
    "messages": [{"role": "user", "content": "Reason briefly and answer OK."}],
    "reasoning_effort": "low",
    "max_tokens": 256,
    "stream": false
  }'
```

OpenAI Responses with `reasoning`:

```bash
curl https://your-router.example.com/v1/responses \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "coding",
    "input": "Reason briefly and answer OK.",
    "reasoning": {"effort": "low", "summary": "auto"},
    "max_output_tokens": 256,
    "stream": false
  }'
```

Anthropic Messages with `thinking`:

```bash
curl https://your-router.example.com/v1/messages \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "coding",
    "max_tokens": 1024,
    "thinking": {"type": "enabled", "budget_tokens": 512},
    "messages": [{"role": "user", "content": "Reason briefly and answer OK."}]
  }'
```

## Verify Model Metadata

Call `/v1/models` with the same router token the client will use. The response is filtered to that token's allow list.

```bash
curl "$ROUTER_BASE_URL/v1/models" \
  -H "Authorization: Bearer $ROUTER_TOKEN"
```

Reasoning-capable groups expose safe Codex metadata such as supported levels and summary support. Groups without validated reasoning targets omit these reasoning fields instead of returning empty or false placeholders.

```json
{
  "object": "list",
  "data": [
    {
      "id": "coding",
      "object": "model",
      "owned_by": "smart-llmrouter",
      "default_reasoning_summary": "none",
      "supported_reasoning_levels": [
        {"effort": "low", "description": "Fast responses with lighter reasoning"},
        {"effort": "medium", "description": "Balances speed and reasoning depth for everyday tasks"},
        {"effort": "high", "description": "Greater reasoning depth for complex problems"}
      ],
      "supports_reasoning_summaries": true
    }
  ]
}
```

The returned `id` values are deployment-defined router model groups, not provider model IDs and not a full upstream inventory.

For OpenAI Responses targets, publish these fields only after direct and router-level smokes pass for the exact model and endpoint. For example, a validated Responses target may advertise `low`, `medium`, and `high` after those `reasoning.effort` values return useful output and tool behavior is verified. Provider minimum output budgets still apply; if an upstream rejects tiny `max_output_tokens` values, keep that caveat in model metadata and use realistic acceptance budgets for coding-agent traffic.

## Prove The Running Deployment

Source config is not enough. After deployment, administrators should prove the running router with the same caller token and model group that clients use:

1. Call `/v1/models` and confirm the group advertises `supported_reasoning_levels` and `default_reasoning_level`.
2. Run one OpenAI Chat request with `reasoning_effort`.
3. Run one OpenAI Responses request with `reasoning.effort`.
4. Run one Anthropic Messages request with `thinking` when that surface is enabled.
5. Join usage telemetry by `X-Request-Id` and confirm the selected provider/model/dialect and translated reasoning control.

The operator smoke script `scripts/prod_reasoning_smoke.py` automates this for staging and production deployments. It prints only safe scalar evidence: request IDs, model group, selected provider/model/dialect, translated reasoning control, and fallback status. It does not print router tokens, provider keys, prompts, tool schemas, raw responses, or full config.

## Negative Eligibility Test

Run a reasoning request against a test group that has no reasoning-compatible target. The router should fail before upstream:

```bash
curl -i "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "text-only-test",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "reasoning_effort": "low",
    "max_tokens": 256,
    "stream": false
  }'
```

Expected result:

```json
{
  "error": {
    "type": "no-eligible-target",
    "message": "no eligible upstream target is configured for model \"text-only-test\" with openai-chat requests requiring text, reasoning, max_tokens",
    "details": {
      "model": "text-only-test",
      "dialect": "openai-chat",
      "requirements": ["text", "reasoning", "max_tokens"],
      "hint": "ask the router administrator to add or enable an upstream target for this model group that supports the requested API dialect, tools, and input modalities"
    }
  }
}
```

## Validation Checklist

Before enabling reasoning metadata in an active group:

- run a direct upstream smoke for the exact provider, model ID, dialect, and reasoning control;
- run the same request through a router-level smoke group;
- test OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, and Anthropic Messages `thinking` separately when those API skins will be exposed;
- use realistic output budgets for acceptance because reasoning-heavy models can spend small caps on internal reasoning and return empty final content;
- run low-cap tests such as `max_tokens: 1`, `max_completion_tokens: 1`, or `max_output_tokens: 1` to prove cap forwarding, target skipping, or configured translation;
- verify `/v1/models` exposes the intended reasoning metadata to allowed callers;
- run a negative `no-eligible-target` test and confirm no upstream attempt is recorded;
- query usage/reporting telemetry after success and failure cases to confirm selected provider/model, attempts, status, latency, throughput, token counts, cost fields, and safe reasoning metadata.

If validation fails, remove the `reasoning` metadata or keep the target catalog-only until the exact request shape passes. Roll back by restoring the previous target metadata, relaxing a reasoning-only contract or dynamic-score hard filter, or reverting the group config from the deployment backup.

## Dynamic Score And Contracts

The same request eligibility rules apply to `weighted`, `failover`, `dynamic_score`, `script`, and `external` strategies. Dynamic score can add `hard_filters.require_reasoning_support_when_requested: true`, but target metadata is still the baseline that tells the router which targets can preserve explicit reasoning controls.

Use a model-group contract `required_capabilities.reasoning` when the group itself is intended to always be reasoning-capable. Use per-target reasoning metadata in a mixed weighted group when ordinary requests should still use non-reasoning targets, but explicit reasoning requests must be restricted to validated reasoning targets.

## Public Documentation Safety

Public docs and examples must not include real router tokens, provider keys, token hashes, production hostnames, private deployment paths, private headers, or exact private production config. Use placeholder URLs, placeholder token variables, sample model group names, and sanitized response examples.
