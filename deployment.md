# Smart LLM Router Production Deployment

Last deployed: 2026-06-15

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

- Router package/image version: `harbor-go-sublist-docs-20260615-linux-amd64`
- Source commit: `a2cb540`
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
compose-router-1   smart-llmrouter:harbor-go-sublist-docs-20260615-linux-amd64
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

## 2026-06-14 OpenRouter GPT-OSS 120B Update

- Historical note: OpenRouter `openai/gpt-oss-120b:nitro` was briefly added to production and reference configs.
- It is no longer active in the 2026-06-15 routing policy because current production/reference groups intentionally exclude OpenAI and Anthropic model IDs.
- At the time, it was activated in `default`, `fast`, `small`, `medium`, and `high` with medium fallback weight while preserving 60% DeepSeek V4 Flash Nitro and 30% MiniMax-M3 anchor weights.
- Verified production OpenRouter direct smoke returned HTTP 200 for `openai/gpt-oss-120b:nitro`.
- Restarted the production router after backing up `config/config.yaml`.
- Verified `/readyz`, local/remote production config SHA-256 parity, and authenticated smokes for `default`, `fast`, `small`, `medium`, and `high`.

## 2026-06-15 OpenRouter/MiniMax/Kimi-Only Routing Policy

- Previous deployed image `smart-llmrouter:no-openai-anthropic-20260615-linux-amd64`.
- Updated production and reference configs so active groups use only OpenRouter, MiniMax, and Kimi/Moonshot upstream models. OpenAI and Anthropic model IDs are not active.
- Kept caller API compatibility for both Codex/OpenAI Responses and Claude Code/Anthropic Messages. Dedicated tool smoke groups now route to MiniMax-M3: `agent-tools-smoke` over Responses and `claude-tools-smoke` over Anthropic-compatible Messages.
- Added Kimi Anthropic-compatible default thinking injection for tool-capable requests and stripped forced Anthropic `tool_choice` when needed for Kimi compatibility.
- Expanded the `chetan` production caller token to all configured groups: `default`, `fast`, `small`, `medium`, `high`, `big-coder`, `agent-tools-smoke`, and `claude-tools-smoke`.
- Verified direct provider smokes: MiniMax Responses tool request HTTP 200, MiniMax Anthropic-compatible tool request HTTP 200, and Kimi Anthropic-compatible thinking/tool request HTTP 200.
- Verified production `/readyz`, local/remote production config SHA-256 parity, `/v1/models` for the `chetan` token, authenticated `big-coder` chat, authenticated `agent-tools-smoke` Responses tool request, and authenticated `claude-tools-smoke` Messages tool request.

## 2026-06-15 OpenRouter Tool-Compatible Routes

- Added OpenRouter as both an OpenAI Responses-compatible provider (`openrouter_responses`) and an Anthropic Messages-compatible bearer-auth provider (`openrouter_anthropic`).
- Validated OpenRouter Nitro model refs in production/reference configs and dropped Qwen from active routes after direct Harbor/Claude Code showed malformed blank tool names. This was a routing decision from the first OpenRouter tool-validation pass, not a permanent provider policy.
- Added dedicated smoke groups `agent-tools-smoke-openrouter` and `claude-tools-smoke-openrouter` for Codex and Claude Code file/tool validation against OpenRouter DeepSeek V4 Flash Nitro.
- Historical direct OpenRouter Harbor checks without the router: Qwen3 Coder 30B Nitro returned blank Claude Code tool names and scored reward `0.0`; OpenRouter Kimi K2.7 Code Nitro scored reward `1.0`; OpenRouter DeepSeek V4 Flash Nitro scored reward `1.0`.
- Deployed production config SHA-256 `5ee667ba6e71677ddcb6d9a263206299b58eeeb01713552c8e08d24bfd4fc62c`; hosted API smokes for `agent-tools-smoke-openrouter` and `claude-tools-smoke-openrouter` both resolved to DeepSeek V4 Flash Nitro.
- Verified local Codex CLI and Claude Code CLI file-write smokes against hosted DeepSeek OpenRouter smoke groups after deployment.

## 2026-06-15 Cleaned Harbor-Validated Routing

