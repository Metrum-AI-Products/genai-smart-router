# Docker Compose Deployment

Docker packages are intended for AWS EC2 or similar hosts where the source tree is not present and no image registry is required. The router image embeds the customer-facing Docusaurus documentation and serves it under `/docs/`; browser requests to `/` redirect there.

The checked-in `Dockerfile` pins the Go builder image to a patched Go patch release. Keep that tag pinned when updating the toolchain so release images do not silently move onto an unreviewed compiler or standard-library patch level.

## Build Package

From the development machine:

```bash
make package-docker
```

This creates:

```text
dist/smart-llmrouter-<version>-docker-linux-amd64.tar.gz
dist/smart-llmrouter-<version>-docker-linux-arm64.tar.gz
```

Docker image builds use `docker buildx build --load` for each packaged platform.

Each package contains:

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
docs/solution-brief.md
docs/USAGE_DB_DESIGN.md
```

## AWS EC2 Host Setup

Use the amd64 package for common x86_64 EC2 instances and arm64 for Graviton instances.

Security group inbound rules:

```text
22/tcp   from trusted admin IPs
80/tcp   from 0.0.0.0/0 and ::/0
443/tcp  from 0.0.0.0/0 and ::/0
```

DNS:

- Create or update the `A` record for your deployment hostname to the EC2 public IPv4 address.
- Create an `AAAA` record only if the instance has a working public IPv6 address.
- DNS can stay with the organization's DNS provider; it only needs to point to the AWS instance.

Install Docker and the Compose plugin on the host, then unpack:

```bash
sudo mkdir -p /opt/smart-llmrouter
sudo tar -C /opt/smart-llmrouter --strip-components=1 -xzf smart-llmrouter-<version>-docker-linux-amd64.tar.gz
cd /opt/smart-llmrouter
```

Use the `docker-linux-amd64` package on x86_64 hosts and the `docker-linux-arm64` package on ARM64 hosts.

Load the packaged image:

```bash
docker load -i images/smart-llmrouter-<version>-linux-amd64.tar
```

Prepare runtime directories and config:

```bash
mkdir -p compose/config/scripts compose/state compose/logs
cp config/config.example.yaml compose/config/config.yaml
cp config/env.example.json compose/config/env.json
cp config/scripts/router.ts compose/config/scripts/router.ts
sudo chown -R 65532:65532 compose/config compose/state compose/logs
chmod 0750 compose/config compose/config/scripts
chmod 0400 compose/config/env.json
```

The router container runs as UID/GID `65532`. The config directory must be traversable by that ID, `env.json` must be readable by that ID, and the state/log bind mounts must be writable by that ID.

If the TypeScript routing script imports local helpers, copy those files into `compose/config/scripts/` as well. If it imports third-party packages, build and lock those dependencies before packaging and deploy either the required dependency tree under `compose/config/scripts/` or a pre-bundled script artifact. The container bundles scripts at router startup; it does not install npm packages at runtime.

External routing-policy calls are opt-in per script model group with `script_http.enabled: true`, exact `allow_hosts`, `timeout_ms`, and `max_response_bytes`. Scripts call allowlisted services with `router.fetchJSON`; unrestricted `fetch`, runtime package installation, provider keys, and raw router tokens are not exposed to scripts. HTTPS is required for non-local services unless `script_http.allow_http: true` is explicitly approved; loopback HTTP is allowed for local demos and sidecars. Redirects must keep an allowed scheme and exact allowlisted hostname. Put policy-service auth in env-expanded `script_http.headers`, not in script source.

For PII-aware script routing, package the policy file under `compose/config/scripts/`, mark private backing targets with deployment-owned metadata such as `tier: private` or `display_name: Private sensitive target`, and smoke-test that likely PII prompts select only those targets for primary routing and retry fallbacks. The `examples/typescript-pii-policy/` demo returns safe labels only, fails closed when no sensitive/private target is eligible, and does not redact outbound content. Use model-group `pii_filter` when the router must redact, restore, or fail requests before provider calls.

For standalone policy services, configure `strategy: external` with `external_policy.url`, exact `allow_hosts`, low `timeout_ms`, `max_response_bytes`, and config-owned auth headers. Use HTTPS unless the service is loopback-local or `external_policy.allow_http: true` is approved for a trusted internal endpoint. Redirects are blocked when any hop changes to a non-allowlisted hostname. The service receives safe routing context and eligible target metadata, then returns the selected target decision.

Edit `compose/config/config.yaml` for container paths:

```yaml
server:
  listen: ":8080"
  logging:
    path: /app/logs/requests.jsonl
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}

