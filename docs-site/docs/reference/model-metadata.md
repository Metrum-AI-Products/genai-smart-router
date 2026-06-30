---
title: Model Metadata
doc_type: reference
---

# Model Metadata

Model metadata tells the router which upstream targets are eligible for a request and how to account for usage. Provider catalogs are metadata; traffic weights live only under `models.<group>.targets[]`.

## Provider Catalog Fields

```yaml
providers:
  provider_name:
    base_url: https://provider.example.com/v1
    dialect: openai-chat
    api_key_env: PROVIDER_API_KEY
    key_id: provider-primary
    models:
      model-ref:
        model: provider/model-id
        tier: general
        input_modalities: [text, image]
        output_modalities: [text]
        input_price_per_million_usd: 0.20
        output_price_per_million_usd: 1.00
        pricing_notes: Link source and update-date evidence in config.example.yaml.
        honors_max_tokens: true
        force_store_false: true
        output_token_field: max_completion_tokens
        tool_support:
          openai_chat: [tools, tool_choice, structured_outputs]
          openai_responses: [function, structured_outputs]
        bridges:
          chat_to_responses:
            enabled: true
            tools: true
            tool_choice: true
        reasoning:
          supported: true
          mode: opt_in
          control: effort_enum
```

## Modalities

`input_modalities` and `output_modalities` control routing eligibility.

| Modality | Use |
|---|---|
| `text` | Plain text input or output |
| `image` | Image input for VLM/OCR/browser-control workflows |
| `video` | Video input when the upstream/provider supports it |

Mark a modality active after validating the exact account, endpoint, model ID, request shape, and deployment region.

## Tool Support

Tool support is API-shape-specific. A model that handles OpenAI Chat tools may not handle Responses function tools or Anthropic Messages tools through the same provider endpoint. Structured-output support is tracked in the same metadata object because it is also a dialect-specific request-shape eligibility requirement.

Catalog metadata is broader than active routing eligibility. The router evaluates capabilities against the resolved provider or target dialect in the requested model group. A catalog entry may list `openai_responses` metadata for an upstream model, but an active target that uses an `openai-chat` provider skin is still only eligible for Chat requests unless a separate Responses skin or explicitly validated bridge target is configured. Admin catalog status separates these cases with `effectiveToolSupport` for the active skin and `inactiveToolSupport` for metadata that is not active for that target.

Declare only what has passed direct upstream and router-level smokes:

```yaml
tool_support:
  openai_chat: [tools, tool_choice, structured_outputs]
  openai_responses: [function, structured_outputs]
  anthropic_messages: [client_tools]
```

Capability labels:

| Label | Meaning |
|---|---|
| `tools` | OpenAI Chat tool payloads are accepted and produce correctly shaped tool calls |
| `tool_choice` | OpenAI Chat `tool_choice` modes used by clients are accepted |
| `function` | OpenAI Responses function tools are accepted and produce correctly shaped tool calls |
| `client_tools` | Anthropic Messages client tools are accepted and produce correctly shaped tool calls |
| `structured_outputs` | The matching OpenAI dialect accepts JSON Schema structured-output requests |
| `provider_hosted` | Reserved for provider-executed tools after exact upstream validation |

`openai_chat` and `openai_responses` are separate validation surfaces. Declare `structured_outputs` under `openai_chat` only after a direct upstream Chat Completions `response_format` smoke and a router-level Chat smoke pass for the exact provider, model ID, dialect, and skin. Declare it under `openai_responses` only after the same direct and router-level evidence exists for Responses `text.format`.

Tool support and structured-output support are independent. A target may support tools but not structured outputs, structured outputs but not tools, or both. A request containing both tools and structured-output fields needs a target that satisfies both requirements. Unsupported targets are skipped before routing policy selection; if no compatible target remains, callers receive `502 no-eligible-target` and no upstream request is sent.

Provider-hosted tool types such as OpenAI Responses `mcp` or `sse` execute server-side at the upstream provider. The router rejects remote provider-hosted entries such as `mcp`, `sse`, file-search, code-interpreter, and computer-use tools by default before upstream. Generic hosted search or image-generation descriptors from compatible clients are stripped unless the deployment explicitly implements and validates those services. Do not use the reserved `provider_hosted` metadata label for caller traffic unless the deployment has an explicit allowlist and security review for provider-executed tool URLs.

The router forwards schema payloads to the selected upstream. It does not validate arbitrary JSON Schema subsets, enforce provider-specific schema limits, or repair nonconforming model output unless a separate implementation adds that behavior. Unsupported schemas may therefore return upstream/provider errors even when the target is correctly marked as structured-output capable.

