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
- Usage persistence uses GORM. Keep the entire usage DB schema purely relational: no JSON/JSONB columns, no array columns, no serialized blobs for structured data, and no packed multi-value text fields. If one request needs multiple related rows, add a child table with scalar columns and a foreign key to `request_usage`.
- Do not put unavailable provider models into active routing. Catalog-only is acceptable when a model exists but the current key is not entitled.
- Prefer structured YAML/JSON parsing for config changes. Avoid fragile text edits for production config.
- Keep sample config, local production snapshot, production config, docs, and tests in sync for behavior changes.
- No stale docs. Before finishing any task that changes behavior, config, deployment, models, auth, CLI usage, tests, or production, search the repo for old names/status and update every matching doc or fixture. If a doc cannot be made current, mark the exact section as historical with a date and reason.

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
7. Update docs for any user-facing config, model, deployment, CLI, or operational change.
8. Run a stale-doc search for changed concepts before final response. Examples:
   - `rtk rg -n "old-model|old-provider|old-image-tag" README.md docs deployment.md config.example.yaml internal scripts`
   - `rtk rg -n "MiniMax-Text-01|text-01|gpt-5\\.5.*not active|big-coder.*failover" README.md docs deployment.md internal scripts`

## Live Provider Testing

- OpenAI Responses smoke:
  - `POST https://api.openai.com/v1/responses`
  - body: `{"model":"<model>","input":"Reply OK only.","max_output_tokens":16}`
- OpenAI-compatible chat smoke:
  - `POST <base_url>/chat/completions`
  - body: `{"model":"<model>","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":false}`
- OpenRouter Nitro variants may not appear as separate IDs in `/models`; validate by making a real completion call with the `:nitro` suffix.
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

## Production Image Deployment Process

For code changes that affect runtime behavior:

1. Run `rtk go test ./...`.
2. Build an amd64 Docker image with an explicit tag that describes the change.
3. `docker save` the image to `dist/deploy/`.
4. Copy the tarball to the host with `scp`.
5. On the host:
   - `sudo docker load -i /tmp/<image>.tar`
   - back up `.env`
   - update `SMART_LLMROUTER_VERSION=<new tag>`
   - `sudo docker compose up -d router`
6. Verify health and route behavior.
7. Update deployment docs with the new image tag and validation results.

## Router Smoke Tests

Authenticated production chat smoke:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && TOKEN=$(sudo cat ROUTER_TOKEN.txt) && curl -fsS https://llm-api-engg.metrum.ai/v1/chat/completions -H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json" -d "{\"model\":\"high\",\"messages\":[{\"role\":\"user\",\"content\":\"Reply OK only.\"}],\"max_tokens\":16,\"stream\":false}"'
```

Use `high` for deterministic failover-first checks, and use repeated calls for weighted groups such as `big-coder`.

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
  - Assert the created file contents, not only text printed by the assistant.
- Requests with tools must bypass response caching; keep regression coverage for this.

## Documentation Expectations

Update docs whenever changing:

- model catalogs or active model groups
- caller token behavior or allowed groups
- provider keys/env requirements
- production deployment commands or image tags
- Codex CLI or Claude Code usage examples
- usage reporting, caching, telemetry, or auth behavior
- DB driver/schema behavior, including SQLite/Postgres config, usage report fields, or durability expectations

Keep docs concrete and tested. Include working commands, but redact secrets.

Before finalizing, check at minimum:

```bash
rtk rg -n "MiniMax-Text-01|text-01|big-coder.*failover|failover route|does not yet have access|not active|gpt-5\\.5.*not active|old image" README.md docs deployment.md internal scripts || true
```

## References

- Codex AGENTS.md guidance: https://developers.openai.com/codex/guides/agents-md
- Codex best practices: https://developers.openai.com/codex/learn/best-practices
- Codex CLI install/update: https://developers.openai.com/codex/cli
- Claude Code auth precedence/env vars: https://code.claude.com/docs/en/authentication
