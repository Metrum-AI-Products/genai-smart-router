---
title: Router Configuration
---

# Router Configuration

The router is configured with YAML plus environment-loaded provider keys. Customers normally keep provider credentials in the deployment environment or an `env.json` file outside source control.

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

Catalog entries should carry cost and capability metadata:

- `input_price_per_million_usd` and `output_price_per_million_usd` are the values used to calculate per-request cost at the time the request runs.
- `input_modalities` and `output_modalities` describe tested model I/O such as `text`, `image`, or `video`. Requests that include image content are automatically filtered to targets with `image` in `input_modalities`; text-only targets are skipped.
- `image_input_price_per_million_tokens_usd` and `image_input_price_per_image_usd` are optional VLM pricing fields. Use them only when the provider or internal chargeback model bills image input differently from ordinary input tokens. If the provider returns billed cost in usage metadata, the router logs that upstream-reported cost separately from the calculated cost.
- `pricing_source`, `pricing_updated_at`, and optional `pricing_notes` make later audits possible.
- `tool_support.openai_chat`, `tool_support.openai_responses`, and `tool_support.anthropic_messages` identify which tool protocol has been tested for that upstream. Tool-bearing requests only use compatible tool targets. Leave the field absent until a direct upstream smoke and router-level tool smoke pass.

## Per-Group Weighted Routing

Weights are local to each model group. A target with weight `60` in `default` has no relationship to a target with weight `60` in `big-coder`.

```yaml
models:
  default:
    strategy: weighted
    targets:
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, weight: 60 }
      - { provider: minimax, model_ref: m3, weight: 30 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 10 }

  big-coder:
    strategy: weighted
    targets:
      - { provider: minimax, model_ref: m3, weight: 50 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 30 }
      - { provider: openrouter, model_ref: deepseek-v4-flash-nitro, weight: 20 }
      - { provider: vllm_internal, model_ref: qwen3-coder-tools, weight: 10 }
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
    allow: [default, fast, small]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }

  - id: example-coding-prod
    user: example-coding
    project: example-project
    environment: prod
    token_sha256: SHA256_HEX_OF_ROUTER_TOKEN
    token_id: rtr_metrum_example-coding_example-project_prod_k20260614
    allow: [default, fast, small, medium, high, big-coder]
    rate: { rpm: 120, tpm: 200000, concurrent: 8 }
```

Disallowed model requests return `403 model-not-allowed` before any upstream provider key is used.

## Cache And Usage Store

```yaml
server:
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
```

The cache is intended for eligible deterministic unary responses. Tool-bearing agent requests bypass cache because tool output can depend on live shell and filesystem state.

Image-bearing requests also bypass response caching. Usage logs and the usage database include `input_has_image`, `input_image_count`, image-token counts when the upstream reports them, calculated VLM costs, and upstream-reported billed costs when available.

Cataloged vision models are not automatically active routes. Keep a vision candidate catalog-only until it passes the exact direct upstream and router-level image smoke for the intended task and API dialect.

See [Image Analysis And VLM Routing](./image-analysis-vlm) for full OpenAI Chat, OpenAI Responses, Anthropic Messages, Codex CLI, and Claude Code image examples.