## Dialect Bridges

`bridges` declares validated cross-dialect compatibility for a provider catalog model or a model-group target. Bridge metadata is opt-in and target-specific. Same-dialect routing does not require it.

```yaml
bridges:
  chat_to_responses:
    enabled: true
    tools: true
    tool_choice: true
    parallel_tool_calls: false
    structured_outputs: false
    images: false
    reasoning: false
    streaming: false
```

`chat_to_responses.enabled: true` allows OpenAI Chat Completions callers to consider an `openai-responses` target after normal request-shape filtering. The current bridge is stateless: it translates the full Chat request into one Responses request and never creates or consumes `previous_response_id`.

Enable only the shapes that passed direct upstream and router-level bridge smokes:

| Field | Meaning |
|---|---|
| `enabled` | Non-streaming text bridge is eligible for this target. |
| `tools` | Chat function tools can translate to Responses function tools. The target must also declare `tool_support.openai_responses`. |
| `tool_choice` | Chat `tool_choice` modes used by callers can translate safely. |
| `parallel_tool_calls` | Caller `parallel_tool_calls` can be forwarded safely. |
| `structured_outputs` | Chat `response_format` can translate to Responses `text.format`; the target must also declare Responses structured-output support. |
| `images` | Chat image content blocks can translate to Responses image input and passed direct plus router image smokes. |
| `reasoning` | Chat reasoning controls can translate to Responses reasoning controls for this target. |
| `streaming` | Reserved for a future streaming bridge. Current bridge streaming is skipped before upstream even if this field is set. |

The generic `responses_to_chat` block is reserved so a future inverse bridge can coexist with `chat_to_responses`. Do not set it for caller traffic unless a separate implementation and validation path exists.

## Request-Shape Support

`request_shape_support` is optional metadata for known request-size, token-estimate, tool-schema, and dialect limits. It can be declared on a provider catalog model and overridden on a model-group target.

```yaml
request_shape_support:
  max_request_bytes: 300000
  max_estimated_input_tokens: 90000
  min_requested_output_tokens: 16
  max_requested_output_tokens: 8192
  max_tool_schema_bytes: 100000
  supports_large_coding_agent_payloads: false
  supported_inbound_dialects: [openai-chat]
  unsupported_request_features:
    - previous_response_id
    - function_call_output
  validation_status: limited
  validation_notes: Large coding-agent payload validation has not passed yet.
```

Known limits are enforced before the routing strategy runs. For example, if estimated input plus requested output cap exceeds `context_tokens`, the target is skipped with `request-shape-context-exceeded`; if a tool schema is too large, it is skipped with `request-shape-tool-schema-bytes`; if a caller-supplied output cap is below a provider's accepted minimum, it is skipped with `request-shape-min-output-tokens`. Weighted routing then recalculates over the remaining eligible targets. Unknown limits remain eligible by default and are recorded as `limit_unknown` in decision telemetry.

Set `supports_large_coding_agent_payloads: true` only after a direct upstream smoke and a router-level smoke pass for the exact provider, model ID, dialect, account, and request shape. The validation note should include the date, approximate request bytes, tool count, serialized tool-schema size, output cap, and prompt-token scale. If that evidence is missing, leave the value unset or set it to `false` with a reason and keep the target in a restricted smoke group.

Estimate and context-fit telemetry is diagnostic, not billed usage. The router stores scalar estimates, caps, request bytes, target context, headroom, fit booleans, and bounded reason labels in relational rows. It does not store raw prompts, raw tool schemas, tool outputs, images, router tokens, token hashes, provider keys, or full config.

## Responses-to-Chat Bridge

`responses_to_chat` is opt-in metadata for allowing `/v1/responses` callers to use a target whose upstream dialect is `openai-chat`. It can be declared on a provider catalog model and overridden on a model-group target.

```yaml
responses_to_chat:
  enabled: true
  text: true
  function_tools: true
  tool_choice: true
  validation_status: passed
  validation_notes: Router-level Responses text and function-tool bridge smokes passed for this target.
```

Enable only the flags that passed direct Chat and router-level Responses-to-Chat bridge smokes. The initial bridge supports stateless non-streaming text and basic function tools. Leave `streaming`, `images`, `reasoning`, and `structured_outputs` unset unless those exact bridge paths are implemented and validated for the target.

