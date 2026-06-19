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
          openai_chat: [tools]
```

## Modalities

`input_modalities` and `output_modalities` control routing eligibility.

| Modality | Use |
|---|---|
| `text` | Plain text input or output |
| `image` | Image input for VLM/OCR/browser-control workflows |
| `video` | Video input when the upstream/provider supports it |

Do not mark a modality as active because a provider marketing page mentions it. Validate the exact account, endpoint, model ID, request shape, and deployment region.

## Tool Support

Tool support is API-shape-specific. A model that handles OpenAI Chat tools may not handle Anthropic Messages tools through the same provider endpoint.

Declare only what has passed direct upstream and router-level smokes:

```yaml
tool_support:
  openai_chat: [tools, structured_outputs]
  openai_responses: [function]
  anthropic_messages: [client_tools]
```

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
- the model exists but should not receive production traffic yet.

## Group Targets

```yaml
models:
  example-general:
    strategy: weighted
    targets:
      - provider: openrouter
        model_ref: deepseek-v4-flash-nitro
        weight: 60
      - provider: internal_vllm
        model_ref: llama-70b
        weight: 40
```

Model group names are deployment-defined. Use names that match the organization's policy and caller contracts.

