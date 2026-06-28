# Smart LLM Router

Go implementation of the Smart LLM Router described in `LLM_Router_SRS_1.docx`.

For an external-facing technical overview, architecture diagrams, feature summary, and configuration walkthrough, see [docs/solution-brief.md](docs/solution-brief.md). The customer-facing hosted documentation is built from `docs-site/` and embedded into release binaries under `/docs/`; browser requests to `/` redirect there. In source checkouts, internal documentation maintenance rules and the public/internal source-of-truth map live in [docs/DOCS_MAINTENANCE.md](docs/DOCS_MAINTENANCE.md).

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

Packaged Markdown is copied only from `scripts/package_docs_allowlist.txt`. `docs/DOCS_MAINTENANCE.md` is an internal source-checkout alignment runbook and is intentionally not packaged. Private production runbooks, private host details, SSH paths, live compose config paths, and raw token/provider-key patterns are blocked by package validation.

The `router` binary embeds the Docusaurus build output. At runtime, browser access to `/` redirects to `/docs/`; API and operations routes such as `/v1/*`, `/metrics`, `/admin/*`, `/healthz`, and `/readyz` keep precedence. Authenticated admin report assets, when enabled, are embedded separately under `/admin/reports/` and are not part of public Docusaurus docs.

## Documentation Map

Public product docs live under `docs-site/docs/` and are organized as a customer/operator journey: overview, getting started, installation, licensing, configuration, routing, providers and models, API compatibility, agents/tools/vision, usage and reports, security and governance, operations, troubleshooting, evaluation, commercial evaluation, competitive landscape, reference, and release/upgrade guidance. These pages must stay customer-safe: use placeholder endpoints, placeholder tokens, deployment-defined model-group examples, and no private hostnames, SSH details, raw secrets, token hashes, full configs, or internal production procedures.

Internal operator and maintainer docs live under `docs/`. Use [docs/DOCS_MAINTENANCE.md](docs/DOCS_MAINTENANCE.md) to decide which internal runbook owns each public section and which verification commands to run. Behavior changes affecting routing, auth, models, CLI/API usage, telemetry, deployment, licensing, security, or production operations normally require both public Docusaurus updates and matching internal/operator doc updates.

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
README.md
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
  --owner-user chetan \
  --project metrum-insights \
  --env dev \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Save the printed `token` value as the caller's bearer token, add the owner to `users`, add the project to `projects`, add an active `project_memberships` row, and copy the generated `callers:` key entry into `config.yaml`. Tokens use a traceable public prefix plus a random secret suffix, while the router stores only `token_sha256` and logs/exports only `token_id`. Identity and authorization come from the explicit account sections and the key's `owner_user`/`project` references, not from parsing the token prefix. Each user id, project id, caller `id`, `token_sha256`, and non-empty `token_id` must be unique after normalization; token hashes are checked case-insensitively. User, project, and membership statuses support `active`, `disabled`, `suspended`, `removed`, and `archived`; caller key statuses also support `expired` and `rotated`.

Provider keys are read from `env.json` in this project before `${VAR}` references in `config.yaml` are expanded. Real `env.json` is gitignored; use `env.example.json` as the placeholder-only template. Do not paste production or personal provider keys into tracked examples; store real values in ignored `env.json`, the shell environment, or your deployment secret manager. Run `make secret-check` before publishing changes that touch tracked env examples.

```bash
go run ./cmd/router --config config.yaml
```

If a variable is already set in the shell, the shell value wins over `env.json`. This lets CI or one-off live tests override local secrets without editing files.

In a packaged deployment, put provider keys in `config/env.json` beside `config/config.yaml`. The same loading rule applies: shell environment values win over `env.json`.

## License Enforcement

Normal release builds enforce offline signed JSON licensing under `server.license`. The router verifies a Metrum-issued license envelope with embedded Ed25519 public keys at startup and on `recheck_interval`, so operators can renew or replace `license.json` without rebuilding the binary. Runtime YAML cannot disable licensing in release builds; deployments should mount the license file read-only and keep the license state file under the deployment state directory.

```yaml
server:
  license:
    enabled: true
    path: /app/config/license.json
    state_path: /app/state/license-state.json
    instance_fingerprint: "issued-instance-fingerprint"
    recheck_interval: 1h
    grace_period_on_validation_error: 24h
```

`/readyz` fails when a required license blocks serving. Caller endpoints return documented `license-*` errors without exposing license payloads, signatures, or keys. Feature gates cover routing, usage reporting, admin reports, security reports, dynamic scoring, TypeScript routing, external policy routing, model-group contracts, retention rollups, and governed content-capture maintenance. Metrics-admin `/metrics` includes safe license gauges, and authorized admin report readers can query `/admin/license/status` for a safe summary only.

