---
title: API Compatibility
---

# API Compatibility

GenAI Smart Router exposes OpenAI-compatible and Anthropic-compatible HTTP surfaces so clients can keep familiar SDKs while routing, provider credentials, policy, quotas, and accounting stay server-side.

The router endpoint is deployment-specific. Use the base URL and model groups issued by your administrator or Metrum-managed instance.

## Supported Surfaces

| Endpoint | Compatibility target | Typical clients |
|---|---|---|
| `/v1/chat/completions` | OpenAI Chat Completions-style requests | OpenAI SDK chat clients, Warp-style OpenAI-compatible agents |
| `/v1/responses` | OpenAI Responses-style requests | Codex CLI, Responses-compatible agent frameworks |
| `/v1/messages` | Anthropic Messages-style requests | Claude Code CLI, Anthropic-compatible clients |
| `/v1/models` | OpenAI-style model discovery | Client setup and allow-list discovery |
| `/v1/usage` | Router usage lookup | Caller quota and usage checks |
| `/readyz`, `/healthz`, `/version` | Router operational endpoints | Load balancers and operators |
| `/admin/auth/check` | Browser-admin Basic Auth validation stub | Operators enabling browser-admin surfaces |
| `/admin/auth/login`, `/admin/auth/callback`, `/admin/auth/me`, `/admin/auth/logout` | Browser-admin OIDC session routes | Operators enabling OIDC browser admin surfaces |
| `/admin/reports/*` | Router-specific admin reports | Authorized administrators |

`/metrics` is an operator telemetry API. It requires a caller token whose subject is authorized for `metrics` `read`; existing `metrics_admin: true` caller entries receive compatible Casbin grants at startup.

Caller tokens are checked by SHA-256 hash. Unknown or missing tokens return `401 unauthorized`. Configured inactive keys return safe status-specific `403` errors after token match, including `key-disabled`, `key-suspended`, `key-expired`, and `key-rotated`. Config validation requires every enabled key to reference an active `owner_user`, active project, and active project membership, so inactive users/projects/memberships are caught before startup.

Content-capture maintenance endpoints are administrative APIs, not model APIs. `DELETE /v1/content-captures/<request_id>` requires `content:capture` `delete`; `POST /v1/content-captures/purge-expired` requires `content:capture` `purge`. Existing `content_admin: true` caller entries receive compatible Casbin grants. These endpoints never return captured content.

`/admin/auth/check` is not a model API. It is available only when `server.admin_auth.basic.enabled: true`; missing or invalid HTTP Basic credentials return `401`, valid credentials without the route permission return `403 admin-forbidden`, and valid credentials with `admin:auth:read` return safe subject metadata.

OIDC admin auth routes are not model APIs. They are available only when `server.admin_auth.oidc.enabled: true`. Login redirects to the IdP, callback creates a server-side session after OIDC verification, `/admin/auth/me` returns safe subject metadata, and logout invalidates the session.

`/admin/reports/*` is not a model API. It is disabled unless `server.admin_reports.enabled: true`, uses Basic Auth or OIDC sessions for browser-admin identity, and uses Casbin policy decisions for read/export access. Ordinary caller tokens receive `403 reports-forbidden`.

## Compatibility Matrix

| Capability | Chat Completions | Responses | Messages |
|---|---|---|---|
| Text input/output | Supported | Supported | Supported |
| Streaming | Supported when the selected target supports the provider path | Supported when the selected target supports the provider path | Supported when the selected target supports the provider path |
| Tool calls | Requires `tool_support.openai_chat` | Requires `tool_support.openai_responses` | Requires `tool_support.anthropic_messages` |
| Structured outputs | `response_format` requires `tool_support.openai_chat: [structured_outputs]` | `text.format` requires `tool_support.openai_responses: [structured_outputs]` | No OpenAI structured-output equivalent |
| Reasoning/thinking | `reasoning_effort` requires target `reasoning` metadata | `reasoning` requires target `reasoning` metadata | `thinking` requires target `reasoning` metadata or validated target default thinking |
| Image input | Requires `image` in target `input_modalities` | Requires `image` in target `input_modalities` | Requires `image` in target `input_modalities` |
| Caller max-token caps | `max_tokens` and `max_completion_tokens` are enforced against configured target metadata | `max_output_tokens` is enforced against configured target metadata | `max_tokens` is enforced against configured target metadata |
| Cache eligibility | Eligible only for deterministic non-tool, non-image requests | Eligible only for deterministic non-tool, non-image requests | Eligible only for deterministic non-tool, non-image requests |
| Usage and cost rows | Recorded | Recorded | Recorded |

If a request includes tools, structured-output fields, images, or an explicit max-token cap, the router filters the model group's target list before policy selection. Targets that do not satisfy the request shape are skipped. If no compatible target remains, the router returns `502 no-eligible-target` before sending an upstream request.

For OpenAI Chat Completions requests, both `max_tokens` and `max_completion_tokens` are treated as explicit output caps. If a Chat request sends both fields, `max_tokens` takes precedence for router eligibility and normalized upstream forwarding.

## Quotas And Output Caps

Before an upstream call, the router reserves the estimated input tokens plus the requested output budget for token-based admission. Chat Completions requests use `max_tokens` or `max_completion_tokens`, Responses requests use `max_output_tokens`, and Messages requests use `max_tokens`. Messages requests without a caller cap reserve the router default output cap when the router injects one.

