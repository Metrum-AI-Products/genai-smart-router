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
| `key-disabled` | 403 | The matched caller key is configured but disabled. | Use an active key or ask for the key to be re-enabled. | Re-enable only if the key should still be trusted and assigned to active ownership records. |
| `key-suspended` | 403 | The matched caller key is temporarily suspended. | Use another active key or wait for an administrator to restore access. | Review the suspension reason and reactivate only after the hold is cleared. |
| `key-expired` | 403 | The matched caller key is past its configured expiration time. | Rotate to a current key. | Issue a replacement key and retire the expired one according to rotation policy. |
| `key-rotated` | 403 | The matched caller key has been replaced by a newer key. | Switch the client to the replacement key issued by the administrator. | Confirm clients have migrated, then retire or delete the rotated key when appropriate. |
| `quota-exceeded` or `rate-limited` | 429 | Request, token, daily, monthly, or concurrency policy blocked the request. | Reduce traffic, lower an unrealistic output cap, or ask for a quota change. | Inspect caller limits, recent usage, and in-flight traffic. |
| `key-exhausted` | 403 | The caller's lifetime token budget is exhausted or the request's reserved token budget would exceed the remaining lifetime budget. | Use a key with remaining budget, lower an unrealistic output cap, or ask for a new budget. | Inspect the key lifetime budget and issue or re-enable keys according to policy. |
| `pii-filter-blocked` | 400 | The requested model group is configured to reject requests that match PII filter rules. | Remove the sensitive value or use an approved workflow. | Review the model group's `pii_filter` rules and mode. |
| `pii-filter-failed` | 502 | The router could not apply the configured PII filter. | Retry after the administrator resolves configuration. | Check regex validation, filter limits, and request shape. |
| `no-eligible-target` | 502 | No configured upstream target satisfies the request requirements. | Try a different allowed group only if instructed. | Add or enable a target that supports the requested dialect, tools, modalities, and cap behavior. |
| `upstream-rate-limited` | 503 | All eligible upstream attempts were rejected by provider-side rate limits. | Retry later with backoff, or contact the administrator with the request ID if it persists. | Inspect `request_attempts`, provider status, and upstream rate-limit policy. |
| `upstream-quota-exhausted` | 503 | All eligible upstream attempts failed because a provider reported exhausted balance, credits, quota, billing, or payment state. | Retry later only after the provider account is funded or quota is restored; include the request ID when escalating. | Inspect `request_attempts` for `upstream_quota_exhausted`, then verify provider account balance, billing, quota, and entitlement state. |
| `upstream-failed` | 502 | All eligible upstream attempts failed for another upstream error class. | Retry if the task is idempotent. | Inspect request attempts, fallback behavior, and provider status. |
| `upstream-timeout` | 504 | The upstream did not complete within configured timeout. | Retry with a smaller task or larger timeout if available. | Tune timeout, fallback, provider mix, or client token budget. |
| `metrics-forbidden` | 403 | `/metrics` was requested with a caller token that is not authorized for metrics. | Use `/v1/usage` for caller usage. | Grant metrics access through Casbin policy or an existing `metrics_admin: true` operator caller. |
| `reports-forbidden` | 403 | `/admin/reports/*` was requested without an authorized admin subject or without `admin:security_reports` for security access reports. | Do not call admin report endpoints from application clients. | Grant Casbin `admin:reports` and, when needed, `admin:security_reports` read/export policy only to approved admin subjects. |
| `security-reports-disabled` | 503 | A security access report API was requested while `server.admin_reports.security.enabled` is false. | Do not call security report APIs on deployments where security reports are disabled. | Enable security reports only after configuring report authorization and trusted proxy IP handling. |
| `reports-disabled` | 503 | Admin reports are unavailable because reporting is disabled or the usage DB is unavailable. | Retry only after an administrator enables reports. | Check `server.admin_reports` and `server.usage_db` configuration. |
| `invalid-report-filter` | 400 | An admin report filter, time range, or row limit is invalid. | Use a bounded time range and valid query parameters. | Check `default_since`, `max_range`, `max_rows`, and request query parameters. |
| `report-query-failed` | 500 | The report query failed. | Retry later or ask an administrator to inspect the request ID. | Inspect usage DB health and router logs. |
| `content-forbidden` | 403 | A content-capture maintenance endpoint was requested without `content:capture` authorization. | Do not call content-capture admin endpoints from application clients. | Grant Casbin `content:capture` `delete`/`purge` policy or use a compatible `content_admin: true` operator caller. |
| `admin-forbidden` | 403 | A browser-admin route was requested by an authenticated Basic subject without the required route permission. | Ask the administrator to grant the appropriate admin policy or route permission. | Verify the subject and authorization policy before enabling broader admin surfaces. |
| `license-missing` | 503 | License enforcement is enabled but no readable license file is available. | Contact the router operator with the request ID. | Mount the issued license file at `server.license.path` and verify permissions. |
| `license-invalid` | 503 | The license file is malformed, unverifiable, uses an unknown key, or otherwise cannot be trusted. | Contact the router operator with the request ID. | Replace the license with a valid Metrum-issued file; do not expose payloads or signatures in tickets. |
| `license-expired` | 403 | The license signature is valid but the license is expired. | Contact the router operator. | Renew or restore a valid license file, then restart or wait for recheck. |
| `license-not-yet-valid` | 503 | The license `not_before` time is in the future. | Contact the router operator. | Check the issued license dates and system clock. |
| `license-product-mismatch` | 503 | The license is not issued for GenAI Smart Router. | Contact the router operator. | Install the correct product license. |
| `license-feature-forbidden` | 403 | The request uses a feature not enabled by the current license. | Use an enabled feature or ask the operator for access. | Review licensed feature gates for routing, reporting, dynamic score, TypeScript, external policy, contracts, rollups, or content capture. |
| `license-limit-exceeded` | 403 | The deployment exceeds a licensed limit such as model groups or callers. | Contact the router operator. | Reduce configured usage or update the license. |
| `license-clock-rollback` | 503 | The local wall clock moved backwards beyond tolerance. | Contact the router operator. | Correct system time and inspect the license state file. |