Set `instance_fingerprint` only when Metrum issues an instance-bound license for the deployment. It must match the licensed instance scope or startup/readiness will fail with `license-instance-limit-exceeded`.

Use `go run ./cmd/router-license safe-summary --license license.json` to inspect safe license metadata. `router-license verify --license license.json --public-key <public-key-file>` is for release/test validation with a supplied public key. Internal operators can use `router-license issue`, `renew`, and `top-up` with the SKU catalog and approved entitlement records. Private signing keys are not required at runtime and must never be copied into router config, logs, images, or source control.

When Metrum provides a signed revocation bundle, configure `server.license.revocation.mode: file` and mount the bundle at `server.license.revocation.path`. Effective `revoked`, `suspended`, or `superseded` entries block serving without license grace; `router-license revocation validate` and `revocation safe-summary` provide operator-safe verification.

Metrum-side license issuance, renewal, replacement, volume top-up, offline customer support, and acceptance checklists are documented in [docs/LICENSE_OPERATIONS.md](docs/LICENSE_OPERATIONS.md). That runbook is internal/operator guidance; public hosted docs describe customer installation and renewal behavior without signing-key or bypass details.

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
FIREWORKS_API_KEY
```

Provider adapter notes:
- `anthropic` targets call Anthropic Messages.
- `openai-chat` and `openai-responses` targets cover OpenAI-compatible API dialects. The current sample and production routes keep the active set intentionally small: Baseten `openai/gpt-oss-120b`, direct Moonshot Kimi `kimi-k2.7-code`, MiniMax `MiniMax-M3`, OpenRouter Gemma 4 26B Nitro capped at low ordinary-text weight where still configured in general groups, Baseten `nvidia/Nemotron-120B-A12B` at low text weight, Baseten `zai-org/GLM-5.2` for reasoning-heavy coding traffic, Crusoe `zai/GLM-5.2` in general ordinary-text routes, Crusoe `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B` as a text-only reference `big-coder` target where configured, Fireworks `accounts/fireworks/models/gpt-oss-20b` in the reference `big-coder` ordinary-text route where configured, Fireworks Responses `accounts/fireworks/models/kimi-k2p7-code` as a low-weight reference `big-coder` tool-only target where configured, and original OpenAI `gpt-5.4-nano` at low non-tool weight.
- Crusoe Managed Inference is configured as an external hosted OpenAI-compatible `openai-chat` provider with `base_url: https://api.inference.crusoecloud.com/v1` and `api_key_env: CRUSOE_API_KEY`. Keep unvalidated Crusoe models catalog-only or in dedicated smoke groups until the exact account, model ID, tool behavior, structured-output behavior, output-cap behavior, and router usage/cost fields have passed. The reference general groups include Crusoe `zai/GLM-5.2` for ordinary text after a direct text smoke, and the reference `big-coder` route includes Crusoe `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B` for text-only ordinary traffic where configured. Crusoe Gemma 4 31B-it remains cataloged and available for dedicated smoke validation, but it is not described as an active/reference `big-coder` target.
- Fireworks AI is configured as external hosted OpenAI-compatible providers with `base_url: https://api.fireworks.ai/inference/v1`, `api_key_env: FIREWORKS_API_KEY`, bearer auth, and an explicit `User-Agent` header. Fireworks Chat `accounts/fireworks/models/gpt-oss-20b` passed direct text, streaming, `max_tokens: 1`, `reasoning_effort` low/medium/high, OpenAI Chat auto tools with `max_tokens >= 256`, forced `tool_choice`, and JSON schema structured-output smokes on 2026-06-27. Fireworks Responses `accounts/fireworks/models/kimi-k2p7-code` passed direct and router-level text, function-tool, continuation, streaming, cap, and `store:false` smokes on 2026-06-28. The Responses target uses `force_store_false: true`; router requests with provider-hosted `mcp` or `sse` tools return `400 provider-hosted-tools-forbidden` before upstream. Keep other Fireworks models catalog-only until the exact account, model ID, entitlement, pricing, cap behavior, router-level smokes, and workload gates pass. Do not claim Fireworks Anthropic Messages, image, video, or audio support until those exact skins pass separately.
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

