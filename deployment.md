# Smart LLM Router Production Deployment

Last deployed: 2026-06-17

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

- Router package/image version: `bac7711-linux-amd64`
- Source commit: `bac7711`
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

## 2026-06-15 High Group GPT-5.5 Weight Update

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
small      DeepSeek V4 Flash Nitro 61%, MiniMax-M3 30%, Gemma 4%, Kimi 4%, OpenAI GPT-5.5 1% non-tool.
medium     DeepSeek V4 Flash Nitro 56%, MiniMax-M3 27%, Gemma 8%, Kimi 8%, OpenAI GPT-5.5 1% non-tool.
high       DeepSeek V4 Flash Nitro 51%, MiniMax-M3 28%, Gemma 10%, Kimi 10%, OpenAI GPT-5.5 1% non-tool.
default    DeepSeek V4 Flash Nitro 56%, MiniMax-M3 28%, Gemma 8%, Kimi 7%, OpenAI GPT-5.5 1% non-tool.
fast       DeepSeek V4 Flash Nitro 61%, MiniMax-M3 28%, Gemma 5%, Kimi 5%, OpenAI GPT-5.5 1% non-tool.
big-coder  Code-heavy route: DeepSeek V4 Flash Nitro 11%, MiniMax-M3 27%, Kimi K2.7 Code 17%, OpenAI GPT-5.5 5%, OpenAI GPT-5.4 Nano 30%, Z.AI GLM 5.2 Nitro 10%.
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
big-coder: weighted smoke selected gpt-5.5 and MiniMax-M3 at that time; current big-coder includes weighted OpenAI GPT-5.5/GPT-5.4 Nano alongside MiniMax, Kimi, and OpenRouter routes
Codex: router prod codex ok
```

### 2026-06-16 `kyai-judge` production group

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