state_path: /app/state/router-state.json
```

Edit `compose/config/env.json` with provider keys, or inject the same variables through your container secret manager or compose environment. `config/env.example.json` is only an empty placeholder template checked by `make secret-check`. Do not commit or publish runtime `env.json`, compose overrides containing real keys, or modified examples with real values.

The packaged compose file includes `postgres:18-bookworm` for the usage DB. It listens only on the internal Docker network at `postgres:5432` by default. Compose fails during `docker compose config` if `SMART_LLMROUTER_VERSION`, `POSTGRES_PASSWORD`, or `ROUTER_USAGE_DB_DSN` are missing. Generate a strong random database password, store it in `compose/.env` as `POSTGRES_PASSWORD`, and use the same value in `ROUTER_USAGE_DB_DSN`.

If temporary host access to Postgres is required for local administration, include the explicit localhost-only override:

```bash
docker compose -f docker-compose.yml -f docker-compose.postgres-localhost.yml up -d
```

That override binds Postgres to `127.0.0.1:${POSTGRES_HOST_PORT:-15432}` on the deployment host. Do not publish Postgres on `0.0.0.0`.

The response cache is in-memory inside the router container. Restarting the container clears cached responses. Cache hits are shared across caller tokens, return fresh router-owned response IDs, and do not consume provider credits or persisted caller token quota. Cache hit/miss/bypass, item count, occupied bytes, max bytes, and occupancy percentage are persisted per request in the usage DB.

Token-budget admission reserves estimated input tokens plus the caller's requested output cap before upstream calls. The reservation uses `max_tokens`, `max_completion_tokens`, `max_output_tokens`, or the router-injected Messages default cap when applicable. TPM, daily token, monthly token, and lifetime key checks include in-flight reservations; request-count quotas are unchanged. Successful requests persist actual upstream-reported usage, and failures, cancellations, and cache hits release or avoid token reservations.

Generate a caller token:

```bash
docker run --rm --entrypoint /app/bin/router-token-gen smart-llmrouter:<version>-linux-amd64 generate \
  --user chetan \
  --project metrum-insights \
  --env dev \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Append the generated caller config to `compose/config/config.yaml` and save the printed `token` for clients.

Verify the caller-facing model groups with the same token before handing it to users:

```bash
curl -H "Authorization: Bearer $ROUTER_TOKEN" "$ROUTER_BASE_URL/v1/models"
```

The response is filtered by that token's `allow` entries. Every returned `id` is a router model group the caller can use in Chat Completions, Responses, Messages, Codex CLI, or Claude Code CLI. Missing groups require an allow-list update.

Review `compose/.env`:

```bash
SMART_LLMROUTER_VERSION=<version>-linux-amd64
ROUTER_HOSTNAME=your-router.example.com
CADDY_EMAIL=chetan@metrum.ai
CADDY_HTTP_PORT=80
CADDY_HTTPS_PORT=443
POSTGRES_DB=llmrouter
POSTGRES_USER=llmrouter
POSTGRES_PASSWORD=<strong-random-db-password>
ROUTER_USAGE_DB_DSN=host=postgres port=5432 user=llmrouter password=<strong-random-db-password> dbname=llmrouter sslmode=disable TimeZone=UTC
```

`SMART_LLMROUTER_VERSION` must match the image tag loaded from the package, such as `<version>-linux-amd64` or `<version>-linux-arm64`; it must not be `latest`.

Validate the rendered compose model before starting:

```bash
docker compose config >/dev/null
```

Start:

```bash
cd /opt/smart-llmrouter/compose
docker compose up -d
```

Caddy terminates TLS and proxies to the private `router:8080` service. Caddy will obtain certificates automatically once DNS points to the instance and ports 80/443 are reachable.

## Smoke Test

From outside the instance:

```bash
export ROUTER_BASE_URL="https://your-router.example.com"
curl "$ROUTER_BASE_URL/healthz"
curl "$ROUTER_BASE_URL/version"
curl -H "Authorization: Bearer $ROUTER_TOKEN" "$ROUTER_BASE_URL/v1/models"
```

