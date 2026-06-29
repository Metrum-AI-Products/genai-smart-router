---
title: Router Configuration
---

# Router Configuration

The router is configured with YAML plus environment-loaded provider keys. Customers normally keep provider credentials in the deployment environment, a secret manager, or a protected runtime `env.json` file. The shipped `env.example.json` is only a shape template with empty provider-key placeholders.

Model group names are deployment-defined. Names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` may appear in examples because they are used by a reference or hosted deployment; the product does not require those names.

For a strategy-by-strategy ownership guide that ties caller access, group-local routing, validation, policy services, and rollback evidence together, see [Customer-Controlled Routing](../routing/customer-controlled-routing).

<div class="contactBanner">
  <p>Metrum can help design a production routing policy. Contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## License

Normal release builds require offline signed JSON license enforcement. The router verifies a Metrum-issued license file with embedded Ed25519 public keys, checks expiry and licensed features, and rechecks the file periodically so renewals do not require a rebuild. Runtime YAML cannot disable licensing in packaged deployments.

```yaml
server:
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
    recheck_interval: 1h
    grace_period_on_validation_error: 24h
```

When enforcement blocks serving, `/readyz` fails and caller APIs return `license-*` errors. License status in logs, metrics, reports, and admin status APIs is limited to safe scalar metadata such as status, license ID, customer ID, SKU, key ID, expiry, and grace flag. License payloads, signatures, and signing keys are not exposed.

For renewal, grace, failure modes, admin visibility, and smoke commands, see [License-Protected Deployments](../operations/license-protected-deployments).

## Provider And Model Catalog

```yaml
providers:
  minimax:
    base_url: https://api.minimax.io/v1
    dialect: openai-chat
    api_key: ${MINIMAX_API_KEY}
    api_key_env: MINIMAX_API_KEY
    key_id: minimax-primary
    models:
      m3:
        model: MiniMax-M3
        tier: heavy
        input_price_per_million_usd: 0.30
        output_price_per_million_usd: 1.20
        input_modalities: [text, image, video]
        output_modalities: [text]
        pricing_source: https://platform.minimax.io/docs/pricing/overview
        pricing_updated_at: "2026-06-17"
        tool_support:
          openai_chat: [tools, tool_choice]
          openai_responses: [function]

  xai:
    base_url: https://api.x.ai/v1
    dialect: openai-chat
    api_key: ${XAI_API_KEY}
    api_key_env: XAI_API_KEY
    key_id: xai-primary
    models:
      grok-4-3:
        model: grok-4.3
        tier: vision
        input_price_per_million_usd: 1.25
        output_price_per_million_usd: 2.50
        image_input_price_per_million_tokens_usd: 1.25
        input_modalities: [text, image]
        output_modalities: [text]
        pricing_source: https://docs.x.ai/developers/models/grok-4.3
        pricing_updated_at: "2026-06-17"
        tool_support:
          openai_chat: [tools, structured_outputs]

  openrouter:
    base_url: https://openrouter.ai/api/v1
    dialect: openai-chat
    api_key: ${OPENROUTER_API_KEY}
    api_key_env: OPENROUTER_API_KEY
    key_id: openrouter-primary
    models:
      claude-sonnet-4-6:
        model: anthropic/claude-sonnet-4.6
        tier: coding
        input_modalities: [text, image]
        output_modalities: [text]
        pricing_source: https://openrouter.ai/api/v1/models
        pricing_updated_at: "2026-06-22"
        tool_support:
          openai_chat: [tools, tool_choice]

  baseten:
    base_url: https://inference.baseten.co/v1
    dialect: openai-chat
    api_key: ${BASETEN_API_KEY}
    api_key_env: BASETEN_API_KEY
    key_id: baseten-primary
    models:
      nemotron-120b-a12b:
        model: nvidia/Nemotron-120B-A12B
        tier: heavy
        input_price_per_million_usd: 0.30
        output_price_per_million_usd: 0.75
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.baseten.co/pricing/
        pricing_updated_at: "2026-06-17"
        pricing_notes: Baseten also publishes a discounted cache-input rate; keep standard input/output rates for router-calculated cost and log upstream-reported billed cost separately when available
        tool_support:
          openai_chat: [tools, tool_choice]
      glm-5-2:
        model: zai-org/GLM-5.2
        tier: coding
        input_price_per_million_usd: 1.50
        output_price_per_million_usd: 4.50
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.baseten.co/pricing/
        pricing_updated_at: "2026-06-18"
        pricing_notes: Baseten also publishes a discounted cache-input rate; GLM 5.2 is reasoning-heavy, so use realistic output budgets for acceptance and coding-agent traffic
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum
      glm-5p2:
        model: accounts/fireworks/models/glm-5p2
        tier: coding
        input_price_per_million_usd: 1.40
        output_price_per_million_usd: 4.40
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://fireworks.ai/models/fireworks/glm-5p2
        pricing_updated_at: "2026-06-28"
        tool_support:
          openai_chat: [tools]
      kimi-k2p7-code:
        model: accounts/fireworks/models/kimi-k2p7-code
        tier: coding
        input_price_per_million_usd: 0.95
        output_price_per_million_usd: 4.00
        input_modalities: [text, image]
        output_modalities: [text]
        pricing_source: https://fireworks.ai/models/fireworks/kimi-k2p7-code
        pricing_updated_at: "2026-06-28"
        tool_support:
          openai_chat: [tools]
      deepseek-v4-flash:
        model: accounts/fireworks/models/deepseek-v4-flash
        tier: coding
        input_price_per_million_usd: 0.14
        output_price_per_million_usd: 0.28
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://fireworks.ai/models/fireworks/deepseek-v4-flash
        pricing_updated_at: "2026-06-28"
        tool_support:
          openai_chat: [tools]
      qwen3p6-plus:
        model: accounts/fireworks/models/qwen3p6-plus
        tier: coding
        input_price_per_million_usd: 0.50
        output_price_per_million_usd: 3.00
        input_modalities: [text, image]
        output_modalities: [text]
        pricing_source: https://app.fireworks.ai/models/fireworks/qwen3p6-plus
        pricing_updated_at: "2026-06-28"
        tool_support:
          openai_chat: [tools]
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        tier: coding
        input_price_per_million_usd: 0.10
        output_price_per_million_usd: 0.50
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.baseten.co/products/model-apis/
        pricing_updated_at: "2026-06-22"
        pricing_notes: Baseten Model API; direct chat, streaming, auto tool, and forced tool-choice smokes passed on 2026-06-22
        tool_support:
          openai_chat: [tools, tool_choice]

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
        tier: balanced
        input_price_per_million_usd: 0.25
        output_price_per_million_usd: 0.75
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: Crusoe Managed Inference hosted OpenAI-compatible endpoint; direct and local router-level text, streaming, max_tokens=1, auto tool, forced tool_choice, structured-output, usage, cost, latency, and no-fallback smokes passed on 2026-06-24 with an explicit User-Agent. Keep cataloged or in dedicated smoke groups unless workload gates pass for the deployment account.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        tier: coding
        input_price_per_million_usd: 0.05
        output_price_per_million_usd: 0.20
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: Catalog-only until direct tool, cap, structured-output, router, and workload smokes pass
      gemma-4-31b-it:
        model: google/gemma-4-31b-it
        tier: balanced
        input_price_per_million_usd: 0.14
        output_price_per_million_usd: 0.40
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: OpenAI Chat text, streaming, max_tokens=1, auto tool, forced tool_choice, structured-output, and combined tool plus structured-output smokes passed on 2026-06-24. Keep cataloged or in dedicated smoke groups unless the exact deployment workload is revalidated; keep out of OpenAI Responses, Anthropic Messages, vision, and broad active tool routes unless those exact skins pass separately.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
      nemotron-3-nano-omni-reasoning-30b-a3b:
        model: nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B
        tier: vision
        input_price_per_million_usd: 0.30
        output_price_per_million_usd: 1.83
        image_input_price_per_million_tokens_usd: 0.30
        input_modalities: [text, image]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-25"
        pricing_notes: Text and output-cap smokes passed on 2026-06-25, but direct receipt-image OCR did not pass acceptance; keep in a dedicated smoke group until image workload gates pass
  fireworks:
    base_url: https://api.fireworks.ai/inference/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${FIREWORKS_API_KEY}
    api_key_env: FIREWORKS_API_KEY
    key_id: fireworks-primary
    headers:
      User-Agent: smart-llmrouter
    models:
      gpt-oss-20b:
        model: accounts/fireworks/models/gpt-oss-20b
        tier: coding
        input_price_per_million_usd: 0.07
        output_price_per_million_usd: 0.30
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://docs.fireworks.ai/serverless/pricing
        pricing_updated_at: "2026-06-27"
        pricing_notes: Fireworks Serverless pricing also lists $0.035/M cached input for GPT OSS 20B. Direct Fireworks OpenAI Chat text, streaming, max_tokens=1, reasoning_effort low/medium/high, auto tool with max_tokens >= 256, forced tool_choice, and structured-output smokes passed on 2026-06-27 with an explicit User-Agent; this ID completed even though it was not listed by /models for the validated account.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum

  fireworks_responses:
    base_url: https://api.fireworks.ai/inference/v1
    dialect: openai-responses
    auth_scheme: bearer
    api_key: ${FIREWORKS_API_KEY}
    api_key_env: FIREWORKS_API_KEY
    key_id: fireworks-responses-primary
    headers:
      User-Agent: smart-llmrouter
    models:
      kimi-k2p7-code:
        model: accounts/fireworks/models/kimi-k2p7-code
        tier: coding
        input_price_per_million_usd: 0.95
        output_price_per_million_usd: 4.00
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://docs.fireworks.ai/serverless/pricing
        pricing_updated_at: "2026-06-28"
        pricing_notes: Fireworks Responses text, function tools, tool-result continuation, streaming tool calls, max_output_tokens=1, max_tool_calls=1, and store=false smokes passed on 2026-06-28
        tool_support:
          openai_responses: [function]
        force_store_false: true

  baseten_anthropic:
    base_url: https://inference.baseten.co
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${BASETEN_API_KEY}
    api_key_env: BASETEN_API_KEY
    key_id: baseten-anthropic-primary
    models:
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        tier: coding
        input_price_per_million_usd: 0.10
        output_price_per_million_usd: 0.50
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.baseten.co/products/model-apis/
        pricing_updated_at: "2026-06-22"
        pricing_notes: Baseten Anthropic Messages API support is beta; direct text and client-tool smokes passed for this model on 2026-06-22
        tool_support:
          anthropic_messages: [client_tools]

  kimi:
    base_url: https://api.moonshot.ai/v1
    dialect: openai-chat
    api_key: ${MOONSHOT_API_KEY}
    api_key_env: MOONSHOT_API_KEY
    key_id: kimi-primary
    models:
      kimi-k2-7-code:
        model: kimi-k2.7-code
        tier: coding
        input_price_per_million_usd: 0.74
        output_price_per_million_usd: 3.50
        pricing_source: https://platform.kimi.ai/docs/pricing/chat-k27-code
        pricing_updated_at: "2026-06-17"

  vllm_internal:
    base_url: http://vllm-qwen-tools.inference.svc.cluster.local:8000/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${VLLM_QWEN_API_KEY}
    api_key_env: VLLM_QWEN_API_KEY
    key_id: vllm-qwen-tools-prod
    models:
      qwen3-coder-tools:
        model: qwen3-coder-tools
        tier: coding
        input_price_per_million_usd: 0.00
        output_price_per_million_usd: 0.00
        input_modalities: [text]
        output_modalities: [text]
        pricing_notes: internal GPU allocation; set chargeback values if used for reporting
        tool_support:
          openai_chat: [tools, tool_choice]
