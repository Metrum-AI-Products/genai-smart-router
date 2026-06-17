# Self-Hosted vLLM And SGLang Upstreams

Smart LLM Router can route to enterprise-owned inference services that expose OpenAI-compatible APIs, including vLLM and SGLang. Keep these services on private network names, expose the router as the governed ingress point, and use router model groups as the caller-facing contract.

Upstream references checked on 2026-06-17:

- vLLM online serving: https://docs.vllm.ai/en/latest/serving/online_serving/
- vLLM tool calling: https://docs.vllm.ai/en/latest/features/tool_calling/
- SGLang tool parser: https://docs.sglang.io/docs/advanced_features/tool_parser

## Configuration Pattern

Use `dialect: openai-chat` for vLLM or SGLang `/v1/chat/completions` services. Use `dialect: openai-responses` only after validating the upstream's `/v1/responses` behavior with the exact client and tool path.

If the internal service requires a bearer token, set `auth_scheme: bearer` and load `api_key` from the deployment environment. If access is enforced by network policy, mTLS, or a service mesh, omit `api_key`; the router only sends upstream authorization when a provider API key is configured.

```yaml
providers:
  vllm_qwen_tools:
    base_url: http://vllm-qwen-tools.inference.svc.cluster.local:8000/v1
    dialect: openai-chat
    auth_scheme: bearer
    api_key: ${VLLM_QWEN_API_KEY}
    api_key_env: VLLM_QWEN_API_KEY
    key_id: vllm-qwen-tools-prod
    models:
      qwen3-coder-tools:
        model: qwen3-coder-tools
        tier: coding
        input_price_per_million_usd: 0.00
        output_price_per_million_usd: 0.00
        pricing_notes: internal GPU allocation; set chargeback values if reports need allocated cost
        tool_support:
          openai_chat: [tools, tool_choice]

models:
  internal-coder:
    strategy: weighted
    targets:
      - { provider: vllm_qwen_tools, model_ref: qwen3-coder-tools, weight: 100 }
      - { provider: vllm_qwen_tools, model_ref: qwen3-coder-tools, weight: 100, tool_only: true }
```

The `model` value must match the upstream server's served model ID. For vLLM this is commonly controlled with `--served-model-name`; otherwise it defaults to the served model argument. Validate with:

```bash
curl -fsS "$UPSTREAM_BASE_URL/models" \
  -H "Authorization: Bearer $UPSTREAM_API_KEY"
```

For self-hosted models, set `input_price_per_million_usd` and `output_price_per_million_usd` to the enterprise chargeback rate if one exists. Use `0.00` only when reports should show token volume without allocated GPU cost. The router stores those prices and calculated costs on each usage row at request time.

## Tool-Calling Notes

The router forwards OpenAI-compatible chat `tools` and `tool_choice` fields to `openai-chat` targets. The model emits `tool_calls`; the client or agent runtime executes those tools and sends tool-result messages back. The router does not execute application tools.

vLLM tool calling depends on the model, chat template, and parser. For example:

```bash
vllm serve Qwen/Qwen3-Coder-30B-A3B-Instruct \
  --host 0.0.0.0 \
  --port 8000 \
  --served-model-name qwen3-coder-tools \
  --api-key "$VLLM_QWEN_API_KEY" \
  --enable-auto-tool-choice \
  --tool-call-parser qwen3_xml
```

SGLang tool calling similarly depends on the model parser. For example:

```bash
python3 -m sglang.launch_server \
  --model-path Qwen/Qwen2.5-7B-Instruct \
  --host 0.0.0.0 \
  --port 30000 \
  --tool-call-parser qwen25
```

Do not assume a model is tool-capable because the server accepts `tools`. Validate that the response contains correctly shaped tool calls in both non-streaming and streaming modes if clients use both. Add `tool_support.openai_chat: [tools, tool_choice]` only after that validation passes for the exact served model, chat template, parser, and client protocol.

## Acceptance Smokes

Direct upstream text smoke:

```bash
curl -fsS "$UPSTREAM_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $UPSTREAM_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-coder-tools",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 16,
    "stream": false
  }'
```

Direct upstream tool smoke:

```bash
curl -fsS "$UPSTREAM_BASE_URL/chat/completions" \
  -H "Authorization: Bearer $UPSTREAM_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-coder-tools",
    "messages": [{"role": "user", "content": "Call get_weather for San Francisco in fahrenheit."}],
    "tools": [{
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get weather for a city.",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {"type": "string"},
            "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]}
          },
          "required": ["location", "unit"]
        }
      }
    }],
    "tool_choice": "auto",
    "stream": false
  }'
```

Router text smoke:

```bash
curl -fsS "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "internal-coder",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 16,
    "stream": false
  }'
```

Before adding a self-hosted tool target to production:

- Directly validate `/v1/models`, text completions, and tool calls against the upstream.
- Validate the same text and tool requests through the router model group.
- Keep `tool_only: true` targets for upstreams that are validated only for tool-bearing traffic.
- Keep caller allow lists scoped to the enterprise model groups the caller actually needs.
- Record the upstream model ID, parser flags, chat template, server version, smoke results, and rollback path in deployment notes.
