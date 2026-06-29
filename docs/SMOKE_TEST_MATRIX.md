# Smoke Test Matrix

Run smokes at the narrowest layer that proves the change, then run production-level smokes for deployed behavior.

For quality complaints or router-versus-fixed-model decisions, do not treat a smoke test as a full evaluation. A smoke proves that one request shape works. Use [Evaluation Evidence Playbook](EVALUATION_EVIDENCE_PLAYBOOK.md) when the decision depends on workload outcomes, repeated runs, cost, latency, fallbacks, and a fixed-model or previous-policy control.

## Core API Smokes

| Change type | Required smoke |
|---|---|
| Config validation | YAML parse and `docker compose config` |
| Health/deploy | `/readyz`, `/version`, router logs |
| Auth/allow list | `/v1/models` with caller token |
| Admin Basic Auth | `/admin/auth/check` with missing, bad, and valid Basic credentials when enabled |
| Omitted model behavior | request without `model`, expect configured default or `400 missing-model` |
| Text routing | relevant dialect with realistic token budget |
| Max-token cap | request with `max_tokens: 1`, OpenAI Chat `max_completion_tokens: 1`, or Responses `max_output_tokens: 1` |
| OpenAI-compatible encoding | prove `force_store_false` and `output_token_field` produce the upstream payload the exact provider accepts |
| Usage/cost fields | query usage DB/report after a request |
| Request evidence bundle | produce one success or error request, query `/admin/reports/api/request-evidence?request_id=<request_id>` with a drilldown-authorized admin, verify section completeness, attempts when applicable, stored request-time costs, `Cache-Control: no-store`, ordinary caller `403 reports-forbidden`, and no raw prompts/images/tool schemas/tool outputs/tokens/token hashes/provider keys/upstream bodies |
| Caller traffic-shaping reports | enable a low caller `traffic_shape`, produce one queued or rejected request, run `router-usage-report --traffic-shaped-only`, and open Traffic shaping overview / Shaping users in admin reports |
| Traffic tuning advisor | run `router-usage-report --traffic-tuning-advisor --since 24h` and `/admin/reports/api/traffic-tuning-advisor?since=24h`; verify admitted upstream 400 cases recommend route-around/investigation, actual shaping rejections recommend burst/queue review, provider 429s across users recommend provider shaping/backoff, and outputs contain only safe scalar evidence |
| Provider/model/target shaping | enable a low local `traffic_shape`, send parallel requests from two caller tokens, verify skip/fallback or `503 upstream-capacity-throttled`, `Retry-After` when calculable, and safe `request_upstream_shape_events` rows |
| Provider-shaping reports | after provider/model/target shaping smoke, open Provider shaping and Backoff admin tabs and confirm charts/tables show skipped targets or cooldown starts without prompts, tokens, token hashes, or provider keys |
| Adaptive upstream backoff | simulate upstream `429` with bounded `Retry-After` and provider quota/billing exhaustion; verify the next request skips the affected target until cooldown expires and records `adaptive-backoff-provider-429` or `adaptive-backoff-provider-quota` |
| Decision telemetry | with `server.decision_telemetry.enabled: true`, run success, no-eligible-target, policy fail-closed, policy fallback, upstream-fallback-success, and cache-bypass requests; query `request_policy_executions`, `request_fallback_transitions`, score/ranking rows, safe fingerprints, and `router-usage-report` summary buckets |
| Multimodal/VLM routing | direct upstream image smoke for the exact provider/model/dialect, router-level image smoke through the intended model group, URL safety negative smoke, tiny-cap smoke, usage row image/cost fields, and no raw image persistence |
| Coding-agent client compatibility | deterministic fixture matrix with `rtk python3 scripts/coding_agent_matrix.py --mode mock`, then live Codex/Claude Code/opencode/aider smokes when the route change affects those clients |
| Kubernetes deployment artifacts | `kubectl kustomize deploy/kubernetes/overlays/example`, YAML parse, `kubectl apply --dry-run=client` or server dry-run when available, then staging port-forward smoke for `/readyz`, `/docs/`, `/version`, `/v1/models`, one chat request, admin reports when enabled, and metrics/admin denial for ordinary caller tokens |

## Release Validation Matrix

Use the release validation matrix before handing a binary, Docker package, Compose bundle, or Kubernetes manifest set to another operator:

```bash
make release-validation-matrix
```

The target runs the package-content self-tests, build-metadata validator, clean-tree validator self-test, Docker build-context guard, Compose security check, and Kubernetes overlay rendering when `kubectl` or `kustomize` is available. It does not require provider keys, router tokens, a Docker daemon, or a live Kubernetes cluster. After packages are built, run the same matrix with artifact validation:

```bash
python3 scripts/validate_release_matrix.py --include-artifacts
```

Full package creation and live deployment validation remain separate, potentially expensive checks:

```bash
make package-all
make package-docker-all
make e2e-compose-live
```

## Release Package Smokes

Release packaging smokes prove that artifacts are deterministic, external-safe, and architecture-correct before handoff.

| Artifact | Required smoke |
|---|---|
| Package validation self-test | `python3 scripts/validate_package_contents_test.py` and `python3 scripts/validate_release_clean_test.py` |
| Binary amd64 package | Clean tree, `make package-all`, validate `dist/smart-llmrouter-*-linux-amd64.tar.gz`, confirm x86-64 ELF binaries |
| Binary arm64 package | Clean tree, `make package-all`, validate `dist/smart-llmrouter-*-linux-arm64.tar.gz`, confirm aarch64 ELF binaries |
| Docker amd64 package | Docker daemon available, `make package-docker-all`, validate `dist/smart-llmrouter-*-docker-linux-amd64.tar.gz`, load image tar, run `/app/bin/router --version` and helper `--version` commands |
| Docker arm64 package | Docker daemon and buildx platform support available, `make package-docker-all`, validate `dist/smart-llmrouter-*-docker-linux-arm64.tar.gz`, load image tar, run `/app/bin/router --version` and helper `--version` commands where runner architecture or emulation allows |
| macOS release host | Confirm package tarballs contain no AppleDouble `._*` entries; package recipes set `COPYFILE_DISABLE=1` and validator rejects any accidental metadata entries |
| Compose assets | Extract Docker package, verify `compose/.env` pins `SMART_LLMROUTER_VERSION=<version>-linux-<arch>`, set deployment-owned passwords/DSNs, and run `docker compose config >/dev/null` |

## Release Package Smokes

Release packaging smokes prove that artifacts are deterministic, external-safe, and architecture-correct before handoff.

| Artifact | Required smoke |
|---|---|
| Package validation self-test | `python3 scripts/validate_package_contents_test.py` and `python3 scripts/validate_release_clean_test.py` |
| Binary amd64 package | Clean tree, `make package-all`, validate `dist/smart-llmrouter-*-linux-amd64.tar.gz`, confirm x86-64 ELF binaries |
| Binary arm64 package | Clean tree, `make package-all`, validate `dist/smart-llmrouter-*-linux-arm64.tar.gz`, confirm aarch64 ELF binaries |
| Docker amd64 package | Docker daemon available, `make package-docker-all`, validate `dist/smart-llmrouter-*-docker-linux-amd64.tar.gz`, load image tar, run `/app/bin/router --version` and helper `--version` commands |
| Docker arm64 package | Docker daemon and buildx platform support available, `make package-docker-all`, validate `dist/smart-llmrouter-*-docker-linux-arm64.tar.gz`, load image tar, run `/app/bin/router --version` and helper `--version` commands where runner architecture or emulation allows |
| macOS release host | Confirm package tarballs contain no AppleDouble `._*` entries; package recipes set `COPYFILE_DISABLE=1` and validator rejects any accidental metadata entries |
| Compose assets | Extract Docker package, verify `compose/.env` pins `SMART_LLMROUTER_VERSION=<version>-linux-<arch>`, set deployment-owned passwords/DSNs, and run `docker compose config >/dev/null` |

## Evidence Evaluation Smokes

Before promoting or rolling back a model group based on a quality claim:

- run the narrow smoke for every affected API shape, tool dialect, modality, reasoning control, token cap, and timeout;
- record the config version or safe routing/config summary, selected provider/model, usage tokens, latency, attempts, fallback, and error fields;
- join the smoke window to any Harbor, acceptance-test, or external evaluation result by timestamp, caller/project, client, model group, request ID, or run label;
- if the smoke passes but the workload evaluation fails, treat it as a model-group quality issue rather than a transport compatibility issue;
- if the smoke fails, fix or roll back the target before interpreting broader evaluation results.


## API Dialect Conformance Gate

Run this gate before changing request parsing, upstream encoding, tool routing, structured outputs, reasoning controls, streaming behavior, max-token handling, or provider-hosted tool policy.

