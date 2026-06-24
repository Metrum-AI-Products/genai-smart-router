# Smoke Test Matrix

Run smokes at the narrowest layer that proves the change, then run production-level smokes for deployed behavior.

## Core API Smokes

| Change type | Required smoke |
|---|---|
| Config validation | YAML parse and `docker compose config` |
| Health/deploy | `/readyz`, `/version`, router logs |
| Auth/allow list | `/v1/models` with caller token |
| Omitted model behavior | request without `model`, expect configured default or `400 missing-model` |
| Text routing | relevant dialect with realistic token budget |
| Max-token cap | request with `max_tokens: 1`, OpenAI Chat `max_completion_tokens: 1`, or Responses `max_output_tokens: 1` |
| Usage/cost fields | query usage DB/report after a request |

## Tool Smokes

| Client/API | Smoke |
|---|---|
| OpenAI Chat tools | `/v1/chat/completions` with `tools`, `tool_choice`, and streaming when supported |
| OpenAI Responses tools | Codex CLI or direct `/v1/responses` function tool request |
| Anthropic Messages tools | Claude Code or direct `/v1/messages` client tool request |
| OpenRouter-specific tool route | validate the OpenRouter skin used by the target |

For agent CLI smokes, the agent must create a file and the test must assert the file contents.

## Structured-Output Smokes

Structured-output requests are dialect-specific. Declare `structured_outputs` only for the exact provider/model/dialect/skin that passes the relevant smoke.

| Client/API | Smoke |
|---|---|
| OpenAI Chat structured outputs | Direct upstream `/chat/completions` with `response_format.type: json_schema`, then the same request through the router group |
| OpenAI Responses structured outputs | Direct upstream `/responses` with `text.format.type: json_schema`, then the same request through the router group |
| Negative eligibility | Router request against a group with no compatible target; expect `502 no-eligible-target` and no upstream attempt |
| Tools plus structured outputs | Combined request when the target claims both capabilities for the same dialect |
| Streaming structured outputs | Verify caller-visible behavior for clients that request streaming; note whether downstream SSE is provider-native or router-synthesized |

The router forwards schema payloads to the selected upstream. It does not validate arbitrary JSON Schema subsets or repair model output. Unsupported schemas may produce upstream/provider errors even when eligibility metadata is correct.

Rollback for failed structured-output validation: remove `structured_outputs` from the provider model or target override. If the target is unsafe beyond that capability, remove it from active `models.<group>.targets[]` and keep it catalog-only until direct upstream and router-level smokes pass again.

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
