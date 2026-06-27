# Smart LLM Router Production Deployment

Last deployed: 2026-06-27

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

- Router package/image version: `432456b-linux-amd64`
- Source commit: `432456b`
- Deployment root: `/opt/smart-llmrouter`
- Compose directory: `/opt/smart-llmrouter/compose`
- Router config: `/opt/smart-llmrouter/compose/config/config.yaml`
- Provider key file: `/opt/smart-llmrouter/compose/config/env.json`
- Routing script: `/opt/smart-llmrouter/compose/config/scripts/router.ts`
- Request log: `/opt/smart-llmrouter/compose/logs/requests.jsonl`
- Usage DB: Postgres compose service (`postgres:18-bookworm`), configured by `ROUTER_USAGE_DB_DSN` in `/opt/smart-llmrouter/compose/.env`
- State file: `/opt/smart-llmrouter/compose/state/router-state.json`
- Production caller token file: `/opt/smart-llmrouter/compose/ROUTER_TOKEN.txt`
- Reusable Harbor benchmark token file: `/opt/smart-llmrouter/compose/ROUTER_TOKEN_HARBOR.txt`
- Steen production token file: `/opt/smart-llmrouter/compose/ROUTER_TOKEN_STEEN.txt`

Do not copy `env.json`, `ROUTER_TOKEN.txt`, `ROUTER_TOKEN_HARBOR.txt`, or `ROUTER_TOKEN_STEEN.txt` into git, chat, tickets, or logs. Token files are stored on the host as `ubuntu:ubuntu` with mode `0600`.

## 2026-06-27 Reasoning docs and admin chart tooltip refresh

Deployed package/image `smart-llmrouter:432456b-linux-amd64` from source commit `432456b` after PRs #151 and #153 merged.

Included changes:

- Admin chart tooltip lifecycle fix from PR #151.
- Hosted Docusaurus reasoning routing guide and internal/operator reasoning rollout docs from PR #153.

Production backup:

```text
/opt/smart-llmrouter.backup.refresh-432456b-20260627T022553Z
```

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 340 tests across 7 packages
rtk make docs-build: passed; npm audit still reports existing docs-site moderate dependency advisories
rtk make secret-check: passed
rtk make package-docker: passed for linux/amd64 and linux/arm64 package artifacts
production docker compose config: passed during deployment
production /readyz after deploy: 200, version 432456b, build_date 2026-06-27T02:21:37Z
production /version after deploy: 432456b, build_date 2026-06-27T02:21:37Z, go1.26.4 linux/amd64
hosted docs /docs/configuration/reasoning-routing returned 200 with x-smart-llmrouter-version 432456b
authenticated /v1/models returned 20 allowed groups for the production admin token
authenticated /v1/chat/completions high max_tokens 128 returned OK from openai/gpt-oss-120b
router logs after deploy: no immediate panic/fatal/error lines in the checked tail
production cleanup: removed uploaded package and ran sudo docker system prune -f
```

## 2026-06-27 Big-coder reasoning routing config update

Enabled explicit reasoning routing eligibility for the production `big-coder` group without changing the running package image. The group remains `weighted` for ordinary traffic. Requests that include OpenAI Chat `reasoning_effort` are now eligible only for the validated `big-coder` ordinary OpenAI Chat targets with reasoning metadata:

- Baseten `openai/gpt-oss-120b`
- Baseten `zai-org/GLM-5.2`
- Crusoe `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B`

Production config backup:

```text
/opt/smart-llmrouter/compose/config/config.yaml.bak.enable-big-coder-reasoning-20260627T012549Z
```

Validation:

```text
direct Baseten zai-org/GLM-5.2 OpenAI Chat reasoning_effort smoke: HTTP 200, finish stop, content OK
direct Baseten openai/gpt-oss-120b OpenAI Chat reasoning_effort smoke: HTTP 200, finish stop, returned content
direct Crusoe Nemotron 3 Nano Omni Reasoning OpenAI Chat reasoning_effort smoke: HTTP 200; max_tokens 64 returned empty final content with finish length, max_tokens 512 returned content and finish stop
production docker compose config: passed
production router restart: passed
production /readyz: 200, version 289ea71
production /v1/models for big-coder: supported_reasoning_levels low, medium, high; supports_reasoning_summaries false
production /v1/chat/completions big-coder with reasoning_effort low and max_tokens 256: HTTP 200, selected Crusoe Nemotron 3 Nano Omni Reasoning, finish stop, content OK
production /v1/chat/completions big-coder without reasoning_effort: HTTP 200, selected MiniMax-M3
local ignored config.production.yaml synced from live production config after validation
```

## 2026-06-26 Signed License Enforcement Refresh

- Deployed package/image `smart-llmrouter:98d4d9a-linux-amd64` from source commit `98d4d9a` after PR #136 merged.
- Production package backup: `/opt/smart-llmrouter.backup.license-98d4d9a-20260626T162406Z`.
- Production config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were carried forward unchanged.
- Deployment cleanup removed the uploaded package and superseded temporary deployment directory, kept the timestamped backup, and ran `sudo docker system prune -f`.
- Posted a terse Google Chat workspace announcement summarizing feature progress since June 23.

Validation:

```text
rtk go test ./cmd/... ./internal/... -count=1: passed, 330 tests across 7 packages
rtk timeout 800s go test ./... -count=1: passed
rtk make docs-build VERSION=98d4d9a COMMIT=98d4d9a: passed; npm audit still reports existing docs-site moderate dependency advisories
rtk make package-docker VERSION=98d4d9a COMMIT=98d4d9a: passed for linux/amd64 and linux/arm64 package artifacts
local scripts/live_full_e2e.sh: failed in the legacy OpenRouter CLI C harness because its generated temporary targets omit tool_support metadata and current routing correctly rejects Claude Code Anthropic/tool passthrough as 502 no-eligible-target
production /readyz after deploy: 200, version 98d4d9a, build_date 2026-06-26T16:06:41Z
production /version: 98d4d9a, commit 98d4d9a, go1.26.4 linux/amd64
hosted /docs/ returned 200 with x-smart-llmrouter-version: 98d4d9a
authenticated /v1/models returned 20 visible model groups
authenticated /v1/chat/completions smoke against default returned OK through openai/gpt-oss-120b
admin /admin/reports/ returned 200 and the Metrum-branded shell
admin /admin/reports/api/savings-by-user?since=2h returned report savings-by-user with 7 rows
admin /admin/reports/api/dynamic-score-buckets?since=2h returned report dynamic-score-buckets with 8 rows
admin /admin/reports/api/anomalies?since=2h returned report anomalies with 27 rows
admin /admin/reports/api/summary?since=2h returned 256 requests and 7 errors for the mixed validation window
admin /admin/reports/api/latency-throughput?since=2h returned report latency-throughput with 34 rows
admin /admin/reports/api/provider-model-mix?since=2h returned report provider-model-mix with 18 rows
```

Production Harbor validation:

```text
Full Harbor matrix `aider/polyglot_python_two-bucket` ran through production using the reusable Harbor caller, agents `codex` and `claude-code`, and groups `default`, `fast`, `small`, `medium`, `high`, and `big-coder`.
Case ID: harbor-prod-98d4d9a-20260626T162525Z
All 12 cells passed with reward 1 and zero verifier errors.
Slowest cells: claude-code/medium 439s, claude-code/fast 360s, codex/big-coder 247s, claude-code/big-coder 166s.
Production Harbor usage report generated: /opt/smart-llmrouter/compose/logs/harbor-prod-98d4d9a-20260626T162525Z.md
```

## 2026-06-26 Admin Reports And Decision Telemetry Refresh

- Deployed package/image `smart-llmrouter:c623323-linux-amd64` from source commit `c623323` after PRs #128 through #132 merged.
- Production package backup: `/opt/smart-llmrouter.backup.refresh-c623323-20260626T143712Z`.
- Production config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were carried forward unchanged.
- Deployment cleanup removed the uploaded package from `/tmp` and ran `sudo docker system prune -f`.

Validation:

```text
rtk go test ./cmd/... ./internal/...: passed, 321 tests across 6 packages
rtk timeout 800s go test ./...: passed
rtk make docs-build: passed; npm audit still reports existing docs-site moderate dependency advisories
rtk make package-docker: passed for linux/amd64 and linux/arm64
production /readyz after deploy: 200, version c623323, build_date 2026-06-26T14:33:00Z
hosted docs /docs/ returned 200 with Docusaurus page title
authenticated /v1/models returned 20 visible model groups
authenticated /v1/chat/completions smoke passed for default and fast groups; MiniMax-M3 returned OK
admin /admin/reports/ returned 200 and includes dynamic report tabs plus provider catalog status
admin /admin/reports/api/dynamic-score-buckets?since=24h returned report dynamic-score-buckets with rows
admin /admin/reports/api/provider-catalog-status returned active target and catalog rows
```

## 2026-06-25 Retention Dry-Run Package Refresh

- Deployed package/image `smart-llmrouter:7698fd1-linux-amd64` from source commit `7698fd1` after PR #126 merged.
- Production package backup: `/opt/smart-llmrouter.backup.harbor-local-validated-20260625T220015Z`.
- Production config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were carried forward unchanged.
- Local Harbor validation before deployment:
  - Full Harbor matrix `aider/polyglot_python_two-bucket` ran against a local router on commit `7698fd1` using the reusable Harbor caller, agents `codex` and `claude-code`, and groups `default`, `fast`, `small`, `medium`, `high`, and `big-coder`.
  - Corrected Docker-bridge case `harbor-local-7698fd1-dockerbridge-20260625T212124Z`: 11/12 cells passed with reward `1` and zero errors; `claude-code/high` completed with reward `0`.
  - Clean rerun case `harbor-local-7698fd1-rerun-claude-high-20260625T215319Z`: `claude-code/high` passed with reward `1` and zero errors.
- `go test ./cmd/... ./internal/...` passed with 315 tests across 6 packages.
- `make package-docker` passed for linux/amd64 and linux/arm64 package artifacts; package content validation passed.
- Verified production `/readyz` reports version `7698fd1`, commit `7698fd1`, and build date `2026-06-25T21:56:22Z`.
- Verified hosted `/docs/evaluation/harbor-case-study` returns 200 with `x-smart-llmrouter-version: 7698fd1`.
- Verified authenticated `/v1/models` for the reusable Harbor caller includes the six Harbor validation groups.
- Verified authenticated production `/v1/chat/completions` against `high` returned HTTP 200 with a concrete upstream model and `finish_reason: stop`.
- Production cleanup: removed the uploaded package and superseded temporary deployment directory, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-25 Aditya TPM Limit Increase

- Applied a config-only production update for caller `aditya-metrum-insights-prod`, increasing `rate.tpm` from `1200000` to `5000000`.
- Reason: Aditya's Cursor `big-coder` traffic hit `429 tpm-exceeded` during a burst of large-context requests around 150K-161K input tokens each.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.aditya-tpm-5m-20260625T203717Z`.
- Local `config.production.yaml` SHA-256 matched the remote deployed config SHA-256: `34a6218e35e3cadf3cac84a7ee09b17e32f03d7979fc0ebd419d6c649f11b187`.
- Verified `/readyz` returned healthy on package/image `smart-llmrouter:87bbf64-linux-amd64`.
- Verified local production snapshot shows `rate: { rpm: 240, tpm: 5000000, concurrent: 16 }` for the caller.