| Surface | Required deterministic checks | Router test coverage |
|---|---|---|
| OpenAI Chat Completions | Plain text, caller streaming flag, `max_tokens`, `max_completion_tokens`, same-dialect tool passthrough, `tool_choice`, JSON-schema `response_format`, and `reasoning_effort` | `go test ./internal/router -run TestAPIDialectConformance` |
| OpenAI Responses | Plain input, `max_output_tokens`, same-dialect function/namespace tool passthrough, generic hosted search/image descriptor stripping, remote hosted tool rejection, JSON-schema `text.format` | `go test ./internal/router -run TestResponsesConformance` |
| Anthropic Messages | Messages payloads, caller `max_tokens`, `thinking` passthrough, default max-token injection when omitted | `go test ./internal/router -run TestAPIDialectConformance` |
| Cross-surface routing | Tool/structured/reasoning/image/cap eligibility and `no-eligible-target` behavior | existing `service_test.go` request-shape and target-filter tests plus live smokes for provider activation |

The conformance gate is intentionally mock-upstream and deterministic. It proves router semantics, not provider quality. Provider/model activation still requires the direct and router-level live smokes in the provider sections below.

For OpenAI-compatible providers, distinguish generic translation from same-dialect passthrough. Generic translation can normalize fields and force upstream unary calls. Same-dialect passthrough is the path that preserves client tool declarations and structured-output payloads for compatible upstreams.

## Robustness And Fallback Smokes

Use deterministic mock upstreams for robustness gates. Do not use live providers for timeout, malformed body, controlled `429`, forced `5xx`, or queue-depth failure injection unless the provider explicitly offers a staging endpoint for that behavior.

| Failure mode | Required proof |
|---|---|
| Fallback ordering | Static/failover groups attempt targets in configured order; weighted groups use a deterministic test seed or mock distribution check; fallback stays inside the requested group. |
| Retryable upstream failure | `429`, `5xx`, timeout, connection reset, malformed JSON, and truncated streaming fixtures either recover through a compatible fallback or return the documented terminal error. |
| Non-retryable upstream failure | Ordinary upstream `400` stops fallback unless a deployment explicitly treats the class as safe to replay. |
| Request-shape fallback safety | Tool, image, structured-output, reasoning, and explicit max-token-cap requests never fallback to a target that was filtered out for that request shape. |
| Caller limits | RPM, TPM, concurrency, quota, lifetime budget, and `traffic_shape` failures return safe `429` or `403` errors with request IDs and no upstream attempt when blocked before routing. |
| Adaptive backoff | Provider `429` with and without `Retry-After` and provider quota/billing responses start bounded cooldown rows and route around the affected target when another compatible target exists. |
| Diagnostics | `request_usage`, `request_attempts`, `request_trace_events`, `request_fallback_transitions`, `request_traffic_shape_events`, and `request_upstream_shape_events` contain safe scalar fields for the request ID without prompts, images, raw provider bodies, provider keys, router tokens, or token hashes. |

Focused local command:

```bash
go test ./internal/router -run 'TestFallbackOrdering429RetryAfterTelemetryAndSecretRedaction|TestFallbackDoesNotCrossToolEligibility|TestCallerRPMErrorIsSafeAndSkipsUpstream|TestAdaptiveBackoffHonorsBoundedRetryAfter|TestTrafficShapeRequestStartRejectsBeforeUpstream'
```

Production-safe robustness smoke:

1. Use a dedicated test caller and a mock/staging upstream target for forced timeout, `429`, and `5xx` behavior.
2. Run a small burst within limits, then a controlled burst exceeding the test caller's shaping or RPM limit.
3. Run one route-around smoke where one target is cooled down and a compatible fallback succeeds.
4. Query a report window by request IDs, caller project/environment, client, and model group.
5. Confirm the report shows attempts, fallback, shaping/backoff, latency, cost, errors, and selected upstreams without secret material.

## Outcome Workload Gates

Smoke tests prove transport compatibility; outcome gates prove the model group still completes the workload. Before promotion, document the run matrix, reward/verifier, client/API matrix, fixed-model or previous-policy control when practical, pass/fail thresholds, cost and latency ceilings, and rollback criteria.

Local/mock CI command:

```bash
python3 scripts/evaluate_workload_gate_test.py
```

Harbor/workload gate command:

```bash
python3 scripts/evaluate_workload_gate.py \
  --matrix examples/harbor-algotune-pca/workload_gate_matrix.json \
  --results examples/harbor-algotune-pca/runs/<CASE_ID>/results.tsv \
  --out-json examples/harbor-algotune-pca/reports/<CASE_ID>/workload-gate.json \
  --out-md examples/harbor-algotune-pca/reports/<CASE_ID>/workload-gate.md
```

When a safe usage JSON export is available, pass `--usage-json` so the gate summary includes selected provider/model distribution, request IDs, stored request-time cost, latency, status, and fallback correlation. Generated gate outputs belong under ignored artifact directories. Do not print raw Harbor caller tokens, provider keys, token hashes, raw prompts, raw images, raw tool outputs, or full production config.

