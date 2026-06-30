---
title: Add A Provider Or Model
doc_type: reference
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

Treat provider access failures as activation blockers. A direct smoke returning `401`, entitlement-shaped `403`, generic access `403`, or model-access `404` means the target should not receive ordinary traffic until the exact provider credential, account/project/region, model ID, dialect, and request shape are fixed and retested. Router-level smokes should show sanitized access-failure classes and never expose provider keys or raw upstream bodies.

## 2. Run Direct Provider Smokes

For repeatable first-pass capability evidence, run the repository probe script from a protected shell that has only the relevant provider key in the named environment variable:

```bash
scripts/probe-model-capabilities.sh \
  --base-url https://api.provider.example/v1 \
  --model provider-model-id \
  --api-key-env PROVIDER_API_KEY \
  --dialect openai-chat \
  --output yaml | tee probe-results.yaml
```

The probe is not a replacement for operator review or workload validation. It produces a structured checklist of direct upstream smokes and a `recommended_config` block that should be copied only after you inspect the failed, skipped, and informational rows. Capabilities that were not tested stay omitted from provider metadata; omitted metadata makes the router skip that target for requests that require the capability.

Run direct upstream requests before involving the router:

- text completion with a realistic output cap for the caller API, such as `max_tokens`, `max_completion_tokens`, or `max_output_tokens`;
- small cap request such as OpenAI Chat `max_completion_tokens: 1` when cap behavior matters;
- tool request for each API shape you plan to support;
- structured-output request when declaring JSON/schema capability;
- streaming request when the route will serve streaming clients;
- image request when declaring `image` modality;
- usage and cost inspection when the upstream returns token or billed-cost fields;
- client compatibility smoke for Codex, Claude Code, Cursor, Warp, or another client that depends on a specific skin.

For coding-agent or retrieval-heavy groups, add a large OpenAI Chat payload smoke when the target will receive Chat Completions traffic from agents. Use a sanitized synthetic fixture rather than captured customer content. The repository helper below generates filler messages and representative function schemas, prints only scalar request-shape metrics, and can run against either a direct upstream endpoint or a router model group:

```bash
rtk python3 scripts/large_payload_chat_smoke.py \
  --base-url https://api.provider.example/v1 \
  --model provider-model-id \
  --api-key-env PROVIDER_API_KEY \
  --target-bytes 524288 \
  --tool-count 24 \
  --max-tokens 32
```

Promote `supports_large_coding_agent_payloads: true` only after direct upstream and router-level smokes pass for the exact provider, model ID, dialect, account, and request shape. Record the date, request bytes, tool count, serialized tool-schema size, output cap, prompt-token scale, latency, status, and token usage in `validation_notes`. Production evidence should use a deployment-owned smoke group and a safe existing caller token; if that caller or report access is not available, record the blocker instead of copying token files or captured payloads.

For opencode-style coding-agent traffic, run the API capability matrix before declaring support for an endpoint. The matrix sends synthetic OpenAI Chat and Anthropic Messages text, client-tool, and image requests and records sanitized pass/fail evidence:

```bash
rtk python3 scripts/opencode_api_matrix.py \
  --base-url https://api.provider.example/v1 \
  --model provider-model-id \
  --api-key-env PROVIDER_API_KEY \
  --dialects openai-chat,anthropic \
  --tasks text,tools,image \
  --output-dir tmp/opencode-api-matrix
```

A partial pass is still useful evidence. For example, a model that passes OpenAI Chat tools and Anthropic Messages tools but rejects image payloads can be routed for text/tool workloads only; do not add `image` metadata or mixed image-bearing agent routing until the exact provider, model, account, and router skin pass image smokes.
The command exits zero after writing the evidence files by default, even when individual capability rows fail. Add `--strict-exit` only when a CI job should fail on any non-passing row. Text rows require the expected text, default `OK`; image rows require the expected receipt text, default `Rite Aid`, before they are marked as passes.

