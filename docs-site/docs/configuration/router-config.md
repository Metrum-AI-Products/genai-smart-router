---
title: Router Configuration
---

# Router Configuration

The router is configured with YAML plus environment-loaded provider keys. Customers normally keep provider credentials in the deployment environment or a protected `env.json` file outside the shipped binary package.

Model group names are deployment-defined. Names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` may appear in examples because they are used by a reference or hosted deployment; the product does not require those names.

<div class="contactBanner">
  <p>Metrum can help design a production routing policy. Contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

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
      deepseek-v4-flash-nitro:
        model: deepseek/deepseek-v4-flash:nitro
        tier: balanced
        input_price_per_million_usd: 0.09
        output_price_per_million_usd: 0.18
        pricing_source: https://openrouter.ai/api/v1/models
        pricing_updated_at: "2026-06-17"
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
      qwen3-7-plus-nitro:
        model: qwen/qwen3.7-plus:nitro
        tier: vision
        input_price_per_million_usd: 0.32
        output_price_per_million_usd: 1.28
        input_modalities: [text, image]
        output_modalities: [text]
        honors_max_tokens: false
        pricing_source: https://openrouter.ai/api/v1/models
        pricing_updated_at: "2026-06-17"
        pricing_notes: receipt image smoke returned Rite Aid on 2026-06-17; capped requests skip this target until max-token behavior is revalidated
      qwen3-6-flash-nitro:
        model: qwen/qwen3.6-flash:nitro
        tier: vision
        input_price_per_million_usd: 0.1875
        output_price_per_million_usd: 1.125
        input_modalities: [text, image, video]
        output_modalities: [text]
        pricing_source: https://openrouter.ai/qwen/qwen3.6-flash/providers
        pricing_updated_at: "2026-06-17"
        pricing_notes: image-capable; one receipt smoke returned a wrong merchant, so use conservative weight for general VLM routing and separate OCR-specific gates

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

External OpenAI-compatible providers follow the same shape. For example, Baseten Model APIs use `base_url: https://inference.baseten.co/v1` with `dialect: openai-chat`; callers still request a deployment-defined router model group, not the upstream Baseten model ID. The router injects `BASETEN_API_KEY` only when that target is selected.

Catalog entries should carry cost and capability metadata:

- `input_price_per_million_usd` and `output_price_per_million_usd` are the values used to calculate per-request cost at the time the request runs.
- `input_modalities` and `output_modalities` describe tested model I/O such as `text`, `image`, or `video`. Requests that include image content are automatically filtered to targets with `image` in `input_modalities`; text-only targets are skipped.
- `image_input_price_per_million_tokens_usd` and `image_input_price_per_image_usd` are optional VLM pricing fields. Use them only when the provider or internal chargeback model bills image input differently from ordinary input tokens. If the provider returns billed cost in usage metadata, the router logs that upstream-reported cost separately from the calculated cost.
- `pricing_source`, `pricing_updated_at`, and optional `pricing_notes` make later audits possible.
- `tool_support.openai_chat`, `tool_support.openai_responses`, and `tool_support.anthropic_messages` identify which tool protocol has been tested for that upstream. Tool-bearing requests only use compatible tool targets. Leave the field absent until a direct upstream smoke and router-level tool smoke pass.
- `honors_max_tokens` defaults to `true`. Set it to `false` for an upstream target that accepts a request but ignores explicit caller caps such as `max_tokens: 1` or `max_output_tokens: 1`; the router then skips that target whenever the caller supplies a positive max-token field.

OpenAI Chat tool clients such as Warp Agent call `/v1/chat/completions` with `tools`, `tool_choice`, and often `stream: true`. For these requests the router preserves the Chat Completions tool payload, selects only targets with explicit `tool_support.openai_chat`, and returns OpenAI Chat-compatible tool-call responses. Users can keep requesting an ordinary deployment-defined model group; they should not have to switch to a separate tools-only model for a coding-agent turn.

## Per-Group Weighted Routing

Weights are local to each model group. A target with weight `60` in one example group has no relationship to a target with weight `60` in another group. The group names in this snippet are examples; use names that match your deployment policy.