Inside the running container, `/app/bin/router --version`, `/app/bin/router-token-gen --version`, and `/app/bin/router-usage-report --version` print the package version, commit, full UTC build timestamp, Go version, OS, and architecture. Hosted browser docs display the package version and build timestamp on every page and return `X-Smart-LLMRouter-*` version headers.

Enterprise deployments can route to private vLLM or SGLang services by configuring them as OpenAI-compatible providers with internal `/v1` base URLs. Before adding those targets to active production groups, validate the upstream `/v1/models` ID, a direct text completion, any required tool-call path, and the same requests through the router. Record model IDs, vLLM/SGLang parser flags, chat templates, server versions, and rollback steps in deployment notes. See `docs/SELF_HOSTED_UPSTREAMS.md`.

Generate a markdown usage report on the host from the running compose data:

```bash
dsn="$(sed -n 's/^ROUTER_USAGE_DB_DSN=//p' .env | tail -n 1)"
docker compose run --rm --entrypoint /app/bin/router-usage-report router \
  --driver postgres \
  --dsn "$dsn" \
  --since 24h \
  --out /app/logs/usage-24h.md
```

Add `--caller-project`, `--caller-environment`, `--token-id`, `--token-id-prefix`, `--resolved-group`, or `--client` to narrow a report to one benchmark, caller cohort, model group, or CLI client.

The report includes internal router API key usage by `token_id`/user/project/environment, caller IP usage, hourly usage by caller IP, external provider/model calls, token totals, request-time USD cost, cache hit/miss/bypass, cache occupancy, attempts, fallbacks, status codes, latency, hourly usage, daily usage, downstream user performance, upstream provider/model/dialect performance, and per-request upstream/downstream tokens/sec. Use the downstream user and upstream endpoint performance sections first when triaging slow UX. It does not include raw router tokens or provider API keys.

Usage rows, throughput fields, request-time pricing/cost fields, and cache snapshots are durable across restarts when the Postgres volume is preserved. The in-memory response cache and `/metrics` process counters reset when the router restarts. `/metrics` is global operational telemetry and is only available to caller tokens configured with `metrics_admin: true`; normal application keys should use `/v1/usage` or usage reports.

To intentionally start production reporting clean after a schema change, stop the stack, back up the Postgres volume or database, remove the Postgres data volume, and start the stack again:

```bash
docker compose down
POSTGRES_VOLUME="${COMPOSE_PROJECT_NAME:-$(basename "$PWD")}_postgres_data"
docker run --rm -v "$POSTGRES_VOLUME":/var/lib/postgresql -v "$PWD":/backup alpine \
  tar -C /var/lib -czf /backup/postgres-data-backup-$(date -u +%Y%m%d%H%M%S).tar.gz postgresql
docker volume rm "$POSTGRES_VOLUME"
docker compose up -d
```

Example hosted deployment model groups:

```text
default    DeepSeek V4 Flash Nitro 46%, MiniMax-M3 27%, Baseten Nemotron 3%, Baseten GLM 5.2 5%, Gemma 7%, Qwen 3.6 Flash 5%, Kimi K2.7 Code 6%, OpenAI GPT-5.4 Nano 1%.
fast       DeepSeek V4 Flash Nitro 51%, MiniMax-M3 26%, Baseten Nemotron 3%, Baseten GLM 5.2 5%, Gemma 4%, Qwen 3.6 Flash 5%, Kimi K2.7 Code 5%, OpenAI GPT-5.4 Nano 1%.
small      DeepSeek V4 Flash Nitro 53%, MiniMax-M3 28%, Baseten Nemotron 3%, Baseten GLM 5.2 2%, Gemma 4%, Qwen 3.6 Flash 5%, Kimi K2.7 Code 4%, OpenAI GPT-5.4 Nano 1%.
medium     DeepSeek V4 Flash Nitro 46%, MiniMax-M3 25%, Baseten Nemotron 3%, Baseten GLM 5.2 5%, Gemma 7%, Qwen 3.6 Flash 5%, Kimi K2.7 Code 8%, OpenAI GPT-5.4 Nano 1%.
high       DeepSeek V4 Flash Nitro 40%, MiniMax-M3 26%, Baseten Nemotron 3%, Baseten GLM 5.2 6%, Gemma 9%, Qwen 3.6 Flash 5%, Kimi K2.7 Code 10%, OpenAI GPT-5.4 Nano 1%.
big-coder  Code-heavy route: DeepSeek V4 Flash Nitro 18%, Qwen 3.6 Flash 5%, MiniMax-M3 38%, Baseten Nemotron 3%, Baseten GLM 5.2 7%, Kimi K2.7 Code 28%, OpenAI GPT-5.4 Nano 1%.
```