OpenRouter Nitro variants may not appear as separate model IDs in `/models`; validate the exact `:nitro` suffix with a real completion call.

Reasoning-heavy models can return HTTP 200 with empty final content when the output budget is too small. Test both a tiny cap and a realistic budget before activating them.

Some providers return reasoning text separately from visible assistant content. For example, Fireworks GPT OSS 20B returns `reasoning_content` on Chat Completions responses and accepts OpenAI Chat `reasoning_effort` values after direct validation. Only declare router `reasoning` metadata after the same reasoning request passes through the router for the exact provider, model, dialect, and skin.

## 3. Capture Capability-Probe Results

Keep an onboarding result for every capability claim. The public-safe version should include:

- provider and endpoint family;
- model ID or customer-facing model group;
- API shape, such as OpenAI Chat, OpenAI Responses, or Anthropic Messages;
- capability tested, such as text, streaming, max-token cap, tools, forced tool choice, structured outputs, reasoning, image input, or usage reporting;
- validation layer, such as direct upstream, router-level, or client smoke;
- test date, pass/fail result, status code or safe error type, latency, token usage, selected upstream model, and fallback status when available;
- promotion decision, such as catalog-only, smoke-only, limited weight, active, or rolled back.

Do not publish raw provider keys, router tokens, token hashes, private hostnames, full config, raw prompts, raw images, raw tool schemas, raw tool outputs, or unsanitized provider responses. Summarize the observed behavior and keep private evidence in deployment-controlled systems.

Map probe results to catalog metadata mechanically:

| Probe result | Catalog field to consider | Rule |
|---|---|---|
| `text: pass` | `model`, text modalities | Required before any route uses the target. |
| `max-tokens-cap: fail` | `honors_max_tokens: false` | Mark false so explicit capped requests skip the target. |
| `auto-tools: pass` | `tool_support.<skin>: [tools]` or `function` / `client_tools` | Add only for the tested dialect. |
| `forced-tools: pass` | `tool_choice` | Add only when object/forced tool choice passed. |
| `structured-outputs: pass` | `structured_outputs` | Add only for the tested dialect and schema form. |
| `image-input: pass` | `input_modalities: [text, image]` | Add only after direct and router-level image smokes pass. |
| `reasoning-effort: pass` | `reasoning` | Set the control type that was actually tested. |
| `skip`, `fail`, or `info` | no active capability tag | Keep the capability omitted and document the result in `pricing_notes`. |

Every API skin needs independent evidence. OpenAI Chat tool support does not prove OpenAI Responses function tools or Anthropic Messages client tools.

One upstream model can have multiple provider skins. Keep each skin as its own provider entry when the upstream exposes distinct endpoints or request contracts:

```yaml
providers:
  minimax:
    base_url: https://api.minimax.io/v1
    dialect: openai-chat
    api_key_env: MINIMAX_API_KEY
    models:
      m3:
        model: MiniMax-M3
        tool_support:
          openai_chat: [tools, tool_choice]
  minimax_responses:
    base_url: https://api.minimax.io/v1
    dialect: openai-responses
    api_key_env: MINIMAX_API_KEY
    models:
      m3:
        model: MiniMax-M3
        input_modalities: [text]
        tool_support:
          openai_responses: [function]
        force_store_false: true
  minimax_anthropic:
    base_url: https://api.minimax.io/anthropic
    dialect: anthropic
    api_key_env: MINIMAX_API_KEY
    models:
      m3:
        model: MiniMax-M3
        tool_support:
          anthropic_messages: [client_tools]
```

This keeps Chat, Responses, and Messages routing eligibility independent. A Codex `/v1/responses` request will not select the Chat-only `minimax` target just because it has the same upstream model name.

## 4. Add Catalog Metadata

Add provider catalog metadata with pricing, modality, tool, and cap fields. Keep routing weights out of provider catalogs.

When one upstream model is available through multiple API skins, model each active skin explicitly. This keeps eligibility understandable for callers and reports:

```yaml
providers:
  provider_chat:
    base_url: https://provider.example.com/v1
    dialect: openai-chat
    api_key_env: PROVIDER_API_KEY
    models:
      shared-model:
        model: provider/shared-model
        tool_support:
          openai_chat: [tools, tool_choice]
  provider_responses:
    base_url: https://provider.example.com/v1
    dialect: openai-responses
    api_key_env: PROVIDER_API_KEY
    models:
      shared-model:
        model: provider/shared-model
        tool_support:
          openai_responses: [function]
  provider_messages:
    base_url: https://provider.example.com
    dialect: anthropic
    api_key_env: PROVIDER_API_KEY
    models:
      shared-model:
        model: provider/shared-model
        tool_support:
          anthropic_messages: [client_tools]

models:
  agent-coding:
    strategy: weighted
    targets:
      - { provider: provider_chat, model_ref: shared-model, weight: 50 }
      - { provider: provider_responses, model_ref: shared-model, weight: 25, tool_only: true }
      - { provider: provider_messages, model_ref: shared-model, weight: 25, tool_only: true }
```

After adding the smoke group, check provider catalog status. The Chat target should show `activeEligibilitySkin: native:openai-chat`; the Responses target should show `native:openai-responses`; and the Messages target should show `native:anthropic`. If a row lists a capability under `inactiveToolSupport`, that capability is metadata-only for that active target and will not make the target eligible for that caller surface.

## 5. Add A Smoke Group First

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
        pricing_notes: Keep source and update-date evidence in config.example.yaml. Direct Crusoe and local router-level text, streaming, max_tokens=1, auto tool, forced tool_choice, structured-output, usage, cost, latency, and no-fallback smokes passed with an explicit User-Agent; keep out of broad groups until workload gates pass for this account and model.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]

models:
  hosted-openai-compatible-smoke:
    strategy: static
    targets:
      - { provider: crusoe, model_ref: llama-3-3-70b-instruct }
```

Do not declare `tool_support`, `structured_outputs`, `reasoning`, image/audio/video modalities, or `honors_max_tokens` behavior from provider marketing copy. Declare them only after the exact request shape passes direct and router smokes. OpenAI Chat support does not imply OpenAI Responses support, and neither implies Anthropic Messages support; each dialect/skin needs independent direct upstream and router-level validation.

### Optional Chat To Responses Bridge Smoke

If a deployment needs OpenAI Chat Completions callers to use a Responses-only upstream, add a restricted smoke target with explicit bridge metadata after the Responses target has passed direct text and function-tool smokes:

```yaml
providers:
  responses_provider:
    base_url: https://provider.example.com/v1
    dialect: openai-responses
    api_key_env: PROVIDER_API_KEY
    models:
      responses-model:
        model: provider/responses-model
        input_modalities: [text]
        output_modalities: [text]
        tool_support:
          openai_responses: [function]

models:
  chat-to-responses-smoke:
    strategy: static
    targets:
      - provider: responses_provider
        model_ref: responses-model
        bridges:
          chat_to_responses:
            enabled: true
            tools: true
            tool_choice: true
```

Run router-level `POST /v1/chat/completions` smokes for non-streaming text, function-tool calls, and a negative unsupported shape such as `stream:true` or image input when those modes are not enabled. Verify usage and diagnostic rows show inbound Chat, target Responses, `/v1/responses` endpoint path, safe translation-shape buckets, and no raw prompt/tool-schema persistence. Do not enable bridge streaming, images, structured outputs, reasoning, or parallel tool calls until those exact bridge shapes pass.

## 6. Add Production Weight Conservatively

Start with a low weight in active groups. Increase only after:

- request logs show normal status and latency;
- usage/cost rows are populated;
- capped requests behave as expected;
- CLI/tool/image smokes pass where relevant;
- production logs do not show repeated fallback or provider failures.

## 7. Update Docs And Reporting

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