Unsupported Responses fields are skipped before selection for Chat-bridged targets with bounded filter reasons such as `responses-to-chat-previous-response-id`, `responses-to-chat-hosted-tools`, `responses-to-chat-image`, `responses-to-chat-reasoning`, and `responses-to-chat-structured-output`. Usage rows preserve both sides of the request: inbound `openai-responses`, target `openai-chat`, and translation `bridge_direction: responses_to_chat`.

## Responses Retention Controls

The router controls provider-side retention fields through target metadata. Same-dialect OpenAI Chat and Responses passthrough strips caller-supplied provider `metadata`; it sends `store:false` upstream only when the resolved target sets `force_store_false: true`. Translated OpenAI Responses calls use the same flag. Validate that text, tools, continuation shape, streaming behavior, and usage accounting still pass with `store:false` before setting the flag, because some OpenAI-compatible upstreams reject the `store` field.

```yaml
models:
  openai-nano:
    model: gpt-5.4-nano
    force_store_false: true
    output_token_field: max_completion_tokens
  crusoe-glm:
    model: zai/GLM-5.2
    # force_store_false omitted because this upstream rejects the store field.
```

## OpenAI Chat Encoding Controls

`output_token_field` controls which output-token cap field the router sends to OpenAI Chat-compatible upstreams after normalizing caller caps. Allowed values are `max_tokens` and `max_completion_tokens`; omitting the field defaults to `max_tokens`.

Use `output_token_field: max_completion_tokens` for models that reject Chat Completions `max_tokens`, including tool-bearing requests. This is independent of reasoning metadata: if reasoning compatibility also rewrites `max_tokens`, both rules converge on `max_completion_tokens` and the router avoids sending both cap fields.

## Reasoning And Thinking

Reasoning metadata is target eligibility metadata. It tells the router whether an upstream can safely receive caller reasoning controls such as OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, or Anthropic Messages `thinking`.

```yaml
reasoning:
  supported: true
  mode: opt_in            # opt_in or always_on
  control: effort_enum    # effort_enum or token_budget
  min_budget_tokens: 2048
  max_budget_tokens: 24576
  budget_must_be_less_than_max_tokens: true
  stream_block: thinking
  rejects_max_tokens: true
  supports_summaries: true
```

Declare reasoning support only after direct upstream and router-level smokes pass for the exact provider, model ID, dialect, and API skin. OpenAI Chat, OpenAI Responses, and Anthropic Messages reasoning controls are separate validation surfaces. For OpenAI Responses effort metadata, validate each advertised effort level with a useful response and document provider output-cap minimums such as rejected tiny `max_output_tokens` values.

When a caller explicitly requests reasoning, the router filters the requested model group's targets to reasoning-capable targets before routing policy selection. If none remain, the caller receives `502 no-eligible-target` with `reasoning` in the requirements. Ordinary non-reasoning requests are not forced to reasoning targets unless the deployment configured those targets as part of the group policy.

`control: effort_enum` targets receive low/medium/high effort controls. `control: token_budget` targets receive Anthropic-style token budgets when the target dialect supports them. Cross-dialect translation uses conservative defaults and records safe scalar decision telemetry; it does not persist prompts, tool schemas, router tokens, provider keys, or full config.

For end-to-end configuration, validation, and caller examples, see [Reasoning Routing](../configuration/reasoning-routing).

## Pricing And Cost Fields

Set pricing metadata for every active target when a price or internal chargeback rate is known:

- `input_price_per_million_usd`
- `output_price_per_million_usd`
- `image_input_price_per_million_tokens_usd`
- `image_input_price_per_image_usd`
- `pricing_source`
- `pricing_updated_at`
- `pricing_notes`

The router stores the prices and calculated costs used at request time. Historical reports therefore keep the cost assumptions that were true when the request ran, even if provider pricing changes later.

For self-hosted models, use the enterprise chargeback rate. Use `0.00` only when reports should show token volume without allocated GPU cost.

## Active, Disabled, And Catalog-Only

Cataloging a model does not send traffic to it. A model becomes active only when referenced under a model group's `targets`.

Keep a model catalog-only when:

- the current key is not entitled;
- direct provider smoke failed;
- tool or image support is unvalidated;
- cap behavior is unsafe;
- the model exists and is best kept out of production traffic until rollout criteria are met.

## Group Targets

```yaml
models:
  example-general:
    strategy: weighted
    targets:
      - provider: baseten
        model_ref: gpt-oss-120b
        weight: 60
      - provider: internal_vllm
        model_ref: llama-70b
        weight: 40
```

Model group names are deployment-defined. Use names that match the organization's policy and caller contracts.
