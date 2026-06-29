# Troubleshooting Runbook

Use this runbook for production issues reported by users or monitoring.

## First Checks

Capture:

- exact timestamp in UTC;
- caller-visible error body;
- `X-Request-Id`;
- requested endpoint and model group;
- client type, such as Codex CLI, Claude Code, Warp, SDK, or custom app;
- whether the request included tools, images, streaming, or a low token cap.

Do not ask users for provider keys or raw router tokens.

## Health And Version

```bash
rtk curl -fsS https://llm-api-engg.metrum.ai/readyz
rtk curl -fsS https://llm-api-engg.metrum.ai/version
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose ps'
```

## Request ID Investigation

Use `X-Request-Id` to inspect relational usage tables:

- `request_usage`: terminal status, caller, selected target, token counts, cost fields, cache status.
- `request_attempts`: upstream provider/model attempts, status, duration, timeout/cancel flags.
- `request_trace_events`: routing decisions, fallback, cache, timeout, terminal failure.
- `request_traffic_shape_events`: per-bucket caller traffic-shaping decisions when shaping was applied.
- `request_errors`: sanitized terminal error class/message.

Diagnostic tables must not store raw prompts, raw image payloads, raw tokens, token hashes, provider keys, full upstream headers, or unsanitized upstream response bodies.

## Common Cases

### `no-eligible-target`

Check the requirements in the error body. The fix is normally a configuration change: enable or add a target in the requested model group that supports the requested dialect, tools, structured outputs, modalities, and max-token cap behavior.

If the requirements include `contract-*`, inspect only the requested group. Contract enforcement is group-local and runs after caller authorization and normal request eligibility. Common fixes are to add a validated target, refresh stale target validation metadata, relax `quality_floor.max_eval_age_days`, lower an operational threshold, or roll back by removing the optional `contract` block. Do not route the caller to another group unless the caller is explicitly allowed to use that deployment-defined group.

If the requirements include `reasoning`, inspect only targets in the requested group. Confirm the request shape is OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, or Anthropic Messages `thinking`, then check whether any target has validated compatible `reasoning` metadata for that exact dialect and skin. A mixed weighted group may intentionally keep non-reasoning targets for ordinary traffic, but explicit reasoning requests need at least one compatible target. Fixes are usually to add or restore validated metadata, add a validated reasoning target, remove an overly broad `required_capabilities.reasoning` contract, or disable a dynamic-score reasoning hard filter that is stricter than the target set.

### Empty Final Content From Reasoning Requests

Reasoning-heavy upstreams can spend a small caller output cap on internal reasoning and return little or no final assistant content. Reproduce with both the original cap and a realistic cap such as 512 or 1024 output tokens. Check target metadata for `min_budget_tokens`, `max_budget_tokens`, `budget_must_be_less_than_max_tokens`, `rejects_max_tokens`, `rejects_temperature`, and `rejects_top_p`. If the realistic-budget smoke passes but low-cap requests fail, configure the target so capped requests are translated or skipped safely, or remove the reasoning metadata until the exact behavior is understood.

### Timeout

Check upstream attempt durations, provider status, client timeout settings, router upstream timeout config, and whether the model is reasoning-heavy with too small a token budget. Run both direct upstream and router smokes with realistic `max_tokens`.

### Rate Limit Or Quota

Check rpm, tpm, concurrency, and daily/monthly/lifetime request and token caps. `rpm` and `tpm` are rolling-window limits; `concurrent` is in-flight request count. Use usage reports to confirm whether the block is expected. Keep caller policies consistent unless there is an explicit product reason for different tiers.

For token-budget rejections, inspect the caller's requested output cap as well as recent actual usage. The router reserves estimated input tokens plus `max_tokens`, `max_completion_tokens`, or `max_output_tokens` before upstream calls, and TPM, daily, monthly, and lifetime checks include other in-flight reservations. A small prompt can be rejected near a budget if it asks for a very large possible output. Failed or canceled upstream calls release the reservation, while successful calls reconcile to actual reported usage.

Safe large-context example: a Cursor or opencode user can hit `429 tpm-exceeded` after several repository-wide or large-diff requests even when RPM and concurrency look normal. Triage with the client, model group, UTC window, and public request IDs, then filter usage by `client` and `resolved_group` to compare input tokens, requested output cap, in-flight reservations, and retry timing. Do not collect raw prompts, repository contents, bearer tokens, token hashes, provider keys, or full production config. Operational fixes are usually to reduce the client's context window or retry burst, use a lower output cap, move the user to a caller policy with a larger TPM budget, or split the workload across smaller requests.

When the workload is trusted and production-critical, raising TPM for that key can be the right fix. For routine or exploratory work, prefer reducing client context, lowering output caps, splitting requests, or moving the key to a cheaper/smaller model group only after that group passes the workload verifier. Distinguish router-side `429` policy failures from upstream provider `429` attempts and user/client cancellations before changing quotas.

### Traffic Shaping

`429 traffic-shaped` is separate from `tpm-exceeded`, `rpm-exceeded`, `concurrency-exceeded`, and upstream `503 upstream-rate-limited`. Traffic shaping smooths how quickly one caller can start requests or reserve estimated input/output token capacity after auth, model allow-list checks, hard quota/rate checks, and token estimation, but before upstream calls. It is disabled unless `server.traffic_shape.enabled` or a caller `traffic_shape.enabled` block is configured.

