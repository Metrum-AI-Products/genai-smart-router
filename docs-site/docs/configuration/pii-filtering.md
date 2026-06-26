---
title: PII Filtering
---

# PII Filtering

Model groups can redact configured text patterns before the router sends a request to an upstream model. This is useful when a deployment needs central privacy controls for LLM and agent traffic without changing every client.

PII filtering is deployment-defined. Group names, rules, and placeholder names below are examples.

## Model Group Configuration

```yaml
models:
  sensitive-workloads:
    strategy: weighted
    pii_filter:
      enabled: true
      mode: redact_and_restore
      restore_response: true
      max_replacements_per_request: 200
      apply_to:
        system: true
        messages: true
        responses_input: true
        tool_results: true
        image_urls: false
      rules:
        - name: email
          expression: '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}'
          placeholder_prefix: EMAIL
        - name: us_phone
          expression: '\b(?:\+1[-. ]?)?\(?[2-9]\d{2}\)?[-. ]?[2-9]\d{2}[-. ]?\d{4}\b'
          placeholder_prefix: PHONE
        - name: us_ssn
          expression: '\b\d{3}-\d{2}-\d{4}\b'
          placeholder_prefix: US_SSN
    targets:
      - { provider: private-vllm, model_ref: internal-sensitive-model, weight: 70 }
      - { provider: baseten, model_ref: gpt-oss-120b, weight: 30 }
```

With this configuration, text such as:

```text
Email jane.doe@example.com or call 415-555-0199.
```

is sent upstream as:

```text
Email [EMAIL_1] or call [PHONE_1].
```

When `mode: redact_and_restore` or `restore_response: true` is enabled, downstream text responses that contain `[EMAIL_1]` or `[PHONE_1]` are restored for the original caller. Placeholder mappings are kept in memory for the request lifecycle and are not persisted by default.

## Modes

- `redact_only`: send placeholders upstream and return placeholders downstream.
- `redact_and_restore`: send placeholders upstream and restore placeholders in downstream text responses.
- `fail_on_match`: reject the request before upstream target selection when any rule matches.

`fail_on_match: true` can also be used with a mode to force blocking behavior.

## Request Surfaces

PII filtering applies to normalized text across supported caller APIs:

- OpenAI Chat `messages[].content`
- OpenAI Responses `input`
- Anthropic Messages `messages[].content`
- text parts in multimodal messages
- tool-result text when `apply_to.tool_results: true`

The router preserves image parts, tool-call IDs, tool schemas, usage fields, request IDs, provider metadata, and model metadata. Image URLs are not filtered unless `apply_to.image_urls: true`.

TypeScript routing scripts receive the same redacted request object used for target selection and upstream calls, including `ctx.request.raw`. External routing policy services receive only safe derived context by default. If `external_policy.include_request: true` is explicitly enabled for a trusted policy service, the external payload includes redacted `request` and `text` fields. Placeholder mappings remain request-local and are not sent to policy code.

## API Examples

OpenAI Chat:

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sensitive-workloads",
    "messages": [
      { "role": "user", "content": "Email jane.doe@example.com or call 415-555-0199." }
    ],
    "max_tokens": 64,
    "stream": false
  }'
```

OpenAI Responses:

```bash
curl "$ROUTER_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sensitive-workloads",
    "input": "Email jane.doe@example.com before drafting the reply.",
    "max_output_tokens": 64,
    "stream": false
  }'
```

Anthropic Messages:

```bash
curl "$ROUTER_BASE_URL/v1/messages" \
  -H "x-api-key: $ROUTER_TOKEN" \
  -H "anthropic-version: 2023-06-01" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sensitive-workloads",
    "max_tokens": 64,
    "messages": [
      { "role": "user", "content": "Summarize account 123-45-6789 for jane.doe@example.com." }
    ]
  }'
```

For all three APIs, the upstream target receives placeholders such as `[EMAIL_1]` and `[US_SSN_1]`. With `redact_and_restore`, non-streaming downstream text responses are restored for the original caller. For streamed responses, restoration applies to router-generated text events after the upstream response is decoded; deployments that need raw upstream SSE pass-through should use `redact_only` or validate the exact streaming path before enabling restoration.

## Logging And Usage

Usage logs and the usage database record safe scalar metadata:

- `pii_filter_applied`
- `pii_filter_mode`
- `pii_filter_replacements`
- `pii_filter_rule_count`

They do not store raw matched values or placeholder mappings by default. Diagnostics and metrics must also avoid raw matched values.

## Operational Guidance

Regex filters are a practical gateway control, not a complete legal or compliance-grade PII detector. Use narrowly reviewed expressions, set `max_replacements_per_request`, and test each model group with representative prompts before rollout.

For production-grade detection, integrate an external DLP/privacy service through a deployment-owned policy service or a future managed detector. External routing policy services do not receive request text by default; keep any service with `external_policy.include_request: true` in trusted infrastructure because that opt-in can share request content after configured router redaction.

## Smoke Test

Use a test upstream or a private validation group and confirm the upstream receives placeholders, not raw values. This OpenAI Chat example should be repeated for any Responses, Anthropic Messages, tool-result, and image-bearing traffic the model group will accept:

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "sensitive-workloads",
    "messages": [
      { "role": "user", "content": "Email jane.doe@example.com or call 415-555-0199." }
    ],
    "max_tokens": 64,
    "stream": false
  }'
```

Check the upstream capture, router logs, and usage rows for the same `X-Request-Id`. Raw matched values should not appear upstream or in diagnostics.
