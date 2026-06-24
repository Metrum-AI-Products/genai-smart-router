---
title: Model Metadata
---

# Model Metadata

Model metadata tells the router which upstream targets are eligible for a request and how to account for usage. Provider catalogs are metadata; traffic weights live only under `models.<group>.targets[]`.

## Provider Catalog Fields

```yaml
providers:
  provider_name:
    base_url: https://provider.example.com/v1
    dialect: openai-chat
    api_key_env: PROVIDER_API_KEY
    key_id: provider-primary
    models:
      model-ref:
        model: provider/model-id
        tier: general
        input_modalities: [text, image]
        output_modalities: [text]
        input_price_per_million_usd: 0.20
        output_price_per_million_usd: 1.00
        pricing_source: https://provider.example.com/pricing
        pricing_updated_at: "2026-06-19"
        honors_max_tokens: true
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
          openai_responses: [function, structured_outputs]
```

## Modalities

`input_modalities` and `output_modalities` control routing eligibility.

| Modality | Use |
|---|---|
| `text` | Plain text input or output |
| `image` | Image input for VLM/OCR/browser-control workflows |
| `video` | Video input when the upstream/provider supports it |

Mark a modality active after validating the exact account, endpoint, model ID, request shape, and deployment region.

## Tool Support

Tool support is API-shape-specific. A model that handles OpenAI Chat tools may not handle Responses function tools or Anthropic Messages tools through the same provider endpoint. Structured-output support is tracked in the same metadata object because it is also a dialect-specific request-shape eligibility requirement.

Declare only what has passed direct upstream and router-level smokes:

```yaml
tool_support:
  openai_chat: [tools, tool_choice, structured_outputs]
  openai_responses: [function, structured_outputs]
  anthropic_messages: [client_tools]
```

Capability labels:

| Label | Meaning |
|---|---|
| `tools` | OpenAI Chat tool payloads are accepted and produce correctly shaped tool calls |
| `tool_choice` | OpenAI Chat `tool_choice` modes used by clients are accepted |
| `function` | OpenAI Responses function tools are accepted and produce correctly shaped tool calls |
| `client_tools` | Anthropic Messages client tools are accepted and produce correctly shaped tool calls |
| `structured_outputs` | The matching OpenAI dialect accepts JSON Schema structured-output requests |
| `provider_hosted` | Reserved for provider-executed tools after exact upstream validation |

`openai_chat` and `openai_responses` are separate validation surfaces. Declare `structured_outputs` under `openai_chat` only after a direct upstream Chat Completions `response_format` smoke and a router-level Chat smoke pass for the exact provider, model ID, dialect, and skin. Declare it under `openai_responses` only after the same direct and router-level evidence exists for Responses `text.format`.

Tool support and structured-output support are independent. A target may support tools but not structured outputs, structured outputs but not tools, or both. A request containing both tools and structured-output fields needs a target that satisfies both requirements. Unsupported targets are skipped before routing policy selection; if no compatible target remains, callers receive `502 no-eligible-target` and no upstream request is sent.

The router forwards schema payloads to the selected upstream. It does not validate arbitrary JSON Schema subsets, enforce provider-specific schema limits, or repair nonconforming model output unless a separate implementation adds that behavior. Unsupported schemas may therefore return upstream/provider errors even when the target is correctly marked as structured-output capable.

## Pricing And Cost Fields

Set pricing metadata for every active target when a price or internal chargeback rate is known:

- `input_price_per_million_usd`
- `output_price_per_million_usd`
- `image_input_price_per_million_tokens_usd`
- `image_input_price_per_image_usd`
- `pricing_source`
- `pricing_updated_at`
- `pricing_notes`

The router stores the prices and calculated costs used at request time. Historical reports therefore keep the cost assumptions that were true when the request ran, even if provider pricing changes later.

For self-hosted models, use the enterprise chargeback rate. Use `0.00` only when reports should show token volume without allocated GPU cost.

## Active, Disabled, And Catalog-Only

Cataloging a model does not send traffic to it. A model becomes active only when referenced under a model group's `targets`.

Keep a model catalog-only when:

- the current key is not entitled;
- direct provider smoke failed;
- tool or image support is unvalidated;
- cap behavior is unsafe;
- the model exists and is best kept out of production traffic until rollout criteria are met.

## Group Targets

```yaml
models:
  example-general:
    strategy: weighted
    targets:
      - provider: baseten
        model_ref: gpt-oss-120b
        weight: 60
      - provider: internal_vllm
        model_ref: llama-70b
        weight: 40
```

Model group names are deployment-defined. Use names that match the organization's policy and caller contracts.