Raw caller tokens, caller token hashes, and raw provider API keys are not exposed to TypeScript routing scripts, logs, metrics, or responses. Scripts get safe identifiers only: caller `id`, legacy-compatible `user`, canonical `ownerUser`/`username`, `project`, `environment`, public `tokenId`, membership role, key status, and target `keyId`, `apiKeyEnv`, and `keyConfigured`. This is enough to route by validated owner/project metadata or by the configured provider key name without making secrets available to script code.

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
        pricing_notes: Crusoe Gemma 4 31B-it OpenAI Chat smokes passed for text, streaming, max_tokens=1, auto tool, forced tool_choice, structured outputs, and combined tool plus structured-output requests on 2026-06-24. Keep it cataloged or in dedicated smoke groups unless the exact deployment workload passes again; do not describe it as an active/reference big-coder target, and do not use it for OpenAI Responses, Anthropic Messages, vision, or broad tool routing unless those exact skins pass direct and router-level smokes separately.
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
  fireworks:
    base_url: https://api.fireworks.ai/inference/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${FIREWORKS_API_KEY}
    api_key_env: FIREWORKS_API_KEY
    key_id: fireworks-default
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
        pricing_notes: Fireworks also publishes a $0.035/M cached-input rate for GPT OSS 20B. Direct OpenAI Chat text, streaming, max_tokens=1, reasoning_effort low/medium/high, auto tool with max_tokens >= 256, forced tool_choice, and structured-output smokes passed on 2026-06-27 with an explicit User-Agent; this ID completed even though it was not listed by /models for the validated account.
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum
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
      - { provider: openrouter, model_ref: gemma-4-26b-a4b-it-nitro, weight: 2 }
      - { provider: crusoe, model_ref: glm-5-2, weight: 5 }
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

Crusoe Managed Inference is an external hosted OpenAI-compatible provider. Crusoe documentation checked on 2026-06-24 lists `https://api.inference.crusoecloud.com/v1` as the OpenAI-compatible base URL, documents API keys from the Intelligence Foundry console, and uses `meta-llama/Llama-3.3-70B-Instruct` in the quickstart. Crusoe's 2026-04-28 Nemotron 3 Nano Omni announcement describes Nemotron 3 Nano Omni 30B A3B Reasoning as a multimodal model for document, GUI-agent, video/audio, and text reasoning workloads with a 256K-token context: `https://www.crusoe.ai/resources/blog/nvidia-nemotron-3-nano-omni-now-available`. A direct `/v1/models` check on 2026-06-24 required an explicit `User-Agent` and returned exact IDs such as `openai/gpt-oss-120b`, `google/gemma-4-31b-it`, `meta-llama/Llama-3.3-70B-Instruct`, `zai/GLM-5.2`, `moonshotai/Kimi-K2.6`, `Qwen/Qwen3-235B-A22B-Instruct-2507`, and `nvidia/NVIDIA-Nemotron-3-Super-120B-A12B`; a 2026-06-25 check also found `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B`. Direct and local router-level Llama text, streaming, `max_tokens: 1`, auto tool, forced tool-choice, and structured-output smokes passed on 2026-06-24. Direct Crusoe Gemma 4 31B-it OpenAI Chat text, streaming, `max_tokens: 1`, auto tool, forced tool-choice, structured-output, and combined tool plus structured-output smokes passed on 2026-06-24, but production Cursor/opencode traffic later showed repeated upstream 400s for the active `big-coder` route. Direct Crusoe `zai/GLM-5.2` text smoke passed on 2026-06-24; a `max_tokens: 1` probe honored the cap but returned empty content with `finish=length`, so the reference config uses it only for ordinary text routing and does not claim Crusoe GLM tool or structured-output support. Crusoe Nemotron 3 Nano Omni 30B A3B Reasoning text smoke passed on 2026-06-25 and it accepted receipt-image requests, but it did not pass the OCR acceptance gate, so the active `big-coder` target overrides `input_modalities: [text]` and the model stays out of broad active `vision` routing. The reference config keeps OpenRouter Gemma 4 26B Nitro ordinary-text weight at no more than 2% in general groups, substitutes the reduced weight with Crusoe GLM 5.2, and uses Crusoe Nemotron 3 Nano Omni 30B A3B Reasoning at 15% in the `big-coder` ordinary-text route after adding Fireworks GPT OSS 20B at 15%. Do not claim Crusoe OpenAI Chat tools, OpenAI Responses, or Anthropic Messages support unless those skins are separately exposed and validated.