## 2026-06-25 Crusoe Big-Coder Nemotron Replacement

- Applied a config-only production update to replace active `big-coder` Crusoe `google/gemma-4-31b-it` targets with Crusoe `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B`.
- The replacement follows repeated production upstream 400s from Crusoe Gemma under Cursor/opencode `big-coder` traffic. Crusoe's 2026-04-28 Nemotron 3 Nano Omni announcement positions Nemotron 3 Nano Omni 30B A3B Reasoning for multimodal document, GUI-agent, video/audio, and text reasoning workloads with a 256K-token context: `https://www.crusoe.ai/resources/blog/nvidia-nemotron-3-nano-omni-now-available`.
- Production `big-coder` initially replaced both Crusoe Gemma entries with Crusoe Nemotron 3 Nano Omni 30B A3B Reasoning.
- Follow-up PR review fix made the active Crusoe Nemotron `big-coder` target text-only with `input_modalities: [text]` and removed the ineligible Crusoe Nemotron `tool_only` target. Nemotron 3 Nano Omni 30B A3B Reasoning is advertised as multimodal, but the current production OCR smoke did not pass, and the catalog does not claim Crusoe OpenAI Chat tool support for this model until a dedicated tool smoke passes.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.crusoe-gemma-to-nemotron-20260625T200255Z`.
- Production follow-up backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.crusoe-nemotron-text-only-20260625T201714Z`.
- Local `config.production.yaml` SHA-256 matched the remote deployed config SHA-256: `e672dae86045158e560191f4990dbb04a10a98d4879e1cc16cd8ffbc37b39a8d`.
- Verified `/readyz` returned healthy on package/image `smart-llmrouter:87bbf64-linux-amd64`.
- Verified live `big-coder` config contains one active Crusoe `nemotron-3-nano-omni-reasoning-30b-a3b` target with `input_modalities: [text]` and no active Crusoe `gemma-4-31b-it` or Crusoe Nemotron `tool_only` target.
- Verified authenticated production text smoke against `crusoe-nemotron-omni-smoke` selected `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B` and returned `OK`.
- Latest smoke usage row recorded HTTP 200, 348 ms total latency, 346 ms upstream duration, 112.72 upstream output tokens/sec, 170.52 upstream total tokens/sec, 20 input tokens, 39 output tokens, 59 total tokens, and no fallback.

## 2026-06-25 Direct OpenAI Vision Rebalance

- Applied a config-only production update to catalog direct OpenAI `gpt-5.4`, add the exact-route `openai-gpt54-vision-smoke` group, and rebalance the hosted `vision` group away from OpenRouter-hosted Claude Sonnet 4.6.
- Production `vision` changed OpenRouter Chat Claude Sonnet 4.6 from weight 15 to 5, added direct OpenAI `gpt-5.4` at weight 30, and changed OpenRouter Anthropic Claude Sonnet 4.6 from weight 30 to 10.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.direct-openai-vision-20260625T152956Z`.
- Local `config.production.yaml` SHA-256 matched the remote deployed config SHA-256: `4683706d2e184551195b79975be25fda0cfe24943bc384c2da0853f7d9fab8e1`.
- Verified `/readyz` returned healthy on package/image `smart-llmrouter:3e9aa55-linux-amd64`.
- Verified authenticated `/v1/models` for the reusable Harbor caller includes `openai-gpt54-vision-smoke` and `vision`.
- Verified authenticated production `/v1/responses` text smoke against `openai-gpt54-vision-smoke` selected `gpt-5.4` and returned `OK`.
- Verified authenticated production `/v1/responses` receipt-image smoke against `openai-gpt54-vision-smoke` selected `gpt-5.4` and returned `Rite Aid`.
- Verified authenticated production `/v1/chat/completions` receipt-image smoke against `openai-gpt54-vision-smoke` selected `gpt-5.4`; one uncapped/non-temperature-pinned run returned `CVS/pharmacy`, then three `temperature: 0` repeats returned `Rite Aid`. Keep the rollout moderate and monitor image OCR quality before increasing further.
- Follow-up review safety patch removed unvalidated `openai_responses: [function]` metadata from `gpt-5.4` and deployed a production config sentinel `tool_support.provider_hosted: [vision_validated_only]` so the current production binary also treats `gpt-5.4` as tool-ineligible until the stricter code fix ships. Backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.direct-openai-vision-tool-sentinel-20260625T155900Z`. Verified tool-bearing `/v1/responses` requests to `openai-gpt54-vision-smoke` return `502 no-eligible-target`; verified image OCR still returns `Rite Aid`. Local and remote config SHA-256 matched: `8c98adfef4bb1177de258093c6aabd5b38e013dab016de89004d8e2271e6b29b`.
- Deployed package/image `smart-llmrouter:87bbf64-linux-amd64` after PR #119 merged so Responses tool passthrough now requires explicit `tool_support.openai_responses` in code, not only the production config sentinel. Production package backup: `/opt/smart-llmrouter.backup.direct-openai-vision-87bbf64-20260625T161818Z`. Verified `/version` reports `87bbf64`, hosted image-analysis docs return 200, `/v1/models` exposes `openai-gpt54-vision-smoke`, image OCR returns `Rite Aid`, and tool-bearing Responses requests to the smoke group return `502 no-eligible-target`. Removed uploaded package/temp files and ran Docker prune with deployment healthy.

## 2026-06-25 Crusoe Nemotron Omni Smoke Group Config

