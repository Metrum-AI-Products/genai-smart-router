# Smart LLM Router Deployment

This project is packaged as a binary distribution. A deployment host does not need the Go toolchain or source tree.

## Package Contents

`make package-all` creates Linux x86_64 and arm64 tarballs under `dist/`:

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
caddy/Caddyfile
```

The config and routing script are packaged together so this command works after unpacking:

```bash
bin/router --config config/config.yaml
```

`config/config.yaml` can keep `script: scripts/router.ts` because script paths are resolved relative to the config file.

Docker Compose packages are built separately:

```bash
make package-docker-all
```

Use `docs/DOCKER_DEPLOYMENT.md` when deploying the packaged Docker image tarball plus Caddy compose stack to AWS EC2 or a similar host.

## Internal Development Host

Initial internal development deployment target:

```text
llm-api-engg.metrum.ai
```

The service is externally reachable, but model, usage, and metrics endpoints still require a valid router caller token. Caddy terminates TLS and reverse-proxies to the router on localhost.

DNS is managed in DigitalOcean. Create or update an `A` record for `llm-api-engg.metrum.ai` pointing to the public IPv4 address of the deployment host. Add an `AAAA` record only if the host has working public IPv6.

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

The response cache is process-local. Restarting the router clears cached responses. Cache hits are shared across caller tokens, return fresh router-owned response IDs, and do not consume provider credits or persisted caller token quota.

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

Edit `/opt/smart-llmrouter/config/config.yaml`:

```yaml
server:
  listen: "127.0.0.1:8080"
  logging:
    path: /var/log/smart-llmrouter/requests.jsonl

state_path: /var/lib/smart-llmrouter/router-state.json
```

Edit `/opt/smart-llmrouter/config/env.json` with provider keys such as `OPENAI_API_KEY`, `OPENROUTER_API_KEY`, `MOONSHOT_API_KEY`, `MINIMAX_API_KEY`, and `XAI_API_KEY`.

Generate a caller token and append the generated caller block to `config.yaml`:

```bash
/opt/smart-llmrouter/bin/router-token-gen generate \
  --user chetan \
  --project metrum-insights \
  --env dev \
  --allow default,fast,big-coder
```

Save the printed `token` value for the client. The router config stores only `token_sha256` and `token_id`.

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

The Caddyfile terminates TLS for `llm-api-engg.metrum.ai` and proxies to `127.0.0.1:8080`. Caddy obtains and renews public certificates automatically after DigitalOcean DNS points the hostname to the host and ports `80` and `443` are reachable.

Reload Caddy:

```bash
sudo caddy validate --config /etc/caddy/Caddyfile
sudo systemctl reload caddy
```

## Smoke Test

From a client machine:

```bash
curl https://llm-api-engg.metrum.ai/healthz
curl -H "Authorization: Bearer $ROUTER_TOKEN" https://llm-api-engg.metrum.ai/v1/models
```

Use the same base URL for local CLIs:

```bash
export ANTHROPIC_BASE_URL=https://llm-api-engg.metrum.ai
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"

export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
```

Codex provider base URL:

```text
https://llm-api-engg.metrum.ai/v1
```

## Release E2E

Full release e2e is live and credit-consuming:

```bash
make e2e-live-full
make e2e-compose-live
```

These checks require live provider keys, actual local Claude Code and Codex CLIs, Docker, and Docker Compose. They verify real provider calls, CLI traffic through the router, Docker Compose startup, and Caddy proxying.