```

Internal vLLM and SGLang services use the same provider catalog structure as external OpenAI-compatible providers. Set `base_url` to the private service `/v1` endpoint and catalog the model ID returned by the upstream `/v1/models` endpoint. See [Self-Hosted Upstreams](./self-hosted-upstreams) for vLLM/SGLang deployment and tool-call examples.

External OpenAI-compatible providers follow the same shape. For example, Baseten Model APIs use `base_url: https://inference.baseten.co/v1` with `dialect: openai-chat`, and Crusoe Managed Inference uses `base_url: https://api.inference.crusoecloud.com/v1` with `dialect: openai-chat`; callers still request a deployment-defined router model group, not the upstream provider model ID. For Claude Code-style traffic, Baseten's Anthropic Messages beta endpoint can be configured as a separate `dialect: anthropic` provider with `base_url: https://inference.baseten.co`. The router injects provider keys such as `BASETEN_API_KEY` or `CRUSOE_API_KEY` only when a matching target is selected.

Crusoe support in these examples is OpenAI Chat only. A deployment can catalog Crusoe models, expose dedicated Crusoe smoke groups, and place a validated Crusoe target in an ordinary-text weighted group. The current/reference `big-coder` example uses Crusoe Nemotron 3 Nano Omni Reasoning only as a text target where configured; it does not claim Crusoe tool, vision, OpenAI Responses, or Anthropic Messages support until those exact direct and router-level smokes pass. Crusoe Gemma 4 31B-it should be treated as historical/catalog/smoke-only unless a deployment revalidates it for the exact active route. If a Crusoe VLM accepts an image but fails the deployment's OCR or image-reasoning acceptance tests, keep it in a smoke group instead of broad `vision` routing.

