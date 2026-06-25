---
title: Router Configuration
---

# Router Configuration

The router is configured with YAML plus environment-loaded provider keys. Customers normally keep provider credentials in the deployment environment, a secret manager, or a protected runtime `env.json` file. The shipped `env.example.json` is only a shape template with empty provider-key placeholders.

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
        pricing_notes: OpenAI Chat text, streaming, max_tokens=1, auto tool, forced tool_choice, structured-output, and combined tool plus structured-output smokes passed on 2026-06-24; use for ordinary OpenAI Chat text and OpenAI Chat tool routing only after workload validation, and keep out of OpenAI Responses or Anthropic Messages routes unless those skins pass separately
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]

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

Crusoe support in these examples is OpenAI Chat only. A deployment can catalog Crusoe models, expose dedicated Crusoe smoke groups, place a validated Crusoe Gemma target in an ordinary-text weighted group, or add a separate OpenAI Chat `tool_only` target. Do not treat a Crusoe OpenAI Chat pass as proof of OpenAI Responses, Anthropic Messages, image, video, or audio support.

Catalog entries should carry cost and capability metadata:

- `input_price_per_million_usd` and `output_price_per_million_usd` are the values used to calculate per-request cost at the time the request runs.
- `input_modalities` and `output_modalities` describe tested model I/O such as `text`, `image`, or `video`. Requests that include image content are automatically filtered to targets with `image` in `input_modalities`; text-only targets are skipped.
- `image_input_price_per_million_tokens_usd` and `image_input_price_per_image_usd` are optional VLM pricing fields. Use them only when the provider or internal chargeback model bills image input differently from ordinary input tokens. If the provider returns billed cost in usage metadata, the router logs that upstream-reported cost separately from the calculated cost.
- `pricing_source`, `pricing_updated_at`, and optional `pricing_notes` make later audits possible.
- `tool_support.openai_chat`, `tool_support.openai_responses`, and `tool_support.anthropic_messages` identify which request-shape capabilities have been tested for that upstream. Tool-bearing and structured-output requests only use compatible targets. Leave each capability absent until a direct upstream smoke and router-level smoke pass for that exact provider, model, dialect, and skin.
- `honors_max_tokens` defaults to `true`. Set it to `false` for an upstream target that accepts a request but ignores explicit caller caps such as `max_tokens: 1`, OpenAI Chat `max_completion_tokens: 1`, or Responses `max_output_tokens: 1`; the router then skips that target whenever the caller supplies a positive max-token field.

OpenAI Chat tool clients such as Warp Agent call `/v1/chat/completions` with `tools`, `tool_choice`, and often `stream: true`. For these requests the router preserves the Chat Completions tool payload, selects only targets with explicit `tool_support.openai_chat`, and returns OpenAI Chat-compatible tool-call responses. Users can keep requesting an ordinary deployment-defined model group; they should not have to switch to a separate tools-only model for a coding-agent turn.

Structured-output callers use OpenAI Chat `response_format` or OpenAI Responses `text.format`. The router treats those fields as eligibility requirements and forwards the schema to the selected upstream. It does not perform application-level JSON Schema validation or guarantee that every provider accepts the same schema subset. Declare `structured_outputs` separately for each dialect that passes validation:

```yaml
tool_support:
  openai_chat: [tools, tool_choice, structured_outputs]
  openai_responses: [function, structured_outputs]
```

A target that supports tools is not automatically structured-output capable, and a target that supports Chat structured outputs is not automatically Responses structured-output capable. Requests that include both tools and structured-output fields require both capabilities on the same eligible target. If structured-output smokes fail after rollout, remove `structured_outputs` from that provider model or target override; if the whole target is unsafe, remove it from active `models.<group>.targets[]` and keep it catalog-only until validation passes.

## Per-Group Weighted Routing

Weights are local to each model group. A target with weight `60` in one example group has no relationship to a target with weight `60` in another group. The group names in this snippet are examples; use names that match your deployment policy.

```yaml
models:
  vision:
    strategy: weighted
    targets:
      - { provider: xai, model_ref: grok-4-3, weight: 77 }
      - { provider: openrouter, model_ref: claude-sonnet-4-6, weight: 15 }
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
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 18 }
      - { provider: minimax, model_ref: m3, weight: 30 }
      - { provider: kimi, model_ref: kimi-k2-7-code, weight: 23 }
      - { provider: crusoe, model_ref: gemma-4-31b-it, weight: 20 }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 2 }
      - { provider: baseten, model_ref: glm-5-2, weight: 6 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }
      - { provider: baseten_anthropic, model_ref: gpt-oss-120b, tool_only: true, weight: 8 }
      - { provider: crusoe, model_ref: gemma-4-31b-it, tool_only: true, weight: 2 }
```

## Scripted Routing Options

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
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, tier: cheap, weight: 70 }
      - { provider: minimax, model_ref: m3, tier: heavy, weight: 30 }