## Hosted OpenAI-Compatible Provider Smokes

Hosted OpenAI-compatible providers such as Crusoe Managed Inference and Fireworks AI use the same router dialect as other `/v1/chat/completions` upstreams, but every provider/model/account combination still needs direct evidence before activation.

For Crusoe, public docs checked on 2026-06-24 list `https://api.inference.crusoecloud.com/v1` as the OpenAI-compatible endpoint and `meta-llama/Llama-3.3-70B-Instruct` as the quickstart model. Direct validation on 2026-06-24 required an explicit `User-Agent`; configure one under provider `headers`. Use `CRUSOE_API_KEY` only from a protected environment or ignored `env.json`; never print it. On 2026-06-25, `nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B` was available in the account and passed direct text/cap smokes, but direct receipt-image smokes returned incorrect or non-merchant answers, so it must remain limited to a dedicated smoke group until OCR/workload validation passes.

Direct checks before any active route:

```bash
curl -fsS https://api.inference.crusoecloud.com/v1/models \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${CRUSOE_API_KEY}"

curl -fsS https://api.inference.crusoecloud.com/v1/chat/completions \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${CRUSOE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"<crusoe-model-id>","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":false}'

curl -N https://api.inference.crusoecloud.com/v1/chat/completions \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${CRUSOE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"<crusoe-model-id>","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":16,"stream":true}'

curl -fsS https://api.inference.crusoecloud.com/v1/chat/completions \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${CRUSOE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"<crusoe-model-id>","messages":[{"role":"user","content":"Write five sentences about routing."}],"max_tokens":1,"stream":false}'
```

Run OpenAI Chat tool, forced `tool_choice`, and `response_format` structured-output checks only for models intended to serve those request shapes. Add `tool_support.openai_chat` entries only after both direct Crusoe and router-level smokes pass for the exact model. Keep Crusoe out of Codex Responses and Claude Code Anthropic groups unless Crusoe exposes and passes those exact skins.

For Fireworks, public docs checked on 2026-06-28 list `https://api.fireworks.ai/inference/v1` as the OpenAI-compatible endpoint and Serverless pricing for active reference candidates. Use `FIREWORKS_API_KEY` only from a protected environment or ignored `env.json`; never print it. Direct validation showed completions may require an explicit `User-Agent` from this environment. Configure one under provider `headers`. Fireworks `accounts/fireworks/models/gpt-oss-20b` passed direct text, streaming, `max_tokens: 1`, OpenAI Chat `reasoning_effort` low/medium/high, auto tools with `max_tokens >= 256`, forced `tool_choice`, and JSON schema structured-output smokes on 2026-06-27. Fireworks `accounts/fireworks/models/glm-5p2`, `accounts/fireworks/models/kimi-k2p7-code`, `accounts/fireworks/models/deepseek-v4-flash`, and `accounts/fireworks/models/qwen3p6-plus` passed direct OpenAI Chat text and auto-tool smokes on 2026-06-28, then production router-level text smokes through `big-coder`. Keep Fireworks image-capable targets text-only until direct image and router-level image smokes pass for the exact endpoint.

Direct Fireworks checks before any active route:

```bash
curl -fsS https://api.fireworks.ai/inference/v1/models \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${FIREWORKS_API_KEY}"

curl -fsS https://api.fireworks.ai/inference/v1/chat/completions \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${FIREWORKS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"accounts/fireworks/models/gpt-oss-20b","messages":[{"role":"user","content":"Reply OK only."}],"max_tokens":64,"stream":false}'

curl -fsS https://api.fireworks.ai/inference/v1/chat/completions \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${FIREWORKS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"accounts/fireworks/models/gpt-oss-20b","messages":[{"role":"user","content":"Reply OK only."}],"reasoning_effort":"low","max_tokens":128,"stream":false}'
```

Fireworks GPT OSS 20B returns `reasoning_content` alongside visible content. Declare `reasoning` metadata only after a router-level `reasoning_effort` smoke confirms the selected target preserves the caller request shape and usage/cost telemetry remains populated. Keep Fireworks Anthropic Messages and image/audio/video routes disabled until those exact direct and router-level skins pass.

Fireworks Responses validation on 2026-06-28 used the official Responses API docs and Serverless pricing docs. The docs list `/inference/v1/responses`, client-executed function tools, provider-executed MCP/SSE tools, streaming, `max_tool_calls`, and `store=false`; they also note the Responses API has different retention behavior from Chat Completions. The router config uses a separate `fireworks_responses` provider, sets `force_store_false: true` on the validated target, rejects caller-supplied remote provider-hosted `mcp`/`sse` tools before upstream, and strips generic hosted search/image tool descriptors when the router is not exposing those services.