Fireworks Chat and Fireworks Responses are separate provider skins. Fireworks Chat uses `dialect: openai-chat`; Fireworks Responses uses a separate `dialect: openai-responses` provider. Fireworks docs checked on 2026-06-28 list `https://api.fireworks.ai/inference/v1` as the OpenAI-compatible base URL and document Responses function tools, provider-hosted MCP/SSE tools, streaming, `max_tool_calls`, and `store=false`. Direct Fireworks OpenAI Chat text and auto-tool smokes passed on 2026-06-28 for GLM 5.2, Kimi K2.7 Code, DeepSeek-V4-Flash, and Qwen3.6 Plus with an explicit `User-Agent`; GPT OSS 20B had already passed text, streaming, cap, reasoning effort, tool-choice, and structured-output smokes on 2026-06-27. Active router targets keep Fireworks image-capable Chat models text-only until direct image and router-level image smokes pass for each exact endpoint. The reference Responses entry is limited to `accounts/fireworks/models/kimi-k2p7-code` after direct and router-level text, function-tool, continuation, streaming, output-cap, and `store=false` smokes passed. Provider-hosted MCP/SSE tools are rejected by the router before upstream unless a deployment adds a separate reviewed allowlist design. Keep Fireworks Anthropic Messages, video, and audio support absent until those exact skins pass direct and router-level smokes.

Catalog entries should carry cost and capability metadata:

- `input_price_per_million_usd` and `output_price_per_million_usd` are the values used to calculate per-request cost at the time the request runs.
- `input_modalities` and `output_modalities` describe tested model I/O such as `text`, `image`, or `video`. Requests that include image content are automatically filtered to targets with `image` in `input_modalities`; text-only targets are skipped.
- `image_input_price_per_million_tokens_usd` and `image_input_price_per_image_usd` are optional VLM pricing fields. Use them only when the provider or internal chargeback model bills image input differently from ordinary input tokens. If the provider returns billed cost in usage metadata, the router logs that upstream-reported cost separately from the calculated cost.
- `pricing_source`, `pricing_updated_at`, and optional `pricing_notes` make later audits possible.
- `tool_support.openai_chat`, `tool_support.openai_responses`, and `tool_support.anthropic_messages` identify which request-shape capabilities have been tested for that upstream. Tool-bearing and structured-output requests only use compatible targets. Leave each capability absent until a direct upstream smoke and router-level smoke pass for that exact provider, model, dialect, and skin.
- `honors_max_tokens` defaults to `true`. Set it to `false` for an upstream target that accepts a request but ignores explicit caller caps such as `max_tokens: 1`, OpenAI Chat `max_completion_tokens: 1`, or Responses `max_output_tokens: 1`; the router then skips that target whenever the caller supplies a positive max-token field.
- Same-dialect OpenAI Chat and Responses passthrough strips caller-supplied provider `metadata` and sends `store:false` upstream. `force_store_false` applies to translated OpenAI Responses targets; set it when the upstream supports `store:false` and deployment policy requires disabling provider-side response storage outside passthrough.

OpenAI Chat tool clients such as Warp Agent call `/v1/chat/completions` with `tools`, `tool_choice`, and often `stream: true`. For these requests the router preserves the Chat Completions tool payload, selects only targets with explicit `tool_support.openai_chat`, and returns OpenAI Chat-compatible tool-call responses. Users can keep requesting an ordinary deployment-defined model group; they should not have to switch to a separate tools-only model for a coding-agent turn.