- Applied a config-only production update to catalog Crusoe `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B` and expose the dedicated `crusoe-nemotron-omni-smoke` group to smoke-test callers.
- The broad `vision` group was not changed because direct and router-level receipt-image smokes accepted the image but did not pass OCR quality validation.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.crusoe-nemotron-omni-20260625T151243Z`.
- Local `config.production.yaml` SHA-256 matched the remote deployed config SHA-256: `3c940868945617726ffd2ba9efc88aee5792c6be38724bb51bc420d98f283810`.
- Verified `/readyz` returned healthy on package/image `smart-llmrouter:3e9aa55-linux-amd64`.
- Verified authenticated `/v1/models` for the reusable Harbor caller includes `crusoe-nemotron-omni-smoke`.
- Verified authenticated production text smoke against `crusoe-nemotron-omni-smoke` selected `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B` and returned `OK`.
- Verified authenticated production receipt-image smoke against `crusoe-nemotron-omni-smoke` selected the same model and returned HTTP 200 with `finish_reason=length` and empty content, matching local validation and confirming it should remain smoke-only.
- Operational note: after replacing `config/config.yaml`, keep it readable by the router container UID/GID `65532`; a root-owned `0640` file caused a temporary startup loop until ownership was corrected.

## 2026-06-25 Model-Group Contracts Deployment

- Deployed package/image `smart-llmrouter:3e9aa55-linux-amd64` from source commit `3e9aa55` after PR #118 merged.
- Added optional model-group contract configuration, target validation metadata, contract-aware eligibility, safe usage/report fields, and internal plus Docusaurus documentation.
- Production package backup: `/opt/smart-llmrouter.backup.model-group-contracts-20260625T143222Z`.
- Production config was carried forward unchanged; no production groups were configured with new contract fields during this refresh.
- Verified public `/readyz` reports version `3e9aa55`, commit `3e9aa55`, and build date `2026-06-25T14:28:37Z`.
- Verified public `/version` reports version `3e9aa55`, commit `3e9aa55`, build date `2026-06-25T14:28:37Z`, and Go `1.26.4` on `linux/amd64`.
- Verified hosted `/docs/configuration/model-group-contracts` returns 200.
- Verified authenticated `/v1/models` returns the caller's allowed model groups.
- Verified authenticated `/admin/reports/api/contract-buckets?since=24h&limit=10` returns the contract-buckets report shape.
- Verified authenticated `/v1/chat/completions` against the `high` group returned HTTP 200 with content `OK` after a realistic output cap.
- Production cleanup: removed the uploaded package, removed temporary unpack files, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

Validation before deploy:

```text
go test ./cmd/... ./internal/...: passed, 297 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker: passed for linux/amd64 and linux/arm64 package artifacts
package content validation: passed
```

## 2026-06-25 Admin Anomaly Reports Refresh

- Deployed package/image `smart-llmrouter:e087c8c-linux-amd64` from source commit `e087c8c` after PR #116 merged.
- Fixed admin anomaly reports so normal `active` key state is not classified as `key-active`.
- Fixed non-savings scalar report tabs so `Baseline`, `Savings`, and `Savings %` columns render only for savings-backed reports.
- Production package backup: `/opt/smart-llmrouter.backup.anomaly-reports-20260625T064739Z`.
- Production config was carried forward unchanged from the previous admin-report deployment.
- Verified public `/readyz` reports version `e087c8c`, commit `e087c8c`, and build date `2026-06-25T06:43:56Z`.
- Verified public `/version` reports version `e087c8c`, commit `e087c8c`, build date `2026-06-25T06:43:56Z`, and Go `1.26.4` on `linux/amd64`.
- Verified hosted `/docs/` returns 200 after the embedded Docusaurus rebuild.
- Verified authenticated `/admin/reports/api/anomalies?since=24h&limit=50` returns anomaly rows with no `key-active` rows and no baseline/savings fields.
- Verified authenticated `/admin/reports/api/savings-by-user?since=24h&limit=5` still returns a selected baseline and savings fields.
- Production cleanup: removed the uploaded package, removed temporary unpack files, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

Validation before deploy:

```text
go test ./cmd/... ./internal/...: passed, 280 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker: passed for linux/amd64 and linux/arm64 package artifacts
package content validation: passed
```

## 2026-06-25 Admin Browser Reports Package Deployment

- Deployed package/image `smart-llmrouter:0af2002-linux-amd64` from source commit `0af2002` after PR #79 merged.
- Production package backup: `/opt/smart-llmrouter.backup-refresh-0af2002-20260625T020054Z`.
- Production config was carried forward unchanged; `server.admin_reports.enabled` remains disabled in the live deployment.
- Verified public `/readyz` reports version `0af2002`, commit `0af2002`, and build date `2026-06-25T01:57:00Z`.
- Verified public `/version` reports version `0af2002`, commit `0af2002`, build date `2026-06-25T01:57:00Z`, and Go `1.26.4` on `linux/amd64`.
- Verified hosted docs `/docs/operations/admin-browser-reports` returns 200 with `x-smart-llmrouter-version: 0af2002`.
- Verified authenticated `/v1/models` returns an OpenAI-compatible model list.
- Verified authenticated `/v1/chat/completions` against the `high` group returned HTTP 200.
- Verified unauthenticated `/admin/reports/` returns 404 because the production admin report surface is still disabled.
- Production cleanup: removed the uploaded package, removed the duplicate previous-active switch directory, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

Validation before deploy:

```text
go test ./cmd/... ./internal/...: passed, 223 tests
make package-docker: passed for linux/amd64 and linux/arm64 package artifacts
package content validation: passed
```

## 2026-06-24 HTTP Basic Admin Authentication Deployment

- Deployed package/image `smart-llmrouter:1e26c71-linux-amd64` from source commit `1e26c71` for issue #72.
- Added disabled-by-default `server.admin_auth.basic` support to the product, plus `GET /admin/auth/check` as the first protected browser-admin identity validation endpoint.
- Added internal and Docusaurus documentation for Basic Auth setup, bcrypt hash generation, TLS/reverse-proxy behavior, smoke tests, rollback, and the AuthN/AuthZ boundary.
- Production package backup: `/opt/smart-llmrouter.backup.basic-auth-1e26c71-20260624T195321Z`.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.basic-auth-20260624T195430Z`.
- Production env backup: `/opt/smart-llmrouter/compose/config/env.json.bak.basic-auth-20260624T195430Z`.
- Enabled production Basic Auth with username `admin`, an env-backed bcrypt hash in `SMART_ROUTER_ADMIN_PASSWORD_HASH`, subject `basic:admin`, domain `metrum/prod`, and the current stub permission `admin:auth:read`.
- Verified public `/readyz` reports version `1e26c71`, commit `1e26c71`, and build date `2026-06-24T19:49:20Z`.
- Verified hosted docs `/docs/configuration/admin-authentication` returns HTTP 200 with `x-smart-llmrouter-version: 1e26c71`.
- Verified public `/admin/auth/check` returns `401` with `WWW-Authenticate` and `Cache-Control: no-store` for missing credentials.
- Verified public `/admin/auth/check` returns `401` with no sensitive detail for invalid credentials.
- Verified public `/admin/auth/check` returns `200` with safe subject metadata for the configured admin credentials.
- Verified existing bearer-token `/v1/models` behavior still works after enabling Basic Auth.
- Local validation: `go test ./...` passed with 206 tests in 6 packages; `make docs-qa` passed; `make secret-check` passed; local e2e covered enabled Basic Auth missing/bad/valid credential paths.
- Production cleanup: removed uploaded package, removed the duplicate previous-active switch directory, kept timestamped backups, and ran `sudo docker system prune -f` with the deployment healthy.
- Review follow-up deployment: deployed package/image `smart-llmrouter:d7ec9a2-linux-amd64` from source commit `d7ec9a2` to require env-backed password hashes only and trust `X-Forwarded-Proto: https` only from configured proxy CIDRs.
- Review follow-up package backup: `/opt/smart-llmrouter.backup.basic-auth-reviewfix-d7ec9a2-20260624T201140Z`.
- Review follow-up config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.basic-auth-trusted-proxy-20260624T201207Z`.
- Production Basic Auth now sets `trusted_proxy_cidrs` to the compose proxy subnet `172.18.0.0/16`.
- Verified public `/readyz` reports version `d7ec9a2`, commit `d7ec9a2`, and build date `2026-06-24T20:07:59Z`.
- Re-verified public `/admin/auth/check` missing credentials -> `401`, invalid credentials -> `401`, valid admin credentials -> `200`, with no-store headers.
- Re-verified existing bearer-token `/v1/models` behavior still works and returns 18 models.

## 2026-06-24 OpenRouter Gemma Cap And Crusoe GLM Production Config Update

- Reduced normal OpenRouter Gemma 4 26B Nitro weight to 2% in `default`, `fast`, `small`, `medium`, and `high`.
- Added Crusoe Managed Inference `zai/GLM-5.2` as ordinary OpenAI Chat text routing to replace the reduced OpenRouter Gemma weight: `default` 5%, `fast` 2%, `small` 2%, `medium` 5%, and `high` 7%.
- Kept existing OpenRouter Anthropic Gemma tool-only targets at 2%; Crusoe GLM is not enabled for tool, structured-output, Responses, or Anthropic skins.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.gemma2-crusoe-glm-20260624T185540Z`.
- Direct Crusoe `zai/GLM-5.2` text smoke from the production host returned HTTP 200 with `OK`; a `max_tokens: 1` cap probe returned `finish=length` with empty content, so the route is ordinary-text only.
- Verified `sudo docker compose config >/dev/null`, restarted the router, and confirmed `/readyz` returned 200 for version `4459ba1`.
- Verified live normal-route weights sum to 100 in `default`, `fast`, `small`, `medium`, `high`, and `big-coder`; OpenRouter Gemma normal weight is at most 2%.
- Public endpoint weighted `high` smoke selected Crusoe `zai/GLM-5.2` on attempt 4 and returned `OK`.
- Production usage DB recorded the Crusoe GLM smoke with status 200, attempts 1, `fallback_used=false`, input/output token counts, and stored cost.
- Local validation: focused config tests passed; `go test ./...` passed with 188 tests in 6 packages; `make docs-qa` passed.
- Deployed package/image `smart-llmrouter:0d5003e-linux-amd64` from source commit `0d5003e` so hosted docs and packaged sample config include the new route policy.
- Production package backup: `/opt/smart-llmrouter.backup.crusoe-glm-0d5003e-20260624T190432Z`.
- Verified public `/readyz` reports version `0d5003e`, commit `0d5003e`, and build date `2026-06-24T19:00:37Z`.
- Verified hosted docs `/docs/configuration/router-config` returns HTTP 200 with `x-smart-llmrouter-version: 0d5003e`.
- Post-package weighted `high` smoke selected Crusoe `zai/GLM-5.2`; `max_tokens: 128` returned an empty content body, while `max_tokens: 1024` returned `OK`. Keep the model on ordinary text routing and use realistic budgets for validation.
- Production cleanup: removed uploaded package, removed the duplicate previous-active switch directory, kept timestamped backups, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-24 Public Docs Refresh Package Deployment

