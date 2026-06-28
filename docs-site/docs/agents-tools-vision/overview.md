---
title: Agents, Tools, And Vision
---

# Agents, Tools, And Vision

GenAI Smart Router keeps agent clients on stable caller-facing model groups while filtering each request to upstream targets that have passed the exact API-skin and capability checks needed for that turn.

This matters because ordinary text, function tools, client tools, image input, structured outputs, and reasoning controls are separate compatibility surfaces. A model that works for one surface is not automatically safe for another.

## Workload Surfaces

| Workload | Caller surface | Required target validation |
|---|---|---|
| OpenAI-compatible chat tools | `/v1/chat/completions` with `tools` and `tool_choice` | `tool_support.openai_chat` for the exact provider/model/dialect. |
| Codex and Responses tools | `/v1/responses` with function tools | `tool_support.openai_responses` for the exact Responses skin. |
| Claude Code client tools | `/v1/messages` with Anthropic-style tools | `tool_support.anthropic_messages` for the exact Messages skin. |
| Image and VLM requests | OpenAI Chat, Responses, or Messages image payloads | Validated `image` input modality on the exact active target and API shape. |
| Structured outputs | Chat `response_format` or Responses `text.format` | `structured_outputs` metadata for the same dialect and target. |

## Routing Behavior

Caller-visible model groups can contain multiple target types. The router filters targets before selection so a text-only target can serve ordinary text while tool-bearing or image-bearing requests use targets validated for those capabilities.

That lets a coding-agent user keep one allowed model group for a mixed task instead of manually switching between a language route and a vision route. If no eligible target remains after filtering, the caller receives a router error before an unsafe upstream call is attempted.

## What To Validate

Before adding or increasing an agent-capable target:

- Run a direct upstream smoke for the exact provider model, API path, tool schema, tool-choice mode, image payload, or structured-output schema.
- Run the same request through the router model group.
- Confirm the selected upstream target, finish reason, usage fields, fallback behavior, and caller-visible response shape.
- Use realistic output budgets for reasoning or VLM models; tiny budgets are useful only for cap-enforcement checks.
- Record any limitation in provider metadata instead of broadening active routing.

## Related Pages

- [Codex CLI](../getting-started/codex-cli)
- [Claude Code CLI](../getting-started/claude-code-cli)
- [Image Analysis And VLM Routing](../configuration/image-analysis-vlm)
- [Structured Outputs](./structured-outputs)
- [API Compatibility](../reference/api-compatibility)
- [Harbor Agentic Coding Case Study](../evaluation/harbor-case-study)
