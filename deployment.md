# Smart LLM Router Production Deployment

Last deployed: 2026-06-19

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

- Router package/image version: `5ec8319-linux-amd64`
- Source commit: `5ec8319`
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
compose-router-1   smart-llmrouter:bac7711-linux-amd64
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

## 2026-06-17 Build Version Metadata

- Deployed image/package: `smart-llmrouter:bac7711-linux-amd64`.
- Source commit: `bac7711`.
- Backup path: `/opt/smart-llmrouter.backup-version-metadata-20260617T061118Z`.
- Build metadata now includes a full UTC build timestamp, not only a date: `2026-06-17T06:07:57Z`.
- Verified production `/readyz` and `/version` return version `bac7711`, commit `bac7711`, and build date `2026-06-17T06:07:57Z`.
- Verified hosted docs responses include `X-Smart-LLMRouter-Version`, `X-Smart-LLMRouter-Commit`, and `X-Smart-LLMRouter-Build-Date`; the rendered docs badge appears on `/docs/overview` with the same full timestamp.
- Verified all packaged CLI binaries report `--version` with the same metadata: `router`, `router-token-gen`, and `router-usage-report`.
- Verified authenticated `/metrics` exposes `smart_llmrouter_build_info` with version, commit, build timestamp, Go version, OS, and architecture labels.
- Verified authenticated `/v1/models` keeps OpenAI-compatible response shape and does not include router version fields.
- Verified an authenticated `fast` chat completion succeeded after deployment.

## 2026-06-17 TypeScript Routing Docs And Policy Helpers

- Deployed image/package: `smart-llmrouter:7f121b6-linux-amd64`.
- Source commit: `7f121b6`.
- Backup path: `/opt/smart-llmrouter.backup-ts-routing-docs-20260617T050212Z`.
- Added router support for bundled TypeScript relative imports and opt-in external policy calls through `router.fetchJSON`.
- Added per-script-group `script_http` config with deployment-owned `allow_hosts`, timeout/response-size limits, and env-expanded headers for policy-service auth.
- Updated hosted Docusaurus docs, internal docs, deployment docs, and `config.example.yaml` to cover admin config, proxy-user behavior, imports/dependencies, external-policy calls, and the tested prompt-size routing example.
- Verified `rtk go test ./cmd/... ./internal/...`, `rtk make docs-build`, production `/readyz`, hosted `/docs/configuration/routing-typescript`, authenticated `/v1/models`, and an authenticated `fast` chat completion routed to `deepseek/deepseek-v4-flash:nitro`.

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
- Removed unvalidated OpenRouter candidate models from local and production active configs. Historical active set on 2026-06-15 was MiniMax-M3, direct Moonshot Kimi K2.7 Code, OpenRouter DeepSeek V4 Flash Nitro, OpenRouter Gemma 4 26B Nitro, and low-weight original OpenAI GPT-5.5 for non-tool traffic; the OpenAI fallback was superseded by GPT-5.4 Nano on 2026-06-17.
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

## 2026-06-15 High Group GPT-5.5 Weight Update

Historical note, superseded on 2026-06-17 by the GPT-5.4 Nano fallback policy.

- Updated the production `high` group standard/non-tool routing pool.
- Increased OpenAI `gpt-5.5` from 1% to 10%.
- Reduced OpenRouter `google/gemma-4-26b-a4b-it:nitro` from 10% to 1%.
- Final `high` standard pool: DeepSeek V4 Flash Nitro 51%, MiniMax-M3 28%, Gemma 4 26B Nitro 1%, Kimi K2.7 Code 10%, GPT-5.5 10%.
- Tool-only routing weights were not changed.
- Verified production `/readyz` and pulled the live remote config back to confirm the updated weights.

## 2026-06-15 Caller Allow-List Expansion

- Updated all 92 production caller records so each key can access `fast`, `small`, `medium`, `high`, and `big-coder`.
- Existing allowed groups such as `default` and tool smoke groups were preserved.
- Local production reference config was updated to match the live production config.
- Restarted the production router after backing up `config/config.yaml`.
- Verified production `/readyz` and pulled the live remote config back to confirm zero callers are missing the requested groups.

## 2026-06-15 Medium/Fast/Big-Coder Routing Weight Update

Historical note, superseded on 2026-06-17 by the GPT-5.4 Nano fallback policy.