Use the response `bucket`, `Retry-After`, and `X-Request-Id` first. Then inspect safe scalar usage fields: `traffic_shape_applied`, `traffic_shape_decision`, `traffic_shape_scope`, `traffic_shape_bucket`, `traffic_shape_retry_after_ms`, `traffic_shape_queue_wait_ms`, `traffic_shape_estimated_input_tokens`, `traffic_shape_reserved_output_tokens`, and `traffic_shape_total_reserved_tokens`.

Join `request_traffic_shape_events` by `request_id` for one row per evaluated bucket. For large-context coding clients, compare shaped buckets with the caller's hard `rate.tpm`, requested output cap, and upstream `request_attempts.error_class`. Operational fixes are usually to reduce retry bursts or context size, lower output caps, increase the specific shaping bucket only for the trusted caller, or temporarily disable `traffic_shape.enabled` for rollback. Do not collect raw prompts, raw images, bearer tokens, token hashes, provider keys, or full production config while triaging.

Separate caller-token quota failures from upstream provider quota or billing exhaustion. Caller policy failures return `429 quota-exceeded` or `429 rate-limited` before any provider call. Provider balance, credit, billing, payment, or quota failures are recorded per attempted target as `request_attempts.error_class = 'upstream_quota_exhausted'`; if no fallback succeeds, callers receive `503 upstream-quota-exhausted` with a sanitized `request_id`. Use that request ID to inspect `request_attempts`, `request_trace_events`, and `request_errors`, then verify the provider account balance, billing state, quota entitlement, and provider status page. If a later fallback succeeds, the terminal `request_usage` row remains `200` with `fallback_used = true`, and the failed provider attempt still appears in `request_attempts`.

Separate provider/model shared shaping from both caller limits and upstream-returned `429`. Provider shaping returns `503 upstream-capacity-throttled` when every otherwise eligible target is locally throttled before an upstream call starts. Use the request ID to inspect `request_upstream_shape_events`: `scope` identifies provider, provider_model, or target; `bucket` identifies request-start, input-token, total-reserved-token, or adaptive-backoff admission; `decision` shows admitted, skipped, rejected, or cooldown_started; and `backoff_reason` distinguishes `provider-shape-throttled`, `model-shape-throttled`, `target-shape-throttled`, `adaptive-backoff-provider-429`, and `adaptive-backoff-provider-quota`. If `request_attempts` has an `upstream_rate_limited` attempt immediately before shape skips, the router is backing off from a real upstream rate-limit response. If there are no new attempts, the local bucket is protecting configured shared capacity.

## License Errors

`license-*` errors occur before upstream routing. `license-expired`, `license-feature-forbidden`, and `license-limit-exceeded` indicate a verified license that does not currently permit the request; other license errors normally mean the file is missing, malformed, unverifiable, for the wrong product, not yet valid, or the local clock moved backwards. Check `/readyz`, safe `request_usage.license_status` and `license_reason` fields, metrics-admin license gauges, and authorized `/admin/license/status`. Do not copy license payloads, signatures, private keys, or full config into tickets; use request IDs and safe status fields.

For issuance, renewal, replacement, volume top-up, offline support, and SKU-specific acceptance checks, use `docs/LICENSE_OPERATIONS.md`. The support case should record only safe scalar license metadata such as license ID, customer alias, SKU, key ID, expiry, grace flag, request ID, status, and reason.

Provider rate limits are recorded as `upstream_rate_limited` and return `503 upstream-rate-limited` only after eligible fallbacks are exhausted. When adaptive backoff is configured, the next requests can skip that provider/model/target and either route around it or return `503 upstream-capacity-throttled` if no alternative is available. Do not paste raw provider error bodies, account IDs, API keys, router tokens, token hashes, prompts, images, or tool outputs into incident notes.

Ordinary upstream 4xx policy, authorization, and malformed-request errors are non-retryable and stop fallback so the same caller payload is not replayed to another provider. Provider quota, credit, billing, rate-limit, timeout, network, and 5xx classes remain retryable when another eligible target exists. Redirect responses are not followed; investigate the configured provider base URL instead of expecting the router to chase `Location` headers.

If an image-bearing request fails before upstream with `image_url_forbidden`, inspect only the URL class, not the raw image content. The default policy blocks `http`/`https` image URLs that point to or resolve to loopback, link-local, RFC1918/private, multicast, or unspecified addresses. Prefer data URLs or a reviewed public object-store URL; use `server.upstream.allow_private_image_urls: true` only for a private VLM deployment with reviewed egress controls.

### Bad Image Analysis

Separate transport success from task quality. A model can remain active for general VLM routing even if it is not good enough for OCR-specific routing. For OCR routes, require exact-answer image smokes.

### Tool-Calling Failure

Validate the exact client dialect:

- Warp and many OpenAI-compatible agents use `/v1/chat/completions` tool passthrough.
- Codex CLI uses `/v1/responses`.
- Claude Code uses `/v1/messages`.

Run the appropriate real tool smoke and assert file contents, not only assistant text.

## Logs

Recent router logs:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose logs --tail=200 router'
```

Prefer DB traces for request-level details because logs should remain sanitized and compact.