Structured-output callers use OpenAI Chat `response_format` or OpenAI Responses `text.format`. The router treats those fields as eligibility requirements and forwards the schema to the selected upstream. It does not perform application-level JSON Schema validation or guarantee that every provider accepts the same schema subset. Declare `structured_outputs` separately for each dialect that passes validation:

```yaml
tool_support:
  openai_chat: [tools, tool_choice, structured_outputs]
  openai_responses: [function, structured_outputs]
```

A target that supports tools is not automatically structured-output capable, and a target that supports Chat structured outputs is not automatically Responses structured-output capable. Requests that include both tools and structured-output fields require both capabilities on the same eligible target. If structured-output smokes fail after rollout, remove `structured_outputs` from that provider model or target override; if the whole target is unsafe, remove it from active `models.<group>.targets[]` and keep it catalog-only until validation passes.

Reasoning and thinking requests are also explicit eligibility requirements. OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, and Anthropic Messages `thinking` are routed only to targets whose provider model or target override declares compatible `reasoning` metadata. The router does not silently drop these fields to fit a cheaper target. If no compatible target remains, callers receive `502 no-eligible-target` with `reasoning` in the requirement details.

```yaml
reasoning:
  supported: true
  mode: opt_in        # opt_in or always_on
  control: effort_enum # effort_enum or token_budget
  supports_summaries: true
  rejects_max_tokens: true
```

Use `control: effort_enum` for upstreams that accept levels such as `low`, `medium`, and `high`. Use `control: token_budget` for Anthropic-style thinking budgets. Set compatibility flags such as `rejects_max_tokens`, `budget_must_be_less_than_max_tokens`, `rejects_temperature`, or `rejects_top_p` only when validated for the exact provider, model, dialect, and skin. As with tool and structured-output metadata, keep reasoning metadata absent until direct upstream and router-level reasoning smokes pass.

For a complete rollout guide with weighted-group examples, caller requests, `/v1/models` verification, and negative `no-eligible-target` tests, see [Reasoning Routing](./reasoning-routing).

## Provider Shared Traffic Shaping

Provider, provider-model, and target entries can declare optional `traffic_shape` blocks. These protect shared upstream capacity across all caller keys. They complement caller `rate` and `quota` policy: caller limits still run first, while provider shaping decides whether a selected upstream target can be used right now.

```yaml
providers:
  baseten:
    base_url: https://inference.example.com/v1
    dialect: openai-chat
    api_key_env: BASETEN_API_KEY
    traffic_shape:
      enabled: true
      request_start_per_sec: 10
      request_burst: 30
      input_tokens_per_sec: 500000
      input_token_burst: 1500000
      total_reserved_tokens_per_sec: 750000
      total_reserved_token_burst: 2000000
      upstream_429_backoff:
        enabled: true
        min_backoff_ms: 1000
        max_backoff_ms: 60000
        multiplier: 2.0
        honor_retry_after: true
      upstream_quota_backoff:
        enabled: true
        min_backoff_ms: 30000
        max_backoff_ms: 300000
        multiplier: 2.0
        honor_retry_after: true
    models:
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        traffic_shape:
          request_start_per_sec: 3
          request_burst: 8

models:
  default:
    strategy: weighted
    targets:
      - provider: baseten
        model_ref: gpt-oss-120b
        weight: 20
        traffic_shape:
          request_start_per_sec: 1
          request_burst: 2
```

Active provider, provider-model, and target scopes are cumulative. A request must pass every configured scope before the router calls upstream. If a lower-level block is omitted, no extra lower-level bucket is created; the provider bucket still applies when configured. Shaping admission happens after cache lookup and immediately before each upstream attempt, so cache hits do not consume shared provider capacity. If the selected target is temporarily throttled, the attempt path skips that target and tries the next fallback target.

`request_start_per_sec` limits upstream request starts. `input_tokens_per_sec` uses the router's request input-token estimate. `total_reserved_tokens_per_sec` uses estimated input plus the caller's output cap reservation. Burst fields are required for the matching rate field.

Adaptive backoff starts when an upstream attempt is classified as `upstream_rate_limited` or `upstream_quota_exhausted`. `Retry-After` is honored only when enabled and bounded by the configured `max_backoff_ms`; provider quota/billing exhaustion can use a separate longer `upstream_quota_backoff`. Shape decisions are logged as safe scalar `request_upstream_shape_events` rows and trace events without request bodies, upstream response bodies, provider keys, router tokens, or token hashes.

## Per-Group Weighted Routing

Weights are local to each model group. A target with weight `60` in one example group has no relationship to a target with weight `60` in another group. The group names in this snippet are examples; use names that match your deployment policy.

