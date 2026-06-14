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

- Router package/image version: `9063ebe-linux-amd64`
- Source commit: `9063ebe Add cache-safe response IDs and docker e2e packaging`
- Deployment root: `/opt/smart-llmrouter`
- Compose directory: `/opt/smart-llmrouter/compose`
- Router config: `/opt/smart-llmrouter/compose/config/config.yaml`
- Provider key file: `/opt/smart-llmrouter/compose/config/env.json`
- Routing script: `/opt/smart-llmrouter/compose/config/scripts/router.ts`
- Request log: `/opt/smart-llmrouter/compose/logs/requests.jsonl`
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
compose-router-1   smart-llmrouter:9063ebe-linux-amd64
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
default    General-purpose weighted routing across configured providers.
fast       Lower-latency/cost weighted routing for everyday work.
big-coder  Coding-focused failover route; recommended for Claude Code and Codex.
```

Clients set one of those router model group names as the model. The router chooses the actual upstream provider/model behind the group.

Unauthenticated health:

```bash
curl -fsS https://llm-api-engg.metrum.ai/healthz
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

For Claude Code, change `--model big-coder` to `--model default` or `--model fast` to use another route.

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

For Codex, change `-c 'model="big-coder"'` to `default` or `fast` to use another route.

Validated during deployment:

```text
healthz: 200
/v1/models: 200 with default, fast, big-coder
Claude Code: router prod claude ok
Codex: router prod codex ok
```
