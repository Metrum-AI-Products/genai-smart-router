# Smart LLM Router

Go implementation of the Smart LLM Router described in `LLM_Router_SRS_1.docx`.

For an external-facing technical overview, architecture diagrams, feature summary, and configuration walkthrough, see [docs/solution-brief.md](docs/solution-brief.md). The customer-facing hosted documentation is built from `docs-site/` and embedded into release binaries under `/docs/`; browser requests to `/` redirect there.

Current MVP capabilities:
- Anthropic Messages, OpenAI Chat Completions, and OpenAI Responses ingress.
- Anthropic token-count estimate endpoint for Claude Code startup.
- Bearer-token auth using configured SHA-256 token hashes.
- Config-driven model groups and static, weighted, failover, generic dynamic-score, script, external-policy, latency, cost, and stub semantic routing.
- TypeScript routing scripts for custom model-selection logic inside the Go router.
- External routing policy services for standalone web-service target selection with safe request, caller, target, pricing, tool, and modality context.
- Separate caller dialects from upstream provider adapters: callers can use Anthropic/OpenAI wire formats while targets route to Anthropic, OpenAI-compatible providers, or Replicate.
- Server-side provider key injection.
- Unary upstream proxying with caller-dialect response encoding.
- Caller-facing SSE framing for streaming requests.
- In-process LRU+TTL cache for eligible unary responses.
- Per-caller RPM, TPM, concurrency, rolling quota, and lifetime key budget enforcement.
- Disk-persisted quota/key state.
- JSONL request logs using the SRS schema.
- Metrics-admin-only Prometheus-compatible `/metrics` with caller/user/project labels.

## Cache Behavior

The response cache is in-process and not persistent. Configure it with:

```yaml
server:
  cache:
    enabled: true
    max_bytes: 134217728
    default_ttl: 15m
```

`default_ttl` is the maximum duration for an entry. `max_bytes` is the total LRU byte budget. Entries are evicted when expired or when the cache exceeds `max_bytes`.

The cache key is based on normalized request semantics and selected target: model group, system/input/messages, max tokens, temperature, stop sequences, provider, and target model. It does not use the raw request body, caller request IDs, router request IDs, caller tokens, caller project/user, or provider response IDs.

Cached payloads are sanitized before storage. The router caches text, model, stop reason, usage, and warnings, but not upstream `id`, raw provider payloads, or provider-specific metadata. Every caller-facing response gets a fresh router-owned `resp_...` ID, including cache hits.

Cache hits are logged with `cache=hit` and cached usage for telemetry. They do not call providers and do not increment persisted quota/lifetime token counters. Each request also records a cache snapshot with enabled state, item count, occupied bytes, max bytes, and occupancy percentage so usage reports can show cache hit rate and occupancy over time.

## Build And Package

The deployment artifact is a binary package. Operators should not need this source tree on the deployment host.

```bash
make build        # build Docusaurus docs, then local router/tool binaries with embedded docs
make build-go-only # local router/tool binaries without rebuilding docs
make package      # linux amd64 and linux arm64 tarballs
make package-all  # same as package
make package-docker # linux amd64 and linux arm64 Docker packages
make package-docker-all # same as package-docker
```

Each tarball contains:

```text
bin/router
bin/router-token-gen
bin/router-usage-report
config/config.example.yaml
config/env.example.json
config/scripts/router.ts
docs/README.md
docs/DEPLOYMENT.md
docs/DOCKER_DEPLOYMENT.md
docs/DYNAMIC_SCORE_ROUTING.md
docs/EXTERNAL_ROUTING_POLICY.md
docs/PII_FILTERING.md
docs/PRODUCT_CAPABILITY_MATRIX.md
docs/SECURITY_REVIEW_NOTES.md
docs/SELF_HOSTED_UPSTREAMS.md
docs/SMOKE_TEST_MATRIX.md
docs/USAGE_DB_DESIGN.md
docs/USAGE_REPORTING_PLAYBOOK.md
docs/solution-brief.md
caddy/Caddyfile
```

Packaged Markdown is copied only from `scripts/package_docs_allowlist.txt`. Private production runbooks, private host details, SSH paths, live compose config paths, and raw token/provider-key patterns are blocked by package validation.

The `router` binary embeds the Docusaurus build output. At runtime, browser access to `/` redirects to `/docs/`; API and operations routes such as `/v1/*`, `/metrics`, `/healthz`, and `/readyz` keep precedence.

Docker packages contain prebuilt image tarballs plus compose deployment assets:

```text
images/smart-llmrouter-<version>-linux-<arch>.tar
compose/docker-compose.yml
compose/docker-compose.postgres-localhost.yml
compose/Caddyfile.compose
compose/.env
compose/.env.example
config/config.example.yaml
config/env.example.json
config/scripts/router.ts
docs/README.md
docs/DEPLOYMENT.md
docs/DOCKER_DEPLOYMENT.md
docs/DYNAMIC_SCORE_ROUTING.md
docs/EXTERNAL_ROUTING_POLICY.md
docs/PII_FILTERING.md
docs/PRODUCT_CAPABILITY_MATRIX.md
docs/SECURITY_REVIEW_NOTES.md
docs/SELF_HOSTED_UPSTREAMS.md
docs/SMOKE_TEST_MATRIX.md
docs/USAGE_DB_DESIGN.md
docs/USAGE_REPORTING_PLAYBOOK.md
docs/solution-brief.md
```

The packaged config expects the routing script at `config/scripts/router.ts`, so the standard packaged run command is:

```bash
bin/router --config config/config.yaml
```

See `docs/DEPLOYMENT.md` for binary deployment guidance with Caddy TLS termination.

For Docker Compose deployments on AWS/EC2-style hosts, use `make package-docker` and follow `docs/DOCKER_DEPLOYMENT.md`. Docker packages include prebuilt image tarballs for linux/amd64 and linux/arm64, `docker-compose.yml`, an optional localhost-only Postgres override, Caddy config, router config templates, and docs; the target host does not need this source tree or a registry pull.

## Run From Source

Create a config from the example:

```bash
cp config.example.yaml config.yaml
go run ./cmd/router-token-gen generate \
  --user chetan \
  --project metrum-insights \
  --env dev \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Save the printed `token` value as the caller's bearer token, and copy the generated `callers:` entry into `config.yaml`. Tokens use the traceable prefix `rtr_metrum_<user>_<project>_<env>_<key>_<secret>`, while the router stores only `token_sha256` and logs/exports only `token_id`. Each caller `id`, `token_sha256`, and non-empty `token_id` must be unique; token hashes are checked case-insensitively.

Provider keys are read from `env.json` in this project before `${VAR}` references in `config.yaml` are expanded. Real `env.json` is gitignored; use `env.example.json` as the placeholder-only template. Do not paste production or personal provider keys into tracked examples; store real values in ignored `env.json`, the shell environment, or your deployment secret manager. Run `make secret-check` before publishing changes that touch tracked env examples.

```bash
go run ./cmd/router --config config.yaml
```

If a variable is already set in the shell, the shell value wins over `env.json`. This lets CI or one-off live tests override local secrets without editing files.

In a packaged deployment, put provider keys in `config/env.json` beside `config/config.yaml`. The same loading rule applies: shell environment values win over `env.json`.

Expected provider env vars in `config.example.yaml`:

```bash
ANTHROPIC_API_KEY
OPENAI_API_KEY
MOONSHOT_API_KEY
KIMI_API_KEY
QWEN_API_KEY
MINIMAX_API_KEY
OPENROUTER_API_KEY
GROQ_API_KEY
REPLICATE_API_KEY
XAI_API_KEY
BASETEN_API_KEY
CRUSOE_API_KEY
```

Provider adapter notes:
- `anthropic` targets call Anthropic Messages.
- `openai-chat` and `openai-responses` targets cover OpenAI-compatible API dialects. The current sample and production routes keep the active set intentionally small: Baseten `openai/gpt-oss-120b`, direct Moonshot Kimi `kimi-k2.7-code`, MiniMax `MiniMax-M3`, OpenRouter Gemma 4 26B Nitro where still configured in general groups, Baseten `nvidia/Nemotron-120B-A12B` at low text weight, Baseten `zai-org/GLM-5.2` for reasoning-heavy coding traffic, Crusoe Gemma 4 31B-it in the reference `big-coder` ordinary-text route, and original OpenAI `gpt-5.4-nano` at low non-tool weight.
- Crusoe Managed Inference is configured as an external hosted OpenAI-compatible `openai-chat` provider with `base_url: https://api.inference.crusoecloud.com/v1` and `api_key_env: CRUSOE_API_KEY`. Keep unvalidated Crusoe models catalog-only or in dedicated smoke groups until the exact account, model ID, tool behavior, structured-output behavior, output-cap behavior, and router usage/cost fields have passed. The reference `big-coder` route includes Crusoe Gemma 4 31B-it for ordinary text traffic after direct and router-level smokes, and keeps a separate tool-only Crusoe Gemma target for OpenAI Chat tool traffic.
- Enterprise-owned vLLM and SGLang services are configured the same way as other OpenAI-compatible providers: set `base_url` to the internal `/v1` endpoint, use `dialect: openai-chat` for `/v1/chat/completions`, set `auth_scheme: bearer` when the service expects bearer auth, and catalog the served model ID under `providers.<name>.models`. See `docs/SELF_HOSTED_UPSTREAMS.md` for vLLM/SGLang examples and tool-call validation smokes.
- MiniMax, Kimi, and OpenRouter can also be configured through Anthropic-compatible skins with `dialect: anthropic` and `auth_scheme: bearer`, which is useful for Claude Code callers without routing to Anthropic models. OpenRouter can also be configured as a separate `openai-responses` provider for Codex tool calls.
- `replicate` targets call Replicate Predictions. Use `target.model` as `owner/model-name`, for example `meta/meta-llama-3-70b-instruct`.

## API Key Flow

The router uses two different classes of keys:

- Caller tokens authenticate clients that call this router. A caller sends `Authorization: Bearer <router-token>` or `X-API-Key: <router-token>`. The router hashes the presented token with SHA-256, compares it to configured `callers[].token_sha256`, checks `allow`, rate limits, quotas, and lifetime token budget, then logs/exports only caller metadata and `token_id`. Config validation rejects duplicate caller `id`, duplicate `token_sha256` values case-insensitively, and duplicate non-empty `token_id` values.
- Provider API keys authenticate the router to upstream LLM providers. They come from `providers.<name>.api_key`, usually via `${OPENAI_API_KEY}`, `${OPENROUTER_API_KEY}`, `${GROQ_API_KEY}`, `${MOONSHOT_API_KEY}`, and similar values loaded from `env.json` or the shell. The router injects the selected provider key only when calling the selected upstream target.

`callers[].allow` is the per-key allow list for internal router model group names. Model group names are deployment-defined; names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, and `vision` are examples from the reference or hosted deployment, not product-required names. Disallowed model requests return `403 model-not-allowed` before provider routing and before any provider API key is used. The authenticated `/v1/models` response is filtered to the caller token's allowed groups.

Token-budget admission reserves the estimated input tokens plus the caller's requested output cap before an upstream call. Chat Completions uses `max_tokens` or `max_completion_tokens`, Responses uses `max_output_tokens`, and Messages uses `max_tokens`; Anthropic Messages requests without a caller cap reserve the router's injected default output cap. TPM, daily token, monthly token, and lifetime key budgets include in-flight reservations so concurrent large-cap requests cannot overshoot the configured budget. Completed requests reconcile the reservation to actual reported usage, failed or canceled requests release it, and cache hits do not consume persisted token quota.

Set realistic output caps for each client workflow. Very large caps can be rejected near a token budget even when the prompt is small, because the router admits based on the maximum output the caller asked the upstream to generate. Request-count quotas are unchanged and still count admitted requests independently from token usage.

Raw caller tokens, caller token hashes, and raw provider API keys are not exposed to TypeScript routing scripts, logs, metrics, or responses. Scripts get safe identifiers only: caller `id`, `user`, `project`, `environment`, `tokenId`, and target `keyId`, `apiKeyEnv`, and `keyConfigured`. This is enough to route by caller key prefix or by the configured provider key name without making secrets available to script code.

## Provider Model Catalogs

A provider can serve many locally configured models without repeating its base URL or credentials:

