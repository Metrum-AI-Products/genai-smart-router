# Smart LLM Router Production Deployment

Last deployed: 2026-06-14

## Live Environment

- Public URL: `https://llm-api-engg.metrum.ai`
- Public IPv4: `100.30.225.66`
- AWS account: `121701826775`
- AWS region/AZ: `us-east-1` / `us-east-1b`
- EC2 instance: `i-0b6c6608d97119832`
- Security group: `sg-0876c70bac41d7d54` (`launch-wizard-9`)
- SSH user: `ubuntu`
- SSH key: `~/.ssh/chetan-jun-2026.pem`
- DNS: DigitalOcean `A` record for `llm-api-engg.metrum.ai` points to `100.30.225.66`; no `AAAA` record is configured.

## Deployed Version

- Router package/image version: `usage-caller-ip-20260614-linux-amd64`
- Source commit: local working tree deployment image with usage TPS reporting, cache snapshots, caller IP reporting, and Postgres usage DB
- Deployment root: `/opt/smart-llmrouter`
- Compose directory: `/opt/smart-llmrouter/compose`
- Router config: `/opt/smart-llmrouter/compose/config/config.yaml`
- Provider key file: `/opt/smart-llmrouter/compose/config/env.json`
- Routing script: `/opt/smart-llmrouter/compose/config/scripts/router.ts`
- Request log: `/opt/smart-llmrouter/compose/logs/requests.jsonl`
- Usage DB: Postgres compose service (`postgres:18-bookworm`), configured by `ROUTER_USAGE_DB_DSN` in `/opt/smart-llmrouter/compose/.env`
- State file: `/opt/smart-llmrouter/compose/state/router-state.json`
- Production caller token file: `/opt/smart-llmrouter/compose/ROUTER_TOKEN.txt`

Do not copy `env.json` or `ROUTER_TOKEN.txt` into git, chat, tickets, or logs. The token file is stored on the host as `ubuntu:ubuntu` with mode `0600`.

## Host Runtime

Docker was installed from Docker's official Ubuntu apt repository, not Ubuntu `docker.io`.

Installed runtime at deployment time:

```text
Docker 29.5.3
Docker Compose v5.1.4
```

Services:

```bash
ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66
cd /opt/smart-llmrouter/compose
sudo docker compose ps
sudo docker compose logs --tail=100 router
sudo docker compose logs --tail=100 caddy
```

Expected containers:

```text
compose-router-1   smart-llmrouter:usage-caller-ip-20260614-linux-amd64
compose-postgres-1 postgres:18-bookworm
compose-caddy-1    caddy:2-alpine
```

## TLS And Networking

Caddy terminates TLS and reverse-proxies to the private Compose service `router:8080`.

Current AWS inbound rules:

```text
22/tcp   0.0.0.0/0
80/tcp   0.0.0.0/0
443/tcp  0.0.0.0/0
```

Port `80` is required for Caddy automatic HTTPS redirects and HTTP-01 fallback. The first certificate was issued successfully by Let's Encrypt using `tls-alpn-01` on port `443`.

Caddy stores ACME account/cert state in the persistent Docker volume `compose_caddy_data`. Do not remove that volume during normal restarts.

Recommended hardening still pending: restrict `22/tcp` to trusted admin IPs instead of `0.0.0.0/0`.

## 2026-06-14 Usage Reporting Update

- Deployed image: `smart-llmrouter:usage-tps-postgres-20260614-linux-amd64`.
- Added `postgres:18-bookworm` as the usage DB service with the `compose_postgres_data` volume.
- Moved old SQLite usage files under `compose/state/usage-sqlite-backup-<timestamp>/`.
- Verified `https://llm-api-engg.metrum.ai/readyz`, an authenticated `fast` chat completion, Postgres `request_usage` table creation, `/metrics` throughput/cache series, and a Postgres-backed usage report at `logs/usage-postgres-smoke.md`.

## 2026-06-14 Caller IP Reporting Update

- Deployed image: `smart-llmrouter:usage-caller-ip-20260614-linux-amd64`.
- Added scalar `caller_ip` capture from `X-Forwarded-For`, `X-Real-IP`, or direct remote address.
- Reset the production Postgres usage DB volume after backing it up, so new production reports start clean with caller IP fields from the first row.
- Reports now include `Usage By Caller IP`, `Hourly Usage By Caller IP`, and caller IP in the per-request throughput table.

