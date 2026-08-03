# Smoke Commands Reference

Source-only internal reference. Do not add this file to `scripts/package_docs_allowlist.txt` or the public Docusaurus sidebar.

These templates use placeholders only. Set secrets in the environment and never paste provider API keys, router tokens, token hashes, raw prompts, raw images, raw tool outputs, or full production config into logs or docs.

Common placeholders:

- `UPSTREAM_BASE_URL`: provider endpoint without a trailing path beyond `/v1`, for example `https://provider.example.com/v1`
- `UPSTREAM_API_KEY`: provider API key from a protected environment or ignored `env.json`
- `ANTHROPIC_API_KEY`: native Anthropic API key from a protected environment or ignored `env.json`
- `ROUTER_BASE_URL`: router base URL, for example `http://127.0.0.1:8080` or `https://router.example.com`
- `ROUTER_TOKEN`: scoped router caller token
- `MODEL_ID`: direct upstream model ID
- `MODEL_GROUP`: router model group

## Deterministic Capability Contract

Run `make capability-smoke-unit` before relying on capability metadata. It is
an offline fake-adapter and fixture test only; it neither calls providers nor
discovers credentials. Synthetic fixtures under `testdata/capability-smokes/`
are non-promotable and must not be used as evidence for activation, routing
weights, or provider claims. `make capability-smoke-live` is intentionally a
fail-closed placeholder in this release.

## OpenAI Chat Text

Direct upstream:

```bash
curl -fsS "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 64,
    "stream": false
  }'
```

Router-level:

```bash
curl -fsS "${ROUTER_BASE_URL}/v1/chat/completions" \
  -H "Authorization: Bearer ${ROUTER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_GROUP}"'",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 64,
    "stream": false
  }'
```

## OpenAI Chat Streaming

```bash
curl -N "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 64,
    "stream": true
  }'
```

## OpenAI Chat Max-Token Cap

```bash
curl -fsS "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{"role": "user", "content": "Write five short sentences about routing."}],
    "max_tokens": 1,
    "stream": false
  }'
```

Use `max_completion_tokens` instead of `max_tokens` only for OpenAI Chat targets that require that field.

## OpenAI Chat Tool Call

```bash
curl -fsS "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{"role": "user", "content": "Call get_weather for San Francisco in fahrenheit."}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Return the weather for a city.",
        "parameters": {
          "type": "object",
          "properties": {
            "city": {"type": "string"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          },
          "required": ["city", "unit"],
          "additionalProperties": false
        }
      }
    }],
    "tool_choice": "auto",
    "max_tokens": 256,
    "stream": false
  }'
```

Forced tool-choice variant:

```bash
curl -fsS "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{"role": "user", "content": "Call get_weather for San Francisco in fahrenheit."}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Return the weather for a city.",
        "parameters": {
          "type": "object",
          "properties": {
            "city": {"type": "string"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          },
          "required": ["city", "unit"],
          "additionalProperties": false
        }
      }
    }],
    "tool_choice": {"type": "function", "function": {"name": "get_weather"}},
    "max_tokens": 256,
    "stream": false
  }'
```

## OpenAI Chat Structured Output

```bash
curl -fsS "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{"role": "user", "content": "Return JSON for status ok and count 1."}],
    "response_format": {
      "type": "json_schema",
      "json_schema": {
        "name": "smoke_result",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "status": {"type": "string"},
            "count": {"type": "integer"}
          },
          "required": ["status", "count"],
          "additionalProperties": false
        }
      }
    },
    "max_tokens": 128,
    "stream": false
  }'
```

## OpenAI Chat Image

```bash
curl -fsS "${UPSTREAM_BASE_URL}/chat/completions" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "Read the receipt image carefully. Reply with only the merchant/store chain name printed on the receipt."},
        {"type": "image_url", "image_url": {"url": "https://cdn.learnopencv.com/wp-content/uploads/2018/06/04100007/receipt.png"}}
      ]
    }],
    "max_tokens": 512,
    "stream": false
  }'
```

## OpenAI Responses Text

Direct upstream:

```bash
curl -fsS "${UPSTREAM_BASE_URL}/responses" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "input": "Reply OK only.",
    "max_output_tokens": 64,
    "store": false
  }'
```

Router-level:

```bash
curl -fsS "${ROUTER_BASE_URL}/v1/responses" \
  -H "Authorization: Bearer ${ROUTER_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_GROUP}"'",
    "input": "Reply OK only.",
    "max_output_tokens": 64
  }'
```

## OpenAI Responses Function Tool

```bash
curl -fsS "${UPSTREAM_BASE_URL}/responses" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "input": "Call get_weather for San Francisco in fahrenheit.",
    "tools": [{
      "type": "function",
      "name": "get_weather",
      "description": "Return the weather for a city.",
      "parameters": {
        "type": "object",
        "properties": {
          "city": {"type": "string"},
          "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
        },
        "required": ["city", "unit"],
        "additionalProperties": false
      }
    }],
    "tool_choice": "auto",
    "max_output_tokens": 256,
    "store": false
  }'
```

## OpenAI Responses Tool Result Continuation