- Removed OpenRouter Kimi K2.7 Code Nitro from local and production active configs. Direct Moonshot AI `kimi-k2.7-code` remains active.
- Removed unvalidated OpenRouter candidate models from local and production active configs. The cleaned active set is MiniMax-M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, OpenRouter Gemma 4 26B Nitro, and low-weight original OpenAI GPT-5.5 for non-tool traffic.
- Original Anthropic remains supported by the adapter, but it is not active because no `ANTHROPIC_API_KEY` is present in local or production `env.json`.
- Deployed production config SHA-256 `929af45877e905414d893c8c5ba91369123a6b3b0cd61c8f2a0aafbc5050baf6` with 12 Harbor caller tokens for case `harbor-cleaned-20260615t031649z`.
- Ran Harbor `aider/polyglot_python_two-bucket` through Codex CLI and Claude Code across `default`, `fast`, `small`, `medium`, `high`, and `big-coder`. All final cells passed with reward `1.0`; Codex `medium` required a clean rerun after the first attempt produced a passing artifact but exited nonzero.
- Scoped production usage for the cleaned run: 89 requests, 1,457,139 total router-tracked tokens, 1,149,320 input tokens, 178,283 output tokens, 86 upstream attempts, 5 fallbacks, and 82 streaming requests.
- Rebuilt and deployed image `smart-llmrouter:cleaned-harbor-20260615-linux-amd64`; production `/readyz` passed and deployed config SHA-256 stayed `929af45877e905414d893c8c5ba91369123a6b3b0cd61c8f2a0aafbc5050baf6`.

## 2026-06-15 Harbor Current-Policy Validation

- Registered 12 one-group Harbor caller tokens for case environment `case-current-policy-20260615t004637z`.
- Ran Harbor task `aider/polyglot_python_two-bucket` through hosted production with Codex CLI and Claude Code CLI across `default`, `fast`, `small`, `medium`, `high`, and `big-coder`.
- Result: 12/12 Harbor cells passed with reward `1.0` and zero Harbor exceptions.
- Generated production Postgres usage report at `/opt/smart-llmrouter/compose/logs/harbor-agentic-case-current-policy-20260615t004637z.md` and copied the report into the local case-study artifacts.
- Observed 121 router requests, 1,682,613 total tokens, 121 cache bypasses, and one upstream `502` during `claude-code/high`; the router recorded one fallback and the Harbor trial still passed.
- Updated `docs/harbor-case-study.md` with the current production measurement.

## 2026-06-15 Embedded Customer Docs Deployment

- Deployed image `smart-llmrouter:metrum-docs-20260615-linux-amd64` from source commit `34e26b3`.
- The router binary embeds the Metrum-themed Docusaurus docs site. Browser requests to `/` return `307` to `/docs/`.
- Verified production `/readyz`, `/docs/` branded HTML, Metrum logo/static asset serving, `/v1/unknown` remains `404`, and authenticated `/v1/models` still returns API JSON.
- Production config was not changed; only `SMART_LLMROUTER_VERSION` in compose `.env` was updated after backing up the previous `.env`.

## 2026-06-15 Customer Docs Charts And Case Study Update

- Deployed image `smart-llmrouter:docs-charts-routing-20260615-linux-amd64` from source commit `43e1658`.
- Added Docusaurus Mermaid support and Metrum-themed Chart.js case-study charts.
- Expanded the Harbor case study with models used, tokenomics, GPT 5.5 and Opus 4.8 comparison pricing, and cost-savings calculations for all runs, Codex CLI, and Claude Code CLI.
- Fixed embedded docs serving so extensionless Docusaurus pages such as `/docs/solution-brief` and `/docs/evaluation/harbor-case-study` resolve to their generated `.html` pages before SPA fallback.
- Verified production `/readyz`, Harbor case-study HTML with cost tables and chart canvases, solution brief page routing, and `/v1/unknown` remains `404`.

## 2026-06-15 Dynamic Deployment-Origin Docs Update

- Deployed image `smart-llmrouter:docs-dynamic-origin-20260615-linux-amd64` from source commit `b0ed36c`.
- Customer-facing Docusaurus pages no longer hardcode the Metrum internal production host in Codex CLI, Claude Code CLI, or hosted quickstart examples.
- Embedded docs now explain that they are served from each customer's hosted router instance and that examples render using the browser origin for that deployment.
- Raw prerendered HTML still contains the neutral fallback `https://your-router.example.com`; hydrated browser pages replace it with `window.location.origin`.
- Verified production `/readyz`, docs page delivery for Codex CLI and hosted quickstart pages, and `/v1/unknown` remains `404`.