- Added OpenAI `gpt-5.4-nano` to the production OpenAI provider catalog.
- Updated `medium` standard/non-tool pool to DeepSeek V4 Flash Nitro 53%, MiniMax-M3 35%, Gemma 4 26B Nitro 1%, Kimi K2.7 Code 8%, and GPT-5.5 3%.
- Updated `fast` standard/non-tool pool to DeepSeek V4 Flash Nitro 45%, MiniMax-M3 28%, Gemma 4 26B Nitro 1%, Kimi K2.7 Code 5%, GPT-5.5 1%, and GPT-5.4 Nano 20%.
- Updated `big-coder` standard/non-tool pool to DeepSeek V4 Flash Nitro 11%, MiniMax-M3 27%, Kimi K2.7 Code 17%, GPT-5.5 25%, and GPT-5.4 Nano 20%.
- Tool-only routing weights were not changed.
- Verified production `/readyz` and pulled the live remote config back to confirm all three groups sum to 100.
- Tested OpenAI `gpt-5.4-nano` directly through the production OpenAI key: HTTP 200, resolved model `gpt-5.4-nano-2026-03-17`, output text `nano-ok`.
- Tested hosted router selection with unique non-tool prompts. `fast` selected `openai/gpt-5.4-nano` multiple times with HTTP 200; `big-coder` selected `openai/gpt-5.4-nano` twice in 15 requests with HTTP 200.

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
small      DeepSeek V4 Flash Nitro 61%, MiniMax-M3 30%, Gemma 4%, Kimi 4%, OpenAI GPT-5.4 Nano 1% non-tool.
medium     DeepSeek V4 Flash Nitro 53%, MiniMax-M3 35%, Gemma 1%, Kimi 8%, OpenAI GPT-5.4 Nano 3% non-tool.
high       DeepSeek V4 Flash Nitro 51%, MiniMax-M3 28%, Gemma 1%, Kimi 10%, OpenAI GPT-5.4 Nano 10% non-tool.
default    DeepSeek V4 Flash Nitro 56%, MiniMax-M3 28%, Gemma 8%, Kimi 7%, OpenAI GPT-5.4 Nano 1% non-tool.
fast       DeepSeek V4 Flash Nitro 45%, MiniMax-M3 28%, Gemma 1%, Kimi 5%, OpenAI GPT-5.4 Nano 21% non-tool.
big-coder  Code-heavy route: DeepSeek V4 Flash Nitro 11%, MiniMax-M3 27%, Kimi K2.7 Code 17%, OpenAI GPT-5.4 Nano 35%, Z.AI GLM 5.2 Nitro 10%.
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
high: 200 with gpt-5.5 at that time; this is no longer an active route under the 2026-06-17 GPT-5.4 Nano fallback policy
big-coder: weighted smoke selected gpt-5.5 and MiniMax-M3 at that time; current big-coder uses OpenAI GPT-5.4 Nano as the low-weight non-tool OpenAI fallback alongside MiniMax, Kimi, and OpenRouter routes
Codex: router prod codex ok
```

### 2026-06-16 `kyai-judge` production group

Historical note, superseded on 2026-06-17 by the GPT-5.4 Nano fallback policy.

Added production-only model group `kyai-judge` as a static route to OpenAI `gpt-5.5`.
Allowed callers:

- `chetan-metrum-insights-prod`
- `jerin-metrum-insights-prod-kyai-judge`

Jerin's KYAI key is a dedicated token for the `metrum-insights` project and allows only `kyai-judge`. Raw token material is stored only in the ignored local credential file `ROUTER_TOKENS_JERIN_KYAI_JUDGE_20260616.txt`.

Validation:

```text
readyz: 200
chetan /v1/models includes kyai-judge
jerin KYAI /v1/models returns only kyai-judge
jitin /v1/models does not include kyai-judge
chetan kyai-judge chat: 200, upstream model gpt-5.5
jerin KYAI kyai-judge chat: 200, upstream model gpt-5.5
jitin kyai-judge chat: 403 model-not-allowed
```

### 2026-06-16 production quota increase

Raised all deployed caller entries to:

- Daily token quota: `50,000,000`
- Monthly token quota: `600,000,000`

Validation:

```text
readyz: 200
production callers: 94
unique daily token limits: [50000000]
unique monthly token limits: [600000000]
sharvesh-metrum-insights-prod: 20,020,045 / 50,000,000 daily tokens
```

### 2026-06-16 hosted docs API examples

Deployed image/package `smart-llmrouter:70aa94b-linux-amd64` from source commit `70aa94b`.

Hosted Docusaurus docs now include tested examples for:

- `/v1/models`
- `/v1/chat/completions`
- `/v1/responses`
- `/v1/messages`
- Python OpenAI SDK usage with `uv`

Validation:

```text
docs-build: passed
uv Python OpenAI SDK smoke: passed against https://llm-api-engg.metrum.ai
curl /v1/models: 200
curl /v1/chat/completions: 200
curl /v1/responses: 200
curl /v1/messages: 200
production readyz after deploy: 200
hosted docs contain OpenAI Responses section: yes
hosted docs contain Python Client section: yes
hosted docs contain uv add openai example: yes
hosted docs static HTML does not hardcode llm-api-engg.metrum.ai: yes
compose image: smart-llmrouter:70aa94b-linux-amd64
```

### 2026-06-17 `big-coder` GLM 5.2 Nitro production update

Historical note, superseded later on 2026-06-17 by the GPT-5.4 Nano fallback policy.

Added OpenRouter `z-ai/glm-5.2:nitro` to the production `big-coder` non-tool pool at 5% weight.

Current `big-coder` non-tool weights:

```text
OpenRouter DeepSeek V4 Flash Nitro 11%
MiniMax-M3 27%
Kimi K2.7 Code 17%
OpenAI GPT-5.5 15%
OpenAI GPT-5.4 Nano 25%
OpenRouter Z.AI GLM 5.2 Nitro 5%
```

Validation:

```text
direct OpenRouter z-ai/glm-5.2:nitro smoke: HTTP 200
direct GLM content smoke needed reasoning.max_tokens cap; tiny max_tokens runs spent the budget on reasoning
production compose config: passed
production readyz: 200
remote big-coder non-tool weight sum: 100
authenticated production big-coder chat smoke: 200
```

### 2026-06-17 `big-coder` OpenAI/GLM weight tune

Historical note, superseded later on 2026-06-17 by the GPT-5.4 Nano fallback policy.

Updated production `big-coder` non-tool weights:

```text
OpenRouter DeepSeek V4 Flash Nitro 11%
MiniMax-M3 27%
Kimi K2.7 Code 17%
OpenAI GPT-5.5 5%
OpenAI GPT-5.4 Nano 30%
OpenRouter Z.AI GLM 5.2 Nitro 10%
```

Validation:

```text
production compose config: passed
production readyz: 200
remote big-coder non-tool weight sum: 100
authenticated production big-coder chat smoke with max_tokens=64: 200, selected GLM, empty content due to reasoning budget
authenticated production big-coder chat smokes with max_tokens=1024: 8/8 HTTP 200; GLM selected twice and returned router ok both times
```

### 2026-06-17 upstream pricing/tool metadata rollout

Deployed image/package `smart-llmrouter:a59125a-linux-amd64` from source commit `a59125a`.

Runtime changes:

- Provider catalog entries now carry input/output dollars per million tokens, pricing source/update date, and dialect-specific tool support metadata.
- JSONL and Postgres usage rows store request-time input/output prices, calculated input/output/total USD cost, pricing source, and pricing update date.
- Usage reports now include cost summaries and cost columns.
- Hosted docs and CLI version endpoints report version `a59125a`, commit `a59125a`, and build timestamp `2026-06-17T14:16:07Z`.

Production backup:

```text
deployment backup: /opt/smart-llmrouter.backup-pricing-meta-20260617T141837Z
config backup: /opt/smart-llmrouter/compose/config/config.yaml.bak.20260617T141837Z
```

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 60 tests
rtk go test ./...: router/cmd packages passed; generated Harbor artifact packages still fail to compile as expected
docs-build/package build: passed
initial router restart issue: config installed 0600; fixed to 0644 and restarted router
production readyz: 200
production /version: a59125a, build_date 2026-06-17T14:16:07Z
hosted docs /docs/configuration/router-config: 200 with X-Smart-LLMRouter-* headers
authenticated /v1/models: 200, 12 model groups
usage DB migration: pricing/cost columns present on request_usage
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes
production chat smoke small max_tokens=128: 200, selected MiniMax-M3, non-empty content
production Responses tool smoke agent-tools-smoke: 200, selected MiniMax-M3, output type function_call
production Anthropic Messages tool smoke claude-tools-smoke: 200, selected MiniMax-M3, content type tool_use
production usage row cost check: latest small row recorded prices and nonzero total_cost_usd
production usage report --since 1h: rendered Cost summary and cost columns
```