Fireworks AI is an external hosted OpenAI-compatible provider. Fireworks documentation checked on 2026-06-28 lists `https://api.fireworks.ai/inference/v1` as the OpenAI-compatible base URL, `FIREWORKS_API_KEY` authentication, account-qualified model IDs, Serverless pricing, and a Responses API with function tools, provider-hosted MCP/SSE tools, streaming, `max_tool_calls`, and `store=false`. A direct `/models` check on 2026-06-27 returned account-visible IDs such as `accounts/fireworks/models/gpt-oss-120b`, `accounts/fireworks/models/deepseek-v4-pro`, `accounts/fireworks/models/kimi-k2p6`, and `accounts/fireworks/models/glm-5p2`; `accounts/fireworks/models/gpt-oss-20b` was not listed but completed successfully. Direct Fireworks `accounts/fireworks/models/gpt-oss-20b` OpenAI Chat text, streaming, `max_tokens: 1`, `reasoning_effort` low/medium/high, auto tool with `max_tokens >= 256`, forced `tool_choice`, and JSON schema structured-output smokes passed with an explicit `User-Agent`. Fireworks Responses `accounts/fireworks/models/kimi-k2p7-code` passed direct and router-level text, function-tool, function-call-output continuation, streaming tool, `max_output_tokens: 1`, `max_tool_calls: 1`, and `store:false` smokes on 2026-06-28, so the reference config exposes it through a separate `fireworks_responses` provider, dedicated smoke groups, and a conservative low-weight `big-coder` tool-only target. Fireworks Responses candidates `glm-5p2`, `deepseek-v4-flash`, and `qwen3p6-plus` passed function-tool continuation but remain unactivated pending workload validation; `gpt-oss-20b` is not activated for Responses tools because the tool-result continuation probe returned unrelated incomplete content. Provider-hosted Fireworks `mcp`/`sse` tool requests are rejected by the router before upstream. Fireworks returns reasoning content alongside visible content in some paths, so usage and acceptance checks should use realistic budgets and inspect final content and token totals. Fireworks Anthropic Messages, image, video, and audio remain disabled until separately validated.

OpenAI Chat tool passthrough is used by OpenAI-compatible agent clients such as Warp Agent. These clients call `/v1/chat/completions`, send `tools`, `tool_choice`, and often request streaming. For those requests, the router preserves the OpenAI Chat tool payload and tool-result messages, selects only upstream targets with explicit `tool_support.openai_chat`, calls the upstream non-streaming, and returns either the raw non-streaming response or synthesized OpenAI Chat SSE chunks containing `delta.tool_calls`. This avoids asking users to switch model groups just because a coding-agent turn includes tools; the configured group filters to compatible targets automatically.

Image requests are detected across OpenAI Chat, OpenAI Responses, and Anthropic Messages content blocks. The router filters image-bearing requests to targets with `image` in `input_modalities`, skips text-only targets, bypasses response caching, and logs `input_has_image`, `input_image_count`, upstream image-token counts when reported, calculated image cost, and upstream-reported billed cost when available.

Coding-agent groups should not force users to switch between a language model group and a vision model group during one task. Add validated multimodal `tool_only` targets to deployment-defined coding groups, for example the reference `big-coder` group, for Codex Responses and Claude Code Anthropic Messages traffic. Text-only requests continue to use the normal weighted coding targets; image-bearing agent requests automatically filter to multimodal tool-capable targets.

Vision catalog entries are not automatically active routes. For example, OpenRouter `qwen/qwen3-vl-32b-instruct:nitro` can be cataloged with `input_modalities: [text, image]` and current OpenRouter pricing, but it should remain out of active OCR routes until a router-level receipt OCR smoke returns the expected merchant name. For general VLM routing, distinguish image processing from OCR-quality gating and keep models that produce weak or inconsistent image analysis out of broad active groups until the deployment's acceptance tests pass.

Direct OpenAI `gpt-5.4` is cataloged as a validated vision target after 2026-06-25 Responses text and receipt-image OCR smokes returned `OK` and `Rite Aid` for the current project. The hosted reference `vision` group uses it to reduce OpenRouter-hosted Anthropic weight. Do not promote cheaper direct OpenAI or Kimi/MiniMax candidates on price alone: on 2026-06-25, `gpt-5-nano`, `gpt-5-mini`, `gpt-4.1-nano`, `gpt-4o-mini`, and `gpt-5.4-nano` did not pass the receipt OCR gate, `gpt-5.4-mini` was not enabled for the project, direct Kimi only passed with data-URL images while rejecting normal image URLs, and MiniMax-M3 OCR was inconsistent across repeated prompts.

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
      - { provider: crusoe, model_ref: nemotron-3-nano-omni-reasoning-30b-a3b, weight: 20, input_modalities: [text] }
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