Replace `PREVIOUS_RESPONSE_ID` with the `id` from the response that produced the function call, and replace `CALL_ID_FROM_TOOL_RESPONSE` with that function call's `call_id`. This preserves the Responses conversation context while submitting the tool result.

```bash
curl -fsS "${UPSTREAM_BASE_URL}/responses" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "previous_response_id": "PREVIOUS_RESPONSE_ID",
    "input": [{
      "type": "function_call_output",
      "call_id": "CALL_ID_FROM_TOOL_RESPONSE",
      "output": "{\"city\":\"San Francisco\",\"unit\":\"fahrenheit\",\"temperature\":65}"
    }],
    "max_output_tokens": 128,
    "store": false
  }'
```

## OpenAI Responses Structured Output

```bash
curl -fsS "${UPSTREAM_BASE_URL}/responses" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "input": "Return JSON for status ok and count 1.",
    "text": {
      "format": {
        "type": "json_schema",
        "name": "smoke_result",
        "strict": true,
        "schema": {
          "type": "object",
          "properties": {
            "status": {"type": "string"},
            "count": {"type": "integer"}
          },
          "required": ["status", "count"],
          "additionalProperties": false
        }
      }
    },
    "max_output_tokens": 128,
    "store": false
  }'
```

## OpenAI Responses Streaming

```bash
curl -N "${UPSTREAM_BASE_URL}/responses" \
  -H "Authorization: Bearer ${UPSTREAM_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "input": "Reply OK only.",
    "max_output_tokens": 64,
    "stream": true,
    "store": false
  }'
```

## Anthropic Messages Text

Direct upstream:

```bash
curl -fsS "${UPSTREAM_BASE_URL}/messages" \
  -H "x-api-key: ${ANTHROPIC_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "max_tokens": 64,
    "messages": [{"role": "user", "content": "Reply OK only."}]
  }'
```

For Anthropic-compatible aggregators that use bearer auth, replace `x-api-key: ${ANTHROPIC_API_KEY}` with `Authorization: Bearer ${UPSTREAM_API_KEY}` and keep the same request body unless that provider documents a different Messages variant.

Router-level:

```bash
curl -fsS "${ROUTER_BASE_URL}/anthropic/v1/messages" \
  -H "Authorization: Bearer ${ROUTER_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "'"${MODEL_GROUP}"'",
    "max_tokens": 64,
    "messages": [{"role": "user", "content": "Reply OK only."}]
  }'
```

## Anthropic Messages Tool Call

```bash
curl -fsS "${UPSTREAM_BASE_URL}/messages" \
  -H "x-api-key: ${ANTHROPIC_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Call get_weather for San Francisco in fahrenheit."}],
    "tools": [{
      "name": "get_weather",
      "description": "Return the weather for a city.",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {"type": "string"},
          "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
        },
        "required": ["city", "unit"],
        "additionalProperties": false
      }
    }],
    "tool_choice": {"type": "auto"}
  }'
```

Forced tool-choice variant:

```bash
curl -fsS "${UPSTREAM_BASE_URL}/messages" \
  -H "x-api-key: ${ANTHROPIC_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "max_tokens": 256,
    "messages": [{"role": "user", "content": "Call get_weather for San Francisco in fahrenheit."}],
    "tools": [{
      "name": "get_weather",
      "description": "Return the weather for a city.",
      "input_schema": {
        "type": "object",
        "properties": {
          "city": {"type": "string"},
          "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
        },
        "required": ["city", "unit"],
        "additionalProperties": false
      }
    }],
    "tool_choice": {"type": "tool", "name": "get_weather"}
  }'
```

## Anthropic Messages Thinking

```bash
curl -fsS "${UPSTREAM_BASE_URL}/messages" \
  -H "x-api-key: ${ANTHROPIC_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "max_tokens": 512,
    "thinking": {"type": "enabled", "budget_tokens": 256},
    "messages": [{"role": "user", "content": "Reply OK only after thinking briefly."}]
  }'
```

## Anthropic Messages Image

Use this with a locally prepared base64 test image. Do not paste customer or production images into docs or logs.

```bash
IMAGE_MEDIA_TYPE="image/png"
IMAGE_BASE64="$(base64 -w0 ./testdata/receipt.png)"

curl -fsS "${UPSTREAM_BASE_URL}/messages" \
  -H "x-api-key: ${ANTHROPIC_API_KEY}" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "'"${MODEL_ID}"'",
    "max_tokens": 512,
    "messages": [{
      "role": "user",
      "content": [
        {"type": "text", "text": "Read the receipt image carefully. Reply with only the merchant/store chain name printed on the receipt."},
        {"type": "image", "source": {"type": "base64", "media_type": "'"${IMAGE_MEDIA_TYPE}"'", "data": "'"${IMAGE_BASE64}"'"}}
      ]
    }]
  }'
```

## Router No-Eligible-Target Check

Send a request shape the smoke group should reject, such as a tool request to a text-only group or an image request to a text-only group. The router should return a safe `no-eligible-target` error with a request ID and without leaking prompts, images, tokens, token hashes, provider keys, or upstream bodies.