### 2026-06-17 multimodal agent routing rollout

Deployed image/package `smart-llmrouter:081ebac-linux-amd64` from source commit `081ebac`.

Runtime changes:

- Added validated multimodal OpenRouter Responses and Anthropic Messages targets to coding-agent routing so deployment-defined coding groups can handle mixed text/image agent requests without requiring users to switch to a separate vision-only group.
- Added OpenRouter Anthropic-compatible VLM targets to the dedicated `vision` route while preserving existing image-capable targets and weights.
- Added clearer caller-facing errors:
  - `no-eligible-target` when the requested group has no upstream target for the requested dialect/tool/modality shape.
  - `upstream-failed` with safe request id, attempt count, and attempted provider/model details when eligible upstreams fail.
- Fixed Anthropic tool passthrough to normalize OpenAI-style `image_url` content into Anthropic `image.source` blocks before forwarding to Anthropic-compatible upstreams.

Production backups:

```text
multimodal config/package backup: /opt/smart-llmrouter.backup-multimodal-agent-20260617T170502Z
config backup: /opt/smart-llmrouter/compose/config/config.yaml.bak.multimodal-agent-20260617T170502Z
passthrough fix package backup: /opt/smart-llmrouter.backup-anthropic-image-passthrough-20260617T171201Z
```

Validation:

```text
rtk go test ./internal/router ./cmd/...: passed, 69 tests
docs-build/package build: passed; npm audit still reports 29 known docs-site dependency findings
production readyz: 200
production /version: 081ebac, build_date 2026-06-17T17:09:51Z
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 87377a68246309c1e635e789cd1146a9d37d47c71cbc28cbeefec88729c904b7
authenticated /v1/models: big-coder and vision advertise text+image and tool support
production /v1/responses big-coder image+function-tool smoke: 200, selected x-ai/grok-4.3, returned Rite Aid
production /v1/messages big-coder image+tool smoke: 200, selected anthropic/claude-sonnet-4.6, returned Rite Aid
Codex CLI production image smoke against big-coder: completed through router; selected an image-capable target but returned WELLNESS+ WITH PLENTI, so it validates CLI compatibility but not OCR quality for every weighted target
Claude Code CLI production text smoke against big-coder with --output-format json: completed, result router claude ok, modelUsage big-coder
```

### 2026-06-17 developer-accessible VLM config rollout

Config-only production update was first applied on image/package `smart-llmrouter:081ebac-linux-amd64`; then package `smart-llmrouter:e376623-linux-amd64` was deployed so hosted `/docs/` includes the Qwen3.6 Flash notes and the VLM/OCR quality distinction. An intermediate `242144f` package was deployed during validation and superseded by `e376623`.

Runtime change:

- Added the same validated OpenRouter Responses and Anthropic Messages multimodal `tool_only` targets to the common developer-accessible groups `default`, `fast`, `small`, `medium`, and `high`, in addition to the existing coding group. Text-only traffic still uses the normal weighted targets; image-bearing Codex/Claude-compatible tool requests can now stay on the caller's usual model group.

Validation:

```text
rtk go test ./internal/router ./cmd/...: passed, 69 tests
production readyz: 200
production /version: 081ebac, build_date 2026-06-17T17:09:51Z
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 87e12f70d353e78bbf987a70925c2d17445c4e8a2a064f1e7978c0c1ea338af8
authenticated /v1/models: default, fast, small, medium, high, and big-coder advertise text+image plus tool support
production /v1/responses small image+function-tool smoke: 200, selected x-ai/grok-4.3, returned Rite Aid
production /v1/messages small image+tool smoke: 200, selected anthropic/claude-sonnet-4.6, returned Rite Aid
production /v1/responses fast image+function-tool smoke: 200, selected MiniMax-M3, returned Rite Aid
production /v1/messages fast image+tool smoke: 200, selected qwen/qwen3.7-plus, returned Rite Aid
Codex CLI production image smoke against small: completed through router and returned Rite Aid
Claude Code CLI production text smoke against small with --output-format json: completed, result router claude ok, modelUsage small
```

### 2026-06-17 Qwen3.6 Flash Nitro config rollout

Config-only production update on image/package `smart-llmrouter:081ebac-linux-amd64`.

Runtime change:

- Added OpenRouter `qwen/qwen3.6-flash:nitro` catalog metadata with current OpenRouter pricing of $0.1875/M input tokens and $1.125/M output tokens, text/image/video input modalities, text output, and tool metadata for the validated skins.
- Added Qwen3.6 Flash at conservative non-tool weight to `default`, `fast`, `small`, `medium`, `high`, `big-coder`, and `vision`.
- Added text-only OpenAI Responses tool targets for Qwen3.6 Flash, plus Anthropic Messages tool targets with `default_thinking` compatibility, to common developer groups.
- Added static smoke groups `agent-tools-smoke-openrouter-qwen36`, `claude-tools-smoke-openrouter-qwen36`, and `vision-smoke-openrouter-qwen36` for exact-route validation.

Production backups:

```text
config/config.yaml.bak.qwen36-flash-20260617T183239Z
config/config.yaml.bak.qwen36-vision-smoke-20260617T183449Z
```

Validation:

```text
OpenRouter provider page checked 2026-06-17: qwen/qwen3.6-flash supports text/image/video input, text output, tools/tool_choice, 1M context, 65,536 max output, and $0.1875/M input plus $1.125/M output pricing
direct OpenRouter qwen/qwen3.6-flash:nitro text smoke: OK
direct OpenRouter qwen/qwen3.6-flash:nitro receipt-image smoke: returned Rite Aid
direct OpenRouter chat tool smoke: tool_call record_answer {"value":"OK"} with tool_choice auto; forced object tool_choice returned provider 400 while thinking mode was enabled
direct OpenRouter Responses text+function-tool smoke: function_call record_answer {"value":"OK"}
direct OpenRouter Anthropic Messages text+tool smoke: tool_use record_answer {"value":"OK"}
direct OpenRouter Anthropic Messages image+tool smoke: tool_use record_answer {"value":"Rite Aid"}
local router Responses static smoke agent-tools-smoke-openrouter-qwen36: 200, function_call record_answer {"value":"OK"}
local router Anthropic static smoke claude-tools-smoke-openrouter-qwen36: 200, tool_use record_answer {"value":"OK"}
rtk go test ./internal/router ./cmd/...: passed, 69 tests
production readyz after config updates: 200
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 0ef2b48205cdcb76a6ee7a3ffcddf82b8f7416ebff958f9bcce17e64f2bf3e68
production /v1/models: static Qwen3.6 smoke groups visible to the smoke token
production Responses static smoke agent-tools-smoke-openrouter-qwen36: 200, function_call record_answer {"value":"OK"}
production Anthropic static smoke claude-tools-smoke-openrouter-qwen36: 200, tool_use record_answer {"value":"OK"}
production vision static smoke vision-smoke-openrouter-qwen36: 200, selected qwen/qwen3.6-flash:nitro, processed the receipt image but returned Ralphs
production log monitor after rollout: earlier restart loop caused by config file mode 0600, fixed with chmod 0644; router recovered and no later startup errors in docker logs
production usage DB check after rollout: Qwen3.6 smoke rows recorded as HTTP 200 with target_model qwen/qwen3.6-flash:nitro
intermediate package deploy backup: /opt/smart-llmrouter.backup-qwen36-docs-20260617T184645Z
final package deploy backup: /opt/smart-llmrouter.backup-qwen36-docs-e376623-20260617T185109Z
production image after final docs/package deploy: smart-llmrouter:e376623-linux-amd64
production /version after final package deploy: e376623, build_date 2026-06-17T18:49:05Z
hosted docs /docs/configuration/image-analysis-vlm: 200 and contains qwen/qwen3.6-flash:nitro plus OCR-specific quality caveat
post-final-package production Responses static smoke agent-tools-smoke-openrouter-qwen36: 200, function_call record_answer {"value":"OK"}
post-final-package production Anthropic static smoke claude-tools-smoke-openrouter-qwen36: 200, tool_use record_answer {"value":"OK"}
```

Quality note:

```text
Keep Qwen3.6 Flash active for general image-capable routing because it accepts and analyzes image inputs. Do not treat the receipt smoke as an OCR-quality pass for exact merchant extraction; use OCR-specific route gates if exact answers are required.
```

### 2026-06-17 Baseten Nemotron config and docs rollout

Config-only production update was applied first on image/package `smart-llmrouter:e376623-linux-amd64`, then package `smart-llmrouter:17df92a-linux-amd64` was deployed so hosted `/docs/` includes Baseten provider configuration examples.

Runtime change:

- Added provider `baseten` with OpenAI-compatible Chat Completions base URL `https://inference.baseten.co/v1`.
- Added `nvidia/Nemotron-120B-A12B` as `nemotron-120b-a12b` with text input/output metadata, Baseten pricing metadata of $0.30/M input tokens and $0.75/M output tokens, and `tool_support.openai_chat: [tools, tool_choice]` based on direct tool-call validation.
- Added conservative low-weight Baseten text targets to `default`, `fast`, `small`, `medium`, `high`, and `big-coder`; `vision` was unchanged because this Baseten model is text-only.
- Added static smoke group `baseten-nemotron-smoke` for exact-route validation.

Production backups:

```text
config/config.yaml.bak.baseten-20260617T191035Z
config/env.json.bak.baseten-20260617T191035Z
/opt/smart-llmrouter.backup-baseten-docs-17df92a-20260617T191930Z
```

Validation:

```text
Baseten docs checked 2026-06-17: Model APIs are OpenAI-compatible at https://inference.baseten.co/v1; pricing page lists NVIDIA Nemotron 3 Super at $0.30/M input, $0.06/M cache input, and $0.75/M output.
direct Baseten nvidia/Nemotron-120B-A12B non-streaming chat smoke: OK with OpenAI-style usage
direct Baseten streaming chat smoke with stream_options.include_usage and continuous_usage_stats: OK, usage chunks seen
direct Baseten OpenAI Chat tool smoke: valid function tool_call get_weather
rtk go test ./internal/router ./cmd/...: passed, 69 tests
local router baseten-nemotron-smoke non-streaming chat smoke: 200, selected baseten nvidia/Nemotron-120B-A12B
local router baseten-nemotron-smoke streaming chat smoke: 200, downstream SSE completed
local usage log: target_provider baseten, target_model nvidia/Nemotron-120B-A12B, input price 0.3, output price 0.75, calculated total cost recorded
production config-only rollout: readyz 200 after env file mode fixed from 0600 to 0644 for container readability
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 743300c198db745fb68e947340d9880dda9e39f0ca3a349abedbc2ca36ea2600
production /v1/models: baseten-nemotron-smoke visible to operator smoke token
production baseten-nemotron-smoke non-streaming chat smoke before package deploy: 200, selected nvidia/Nemotron-120B-A12B
production baseten-nemotron-smoke streaming chat smoke before package deploy: 200, downstream SSE completed
production usage DB: recent baseten-nemotron-smoke rows recorded status 200, provider baseten, model nvidia/Nemotron-120B-A12B, input price 0.3, output price 0.75
production package deploy: smart-llmrouter:17df92a-linux-amd64
production /version after package deploy: 17df92a, build_date 2026-06-17T19:16:43Z
hosted docs /docs/configuration/router-config: 200 and contains Baseten provider example plus BASETEN_API_KEY
post-package production baseten-nemotron-smoke non-streaming chat smoke: 200, selected nvidia/Nemotron-120B-A12B
post-package production logs: router listening on :8080, no errors in recent logs
```

### 2026-06-18 Diagnostics and upstream timeout rollout

Package `smart-llmrouter:72fe882-linux-amd64` was deployed to production to improve timeout/error troubleshooting and add configurable upstream attempt caps.

Runtime change:

- Added relational diagnostic tables `request_attempts`, `request_trace_events`, and `request_errors`, keyed by `request_id`.
- Added structured per-attempt/error classification for upstream timeout, provider rate limit, upstream failure, no eligible target, and client cancellation paths.
- Added `server.upstream` and `server.diagnostics` config sections.
- Set production `medium` and `big-coder` `attempt_timeout_ms` to `180000`.

Production backups:

```text
/opt/smart-llmrouter.backup-diagnostics-72fe882-20260618T125752Z
config/config.yaml.bak.20260618T125752Z
```

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 70 tests
rtk go test ./...: known generated Harbor/job artifact package failures only
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 72fe882, build_date 2026-06-18T12:54:39Z
production /version after deploy: 72fe882, build_date 2026-06-18T12:54:39Z
hosted docs /docs/configuration/router-config: 200 with version headers for 72fe882
production config: diagnostics enabled, upstream timeout 600000 ms, medium/big-coder attempt_timeout_ms 180000
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 403384ac89fb339c63fb94f8933746096d6874d6ef4050617719ec9050f30a59
production medium chat smoke: 200, selected deepseek/deepseek-v4-flash:nitro, returned OK
production big-coder chat smoke: 200, selected nvidia/Nemotron-120B-A12B, returned OK
production usage DB: request_attempts, request_trace_events, and request_errors tables exist
production usage DB: recent medium/big-coder smokes recorded one request_attempt and four request_trace_events each
Codex CLI production smoke through router Responses API with model medium: returned OK
Claude Code CLI production smoke with model medium: JSON result OK and modelUsage present
Claude Code CLI production smoke with model big-coder: JSON result OK and modelUsage present
production cleanup: removed uploaded package/temp files; docker image/build-cache prune reclaimed about 1.5 GB; volumes were not pruned
```

### 2026-06-18 OpenAI Chat tool passthrough and Warp Agent rollout

Package `smart-llmrouter:2ccc852-linux-amd64` was deployed to production to support OpenAI Chat Completions tool passthrough for OpenAI-compatible agent clients such as Warp Agent.

Runtime change:

- Preserves OpenAI Chat `tools`, `tool_choice`, `parallel_tool_calls`, and tool-result messages for same-dialect `openai-chat` upstreams.
- Requires explicit `tool_support.openai_chat` metadata before an OpenAI Chat tool request can select a target.
- Synthesizes OpenAI Chat SSE chunks with `delta.tool_calls` when the downstream caller requests `stream: true`.
- Added static production/reference smoke group `warp-agent-smoke` routed to validated Baseten `nvidia/Nemotron-120B-A12B`.

Production backups:

```text
/opt/smart-llmrouter.backup.warp-chat-tools-20260618T132828Z
config/config.yaml.bak.20260618T132843Z
```

Validation:

```text
rtk go test ./internal/router: passed, 68 tests
rtk go test ./cmd/... ./internal/...: passed, 72 tests
rtk go test ./...: known generated Harbor/job artifact package failures only
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 2ccc852, build_date 2026-06-18T13:25:47Z
production /version after deploy: 2ccc852, build_date 2026-06-18T13:25:47Z
hosted docs /docs/configuration/router-config: 200 with version headers for 2ccc852
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, af48441185bfc145f999d082a50b045caed923b106421e83a2d9f6e95803a161
production Warp-style /v1/chat/completions smoke with tools, tool_choice, parallel_tool_calls, stream=true: warp-agent-smoke 200, streamed tool_call get_weather
production Warp-style /v1/chat/completions smoke with tools, tool_choice, parallel_tool_calls, stream=true: small 200, streamed tool_call get_weather
production Warp-style /v1/chat/completions smoke with tools, tool_choice, parallel_tool_calls, stream=true: big-coder 200, streamed tool_call get_weather
production /v1/models: small, big-coder, and warp-agent-smoke visible to the operator smoke token
Codex CLI production tool smoke through router Responses API with model agent-tools-smoke: created expected file
Claude Code CLI production tool smoke with model claude-tools-smoke: created expected file and JSON result contained expected text
production logs: router listening on :8080, no errors in recent router logs
production usage DB: recent Warp, Codex, and Claude smokes recorded status 200 with no error
production cleanup: removed uploaded package; dangling Docker image prune reclaimed 0 B; volumes were not pruned
```

### 2026-06-18 Metrics admin authorization rollout

Package `smart-llmrouter:d315be0-linux-amd64` was deployed to production to close cross-tenant exposure from `/metrics`.

Runtime change:

- Added caller config field `metrics_admin`.
- `/metrics` now requires an authenticated caller with `metrics_admin: true`.
- Authenticated non-admin callers receive `403 metrics-forbidden` and no Prometheus metric body.
- Production config marks only `chetan-metrum-insights-prod` as metrics admin.

Production backups:

```text
/opt/smart-llmrouter.backup.metrics-admin-20260618T134841Z
config/config.yaml.bak.20260618T134853Z
```

Validation:

```text
rtk go test ./internal/router: passed, 68 tests
rtk go test ./cmd/... ./internal/...: passed, 72 tests
rtk go test ./...: known generated Harbor/job artifact package failures only
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version d315be0, build_date 2026-06-18T13:46:34Z
production /version after deploy: d315be0, build_date 2026-06-18T13:46:34Z
hosted docs /docs/configuration/router-config: 200 with version headers for d315be0 and metrics_admin content present
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 1fa9980e41700dc00f4ba6b50b6ce387ec2f23a1281951c5bf1763a28f7b5d34
production admin /metrics smoke with operator token: 200, smart_llmrouter_build_info present
production authenticated non-admin /v1/models smoke: 200
production authenticated non-admin /metrics smoke: 403 metrics-forbidden, no Prometheus labels or metric names in body
production usage DB: recent metrics rows show admin 200 and non-admin 403 metrics-forbidden
production logs: router listening on :8080, no errors in recent router logs
production cleanup: removed uploaded package; dangling Docker image prune reclaimed 0 B; volumes were not pruned
```

### 2026-06-18 Max-token cap enforcement rollout

Package `smart-llmrouter:5e8a11f-linux-amd64` was deployed to production to fix capped request handling for the `vision` model group and related translated API surfaces.

Runtime/config changes:

- Anthropic Messages encoding now forwards positive caller `max_tokens` exactly and defaults to 1024 only when omitted.
- OpenAI Responses `max_output_tokens` is decoded into router IR and translated to OpenAI Chat `max_tokens` when needed.
- Added provider/target metadata `honors_max_tokens`; targets marked `false` are skipped whenever the caller supplies a positive max-token field.
- Marked OpenRouter-hosted VLM targets that failed or had not proven cap-safe as `honors_max_tokens: false` across Chat, Responses, and Anthropic provider skins, while keeping them cataloged and available for uncapped requests.

Production backups:

```text
/opt/smart-llmrouter.backup.max-tokens-20260618T140315Z
/opt/smart-llmrouter.backup.max-tokens-filter-20260618T141502Z
/opt/smart-llmrouter.backup.responses-max-output-20260618T142233Z
config/config.yaml.bak.max-tokens-20260618T141030Z
config/config.yaml.bak.max-tokens-openrouter-vlm-20260618T141722Z
```

Validation:

```text
rtk go test ./internal/router: passed, 76 tests
rtk go test ./cmd/... ./internal/...: passed, 80 tests
rtk go test ./...: known generated Harbor/job artifact package failures only
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after final deploy: 200, version 5e8a11f, build_date 2026-06-18T14:20:31Z
production /version after final deploy: 5e8a11f, build_date 2026-06-18T14:20:31Z
hosted docs /docs/configuration/router-config: 200 with honors_max_tokens content present
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, f992d3369b1251b5079433fca58ce310de59b9373e8828133c2fbadc1f44d5e7
initial production /v1/messages max_tokens=1 smoke reproduced cap-unreliable OpenRouter VLM behavior before metadata tightening: qwen/qwen3.7-plus:nitro 1727 output tokens; qwen/qwen3.6-flash:nitro 1072-1241 output tokens; OpenRouter-hosted x-ai/grok-4.3 159 output tokens
production /v1/messages vision max_tokens=1 after final deploy: 6/6 HTTP 200 with output_tokens=1
production /v1/chat/completions vision max_tokens=1 after final deploy: HTTP 200 with output_tokens=1
production /v1/responses vision max_output_tokens=1 after final deploy: HTTP 200 with output_tokens=1
Claude Code CLI production tool smoke using `claude -p` and model claude-tools-smoke: created expected file
Codex CLI production tool smoke through router Responses API with model agent-tools-smoke: created expected file
production logs: router listening on :8080, no errors in recent router logs
production usage DB: recent capped vision smokes recorded status 200 with output_tokens=1; unrelated small-group 429 rows were quota/tpm enforcement
production cleanup: removed uploaded packages for 227d4fd, 0fa3034, and 5e8a11f; dangling Docker image prune reclaimed 0 B; volumes were not pruned
```

### 2026-06-18 Uniform caller limit config update

Production caller key limits were normalized so every configured caller entry uses the same rate, quota, and lifetime token policy.

Uniform policy:

```text
rpm=240
tpm=1200000
concurrent=16
daily_requests=10000
daily_tokens=100000000
monthly_requests=0
monthly_tokens=1200000000
lifetime_tokens=4000000000
```

Production backup:

```text
config/config.yaml.bak.uniform-caller-limits-20260618T153524Z
```

Validation:

```text
production /readyz after restart: 200, version 5e8a11f, build_date 2026-06-18T14:20:31Z
live config caller limit check: 94 caller entries share the same limit tuple
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, ae168b79e0eb51571651931921eac2f65def54eb33e2a1520944ea8f08cf11a1
```

### 2026-06-18 Baseten GLM 5.2 routing rollout

Package `smart-llmrouter:5cf44a4-linux-amd64` was deployed to production with hosted docs and reference config updates for Baseten `zai-org/GLM-5.2`.

Routing/config changes:

- Added Baseten provider catalog entry `glm-5-2` for `zai-org/GLM-5.2` with pricing metadata, text modalities, and OpenAI Chat tool support.
- Added static smoke group `baseten-glm52-smoke`.
- Replaced active OpenRouter GLM 5.2 routing with Baseten GLM 5.2 in the normal text pools for `default`, `fast`, `small`, `medium`, `high`, and `big-coder`.
- Kept Baseten GLM lower in `small` at 2% and at 7% in `big-coder`.

Production backups:

```text
/opt/smart-llmrouter.backup.baseten-glm52-20260618T232903Z
/opt/smart-llmrouter/compose/config/config.yaml.bak.pre-baseten-glm52-20260618T232903Z
```

Validation:

```text
rtk go test ./internal/router: passed, 76 tests
rtk go test ./cmd/... ./internal/...: passed, 80 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after final deploy: 200, version 5cf44a4, build_date 2026-06-18T23:26:40Z
production /version after final deploy: 5cf44a4, build_date 2026-06-18T23:26:40Z
hosted docs /docs/configuration/router-config: 200 with zai-org/GLM-5.2 content present
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 371460627fc9d1ad29a0b012dd5cc501b70a03d4facbd0db4e2196bb25ced487
live config active OpenRouter GLM targets: none
production /v1/models: default, fast, small, medium, high, big-coder, and baseten-glm52-smoke present
production baseten-glm52-smoke realistic text smoke: HTTP 200, model zai-org/GLM-5.2, finish stop, completion_tokens=74
production baseten-glm52-smoke max_tokens=1 cap smoke: HTTP 200, finish length, completion_tokens=1
production baseten-glm52-smoke OpenAI Chat tool smoke: HTTP 200, finish tool_calls, tool_calls present
Claude Code CLI production tool smoke using claude -p and model claude-tools-smoke: created expected file
Codex CLI production tool smoke through router Responses API with model agent-tools-smoke: created expected file
production startup issue: initial copied config/env permissions blocked container reads; fixed ownership for runtime UID/GID 65532:65532 and recreated router container
final smoke rerun after cleanup: /v1/models present check passed; Baseten GLM realistic text, max_tokens=1 cap, and tool-call smokes passed; Claude Code and Codex CLI tool smokes passed with file assertions
production cleanup: removed uploaded package/config/check files, removed redundant /opt/smart-llmrouter.old.20260618T232903Z, removed stale /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f; reclaimed 0 B from Docker; /tmp is 18% used; volumes were not pruned
production recent logs after final smoke: router listening on :8080, no errors in the last 10 minutes
```

### 2026-06-19 Deployment-defined model groups and endpoint-neutral docs

Package `smart-llmrouter:5df71f8-linux-amd64` was deployed to production. This release removes product-level assumptions that model group names such as `default`, `fast`, `small`, `medium`, `high`, `big-coder`, or `vision` are required names, and treats the current production URL as one Metrum-managed deployment rather than the only hosting location. The product docs now describe on-prem, enterprise-cloud, and Metrum-managed deployments.

Runtime/config changes:

- Added `server.default_model_group` as an explicit config fallback for compatible API requests that omit `model`.
- Removed parser-level hardcoded fallback to `default`; omitted-model requests now use `server.default_model_group` or return `400 missing-model` if no fallback is configured.
- Updated `router-token-gen` so `--allow` is required and no model group is granted by default.
- Added `ROUTER_HTTP_REFERER` to env metadata and production env; sample configs use env expansion for OpenRouter referer headers.
- Updated public docs and hosted Docusaurus pages so deployment endpoint and model group names are placeholders or clearly labeled examples.

Production backups:

```text
/opt/smart-llmrouter.backup.model-groups-configurable-20260619T005052Z
/opt/smart-llmrouter/compose/config/config.yaml.bak.pre-model-groups-configurable-20260619T005052Z
```

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 83 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 5df71f8, build_date 2026-06-19T00:48:20Z
production /version after deploy: 5df71f8, build_date 2026-06-19T00:48:20Z
local config.production.yaml SHA-256 matches live runtime config SHA-256: yes, 852c658f30aa0bc890fe67ad289ccf1dc0e052056785cd0f9663bdb508fc49ee
hosted docs /docs/overview: 200, displayed version 5df71f8 and build timestamp, used generic your-router.example.com metadata
production env.json: ROUTER_HTTP_REFERER set
production /v1/models with router token: returned 19 allowed groups
production omitted-model /v1/chat/completions: HTTP 200, routed through configured default_model_group
production explicit high /v1/chat/completions: HTTP 200, returned OK
production router-token-gen without --allow: exited nonzero with "at least one allowed model group is required"
Claude Code CLI production tool smoke using claude -p and model claude-tools-smoke: created expected file
Codex CLI production tool smoke through router Responses API with model agent-tools-smoke: created expected file
production logs after deploy: router listening on :8080, no errors in recent router logs
production cleanup: removed uploaded package/config files, removed replaced deployment tree, removed stale /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f; reclaimed 0 B from Docker; /tmp smart-llmrouter package files remaining: 0
```