```yaml
models:
  vision:
    strategy: weighted
    targets:
      - { provider: xai, model_ref: grok-4-3, weight: 77 }
      - { provider: openai, model_ref: gpt-5.4, weight: 30 }
      - { provider: openrouter, model_ref: claude-sonnet-4-6, weight: 5 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 8 }

  default:
    strategy: weighted
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 51 }
      - { provider: minimax, model_ref: m3, weight: 27 }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 3 }
      - { provider: baseten, model_ref: glm-5-2, weight: 5 }
      - { provider: openrouter, model_ref: gemma-4-26b-a4b-it-nitro, weight: 2 }
      - { provider: crusoe, model_ref: glm-5-2, weight: 5 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 6 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }
      - { provider: baseten_anthropic, model_ref: gpt-oss-120b, tool_only: true, weight: 8 }

  big-coder:
    strategy: weighted
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 15 }
      - { provider: minimax, model_ref: m3, weight: 20 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 17 }
      - { provider: crusoe, model_ref: nemotron-3-nano-omni-reasoning-30b-a3b, weight: 11, input_modalities: [text] }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 2 }
      - { provider: baseten, model_ref: glm-5-2, weight: 6 }
      - { provider: fireworks, model_ref: gpt-oss-20b, weight: 11 }
      - { provider: fireworks, model_ref: glm-5p2, weight: 3 }
      - { provider: fireworks, model_ref: kimi-k2p7-code, weight: 5, input_modalities: [text] }
      - { provider: fireworks, model_ref: deepseek-v4-flash, weight: 5 }
      - { provider: fireworks, model_ref: qwen3p6-plus, weight: 5, input_modalities: [text] }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }
      - { provider: minimax, model_ref: m3, dialect: openai-responses, tool_only: true, weight: 18 }
      - { provider: fireworks_responses, model_ref: kimi-k2p7-code, dialect: openai-responses, tool_only: true, weight: 2 }
      - { provider: baseten_anthropic, model_ref: gpt-oss-120b, tool_only: true, weight: 8 }
      - { provider: kimi_anthropic, model_ref: kimi-k2.7-code, tool_only: true, weight: 4 }
      - { provider: minimax_anthropic, model_ref: m3, tool_only: true, weight: 7 }
      - { provider: openrouter_responses, model_ref: openrouter-xai-grok-4-3, tool_only: true, weight: 3 }
      - { provider: openrouter_responses, model_ref: openrouter-minimax-m3, tool_only: true, weight: 1 }
      - { provider: openrouter_anthropic, model_ref: gemma-4-26b-a4b-it-nitro, tool_only: true, weight: 2 }
      - { provider: openrouter_anthropic, model_ref: openrouter-xai-grok-4-3, tool_only: true, weight: 3 }
      - { provider: openrouter_anthropic, model_ref: openrouter-minimax-m3, tool_only: true, weight: 1 }
```

## Scripted Routing Options

Model groups can declare optional [model-group contracts](./model-group-contracts) for workload labels, supported API shapes, hard capability requirements, target validation quality floors, and operational thresholds. Contracts filter targets after caller authorization and ordinary request eligibility, before any strategy selects a target.

For choosing between `static`, `failover`, `weighted`, `dynamic_score`, `script`, `external`, and contract-backed policies, see [Customer-Controlled Routing](../routing/customer-controlled-routing).

Use `strategy: dynamic_score` when a group should adapt inside its own target list using bounded request-shape, prompt-feature, cost, observed-performance, reliability, and evaluation metadata signals. The strategy never scores targets from another model group; callers still request one allowed deployment-defined group. See [Dynamic Score Routing](./dynamic-score-routing) for configuration, diagnostics, rollout, and rollback guidance.

Use `strategy: script` when a model group should choose a target with TypeScript policy. Script paths are resolved relative to the config file.

```yaml
models:
  adaptive:
    strategy: script
    script: scripts/router.ts
    script_http:
      enabled: true
      allow_hosts: [routing-policy.internal.example]
      timeout_ms: 200
      max_response_bytes: 65536
      headers:
        Authorization: ${ROUTING_POLICY_AUTH_HEADER}
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

`script_http` is disabled unless explicitly enabled for that model group. `allow_hosts` is deployment-owned and must list exact hostnames the script may call through `router.fetchJSON`. HTTPS is required by default; plaintext HTTP is accepted only for loopback hosts or when `script_http.allow_http: true` is approved for a trusted non-local endpoint. Redirects are revalidated at each hop against the same scheme and exact-host allowlist. `headers` can carry deployment-owned policy-service authentication such as `Authorization: ${ROUTING_POLICY_AUTH_HEADER}` without exposing it to script code. `timeout_ms` is capped at `5000`; smaller timeouts and response-size limits are recommended because routing happens before the provider request is sent.

Relative imports such as `import { scorePrompt } from "./policy"` are bundled from the script directory at router startup. Package local helpers and any third-party dependencies with the deployment; the router does not install packages at runtime.

## External Routing Policy Service

Use `strategy: external` when target selection should be delegated to a standalone routing policy service. The router still enforces caller allow lists, request-shape eligibility, tool support, modalities, and max-token safety before calling the service.

```yaml
models:
  adaptive:
    strategy: external
    external_policy:
      url: https://routing-policy.internal.example/route
      allow_hosts: [routing-policy.internal.example]
      timeout_ms: 500
      max_response_bytes: 65536
      headers:
        Authorization: ${ROUTING_POLICY_AUTH_HEADER}
      on_error: fail_closed
      include_request: false
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

External policy egress uses HTTPS by default, exact-host allowlisting, and redirect revalidation on every hop. Plain HTTP is accepted only for loopback hosts or when `external_policy.allow_http: true` is explicitly approved for trusted internal infrastructure.

The policy service receives safe derived request context, safe caller metadata, safe contract metadata when configured, eligible targets, pricing metadata, tool support, validation metadata, and modality metadata. By default it does not receive prompt text, message bodies, image URLs/data, tool schemas, tool outputs, or `request.raw`; route on fields such as `context.textChars`, `context.estimatedTokens`, `context.imageCount`, and `context.toolCount`. Set `external_policy.include_request: true` only for a trusted service that is allowed to receive request content. With `pii_filter`, that opt-in request mirror is redacted before dispatch and placeholder mappings are not sent. It never receives raw router tokens, token hashes, or provider API keys. See [External Routing Policy Service](./external-routing-policy) for the tested demo service and response schema.

## Caller Tokens And Allow Lists