## 2026-06-14 MiniMax-M3 Weight Update

- Updated production and reference configs so general router model groups are `weighted` with OpenRouter DeepSeek V4 Flash Nitro at 60% and MiniMax `MiniMax-M3` at 30%; `big-coder` is limited to MiniMax-M3 50%, Kimi 30%, and DeepSeek V4 Flash Nitro 20%.
- Verified MiniMax-M3 direct provider smoke returned HTTP 200.
- Restarted the production router after backing up `config/config.yaml`.
- Verified `/readyz`, local/remote production config SHA-256 parity, and authenticated smokes for `default`, `fast`, `small`, `medium`, `high`, and `big-coder`; production smoke requests completed after the weighting update.

## 2026-06-14 DeepSeek/MiniMax/Kimi Weight Update

- Updated production and reference configs so `default`, `fast`, `small`, `medium`, and `high` route 60% to OpenRouter `deepseek/deepseek-v4-flash:nitro`, 30% to MiniMax `MiniMax-M3`, and 10% across remaining fallback targets.
- Updated `big-coder` to exactly three targets: MiniMax-M3 50%, Kimi `kimi-k2.7-code` 30%, and OpenRouter `deepseek/deepseek-v4-flash:nitro` 20%.
- Verified direct provider smokes: MiniMax-M3 HTTP 200, Kimi `kimi-k2.7-code` HTTP 200, and production OpenRouter DeepSeek V4 Flash Nitro HTTP 200.
- Restarted the production router after backing up `config/config.yaml`.
- Verified `/readyz`, local/remote production config SHA-256 parity, and authenticated smokes for all six router model groups.

## Operations

Restart:

```bash
ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66
cd /opt/smart-llmrouter/compose
sudo docker compose restart
```

Stop/start:

```bash
sudo docker compose down
sudo docker compose up -d
```

Deploy a new amd64 package:

```bash
make package-docker GOOS=linux GOARCH=amd64
scp -i ~/.ssh/chetan-jun-2026.pem dist/smart-llmrouter-<version>-docker-linux-amd64.tar.gz ubuntu@100.30.225.66:/tmp/
ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66
sudo mv /opt/smart-llmrouter /opt/smart-llmrouter.backup.$(date +%Y%m%d%H%M%S)
sudo mkdir -p /opt/smart-llmrouter
sudo tar -C /opt/smart-llmrouter --strip-components=1 -xzf /tmp/smart-llmrouter-<version>-docker-linux-amd64.tar.gz
cd /opt/smart-llmrouter
sudo docker load -i images/smart-llmrouter-<version>-linux-amd64.tar
```

After unpacking a new package, copy forward the live config, env, state, logs, and token from the backup unless intentionally rotating them:

```bash
sudo cp -a /opt/smart-llmrouter.backup.<timestamp>/compose/config /opt/smart-llmrouter/compose/
sudo cp -a /opt/smart-llmrouter.backup.<timestamp>/compose/state /opt/smart-llmrouter/compose/
sudo cp -a /opt/smart-llmrouter.backup.<timestamp>/compose/logs /opt/smart-llmrouter/compose/
sudo cp -a /opt/smart-llmrouter.backup.<timestamp>/compose/ROUTER_TOKEN.txt /opt/smart-llmrouter/compose/
cd /opt/smart-llmrouter/compose
sudo docker compose up -d
```

## Smoke Tests

Supported production router model groups:

```text
small      DeepSeek V4 Flash Nitro 60% and MiniMax-M3 30%, with low-latency fallback targets for routine work.
medium     DeepSeek V4 Flash Nitro 60% and MiniMax-M3 30%, with balanced fallback targets for general work.
high       DeepSeek V4 Flash Nitro 60% and MiniMax-M3 30%, with premium fallback targets for complex work.
default    DeepSeek V4 Flash Nitro 60% and MiniMax-M3 30%, with broad configured provider fallbacks.
fast       DeepSeek V4 Flash Nitro 60% and MiniMax-M3 30%, with lower-latency fallback targets for everyday work.
big-coder  Code-heavy route: MiniMax-M3 50%, Kimi 30%, and DeepSeek V4 Flash Nitro 20%; recommended for Claude Code and Codex.
```

