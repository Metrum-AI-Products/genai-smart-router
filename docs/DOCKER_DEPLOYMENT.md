# Docker Compose Deployment

Docker packages are intended for AWS EC2 or similar hosts where the source tree is not present and no image registry is required.

## Build Package

From the development machine:

```bash
make package-docker-all
```

This creates:

```text
dist/smart-llmrouter-<version>-docker-linux-amd64.tar.gz
dist/smart-llmrouter-<version>-docker-linux-arm64.tar.gz
```

Each package contains:

```text
images/smart-llmrouter-<version>-linux-<arch>.tar
compose/docker-compose.yml
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

- Create or update the `A` record for `llm-api-engg.metrum.ai` to the EC2 public IPv4 address.
- Create an `AAAA` record only if the instance has a working public IPv6 address.
- DNS can stay in DigitalOcean; it only needs to point to the AWS instance.

Install Docker and the Compose plugin on the host, then unpack:

```bash
sudo mkdir -p /opt/smart-llmrouter
sudo tar -C /opt/smart-llmrouter --strip-components=1 -xzf smart-llmrouter-<version>-docker-linux-amd64.tar.gz
cd /opt/smart-llmrouter
```

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

Edit `compose/config/config.yaml` for container paths:

```yaml
server:
  listen: ":8080"
  logging:
    path: /app/logs/requests.jsonl
  usage_db:
    enabled: true
    path: /app/state/usage.sqlite

state_path: /app/state/router-state.json
```

Edit `compose/config/env.json` with provider keys. Do not commit or publish this file.

The response cache is in-memory inside the router container. Restarting the container clears cached responses. Cache hits are shared across caller tokens, return fresh router-owned response IDs, and do not consume provider credits or persisted caller token quota.

Generate a caller token:

```bash
docker run --rm --entrypoint /app/bin/router-token-gen smart-llmrouter:<version>-linux-amd64 generate \
  --user chetan \
  --project metrum-insights \
  --env dev \
  --allow default,fast,big-coder
```

Append the generated caller config to `compose/config/config.yaml` and save the printed `token` for clients.

Review `compose/.env`:

```bash
SMART_LLMROUTER_VERSION=<version>-linux-amd64
ROUTER_HOSTNAME=llm-api-engg.metrum.ai
CADDY_EMAIL=chetan@metrum.ai
CADDY_HTTP_PORT=80
CADDY_HTTPS_PORT=443
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
curl https://llm-api-engg.metrum.ai/healthz
curl -H "Authorization: Bearer $ROUTER_TOKEN" https://llm-api-engg.metrum.ai/v1/models
```

Generate a markdown usage report on the host from the running compose data:

```bash
docker compose run --rm --entrypoint /app/bin/router-usage-report router \
  --db /app/state/usage.sqlite \
  --log /app/logs/requests.jsonl \
  --since 24h \
  --out /app/logs/usage-24h.md
```

The report includes internal router API key usage by `token_id`/user/project/environment, external provider/model calls, token totals, cache hit/miss/bypass, attempts, fallbacks, status codes, latency, hourly usage, and daily usage. It does not include raw router tokens or provider API keys.

Supported router model groups:

```text
small      OpenAI GPT-5.4 nano/mini weighted toward nano for lowest cost and latency.
medium     OpenAI GPT-5.4 mini/full weighted toward mini for balanced work.
high       OpenAI GPT-5.4 full-first route for complex coding and professional work.
default    General-purpose weighted routing across configured providers.
fast       Lower-latency/cost weighted routing for everyday work.
big-coder  Coding-focused failover route; recommended for Claude Code and Codex.
```

Caller tokens are restricted by `callers[].allow`. Standard access is `default`, `fast`, and `small`; coding/premium access additionally includes `medium`, `high`, and `big-coder`. `/v1/models` only lists model groups allowed for the presented token, and disallowed requests return `403 model-not-allowed` before any upstream provider call.

Claude Code:

```bash
export ANTHROPIC_BASE_URL=https://llm-api-engg.metrum.ai
export ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN"
claude --bare --print --model default "Reply with exactly: router claude ok"
```

Codex:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  -c 'model="default"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="https://llm-api-engg.metrum.ai/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router codex ok" </dev/null
```

The `exec` subcommand is required for `--ignore-user-config`, `--ephemeral`, `--ignore-rules`, and `--skip-git-repo-check`; those flags are not accepted by the top-level interactive `codex` command.

Interactive Codex uses top-level `codex`, without the `exec`-only flags:

```bash
export METRUM_ROUTER_KEY="$ROUTER_TOKEN"
codex \
  -c 'model="default"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="https://llm-api-engg.metrum.ai/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"'
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
