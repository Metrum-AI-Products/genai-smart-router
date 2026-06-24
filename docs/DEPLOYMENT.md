# Smart LLM Router Deployment

This project is packaged as a binary distribution. A deployment host does not need the Go toolchain, Node.js, Docusaurus, or source tree. Release binaries embed the customer-facing Docusaurus documentation and serve it under `/docs/`; browser requests to `/` redirect there.

## Package Contents

`make package` creates Linux x86_64 and arm64 tarballs under `dist/`:

```text
smart-llmrouter-<version>-linux-amd64.tar.gz
smart-llmrouter-<version>-linux-arm64.tar.gz
```

Each tarball contains:

```text
bin/router
bin/router-token-gen
config/config.example.yaml
config/env.example.json
config/scripts/router.ts
docs/README.md
docs/DEPLOYMENT.md
docs/DOCKER_DEPLOYMENT.md
docs/solution-brief.md
caddy/Caddyfile
```

The config and routing script are packaged together so this command works after unpacking:

```bash
bin/router --config config/config.yaml
```

`config/config.yaml` can keep `script: scripts/router.ts` because script paths are resolved relative to the config file.

If `router.ts` imports local helpers, place those files under `config/scripts/` and include them in the release package. If it imports third-party packages, install and lock them before packaging and ship either the resolved dependency tree needed by esbuild or a pre-bundled script artifact. The router bundles from the deployment filesystem at startup and does not run `npm install` on the host.

External TypeScript policy calls are disabled unless a script model group enables `script_http` in `config.yaml`. Configure exact `allow_hosts`, a small `timeout_ms`, and `max_response_bytes`; scripts call these services through `router.fetchJSON`, not unrestricted browser `fetch`. Put policy-service auth in `script_http.headers` with env-expanded values such as `${ROUTING_POLICY_AUTH_HEADER}` instead of hardcoding secrets in script source.

For standalone policy services, prefer `strategy: external` with `external_policy.url`, exact `allow_hosts`, low `timeout_ms`, response-size limits, and config-owned auth headers. The external routing policy service receives normalized request context and eligible target metadata, returns `targetIndex` or `target`, and is validated before any upstream provider call. See `docs/EXTERNAL_ROUTING_POLICY.md`.

For built-in adaptive routing, prefer `strategy: dynamic_score` before adding custom strategies. It scores only the requested model group's eligible targets, uses in-memory rolling observations instead of hot-path database reads, emits safe scalar `routing_decision` traces, and rolls back by switching the group to `weighted`. See `docs/DYNAMIC_SCORE_ROUTING.md`.

Docker Compose packages are built separately:

```bash
make package-docker
```

Use `docs/DOCKER_DEPLOYMENT.md` when deploying the packaged Docker image tarball plus Caddy compose stack to AWS EC2 or a similar host.

## Example Internal Development Host

One internal Metrum-operated deployment target:

```text
llm-api-engg.metrum.ai
```

The service is externally reachable, but model and usage endpoints require a valid router caller token, and `/metrics` requires a caller token configured with `metrics_admin: true`. Caddy terminates TLS and reverse-proxies to the router on localhost.

DNS for that example host is managed in DigitalOcean. For other enterprise, on-prem, or Metrum-managed deployments, use that deployment's hostname and DNS provider. Create or update an `A` record pointing to the public IPv4 address of the deployment host. Add an `AAAA` record only if the host has working public IPv6.

## Host Layout

Recommended paths:

```text
/opt/smart-llmrouter/bin/router
/opt/smart-llmrouter/bin/router-token-gen
/opt/smart-llmrouter/config/config.yaml
/opt/smart-llmrouter/config/env.json
/opt/smart-llmrouter/config/scripts/router.ts
/var/lib/smart-llmrouter/router-state.json
/var/log/smart-llmrouter/requests.jsonl
/etc/caddy/Caddyfile
```

Use `env.json` for provider API keys on the deployment host. Keep it mode `0600` and do not place it in a web-served path.

The response cache is process-local. Restarting the router clears cached responses. Cache hits are shared across caller tokens, return fresh router-owned response IDs, and do not consume provider credits or persisted caller token quota. Cache telemetry is durable because each usage row stores cache hit/miss/bypass, item count, occupied bytes, max bytes, and occupancy percentage.

## Install

On the deployment host:

```bash
sudo mkdir -p /opt/smart-llmrouter /var/lib/smart-llmrouter /var/log/smart-llmrouter
sudo tar -C /opt/smart-llmrouter --strip-components=1 -xzf smart-llmrouter-<version>-linux-amd64.tar.gz
sudo cp /opt/smart-llmrouter/config/config.example.yaml /opt/smart-llmrouter/config/config.yaml
sudo cp /opt/smart-llmrouter/config/env.example.json /opt/smart-llmrouter/config/env.json
sudo chmod 0755 /opt/smart-llmrouter/bin/router /opt/smart-llmrouter/bin/router-token-gen
sudo chmod 0600 /opt/smart-llmrouter/config/env.json
```