Direct Fireworks Responses results on 2026-06-28:

| Model | Text `store:false` | Function tool | Tool-result continuation | Streaming tool | `max_tool_calls:1` | `max_output_tokens:1` | Activation |
|---|---|---|---|---|---|---|---|
| `accounts/fireworks/models/kimi-k2p7-code` | Passed | Passed | Passed | Passed | Returned one call with incomplete status | Honored cap with incomplete status | Added to `fireworks_responses`, smoke groups, and low-weight `big-coder` tool-only target |
| `accounts/fireworks/models/glm-5p2` | Accepted but tiny text budget returned incomplete reasoning text | Passed | Passed | Passed | Returned one call with incomplete status | Honored cap with incomplete status | Not activated; keep for future workload validation |
| `accounts/fireworks/models/deepseek-v4-flash` | Accepted but tiny text budget returned incomplete text | Passed | Passed | Passed | Returned one call with incomplete status | Honored cap with incomplete status | Active in production/reference `big-coder` ordinary-text routing at 50% as of 2026-06-29; keep image and non-Chat skins disabled until separately validated |
| `accounts/fireworks/models/qwen3p6-plus` | Accepted but tiny text budget returned incomplete text | Passed | Passed | Passed | Returned one call with incomplete status | Honored cap with incomplete status | Not activated; keep for future workload validation |
| `accounts/fireworks/models/gpt-oss-20b` | Passed | Passed | Failed acceptance: continuation returned unrelated incomplete content | Passed | Did not call the tool in the probe | Honored cap with incomplete status | Not activated for Responses tools |

Direct hosted-tool probes using `mcp` and `sse` with a public test URL returned Fireworks HTTP 500. Do not expose provider-hosted Fireworks tools through the router without a separate security design with explicit allowlists, timeouts, and privacy review.

Direct Fireworks Responses checks:

```bash
curl -fsS https://api.fireworks.ai/inference/v1/responses \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${FIREWORKS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"accounts/fireworks/models/kimi-k2p7-code","input":"Reply OK only.","max_output_tokens":64,"store":false}'

curl -fsS https://api.fireworks.ai/inference/v1/responses \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${FIREWORKS_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"accounts/fireworks/models/kimi-k2p7-code","input":"Use the weather tool for San Francisco, CA.","max_output_tokens":512,"store":false,"tool_choice":"auto","tools":[{"type":"function","name":"get_weather","description":"Get current weather for a city","parameters":{"type":"object","properties":{"location":{"type":"string"}},"required":["location"],"additionalProperties":false}}]}'
```

Router-level Fireworks Responses smokes on 2026-06-28 passed for `fireworks-responses-smoke` text and `fireworks-responses-tool-smoke` function-tool requests, tool-result continuation, downstream SSE synthesis, usage fields, and `store:false` passthrough on tool requests. Negative router smokes for `mcp` and `sse` tools returned `400 provider-hosted-tools-forbidden` before upstream.

For Crusoe VLM candidates, add an image smoke before broad routing:

```bash
curl -fsS https://api.inference.crusoecloud.com/v1/chat/completions \
  -H "User-Agent: smart-llmrouter-validation" \
  -H "Authorization: Bearer ${CRUSOE_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{"model":"<crusoe-model-id>","messages":[{"role":"user","content":[{"type":"text","text":"Read the receipt image carefully. Reply with only the merchant/store chain name printed on the receipt."},{"type":"image_url","image_url":{"url":"https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}]}],"max_tokens":512,"stream":false}'
```

Accepting an image is not enough for production `vision` traffic. The response must satisfy the deployment's quality gate, for example returning the expected receipt merchant for OCR validation, and the router-level smoke must log image request metadata and request-time pricing.

Router-level checks for a dedicated Crusoe smoke group:

- `/readyz` starts cleanly with `CRUSOE_API_KEY` configured and logs do not contain secrets.
- `/v1/models` only exposes the Crusoe smoke group to an explicitly allowed test caller.
- Non-streaming `/v1/chat/completions` returns `OK` and usage rows include `target_provider=crusoe`, upstream model, tokens, cost, latency, TTFB, duration, attempts, and no fallback.
- Streaming `/v1/chat/completions` returns valid SSE if the route will serve streaming callers.
- Tool, forced-tool, and structured-output requests return `502 no-eligible-target` before an upstream attempt until validated capability metadata is present.
- Bad-key, 401/403, 429, timeout, and 5xx responses are sanitized and produce stable caller-visible router errors.
- Harbor e2e passes before a hosted OpenAI-compatible provider joins broad ordinary-text coding-agent traffic. Limited tool-only targets may be added after exact direct and router-level tool smokes when the caller dialect matches the upstream dialect and the existing fallback target set remains intact.

