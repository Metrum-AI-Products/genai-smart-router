---
title: Provider Catalog
doc_type: reference
---

# Provider Catalog

Provider catalog entries define upstream skins, credentials, model IDs, request dialects, tested capabilities, modalities, and request-time cost metadata. Callers request deployment-defined model groups; they do not need to know upstream provider model IDs.

This example is a partial subset of `config.example.yaml`; the shipped sample config is the source of truth for exact model IDs, pricing fields, source URLs, and update dates.

```yaml title="config.example.yaml"
providers:
  baseten:
    base_url: https://inference.baseten.co/v1
    dialect: openai-chat
    api_key: ${BASETEN_API_KEY}
    api_key_env: BASETEN_API_KEY
    key_id: baseten-default
    models:
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        tier: coding
        input_price_per_million_usd: 0.1
        output_price_per_million_usd: 0.5
        input_modalities:
        - text
        output_modalities:
        - text
        tool_support:
          openai_chat:
          - tools
          - tool_choice
```

## Schema

Provider-level fields identify the upstream API skin:

- `base_url`, `dialect`, `auth_scheme`, `api_key_env`, and `headers` control how the router calls upstream.
- `models.<ref>.model` is the exact upstream model ID.
- `input_price_per_million_usd`, `output_price_per_million_usd`, optional image pricing fields, `pricing_source`, `pricing_updated_at`, and `pricing_notes` are stored in `config.example.yaml` and copied into request usage rows at request time.
- `input_modalities` and `output_modalities` describe validated I/O such as `text`, `image`, or `video`.
- `tool_support.openai_chat`, `tool_support.openai_responses`, and `tool_support.anthropic_messages` are per-skin validation evidence, not marketing claims.
- `reasoning` and `honors_max_tokens` further constrain eligible targets for reasoning requests or explicit caller output caps.

Internal vLLM, SGLang, Baseten, Crusoe, Fireworks, OpenRouter, Anthropic, MiniMax, Kimi, xAI, and OpenAI-compatible services all use this same catalog shape. Configure separate provider skins when the same upstream exposes multiple dialects, such as OpenAI Chat and OpenAI Responses.

## Rollback

If a cataloged model fails validation, remove it from every active `models.<group>.targets[]` entry first, keep the provider catalog entry only if it remains useful for smoke testing, and restart or reload through the normal deployment path. Remove capability fields such as `structured_outputs`, `image`, or `tool_choice` when only that request shape fails.

## Related

See [Providers And Models](../providers-models/overview), [Add A Provider Or Model](../reference/add-provider-model), [Model Metadata](../reference/model-metadata), [Self-Hosted Upstreams](./self-hosted-upstreams), and [Router Configuration](./router-config).