Use the `linux-amd64` tarball for x86_64 hosts and the `linux-arm64` tarball for ARM64 hosts.

Edit `/opt/smart-llmrouter/config/config.yaml`:

```yaml
server:
  listen: "127.0.0.1:8080"
  logging:
    path: /var/log/smart-llmrouter/requests.jsonl
  usage_db:
    enabled: true
    driver: sqlite
    path: /var/lib/smart-llmrouter/usage.sqlite

state_path: /var/lib/smart-llmrouter/router-state.json
```

For Docker Compose production, use the packaged `postgres:18-bookworm` service instead:

```yaml
server:
  usage_db:
    enabled: true
    driver: postgres
    dsn: ${ROUTER_USAGE_DB_DSN}
```

The compose Postgres service listens on `postgres:5432` internally and publishes host port `15432` by default for admin access.

Edit `/opt/smart-llmrouter/config/env.json` with provider keys such as `OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `GROQ_API_KEY`, `MOONSHOT_API_KEY`, `MINIMAX_API_KEY`, and `XAI_API_KEY`.

Generate a caller token and append the generated caller block to `config.yaml`:

```bash
/opt/smart-llmrouter/bin/router-token-gen generate \
  --user chetan \
  --project metrum-insights \
  --env dev \
  --allow <allowed-model-group>[,<allowed-model-group>...]
```

Save the printed `token` value for the client. The router config stores only `token_sha256` and `token_id`.

Use `--allow` to restrict each generated key to specific internal model groups. Model group names are deployment-defined; any names shown in examples are reference deployment names only. `/v1/models` only lists model groups allowed for the presented token, and disallowed requests return `403 model-not-allowed` before any upstream provider call.

Caller model discovery is part of the access contract. After issuing or rotating a token, verify the caller-facing list with the same token:

```bash
curl -H "Authorization: Bearer $ROUTER_TOKEN" "$ROUTER_BASE_URL/v1/models"
```

Every listed `id` is a router model group the caller can request. Missing groups require an allow-list change, not a client-side workaround.

## systemd

Create `/etc/systemd/system/smart-llmrouter.service`:

```ini
[Unit]
Description=Smart LLM Router
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=smart-llmrouter
Group=smart-llmrouter
WorkingDirectory=/opt/smart-llmrouter
ExecStart=/opt/smart-llmrouter/bin/router --config /opt/smart-llmrouter/config/config.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/smart-llmrouter /var/log/smart-llmrouter

[Install]
WantedBy=multi-user.target
```

Create the service user and start the service:

```bash
sudo useradd --system --home /opt/smart-llmrouter --shell /usr/sbin/nologin smart-llmrouter
sudo chown -R smart-llmrouter:smart-llmrouter /opt/smart-llmrouter /var/lib/smart-llmrouter /var/log/smart-llmrouter
sudo systemctl daemon-reload
sudo systemctl enable --now smart-llmrouter
```

## Caddy TLS Termination

Install Caddy on the host and place the packaged `caddy/Caddyfile` at `/etc/caddy/Caddyfile`.

The Caddyfile terminates TLS for the configured deployment hostname and proxies to `127.0.0.1:8080`. Caddy obtains and renews public certificates automatically after DNS points the hostname to the host and ports `80` and `443` are reachable.

Reload Caddy:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

## Smoke Test

From a client machine:

```bash
export ROUTER_BASE_URL="https://your-router.example.com"
curl "$ROUTER_BASE_URL/healthz"
curl "$ROUTER_BASE_URL/version"
curl -H "Authorization: Bearer $ROUTER_TOKEN" "$ROUTER_BASE_URL/v1/models"
```

On the host, `bin/router --version`, `bin/router-token-gen --version`, and `bin/router-usage-report --version` print the package version, commit, full UTC build timestamp, Go version, OS, and architecture. Hosted browser docs display the package version and build timestamp on every page and return `X-Smart-LLMRouter-*` version headers.

Enterprise deployments can route to internally hosted vLLM or SGLang services by configuring them as OpenAI-compatible providers with private `/v1` base URLs. Validate the upstream `/v1/models` ID, a direct text completion, any intended tool-call path, and the same requests through the router before activating those targets in production groups. Keep model IDs, parser flags, chat templates, server versions, and rollback notes in deployment records. See `docs/SELF_HOSTED_UPSTREAMS.md`.

Use the same base URL for local CLIs:

```bash
unset ANTHROPIC_API_KEY
export ANTHROPIC_BASE_URL="$ROUTER_BASE_URL"
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"

export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
```

Codex provider base URL:

```text
https://your-router.example.com/v1
```

## Release E2E

Full release e2e is live and credit-consuming:

```bash
make e2e-live-full
make e2e-compose-live
```

These checks require live provider keys, actual local Claude Code and Codex CLIs, Docker, and Docker Compose. They verify real provider calls, CLI traffic through the router, Docker Compose startup, and Caddy proxying.