## Tool Smokes

| Client/API | Smoke |
|---|---|
| OpenAI Chat tools | `/v1/chat/completions` with `tools`, `tool_choice`, and streaming when supported |
| OpenAI Responses tools | Codex CLI or direct `/v1/responses` function tool request |
| Anthropic Messages tools | Claude Code or direct `/v1/messages` client tool request |
| OpenRouter-specific tool route | validate the OpenRouter skin used by the target |

For agent CLI smokes, the agent must create a file and the test must assert the file contents.

## Multimodal And VLM Smokes

Before activating an image-capable target in a broad coding or VLM group, run both direct-provider and router-level checks for the exact provider, model ID, dialect, and account entitlement.

| Gate | Required evidence |
|---|---|
| Direct upstream image smoke | Exact upstream API path accepts the intended image shape and returns a useful answer with `max_tokens` or equivalent at least `512` |
| Router-level image smoke | Same workload through the intended model group selects the expected image-capable target and records request ID, provider/model/dialect, latency, attempts, usage, and fallback state |
| Payload shape | OpenAI Chat `image_url`, OpenAI Responses `input_image`, and Anthropic Messages `image.source` shapes are validated when the group serves those clients |
| URL safety | Loopback, link-local, RFC1918/private, multicast, unspecified, malformed schemes, and redirect-to-private image URLs fail before upstream and no provider key is used |
| Private URL override | `server.upstream.allow_private_image_urls: true` is tested only for deployments that intentionally allow private VLM dereference |
| Cost and telemetry | Usage rows include `input_has_image`, `input_image_count`, upstream image tokens when reported, calculated image cost, and upstream-reported billed costs when present |
| Cap behavior | Tiny explicit caps are forwarded exactly; targets marked `honors_max_tokens: false` are skipped for capped requests |
| Quality | OCR-specific routes must return the expected merchant/name/value; merely accepting or describing an image is not sufficient |

Production examples should include receipt OCR, screenshot/UI understanding, a mixed coding task with an attached image, and negative SSRF URL rejection. Keep raw images, provider keys, router tokens, token hashes, and full production config out of logs and reports.

If an image target fails quality, cap, or URL-safety smokes after activation, roll back by removing or lowering that target in the affected model group, restoring the previous config backup, restarting the router, and rerunning `/readyz`, `/v1/models`, a text request, and the failing image smoke.

## Coding-Agent Client Matrix

Run [Coding-Agent E2E Matrix](CODING_AGENT_E2E_MATRIX.md) for route changes that affect coding groups, tool support, multimodal agent traffic, client authentication behavior, or model-group access.

Minimum deterministic check:

```bash
rtk python3 scripts/coding_agent_matrix.py --mode mock --output-dir tmp/coding-agent-matrix
```

Production or staging promotion should add live smokes for Codex CLI over OpenAI Responses, Claude Code CLI over Anthropic Messages, opencode, and aider where the client is installed and supported. Record client version, model group, request dialect, request IDs, selected upstream provider/model/dialect when available from reports, verifier result, elapsed time, and token totals.

## Reasoning And Thinking Smokes

Reasoning and thinking controls are dialect-specific. Declare `reasoning` metadata only for the exact provider/model/dialect/skin that passes the relevant direct upstream and router-level smoke. A model name or marketing claim is not enough.

| Client/API | Smoke |
|---|---|
| OpenAI Chat reasoning | Direct upstream `/chat/completions` with `reasoning_effort`, then the same request through the router group |
| OpenAI Responses reasoning | Direct upstream `/responses` with a `reasoning` object, then the same request through the router group |
| Anthropic Messages thinking | Direct upstream `/v1/messages` with `thinking.type: enabled` and `budget_tokens`, then the same request through the router group |
| Negative eligibility | Router request against a group with no compatible target; expect `502 no-eligible-target` and no upstream attempt |
| Low output cap | Verify any required `max_tokens` to `max_completion_tokens` translation and budget/max-token constraints for the exact upstream |

Add metadata only after validation:

```yaml
providers:
  example:
    models:
      reasoning-model:
        model: provider-model-id
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum
```

Use `control: effort_enum` for OpenAI-style levels and `control: token_budget` for Anthropic-style thinking budgets. If an upstream rejects `max_tokens`, temperature, top-p, or requires thinking budget to be lower than the output cap, record that as scalar `reasoning` metadata and run a router-level smoke that proves the translation or filter.