```yaml
models:
  vision:
    strategy: weighted
    targets:
      - { provider: xai, model_ref: grok-4-3, weight: 45 }
      - { provider: openrouter, model_ref: qwen3-7-plus-nitro, weight: 20 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 8 }

  default:
    strategy: weighted
    targets:
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, weight: 46 }
      - { provider: minimax, model_ref: m3, weight: 27 }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 3 }
      - { provider: baseten, model_ref: glm-5-2, weight: 5 }
      - { provider: openrouter, model_ref: gemma-4-26b-a4b-it-nitro, weight: 7 }
      - { provider: openrouter, model_ref: qwen3-6-flash-nitro, weight: 5 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 6 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }

  big-coder:
    strategy: weighted
    targets:
      - { provider: minimax, model_ref: m3, weight: 38 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 28 }
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, weight: 18 }
      - { provider: openrouter, model_ref: qwen3-6-flash-nitro, weight: 5 }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 3 }
      - { provider: baseten, model_ref: glm-5-2, weight: 7 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }
```

## Scripted Routing Options

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
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

`script_http` is disabled unless explicitly enabled for that model group. `allow_hosts` is deployment-owned and must list exact hostnames the script may call through `router.fetchJSON`. `headers` can carry deployment-owned policy-service authentication such as `Authorization: ${ROUTING_POLICY_AUTH_HEADER}` without exposing it to script code. `timeout_ms` is capped at `5000`; smaller timeouts and response-size limits are recommended because routing happens before the provider request is sent.

Relative imports such as `import { scorePrompt } from "./policy"` are bundled from the script directory at router startup. Package local helpers and any third-party dependencies with the deployment; the router does not install packages at runtime.

## Caller Tokens And Allow Lists

```yaml
callers:
  - id: example-standard-prod
    user: example-standard
    project: example-project
    environment: prod
    token_sha256: SHA256_HEX_OF_ROUTER_TOKEN
    token_id: rtr_metrum_example-standard_example-project_prod_k20260614
    metrics_admin: false
    allow: [default, fast, small]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }

  - id: example-coding-prod
    user: example-coding
    project: example-project
    environment: prod
    token_sha256: SHA256_HEX_OF_ROUTER_TOKEN
    token_id: rtr_metrum_example-coding_example-project_prod_k20260614
    metrics_admin: false
    allow: [default, fast, small, medium, high, big-coder]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }

  - id: example-metrics-prod
    user: metrics-admin
    project: observability
    environment: prod
    token_sha256: SHA256_HEX_OF_ROUTER_TOKEN
    token_id: rtr_metrum_metrics-admin_observability_prod_k20260614
    metrics_admin: true
    allow: []
    rate: { rpm: 60, tpm: 0, concurrent: 2 }
```

Disallowed model requests return `403 model-not-allowed` before any upstream provider key is used. `/metrics` is separate from model access: it returns global operational telemetry only for callers with `metrics_admin: true`; ordinary callers receive `403 metrics-forbidden`.

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
  upstream:
    timeout_ms: 600000
    default_attempt_timeout_ms: 0
  diagnostics:
    enabled: true
    retention_days: 30
    store_sanitized_upstream_errors: false
    max_error_bytes: 2048
```

The cache is intended for eligible deterministic unary responses. Tool-bearing agent requests bypass cache because tool output can depend on live shell and filesystem state.

Image-bearing requests also bypass response caching. Usage logs and the usage database include `input_has_image`, `input_image_count`, image-token counts when the upstream reports them, calculated VLM costs, and upstream-reported billed costs when available.

Model groups can enable `pii_filter` to redact configured text expressions before upstream calls. Usage rows record only safe scalar PII-filter metadata such as whether filtering applied, mode, replacement count, and matched-rule count; raw matched values and placeholder mappings are not persisted by default. See [PII Filtering](./pii-filtering).

Diagnostics add relational child rows for troubleshooting: `request_attempts`, `request_trace_events`, and `request_errors`. Use the `X-Request-Id` header or the `request_id` in an error body to join these rows with `request_usage`. Diagnostic rows store provider/model/status/timing/error-class data; they do not store raw prompts, images, bearer tokens, provider keys, or full upstream headers.

Per-attempt upstream timeouts can be configured globally, per model group, or per target. `0` disables the per-attempt cap while the global `server.upstream.timeout_ms` still bounds the HTTP client. Exhausted upstream timeouts return `504 upstream-timeout`; exhausted provider rate limits return `503 upstream-rate-limited`; other exhausted upstream failures return `502 upstream-failed`.

Cataloged vision models are not automatically active routes. Keep a vision candidate catalog-only until it passes the exact direct upstream and router-level image smoke for the intended task and API dialect.

See [Image Analysis And VLM Routing](./image-analysis-vlm) for full OpenAI Chat, OpenAI Responses, Anthropic Messages, Codex CLI, and Claude Code image examples.