- Deployed package/image `smart-llmrouter:4459ba1-linux-amd64` from source commit `4459ba1` after PR #70 merged.
- Production package backup: `/opt/smart-llmrouter.backup.refresh-4459ba1-20260624T182001Z`.
- Runtime config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were copied forward from the backup.
- Verified `sudo docker compose config >/dev/null` and restarted the router with Docker Compose.
- Verified public `/readyz` reports version `4459ba1`, commit `4459ba1`, and build date `2026-06-24T18:16:11Z`.
- Verified hosted docs `/docs/` returns HTTP 200 with `x-smart-llmrouter-version: 4459ba1`.
- Verified authenticated public `high` completion returned `OK` with a realistic `max_tokens: 128` budget. A tiny `max_tokens: 16` probe returned a valid response envelope and usage but empty content from `openai/gpt-oss-120b`.
- Local pre-deploy validation passed:
  - `go test ./cmd/... ./internal/...`: 188 tests passed in 6 packages.
  - `make docs-build`: passed; npm audit still reports the existing 23 moderate docs-site dependency advisories.
  - `make package-docker`: built and validated linux/amd64 and linux/arm64 Docker packages.
- Production cleanup: removed uploaded package, removed the duplicate previous-active switch directory, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-24 Crusoe Gemma `big-coder` Production Config Update

