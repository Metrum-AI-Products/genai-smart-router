---
title: Error Reference
---

# Error Reference

GenAI Smart Router returns structured errors intended to be useful to both callers and administrators. Every response includes `X-Request-Id`; include that ID when asking an administrator to inspect router traces.

## Common Errors

| Type | HTTP status | Meaning | Caller action | Admin action |
|---|---:|---|---|---|
| `missing-model` | 400 | The request omitted `model` and no `server.default_model_group` is configured. | Set a model group allowed for your token. | Configure `server.default_model_group` if omitted model should be accepted. |
| `model-not-allowed` | 403 | The caller token is not allowed to use the requested model group. | Use `/v1/models` to see allowed groups; see [Available Models And Access](../getting-started/available-models). | Update the caller `allow` list if access is intended. |
| `quota-exceeded` or `rate-limited` | 429 | Request, token, daily, monthly, or concurrency policy blocked the request. | Reduce traffic, lower an unrealistic output cap, or ask for a quota change. | Inspect caller limits, recent usage, and in-flight traffic. |
| `key-exhausted` | 403 | The caller's lifetime token budget is exhausted or the request's reserved token budget would exceed the remaining lifetime budget. | Use a key with remaining budget, lower an unrealistic output cap, or ask for a new budget. | Inspect the key lifetime budget and issue or re-enable keys according to policy. |
| `pii-filter-blocked` | 400 | The requested model group is configured to reject requests that match PII filter rules. | Remove the sensitive value or use an approved workflow. | Review the model group's `pii_filter` rules and mode. |
| `pii-filter-failed` | 502 | The router could not apply the configured PII filter. | Retry after the administrator resolves configuration. | Check regex validation, filter limits, and request shape. |
| `no-eligible-target` | 502 | No configured upstream target satisfies the request requirements. | Try a different allowed group only if instructed. | Add or enable a target that supports the requested dialect, tools, modalities, and cap behavior. |
| `upstream-error` | 502 | The selected upstream failed and no fallback succeeded. | Retry if the task is idempotent. | Inspect request attempts and provider status. |
| `upstream-timeout` | 504 | The upstream did not complete within configured timeout. | Retry with a smaller task or larger timeout if available. | Tune timeout, fallback, provider mix, or client token budget. |
| `metrics-forbidden` | 403 | `/metrics` was requested with a non-metrics-admin token. | Use `/v1/usage` for caller usage. | Issue a separate metrics-admin token only for operators. |
| `content-forbidden` | 403 | A content-capture maintenance endpoint was requested with a non-content-admin token. | Do not call content-capture admin endpoints from application clients. | Issue a separate `content_admin: true` token only for governed content maintenance. |
| `admin-forbidden` | 403 | A browser-admin route was requested by an authenticated Basic subject without the required route permission. | Ask the administrator to grant the appropriate admin policy or route permission. | Verify the subject and authorization policy before enabling broader admin surfaces. |

## Eligibility Requirements

The `no-eligible-target` response includes a `requirements` list. Examples:

- `text`: request needs text input support.
- `image`: request includes image input.
- `tools`: request includes tool definitions.
- `openai-chat_tool_passthrough`: OpenAI Chat tool payload must be preserved.
- `openai-responses_function`: Responses function tools are required.
- `anthropic-messages_client_tools`: Anthropic Messages client tools are required.

Resolution is usually a configuration update. The model group must contain at least one enabled target whose provider dialect and metadata satisfy those requirements.

## Max-Token Cap Errors

If a caller sets `max_tokens`, OpenAI Chat `max_completion_tokens`, or Responses `max_output_tokens`, the router skips targets known not to honor output caps when that metadata is configured. Keep a target cataloged but inactive for capped traffic by setting `honors_max_tokens: false` after a failed cap smoke. For OpenAI Chat requests that include both `max_tokens` and `max_completion_tokens`, `max_tokens` takes precedence.

Explicit output caps also affect quota admission. The router reserves estimated input tokens plus `max_tokens`, `max_completion_tokens`, or `max_output_tokens` before upstream calls, then reconciles the reservation to actual usage when the request finishes. Failed or canceled upstream calls release the reservation, and cache hits do not consume persisted token quota.

## Troubleshooting With Request IDs

Administrators can use `X-Request-Id` to inspect:

- `request_usage` for terminal status, selected target, token counts, cost, and cache behavior.
- `request_attempts` for each provider/model attempt.
- `request_trace_events` for routing, fallback, timeout, and cache decisions.
- `request_errors` for sanitized terminal error summaries.

Diagnostic rows exclude prompt text, raw image payloads, raw router tokens, token hashes, provider API keys, full upstream headers, and unsanitized upstream bodies.

If governed content capture is enabled by an operator, captured content lives in separate content-capture tables and remains outside usage reports and diagnostics. Delete and retention-purge maintenance endpoints require `content_admin: true`.
