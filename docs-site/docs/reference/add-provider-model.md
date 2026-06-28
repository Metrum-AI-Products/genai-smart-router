---
title: Add A Provider Or Model
---

# Add A Provider Or Model

Use this process before adding a new upstream model to active routing. It applies to external providers, OpenAI-compatible aggregators, Baseten-style endpoints, and self-hosted vLLM/SGLang deployments.

For hosted OpenAI-compatible services such as Crusoe Managed Inference or Fireworks AI, validate each API skin separately. Crusoe public docs checked on 2026-06-24 show `https://api.inference.crusoecloud.com/v1` as the OpenAI-compatible endpoint and API keys from the Crusoe Intelligence Foundry console. Fireworks public docs checked on 2026-06-28 show `https://api.fireworks.ai/inference/v1` as the OpenAI-compatible endpoint, `FIREWORKS_API_KEY` authentication, account-qualified model IDs, Serverless per-token pricing, and a Responses API with function tools plus provider-hosted MCP/SSE tools. Treat public model lists and pricing as source-dated discovery input; keep models catalog-only until the deployment account and exact model IDs pass direct provider smokes, router-level smokes, and any workload acceptance tests. Do not infer Fireworks Responses, Anthropic Messages, image, video, or audio support from Fireworks Chat validation.

Provider examples in these docs are validation patterns, not promises that a public provider, account, region, or model is active in every deployment. Revalidate provider docs, account entitlement, pricing, model IDs, tool behavior, modality support, streaming, usage reporting, and max-token cap behavior for the exact deployment before promotion.

## 1. Capture Required Metadata

Record:

- provider name and `base_url`;
- API dialect and authentication scheme;
- served model ID;
- model modalities;
- tool support by API shape;
- pricing or internal chargeback rates;
- pricing source and update date;
- operating notes such as `honors_max_tokens: false`.

Keep unavailable or unvalidated provider models catalog-only. Move a model into active routing after the deployment has entitlement and validation evidence for the API shapes it will serve.

## 2. Run Direct Provider Smokes

Run direct upstream requests before involving the router:

- text completion with a realistic output cap for the caller API, such as `max_tokens`, `max_completion_tokens`, or `max_output_tokens`;
- small cap request such as OpenAI Chat `max_completion_tokens: 1` when cap behavior matters;
- tool request for each API shape you plan to support;
- structured-output request when declaring JSON/schema capability;
- streaming request when the route will serve streaming clients;
- image request when declaring `image` modality;
- usage and cost inspection when the upstream returns token or billed-cost fields;
- client compatibility smoke for Codex, Claude Code, Cursor, Warp, or another client that depends on a specific skin.

OpenRouter Nitro variants may not appear as separate model IDs in `/models`; validate the exact `:nitro` suffix with a real completion call.

Reasoning-heavy models can return HTTP 200 with empty final content when the output budget is too small. Test both a tiny cap and a realistic budget before activating them.

Some providers return reasoning text separately from visible assistant content. For example, Fireworks GPT OSS 20B returns `reasoning_content` on Chat Completions responses and accepts OpenAI Chat `reasoning_effort` values after direct validation. Only declare router `reasoning` metadata after the same reasoning request passes through the router for the exact provider, model, dialect, and skin.

## 3. Add Catalog Metadata

Add provider catalog metadata with pricing, modality, tool, and cap fields. Keep routing weights out of provider catalogs.

## 4. Add A Smoke Group First

Create a deployment-defined smoke group with one target and no broad caller access. Run router-level smokes against the same API shapes tested directly.

Example hosted OpenAI-compatible smoke group:

```yaml
providers:
  crusoe:
    base_url: https://api.inference.crusoecloud.com/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${CRUSOE_API_KEY}
    api_key_env: CRUSOE_API_KEY
    key_id: crusoe-primary
    headers:
      User-Agent: smart-llmrouter
    models:
      llama-3-3-70b-instruct:
        model: meta-llama/Llama-3.3-70B-Instruct
        input_price_per_million_usd: 0.25
        output_price_per_million_usd: 0.75
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: Direct Crusoe and local router-level text, streaming, max_tokens=1, auto tool, forced tool_choice, structured-output, usage, cost, latency, and no-fallback smokes passed on 2026-06-24 with an explicit User-Agent; keep out of broad groups until workload gates pass for this account and model.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]

models:
  hosted-openai-compatible-smoke:
    strategy: static
    targets:
      - { provider: crusoe, model_ref: llama-3-3-70b-instruct }
```

Do not declare `tool_support`, `structured_outputs`, `reasoning`, image/audio/video modalities, or `honors_max_tokens` behavior from provider marketing copy. Declare them only after the exact request shape passes direct and router smokes. OpenAI Chat support does not imply OpenAI Responses support, and neither implies Anthropic Messages support; each dialect/skin needs independent direct upstream and router-level validation.

## 5. Add Production Weight Conservatively

Start with a low weight in active groups. Increase only after:

- request logs show normal status and latency;
- usage/cost rows are populated;
- capped requests behave as expected;
- CLI/tool/image smokes pass where relevant;
- production logs do not show repeated fallback or provider failures.

## 6. Update Docs And Reporting

Update:

- external model metadata docs if the new capability is user-visible;
- operator rollout history and deployment notes;
- sample config if the provider/model should be part of reference config;
- usage reports if a new cost or modality field affects accounting.

## Rollback

Rollback should be a config-only weight or target change when possible:

1. Remove the active target from affected groups, or isolate it in a restricted smoke group when continued validation is needed.
2. Keep the catalog entry with notes unless the model ID was wrong.
3. Restart the router and run `/readyz`.
4. Run a request through affected groups to confirm another eligible target is selected.