- Updated production config to add Crusoe Managed Inference `google/gemma-4-31b-it` to the ordinary-text `big-coder` weighted route at 20%.
- Rebalanced existing normal targets so the non-tool total remains 100: Baseten GPT OSS 120B 18%, MiniMax-M3 30%, Baseten Nemotron 120B 2%, Baseten GLM 5.2 6%, Kimi K2.7 Code 23%, OpenAI GPT-5.4 Nano 1%, Crusoe Gemma 4 31B-it 20%.
- Kept the existing Crusoe Gemma OpenAI Chat `tool_only` target separate for tool-bearing OpenAI Chat requests; Anthropic-compatible Gemma fallbacks were not replaced.
- Production config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.crusoe-gemma-20260624T173317Z`.
- Production env backup: `/opt/smart-llmrouter/compose/config/env.json.bak.20260624T173235Z`.
- Added `CRUSOE_API_KEY` to production `env.json` without printing the key.
- Direct Crusoe Gemma smoke from the production host returned HTTP 200 from `google/gemma-4-31b-it` with `OK`.
- Verified `sudo docker compose config >/dev/null`, restarted the router, and confirmed `/readyz` returned 200 for version `d43ef7d`.
- Verified local `config.production.yaml` SHA-256 matched remote `/opt/smart-llmrouter/compose/config/config.yaml`.
- Public endpoint structured-output `big-coder` smoke selected `google/gemma-4-31b-it` on attempt 3 and returned `{"status":"ok"}`.
- Production request log for the Crusoe `big-coder` smoke showed status 200, attempts 1, `fallback_used=false`, and Crusoe pricing/cost fields.
- Removed temporary uploaded config from `/tmp`.
- Local validation: focused config tests passed; `go test ./...` passed with 188 tests in 6 packages; `make docs-build` passed, with npm still reporting existing docs-site dependency advisories.

## 2026-06-24 Max Tokens, Quota Reservations, And Package Docs Deployment

- Deployed package/image `smart-llmrouter:704148a-linux-amd64` from source commit `704148a` after PRs #55, #56, and #57 merged.
- Production package backup: `/opt/smart-llmrouter.backup-issues-15-17-704148a-20260624T135313Z`.
- Runtime config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were copied forward from the backup.
- Verified `sudo docker compose config >/dev/null` and restarted the router with Docker Compose.
- Verified public `/readyz` and `/version` report version `704148a`, commit `704148a`, build date `2026-06-24T13:49:09Z`, and Go `1.26.4`.
- Verified hosted docs page `/docs/reference/api-compatibility` returns HTTP 200 with `x-smart-llmrouter-version: 704148a`.
- Verified authenticated public `/v1/models` returned 18 model groups.
- Verified authenticated public `high` completion returned exactly `OK` through `openai/gpt-oss-120b`.
- Local pre-deploy validation passed:
  - `make secret-check`: passed.
  - `make docs-build VERSION=704148a COMMIT=704148a`: passed; npm audit still reports the existing 23 moderate docs-site dependency advisories.
  - `go test ./...`: 170 tests passed in 6 packages after docs build refreshed embedded docs.
  - `make package-docker VERSION=704148a COMMIT=704148a`: built and validated linux/amd64 and linux/arm64 Docker packages.
- Production cleanup: removed uploaded package, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-24 Security, PII, And Content Capture Package Deployment

- Deployed package/image `smart-llmrouter:b34aaf6-linux-amd64` from source commit `b34aaf6` after PRs #49, #50, #51, #52, and #53 merged.
- Production package backup: `/opt/smart-llmrouter.backup-security-b34aaf6-20260624T065431Z`.
- Runtime config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were copied forward from the backup.
- Verified `sudo docker compose config >/dev/null` and restarted the router with Docker Compose.
- Verified public `/readyz` and `/version` report version `b34aaf6`, commit `b34aaf6`, build date `2026-06-24T06:50:29Z`, and Go `1.26.4`.
- Verified hosted docs pages return HTTP 200 with `x-smart-llmrouter-version: b34aaf6`:
  - `/docs/configuration/router-config`
  - `/docs/configuration/pii-filtering`
  - `/docs/configuration/routing-typescript`
  - `/docs/configuration/external-routing-policy`
  - `/docs/evaluation/deployment-security-assessment`
- Verified authenticated public `/v1/models` returned 18 model groups.
- Verified authenticated public static completion smoke against `baseten-gpt-oss-120b-smoke` returned exactly `OK` through `openai/gpt-oss-120b`.
- Verified authenticated public weighted `high` completion returned HTTP 200 through `MiniMax-M3`; the model included reasoning text instead of the requested exact `OK`, so the deterministic static smoke above is the acceptance signal for basic completion health.
- Checked router logs after deployment; only the normal startup line was present.
- Local pre-deploy validation passed:
  - `make secret-check`: passed.
  - `make docs-build`: passed; npm audit still reports the existing 23 moderate docs-site dependency advisories.
  - `go test ./cmd/... ./internal/...`: 158 tests passed in 6 packages after docs build refreshed embedded docs.
  - `go test ./...`: 158 tests passed in 6 packages.
  - `make package-docker VERSION=b34aaf6 COMMIT=b34aaf6`: built linux/amd64 and linux/arm64 Docker packages.
- Production cleanup: removed uploaded package and temporary Docker load log, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-24 Public Docs Refresh Package Deployment

- Deployed package/image `smart-llmrouter:b938bbc-linux-amd64` from source commit `b938bbc` after PR #34 merged.
- Production package backup: `/opt/smart-llmrouter.backup-docs-b938bbc-20260624T033455Z`.
- Runtime config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were copied forward from the backup.
- Verified `sudo docker compose config >/dev/null` and restarted the router with Docker Compose.
- Verified `/readyz` and `/version` report version `b938bbc`, commit `b938bbc`, build date `2026-06-24T03:31:04Z`.
- Verified hosted docs pages `/docs/evaluation/evaluate-smart-router` and `/docs/evaluation/security-and-trust` return HTTP 200 with `x-smart-llmrouter-version: b938bbc`.
- Verified authenticated `/v1/models` returned 18 model groups.
- Verified authenticated production chat smoke against `high` returned `OK` through `openai/gpt-oss-120b`.
- Local pre-deploy validation passed:
  - `go test ./...`: 113 tests passed in 6 packages.
  - `make docs-build`: passed; npm audit still reports existing docs-site dependency advisories.
  - `make package-docker VERSION=b938bbc COMMIT=b938bbc`: built linux/amd64 and linux/arm64 Docker packages.
- Production cleanup: removed uploaded package and temporary unpack directory, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-24 Dynamic Score Routing Package Deployment

- Deployed package/image `smart-llmrouter:566956f-linux-amd64` from source commit `566956f` after PR #31 merged.
- Production package backup: `/opt/smart-llmrouter.backup.dynamic-score-566956f-20260624T022305Z`.
- Runtime config, state, logs, `.env`, and `ROUTER_TOKEN*.txt` files were copied forward from the backup.
- Verified `sudo docker compose config >/dev/null` and restarted the router with Docker Compose.
- Verified `/readyz` and `/version` report version `566956f`, commit `566956f`, build date `2026-06-24T02:19:02Z`.
- Verified hosted docs page `/docs/configuration/dynamic-score-routing` returns HTTP 200 with `x-smart-llmrouter-version: 566956f`.
- Verified authenticated `/v1/models` returned 18 model groups.
- Verified authenticated production chat smoke against `high` returned `OK` through `openai/gpt-oss-120b`.
- Local pre-deploy validation passed:
  - `go test ./...`: 113 tests passed in 6 packages.
  - `make docs-build`: passed; npm audit still reports existing docs-site dependency advisories.
  - `make package-docker VERSION=566956f COMMIT=566956f`: built linux/amd64 and linux/arm64 Docker packages.
- Harbor production validation:
  - Full default Harbor matrix `aider/polyglot_python_two-bucket` ran through production using the reusable Harbor caller, agents `codex` and `claude-code`, groups `default`, `fast`, `small`, `medium`, `high`, and `big-coder`.
  - Initial full matrix case `harbor-prod-566956f-20260624T022347Z`: Codex passed 6/6; Claude Code passed `default`, `medium`, `high`, and `big-coder`, while `fast` and `small` completed without Harbor exceptions but scored reward `0`.
  - Clean rerun case `harbor-prod-566956f-rerun-failed-20260624T025300Z`: Claude Code `fast` and `small` both passed with reward `1` and zero errors.
  - Production usage report generated on the host at `logs/harbor-dynamic-score-566956f-20260624T022347Z.md`.
- Production cleanup: removed uploaded package and temporary unpack directory, kept the timestamped backup, and ran `sudo docker system prune -f` with the deployment healthy.

## 2026-06-22 Baseten GPT OSS 120B Replacement

- Replaced active OpenRouter Qwen and OpenRouter DeepSeek targets in production model groups with Baseten `openai/gpt-oss-120b` for OpenAI Chat text traffic.
- Added `baseten_anthropic` provider skin for Baseten Anthropic Messages beta at `https://inference.baseten.co`, validated with `openai/gpt-oss-120b` text and client-tool smokes.
- Kept Codex/OpenAI Responses tool traffic on validated Responses-compatible targets because Baseten has not been validated as a Responses API provider.
- Rebalanced VLM/image routes away from OpenRouter Qwen toward already validated image-capable targets; Baseten GPT OSS 120B remains text-only in metadata.
- Production config backup: `config/config.yaml.bak.gptoss-20260622T205146Z`.
- Production package backup: `/opt/smart-llmrouter.backup.gptoss-7659981-20260622T205746Z`.
- Deployed image: `smart-llmrouter:7659981-linux-amd64`.
- Live config SHA-256 after metadata note: `f2026ca3ab6052657cb1e13edcb6ab7e31d659251bc3b35a3567871bad292d06`.
- Verified live config has `active_openrouter_qwen_deepseek 0`.
- Verified `/readyz` and `/version` report version `7659981`, commit `7659981`, build date `2026-06-22T20:55:41Z`.
- Verified hosted docs page `/docs/configuration/router-config` includes `gpt-oss-120b` and `baseten_anthropic`.
- Direct Baseten smokes passed for `openai/gpt-oss-120b`: OpenAI Chat text, streaming with usage chunks, auto tool call, forced tool choice, Anthropic Messages text, and Anthropic Messages client tool.
- Local router smokes passed for static Baseten GPT OSS groups: OpenAI Chat text/tool and Anthropic Messages text/tool.
- Production router smokes passed for static Baseten GPT OSS groups: OpenAI Chat realistic-budget text, OpenAI Chat tool, Anthropic Messages text, and Anthropic Messages tool.
- Claude Code CLI production smoke passed for `baseten-gpt-oss-120b-claude-smoke` text and tool/file creation.
- Codex CLI production smoke passed for `big-coder` text and tool/file creation through Responses wire API. The first Codex tool attempt failed only because local bubblewrap sandboxing blocked file writes; rerun with `--sandbox danger-full-access` in an isolated temp directory passed.
- Note: a production OpenAI Chat smoke with `max_tokens: 64` returned HTTP 200 with empty content once; realistic `max_tokens: 512` returned `OK`. GPT OSS 120B metadata now notes that small budgets can be consumed by reasoning before final content.

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
compose-router-1   smart-llmrouter:d43ef7d-linux-amd64
compose-postgres-1 postgres:18-bookworm
compose-caddy-1    caddy:2-alpine
```

## 2026-06-22 External Policy And PII Filtering Deployment

- Deployed image/package: `smart-llmrouter:7d8bc80-linux-amd64`.
- Source commit: `7d8bc80`.
- Backup path: `/opt/smart-llmrouter.backup.reconcile-pii-external-20260622T192332Z`.
- Build metadata: version `7d8bc80`, commit `7d8bc80`, build date `2026-06-22T19:20:46Z`.
- Reconciled the local external routing policy feature with merged first-class model-group `pii_filter` support, preserving both feature sets and their test coverage.
- Added production-hosted Docusaurus docs for PII filtering and kept the external routing policy docs available under `/docs/configuration/external-routing-policy`.
- Preserved live production `compose/config`, `compose/state`, `compose/logs`, `.env`, and `ROUTER_TOKEN*.txt` files during package replacement.
- Verified locally before deployment: focused `ExternalRoutingPolicy|PIIFilter|DocumentedYAMLShape` tests, full `rtk go test ./...`, and `rtk make docs-build`. Docs npm audit still reports the existing 31 advisories: 30 moderate and 1 high.
- Verified production `/readyz` returns version `7d8bc80`, hosted `/docs/configuration/pii-filtering`, authenticated `/v1/models`, authenticated `high` chat completion, Codex CLI text smoke through OpenAI Responses on `big-coder`, and Claude Code `claude -p` text smoke through Anthropic Messages on `big-coder`.
- Checked router logs after deployment; no new startup/runtime errors were present.
- Removed uploaded package files from `/tmp` and ran `sudo docker system prune -f` after the deployment was healthy.

## 2026-06-19 Ajoshi TPM Increase

- Production config-only update; deployed image remains `smart-llmrouter:99088b7-linux-amd64`.
- Config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.ajoshi-tpm-20260619T170533Z`.
- Increased `ajoshi-metrum-insights-prod` from `1,200,000 TPM` to `2,500,000 TPM`.
- Left RPM and concurrency unchanged at `240 RPM` and `16 concurrent`.
- Reason: production usage showed 16 recent `429 tpm-exceeded` errors for Ajoshi from Cursor on `big-coder`/`high`, while RPM/concurrency were not the bottleneck.
- Verified `sudo docker compose config >/dev/null`, router restart, `/readyz`, live config summary, local `config.production.yaml` SHA-256 match, and an authenticated production chat smoke returning `OK`.
- Ajoshi's raw token is not stored in `ROUTER_TOKEN*.txt` on the host, so the authenticated smoke used another production caller token after validating Ajoshi's live config entry directly.

