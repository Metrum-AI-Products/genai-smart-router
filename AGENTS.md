# Smart LLM Router Agent Instructions

These instructions apply to the whole repository.

## Project Shape

- Go router implementation lives under `internal/router` and CLI entrypoints live under `cmd/`.
- Main checked-in sample config is `config.example.yaml`.
- Local real provider keys are in ignored `env.json`.
- Local production snapshot is ignored `config.production.yaml`; keep it synced with the deployed config when production changes.
- Deployment notes are in `deployment.md` and `docs/DOCKER_DEPLOYMENT.md`, but always verify live production state before acting.

## Core Rules

- Use `rtk` before shell commands in this repo.
- Do not print provider API keys, router tokens, token hashes, or full production config contents.
- Do not commit `env.json`, `config.production.yaml`, `ROUTER_TOKEN*.txt`, generated logs, DBs, or `dist/`.
- Provider model catalogs are metadata only. Routing weights belong only under `models.<group>.targets[]`.
- Provider model catalogs must include current `input_price_per_million_usd`, `output_price_per_million_usd`, `pricing_source`, and `pricing_updated_at` for every active or cataloged upstream model when pricing is known. Use current primary/provider docs when possible; use OpenRouter model metadata for OpenRouter-hosted routes. For VLMs, add `image_input_price_per_million_tokens_usd` or `image_input_price_per_image_usd` only when the upstream/provider or enterprise chargeback model uses separate image pricing. For self-hosted models, use the enterprise chargeback rate or explicit `0.00` with `pricing_notes`.
- Provider model catalogs must include `input_modalities` and `output_modalities` for active vision, video, audio, or other multimodal targets. Do not mark a target with `image` until a direct upstream image smoke and a router-level image smoke pass for the exact provider/model/dialect/skin.
- Tool capability metadata belongs in provider catalogs as `tool_support` and must be based on a real direct upstream smoke plus router-level smoke for the exact dialect/skin. Do not claim `openai_chat`, `openai_responses`, `anthropic_messages`, or `provider_hosted` support from marketing copy alone.
- Usage persistence uses GORM. Keep the entire usage DB schema purely relational: no JSON/JSONB columns, no array columns, no serialized blobs for structured data, and no packed multi-value text fields. If one request needs multiple related rows, add a child table with scalar columns and a foreign key to `request_usage`.
- Usage rows store request-time cost inputs, image/VLM event fields, calculated costs, and upstream-reported billed costs as scalar columns. Reports must sum stored cost values, not recalculate historical cost from current config.
- Do not put unavailable provider models into active routing. Catalog-only is acceptable when a model exists but the current key is not entitled.
- Do not put unavailable provider models into active routing. The 2026-06-17 production policy keeps active tool-capable routes on OpenRouter, MiniMax, and Kimi/Moonshot models that passed Harbor/tool validation. Original OpenAI `gpt-5.4-nano` is allowed at low non-tool fallback weight; do not use `gpt-5.5` in active routing. Original Anthropic remains catalog/support-only until `ANTHROPIC_API_KEY` is present and a live smoke passes.
- Prefer structured YAML/JSON parsing for config changes. Avoid fragile text edits for production config.
- Keep sample config, local production snapshot, production config, docs, and tests in sync for behavior changes.
- No stale docs. Before finishing any task that changes behavior, config, deployment, models, auth, CLI usage, tests, or production, search the repo for old names/status and update every matching doc or fixture. If a doc cannot be made current, mark the exact section as historical with a date and reason.
- Documentation updates must cover both audiences when a behavior affects routing, auth, models, CLI/API usage, telemetry, deployment, or production operations:
  - Admin/internal docs (`README.md`, `docs/`, `deployment.md`, scripts, and config comments) must explain configuration, validation, rollout, rollback, and operational impact.
  - Proxy-user docs (`docs-site/`, embedded under `/docs/`) must explain what callers request, what behavior they can expect from the proxy, and any client-facing examples without internal deployment details.