```yaml
providers:
  minimax:
    base_url: https://api.minimax.io/v1
    dialect: openai-chat
    api_key: ${MINIMAX_API_KEY}
    api_key_env: MINIMAX_API_KEY
    key_id: minimax-default
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
    key_id: openrouter-default
    models:
      gemma-4-26b-a4b-it-nitro:
        model: google/gemma-4-26b-a4b-it:nitro
        tier: balanced
        input_price_per_million_usd: 0.06
        output_price_per_million_usd: 0.33
        pricing_source: https://openrouter.ai/api/v1/models
        pricing_updated_at: "2026-06-17"
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
  baseten:
    base_url: https://inference.baseten.co/v1
    dialect: openai-chat
    api_key: ${BASETEN_API_KEY}
    api_key_env: BASETEN_API_KEY
    key_id: baseten-default
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
        pricing_notes: Baseten also lists a discounted cache-input rate; router cost logs use standard input/output rates plus upstream-reported billed cost when available.
        tool_support:
          openai_chat: [tools, tool_choice]
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        tier: coding
        input_price_per_million_usd: 0.10
        output_price_per_million_usd: 0.50
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.baseten.co/products/model-apis/
        pricing_updated_at: "2026-06-22"
        pricing_notes: Direct Baseten OpenAI Chat and Anthropic Messages text/tool smokes passed on 2026-06-22.
        tool_support:
          openai_chat: [tools, tool_choice]
  crusoe:
    base_url: https://api.inference.crusoecloud.com/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${CRUSOE_API_KEY}
    api_key_env: CRUSOE_API_KEY
    key_id: crusoe-default
    headers:
      User-Agent: smart-llmrouter
    models:
      gpt-oss-120b:
        model: openai/gpt-oss-120b
        tier: coding
        input_price_per_million_usd: 0.05
        output_price_per_million_usd: 0.20
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: Crusoe also publishes cached-token pricing; keep standard input/output rates for router-calculated cost until cached-token upstream billing has dedicated accounting. Catalog-only until direct Crusoe and router-level text, streaming, cap, tool, structured-output, and Harbor smokes pass.
      gemma-4-31b-it:
        model: google/gemma-4-31b-it
        tier: balanced
        input_price_per_million_usd: 0.14
        output_price_per_million_usd: 0.40
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: Crusoe Gemma 4 31B-it OpenAI Chat smokes passed for text, streaming, max_tokens=1, auto tool, forced tool_choice, structured outputs, and combined tool plus structured-output requests. The reference big-coder group uses it for ordinary OpenAI Chat text traffic and as a separate OpenAI Chat tool-only target; do not use it for OpenAI Responses or Anthropic Messages unless those skins pass separately.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
      llama-3-3-70b-instruct:
        model: meta-llama/Llama-3.3-70B-Instruct
        tier: balanced
        input_price_per_million_usd: 0.25
        output_price_per_million_usd: 0.75
        input_modalities: [text]
        output_modalities: [text]
        pricing_source: https://www.crusoe.ai/cloud/pricing
        pricing_updated_at: "2026-06-24"
        pricing_notes: Crusoe quickstart example model. Direct Crusoe text, streaming, max_tokens=1, auto tool, forced tool_choice, and structured-output smokes passed on 2026-06-24 with an explicit User-Agent. Local router-level text, streaming, cap, tool, structured-output, usage, cost, latency, and no-fallback smokes also passed. Keep cataloged or in dedicated smoke groups unless workload validation passes for the deployment.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
  openai:
    base_url: https://api.openai.com/v1
    dialect: openai-responses
    api_key: ${OPENAI_API_KEY}
    api_key_env: OPENAI_API_KEY
    key_id: openai-default
    models:
      gpt-5.4-nano:
        model: gpt-5.4-nano
        tier: small
        input_price_per_million_usd: 0.20
        output_price_per_million_usd: 1.25
        pricing_source: https://developers.openai.com/api/docs/models/gpt-5.4-nano
        pricing_updated_at: "2026-06-17"
        tool_support:
          openai_responses: [function]
  baseten_anthropic:
    base_url: https://inference.baseten.co
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${BASETEN_API_KEY}
    api_key_env: BASETEN_API_KEY
    key_id: baseten-anthropic-default
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
        tool_support:
          anthropic_messages: [client_tools]
  openrouter_anthropic:
    base_url: https://openrouter.ai/api
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${OPENROUTER_API_KEY}
    api_key_env: OPENROUTER_API_KEY
    key_id: openrouter-anthropic-default
    models:
      gemma-4-26b-a4b-it-nitro:
        model: google/gemma-4-26b-a4b-it:nitro
        tier: balanced
        input_price_per_million_usd: 0.06
        output_price_per_million_usd: 0.33
        pricing_source: https://openrouter.ai/api/v1/models
        pricing_updated_at: "2026-06-17"
        tool_support:
          anthropic_messages: [client_tools]
  kimi:
    base_url: https://api.moonshot.ai/v1
    dialect: openai-chat
    api_key: ${MOONSHOT_API_KEY}
    api_key_env: MOONSHOT_API_KEY
    key_id: moonshot-kimi-default
    models:
      kimi-k2.7-code:
        model: kimi-k2.7-code
        tier: heavy
        input_price_per_million_usd: 0.74
        output_price_per_million_usd: 3.50
        pricing_source: https://platform.kimi.ai/docs/pricing/chat-k27-code
        pricing_updated_at: "2026-06-17"

models:
  default:
    strategy: script
    script: scripts/router.ts
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 51 }
      - { provider: minimax, model_ref: m3, weight: 27 }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 3 }
      - { provider: baseten, model_ref: glm-5-2, weight: 5 }
      - { provider: openrouter, model_ref: gemma-4-26b-a4b-it-nitro, weight: 7 }
      - { provider: kimi, model_ref: kimi-k2.7-code, weight: 6 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }
      - { provider: baseten_anthropic, model_ref: gpt-oss-120b, tool_only: true, weight: 8 }
```

`model_ref` is local to its provider. Provider model catalogs are reusable upstream model metadata, not routing policy. Weights are group-local and only belong under `models.<group>.targets[]`, so the same `model_ref` can have different relative weights in `default`, `fast`, `big-coder`, or any other group. Direct `{ provider, model }` targets are still supported.

Catalog metadata can include `input_price_per_million_usd`, `output_price_per_million_usd`, optional VLM fields such as `image_input_price_per_million_tokens_usd` and `image_input_price_per_image_usd`, `input_modalities`, `output_modalities`, `pricing_source`, `pricing_updated_at`, and `tool_support`. Pricing is copied onto the selected target at request time and logged with calculated input, image, output, and total USD cost, so historical usage rows keep the price that was used even if provider pricing changes later. When an upstream returns billed cost metadata, the router logs that upstream-reported cost separately from router-calculated cost. `tool_support` is dialect-specific:

- `openai_chat`: upstream supports OpenAI-compatible chat `tools`, `tool_choice`, and/or `response_format` structured outputs.
- `openai_responses`: upstream supports Responses function tools and/or `text.format` structured outputs.
- `anthropic_messages`: upstream supports Anthropic Messages client tools.
- `provider_hosted`: reserved for provider-executed tools such as web search or code execution after that exact upstream capability is validated.

Use explicit capability labels such as `tools`, `tool_choice`, `function`, `client_tools`, and `structured_outputs`. OpenAI Chat and OpenAI Responses are separate validation surfaces; a Chat `response_format` pass does not prove Responses `text.format`, and Anthropic Messages has no OpenAI structured-output equivalent unless a deployment adds and documents one. Structured-output requests route only to targets with matching `structured_outputs` metadata for the caller dialect. The router forwards JSON Schema payloads to the selected upstream and does not perform application-level JSON Schema validation, schema-subset enforcement, or output repair unless a separate feature implements that behavior. Unsupported schemas can still fail with upstream/provider errors.

Before declaring `structured_outputs`, run a direct upstream schema smoke and the same request through the router for the exact provider, model ID, dialect, skin, and client request shape. If clients combine tools and structured-output fields, run a combined tool plus structured-output smoke and require both capabilities on the same target. If smokes fail after rollout, remove the `structured_outputs` metadata from the provider model or target override; if the target is broadly unsafe, remove it from active `models.<group>.targets[]` and keep the catalog entry disabled until validation passes.

Cataloging a model does not route traffic to it. Add a cataloged model to a group target only after its provider key has access and a direct live smoke test succeeds. The current reference config keeps OpenAI `gpt-5.4-nano` at a low non-tool fallback weight and includes an example opt-in image-analysis group. Original Anthropic is supported by the provider adapter, but it is not active in the production/reference routing set until an Anthropic key is present and a live smoke passes.

Baseten Model APIs are configured as OpenAI-compatible `openai-chat` providers, and Baseten's Anthropic Messages beta endpoint can be configured as a separate `dialect: anthropic` provider for Claude Code-style traffic. On 2026-06-17, `nvidia/Nemotron-120B-A12B` passed direct non-streaming chat, streaming chat with `stream_options.include_usage` and `continuous_usage_stats`, and an OpenAI Chat function-call smoke that returned a valid `tool_calls` response. On 2026-06-18, `zai-org/GLM-5.2` passed direct realistic-budget text, `max_tokens` cap, and OpenAI Chat tool-call smokes. On 2026-06-22, `openai/gpt-oss-120b` passed direct Baseten OpenAI Chat text, streaming, auto tool, forced tool-choice, Anthropic Messages text, and Anthropic Messages client-tool smokes. The router still synthesizes downstream streaming for normal upstream calls, so Baseten-specific upstream streaming options are a provider validation detail rather than a required caller setting.