## 2026-06-19 Docs Information Architecture Deployment

- Deployed image/package: `smart-llmrouter:99088b7-linux-amd64`.
- Source commit: `99088b7`.
- Backup path: `/opt/smart-llmrouter.backup-docs-ia-20260619T154713Z`.
- Build metadata: version `99088b7`, commit `99088b7`, build date `2026-06-19T15:43:52Z`.
- Added hosted **Available Models And Access** docs, moved model discovery earlier in Hosted Quickstart, added a Reference sidebar section, and linked CLI/error/admin pages to the model-access guide.
- Preserved live production `compose/config`, `compose/state`, `compose/logs`, `.env`, and `ROUTER_TOKEN*.txt` files during package replacement.
- Verified `/readyz`, hosted docs headers, hosted `/docs/getting-started/available-models` content and sidebar, authenticated `/v1/models` returning the expected caller allow list, and an authenticated `small` chat completion returning `OK`.
- Removed the uploaded package from `/tmp` and ran `sudo docker system prune -f` after the deployment was healthy.

## 2026-06-19 Model Discovery Docs Deployment

- Deployed image/package: `smart-llmrouter:8c2ad4e-linux-amd64`.
- Source commit: `8c2ad4e`.
- Backup path: `/opt/smart-llmrouter.backup-model-discovery-docs-20260619T152229Z`.
- Build metadata: version `8c2ad4e`, commit `8c2ad4e`, build date `2026-06-19T15:20:23Z`.
- Updated hosted Docusaurus docs so callers can clearly discover allowed model groups by calling `/v1/models` with their router token.
- Preserved live production `compose/config`, `compose/state`, `compose/logs`, `.env`, and `ROUTER_TOKEN*.txt` files during package replacement.
- Verified `/readyz`, hosted docs headers, hosted API Compatibility content, authenticated `/v1/models` returning the expected caller allow list, and an authenticated `small` chat completion returning `OK` with `finish_reason: stop`.
- Removed the uploaded package from `/tmp` and ran `sudo docker system prune -f` after the deployment was healthy.

## 2026-06-19 Steen Production Caller