## 2026-06-15 Harbor Go Sublist Case Study Docs Update

- Deployed image `smart-llmrouter:harbor-go-sublist-docs-20260615-linux-amd64` from source commit `a2cb540`.
- Ran Harbor task `aider/polyglot_go_sublist` through hosted production using both Codex CLI and Claude Code CLI on the `default` model group.
- Final clean case `harbor-go-sublist-default-20260615T063200Z` passed for both agents with reward `1.0` and zero Harbor exceptions.
- Scoped production usage report: 19 requests, 0 errors, 189,764 total router-tracked tokens, 146,229 input tokens, 4,879 output tokens, 16 upstream attempts, 0 fallbacks, and 19 cache bypasses.
- Updated hosted Docusaurus Harbor case study with the Go sublist task, results, provider usage, tokenomics, cache behavior, caller IP, and GPT 5.5 / Opus 4.8 / Metrum cost comparison.
- Verified production `/readyz`, hosted Harbor case-study page content, chart canvases for Case Study #2, and `/v1/unknown` remains `404`.

## 2026-06-15 Lucas Project Caller Tokens

- Created and registered two new production caller tokens for user `lucas`.
- Projects: `growth_stack` and `openfang_daily_reports`.
- Allowed model groups for both keys: `default`, `fast`, and `small`.
- Raw tokens were saved locally in `ROUTER_TOKENS_LUCAS_PROJECTS_20260615.txt`; this file is ignored by git and must be shared only through a secure channel.
- Production router was restarted after the caller config update.
- Verified production `/readyz` and `/v1/models` for both new tokens; both tokens returned only `default`, `fast`, and `small`.

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
small      DeepSeek V4 Flash Nitro 61%, MiniMax-M3 30%, Gemma 4%, Kimi 4%, OpenAI GPT-5.5 1% non-tool.
medium     DeepSeek V4 Flash Nitro 56%, MiniMax-M3 27%, Gemma 8%, Kimi 8%, OpenAI GPT-5.5 1% non-tool.
high       DeepSeek V4 Flash Nitro 51%, MiniMax-M3 28%, Gemma 10%, Kimi 10%, OpenAI GPT-5.5 1% non-tool.
default    DeepSeek V4 Flash Nitro 56%, MiniMax-M3 28%, Gemma 8%, Kimi 7%, OpenAI GPT-5.5 1% non-tool.
fast       DeepSeek V4 Flash Nitro 61%, MiniMax-M3 28%, Gemma 5%, Kimi 5%, OpenAI GPT-5.5 1% non-tool.
big-coder  Code-heavy route: MiniMax-M3 49%, direct Kimi 30%, DeepSeek V4 Flash Nitro 20%, OpenAI GPT-5.5 1% non-tool.
```

Clients set one of those router model group names as the model. The router chooses the actual upstream provider/model behind the group. Current active production/reference targets are limited to Harbor-validated OpenRouter, MiniMax, Kimi/Moonshot, and low-weight original OpenAI non-tool targets. Anthropic original-provider routing is supported but inactive until an Anthropic key is present and validated.

Production caller tokens are restricted by `callers[].allow`. Standard access is `default`, `fast`, and `small`; coding/premium access additionally includes `medium`, `high`, and `big-coder`. `/v1/models` only lists the groups allowed for the presented token, and disallowed requests return `403 model-not-allowed` before any upstream provider call.

Unauthenticated health:

```bash
curl -fsS https://llm-api-engg.metrum.ai/healthz
```

Generate a production usage report on the instance:

```bash
cd /opt/smart-llmrouter/compose
dsn="$(sudo sed -n 's/^ROUTER_USAGE_DB_DSN=//p' .env | tail -n 1)"
sudo docker compose run --rm --entrypoint /app/bin/router-usage-report router \
  --driver postgres \
  --dsn "$dsn" \
  --since 24h \
  --out /app/logs/usage-24h.md
```

For a scoped report, add filters such as:

```bash
  --caller-project harbor-algotune-pca \
  --caller-environment case-current-policy-20260615t004637z \
  --resolved-group big-coder \
  --client codex
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
unset ANTHROPIC_API_KEY
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
high: 200 with gpt-5.5 at that time; this is no longer an active route under the 2026-06-15 policy
big-coder: weighted smoke selected gpt-5.5 and MiniMax-M3 at that time; current big-coder excludes OpenAI and Anthropic model IDs
Codex: router prod codex ok
```