Crusoe Managed Inference is an external hosted OpenAI-compatible provider. Crusoe documentation checked on 2026-06-24 lists `https://api.inference.crusoecloud.com/v1` as the OpenAI-compatible base URL, documents API keys from the Intelligence Foundry console, and uses `meta-llama/Llama-3.3-70B-Instruct` in the quickstart. A direct `/v1/models` check on 2026-06-24 required an explicit `User-Agent` and returned exact IDs such as `openai/gpt-oss-120b`, `google/gemma-4-31b-it`, `meta-llama/Llama-3.3-70B-Instruct`, `zai/GLM-5.2`, `moonshotai/Kimi-K2.6`, `Qwen/Qwen3-235B-A22B-Instruct-2507`, and `nvidia/NVIDIA-Nemotron-3-Super-120B-A12B`. Direct and local router-level Llama text, streaming, `max_tokens: 1`, auto tool, forced tool-choice, and structured-output smokes passed on 2026-06-24. Direct Crusoe Gemma 4 31B-it OpenAI Chat text, streaming, `max_tokens: 1`, auto tool, forced tool-choice, structured-output, and combined tool plus structured-output smokes also passed on 2026-06-24. The reference config catalogs other source-dated Crusoe text models, includes dedicated `crusoe-smoke` and `crusoe-gemma-smoke` groups, uses Crusoe Gemma 4 31B-it at 20% in the `big-coder` ordinary-text route, and keeps a separate Crusoe Gemma OpenAI Chat tool-only target. Do not claim Crusoe OpenAI Responses or Anthropic Messages support unless those skins are separately exposed and validated.

OpenAI Chat tool passthrough is used by OpenAI-compatible agent clients such as Warp Agent. These clients call `/v1/chat/completions`, send `tools`, `tool_choice`, and often request streaming. For those requests, the router preserves the OpenAI Chat tool payload and tool-result messages, selects only upstream targets with explicit `tool_support.openai_chat`, calls the upstream non-streaming, and returns either the raw non-streaming response or synthesized OpenAI Chat SSE chunks containing `delta.tool_calls`. This avoids asking users to switch model groups just because a coding-agent turn includes tools; the configured group filters to compatible targets automatically.

Image requests are detected across OpenAI Chat, OpenAI Responses, and Anthropic Messages content blocks. The router filters image-bearing requests to targets with `image` in `input_modalities`, skips text-only targets, bypasses response caching, and logs `input_has_image`, `input_image_count`, upstream image-token counts when reported, calculated image cost, and upstream-reported billed cost when available.

Coding-agent groups should not force users to switch between a language model group and a vision model group during one task. Add validated multimodal `tool_only` targets to deployment-defined coding groups, for example the reference `big-coder` group, for Codex Responses and Claude Code Anthropic Messages traffic. Text-only requests continue to use the normal weighted coding targets; image-bearing agent requests automatically filter to multimodal tool-capable targets.

Vision catalog entries are not automatically active routes. For example, OpenRouter `qwen/qwen3-vl-32b-instruct:nitro` can be cataloged with `input_modalities: [text, image]` and current OpenRouter pricing, but it should remain out of active OCR routes until a router-level receipt OCR smoke returns the expected merchant name. For general VLM routing, distinguish image processing from OCR-quality gating and keep models that produce weak or inconsistent image analysis out of broad active groups until the deployment's acceptance tests pass.

Do not add `~google/gemini-flash-latest`, `google/gemini-3.5-flash`, `google/gemini-3.1-flash-lite`, or `google/gemini-3.1-pro-preview` to active OpenRouter routes for the current production key until access is fixed and a live smoke passes. On 2026-06-17 the OpenRouter catalog advertised multimodal support for those IDs, but the current account returned a 404 provider-privacy error for direct image requests.

xAI Grok 4.3 is cataloged as an OpenAI-compatible `openai-chat` provider with `input_modalities: [text, image]`, `output_modalities: [text]`, official pricing of $1.25/M input and $2.50/M output tokens, and `image_input_price_per_million_tokens_usd: 1.25` because xAI reports image tokens in prompt usage. Direct xAI text/image smokes and local router-level text/image smokes passed on 2026-06-17. A deployment-defined VLM route can include Grok alongside validated multimodal targets such as OpenRouter-hosted Claude Sonnet, OpenRouter-hosted Grok, MiniMax-M3, and original OpenAI `gpt-5.4-nano` with `input_modalities: [text, image]`. If a target accepts image requests but ignores explicit caller caps, mark it `honors_max_tokens: false`; production capped-request smokes on 2026-06-18 found this on several OpenRouter-hosted VLM targets, so capped requests skip those targets until revalidated. OpenAI Chat `max_tokens` and `max_completion_tokens`, Responses `max_output_tokens`, and Anthropic Messages `max_tokens` all count as explicit output caps for eligibility.

Self-hosted OpenAI-compatible services such as vLLM and SGLang should be validated exactly like SaaS providers before activation. Confirm `/v1/models`, run a direct text completion smoke, run a direct tool-call smoke if the model is intended for agent tools, then repeat the same request through the router group. Tool calling depends on the upstream model, chat template, parser flags, and `tool_choice` support; do not mark a self-hosted target tool-capable just because the server accepts a `tools` field.

Agentic tool-call traffic can use a separate target set from ordinary text traffic. Mark a target with `tool_only: true` when it should only be considered for requests that include supported tools, such as Codex OpenAI Responses tool calls or Claude Code Anthropic tool calls:

```yaml
models:
  big-coder:
    strategy: weighted
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 18 }
      - { provider: minimax, model_ref: m3, weight: 30 }
      - { provider: kimi, model_ref: kimi-k2.7-code, weight: 23 }
      - { provider: crusoe, model_ref: gemma-4-31b-it, weight: 20 }
      - { provider: baseten, model_ref: glm-5-2, weight: 6 }
      - { provider: baseten, model_ref: nemotron-120b-a12b, weight: 2 }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 1 }
      - { provider: minimax, model_ref: m3, dialect: openai-responses, tool_only: true }
      - { provider: openrouter_responses, model_ref: openrouter-claude-sonnet-4-6, tool_only: true, weight: 13 }
      - { provider: minimax_anthropic, model_ref: m3, tool_only: true }
      - { provider: baseten_anthropic, model_ref: gpt-oss-120b, tool_only: true, weight: 8 }
      - { provider: kimi_anthropic, model_ref: kimi-k2.7-code, tool_only: true, default_thinking: { type: enabled, budget_tokens: 512 } }
      - { provider: openrouter_anthropic, model_ref: openrouter-claude-sonnet-4-6, tool_only: true, weight: 4 }
      - { provider: openrouter_anthropic, model_ref: gemma-4-26b-a4b-it-nitro, tool_only: true, weight: 2 }
      - { provider: crusoe, model_ref: gemma-4-31b-it, tool_only: true, weight: 2 }
```

Non-tool requests ignore `tool_only` targets. Tool-bearing requests only use targets whose upstream dialect can preserve the caller's tool protocol; for example, an OpenAI Chat tool request can use an OpenAI Chat-compatible Crusoe target, while an Anthropic Messages tool request needs an Anthropic-compatible target. Tool-bearing requests also bypass response caching because tool results depend on external filesystem, shell, and agent state.

## Dynamic Score Routing

Use `strategy: dynamic_score` when a deployment wants one configurable per-group policy instead of separate hardcoded strategies for cheap-fast routing, latency-aware routing, workload complexity, budget pressure, or evaluation-backed quality preferences. Callers still request a model group they are allowed to use. The router authenticates the caller, validates that group access, filters only that group's `targets[]` for API dialect, tool support, modalities, and explicit max-token safety, then scores only the remaining targets in that same group.

