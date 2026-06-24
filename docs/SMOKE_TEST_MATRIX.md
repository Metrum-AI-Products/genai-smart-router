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
