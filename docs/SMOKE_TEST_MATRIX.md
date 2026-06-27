# Smoke Test Matrix

Run smokes at the narrowest layer that proves the change, then run production-level smokes for deployed behavior.

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
| Usage/cost fields | query usage DB/report after a request |
| Decision telemetry | with `server.decision_telemetry.enabled: true`, run success, no-eligible-target, policy fail-closed, policy fallback, upstream-fallback-success, and cache-bypass requests; query `request_policy_executions`, `request_fallback_transitions`, score/ranking rows, safe fingerprints, and `router-usage-report` summary buckets |

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

For Fireworks, public docs checked on 2026-06-27 list `https://api.fireworks.ai/inference/v1` as the OpenAI-compatible endpoint and Serverless pricing where GPT OSS 20B is $0.07/M input, $0.035/M cached input, and $0.30/M output. Use `FIREWORKS_API_KEY` only from a protected environment or ignored `env.json`; never print it. Direct validation on 2026-06-27 showed completions may require an explicit `User-Agent` from this environment. Configure one under provider `headers`. Fireworks `accounts/fireworks/models/gpt-oss-20b` passed direct text, streaming, `max_tokens: 1`, OpenAI Chat `reasoning_effort` low/medium/high, auto tools with `max_tokens >= 256`, forced `tool_choice`, and JSON schema structured-output smokes even though it was not listed by `/models` for the validated account. Other account-visible Fireworks candidates also passed direct OpenAI Chat smokes, but they are separate activation candidates and should not be added to production without an explicit weight and workload-validation decision.

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

Fireworks GPT OSS 20B returns `reasoning_content` alongside visible content. Declare `reasoning` metadata only after a router-level `reasoning_effort` smoke confirms the selected target preserves the caller request shape and usage/cost telemetry remains populated. Keep Fireworks out of OpenAI Responses, Anthropic Messages, and image/audio/video routes until those exact direct and router-level skins pass.

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

If validation fails, remove `structured_outputs` or `json_schema` from the affected target metadata. If the target is already active, remove it from the model group or lower its active weight to zero, restart/reload using the normal deployment process, and rerun the negative router smoke to confirm structured-output traffic no longer reaches that upstream. Do not leave a target in active routing with stale structured-output metadata.

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

Claude Code should use router bearer token settings:

```bash
env -u ANTHROPIC_API_KEY \
  ANTHROPIC_BASE_URL="$ROUTER_BASE_URL" \
  ANTHROPIC_AUTH_TOKEN="$ROUTER_TOKEN" \
  claude -p "Create claude_tool_smoke.txt containing exactly claude-tool-ok, run cat claude_tool_smoke.txt, then finish with claude-tool-ok." \
  --model "<tool-smoke-model-group>" \
  --permission-mode bypassPermissions \
  --allowedTools "Write,Bash"
```

Codex CLI should use an OpenAI-compatible provider with `wire_api="responses"` and a router-issued token.

## Acceptance Rule

Do not activate a provider/model in broad routing until the relevant direct provider smokes, router smokes, docs, config, and production checks have all passed.