- Customer-facing hosted docs live in `docs-site/` and are embedded into release binaries under `/docs/`. Keep public docs free of raw provider keys, real router tokens, private host paths, SSH details, and internal-only deployment notes. Route interested readers to `mailto:contact@metrum.ai`.
- TypeScript routing changes require both admin and proxy-user docs. Include the script context shape, model-group configuration, caller-visible behavior, and at least one tested example when documenting a new script policy pattern.
- Self-hosted upstream changes or examples require both admin and proxy-user docs. Cover enterprise-hosted vLLM/SGLang-style OpenAI-compatible services, private `/v1` base URLs, served model IDs, parser/chat-template requirements, tool-call behavior, caller-visible model groups, direct upstream smokes, router smokes, and rollback/operational notes. Verify current upstream documentation online before documenting vLLM, SGLang, or similar fast-moving serving frameworks.
- Public API examples in `docs-site/` must be tested before deployment. For Python examples, use `uv` in an ignored temporary project under `tmp/`, run the exact documented dependency/install flow, and keep docs generic with placeholder router tokens.

## Development Workflow

1. Inspect current state:
   - `rtk git status --short`
   - `rtk rg -n "<term>" config.example.yaml internal docs README.md scripts`
   - For production-impacting work, inspect `config.production.yaml` and the live host summary.
2. Make scoped edits using `apply_patch`.
3. Run formatting when Go files change:
   - `rtk gofmt -w <go files>`
4. Run tests:
   - `rtk go test ./...`
5. For provider/model changes, run direct live provider smoke tests with the relevant key from `env.json` before activating the model in routing.
6. For router behavior changes, run a router-level smoke test locally or against production, depending on the requested scope.
7. For pricing/tool/modality metadata changes, verify current pricing/capability docs online, update `config.example.yaml`, ignored `config.production.yaml`, production config when requested, tests, and public/internal docs together.
8. Update docs for any user-facing config, model, deployment, CLI, or operational change.
9. Run a stale-doc search for changed concepts before final response. Examples:
   - `rtk rg -n "old-model|old-provider|old-image-tag" README.md docs deployment.md config.example.yaml internal scripts`
   - `rtk rg -n "MiniMax-Text-01|text-01|openrouter/pareto|moonshotai/kimi|qwen|glm|hy3|kat-coder|nemotron|mercury|ling-2\\.6|big-coder.*failover" README.md docs deployment.md internal scripts`

## Live Provider Testing

- OpenAI Responses smoke:
  - `POST https://api.openai.com/v1/responses`
  - body: `{"model":"<model>","input":"Reply OK only.","max_output_tokens":16}`
- OpenAI-compatible chat smoke:
  - `POST <base_url>/chat/completions`
  - body: `{"model":"<model>","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":false}`
- xAI/Grok candidates:
  - Official Grok 4.3 docs checked on 2026-06-17 list `grok-4.3` with text+image input, text output, function calling, structured outputs, configurable reasoning, 1M context, and $1.25/M input plus $2.50/M output pricing.
  - Direct xAI `grok-4.3` smokes passed on 2026-06-17 after billing was funded: text returned `OK`, receipt-image OCR returned `Rite Aid`, and xAI usage reported image tokens. Local router-level text and image smokes also passed; the image request logged `input_image_tokens`, separated image cost, and no warnings after `image_input_price_per_million_tokens_usd: 1.25` was configured.
- Vision/OpenAI-compatible chat smoke:
  - `POST <base_url>/chat/completions`
  - body: `{"model":"<model>","messages":[{"role":"user","content":[{"type":"text","text":"Read the receipt. Reply with only the merchant name."},{"type":"image_url","image_url":{"url":"https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}]}],"max_tokens":64,"stream":false}`
  - Accept only a useful OCR answer, such as the receipt merchant name. If the response is empty, spends all tokens on reasoning, or merely accepts the image without reading it, do not mark the target vision-capable.
- Self-hosted vLLM/SGLang OpenAI-compatible smoke:
  - Validate `GET <base_url>/models` and confirm the served model ID matches `providers.<name>.models.<ref>.model`.
  - Run a direct `/chat/completions` text smoke before routing traffic through the router.
  - For tool-capable routes, run a direct `/chat/completions` request with the exact `tools`, `tool_choice`, parser/chat-template, streaming mode, and model version expected in production; then repeat through the router group.
  - Do not mark a self-hosted target `tool_only` or add it to active tool routing until the direct and router-level tool smokes return correctly shaped tool calls.