```yaml
users:
  - id: example-standard
    name: Example Standard Caller
    type: service_account
    status: active
  - id: example-coding
    name: Example Coding Caller
    type: service_account
    status: active
  - id: metrics-admin
    name: Metrics Admin
    type: service_account
    status: active
  - id: content-admin
    name: Content Admin
    type: service_account
    status: active

projects:
  - id: example-project
    name: Example Project
    status: active
  - id: observability
    name: Observability
    status: active
  - id: compliance
    name: Compliance
    status: active

project_memberships:
  - user_id: example-standard
    project: example-project
    role: developer
    status: active
  - user_id: example-coding
    project: example-project
    role: developer
    status: active
  - user_id: metrics-admin
    project: observability
    role: operator
    status: active
  - user_id: content-admin
    project: compliance
    role: operator
    status: active

callers:
  - id: example-standard-prod
    owner_user: example-standard
    project: example-project
    environment: prod
    status: active
    token_sha256: SHA256_HEX_OF_STANDARD_ROUTER_TOKEN
    token_id: rtr_metrum_example-standard_example-project_prod_k20260614
    metrics_admin: false
    content_admin: false
    allow: [default, fast, small]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }

  - id: example-coding-prod
    owner_user: example-coding
    project: example-project
    environment: prod
    status: active
    token_sha256: SHA256_HEX_OF_CODING_ROUTER_TOKEN
    token_id: rtr_metrum_example-coding_example-project_prod_k20260614
    metrics_admin: false
    content_admin: false
    allow: [default, fast, small, medium, high, big-coder]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }

  - id: example-metrics-prod
    owner_user: metrics-admin
    project: observability
    environment: prod
    status: active
    token_sha256: SHA256_HEX_OF_METRICS_ROUTER_TOKEN
    token_id: rtr_metrum_metrics-admin_observability_prod_k20260614
    metrics_admin: true
    content_admin: false
    allow: []
    rate: { rpm: 60, tpm: 0, concurrent: 2 }

  - id: example-content-admin-prod
    owner_user: content-admin
    project: compliance
    environment: prod
    status: active
    token_sha256: SHA256_HEX_OF_CONTENT_ADMIN_ROUTER_TOKEN
    token_id: rtr_metrum_content-admin_compliance_prod_k20260614
    metrics_admin: false
    content_admin: true
    allow: []
    rate: { rpm: 60, tpm: 0, concurrent: 2 }
```

Users and projects are explicit account records, not values inferred from API-key names. Project memberships bind a user to a project with a role and status. A user or project can have multiple caller keys for different environments, clients, rotations, or workload tiers, and reports keep those keys distinct with public `token_id` and caller ID fields.

User ids, project ids, membership pairs, caller `id`, `token_sha256`, and non-empty `token_id` values must be unique after normalization. Each key must reference an active `owner_user`, active `project`, and active project membership. Legacy `callers[].user` is still accepted as a deprecated alias for `owner_user`; if both fields are present they must normalize to the same id. Token hashes are compared case-insensitively during config validation, and duplicate-hash validation errors identify the caller IDs without printing hash values. User, project, and membership statuses support `active`, `disabled`, `suspended`, `removed`, and `archived`; caller key statuses also support `expired` and `rotated`.

Disallowed model requests return `403 model-not-allowed` before any upstream provider key is used. Inactive keys return safe status-specific errors after token match, such as `403 key-disabled`, `403 key-suspended`, `403 key-expired`, or `403 key-rotated`; inactive users, projects, or memberships are rejected by config validation before startup. `/metrics` is separate from model access: it returns global operational telemetry only for caller subjects authorized for `metrics` `read`; existing callers with `metrics_admin: true` remain compatible through generated Casbin grants. Ordinary callers receive `403 metrics-forbidden`. Content-capture delete and purge operations require `content:capture` `delete`/`purge` authorization; delete-by-request is scoped to the captured row's caller project/environment domain. Existing callers with `content_admin: true` remain compatible through generated Casbin grants for their own domain. Metrics-admin tokens do not grant content-admin access.

Reports can group by caller ID, owner user, project, environment, public token ID/key label, client, requested model group, selected provider/model/dialect, source IP when stored, status, quota/rate-limit bucket, and cost. `/v1/models` is the caller-facing source of truth for the allowed model groups attached to the presented key.

## Caller Traffic Shaping

Traffic shaping smooths how quickly one caller can start requests and reserve estimated token capacity. It complements hard controls instead of replacing them:

- `rpm` limits requests over a rolling minute.
- `tpm` limits estimated input plus reserved output tokens over a rolling minute.
- `concurrent` limits in-flight requests.
- daily, monthly, and lifetime quotas limit total use.
- traffic shaping limits short-burst request starts, input-token throughput, output-reservation throughput, and total reserved-token throughput.

Shaping runs after authentication, model-group allow-list checks, hard rate/quota checks, and request token estimation, but before upstream calls. Request-start shaping applies to all admitted requests, including cache hits. Input/output/total token-reservation shaping applies only to cache misses that would otherwise call an upstream. Bucket state is in memory and resets on router restart.

Server defaults are disabled unless `server.traffic_shape.enabled: true`. Caller-level `callers[].traffic_shape` overrides the server default; set `enabled: false` on a caller to opt out of an enabled default. Project/user inheritance is not part of this release.

Disabled inherited default:

```yaml
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

Conservative coding-agent caller:

```yaml
callers:
  - id: example-coding-prod
    owner_user: example-coding
    project: example-project
    environment: prod
    allow: [default, fast, small, medium, high, big-coder]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }
    traffic_shape:
      enabled: true
      request_start_per_sec: 2.0
      request_burst: 8
      input_tokens_per_sec: 50000
      input_token_burst: 200000
      output_reservation_tokens_per_sec: 30000
      output_reservation_token_burst: 120000
      total_reserved_tokens_per_sec: 70000
      total_reserved_token_burst: 250000
      queue:
        enabled: false
        max_wait_ms: 0
        max_depth: 0