- Production config-only update; deployed image remains `smart-llmrouter:a07c60c-linux-amd64`.
- Config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.steen-token-20260619T150904Z`.
- Added caller `steen-metrum-insights-prod` for Steen with public token ID `rtr_metrum_steen_metrum-insights_prod_k20260619`, project `metrum-insights`, environment `prod`.
- Matched the regular developer access profile: `default`, `vision`, `fast`, `small`, `medium`, `high`, `big-coder`, and `warp-agent-smoke`.
- Matched the regular developer rate/quota profile: 240 RPM, 1,200,000 TPM, 16 concurrent, 10,000 daily requests, 100,000,000 daily tokens, 1,200,000,000 monthly tokens, and 4,000,000,000 lifetime tokens.
- Stored the raw token only in `/opt/smart-llmrouter/compose/ROUTER_TOKEN_STEEN.txt` with mode `0600`.
- Verified `sudo docker compose config >/dev/null`, router restart, `/readyz`, authenticated `/v1/models` returning the 8 expected groups, and an authenticated `small` chat completion returning `OK` with `finish_reason: stop`.
- Synced ignored local `config.production.yaml` from the live production config; SHA-256 `4b6fee522674c3d9c62a0ece700c34f671c8e71ba17df2a238f086e647051175` matches production.

## 2026-06-19 Harbor Docs Package Deployment

- Deployed image/package: `smart-llmrouter:a07c60c-linux-amd64`.
- Source commit: `a07c60c`.
- Backup path: `/opt/smart-llmrouter.backup-harbor-reusable-docs-20260619T050447Z`.
- Build metadata: version `a07c60c`, commit `a07c60c`, build date `2026-06-19T05:02:26Z`.
- Deployed hosted Docusaurus docs with the reusable Harbor caller guidance on `/docs/evaluation/harbor-case-study`.
- Preserved live production `compose/config`, `compose/state`, `compose/logs`, `.env`, `ROUTER_TOKEN.txt`, and `ROUTER_TOKEN_HARBOR.txt` during package replacement.
- Verified `/readyz`, hosted docs headers, hosted Harbor page content, authenticated `/v1/models` with the Harbor token returning 19 groups, and an authenticated `high` chat completion returning `OK` with `finish_reason: stop`.
- Removed the uploaded package from `/tmp` and ran `sudo docker system prune -f` after the deployment was healthy.

## 2026-06-19 Reusable Harbor Caller

- Production config-only update; deployed image at the time was `smart-llmrouter:1fa6bef-linux-amd64`.
- Config backup: `/opt/smart-llmrouter/compose/config/config.yaml.bak.harbor-reusable-20260619T045217Z`.
- State backup: `/opt/smart-llmrouter/compose/state/router-state.json.bak.harbor-reusable-20260619T045217Z`.
- Removed 66 temporary Harbor benchmark callers from production config and pruned stale Harbor benchmark quota state.
- Added reusable caller `harbor-reusable-prod` with public token ID `rtr_metrum_harbor_harbor_prod_reusable-20260619`, project `harbor`, environment `prod`, and access to all 19 deployed model groups.
- Stored the raw Harbor token only in `/opt/smart-llmrouter/compose/ROUTER_TOKEN_HARBOR.txt` with mode `0600`.
- Verified production config summary: 29 callers, 19 model groups, 0 temporary Harbor callers, 1 reusable Harbor caller.
- Verified `sudo docker compose config >/dev/null`, router restart, `/readyz`, authenticated `/v1/models` with the Harbor token returning 19 groups, and an authenticated `high` chat smoke with a realistic 1024-token budget returning `OK`.
- Synced ignored local `config.production.yaml` from the live production config; SHA-256 matches the remote config.

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

Build both Docker package architectures, then deploy the package that matches the production host:

```bash
make package-docker
scp -i ~/.ssh/chetan-jun-2026.pem dist/smart-llmrouter-<version>-docker-linux-amd64.tar.gz ubuntu@100.30.225.66:/tmp/
ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66
sudo mv /opt/smart-llmrouter /opt/smart-llmrouter.backup.$(date +%Y%m%d%H%M%S)
sudo mkdir -p /opt/smart-llmrouter
sudo tar -C /opt/smart-llmrouter --strip-components=1 -xzf /tmp/smart-llmrouter-<version>-docker-linux-amd64.tar.gz
cd /opt/smart-llmrouter
sudo docker load -i images/smart-llmrouter-<version>-linux-amd64.tar
```

Package builds copy Markdown only from `scripts/package_docs_allowlist.txt` and run content validation against the tarball. Internal production runbooks and notes with private host/IP markers, SSH usernames/key paths, live compose config/env/token paths, or raw token/provider-key patterns must stay in this repository's private operational docs and must not be added to package artifacts.

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
  - `no-eligible-target` when the requested group has no upstream target for the requested dialect/tool/structured-output/modality shape.
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
- Historical packaging note: this deployment packaged every `docs/*.md` internal document. Superseded on 2026-06-24 by explicit package-doc allowlisting and package-content validation that keeps private production runbooks out of release artifacts.
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

### 2026-06-19 Plan & Validate docs deployment

Package `smart-llmrouter:7635a89-linux-amd64` was deployed to production to publish the customer-facing deployment planning and quality documentation cleanup.

Source commit: `7635a89` (`Polish deployment planning docs`)

Documentation changes:

- Renamed the public docs section from `Evaluation` to `Plan & Validate`.
- Replaced the old deployment-evaluation page with `Deployment Readiness`.
- Cleaned visible headings across the public docs to use product-oriented terms such as `Proof Points To Verify`, `External Vendor Links`, `Operational Readiness`, `Activation Standard`, `Agentic Quality Validation`, `Release Gates`, and `Ongoing Governance`.
- Updated `AGENTS.md` guidance so future docs use deployment readiness, validation, outcome, and operational wording instead of internal evaluation-language framing.

Production backup:

```text
/opt/smart-llmrouter.backup.plan-validate-docs-20260619T031840Z
```

Validation:

```text
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 7635a89, build_date 2026-06-19T03:16:30Z
production /version after deploy: 7635a89, build_date 2026-06-19T03:16:30Z, go1.25.11 linux/amd64
hosted docs /docs/evaluation/deployment-readiness returned 200 with `Deployment Readiness`, `Outcome And Cost Control`, and `Operational Readiness`
hosted docs /docs/evaluation/competitive-landscape returned 200 with `Where GenAI Smart Router Wins`, `Proof Points To Verify`, and `External Vendor Links`
hosted docs /docs/evaluation/model-group-quality returned 200 with `Agentic Quality Validation`, `Release Gates`, and `Ongoing Governance`
production explicit high /v1/chat/completions: HTTP 200, finish_reason=stop, final content OK with realistic token budget
production cleanup: removed uploaded package, confirmed zero /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f
```

### 2026-06-19 Public docs binary-version wording deployment

Package `smart-llmrouter:1fa6bef-linux-amd64` was deployed to production to remove source-control terminology from the public Docusaurus product docs and docs response metadata.

Source commit: `1fa6bef` (`Hide source-control metadata from public docs`)

Documentation/runtime changes:

- Replaced public wording such as `source commit and package version` with `binary release version`.
- Updated product docs to describe running binary version and build timestamp rather than source-control details.
- Removed the docs-browser commit badge field from the Docusaurus config and rendered badge.
- Removed `X-Smart-LLMRouter-Commit` from docs responses while keeping `X-Smart-LLMRouter-Version` and `X-Smart-LLMRouter-Build-Date`.
- Added regression coverage that embedded docs responses do not expose the docs commit header.

Production backup:

```text
/opt/smart-llmrouter.backup.public-docs-no-source-metadata-20260619T033643Z
```

Validation:

```text
go test ./internal/router: passed, 79 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version 1fa6bef, build_date 2026-06-19T03:34:44Z
production /version after deploy: 1fa6bef, build_date 2026-06-19T03:34:44Z, go1.25.11 linux/amd64
hosted docs /docs/evaluation/operational-acceptance returned 200 with `binary release version` and `build timestamp`, and without `source commit` or `package version`
hosted docs /docs/reference/api-compatibility returned 200 with `running binary version` and `build timestamp`, and without `commit`
hosted docs response headers include `x-smart-llmrouter-version` and `x-smart-llmrouter-build-date`; `x-smart-llmrouter-commit` is absent
hosted docs JS bundle contains no `routerCommit`, `X-Smart-LLMRouter-Commit`, `source commit`, or `package version`
production explicit high /v1/chat/completions: HTTP 200, finish_reason=stop, final content OK with realistic token budget
production cleanup: removed uploaded packages, confirmed zero /tmp/smart-llmrouter-*tar* files, ran sudo docker system prune -f
```

### 2026-06-21 External routing policy deployment

Package `smart-llmrouter:e1f7749-linux-amd64` was deployed to production to add first-class external routing policy services.

Source commit: `e1f7749` (`Add external routing policy strategy`)

Runtime and documentation changes:

- Added `strategy: external` with `external_policy` config for standalone HTTP routing policy services.
- Added fail-closed `502 routing-policy-error` behavior for invalid or unavailable policy services, with explicit `on_error: fallback` support.
- Added public Docusaurus docs for external routing policy service setup, request/response schema, security boundaries, and the tested prompt-size demo service.
- Added internal runbook and committed demo service under `examples/external-routing-policy/`.
- Cleaned local generated Harbor artifacts so `go test ./...` runs cleanly in the repo.

Production backup:

```text
/opt/smart-llmrouter.backup.external-policy-20260621T164600Z
```

Validation:

```text
go test ./...: passed, 86 tests after cleaning ignored Harbor artifact workspaces
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64: passed
production /readyz after deploy: 200, version e1f7749, build_date 2026-06-21T16:43:44Z
production /version after deploy: e1f7749, build_date 2026-06-21T16:43:44Z, go1.25.11 linux/amd64
hosted docs /docs/configuration/external-routing-policy returned 200 with version headers
production normal chat smoke against `high`: HTTP 200
temporary production external-policy smoke:
  short prompt selected openrouter/deepseek-v4-flash:nitro with strategy external, status 200
  long prompt selected minimax/MiniMax-M3 with strategy external, status 200
temporary external-policy-smoke group and policy-service container were removed after validation
production cleanup: removed uploaded package and /tmp/smart-llmrouter-* scratch files, removed older smart-llmrouter Docker images while keeping current e1f7749 and previous 99088b7 rollback image, ran sudo docker system prune -f
```

### 2026-06-24 Structured-output routing and Harbor validation deployment

Package `smart-llmrouter:d43ef7d-linux-amd64` was deployed to production to bring the structured-output routing, documentation, and smoke-validation changes from `origin/main` into the Metrum engineering deployment.

Source commit: `d43ef7d` (`Add structured output smoke validation`)

Runtime and validation changes:

- Deployed the latest router package containing structured-output request eligibility, docs, and validation updates.
- Ran production Harbor e2e validation for `aider/polyglot_python_two-bucket` across Codex CLI and Claude Code CLI for `default`, `fast`, `small`, `medium`, `high`, and `big-coder`.
- Used the reusable production Harbor caller for the matrix; no per-run production caller tokens were created.
- Created GitHub issue #65 to track caller-visible handling of upstream balance exhaustion, quota, and provider rate-limit failures after OpenRouter credits were exhausted during the first validation pass.

Production backup:

```text
/opt/smart-llmrouter.backup-d43ef7d-20260624T162206Z
```

Validation:

```text
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
go test ./cmd/... ./internal/...: passed, 188 tests
go test ./...: passed, 188 tests
make package-docker produced dist/smart-llmrouter-d43ef7d-docker-linux-amd64.tar.gz
package SHA-256: 4145ecd6e6f4dd9f93c1e82c8a4fb886839de1fa7742584e70c1d15f7007bf3e
Harbor production e2e:
  Claude Code default/fast/small: passed, reward 1, errors 0
  Claude Code medium/high/big-coder: passed, reward 1, errors 0
  Codex high/big-coder: passed, reward 1, errors 0
  Codex default/fast: passed on serialized rerun after OpenRouter credits were restored, reward 1, errors 0
  Codex small/medium: passed on second serialized rerun after cooldown, reward 1, errors 0
  Earlier Codex rows produced verifier-pass or early-process failures while OpenRouter credits/rate limits were exhausted; follow-up is tracked in issue #65
production Harbor usage report generated: /opt/smart-llmrouter/compose/logs/harbor-predeploy-20260624T154224Z-20260624T160418Z.md
production /readyz after deploy: 200, version d43ef7d, build_date 2026-06-24T15:41:01Z
production /version after deploy: d43ef7d, build_date 2026-06-24T15:41:01Z, go1.26.4 linux/amd64
hosted docs /docs/reference/api-compatibility returned 200 with d43ef7d version headers
production authenticated /v1/models returned 18 visible model groups
production authenticated chat smoke against `high`: HTTP 200, returned OK
production structured-output chat smoke against `high`: HTTP 200, returned {"status":"ok"}
production cleanup: removed uploaded package and ran sudo docker system prune -f
```

### 2026-06-23 Usage report performance sections deployment

Package `smart-llmrouter:133e645-linux-amd64` was deployed to production to add first-class performance triage sections to generated usage reports.

Source commit: `133e645` (`Add performance sections to usage reports`)

Runtime and documentation changes:

- Added `Downstream User Performance` grouped by user, project, environment, and client.
- Added `Upstream Endpoint Performance` grouped by provider, model, and API dialect.
- Included average/max latency, TTFB, upstream/downstream duration, token throughput, errors, attempts, fallbacks, streams, tokens, and cost where applicable.
- Updated internal usage reporting docs, Docker deployment docs, public Docusaurus usage reporting docs, README, and AGENTS.md.

Production backup:

```text
/opt/smart-llmrouter.backup.perf-report-20260623T134718Z
```

Validation:

```text
go test ./internal/router -run 'TestUsageReport': passed, 3 tests
go test ./cmd/... ./internal/...: passed, 102 tests
go test ./...: passed, 102 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64 from clean worktree: passed
production /readyz after deploy: 200, version 133e645, build_date 2026-06-23T13:44:58Z
production /version after deploy: 133e645, build_date 2026-06-23T13:44:58Z, go1.25.11 linux/amd64
hosted docs /docs/operations/usage-reporting returned 200 with version headers
production router-usage-report --since 24h generated a report containing Downstream User Performance, Upstream Endpoint Performance, and Per-Request Throughput sections
production authenticated chat smoke with realistic token budget returned HTTP 200 through the weighted high group
production cleanup: removed uploaded package, removed superseded switch directory, ran sudo docker system prune -f
```

### 2026-06-25 Auth, reporting, and upstream-error production refresh

Package `smart-llmrouter:97d796d-linux-amd64` was deployed to production after merging the account key lifecycle, upstream quota-error, Casbin authorization, content-capture authorization, DB-backed authz policy, and OIDC admin-session PRs.

### 2026-06-26 Production refresh to upstream main 289ea71

Package `smart-llmrouter:289ea71-linux-amd64` was deployed to production after refreshing local `main` from `origin/main`.

Source commit: `289ea71`

Production backup:

```text
/opt/smart-llmrouter.backup.refresh-289ea71-20260626T211109Z
```

Validation:

```text
go test ./cmd/... ./internal/...: passed, 340 tests across 7 packages
make docs-build: passed; npm audit still reports existing moderate docs-site dependency advisories
make package-docker from clean tracked worktree: passed for linux/amd64 and linux/arm64 packages
production /readyz after deploy: 200, version 289ea71, build_date 2026-06-26T21:07:13Z
hosted docs /docs/ returned 200 with 289ea71 version headers
browser admin reports /admin/reports/ without credentials returned 401
browser admin reports /admin/reports/ with the configured Basic admin user returned 200 HTML
production authenticated /v1/models returned 20 model groups
production authenticated chat smoke against high with max_tokens 128 returned HTTP 200 and content OK
production authenticated chat smoke against default with max_tokens 64 returned HTTP 200 and content OK
production Harbor full matrix case prod-refresh-289ea71-20260626T211238Z ran codex and claude-code across default, fast, small, medium, high, and big-coder
Harbor full matrix result: 10/12 passed; codex passed all six groups, claude-code passed fast, medium, high, and big-coder; claude-code default and small produced verifier reward 0 with zero Harbor exceptions
Harbor retry case prod-refresh-289ea71-retry-20260626T213832Z passed claude-code default; claude-code small failed again with verifier reward 0 and zero Harbor exceptions
production Harbor filtered report for caller harbor/harbor/prod generated 108 requests, 0 router errors, 1,155,436 total tokens, and status 200 for the Harbor validation window
production cleanup: removed uploaded package and ran sudo docker system prune -f
```

Source commit: `97d796d`

Production backup:

```text
/opt/smart-llmrouter.backup.refresh-97d796d-20260625T034812Z
```

Validation:

```text
go test ./cmd/... ./internal/...: passed, 277 tests
make secret-check: passed
make package-docker from clean tracked worktree: passed for linux/amd64 and linux/arm64 packages
production /readyz after deploy: 200, version 97d796d, build_date 2026-06-25T03:44:18Z
production /version after deploy: 97d796d, build_date 2026-06-25T03:44:18Z, go1.26.4 linux/amd64
hosted docs /docs/overview returned 200 with 97d796d version headers
production authenticated chat smoke against high with max_tokens 256 returned HTTP 200 and content OK
browser admin auth check with the configured Basic admin user returned 200 safe subject metadata
browser admin reports returned 404 because server.admin_reports is not enabled in the preserved production config
production router-usage-report --since 1h generated a report with Downstream User Performance, Upstream Endpoint Performance, and Per-Request Throughput sections
production Harbor e2e smoke case harbor-prod-97d796d-20260625T035100Z passed for codex/small and claude-code/small with reward 1 and zero Harbor verifier errors
production Harbor filtered report for caller harbor/harbor/prod generated 14 requests, 0 errors, 140951 tokens, and 0 fallbacks for the Harbor smoke window
production cleanup: removed uploaded package, removed /opt/smart-llmrouter.replaced.refresh-97d796d-20260625T034812Z, ran sudo docker system prune -f
```

### 2026-06-25 Production admin reports enablement

Enabled the authenticated browser admin reports surface in the live production config after the `97d796d` package deployment.

Live config backup:

```text
/opt/smart-llmrouter/compose/config/config.yaml.bak.enable-admin-reports-20260625T035706Z
```

Config changes:

- Enabled `server.admin_reports` at `/admin/reports`.
- Enabled static Casbin authorization under `server.admin_auth.authorization`.
- Granted the existing `basic:admin` subject in domain `metrum/prod` `admin:reports` `read|export`.
- Synced ignored local `config.production.yaml` from the live host after validation.

Validation:

```text
production /readyz after config restart: 200, version 97d796d
production /admin/reports/ with Basic admin credentials: 200 HTML
production /admin/reports/api/summary?since=24h with Basic admin credentials: 200 JSON with production request totals
production /admin/reports/export.md?since=1h with Basic admin credentials: 200 Markdown report
ordinary router caller token against /admin/reports/api/summary?since=24h: 403 reports-forbidden
```

### 2026-06-23 Harbor case study context docs deployment

Package `smart-llmrouter:6b3c1fc-linux-amd64` was deployed to production to update the hosted Docusaurus Harbor case study with clearer product context for outcome-based model-group evaluation.

Source commit: `6b3c1fc` (`Improve Harbor case study context`)

Documentation changes:

- Added public Harbor case study framing that explains why different GenAI/VLM/agent models fit different workloads.
- Clarified that the operational target is the best cheapest model or model mix that still completes the job with the required quality, latency, and reliability.
- Explained that Harbor is one example agent-evaluation harness, while deployments can use any workload-specific verifier such as unit tests, extraction accuracy checks, OCR targets, tool-call correctness, browser-control tasks, golden datasets, or product acceptance tests.
- Mirrored the same context in the internal Harbor case study note.
- Updated AGENTS.md so future benchmark and case-study docs include product decision context before raw benchmark tables.

Production backup:

```text
/opt/smart-llmrouter.backup.harbor-docs-20260623T172653Z
```

Validation:

```text
go test ./cmd/... ./internal/...: passed, 102 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64 VERSION=6b3c1fc COMMIT=6b3c1fc from clean worktree: passed
production /readyz after deploy: 200, version 6b3c1fc, build_date 2026-06-23T17:24:51Z
production /version after deploy: 6b3c1fc, build_date 2026-06-23T17:24:51Z, go1.25.11 linux/amd64
hosted docs /docs/evaluation/harbor-case-study returned 200 and contains the new Why Outcome-Based Model Evaluation Matters and How To Read This Case Study sections
production cleanup: removed uploaded package, removed duplicate previous switch directory, ran sudo docker system prune -f
local cleanup: removed temporary clean deployment worktree and stale Claude Code worktree
```

### 2026-06-23 Graphical report examples docs deployment

Package `smart-llmrouter:d9aa8e3-linux-amd64` was deployed to production to add anonymized graphical usage, savings, and performance report examples to the hosted Docusaurus docs.

Source commit: `d9aa8e3` (`Add graphical usage report examples`)

Documentation changes:

- Added `docs/operations/report-examples` with anonymized production-derived daily usage, savings, caller cohort, and upstream endpoint performance examples.
- Added Chart.js-based report example visualizations in the Docusaurus layer instead of hardcoding charts in the router report generator.
- Linked the examples from Usage Reporting and Cost Governance.
- Replaced a concrete project value in the public usage-reporting filter example with neutral placeholders.
- Updated AGENTS.md to keep graphical report examples in Docusaurus unless chart output is explicitly required from the binary.

Production backup:

```text
/opt/smart-llmrouter.backup.report-examples-20260623T140358Z
```

Validation:

```text
go test ./...: passed, 102 tests
make docs-build: passed; npm audit still reports existing docs-site dependency advisories
make package-docker GOOS=linux GOARCH=amd64 from clean worktree: passed
production /readyz after deploy: 200, version d9aa8e3, build_date 2026-06-23T14:01:51Z
production /version after deploy: d9aa8e3, build_date 2026-06-23T14:01:51Z, go1.25.11 linux/amd64
hosted docs /docs/operations/report-examples returned 200 with Chart.js canvas elements and expected chart headings
hosted docs /docs/operations/usage-reporting returned 200 and links to Report Examples
production router-usage-report --since 24h still generated Downstream User Performance, Upstream Endpoint Performance, and Per-Request Throughput sections
browser pixel-level validation was not run because no local browser automation runtime was installed in the workspace
production cleanup: removed uploaded package, removed superseded switch directory, ran sudo docker system prune -f
```