When decision telemetry is enabled, usage/admin reports expose safe dynamic-score, policy, fallback, and admission buckets for operations: enabled signal names, score/value/final-score buckets, threshold/filter buckets, policy outcomes/error classes, fallback transition reasons, max-token cap filtering, max-token buckets, input-token buckets, and quota/admission reasons. Request usage rows also store non-secret routing/model-group/policy/pricing fingerprints for reproducibility after config or pricing changes. Daily rollups preserve report-critical buckets in normalized scalar rows so commercial reports can outlive raw request-level detail retention.

Rollout should start on a deployment-defined test group with interchangeable validated targets. Use mock or local router smokes for simple text, code/debug prompts, tool calls, forced tool calls, image requests when supported, structured-output requests when supported, and low output caps for each caller API. Roll back by switching the group strategy to `weighted` or by removing score terms and thresholds that are too strict for the workload.

For structured-output rollout, smoke both Chat Completions `response_format` and Responses `text.format` if both dialects are configured. Also run a negative router smoke against a group with no structured-output-capable target and expect `502 no-eligible-target` with no upstream attempt. If a target claims both tools and structured outputs, include a combined request in rollout validation. Streaming clients should be told whether the router is returning provider-native streaming or synthesizing downstream SSE from a unary upstream call; schema-constrained incremental chunks are provider-specific and not guaranteed by the router.

For reasoning routing, see the Docusaurus [Reasoning Routing](docs-site/docs/configuration/reasoning-routing.md) guide and the operator [Smoke Test Matrix](docs/SMOKE_TEST_MATRIX.md). Explicit OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, and Anthropic Messages `thinking` requests must use targets with validated reasoning metadata inside the requested group; ordinary traffic can still use the group's ordinary eligible weighted mix.

For OpenAI Chat tool clients, for example Warp Agent, configure the client with:

```text
Base URL: https://your-router.example.com/v1
API key: <router caller token>
Model: <allowed-model-group>
```

Use whichever deployment-defined model group the caller token allows. If a request includes `tools`, structured-output fields, images, or explicit output caps and no eligible target in that group declares the required support, the router returns `502 no-eligible-target` with a hint to enable an upstream target that supports the requested dialect, tools, structured outputs, modalities, and cap behavior.

### Model Group Contracts

Model groups may declare an optional `contract` that makes the group’s workload, API surfaces, hard capability requirements, validation quality floor, and operational thresholds first-class config. Existing groups without a contract behave as before. Contract enforcement is strictly group-local: after authentication and caller allow-list checks, the router filters only the requested group’s already eligible targets, then runs `static`, `weighted`, `failover`, `dynamic_score`, `script`, or `external` on the remaining targets.

```yaml
models:
  support-chat:
    strategy: weighted
    contract:
      display_name: Support chat
      caller_visible_notes: Deployment-defined low-latency support group.
      intended_workloads: [support_chat]
      supported_api_shapes: [openai_chat]
      required_capabilities:
        input_modalities: [text]
        output_modalities: [text]
        honors_max_tokens_when_caller_capped: true
      quality_floor:
        require_tags: [validated]
        min_eval_quality_score: 0.90
        min_eval_pass_rate: 0.95
        max_eval_age_days: 30
        allowed_validation_status: [passed]
      operational_targets:
        max_p95_latency_ms: 10000
        max_error_rate: 0.03
        max_timeout_rate: 0.02
      reporting:
        expose_workload_labels: true
        expose_quality_floor_bucket: true
    targets:
      - provider: private-gpu
        model_ref: support-balanced
        weight: 70
        tags: [validated, low_latency]
        validation:
          status: passed
          workload: support_chat
          validated_at: "2026-06-25"
          quality_score: 0.94
          pass_rate: 0.98
          harness: golden-support-set
      - provider: hosted
        model_ref: support-fallback
        weight: 30
        tags: [validated, fallback]
        validation:
          status: passed
          workload: support_chat
          validated_at: "2026-06-25"
          quality_score: 0.92
          pass_rate: 0.96
          harness: golden-support-set
```

Startup validation rejects unsupported API shapes, invalid modalities, impossible validation status values, bad dates, out-of-range quality scores/pass rates, negative thresholds, required tags that no target has, declared API shapes that no target serves, and contracts no target can satisfy. Runtime contract failures return the existing `502 no-eligible-target` style response with safe buckets such as `contract-required-api-shape`, `contract-required-modality`, `contract-quality-floor`, `contract-validation-expired`, or `contract-no-validated-target`.