TPM, daily token, monthly token, and lifetime key budgets include in-flight reservations. This prevents several concurrent large-cap requests from collectively exceeding a caller's budget. When a request completes, the reservation is reconciled to the actual usage reported by the upstream. Failed or canceled upstream requests release the reservation, and cache hits do not consume persisted token quota.

Use realistic output caps in examples and clients. A small prompt with a very large output cap can be rejected near a token budget because the caller asked the router to reserve that much possible output.

## Model Names

The `model` field is a router model group, not necessarily a provider model ID. Model group names are deployment-defined. Names shown in examples are examples only.

If a compatible API request omits `model`, the router uses `server.default_model_group` when configured. If no default is configured, the router returns `400 missing-model`.

## Discover Allowed Model Groups

Call `/v1/models` with the same router token that the client will use for completions. The response is filtered to that token's allow list, so it shows the deployment-defined model groups the caller can request.

```bash
curl "$ROUTER_BASE_URL/v1/models" \
  -H "Authorization: Bearer $ROUTER_TOKEN"
```

Example response:

```json
{
  "object": "list",
  "data": [
    {
      "id": "default",
      "object": "model",
      "owned_by": "smart-llmrouter"
    },
    {
      "id": "vision",
      "object": "model",
      "owned_by": "smart-llmrouter"
    }
  ]
}
```

Use one of the returned `id` values as the `model` field in `/v1/chat/completions`, `/v1/responses`, or `/v1/messages`. If a group is not listed, that token is not allowed to use it. Requests for unlisted groups fail with `403 model-not-allowed` before any upstream provider is called.

The returned IDs are router model groups, not a full inventory of every upstream provider model. Platform teams can change the upstream provider/model mix behind a group without changing the caller-facing group name.

For caller-facing troubleshooting and administrator handoff guidance, see [Available Models And Access](../getting-started/available-models).

## Tool Calls

Tool requests only route to upstream targets that explicitly advertise support for the caller's API dialect and tool mode.

| Caller shape | Required target metadata |
|---|---|
| OpenAI Chat tools | `tool_support.openai_chat` |
| OpenAI Responses function tools | `tool_support.openai_responses` |
| Anthropic Messages client tools | `tool_support.anthropic_messages` |

Tool-bearing requests bypass response caching because tool results depend on external shell, filesystem, browser, or client tool state.

## Reasoning And Thinking

The router detects explicit reasoning requests in all supported caller dialects:

- OpenAI Chat Completions: `reasoning_effort`.
- OpenAI Responses: `reasoning`.
- Anthropic Messages: `thinking`.

Reasoning is handled inside the requested model group. The router does not switch callers to another group and does not silently drop explicit reasoning controls. If no configured target in that group can satisfy the requested reasoning shape together with tools, images, structured outputs, and max-token cap behavior, the response is `502 no-eligible-target`.

For compatible targets, the router translates safe controls where configured. For example, an Anthropic budget can map to an OpenAI effort level, and an OpenAI effort can map to an Anthropic token budget. Targets that reject `max_tokens` for reasoning traffic can be configured so the router sends `max_completion_tokens` instead.

## Structured Outputs

Structured-output requests are routing contracts, not router-side schema execution. The router detects OpenAI Chat `response_format` and OpenAI Responses `text.format`, selects only targets with explicit dialect-matching `structured_outputs` metadata, and forwards the schema payload to the selected upstream. It does not validate arbitrary JSON Schema subsets or repair provider output unless a separate implementation adds that behavior. Unsupported schemas, strictness settings, or provider-specific JSON Schema subsets may produce upstream/provider errors.

Structured-output support is dialect-specific. Passing Chat Completions `response_format` does not prove Responses `text.format`, and Anthropic Messages has no OpenAI structured-output equivalent unless a deployment adds and documents an explicit compatible behavior.

Chat Completions JSON Schema example:

```bash
curl "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "example-structured",
    "messages": [{"role": "user", "content": "Extract the ticket id and priority from: INC-1234 high"}],
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
    }
  }'
```

Responses JSON Schema example:

```bash
curl "$ROUTER_BASE_URL/v1/responses" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "example-structured",
    "input": "Extract the ticket id and priority from: INC-1234 high",
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
    }
  }'
```

Use a deployment-defined model group returned by `/v1/models`; `example-structured` is only a placeholder group name.

## Image Inputs

Image-bearing requests are accepted through Chat Completions, Responses, and Messages shapes. The router selects only targets with `image` in `input_modalities`.

Text-only and image-capable work do not need separate user workflows. A deployment can put text-capable and vision-capable upstreams behind the same model group, as long as each request is routed only to targets that satisfy its actual requirements.

## Router-Only Endpoints

Router-only endpoints are not part of OpenAI or Anthropic compatibility:

- `/readyz` and `/healthz` report service health and build metadata.
- `/version` returns the running binary version, build timestamp, Go runtime version, OS, and architecture.
- `/v1/usage` returns usage/quota information for the authenticated caller.
- `/metrics` returns Prometheus telemetry only for metrics-admin tokens.

Use SDKs for the compatible provider-style APIs they support. Router-only endpoints such as `/readyz`, `/version`, and `/v1/usage` are best called with ordinary HTTP clients.