Caller tokens are restricted by `callers[].allow`. Model group names are deployment-defined; the names above are examples from this hosted/reference deployment. `/v1/models` only lists model groups allowed for the presented token, and disallowed requests return `403 model-not-allowed` before any upstream provider call.

Claude Code:

```bash
unset ANTHROPIC_API_KEY
export ANTHROPIC_BASE_URL="$ROUTER_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"
claude --bare --print --model "<allowed-model-group>" "Reply with exactly: router claude ok"
```

Codex:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  -c 'model="<allowed-model-group>"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="'"$ROUTER_BASE_URL"'/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router codex ok" </dev/null
```

The `exec` subcommand is required for `--ignore-user-config`, `--ephemeral`, `--ignore-rules`, and `--skip-git-repo-check`; those flags are not accepted by the top-level interactive `codex` command.

Interactive Codex uses top-level `codex`, without the `exec`-only flags:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
codex \
  -c 'model="<allowed-model-group>"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="'"$ROUTER_BASE_URL"'/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"'
```

Tool-capable acceptance checks should exercise the real agent tool paths, not just text echo. Use `claude-tools-smoke` with Claude Code over the Anthropic Messages API and `agent-tools-smoke` with Codex over OpenAI Responses. Tool-bearing requests are not cacheable, because their results depend on shell/filesystem/tool state.
Use `claude-tools-smoke-openrouter` and `agent-tools-smoke-openrouter` when the acceptance gate must specifically validate OpenRouter's Anthropic-compatible and Responses-compatible tool routes.
For OpenAI Chat Completions agent clients such as Warp Agent, run a `/v1/chat/completions` smoke with `tools`, `tool_choice`, `stream: true`, and a realistic agent User-Agent. The router should preserve the OpenAI Chat tool payload, select a target with explicit `tool_support.openai_chat`, and stream back `delta.tool_calls` plus `finish_reason: "tool_calls"`.
For self-hosted vLLM or SGLang tool routes, first run an OpenAI-compatible chat `tools` request directly against the upstream, then route the same request through the configured router model group. Tool support depends on the model, parser, chat template, streaming mode, and `tool_choice` mode.

Claude Code tool smoke:

```bash
unset ANTHROPIC_API_KEY
mkdir -p /tmp/router-claude-tool-smoke
cd /tmp/router-claude-tool-smoke
ANTHROPIC_BASE_URL="$ROUTER_BASE_URL" \
ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN" \
claude --bare --print --model claude-tools-smoke \
  --permission-mode bypassPermissions \
  --allowedTools "Write,Bash" \
  "Create claude_tool_smoke.txt containing exactly claude-tool-ok, run cat claude_tool_smoke.txt, then finish with claude-tool-ok."
test "$(cat claude_tool_smoke.txt)" = "claude-tool-ok"
```

Codex tool smoke:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
mkdir -p /tmp/router-codex-tool-smoke
codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  --dangerously-bypass-approvals-and-sandbox \
  -C /tmp/router-codex-tool-smoke \
  -c 'model="agent-tools-smoke"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="'"$ROUTER_BASE_URL"'/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Create codex_tool_smoke.txt containing exactly codex-tool-ok, run cat codex_tool_smoke.txt, then finish with codex-tool-ok." </dev/null
test "$(cat /tmp/router-codex-tool-smoke/codex_tool_smoke.txt)" = "codex-tool-ok"
```

## Local Compose E2E

For local e2e, run Caddy on plain HTTP by setting:

```bash
ROUTER_HOSTNAME=:80 CADDY_HTTP_PORT=18080 CADDY_HTTPS_PORT=18443 docker compose up -d
```

Then use:

```text
http://127.0.0.1:18080
```