## Eligibility Requirements

The `no-eligible-target` response includes a `requirements` list. Examples:

- `text`: request needs text input support.
- `image`: request includes image input.
- `tools`: request includes tool definitions.
- `openai-chat_tool_passthrough`: OpenAI Chat tool payload must be preserved.
- `openai-responses_function`: Responses function tools are required.
- `anthropic-messages_client_tools`: Anthropic Messages client tools are required.

Resolution is usually a configuration update. The model group must contain at least one enabled target whose provider dialect and metadata satisfy those requirements.

When a model group has an optional contract, the same error can include safe `contract-*` requirement buckets. Examples include `contract-required-api-shape`, `contract-required-modality`, `contract-required-tools`, `contract-quality-floor`, `contract-validation-expired`, and `contract-no-validated-target`. These buckets mean the requested group was allowed for the caller, but no target inside that same group satisfied the configured contract after ordinary request eligibility.

## Max-Token Cap Errors

If a caller sets `max_tokens`, OpenAI Chat `max_completion_tokens`, or Responses `max_output_tokens`, the router skips targets known not to honor output caps when that metadata is configured. Keep a target cataloged but inactive for capped traffic by setting `honors_max_tokens: false` after a failed cap smoke. For OpenAI Chat requests that include both `max_tokens` and `max_completion_tokens`, `max_tokens` takes precedence.

Explicit output caps also affect quota admission. The router reserves estimated input tokens plus `max_tokens`, `max_completion_tokens`, or `max_output_tokens` before upstream calls, then reconciles the reservation to actual usage when the request finishes. Failed or canceled upstream calls release the reservation, and cache hits do not consume persisted token quota.

## Cursor And Large-Context TPM Troubleshooting

Router caller limits can include:

- `rpm`: requests per rolling minute;
- `tpm`: estimated input plus reserved output tokens per rolling minute;
- `concurrent`: in-flight request count.

Large-context clients such as Cursor, opencode, coding agents, and repository-wide tools can hit `429 tpm-exceeded` even when daily or monthly budgets are healthy. A few 150K-token requests inside the same rolling minute can exceed TPM, especially when each request also reserves the requested output cap.

Caller guidance:

- reduce selected files, repository context, diff size, or prompt attachments;
- lower unrealistic output caps;
- retry after the rolling window clears;
- include `X-Request-Id` when escalating.

Administrator guidance:

- inspect usage by client, owner user, project, public token ID, requested model group, input-token bucket, max-token bucket, and quota bucket;
- raise TPM for trusted production keys when the workload is approved;
- route routine large-context work to cheaper or smaller groups only after those groups pass the workload verifier;
- distinguish router `429 quota-exceeded`, `rate-limited`, or `tpm-exceeded` from upstream provider `429` attempts and client cancellations by reviewing `request_usage`, `request_attempts`, `request_trace_events`, and `request_errors`.

## Upstream Provider Quota And Billing Errors

Provider-side balance, credit, quota, billing, and payment failures are distinct from caller-token `quota-exceeded` responses. The router first tries eligible fallback targets. If a fallback succeeds, the caller receives the successful response and diagnostics record the failed attempt. If every eligible attempt fails with provider quota or billing signals, the caller receives `503 upstream-quota-exhausted`.

Safe example:

```json
{
  "error": {
    "type": "upstream-quota-exhausted",
    "message": "upstream provider quota, credits, or billing limits were exhausted for model \"default\" after 2 attempt(s); retry later or contact the router operator with the request_id",
    "details": {
      "model": "default",
      "dialect": "openai-chat",
      "attempts": 2,
      "last_error": "upstream status 402 upstream provider quota, credits, or billing limit exhausted",
      "retryable": true,
      "request_id": "req_0123456789abcdef0123456789abcdef",
      "fallbackUsed": true
    }
  }
}
```

The error body is sanitized. It does not include provider account identifiers, raw upstream response bodies, upstream headers, provider API keys, router tokens, token hashes, prompts, images, or tool output.

## Troubleshooting With Request IDs

Administrators can use `X-Request-Id` to inspect:

- `request_usage` for terminal status, selected target, token counts, cost, and cache behavior.
- `request_attempts` for each provider/model attempt.
- `request_trace_events` for routing, fallback, timeout, and cache decisions.
- `request_errors` for sanitized terminal error summaries.

Diagnostic rows exclude prompt text, raw image payloads, raw router tokens, token hashes, provider API keys, full upstream headers, and unsanitized upstream bodies.

For provider quota or billing incidents, look for `request_attempts.error_class = 'upstream_quota_exhausted'` and terminal `request_errors.error_type = 'upstream-quota-exhausted'`. A successful request can still have an `upstream_quota_exhausted` attempt row when fallback succeeded.

If governed content capture is enabled by an operator, captured content lives in separate content-capture tables and remains outside usage reports and diagnostics. Delete and retention-purge maintenance endpoints require `content:capture` `delete`/`purge` authorization.