- OpenRouter Nitro variants may not appear as separate IDs in `/models`; validate by making a real completion call with the `:nitro` suffix.
- OpenRouter catalog modality is not enough for activation. `qwen/qwen3-vl-32b-instruct:nitro` accepted the receipt image on 2026-06-17 but answered loyalty-program text rather than merchant name, so it is catalog-only. `qwen/qwen3.7-plus:nitro` returned one correct receipt answer but was slow/inconsistent, spent excessive reasoning tokens, and hallucinated CVS on the stricter prompt, so do not activate it without a better validated prompt/settings path. `~google/gemini-flash-latest` and `google/gemini-3.1-flash-lite-preview` advertised multimodal support on 2026-06-17 but returned provider-privacy 404s with the current OpenRouter account, so do not add them to config until access is fixed and live image smoke passes.
- OpenRouter reasoning-heavy models such as `z-ai/glm-5.2:nitro` can return HTTP 200 with empty assistant content when `max_tokens` is too small because the budget is spent on reasoning. Before activating or increasing weight for such models, smoke test both a tiny budget and a realistic budget. For GLM 5.2, use a realistic smoke such as `max_tokens: 1024`; note if low-budget requests need a `reasoning.max_tokens` cap or should not be used as acceptance evidence.
- For OpenRouter candidates intended for coding agents, validate all configured skins before adding them to active groups: `/chat/completions`, `/responses` with a function tool, and `/messages` with an Anthropic-style tool. Keep `openrouter`, `openrouter_responses`, and `openrouter_anthropic` model catalogs in sync for models that pass all three checks.
- MiniMax Codex smoke should use MiniMax-M3 through a Responses-compatible endpoint and Codex `wire_api="responses"`.
- Codex CLI image smoke should use `codex exec --image <file>` against the router Responses provider config. Claude Code image support should be validated with the Anthropic Messages image payload shape and, where the installed CLI supports direct image attachment, with the actual CLI workflow.
- If a direct provider smoke returns 403 or model-not-found, do not add that model to active route targets.

## Production Host

- Host: `ubuntu@100.30.225.66`
- SSH key: `~/.ssh/chetan-jun-2026.pem`
- Public URL: `https://llm-api-engg.metrum.ai`
- Compose directory: `/opt/smart-llmrouter/compose`
- Runtime config: `/opt/smart-llmrouter/compose/config/config.yaml`
- Provider keys: `/opt/smart-llmrouter/compose/config/env.json`
- Router token file: `/opt/smart-llmrouter/compose/ROUTER_TOKEN.txt`

Useful commands:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose ps'
rtk curl -fsS https://llm-api-engg.metrum.ai/readyz
```

## Production Config Update Process

For config-only production changes:

1. Update `config.example.yaml`.
2. Update ignored `config.production.yaml` locally.
3. Validate both with Python/YAML and confirm no forbidden references remain.
4. Run `rtk go test ./...`.
5. On the host, back up and patch `/opt/smart-llmrouter/compose/config/config.yaml` with a structured YAML script.
6. Run:
   - `sudo docker compose config >/dev/null`
   - `sudo docker compose restart router`
   - `sudo docker compose ps router`
7. Verify:
   - `curl -fsS https://llm-api-engg.metrum.ai/readyz`
   - local `config.production.yaml` SHA-256 matches the remote config SHA-256
   - an authenticated router smoke test hits the expected route/model when relevant
8. Update `deployment.md` or docs if the deployed image, model groups, operational process, or validated targets changed.
9. Search for stale deployment facts such as old image tags, removed models, and outdated route descriptions.

Never edit production config without a timestamped backup:

```text
config/config.yaml.bak.<UTC timestamp>
```

## Production Package Deployment Process

For code changes that affect runtime behavior or embedded hosted docs:

1. Run relevant tests:
   - Prefer `rtk go test ./cmd/... ./internal/...` for router code.
   - `rtk go test ./...` may fail on generated Harbor/job artifact directories; if so, report that separately and do not treat it as a router package failure.
   - For public docs examples, run the exact curl/Python commands against the intended endpoint using ignored local credentials.
2. Run `rtk make docs-build` when `docs-site/` changes.
3. Commit source/docs changes before packaging so `VERSION=$(git describe --tags --always --dirty)` is a stable commit tag and not `-dirty`.
4. Build the amd64 Docker package:
   - `rtk make package-docker GOOS=linux GOARCH=amd64`