Clients set one of those router model group names as the model. The router chooses the actual upstream provider/model behind the group. Active validated targets include OpenAI `gpt-5.5`, `gpt-5.4-nano`, `gpt-5.4-mini`, and `gpt-5.4`; MiniMax `MiniMax-M3` and `MiniMax-M2.7-highspeed`; Groq `llama-3.1-8b-instant`, `groq/compound-mini`, `qwen/qwen3-32b`, and `llama-3.3-70b-versatile`; and configured OpenRouter Nitro targets. Direct validation showed production has access to `gpt-5.5`; `gpt-5.5-pro` remains catalog-only until the OpenAI project is entitled for it.

Production caller tokens are restricted by `callers[].allow`. Standard access is `default`, `fast`, and `small`; coding/premium access additionally includes `medium`, `high`, and `big-coder`. `/v1/models` only lists the groups allowed for the presented token, and disallowed requests return `403 model-not-allowed` before any upstream provider call.

Unauthenticated health:

```bash
curl -fsS https://llm-api-engg.metrum.ai/healthz
```

Generate a production usage report on the instance:

```bash
cd /opt/smart-llmrouter/compose
set -a; . ./.env; set +a
sudo docker compose run --rm --entrypoint /app/bin/router-usage-report router \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out /app/logs/usage-24h.md
```

Authenticated models:

```bash
ROUTER_TOKEN="$(ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cat /opt/smart-llmrouter/compose/ROUTER_TOKEN.txt')"
curl -fsS -H "Authorization: Bearer $ROUTER_TOKEN" https://llm-api-engg.metrum.ai/v1/models
```

Direct OpenAI-compatible chat:

```bash
curl -fsS https://llm-api-engg.metrum.ai/v1/chat/completions \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"model":"big-coder","messages":[{"role":"user","content":"Reply with exactly: router ok"}]}'
```

Claude Code:

```bash
ANTHROPIC_BASE_URL=https://llm-api-engg.metrum.ai \
ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN" \
claude --bare --print --model big-coder "Reply with exactly: router prod claude ok"
```

Do not set `ANTHROPIC_API_KEY` for router traffic. Claude Code uses `ANTHROPIC_AUTH_TOKEN` as a bearer token for gateways/proxies, while `ANTHROPIC_API_KEY` is for direct Anthropic API keys.

For Claude Code, change `--model big-coder` to `--model small`, `medium`, `high`, `default`, or `fast` to use another route.

The caller token must allow the selected model group.

Codex:

```bash
METRUM_ROUTER_KEY="$ROUTER_TOKEN" codex exec --ignore-user-config --ephemeral \
  --ignore-rules \
  --skip-git-repo-check \
  -c 'model="big-coder"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="https://llm-api-engg.metrum.ai/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"' \
  "Reply with exactly: router prod codex ok" </dev/null
```

The `exec` subcommand is required for `--ignore-user-config`, `--ephemeral`, `--ignore-rules`, and `--skip-git-repo-check`; those flags are not accepted by the top-level interactive `codex` command.

Interactive Codex uses top-level `codex`, without the `exec`-only flags:

```bash
METRUM_ROUTER_KEY="$ROUTER_TOKEN" codex \
  -c 'model="big-coder"' \
  -c 'model_provider="metrum-router"' \
  -c 'model_providers.metrum-router.name="Metrum Router"' \
  -c 'model_providers.metrum-router.base_url="https://llm-api-engg.metrum.ai/v1"' \
  -c 'model_providers.metrum-router.env_key="METRUM_ROUTER_KEY"' \
  -c 'model_providers.metrum-router.wire_api="responses"'
```

For Codex, change `-c 'model="big-coder"'` to `small`, `medium`, `high`, `default`, or `fast` to use another route.

Historical validation during the initial deployment:

```text
healthz: 200
/v1/models: 200 with default, fast, big-coder
Claude Code: router prod claude ok
high: 200 with gpt-5.5 at that time
big-coder: weighted smoke selected gpt-5.5 and MiniMax-M3 at that time
Codex: router prod codex ok
```