```yaml
models:
  adaptive-agent:
    strategy: dynamic_score
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 60, tags: [validated, coding, tool_capable] }
      - { provider: minimax, model_ref: m3, weight: 25, tags: [validated, low_cost, tool_capable] }
      - { provider: openai, model_ref: gpt-5.4-nano, weight: 5, tags: [fallback] }
    routing_policy:
      dynamic_score:
        cold_start_policy: configured_weight
        min_observations: 20
        observation_window_seconds: 600
        max_score_adjustment_percent: 70
        hard_filters:
          require_requested_api_skin: true
          require_input_modalities: true
          require_tool_support_when_tools_present: true
          require_honors_max_tokens_when_caller_capped: true
        signals:
          request_shape: { enabled: true }
          prompt_features:
            enabled: true
            max_scan_bytes: 16384
            features: [code, diff, stack_trace, summarize, extract, security_review, tool_agent]
          complexity: { enabled: true }
          observed_performance: { enabled: true }
          cost: { enabled: true }
          evaluation_metadata: { enabled: true }
        score_terms:
          - name: cheapest_fast_enough
            when: { complexity_lte: standard }
            expression: "0.45 * cost_score + 0.25 * latency_score + 0.20 * throughput_score + 0.10 * reliability_score"
          - name: complex_quality_floor
            when: { complexity_gte: complex }
            require_tags: [validated]
            expression: "0.45 * eval_quality_score + 0.25 * reliability_score + 0.20 * latency_score + 0.10 * cost_score"
        thresholds:
          max_error_rate: 0.03
          max_timeout_rate: 0.02
          max_p95_latency_ms: 10000
```

Cold start is deterministic: until `min_observations` is reached, targets are ordered by configured group-local weight. After that, the router uses in-memory rolling observations for latency, throughput, error rate, timeout rate, and fallback rate; it does not read the usage database on the hot path. Decision traces contain only safe scalar metadata such as enabled signal names, request-shape buckets, candidate count, selected provider/model, score bucket, observation count, and cold-start mode. They must not contain raw prompts, images, tool outputs, router tokens, token hashes, provider keys, or full upstream headers.

Rollout should start on a deployment-defined test group with interchangeable validated targets. Use mock or local router smokes for simple text, code/debug prompts, tool calls, forced tool calls, image requests when supported, structured-output requests when supported, and low output caps for each caller API. Roll back by switching the group strategy to `weighted` or by removing score terms and thresholds that are too strict for the workload.

For structured-output rollout, smoke both Chat Completions `response_format` and Responses `text.format` if both dialects are configured. Also run a negative router smoke against a group with no structured-output-capable target and expect `502 no-eligible-target` with no upstream attempt. If a target claims both tools and structured outputs, include a combined request in rollout validation. Streaming clients should be told whether the router is returning provider-native streaming or synthesizing downstream SSE from a unary upstream call; schema-constrained incremental chunks are provider-specific and not guaranteed by the router.

For OpenAI Chat tool clients, for example Warp Agent, configure the client with:

```text
Base URL: https://your-router.example.com/v1
API key: <router caller token>
Model: <allowed-model-group>
```

Use whichever deployment-defined model group the caller token allows. If a request includes `tools`, structured-output fields, images, or explicit output caps and no eligible target in that group declares the required support, the router returns `502 no-eligible-target` with a hint to enable an upstream target that supports the requested dialect, tools, structured outputs, modalities, and cap behavior.

For providers that use Anthropic Messages shape but bearer-token authentication, set `auth_scheme: bearer`:

```yaml
providers:
  minimax_anthropic:
    base_url: https://api.minimax.io/anthropic
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${MINIMAX_API_KEY}
    api_key_env: MINIMAX_API_KEY
    models:
      m3: { model: MiniMax-M3, tier: heavy }
  kimi_anthropic:
    base_url: https://api.moonshot.ai/anthropic
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${MOONSHOT_API_KEY}
    api_key_env: MOONSHOT_API_KEY
    models:
      kimi-k2.7-code: { model: kimi-k2.7-code, tier: heavy }
  openrouter_anthropic:
    base_url: https://openrouter.ai/api
    dialect: anthropic
    auth_scheme: bearer
    api_key: ${OPENROUTER_API_KEY}
    api_key_env: OPENROUTER_API_KEY
    models:
```

## TypeScript Routing

Use `strategy: script` on a model group and point `script` at a TypeScript file:

```yaml
models:
  default:
    strategy: script
    script: scripts/router.ts
    targets:
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 60 }
      - { provider: minimax, model_ref: m3, weight: 30 }
      - { provider: kimi, model_ref: kimi-k2.7-code, weight: 10 }
```

The script must export `route(ctx)` and return one configured target by index or by `{ provider, model }`. Proxy users still request a deployment-defined model group name; the script chooses one backing target from that group's configured `targets`.

The script context uses top-level `ctx.text` for normalized request text, plus `ctx.group`, `ctx.request`, `ctx.caller`, and `ctx.targets`. Target metadata includes provider, model, modelRef, baseUrl, dialect, weight, keyId, apiKeyEnv, and keyConfigured. For groups with `pii_filter`, `ctx.text`, normalized request fields, and `ctx.request.raw` are redacted before the script runs, and placeholder mappings are not exposed. Raw provider API keys, raw caller tokens, and caller token hashes are never passed to scripts; returned targets are validated against the configured list. Scripts run synchronously inside the router process, so keep policy local and fast; unrestricted network calls and file access are not part of the script runtime.

Relative TypeScript imports are bundled at router startup, so a script can use local helpers such as `import { scorePrompt } from "./policy"`. Keep deployment-owned helpers next to the script, for example `config/scripts/router.ts`, `config/scripts/policy.ts`, and `config/scripts/scoring.ts`.

Third-party dependencies must be installed, locked, and packaged before deployment. The router bundles from the deployment filesystem at startup; it does not run `npm install`, download packages, or resolve network dependencies at runtime. For npm-based policy helpers, manage dependencies under the script directory, package `package.json`, the lockfile, and the resolved dependency tree or a pre-bundled script artifact, and keep that tree free of provider keys, router tokens, and private host credentials. For large dependencies or native modules, prefer pre-bundling during release and deploying the generated entrypoint.

External policy calls are opt-in per model group through deployment config:

```yaml
models:
  default:
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

Scripts call external policy with `router.fetchJSON(url, options)`, not browser `fetch`. The helper supports `GET` and `POST`, JSON request bodies, JSON responses, script-supplied headers limited to `Accept`, `Content-Type`, and `X-*`, and only hosts in the model group's `script_http.allow_hosts`. HTTPS is required by default. Plain HTTP is accepted only for loopback hosts such as `localhost`, `127.0.0.1`, and `::1`, or when `script_http.allow_http: true` is set for a trusted non-local policy service. Redirects are followed only when each hop keeps an allowed `http`/`https` scheme and an exact allowlisted hostname. Put policy-service auth in deployment config with `script_http.headers`, for example `Authorization: ${ROUTING_POLICY_AUTH_HEADER}`, rather than in script source. `timeout_ms` is capped at `5000`; use smaller values for routing policy because it runs before the upstream model request.

A demo PII-aware routing policy lives in `examples/typescript-pii-policy/`. It detects common PII-like patterns in `ctx.text`, sends matching requests only to configured targets marked `sensitive` or `private`, restricts retry fallbacks to those same sensitive/private targets, sends non-matching requests to a normal target, and returns only safe class labels such as `pii-detected:sensitive-route` or `pii-detected:none`. If PII is detected and no sensitive/private target is eligible, the demo fails closed with `pii-detected:no-sensitive-target`. This is routing only: TypeScript scripts do not redact outbound request content. Use model-group `pii_filter` when the router must redact, restore, or fail requests before upstream calls.

## External Routing Policy Service

Use `strategy: external` when routing policy should live in a standalone web service instead of in TypeScript. The router sends normalized request context, safe caller metadata, eligible target metadata, pricing, tools, and modalities to the configured policy URL, then validates the returned target against the model group's eligible targets. For groups with `pii_filter`, the policy payload is built from the redacted request object, including `request.raw`; placeholder mappings are not sent. Raw router tokens, token hashes, and provider API keys are never sent.

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

Policy responses use the same selector shape as TypeScript: `targetIndex` or `target`, optional `fallbackIndexes`/`fallbacks`, and optional `classLabel`. When a TypeScript script omits `fallbackIndexes` and `fallbacks`, remaining eligible targets are used as retries; when either field is present, the supplied entries are the complete retry set. The default `on_error` behavior is `fail_closed`, returning `502 routing-policy-error`; `fallback` can be configured when the target order is an acceptable default. A runnable demo service lives at `examples/external-routing-policy/prompt_size_policy.py`.

External policy URLs use the same egress rules as `router.fetchJSON`: HTTPS by default, plaintext HTTP only for loopback hosts or with `external_policy.allow_http: true`, exact-host allowlisting, and redirect revalidation on every hop. A redirect to a host outside `allow_hosts`, including a loopback address that was not explicitly allowed, fails before the redirected service is reached.

The default `scripts/router.ts` does three things:

- Removes targets whose provider key is not configured or whose target weight is zero.
- Applies named regex rules against safe caller-key metadata and safe target-key metadata.
- Falls back to weighted random routing across eligible targets, using group target weights as relative probabilities.

## PII Filtering

Model groups can configure `pii_filter` rules to replace matched text with typed placeholders before target selection, cache-key generation, routing-policy inputs, and upstream provider calls. The redacted request object is the source of truth for policy contexts, including script `ctx.request.raw` and external policy `request.raw`. Modes support `redact_only`, `redact_and_restore`, and `fail_on_match`. Usage logs and the usage database store only safe scalar metadata such as applied flag, mode, replacement count, and matched-rule count; raw matched values and placeholder mappings remain in memory for the request lifecycle by default.

See `docs/PII_FILTERING.md` and the Docusaurus PII Filtering page for configuration examples and smoke-test guidance.

Prompt-size routing example:

```ts
type Target = {
  provider: string;
  model: string;
  tier?: string;
  weight: number;
  keyConfigured: boolean;
};