`dynamic_score` can use target `tags` and `validation.quality_score`/`validation.pass_rate` as evaluation hints. TypeScript and external policy strategies receive the same safe contract and target validation metadata after contract filtering, and returned decisions are validated against the filtered target list. Usage rows store only scalar contract buckets, optional workload labels, and target validation status/workload/age buckets.

Roll out contracts on a deployment-defined test group first. Add validation metadata to each intended target, run text/tool/image/structured-output smokes that match the declared contract, confirm no-eligible failures use safe reason buckets, and verify reports show only safe scalar buckets. Roll back by removing or relaxing the `contract` block, removing a too-strict quality floor, or switching the group back to its previous strategy/weights.

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

The script context uses top-level `ctx.text` for normalized request text, plus `ctx.group`, `ctx.request`, `ctx.caller`, and `ctx.targets`. Caller metadata includes `id`, legacy-compatible `user`, canonical `ownerUser`/`username`, `project`, `environment`, public `tokenId`, `membershipRole`, `keyStatus`, and the key allow list. Target metadata includes provider, model, modelRef, baseUrl, dialect, weight, keyId, apiKeyEnv, and keyConfigured. For groups with `pii_filter`, `ctx.text`, normalized request fields, and `ctx.request.raw` are redacted before the script runs, and placeholder mappings are not exposed. Raw provider API keys, raw caller tokens, and caller token hashes are never passed to scripts; returned targets are validated against the configured list. Scripts run synchronously inside the router process, so keep policy local and fast; unrestricted network calls and file access are not part of the script runtime.

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

Use `strategy: external` when routing policy should live in a standalone web service instead of in TypeScript. The router sends safe derived request context, safe caller metadata, safe contract metadata when configured, eligible target metadata, validation metadata, pricing, tool capability metadata, and modalities to the configured policy URL, then validates the returned target against the model group's eligible targets. By default the policy payload does not include prompt text, message bodies, image URLs/data, tool schemas, tool outputs, or `request.raw`; route on fields such as `context.textChars`, `context.estimatedTokens`, `context.imageCount`, and `context.toolCount`. Set `external_policy.include_request: true` only for a trusted policy service that is allowed to receive request content. With `pii_filter`, that opt-in request mirror is redacted before dispatch and placeholder mappings are not sent. Raw router tokens, token hashes, and provider API keys are never sent.

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

Policy responses use the same selector shape as TypeScript: `targetIndex` or `target`, optional `fallbackIndexes`/`fallbacks`, and optional `classLabel`. When a TypeScript script omits `fallbackIndexes` and `fallbacks`, remaining eligible targets are used as retries; when either field is present, the supplied entries are the complete retry set. The default `on_error` behavior is `fail_closed`, returning `502 routing-policy-error`; `fallback` can be configured when the target order is an acceptable default. A runnable demo service lives at `examples/external-routing-policy/prompt_size_policy.py`.

External policy URLs use the same egress rules as `router.fetchJSON`: HTTPS by default, plaintext HTTP only for loopback hosts or with `external_policy.allow_http: true`, exact-host allowlisting, and redirect revalidation on every hop. A redirect to a host outside `allow_hosts`, including a loopback address that was not explicitly allowed, fails before the redirected service is reached.

The default `scripts/router.ts` does three things:

- Removes targets whose provider key is not configured or whose target weight is zero.
- Applies named regex rules against safe caller-key metadata and safe target-key metadata.
- Falls back to weighted random routing across eligible targets, using group target weights as relative probabilities.

## PII Filtering

Model groups can configure `pii_filter` rules to replace matched text with typed placeholders before target selection, cache-key generation, routing-policy inputs, and upstream provider calls. The redacted request object is the source of truth for TypeScript script `ctx.request.raw` and for external policy `request`/`text` only when `external_policy.include_request: true` is explicitly enabled; external policy services otherwise receive safe derived context without raw request mirrors. Modes support `redact_only`, `redact_and_restore`, and `fail_on_match`. Usage logs and the usage database store only safe scalar metadata such as applied flag, mode, replacement count, and matched-rule count; raw matched values and placeholder mappings remain in memory for the request lifecycle by default.

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

Caller metadata enables owner-, project-, environment-, or key-class routing without exposing secrets. Target metadata also lets the script route to targets backed by a specific configured provider key identifier or environment variable name:

```ts
export function route(ctx) {
  if (
    /^chetan$/.test(ctx.caller?.ownerUser || ctx.caller?.username || "") &&
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

`ctx.caller.ownerUser` and `ctx.caller.project` come from validated config references. `ctx.caller.tokenId` is the generated public token id without the secret suffix, for example `rtr_metrum_chetan_metrum-insights_prod_key1`; use it for traceable key classes, not identity. Do not route on raw token secrets; the router never passes them to scripts.

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

`/metrics` is intentionally restricted to caller subjects authorized for `metrics` `read`; existing caller entries with `metrics_admin: true` receive equivalent Casbin grants at startup. Normal application keys receive `403 metrics-forbidden`. Use `/v1/usage` and durable usage reports for caller-scoped usage views.

Browser-admin HTTP Basic authentication is configured under `server.admin_auth.basic` and is disabled by default. When enabled, `GET /admin/auth/check` validates the first admin identity path: missing or invalid Basic credentials receive `401`, valid credentials without the stub permission receive `403 admin-forbidden`, and valid credentials with `admin:auth:read` receive safe subject metadata. Basic Auth establishes identity such as `basic:admin`; it does not grant broader admin permissions by itself. See [docs/ADMIN_AUTH.md](docs/ADMIN_AUTH.md).

Administrator browser reports can be enabled under `server.admin_reports` and are served under `/admin/reports/`. They require browser-admin identity plus Casbin policy under `server.admin_auth.authorization`, expose safe usage/performance/cost/savings/cache/fallback/routing/capability/anomaly/troubleshooting/request-drilldown data through a Metrum-branded dark dashboard with filters for caller ID, caller IP, requested model, provider/model/dialect, status, cache, project, group, and client, plus shared search, sorting, URL state, CSV export, and a safe version chip populated from `/admin/reports/api/version`. The browser also includes safe provider catalog/validation status and retention/rollup status tabs, and keeps Markdown export available at `/admin/reports/export.md`. Optional security access reports under `server.admin_reports.security` persist safe scalar access events and require separate `admin:security_reports` policy. Savings baselines under `server.admin_reports.baselines` are source-dated hypothetical comparison prices; actual cost is summed from stored request-time usage rows. See [docs/AUTHORIZATION.md](docs/AUTHORIZATION.md) for metrics, content-capture maintenance, security report, and report policy examples. Ordinary router caller tokens receive `403 reports-forbidden`.

Authorization policy can come from a deployment-owned file/inline config with `server.admin_auth.authorization.source: static`, or from a validated active policy set in the usage DB with `source: db`. DB-backed policy mode fails closed when no single valid active policy set exists, keeps static policy support intact, and records create, activation, rollback, and validation-failure audit events with safe scalar fields only.

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

When license enforcement is enabled, request logs and `request_usage` store only safe scalar license metadata such as status, reason, license ID, customer ID, SKU, key ID, expiry, and grace-active flag. They do not store the license payload, detached signature, public/private key bytes, or signing material.

Diagnostics are written alongside usage when `server.diagnostics.enabled` is true. Each request can have child rows in `request_attempts`, `request_trace_events`, and `request_errors`, all keyed by `request_id`. Use the `X-Request-Id` response header or the `request_id` in an error body to join these rows during incident response. Diagnostic rows store provider/model/status/timing/error-class data, not raw prompts, images, bearer tokens, provider keys, token hashes, full upstream headers, or raw upstream response bodies. `store_sanitized_upstream_errors` can keep bounded sanitized error context for troubleshooting, but it is not content capture and still redacts prompt-like fields, nested upstream bodies, and secret-shaped values before JSONL or usage DB persistence.

Governed content capture is a separate opt-in feature under `server.content_capture`. It is disabled by default and writes redacted request, response, and upstream-error content to `request_content_captures` plus allowlisted headers in `request_content_headers`, both joinable to `request_usage` by `request_id`. Admin delete and retention purge write `request_content_audit_events` and require a caller subject authorized for `content:capture` `delete|purge`; existing caller entries with `content_admin: true` receive equivalent Casbin grants at startup. Metrics-admin tokens do not grant content maintenance access. The first slice always requires `redact_before_storage: true`, rejects forbidden header names such as authorization/API-key/token headers, and rejects `encryption.enabled: true` until KMS-backed encryption is implemented.

Commercial retention policy is configured under `server.retention`. Defaults are conservative with `dry_run: true`; status jobs store active policy versions and rules, legal-hold rows, retention jobs, and per-table counts for `usage_diagnostics`, `decision_telemetry`, `security_access_events`, `content_capture`, and `usage_detail`. Legal holds match by `data_class`, optional `request_id`, and timestamp range. When a reviewed config sets `dry_run: false`, `router-usage-report --retention-run` deletes at most one configured batch per supported table for `usage_diagnostics` and `usage_detail`; other data classes are counted and recorded as blocked. `usage_detail` deletion is blocked unless finalized daily usage rollups continuously cover the candidate window, preserving immutable #122 rollup history. Archive/export, scheduler, admin UI/API workflows, and broader data-class purge execution remain future slices.

Upstream timing is configurable with `server.upstream.timeout_ms`, `server.upstream.default_attempt_timeout_ms`, model-group `attempt_timeout_ms`, and per-target `timeout_ms`. A target timeout overrides a group timeout, and a group timeout overrides the global default attempt timeout. `0` disables the per-attempt cap while preserving the global HTTP client timeout. If all eligible attempts fail, exhausted upstream timeouts return `504 upstream-timeout`, provider rate limits return `503 upstream-rate-limited`, provider balance/credit/quota/billing exhaustion returns `503 upstream-quota-exhausted`, and other exhausted upstream failures return `502 upstream-failed`. Fallbacks are still attempted first; if a later target succeeds, the caller receives the successful model response while usage diagnostics retain the failed attempt class.

Each request row stores the configured input/output price per million tokens for the selected upstream model, the pricing source/update date, and calculated input/output/total USD cost. These values are logged at request time instead of recalculated during reporting, so historical cost reports remain stable after upstream providers change pricing.

The JSONL file is useful for raw audit/debugging. The relational DB is the source for periodic reports. In container deployments using SQLite, use `/app/logs/requests.jsonl` and `/app/state/usage.sqlite`. In Postgres deployments, the report tool reads from the configured DSN.

For routine browser inspection, deployments may enable `/admin/reports/`. The browser report UI is disabled by default, embedded in the router binary, uses local Metrum logo/font/chart assets, shows authenticated build metadata from `/admin/reports/api/version`, and calls bounded JSON APIs over the same relational usage DB. The CLI remains the supported path for automation, incident exports, and headless workflows.

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
--caller-user USER
                Filter to one caller owner user.
--caller-project PROJECT
                Filter to one caller project.
--caller-environment ENV
                Filter to one caller environment.
--resolved-group GROUP
                Filter to one resolved router model group.
--client CLIENT Filter to one client, such as codex or claude-code.
--rollup        Generate relational rollup rows for the selected period instead of markdown.
--rollup-type TYPE
                Rollup granularity: hourly, daily, or monthly. Defaults to daily.
--rollup-finalize
                Mark the selected rollup window immutable after generation.
--baseline-id ID
                Optional savings baseline id to store on rollup rows.
--baseline-name NAME
                Optional savings baseline name to store on rollup rows.
--baseline-version VERSION
                Optional savings baseline version/source date to store on rollup rows.
--baseline-input-price-per-million-usd USD
                Optional baseline input price in USD per million tokens.
--baseline-output-price-per-million-usd USD
                Optional baseline output price in USD per million tokens.
--retention-status
                Record a dry-run retention status job from --config.
--retention-run Run retention from --config; deletes one batch per supported table only when config dry_run=false.
--config PATH   Router config path for --retention-status or --retention-run.
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

Record a dry-run retention status job from reviewed router config:

```bash
./router-usage-report --retention-status --config config.example.yaml
```

Run one reviewed retention batch after finalized rollups and legal holds have been checked:

```bash
./router-usage-report --retention-run --config config.production.yaml
```

Reports include totals, external provider/model usage, internal router API key usage by `token_id`/owner user/project/environment, caller IP usage, hourly usage by caller IP, client usage, status codes, cache hit/miss/bypass, attempts, fallbacks, token totals, latency, downstream user performance, upstream provider/model/dialect performance, per-request upstream/downstream output-token/sec, per-request upstream/downstream total-token/sec, contract pass/fail buckets, optional contract workload labels, target validation buckets, and cache occupancy snapshots. Raw router tokens and provider API keys are never written to the report.

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
  --owner-user readme \
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
users:
  - id: readme
    name: README Smoke
    type: service_account
    status: active
projects:
  - id: metrum-insights
    name: Metrum Insights
    status: active
project_memberships:
  - user_id: readme
    project: metrum-insights
    role: developer
    status: active
callers:
  - id: readme-metrum-insights-dev
    owner_user: readme
    project: metrum-insights
    environment: dev
    status: active
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
big-coder  Code-heavy route: MiniMax-M3 25%, direct Kimi 20%, Baseten GPT OSS 120B 17%, Crusoe Nemotron 3 Nano Omni Reasoning 15% text-only, Fireworks GPT OSS 20B 15%, Baseten GLM 5%, Baseten Nemotron 2%, OpenAI GPT-5.4 Nano 1% non-tool, plus separately validated tool-only OpenAI Chat/Responses/Anthropic-compatible fallbacks.
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