### 2026-06-19 Product and operator reference docs deployment

Package `smart-llmrouter:d2aa84e-linux-amd64` was deployed to production to publish new external Docusaurus reference docs and packaged internal operator runbooks.

Documentation changes:

- Added hosted Docusaurus pages for API compatibility, error reference, model metadata, and provider/model onboarding.
- Added internal runbooks for production deployment, troubleshooting, smoke testing, usage reporting, and security review notes.
- Updated packaging so release packages include every `docs/*.md` internal document.
- Updated `AGENTS.md` documentation expectations so future changes maintain both external product docs and internal operator docs.

Production backup:

```text
/opt/smart-llmrouter.backup.docs-reference-20260619T020557Z
```

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 83 tests
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version d2aa84e, build_date 2026-06-19T02:03:41Z
production /version after deploy: d2aa84e, build_date 2026-06-19T02:03:41Z
hosted docs pages: /docs/reference/api-compatibility, /docs/reference/errors, /docs/reference/model-metadata, and /docs/reference/add-provider-model all returned 200 with expected titles
production /v1/models with router token: returned 19 allowed groups
production explicit high /v1/chat/completions: HTTP 200, returned OK
Claude Code CLI production tool smoke using claude -p and model claude-tools-smoke: created expected file
Codex CLI production tool smoke through router Responses API with model agent-tools-smoke: created expected file
production logs after deploy: router listening on :8080, no errors in recent router logs
production cleanup: removed uploaded package, removed replaced deployment tree, removed stale /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f; reclaimed 0 B from Docker; /tmp smart-llmrouter package files remaining: 0
```

### 2026-06-19 Competitive and capability docs deployment

Package `smart-llmrouter:3099886-linux-amd64` was deployed to production to publish truthful, capability-grounded product evaluation and competitive landscape docs.

Documentation changes:

- Added hosted Docusaurus pages for Product Capabilities, Competitive Landscape, Enterprise Evaluation Guide, and Cost Governance.
- Updated Overview, Solution Brief, Usage Reporting, and the Docusaurus sidebar to link the new evaluation docs.
- Added internal `docs/COMPETITIVE_NOTES.md` and `docs/PRODUCT_CAPABILITY_MATRIX.md`.
- Updated `AGENTS.md` so future competitive/public market claims are primary-source-first, source-dated, and explicit about current product boundaries.

Production backup:

```text
/opt/smart-llmrouter.backup.competitive-docs-20260619T022017Z
```

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 83 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 3099886, build_date 2026-06-19T02:18:08Z
production /version after deploy: 3099886, build_date 2026-06-19T02:18:08Z
hosted docs pages: /docs/evaluation/product-capabilities, /docs/evaluation/competitive-landscape, /docs/evaluation/enterprise-evaluation, and /docs/evaluation/cost-governance all returned 200 with expected titles
production /v1/models with router token: returned 19 allowed groups
production explicit high /v1/chat/completions: HTTP 200, returned OK
Claude Code CLI production tool smoke using claude -p and model claude-tools-smoke: created expected file
Codex CLI production tool smoke through router Responses API with model agent-tools-smoke: created expected file
production logs after deploy: router listening on :8080, no errors in recent router logs
production cleanup: removed uploaded package, removed replaced deployment tree, removed stale /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f; reclaimed 0 B from Docker; /tmp smart-llmrouter package files remaining: 0
```