type RouteContext = {
  text: string;
  targets: Target[];
};

export function route(ctx: RouteContext) {
  const eligible = ctx.targets
    .map((target, index) => ({ target, index }))
    .filter((entry) => entry.target.keyConfigured && entry.target.weight > 0);

  if (eligible.length === 0) {
    return { targetIndex: 0, classLabel: "prompt-size:no-eligible-targets" };
  }

  const preferredTier = ctx.text.length > 8000 ? "heavy" : "cheap";
  const preferred = eligible.find((entry) => entry.target.tier === preferredTier) || eligible[0];

  return {
    targetIndex: preferred.index,
    fallbackIndexes: eligible
      .filter((entry) => entry.index !== preferred.index)
      .map((entry) => entry.index),
    classLabel: `prompt-size:${preferredTier}`,
  };
}
```

Caller metadata enables key-specific routing with regular expressions over the generated router-token prefix. Target metadata also lets the script route to targets backed by a specific configured provider key identifier or environment variable name:

```ts
export function route(ctx) {
  if (
    /^rtr_metrum_chetan_metrum-insights_prod_/.test(ctx.caller?.tokenId || "") &&
    /^metrum-insights$/.test(ctx.caller?.project || "")
  ) {
    const heavyIndex = ctx.targets.findIndex((target) =>
      target.tier === "heavy" &&
      (
        (/^openrouter-default$/.test(target.keyId || "") &&
          /^OPENROUTER_API_KEY$/.test(target.apiKeyEnv || "")) ||
        (/^openai-default$/.test(target.keyId || "") &&
          /^OPENAI_API_KEY$/.test(target.apiKeyEnv || ""))
      ) &&
      target.keyConfigured
    );
    if (heavyIndex >= 0) {
      return { targetIndex: heavyIndex, classLabel: "key-regex:prod-heavy" };
    }
  }
  return { targetIndex: 0, classLabel: "default" };
}
```

`ctx.caller.tokenId` is the generated token prefix without the secret suffix, for example `rtr_metrum_chetan_metrum-insights_prod_key1`. Use it for traceable key classes. Do not route on raw token secrets; the router never passes them to scripts.

Check which names are present without printing secret values:

```bash
env | grep -E '^(ANTHROPIC|OPENAI|MOONSHOT|KIMI|QWEN|MINIMAX|OPENROUTER|REPLICATE|XAI)_.*=' | sed 's/=.*/=***REDACTED***/'
```

Health checks:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl -H "Authorization: Bearer $METRICS_ADMIN_ROUTER_TOKEN" http://127.0.0.1:8080/metrics
```

`/metrics` is intentionally restricted to caller entries with `metrics_admin: true`; normal application keys receive `403 metrics-forbidden`. Use `/v1/usage` and durable usage reports for caller-scoped usage views.

Version checks:

```bash
./router --version
./router-token-gen --version
./router-usage-report --version
curl http://127.0.0.1:8080/version
```

`/healthz`, `/readyz`, and `/version` include the router version, commit, and full UTC build timestamp. Metrics-admin `/metrics` exports `smart_llmrouter_build_info`. Browser docs show the running docs package version and build timestamp on every page, and docs responses include `X-Smart-LLMRouter-Version`, `X-Smart-LLMRouter-Commit`, and `X-Smart-LLMRouter-Build-Date` headers. OpenAI-compatible `/v1/*` response bodies do not include router-specific version fields.

## Usage Reports

Usage is written to both JSONL and a GORM-backed relational database. SQLite is the default for local use; Docker Compose deployments can use Postgres via `server.usage_db.driver: postgres` and `server.usage_db.dsn`. The schema is scalar and relational only: no JSONB, JSON, array, or packed multi-value DB columns.

Diagnostics are written alongside usage when `server.diagnostics.enabled` is true. Each request can have child rows in `request_attempts`, `request_trace_events`, and `request_errors`, all keyed by `request_id`. Use the `X-Request-Id` response header or the `request_id` in an error body to join these rows during incident response. Diagnostic rows store provider/model/status/timing/error-class data, not raw prompts, images, bearer tokens, provider keys, token hashes, full upstream headers, or raw upstream response bodies. `store_sanitized_upstream_errors` can keep bounded sanitized error context for troubleshooting, but it is not content capture and still redacts prompt-like fields, nested upstream bodies, and secret-shaped values before JSONL or usage DB persistence.

Governed content capture is a separate opt-in feature under `server.content_capture`. It is disabled by default and writes redacted request, response, and upstream-error content to `request_content_captures` plus allowlisted headers in `request_content_headers`, both joinable to `request_usage` by `request_id`. Admin delete and retention purge write `request_content_audit_events` and require a caller with `content_admin: true`; metrics-admin tokens do not grant content maintenance access. The first slice always requires `redact_before_storage: true`, rejects forbidden header names such as authorization/API-key/token headers, and rejects `encryption.enabled: true` until KMS-backed encryption is implemented.

Upstream timing is configurable with `server.upstream.timeout_ms`, `server.upstream.default_attempt_timeout_ms`, model-group `attempt_timeout_ms`, and per-target `timeout_ms`. A target timeout overrides a group timeout, and a group timeout overrides the global default attempt timeout. `0` disables the per-attempt cap while preserving the global HTTP client timeout. Exhausted upstream timeouts return `504 upstream-timeout`; exhausted provider 429s return `503 upstream-rate-limited`; other exhausted upstream failures return `502 upstream-failed`.

Each request row stores the configured input/output price per million tokens for the selected upstream model, the pricing source/update date, and calculated input/output/total USD cost. These values are logged at request time instead of recalculated during reporting, so historical cost reports remain stable after upstream providers change pricing.