If validation fails, remove `reasoning` metadata from the provider model or target override. If the target is active and unsafe for explicit reasoning traffic, remove it from active `models.<group>.targets[]` or keep it catalog-only until validation passes.

### Existing Weighted Group Rollout Checklist

When enabling reasoning routing in an existing weighted group, keep ordinary traffic and explicit reasoning traffic distinct:

- inventory the requested group and identify which active targets should continue to serve ordinary traffic;
- direct-smoke each intended reasoning target with the exact provider, model ID, dialect, API skin, and control field before adding metadata;
- run router-level smokes for OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, and Anthropic Messages `thinking` for every skin the group exposes;
- use realistic acceptance budgets, because reasoning-heavy models can consume tiny output caps and return empty final content;
- run low-cap smokes with `max_tokens: 1`, `max_completion_tokens: 1`, or `max_output_tokens: 1` to prove cap forwarding, skip behavior, or configured translation;
- add `reasoning` metadata only to validated provider models or target overrides, leaving non-reasoning targets in the weighted mix for ordinary requests;
- verify `/v1/models` with an allowed caller and confirm safe reasoning metadata such as `supported_reasoning_levels` and `supports_reasoning_summaries` appears only where intended;
- run a negative request against a test group with no compatible reasoning target and expect `502 no-eligible-target` with no upstream attempt;
- query usage, attempts, traces, and reports after success and failure cases for selected provider/model, status, latency, TTFB, duration, throughput, token counts, cost fields, fallback state, and safe reasoning metadata;
- document rollback: remove unsafe reasoning metadata, relax a reasoning-only contract or dynamic-score hard filter, restore the previous group config backup, and rerun the failing smoke.

## Structured-Output Smokes

Structured-output requests are dialect-specific. Declare `structured_outputs` only for the exact provider/model/dialect/skin that passes the relevant smoke. A target that only accepts the request field syntactically is not validated until it returns schema-shaped content, reports normal usage when the upstream normally does, and fails or rejects unsupported strict schemas in an understandable way.

| Client/API | Smoke |
|---|---|
| OpenAI Chat structured outputs | Direct upstream `/chat/completions` with `response_format.type: json_schema`, then the same request through the router group |
| OpenAI Responses structured outputs | Direct upstream `/responses` with `text.format.type: json_schema`, then the same request through the router group |
| Negative eligibility | Router request against a group with no compatible target; expect `502 no-eligible-target` and no upstream attempt |
| Tools plus structured outputs | Combined request when the target claims both capabilities for the same dialect |
| Streaming structured outputs | Verify caller-visible behavior for clients that request streaming; note whether downstream SSE is provider-native or router-synthesized |

The router forwards schema payloads to the selected upstream. It does not validate arbitrary JSON Schema subsets or repair model output. Unsupported schemas may produce upstream/provider errors even when eligibility metadata is correct.

Add metadata only after validation:

```yaml
providers:
  example:
    models:
      example-model:
        model: provider-model-id
        tool_support:
          openai_chat: [structured_outputs]
          openai_responses: [structured_outputs]
```

If a target supports both tools and structured outputs on one surface, keep both capabilities on that same surface, for example `openai_chat: [tools, tool_choice, structured_outputs]`. A tool-bearing structured-output request must not route to a tools-only target or a structured-only target.

### Direct Upstream Checks

Run these checks directly against the upstream endpoint before routing traffic through the router. Use placeholder-safe prompts and do not print provider keys.

OpenAI Chat `response_format.type: json_schema`:

```bash
curl -fsS "$UPSTREAM_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $UPSTREAM_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<provider-model-id>",
    "messages": [{"role": "user", "content": "Extract INC-1234 high"}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "ticket_extract",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "ticket_id": {"type": "string"},
            "priority": {"type": "string", "enum": ["low", "medium", "high"]}
          },
          "required": ["ticket_id", "priority"],
          "additionalProperties": false
        }
      }
    },
    "max_tokens": 128,
    "stream": false
  }'
```

OpenAI Responses `text.format.type: json_schema`:

```bash
curl -fsS "$UPSTREAM_BASE_URL/responses" \
  -H "Authorization: Bearer $UPSTREAM_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<provider-model-id>",
    "input": "Extract INC-1234 high",
    "text": {
      "format": {
        "type": "json_schema",
        "name": "ticket_extract",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "ticket_id": {"type": "string"},
            "priority": {"type": "string", "enum": ["low", "medium", "high"]}
          },
          "required": ["ticket_id", "priority"],
          "additionalProperties": false
        }
      }
    },
    "max_output_tokens": 128,
    "stream": false
  }'
```