### 2026-06-19 GenAI Smart Router branding and competitive docs deployment

Package `smart-llmrouter:6782a0d-linux-amd64` was deployed to production to publish the GenAI Smart Router product branding and stronger customer-facing competitive landscape docs.

Source commit: `6782a0d` (`Rebrand docs for GenAI Smart Router`)

Documentation changes:

- Rebranded public Docusaurus product copy from Smart LLM Router to GenAI Smart Router.
- Rewrote the competitive landscape page to highlight the product's combined strengths: high-performance gateway path, telemetry, budgets/rate limits, programmable TypeScript policy, private upstreams, VLM/tool-aware eligibility, agent-client compatibility, outcome-oriented Harbor-style evaluation, and request-time accounting.
- Removed public-doc language that read like internal caveat/review wording and replaced it with customer-facing deployment fit and capability language.
- Updated `AGENTS.md` documentation guidance so future public product/competitive docs use GenAI Smart Router positioning and avoid meta/internal review phrases.

Production backup:

```text
/opt/smart-llmrouter.backup.genai-brand-docs-20260619T024212Z
```

Validation:

```text
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 6782a0d, build_date 2026-06-19T02:39:53Z
production /version after deploy: 6782a0d, build_date 2026-06-19T02:39:53Z, go1.25.11 linux/amd64
hosted docs pages: /docs/overview and /docs/evaluation/competitive-landscape returned 200 with GenAI Smart Router branding, "strongest offer" competitive positioning, TypeScript policy, and outcome-oriented evaluation language
production explicit high /v1/chat/completions: HTTP 200, finish_reason=stop, final content OK with realistic token budget
production cleanup: removed uploaded package, confirmed zero /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f; reclaimed 0 B from Docker; /tmp remained 18% used
```

### 2026-06-19 Docusaurus terminology cleanup deployment

Package `smart-llmrouter:5ec8319-linux-amd64` was deployed to production to publish customer-facing Docusaurus terminology cleanup across the hosted docs.

Source commit: `5ec8319` (`Polish Docusaurus product terminology`)

Documentation changes:

- Renamed the competitive page source section from `Further Reading` to `Vendor Reference Links`.
- Changed the source intro to `External product and pricing references checked on June 19, 2026`.
- Replaced remaining meta/internal phrasing such as `customer-facing capabilities`, `embedded hosted Docusaurus docs`, `Proxy users`, `internal key`, and negative browser-feature wording with product/operator language.
- Smoothed directive language in model metadata, provider onboarding, API compatibility, error reference, cost governance, enterprise evaluation, and self-hosted upstream docs.

Production backup:

```text
/opt/smart-llmrouter.backup.docs-terminology-20260619T025149Z
```

Validation:

```text
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 5ec8319, build_date 2026-06-19T02:49:46Z
production /version after deploy: 5ec8319, build_date 2026-06-19T02:49:46Z, go1.25.11 linux/amd64
hosted docs /docs/evaluation/competitive-landscape returned 200 with `Vendor Reference Links` and no `Further Reading`
hosted docs /docs/evaluation/product-capabilities returned 200 with `enterprise deployments` and `embedded product documentation`
production explicit high /v1/chat/completions: HTTP 200, finish_reason=stop, final content OK with realistic token budget
production cleanup: removed uploaded package, confirmed zero /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f; reclaimed 0 B from Docker; /tmp remained 18% used
```