The JSONL file is useful for raw audit/debugging. The relational DB is the source for periodic reports. In container deployments using SQLite, use `/app/logs/requests.jsonl` and `/app/state/usage.sqlite`. In Postgres deployments, the report tool reads from the configured DSN.

`router-usage-report` flags:

```text
--driver NAME   Usage DB driver: sqlite or postgres; defaults to sqlite.
--db PATH       SQLite usage DB path; defaults to usage.sqlite.
--dsn DSN       Postgres DSN when --driver=postgres.
--log PATH      Optional JSONL request log to import before reporting.
--since DUR     Relative period when --from is omitted, such as 24h, 7d, or 30d.
--from TIME     Start time, RFC3339, YYYY-MM-DD HH:MM:SS, or YYYY-MM-DD.
--to TIME       End time; defaults to now.
--out PATH      Markdown output path; defaults to stdout.
--token-id ID   Filter to one public router token id.
--token-id-prefix PREFIX
                Filter to public router token ids with this prefix.
--caller-project PROJECT
                Filter to one caller project.
--caller-environment ENV
                Filter to one caller environment.
--resolved-group GROUP
                Filter to one resolved router model group.
--client CLIENT Filter to one client, such as codex or claude-code.
```

Generate a markdown report for the last 24 hours:

```bash
./router-usage-report --db usage.sqlite --since 24h --out usage-24h.md
```

Generate a report from Postgres:

```bash
./router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out usage-24h.md
```

Generate a report for an explicit period and import existing JSONL first. Imports are duplicate-safe by router `request_id`:

```bash
./router-usage-report \
  --db usage.sqlite \
  --log requests.jsonl \
  --from 2026-06-14T00:00:00Z \
  --to 2026-06-15T00:00:00Z \
  --out usage-2026-06-14.md
```

Generate a report from a Docker Compose deployment:

```bash
dsn="$(sed -n 's/^ROUTER_USAGE_DB_DSN=//p' .env | tail -n 1)"
docker compose run --rm --entrypoint /app/bin/router-usage-report router \
  --driver postgres \
  --dsn "$dsn" \
  --since 24h \
  --out /app/logs/usage-24h.md
```

Generate a report for one benchmark or case study by caller project/environment:

```bash
./router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --caller-project harbor-algotune-pca \
  --caller-environment case-current-policy-20260615t004637z \
  --out harbor-agentic-usage.md
```

Reports include totals, external provider/model usage, internal router API key usage by `token_id`/user/project/environment, caller IP usage, hourly usage by caller IP, client usage, status codes, cache hit/miss/bypass, attempts, fallbacks, token totals, latency, downstream user performance, upstream provider/model/dialect performance, per-request upstream/downstream output-token/sec, per-request upstream/downstream total-token/sec, and cache occupancy snapshots. Raw router tokens and provider API keys are never written to the report.

Durability:

- Durable across container restarts when volumes are preserved: JSONL request logs, relational usage DB rows, per-request throughput fields, and per-request cache snapshots.
- Not durable across container restarts: in-memory response cache contents and in-process Prometheus counters/gauges.

## Make Targets

```bash
make test       # Go unit tests
make docs-build # build customer-facing Docusaurus docs into the Go embed directory
make docs-dev   # run the Docusaurus development server
make build      # build docs, then ./router, ./router-token-gen, and ./router-usage-report
make build-go-only # build Go binaries without refreshing embedded docs
make build-all  # build docs, then linux amd64 and linux arm64 binaries under dist/build
make package    # build linux amd64 and linux arm64 tarballs
make package-all # same as package
make docker-image # build one smart-llmrouter image for GOOS/GOARCH with docker buildx
make package-docker # build linux amd64 and linux arm64 Docker packages
make package-docker-all # same as package-docker
make e2e-mock   # local mock Claude/Codex C harness
make e2e-live-c # live OpenRouter :nitro C-generation e2e through Claude Code and Codex
make e2e-live-full # live provider HTTP cache checks plus live CLI C e2e
make e2e-compose-live # live provider + Claude/Codex checks through docker compose and Caddy
```

The Harbor agentic coding case-study example in `examples/harbor-algotune-pca/` uses `uv tool install harbor`, runs Harbor's `aider/polyglot_python_two-bucket` task through Codex CLI and Claude Code, and emits a markdown usage comparison report. Current production Harbor runs use one reusable Harbor caller token with access to the deployed model groups, then separate results by run matrix, client, model group, timestamps, and usage-report filters. The older per-`{agent, model_group}` token generator remains available only for isolated local or one-off investigations. The current production run is checked in at `docs/harbor-case-study.md`.

`make e2e-live-c` starts the router once per OpenRouter sample target, runs both local CLIs, extracts the generated C source, compiles it with `cc -std=c11 -Wall -Wextra -Werror`, and runs the binary. It reads the project `env.json` before invoking the router. To keep logs and generated C files:

```bash
KEEP_LIVE_E2E_WORKDIR=1 make e2e-live-c
```

To run one live case:

```bash
LIVE_E2E_CASE_REGEX=baseten-gpt-oss-120b make e2e-live-c
```

## CLI Smoke Tests

The following commands were tested locally with `Claude Code 2.1.177`, `codex-cli 0.139.0`, router port `18081`, and deployment-defined model groups. Provider-backed smokes require the relevant provider keys in the project `env.json`.

CLI install/update references:

```bash
# Codex CLI, official standalone installer/update path:
curl -fsSL https://chatgpt.com/codex/install.sh | sh

# Codex CLI, npm install/update path:
npm install -g @openai/codex@latest

# Claude Code, npm install/update path:
npm install -g @anthropic-ai/claude-code@latest
```

On 2026-06-14, the local installs matched the latest npm registry versions: `@openai/codex` `0.139.0` and `@anthropic-ai/claude-code` `2.1.177`.

Create a temporary router config and caller token:

```bash
make build

export WORK=/tmp/smart-llmrouter-readme-smoke
rm -rf "$WORK"
mkdir -p "$WORK"

./router-token-gen generate \
  --user readme \
  --project metrum-insights \
  --env dev \
  --allow cli-smoke \
  --format json > "$WORK/token.json"

python3 - <<'PY'
import json
import os
import shlex
from pathlib import Path

work = Path(os.environ["WORK"])
generated = json.loads((work / "token.json").read_text())
(work / "token.env").write_text(
    f"ROUTER_TOKEN={shlex.quote(generated['token'])}\n"
    f"ROUTER_MODEL=cli-smoke\n"
)
(work / "config.yaml").write_text(f"""server:
  listen: ":18081"
  cache: {{ enabled: false }}
  logging:
    path: {work}/requests.jsonl
state_path: {work}/state.json
providers:
  baseten:
    base_url: https://inference.baseten.co/v1
    dialect: openai-chat
    api_key: ${{BASETEN_API_KEY}}
    api_key_env: BASETEN_API_KEY
    key_id: baseten-readme-smoke
models:
  cli-smoke:
    strategy: static
    targets:
      - {{ provider: baseten, model: "openai/gpt-oss-120b" }}
callers:
  - id: readme-metrum-insights-dev
    user: readme
    project: metrum-insights
    environment: dev
    token_sha256: "{generated['token_sha256']}"
    token_id: "{generated['token_id']}"
    allow: ["cli-smoke"]
    rate: {{ rpm: 120, tpm: 200000, concurrent: 4 }}
    quota:
      day: {{ requests: 1000, tokens: 2000000 }}
      month: {{ tokens: 10000000 }}
      soft_pct: 80
    key: {{ lifetime_tokens: 10000000, soft_pct: 90, on_exhaust: disable }}
""")
PY
```