5. Copy `dist/smart-llmrouter-<version>-docker-linux-amd64.tar.gz` to the host with `scp`.
6. On the host:
   - back up `/opt/smart-llmrouter` to `/opt/smart-llmrouter.backup.<purpose>-<UTC timestamp>`
   - unpack the package into a fresh `/opt/smart-llmrouter`
   - copy forward live `compose/config`, `compose/state`, `compose/logs`, `.env`, and `ROUTER_TOKEN*.txt` from the backup
   - set `SMART_LLMROUTER_VERSION=<version>-linux-amd64` in `compose/.env`
   - `sudo docker load -i images/smart-llmrouter-<version>-linux-amd64.tar`
   - `sudo docker compose config >/dev/null`
   - `sudo docker compose up -d`
7. Verify health, route behavior, and hosted docs when relevant:
   - `curl -fsS https://llm-api-engg.metrum.ai/readyz`
   - `curl -fsS https://llm-api-engg.metrum.ai/docs/...`
   - authenticated `/v1/models` or completion smoke for API compatibility
8. Update `deployment.md` with image/package tag, source commit, backup path when useful, and validation results.
9. Commit the deployment note after production verification.

## Router Smoke Tests

Authenticated production chat smoke:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && TOKEN=$(sudo cat ROUTER_TOKEN.txt) && curl -fsS https://llm-api-engg.metrum.ai/v1/chat/completions -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" -d "{\"model\":\"high\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply OK only.\"}],\"max_tokens\":16,\"stream\":false}"'
```

Use `high` for deterministic failover-first checks, and use repeated calls for weighted groups such as `big-coder`. When validating weighted groups that include reasoning-heavy OpenRouter targets, include a realistic `max_tokens` budget; a `max_tokens:16` smoke can produce false failures for GLM-style models that spend the completion budget on reasoning before emitting final content.

## CLI E2E Expectations

- Claude Code should use router bearer token settings:
  - `ANTHROPIC_BASE_URL=https://llm-api-engg.metrum.ai`
  - `ANTHROPIC_AUTH_TOKEN=$ROUTER_TOKEN`
  - Do not set `ANTHROPIC_API_KEY` for router traffic.
  - In scripts, use `env -u ANTHROPIC_API_KEY ... claude ...` so a developer shell cannot accidentally force direct Anthropic `X-Api-Key` auth.
- Codex CLI should use an OpenAI-compatible provider config pointing at:
  - `https://llm-api-engg.metrum.ai/v1`
  - env key such as `METRUM_ROUTER_KEY`
  - `model_providers.<name>.wire_api="responses"`
- For CLI-generated C program tests, the CLI must generate the C program. Do not replace that with a static harness.
- For agent-tool validation, use real tool calls:
  - Claude Code via Anthropic Messages API and `claude-tools-smoke`.
  - Codex via OpenAI Responses API and `agent-tools-smoke`.
  - OpenRouter-specific Claude Code via `claude-tools-smoke-openrouter`.
  - OpenRouter-specific Codex via `agent-tools-smoke-openrouter`.
  - Assert the created file contents, not only text printed by the assistant.
- Requests with tools must bypass response caching; keep regression coverage for this.

## Documentation Expectations

Update docs whenever changing:

- model catalogs or active model groups
- caller token behavior or allowed groups
- provider keys/env requirements
- production deployment commands or image tags
- Codex CLI or Claude Code usage examples
- TypeScript routing script behavior, context fields, or model-group policy examples
- self-hosted vLLM/SGLang/OpenAI-compatible upstream deployment, served model IDs, parser/chat-template flags, or tool-call validation behavior
- provider model pricing metadata, pricing source/update dates, tool support metadata, or upstream capability claims
- usage reporting, caching, telemetry, or auth behavior
- DB driver/schema behavior, including SQLite/Postgres config, usage report fields, or durability expectations

Keep docs concrete and tested. Include working commands, but redact secrets.

Before finalizing, check at minimum:

```bash
rtk rg -n "MiniMax-Text-01|text-01|big-coder.*failover|failover route|does not yet have access|old image|openrouter/pareto|moonshotai/kimi|qwen|glm|hy3|kat-coder|nemotron|mercury|ling-2\\.6" README.md docs deployment.md internal scripts || true
rtk rg -n "openai/gpt|anthropic/claude|claude-sonnet|MiniMax-M2\\.7|m27-highspeed" config.example.yaml README.md docs deployment.md scripts || true
```

## References

- Codex AGENTS.md guidance: https://developers.openai.com/codex/guides/agents-md
- Codex best practices: https://developers.openai.com/codex/learn/best-practices
- Codex CLI install/update: https://developers.openai.com/codex/cli
- Claude Code auth precedence/env vars: https://code.claude.com/docs/en/authentication