```

Large-context Cursor or Codex caller:

```yaml
callers:
  - id: example-large-context-prod
    owner_user: example-coding
    project: example-project
    environment: prod
    allow: [default, high, big-coder]
    rate: { rpm: 240, tpm: 5000000, concurrent: 16 }
    traffic_shape:
      enabled: true
      request_start_per_sec: 2.0
      request_burst: 8
      input_tokens_per_sec: 75000
      input_token_burst: 300000
      output_reservation_tokens_per_sec: 60000
      output_reservation_token_burst: 250000
      total_reserved_tokens_per_sec: 120000
      total_reserved_token_burst: 500000
      queue:
        enabled: false
        max_wait_ms: 0
        max_depth: 0
```

Bounded queue example:

```yaml
callers:
  - id: example-bounded-queue-prod
    traffic_shape:
      enabled: true
      request_start_per_sec: 1.0
      request_burst: 2
      input_tokens_per_sec: 25000
      input_token_burst: 100000
      total_reserved_tokens_per_sec: 40000
      total_reserved_token_burst: 160000
      queue:
        enabled: true
        max_wait_ms: 250
        max_depth: 4
```

Default behavior is reject, not queue. Shaping rejections return `429 traffic-shaped` with `Retry-After` when a retry time is known and a safe `bucket` such as `caller.input_tokens_per_sec`. Queued requests count against the per-caller queue depth while waiting and count against `concurrent` only after admission. Client disconnects cancel queued admission.

Roll out by leaving server defaults disabled, enabling one canary caller, running a controlled burst test, then comparing `traffic_shape_*` usage fields, upstream provider 429 attempts, and user latency before broadening the policy. Roll back by setting the caller or server default `traffic_shape.enabled: false` and restarting or reloading through the normal deployment process.

## Cache And Usage Store

```yaml
server:
  # Optional fallback when a compatible API request omits model.
  # The value must be one of this deployment's configured model groups.
  default_model_group: default
  cache:
    enabled: true
    max_bytes: 134217728
    default_ttl: 15m
  logging:
    path: /app/logs/requests.jsonl
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}
  decision_telemetry:
    enabled: false
    max_candidates: 64
    max_filter_reasons: 256
    record_candidates: true
    record_cache_reasons: true
  upstream:
    timeout_ms: 600000
    default_attempt_timeout_ms: 0
  diagnostics:
    enabled: true
    retention_days: 30
    store_sanitized_upstream_errors: false
    max_error_bytes: 2048
  admin_auth:
    basic:
      enabled: false
      realm: GenAI Smart Router Admin
      allow_insecure_http: false
      trusted_proxy_cidrs: []
      users: []
    oidc:
      enabled: false
      issuer_url: https://accounts.google.com
      client_id_env: GOOGLE_OIDC_CLIENT_ID
      client_secret_env: GOOGLE_OIDC_CLIENT_SECRET
      redirect_url: https://router.example.com/admin/auth/callback
      allowed_domains:
        - example.com
      groups_claim: groups
      email_claim: email
      subject_claim: email
      domain: example/prod
    sessions:
      cookie_name: smart_router_admin_session
      ttl: 8h
      secure_cookies: true
      same_site: strict
  content_capture:
    enabled: false
    retention_days: 30
    capture_request: false
    capture_response: false
    capture_tool_calls: false
    capture_images: false
    capture_upstream_errors: false
    capture_headers_allowlist: []
    redact_before_storage: true
    redaction_patterns: []
    max_capture_bytes: 65536
    encryption:
      enabled: false
      kms_key_id: ""
  retention:
    enabled: false
    dry_run: true
    default_batch_size: 500
    classes:
      - data_class: usage_diagnostics
        enabled: true
        retention_days: 30
        batch_size: 500
      - data_class: decision_telemetry
        enabled: true
        retention_days: 30
        batch_size: 500
      - data_class: security_access_events
        enabled: true
        retention_days: 90
        batch_size: 500
      - data_class: content_capture
        enabled: true
        retention_days: 30
        batch_size: 500
      - data_class: usage_detail
        enabled: false
        retention_days: 365
        batch_size: 500
        require_finalized_rollup: true
