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

`/metrics` is not a caller API. It is global operational telemetry and requires a caller token configured with `metrics_admin: true`.

## Model Names

The `model` field is a router model group, not necessarily a provider model ID. Model group names are deployment-defined. Names shown in examples are examples only.

If a compatible API request omits `model`, the router uses `server.default_model_group` when configured. If no default is configured, the router returns `400 missing-model`.

## Tool Calls

Tool requests only route to upstream targets that explicitly advertise support for the caller's API dialect and tool mode.

| Caller shape | Required target metadata |
|---|---|
| OpenAI Chat tools | `tool_support.openai_chat` |
| OpenAI Responses function tools | `tool_support.openai_responses` |
| Anthropic Messages client tools | `tool_support.anthropic_messages` |

Tool-bearing requests bypass response caching because tool results depend on external shell, filesystem, browser, or client tool state.

## Image Inputs

Image-bearing requests are accepted through Chat Completions, Responses, and Messages shapes. The router selects only targets with `image` in `input_modalities`.

Text-only and image-capable work do not need separate user workflows. A deployment can put text-capable and vision-capable upstreams behind the same model group, as long as each request is routed only to targets that satisfy its actual requirements.

## Router-Only Endpoints

Router-only endpoints are not part of OpenAI or Anthropic compatibility:

- `/readyz` and `/healthz` report service health and build metadata.
- `/version` returns version, commit, build timestamp, Go version, OS, and architecture.
- `/v1/usage` returns usage/quota information for the authenticated caller.
- `/metrics` returns Prometheus telemetry only for metrics-admin tokens.

Do not configure OpenAI or Anthropic SDKs to call router-only endpoints unless the SDK supports custom paths.