```

External policy egress uses HTTPS by default, exact-host allowlisting, and redirect revalidation on every hop. Plain HTTP is accepted only for loopback hosts or when `external_policy.allow_http: true` is explicitly approved for trusted internal infrastructure.

The policy service receives normalized request context, safe caller metadata, eligible targets, pricing metadata, tool support, and modality metadata. For groups with `pii_filter`, that context is built from the redacted request object, including `request.raw`; placeholder mappings are not sent. It never receives raw router tokens, token hashes, or provider API keys. See [External Routing Policy Service](./external-routing-policy) for the tested demo service and response schema.

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

User ids, project ids, membership pairs, caller `id`, `token_sha256`, and non-empty `token_id` values must be unique after normalization. Each key must reference an active `owner_user`, active `project`, and active project membership. Legacy `callers[].user` is still accepted as a deprecated alias for `owner_user`; if both fields are present they must normalize to the same id. Token hashes are compared case-insensitively during config validation, and duplicate-hash validation errors identify the caller IDs without printing hash values. User, project, and membership statuses support `active`, `disabled`, `suspended`, `removed`, and `archived`; caller key statuses also support `expired` and `rotated`.

Disallowed model requests return `403 model-not-allowed` before any upstream provider key is used. Inactive keys return safe status-specific errors after token match, such as `403 key-disabled`, `403 key-suspended`, `403 key-expired`, or `403 key-rotated`; inactive users, projects, or memberships are rejected by config validation before startup. `/metrics` is separate from model access: it returns global operational telemetry only for callers with `metrics_admin: true` that are also authorized for `metrics` `read`; existing callers with `metrics_admin: true` remain compatible through generated Casbin grants. Ordinary callers receive `403 metrics-forbidden`. Content-capture delete and purge operations require `content:capture` `delete`/`purge` authorization; existing callers with `content_admin: true` remain compatible through generated Casbin grants. Metrics-admin tokens do not grant content-admin access.

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
```

The cache is intended for eligible deterministic unary responses. Tool-bearing agent requests bypass cache because tool output can depend on live shell and filesystem state.

Image-bearing requests also bypass response caching. Usage logs and the usage database include `input_has_image`, `input_image_count`, image-token counts when the upstream reports them, calculated VLM costs, and upstream-reported billed costs when available.

Model groups can enable `pii_filter` to redact configured text expressions before routing policy, cache keys, and upstream calls. Usage rows record only safe scalar PII-filter metadata such as whether filtering applied, mode, replacement count, and matched-rule count; raw matched values and placeholder mappings are not persisted or passed to policy contexts by default. See [PII Filtering](./pii-filtering).

Diagnostics add relational child rows for troubleshooting: `request_attempts`, `request_trace_events`, and `request_errors`. Use the `X-Request-Id` header or the `request_id` in an error body to join these rows with `request_usage`. Diagnostic rows store provider/model/status/timing/error-class data; they do not store raw prompts, images, bearer tokens, provider keys, token hashes, full upstream headers, or raw upstream response bodies. `store_sanitized_upstream_errors` can keep bounded sanitized error context, but it is not content capture and still redacts prompt-like fields, nested upstream bodies, and secret-shaped values before JSONL or usage DB persistence.

Browser-admin authentication is configured under `server.admin_auth.basic` and `server.admin_auth.oidc`; both remain disabled unless an operator explicitly enables them. Basic Auth establishes subjects such as `basic:admin`. OIDC sessions establish subjects such as `user:alice@example.com` when `subject_claim: email`. Browser-admin identity is separate from router caller tokens for `/v1/*` and from caller-token metrics access for `/metrics`. Keep password hashes and OIDC client secrets in environment variables, configure trusted proxy CIDRs for forwarded HTTPS state, and use secure session cookies in production. See [Admin Authentication](./admin-authentication) for bcrypt hash setup, TLS requirements, and admin auth validation endpoints. See [Admin Authorization](./admin-authorization) for Casbin metrics, content-capture maintenance, and report policy.

Governed content capture is separate from diagnostics and remains disabled unless `server.content_capture.enabled: true` and at least one scope is enabled. Captured rows live in `request_content_captures`, allowlisted headers in `request_content_headers`, and delete/purge audit events in `request_content_audit_events`. Rows are keyed by `request_id` for joins to usage metadata. Built-in secret redaction and configured `redaction_patterns` run before storage; `redact_before_storage: false` is rejected. Header capture is allowlist-only and rejects authorization, API-key, token, secret, cookie, and key-like header names. The current foundation supports retention purge and delete-by-request maintenance; KMS/encryption-at-rest and content export/read APIs are follow-up work, and `encryption.enabled: true` is rejected until implemented.

Deployments can keep capture disabled globally and enable a scoped override on a specific `callers[]` entry or `models.<group>.content_capture` block for a governed workload. Each enabled block must name at least one capture scope.

Content-capture maintenance endpoints:

These endpoints require caller-token subjects authorized for `content:capture`; delete-by-request uses action `delete`, and retention purge uses action `purge`. Existing `content_admin: true` callers receive compatible grants at startup.

```bash
curl -X DELETE "$SMART_ROUTER_BASE_URL/v1/content-captures/<request_id>" \
  -H "Authorization: Bearer $CONTENT_ADMIN_ROUTER_TOKEN"

curl -X POST "$SMART_ROUTER_BASE_URL/v1/content-captures/purge-expired" \
  -H "Authorization: Bearer $CONTENT_ADMIN_ROUTER_TOKEN"
```

Per-attempt upstream timeouts can be configured globally, per model group, or per target. `0` disables the per-attempt cap while the global `server.upstream.timeout_ms` still bounds the HTTP client. Exhausted upstream timeouts return `504 upstream-timeout`; exhausted provider rate limits return `503 upstream-rate-limited`; other exhausted upstream failures return `502 upstream-failed`.

Cataloged vision models are not automatically active routes. Keep a vision candidate catalog-only until it passes the exact direct upstream and router-level image smoke for the intended task and API dialect.

See [Image Analysis And VLM Routing](./image-analysis-vlm) for full OpenAI Chat, OpenAI Responses, Anthropic Messages, Codex CLI, and Claude Code image examples.