```

`server.decision_telemetry` is optional and disabled by default. When enabled, the router writes normalized scalar rows keyed by request ID for request-shape features, bounded target candidates, safe target-filter reason buckets, selected routing decisions, routing signals, score/ranking terms, script/external policy executions including fail-closed pre-selection errors, fallback transitions after upstream failures, and cache reason buckets. Request usage rows also store non-secret router/config/model-group/policy/pricing fingerprints for reproducibility after config or pricing changes. It is intended for operator explainability, admin request drilldown, and usage-report summaries; caller responses keep the same behavior.

Decision telemetry does not store prompt text, image bytes or URLs, tool schemas, tool outputs, bearer tokens, provider keys, token hashes, policy request/response JSON, full config, or raw routing-script request mirrors. Fingerprints are hashes of redacted canonical routing metadata and exclude provider keys, token hashes, raw URLs, policy headers, script paths, and full config contents. Use `max_candidates` and `max_filter_reasons` to bound per-request row volume, and set `record_candidates` or `record_cache_reasons` to `false` when a deployment wants only the lighter routing-decision rows.

The cache is intended for eligible deterministic unary responses. Tool-bearing agent requests bypass cache because tool output can depend on live shell and filesystem state.

Image-bearing requests also bypass response caching. Usage logs and the usage database include `input_has_image`, `input_image_count`, image-token counts when the upstream reports them, calculated VLM costs, and upstream-reported billed costs when available.

Model groups can enable `pii_filter` to redact configured text expressions before routing policy, cache keys, and upstream calls. Requests that exceed `max_replacements_per_request` fail closed with `pii-filter-blocked` before any upstream call. Usage rows record only safe scalar PII-filter metadata such as whether filtering applied, mode, replacement count, and matched-rule count; raw matched values and placeholder mappings are not persisted or passed to policy contexts by default. External policy services receive only derived safe context unless `external_policy.include_request: true` is explicitly enabled, in which case the request mirror is redacted first. See [PII Filtering](./pii-filtering).

Diagnostics add relational child rows for troubleshooting: `request_attempts`, `request_trace_events`, `request_traffic_shape_events`, `request_upstream_shape_events`, and `request_errors`. Use the `X-Request-Id` header or the `request_id` in an error body to join these rows with `request_usage`. Diagnostic rows store provider/model/status/timing/error-class data plus safe caller and upstream shaping bucket decisions; they do not store raw prompts, images, bearer tokens, provider keys, token hashes, full upstream headers, or raw upstream response bodies. `store_sanitized_upstream_errors` can keep bounded sanitized error context, but it is not content capture: arbitrary upstream bodies are collapsed to a redaction marker, and prompt-like fields, nested upstream bodies, and secret-shaped values are redacted before JSONL or usage DB persistence.

Browser-admin authentication is configured under `server.admin_auth.basic` and `server.admin_auth.oidc`; both remain disabled unless an operator explicitly enables them. Basic Auth establishes subjects such as `basic:admin`. OIDC sessions establish subjects such as `user:alice@example.com` when `subject_claim: email`. Browser-admin identity is separate from router caller tokens for `/v1/*` and from caller-token metrics access for `/metrics`. Keep password hashes and OIDC client secrets in environment variables, configure trusted proxy CIDRs for forwarded HTTPS state, and use secure session cookies in production. See [Admin Authentication](./admin-authentication) for bcrypt hash setup, TLS requirements, and admin auth validation endpoints. See [Admin Authorization](./admin-authorization) for Casbin metrics, content-capture maintenance, and report policy.

Governed content capture is separate from diagnostics and remains disabled unless `server.content_capture.enabled: true` and at least one scope is enabled. Captured rows live in `request_content_captures`, allowlisted headers in `request_content_headers`, and delete/purge audit events in `request_content_audit_events`. Rows are keyed by `request_id` for joins to usage metadata. Built-in secret redaction and configured `redaction_patterns` run before storage; `redact_before_storage: false` is rejected. For `redact_and_restore` groups, response capture stores the pre-restore placeholder response, while the original caller can still receive restored response text. Header capture is allowlist-only and rejects authorization, API-key, token, secret, cookie, and key-like header names. The current foundation supports retention purge and delete-by-request maintenance; KMS/encryption-at-rest and content export/read APIs are follow-up work, and `encryption.enabled: true` is rejected until implemented.

Deployments can keep capture disabled globally and enable a scoped override on a specific `callers[]` entry or `models.<group>.content_capture` block for a governed workload. Each enabled block must name at least one capture scope.

Commercial retention policy is configured under `server.retention` and is disabled by default. Defaults are conservative: `dry_run` defaults to true, batch sizes are explicit scalar values, and `usage_detail` is disabled unless an operator enables it with `require_finalized_rollup: true`. Status jobs record policy versions/rules, legal-hold rows, retention jobs, and per-table counts without deleting rows. Supported data classes are `usage_diagnostics`, `decision_telemetry`, `security_access_events`, `content_capture`, and `usage_detail`. Legal holds match by `data_class`, optional `request_id`, and timestamp range.

When a reviewed deployment sets `dry_run: false`, `router-usage-report --retention-run` deletes at most one configured batch for `usage_diagnostics` and `usage_detail` tables. Other data classes are counted and recorded as blocked until a later retention slice implements their generic purge path. `usage_detail` deletion is blocked until finalized daily rollups continuously cover the candidate window, and finalized rollups remain immutable. Archives, scheduler, and full legal-hold/admin write workflows remain future slices.

Content-capture maintenance endpoints:

These endpoints require caller-token subjects authorized for `content:capture`; delete-by-request uses action `delete`, and retention purge uses action `purge`. Delete-by-request also checks authorization in the captured row's caller project/environment domain before removing rows. Existing `content_admin: true` callers receive compatible grants for their own domain at startup.

```bash
curl -X DELETE "$SMART_ROUTER_BASE_URL/v1/content-captures/<request_id>" \
  -H "Authorization: Bearer $CONTENT_ADMIN_ROUTER_TOKEN"

curl -X POST "$SMART_ROUTER_BASE_URL/v1/content-captures/purge-expired" \
  -H "Authorization: Bearer $CONTENT_ADMIN_ROUTER_TOKEN"
```

Per-attempt upstream timeouts can be configured globally, per model group, or per target. `0` disables the per-attempt cap while the global `server.upstream.timeout_ms` still bounds the HTTP client. If every eligible attempt fails, exhausted upstream timeouts return `504 upstream-timeout`, provider rate limits return `503 upstream-rate-limited`, provider balance/credit/quota/billing exhaustion returns `503 upstream-quota-exhausted`, provider/model/target shared-capacity shaping returns `503 upstream-capacity-throttled`, and other exhausted upstream failures return `502 upstream-failed`. Fallback targets are attempted before the router returns one of these terminal errors.

Cataloged vision models are not automatically active routes. Keep a vision candidate catalog-only until it passes the exact direct upstream and router-level image smoke for the intended task and API dialect.

See [Image Analysis And VLM Routing](./image-analysis-vlm) for full OpenAI Chat, OpenAI Responses, Anthropic Messages, Codex CLI, and Claude Code image examples.