When declaring strict tool/function argument support, also run a tool-call request with a strict parameter schema on the same surface. Record only safe evidence: provider, model ID, API surface, HTTP status, finish reason or response status, whether parsed content matched the schema, usage token counts, and whether intentionally unsupported schemas failed clearly.

### Router-Level Checks

After direct upstream checks pass, add the capability metadata to the target and run router-level smokes through a deployment-defined test group before activating or increasing the target in broad groups.

For OpenAI-compatible providers, validate encoding metadata explicitly. Set `force_store_false: true` only after the upstream accepts `store:false`; leave it unset for providers that reject `store`. Set `output_token_field: max_completion_tokens` only after a Chat Completions smoke proves the target requires `max_completion_tokens` instead of `max_tokens`. If a tool route also needs a target-level dialect override, keep that routing/config change separate from the metadata unless the rollout scope includes it.

OpenAI Chat through the router:

```bash
curl -fsS "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<structured-chat-test-group>",
    "messages": [{"role": "user", "content": "Extract INC-1234 high"}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "ticket_extract",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "ticket_id": {"type": "string"},
            "priority": {"type": "string", "enum": ["low", "medium", "high"]}
          },
          "required": ["ticket_id", "priority"],
          "additionalProperties": false
        }
      }
    },
    "max_tokens": 128,
    "stream": false
  }'
```

OpenAI Responses through the router:

```bash
curl -fsS "$ROUTER_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "<structured-responses-test-group>",
    "input": "Extract INC-1234 high",
    "text": {
      "format": {
        "type": "json_schema",
        "name": "ticket_extract",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "ticket_id": {"type": "string"},
            "priority": {"type": "string", "enum": ["low", "medium", "high"]}
          },
          "required": ["ticket_id", "priority"],
          "additionalProperties": false
        }
      }
    },
    "max_output_tokens": 128,
    "stream": false
  }'
```

Negative and mixed-capability router checks:

- Send the same structured-output request to a test group with no `structured_outputs` target and expect `502 no-eligible-target` with `structured_outputs` in `details.requirements`.
- For tool plus structured-output requests, verify this matrix:

| Target capabilities | Expected eligibility |
|---|---|
| tools only | skipped |
| structured outputs only | skipped for tool-bearing request |
| tools + structured outputs | eligible |
| neither | skipped |

Structured-output requests bypass the response cache because schema fields are part of the contract and should not be coalesced with ordinary text requests or other schemas. Confirm repeated structured-output smokes reach the upstream each time and that request logs record `cache=bypass`.

### Rollback

If validation fails, remove `structured_outputs` or `json_schema` from the affected target metadata. If the target is already active, remove it from the affected model groups, restart/reload using the normal deployment process, and rerun the negative router smoke to confirm structured-output traffic no longer reaches that upstream. Do not leave a target in active routing with stale structured-output metadata.

## Image Smokes

Use the receipt image when validating generic VLM/OCR transport:

```text
https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png
```

Check separately:

- transport accepted the image;
- output is nonempty;
- upstream reports image/token usage when available;
- OCR/analysis answer is correct when the route is intended for OCR accuracy.

## CLI Smokes

Claude Code should use router bearer token settings. Run tool-bearing CLI smokes inside a disposable container or equivalent sandbox with only the scratch workdir mounted and only the scoped router token in the environment:

```bash
mkdir -p "$PWD/claude-tool-smoke"
docker run --rm --network host --cap-drop ALL --security-opt no-new-privileges \
  --cpus 1 --memory 1g --pids-limit 256 --read-only \
  --tmpfs /tmp:rw,nosuid,nodev,size=256m \
  --mount type=bind,source="$PWD/claude-tool-smoke",target=/workspace \
  -e "ANTHROPIC_BASE_URL=$ROUTER_BASE_URL" \
  -e "ANTHROPIC_AUTH_TOKEN=$ROUTER_TOKEN" \
  -w /workspace "$TOOL_SMOKE_IMAGE" \
  claude -p "Create claude_tool_smoke.txt containing exactly claude-tool-ok, run cat claude_tool_smoke.txt, then finish with claude-tool-ok." \
    --model "<tool-smoke-model-group>" \
    --permission-mode bypassPermissions \
    --allowedTools "Write,Bash"
```

Codex CLI should use an OpenAI-compatible provider with `wire_api="responses"` and a router-issued token.

## Acceptance Rule

Do not activate a provider/model in broad routing until the relevant direct provider smokes, router smokes, docs, config, and production checks have all passed.