Start the router in one terminal. This form intentionally reads the project `env.json` for the smoke test, so a stale shell variable does not override the tested provider key:

```bash
export WORK=/tmp/smart-llmrouter-readme-smoke

OPENROUTER_API_KEY=$(python3 - <<'PY'
import json
from pathlib import Path
print(json.loads(Path("env.json").read_text())["OPENROUTER_API_KEY"])
PY
) ./router --config "$WORK/config.yaml"
```

Then run the CLI checks in another terminal:

```bash
export WORK=/tmp/smart-llmrouter-readme-smoke

set -a
. "$WORK/token.env"
set +a
```

Example hosted deployment model groups:

```text
small      Baseten GPT OSS 120B 58%, MiniMax-M3 28%, Gemma 4%, Kimi 4%, Baseten Nemotron 3%, Baseten GLM 2%, OpenAI GPT-5.4 Nano 1% non-tool.
medium     Baseten GPT OSS 120B 51%, MiniMax-M3 25%, Gemma 7%, Kimi 8%, Baseten Nemotron 3%, Baseten GLM 5%, OpenAI GPT-5.4 Nano 1% non-tool.
high       Baseten GPT OSS 120B 45%, MiniMax-M3 26%, Gemma 9%, Kimi 10%, Baseten Nemotron 3%, Baseten GLM 6%, OpenAI GPT-5.4 Nano 1% non-tool.
default    Baseten GPT OSS 120B 51%, MiniMax-M3 27%, Gemma 7%, Kimi 6%, Baseten Nemotron 3%, Baseten GLM 5%, OpenAI GPT-5.4 Nano 1% non-tool.
fast       Baseten GPT OSS 120B 56%, MiniMax-M3 26%, Gemma 4%, Kimi 5%, Baseten Nemotron 3%, Baseten GLM 5%, OpenAI GPT-5.4 Nano 1% non-tool.
big-coder  Code-heavy route: MiniMax-M3 30%, direct Kimi 23%, Crusoe Gemma 4 31B-it 20%, Baseten GPT OSS 120B 18%, Baseten GLM 6%, Baseten Nemotron 2%, OpenAI GPT-5.4 Nano 1% non-tool, plus tool-only OpenAI Chat/Responses/Anthropic-compatible fallbacks including Crusoe Gemma 4 31B-it for OpenAI Chat tool traffic.
```

The key used in `ROUTER_TOKEN` must allow the selected `ROUTER_MODEL`. The group names shown above are example hosted deployment names; your deployment can expose different names and access tiers.

### Claude Code

Claude Code uses Anthropic-style requests. For this router, set `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN` only. Do not set `ANTHROPIC_API_KEY` for router traffic; Claude Code uses that variable for direct Anthropic Console API keys via `X-Api-Key`, while this router expects a bearer token.

```bash
unset ANTHROPIC_API_KEY
export ANTHROPIC_BASE_URL="http://127.0.0.1:18081"
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"
export ANTHROPIC_MODEL="$ROUTER_MODEL"

claude --bare --print --model "$ROUTER_MODEL" \
  "Reply with exactly: router claude ok"
```

Expected output:

```text
router claude ok
```

Expected log fields include `client=claude-code`, `inbound_dialect=anthropic`, `requested_model=cli-smoke`, and a concrete target provider/model. Provider keys must not appear in output or logs.

Tool-capable smoke for Claude Code:

```bash
unset ANTHROPIC_API_KEY
mkdir -p "$WORK/claude-tool-work"
cd "$WORK/claude-tool-work"

ANTHROPIC_BASE_URL="http://127.0.0.1:18081" \
ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN" \
ANTHROPIC_MODEL="claude-tools-smoke" \
claude --bare --print --model claude-tools-smoke \
  --permission-mode bypassPermissions \
  --allowedTools "Write,Bash" \
  "Create claude_tool_smoke.txt containing exactly claude-tool-ok, run cat claude_tool_smoke.txt, then finish with claude-tool-ok."

test "$(cat claude_tool_smoke.txt)" = "claude-tool-ok"
```

OpenRouter-specific Claude Code tool smoke:

```bash
unset ANTHROPIC_API_KEY
mkdir -p "$WORK/claude-openrouter-tool-work"
cd "$WORK/claude-openrouter-tool-work"

ANTHROPIC_BASE_URL="http://127.0.0.1:18081" \
ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN" \
ANTHROPIC_MODEL="claude-tools-smoke-openrouter" \
claude --bare --print --model claude-tools-smoke-openrouter \
  --permission-mode bypassPermissions \
  --allowedTools "Write,Bash" \
  "Create claude_openrouter_tool_smoke.txt containing exactly claude-openrouter-tool-ok, run cat claude_openrouter_tool_smoke.txt, then finish with claude-openrouter-tool-ok."

test "$(cat claude_openrouter_tool_smoke.txt)" = "claude-openrouter-tool-ok"
```

### Codex CLI

Codex is configured with ephemeral provider settings and the OpenAI Responses wire API:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
mkdir -p "$WORK/codex-work"

codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  -C "$WORK/codex-work" \
  -c "model=\"$ROUTER_MODEL\"" \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:18081/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router codex ok" </dev/null
```

The `exec` subcommand is required for `--ignore-user-config`, `--ephemeral`, `--ignore-rules`, and `--skip-git-repo-check`; those flags are not accepted by the top-level interactive `codex` command.

For interactive Codex, omit the `exec`-only flags and run top-level `codex` with the same provider settings:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"

codex \
  -c "model=\"$ROUTER_MODEL\"" \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:18081/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"'
```

Expected final assistant output:

```text
router codex ok
```

Expected log fields include `client=codex`, `inbound_dialect=openai-responses`, `requested_model=cli-smoke`, and no leaked credentials. A local Codex installation may print a bubblewrap/user-namespace warning; that is separate from the router request and does not indicate provider failure.

Tool-capable smoke for Codex:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
mkdir -p "$WORK/codex-tool-work"

codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  --dangerously-bypass-approvals-and-sandbox \
  -C "$WORK/codex-tool-work" \
  -c 'model="agent-tools-smoke"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:18081/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Create codex_tool_smoke.txt containing exactly codex-tool-ok, run cat codex_tool_smoke.txt, then finish with codex-tool-ok." </dev/null

test "$(cat "$WORK/codex-tool-work/codex_tool_smoke.txt")" = "codex-tool-ok"
```

OpenRouter-specific Codex tool smoke:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
mkdir -p "$WORK/codex-openrouter-tool-work"

codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  --dangerously-bypass-approvals-and-sandbox \
  -C "$WORK/codex-openrouter-tool-work" \
  -c 'model="agent-tools-smoke-openrouter"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="http://127.0.0.1:18081/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Create codex_openrouter_tool_smoke.txt containing exactly codex-openrouter-tool-ok, run cat codex_openrouter_tool_smoke.txt, then finish with codex-openrouter-tool-ok." </dev/null

test "$(cat "$WORK/codex-openrouter-tool-work/codex_openrouter_tool_smoke.txt")" = "codex-openrouter-tool-ok"
```

Tool-bearing requests bypass the router response cache. They are intentionally routed to the provider every time because tool calls depend on external filesystem, shell, and agent state.

## Test

```bash
go test ./...
go build ./cmd/router
```

The automated suite uses deterministic mock upstreams. The Claude Code and Codex commands above are the live provider acceptance gates.

Full release validation is live and credit-consuming:

```bash
make e2e-live-full
make e2e-compose-live
```

These require live provider keys in `env.json` or the shell plus locally installed `claude`, `codex`, Docker, and Docker Compose.
