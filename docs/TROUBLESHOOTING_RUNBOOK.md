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
- `request_shapes`: one safe request-shape row with dialect, stream flag, item/message/role counts, tool count, tool-choice mode, structured-output/reasoning/multimodal flags, coarse size/token/output-cap buckets, and non-reversible request/tool-schema fingerprints.
- `request_translation_shapes`: one safe translated-shape row per upstream attempt with provider/model/dialect/path, translated stream/tools/tool-choice/output-cap/reasoning controls, translated request bytes bucket, and strip/rewrite/warning counts.
- `request_translation_field_events`: bounded child rows for allowlisted translated fields or `other`, with actions such as `stripped`, `rewritten`, and `unsupported`.
- `request_upstream_error_details`: bounded allowlisted provider 4xx/5xx fields such as code, type, param, request ID, and categorized provider message when `store_sanitized_upstream_errors` is enabled.
- `request_errors`: sanitized terminal error class/message.

Diagnostic tables must not store raw prompts, raw image payloads, image URLs, raw tool schemas, raw tool outputs, raw tokens, token hashes, provider keys, full upstream headers, or unsanitized upstream response bodies.

For browser-first incident response, use `/admin/reports/` before falling back to manual SQL:

- Upstream failures: filter by `since`, provider, target model, dialect, status, caller user, project, environment, client, request bytes bucket, or tool-choice mode to find provider/model/status/error-code spikes.
- Shape failures: compare success and failure rates by safe request-shape bucket, translated-shape bucket, request-shape fingerprint, and tool-schema fingerprint.
- Fallback health: confirm whether multi-attempt requests recovered through fallback or still ended in caller-visible 4xx/5xx responses.
- User impact: rank affected caller users and clients by error rate, latency, and resolved model group.

These reports use only safe scalar telemetry and request IDs. They do not expose raw prompts, image payloads, image URLs, raw tool schemas, tool outputs, bearer tokens, provider keys, token hashes, full upstream headers, raw upstream bodies, or free-form provider prose. Use request drilldown from a report row when a single request needs attempt, trace, upstream-error, request-shape, translation-shape, or decision telemetry.

For a single failed request, prefer the evidence bundle endpoint before writing ad hoc SQL:

```bash
rtk curl -u admin:<password> \
  "https://llm-api-engg.metrum.ai/admin/reports/api/request-evidence?request_id=<request_id>"
```

The same data is available at `/admin/reports/api/request/<request_id>` for path-style drilldown links. Both endpoints require `admin:reports` `drilldown`, send `Cache-Control: no-store`, and return `403 reports-forbidden` to ordinary router caller tokens. Domain-scoped admins receive `404` for request IDs outside their Casbin domain.

Evidence bundles are assembled from normalized relational rows keyed by `request_id`. They include a safe request summary, caller/project/client labels, requested and resolved model group, selected provider/model/dialect, stored request-time token and cost fields, upstream-reported billed cost fields, latency/TTFB/upstream/downstream timing, quota/key/cache state, traffic-shaping state, target candidate/filter/routing summaries, attempt rows, sanitized upstream error fields, trace rows, and completeness metadata.

Use `diagnosticCompleteness` and `evidenceSections` to decide whether missing evidence is expected:

- `complete`: expected sections are present.
- `partial_expected`: only disabled or not-applicable sections are absent.
- `partial_missing`: one or more expected diagnostic sections are missing unexpectedly.
- `minimal`: only the usage row and a small subset of expected evidence are present.

Do not treat a missing optional section as proof that routing skipped that phase unless the section status says `not_applicable`. For non-2xx requests, missing `attempts`, `terminal_errors`, `request_shape`, `target_eligibility`, `sanitized_upstream_errors`, or `trace_timeline` should be investigated as a telemetry regression unless the request was rejected before that phase.

For incident windows with many requests, page the admin request API instead of asking the browser to load the whole result set:

```bash
rtk curl -u admin:<password> \
  "https://llm-api-engg.metrum.ai/admin/reports/api/requests?since=24h&limit=50&client=codex-cli&sort=timeUtc&direction=desc"
```

Use the returned `pagination.next_cursor` for the next page. The cursor is opaque and bound to the endpoint, sort, and direction; a malformed or stale cursor returns `400 invalid-report-filter`. Domain-scoped admins continue to see only their project/environment on every page. Aggregate tabs such as usage by key or provider/model are top-N summaries and should be used to identify dimensions before drilling into the cursor-paged request or security-event APIs.

In the browser admin UI, use `/admin/reports/?tab=requests&since=24h&limit=50&sort=timeUtc&direction=desc` for the same flow. Add filters such as `caller_user`, `client`, `resolved_group`, or `status`, then page with Next. Filter, tab, limit, and sort changes reset the cursor to the first page. Use `CSV current page` only for the rows currently returned by the server; use top-N aggregate tabs such as Provider/model to find dimensions, not as page 1 of every matching request.

## Common Cases

### `no-eligible-target`

Check the requirements in the error body. The fix is normally a configuration change: enable or add a target in the requested model group that supports the requested dialect, tools, structured outputs, modalities, and max-token cap behavior.

If a model catalog appears to advertise the missing capability, verify whether that metadata is effective for the active provider skin. Open `/admin/reports/api/provider-catalog-status` or the Provider catalog tab and filter to the requested group/provider/model. Active target rows show `activeEligibilitySkin`, `effectiveToolSupport`, `inactiveToolSupport`, `effectiveStructuredOutputs`, `effectiveReasoning`, and `effectiveImageInput`. A Chat target with Responses metadata in `inactiveToolSupport` is not eligible for `/v1/responses`; add a validated Responses skin or explicitly documented bridge target instead of assuming catalog metadata activates the route.

If the requirements include `contract-*`, inspect only the requested group. Contract enforcement is group-local and runs after caller authorization and normal request eligibility. Common fixes are to add a validated target, refresh stale target validation metadata, relax `quality_floor.max_eval_age_days`, lower an operational threshold, or roll back by removing the optional `contract` block. Do not route the caller to another group unless the caller is explicitly allowed to use that deployment-defined group.

If the requirements include `reasoning`, inspect only targets in the requested group. Confirm the request shape is OpenAI Chat `reasoning_effort`, OpenAI Responses `reasoning`, or Anthropic Messages `thinking`, then check whether any target has validated compatible `reasoning` metadata for that exact dialect and skin. A mixed weighted group may intentionally keep non-reasoning targets for ordinary traffic, but explicit reasoning requests need at least one compatible target. Fixes are usually to add or restore validated metadata, add a validated reasoning target, remove an overly broad `required_capabilities.reasoning` contract, or disable a dynamic-score reasoning hard filter that is stricter than the target set.

For Anthropic endpoint split regressions, classify the request by path first. `/anthropic/v1/messages` and legacy `/v1/messages` are Anthropic Messages inbound traffic even when the selected upstream would be OpenAI Chat or Responses. Plain Messages text can use a non-native target only when the active target has `request_shape_support.supported_inbound_dialects` including `anthropic`; otherwise the stricter eligibility filter should return `502 no-eligible-target` before upstream. Native Anthropic Messages provider skins are eligible by dialect, but Claude Code tools still require `tool_support.anthropic_messages` or an explicitly validated Messages bridge. Safe evidence should show the request ID, inbound dialect, requested group, candidate target dialects, filter reasons, selected target when any, attempts count, and terminal error class. Do not copy the full production config, raw tokens, token hashes, provider keys, prompts, images, or tool schemas into an issue.

If a production config migration causes ordinary Claude Code or Messages text to fail, roll back locally in the requested group: restore the prior target list, remove an invalid Anthropic inbound opt-in, add or restore a native Messages target, or temporarily move affected callers to a previously validated group they are already allowed to use. Verify recovery with authenticated `/v1/models`, `/anthropic/v1/messages`, `/anthropic/v1/messages/count_tokens` when supported, a legacy `/v1/messages` compatibility smoke if that path is still used, and a negative `no-eligible-target` smoke against a group with no compatible target.

### Reasoning And Bridge Decision Tree

When a caller reports missing reasoning or an unexpected bridge failure, prove each layer with safe metadata rather than copying payloads:

1. Identify the endpoint and shape: `/v1/chat/completions` with `reasoning_effort`, `/v1/responses` with `reasoning`, or `/anthropic/v1/messages` with `thinking`. Legacy `/v1/messages` requests are still Anthropic Messages traffic. Some IDE and custom-provider clients can send OpenAI Responses-looking fields to a Chat endpoint; classify by actual router path and stored `request_shapes.inbound_dialect`.
2. Call `/v1/models` with the same caller token and confirm the requested deployment-defined group is allowed and advertises reasoning metadata only when expected.
3. Inspect target candidates for the requested group. A catalog entry is not enough; check active target dialect, `tool_only`, `effectiveReasoning`, bridge metadata, and safe filter reasons.
4. For same-dialect routing, confirm the selected upstream dialect matches the caller surface and `request_translation_shapes.translated_reasoning_control` is `reasoning_effort`, `reasoning`, or `thinking` as appropriate.
5. For Chat-to-Responses, confirm `bridges.chat_to_responses.enabled: true`; for reasoning, also require `bridges.chat_to_responses.reasoning: true` and compatible target `reasoning` metadata. Evidence should show inbound `openai-chat`, target `openai-responses`, `bridge_direction = chat_to_responses`, and a translated Responses reasoning control.
6. For Responses-to-Chat, assume reasoning is unsupported unless the target has explicit `responses_to_chat.reasoning` validation. A `reasoning` object, `previous_response_id`, hosted tools, images, structured outputs, or streaming should produce a bounded filter reason unless the exact flag is enabled.
7. If the request failed, prove no silent drop occurred: the terminal error should be `no-eligible-target` or a bounded bridge error, the selected target should be absent, and there should be zero upstream attempts for pre-selection failures.

Common client expectations: Codex normally uses OpenAI Responses and may need Responses-native or explicitly bridged targets. Claude Code uses Anthropic Messages and may include `thinking` or tool-related thinking constraints. Cursor, opencode, aider, and SDK-based IDE clients can use OpenAI Chat, Anthropic-compatible, or mixed legacy OpenAI-compatible shapes depending on version and configuration; always use the stored inbound dialect, request ID, and selected provider skin as evidence.

### Codex Does Not Show Reasoning Controls

Use the exact caller token that Codex uses and call `/v1/models`. If the requested model group is missing `supported_reasoning_levels` and `default_reasoning_level`, the running deployment is not advertising active reasoning metadata to that caller. Check these in order:

1. The caller allow list includes the intended group.
2. The reasoning target is active under `models.<group>.targets[]`, not catalog-only.
3. The active target is not `tool_only` unless the tested request shape is specifically a tool-only path.
4. The resolved target skin matches the client surface: OpenAI Chat for `reasoning_effort`, OpenAI Responses for `reasoning`, and Anthropic Messages for `thinking`.
5. The running production config and package are the deployed ones, not only updated source files.

Then run the repeatable proof:

```bash
rtk python3 scripts/reasoning_smoke.py \
  --base-url https://llm-api-engg.metrum.ai \
  --token-file <router-token-file> \
  --model <group> \
  --postgres-dsn "$ROUTER_USAGE_DB_DSN"
```

The failure tells you which layer is stale: `/v1/models` metadata, an API surface smoke, missing usage DB rows, wrong `translated_reasoning_control`, unexpected selected target, or fallback to a non-proof target. Roll back or restore the previous reasoning-capable target set if production source and runtime config disagree.

For `/v1/responses` requests that should be able to use a Chat-only target, inspect target candidates and filter reasons for `responses-to-chat-*`. A Chat target is eligible only when its resolved target metadata has `responses_to_chat.enabled: true` and flags for the requested shape. Common bridge reasons are `responses-to-chat-bridge-disabled`, `responses-to-chat-previous-response-id`, `responses-to-chat-hosted-tools`, `responses-to-chat-tool-choice`, `responses-to-chat-image`, `responses-to-chat-reasoning`, `responses-to-chat-structured-output`, and `responses-to-chat-streaming`. These failures should have zero upstream attempts. If the bridge succeeds, `request_usage.inbound_dialect` remains `openai-responses`, `request_usage.target_dialect` is `openai-chat`, and `request_translation_shapes.bridge_direction` is `responses_to_chat`.

### Empty Final Content From Reasoning Requests

Reasoning-heavy upstreams can spend a small caller output cap on internal reasoning and return little or no final assistant content. Reproduce with both the original cap and a realistic cap such as 512 or 1024 output tokens. Check target metadata for `min_budget_tokens`, `max_budget_tokens`, `budget_must_be_less_than_max_tokens`, `rejects_max_tokens`, `rejects_temperature`, and `rejects_top_p`. If the realistic-budget smoke passes but low-cap requests fail, configure the target so capped requests are translated or skipped safely, or remove the reasoning metadata until the exact behavior is understood.

### Timeout

Check upstream attempt durations, provider status, client timeout settings, router upstream timeout config, and whether the model is reasoning-heavy with too small a token budget. Run both direct upstream and router smokes with realistic `max_tokens`.

### Rate Limit Or Quota

Check rpm, tpm, concurrency, and daily/monthly/lifetime request and token caps. `rpm` and `tpm` are rolling-window limits; `concurrent` is in-flight request count. Use usage reports to confirm whether the block is expected. Keep caller policies consistent unless there is an explicit product reason for different tiers.

For token-budget rejections, inspect the caller's requested output cap as well as recent actual usage. The router reserves estimated input tokens plus `max_tokens`, `max_completion_tokens`, or `max_output_tokens` before upstream calls, and TPM, daily, monthly, and lifetime checks include other in-flight reservations. A small prompt can be rejected near a budget if it asks for a very large possible output. Failed or canceled upstream calls release the reservation, while successful calls reconcile to actual reported usage.

Safe large-context example: a Cursor or opencode user can hit `429 tpm-exceeded` after several repository-wide or large-diff requests even when RPM and concurrency look normal. Triage with the client, model group, UTC window, and public request IDs, then filter usage by `client` and `resolved_group` to compare input tokens, requested output cap, in-flight reservations, and retry timing. Do not collect raw prompts, repository contents, bearer tokens, token hashes, provider keys, or full production config. Operational fixes are usually to reduce the client's context window or retry burst, use a lower output cap, move the user to a caller policy with a larger TPM budget, or split the workload across smaller requests.

For Cursor-style OpenAI Chat requests that include tools and an image, confirm the selected model group has at least one target that supports both OpenAI Chat tool passthrough and image input on the same target. A text-only tool target, an OpenAI Responses image/function target, or an Anthropic Messages tool target is not eligible for that exact OpenAI Chat request shape. If every target is filtered, the expected failure is `502 no-eligible-target` with zero upstream attempts plus safe `request_target_candidates` and `request_target_filter_reasons` rows such as `input-modality-image` or `dialect-tool-passthrough`.

For OpenAI Chat callers that are expected to use a Responses-only upstream, inspect the resolved target metadata. The target must be `openai-responses` and explicitly set `bridges.chat_to_responses.enabled: true`; tool requests also need bridge `tools: true` and `tool_support.openai_responses`. Streaming, images, structured outputs, reasoning, `tool_choice`, and `parallel_tool_calls` are individually gated. Reasoning requests require both `bridges.chat_to_responses.reasoning: true` and compatible target `reasoning` metadata; otherwise expect `chat-to-responses-reasoning-unsupported` or a reasoning support filter before upstream. If `stateful_sessions.enabled` is set, confirm the caller sends the configured session header, the deployment is single-process or sticky-routed, and request traces show `bridge_session_requested`; a follow-up request in the same session should show `bridge_session_previous_response_applied`. If the upstream rejects an injected `previous_response_id` as stale, expired, invalid, or missing, the expected recovery trace is `bridge_session_previous_response_stale_purged` followed by `bridge_session_stateless_retry`. Expected safe filter reasons include `chat-to-responses-bridge-disabled`, `chat-to-responses-streaming-unsupported`, `chat-to-responses-tools-unsupported`, `chat-to-responses-tool-choice-unsupported`, `chat-to-responses-reasoning-unsupported`, and `chat-to-responses-image-unsupported`.

For successful bridged requests, usage and evidence should show inbound dialect `openai-chat`, target/attempt dialect `openai-responses`, endpoint path `/v1/responses`, `request_translation_shapes.bridge_direction = chat_to_responses`, and translation-shape buckets for output cap, tool count, stripped/rewritten fields, reasoning control, and request bytes. Stateful session proof should use safe trace event names and direct mock/live upstream request bodies during validation; do not collect raw session header values in logs. Do not collect raw prompts, raw tool schemas, tool outputs, images, bearer tokens, token hashes, provider keys, or full config while triaging bridge behavior.

When the request ID matches a production-derived class such as large Cursor/OpenAI Chat tool payloads, Codex Responses reasoning/tools, Claude Code thinking/tools, provider-skin mismatch, no-eligible diagnostics, or upstream error classification, replay the sanitized fixture matrix before closeout:

```bash
rtk go test ./internal/router -run 'ProductionDerived'
rtk python3 scripts/prod_smoke_regressions.py --mode prod --fixture all --model-group reasoning-bridge-smoke
```

Use a deployment-defined smoke group and a scoped smoke caller with access granted in config, for example Harbor/Chetan access in the managed deployment. Do not change production `big-coder` just to run these fixtures; missing smoke group, caller access, or report DB access should be recorded as the blocker.

When the workload is trusted and production-critical, raising TPM for that key can be the right fix. For routine or exploratory work, prefer reducing client context, lowering output caps, splitting requests, or moving the key to a cheaper/smaller model group only after that group passes the workload verifier. Distinguish router-side `429` policy failures from upstream provider `429` attempts and user/client cancellations before changing quotas.

### Traffic Shaping

`429 traffic-shaped` is separate from `tpm-exceeded`, `rpm-exceeded`, `concurrency-exceeded`, and upstream `503 upstream-rate-limited`. Traffic shaping smooths how quickly one caller can start requests or reserve estimated input/output token capacity after auth, model allow-list checks, and token estimation, but before upstream calls. Request-start queueing happens before the active concurrency slot is acquired so queued requests do not occupy `concurrent`; hard `rpm`, `tpm`, `concurrent`, quota, lifetime-budget, and license checks still run and cannot be bypassed. It is disabled unless `server.traffic_shape.enabled` or a caller `traffic_shape.enabled` block is configured.

Use the response `bucket`, `Retry-After`, and `X-Request-Id` first. Then inspect safe scalar usage fields: `traffic_shape_applied`, `traffic_shape_decision`, `traffic_shape_scope`, `traffic_shape_bucket`, `traffic_shape_retry_after_ms`, `traffic_shape_queue_wait_ms`, `traffic_shape_estimated_input_tokens`, `traffic_shape_reserved_output_tokens`, and `traffic_shape_total_reserved_tokens`.

Join `request_traffic_shape_events` by `request_id` for one row per evaluated bucket. For large-context coding clients, compare shaped buckets with the caller's hard `rate.tpm`, requested output cap, and upstream `request_attempts.error_class`. Operational fixes are usually to reduce retry bursts or context size, lower output caps, increase the specific shaping bucket only for the trusted caller, or temporarily disable `traffic_shape.enabled` for rollback. Do not collect raw prompts, raw images, bearer tokens, token hashes, provider keys, or full production config while triaging.

For aggregate triage, run `router-usage-report --traffic-shaped-only --since 24h` and inspect Traffic Shaping Summary plus Traffic Shaping By User / Project, By Key, By Client, and By Model Group. In the browser admin UI, use the Traffic shaping overview, Shaping users, Shaping keys, Shaping clients, and Shaping groups tabs. A `queued` decision with nonzero queue wait explains bounded added latency; a `rejected` decision with nonzero queue wait usually means the request timed out in the bounded queue, while a `rejected` decision with zero queue wait usually means the queue was disabled, full, or the reservation could never fit within the burst.

Run the traffic tuning advisor before changing production shaping values:

```bash
router-usage-report --driver postgres --dsn "$ROUTER_USAGE_DB_DSN" --since 24h --traffic-tuning-advisor --caller-user <owner-user>
```

Decision tree:

- `route_around_incompatible_target`: the user saw errors, but caller shaping did not reject or queue the traffic and upstream 400/request-shape failures dominate. Inspect request-shape failures, upstream failures, target tool/modality/cap metadata, and remove or lower the incompatible target. Do not increase burst or queue depth for this symptom.
- `enable_queue`, `increase_queue_depth`, or `increase_queue_wait`: router-side shaping is actually rejecting or queueing traffic. Change only the named caller/server queue or burst fields, then verify queue wait p50/p95/max, user latency, and upstream errors after rollout.
- `disable_queue_for_latency_sensitive_client`: client cancellations rose while queue waits were high. Lower `queue.max_wait_ms` or disable queueing for that caller; fail-fast is better for many interactive coding clients.
- `investigate_provider_429_capacity`: provider 429 or adaptive backoff is shared across users. Tune provider/model/target `traffic_shape`, `upstream_429_backoff`, route weights, or provider entitlement before raising one caller's burst.
- `decrease_caller_rate`: user-visible errors and upstream 5xx/timeouts are both high. Slow the client, lower concurrency, or reduce retry intensity while provider health is investigated.
- `no_shaping_change_indicated`: no conservative threshold crossed. Keep current shaping and inspect request-level failures, client behavior, provider health, or model compatibility instead.

Separate caller-token quota failures from upstream provider quota or billing exhaustion. Caller policy failures return `429 rpm-exceeded`, `429 tpm-exceeded`, `429 concurrency-exceeded`, or `429 quota-exhausted` before any provider call. Provider balance, credit, billing, payment, or quota failures are recorded per attempted target as `request_attempts.error_class = 'upstream_quota_exhausted'`; if no fallback succeeds, callers receive `503 upstream-quota-exhausted` with a sanitized `request_id`. Use that request ID to inspect `request_attempts`, `request_trace_events`, and `request_errors`, then verify the provider account balance, billing state, quota entitlement, and provider status page. If a later fallback succeeds, the terminal `request_usage` row remains `200` with `fallback_used = true`, and the failed provider attempt still appears in `request_attempts`.

Separate provider/model shared shaping from both caller limits and upstream-returned `429`. Provider shaping returns `503 upstream-capacity-throttled` when every otherwise eligible target is locally throttled before an upstream call starts. Use the request ID to inspect `request_upstream_shape_events`: `scope` identifies provider, provider_model, or target; `bucket` identifies request-start, input-token, total-reserved-token, or adaptive-backoff admission; `decision` shows admitted, skipped, rejected, or cooldown_started; and `backoff_reason` distinguishes `provider-shape-throttled`, `model-shape-throttled`, `target-shape-throttled`, `adaptive-backoff-provider-429`, and `adaptive-backoff-provider-quota`. If `request_attempts` has an `upstream_rate_limited` attempt immediately before shape skips, the router is backing off from a real upstream rate-limit response. If there are no new attempts, the local bucket is protecting configured shared capacity.

For aggregate provider-shaping triage, use `router-usage-report --provider <provider> --traffic-shape-scope provider --since 24h` and the Provider shaping and Backoff admin tabs. Confirm whether route-around successes are high enough before tightening limits or removing active targets.

## License Errors

`license-*` errors occur before upstream routing. `license-expired`, `license-feature-forbidden`, and `license-limit-exceeded` indicate a verified license that does not currently permit the request; other license errors normally mean the file is missing, malformed, unverifiable, for the wrong product, not yet valid, or the local clock moved backwards. Check `/readyz`, safe `request_usage.license_status` and `license_reason` fields, metrics-admin license gauges, and authorized `/admin/license/status`. Do not copy license payloads, signatures, private keys, or full config into tickets; use request IDs and safe status fields.

For issuance, renewal, replacement, volume top-up, offline support, and SKU-specific acceptance checks, use `docs/LICENSE_OPERATIONS.md`. The support case should record only safe scalar license metadata such as license ID, customer alias, SKU, key ID, expiry, grace flag, request ID, status, and reason.

Provider rate limits are recorded as `upstream_rate_limited` and return `503 upstream-rate-limited` only after eligible fallbacks are exhausted. When adaptive backoff is configured, the next requests can skip that provider/model/target and either route around it or return `503 upstream-capacity-throttled` if no alternative is available. Do not paste raw provider error bodies, account IDs, API keys, router tokens, token hashes, prompts, images, or tool outputs into incident notes.

Separate upstream provider access failures from caller authentication and caller quota. Caller-token auth and allow-list failures happen before any provider call. Provider-key, account, region, policy, entitlement, or model-access failures appear in `request_attempts.error_class` as `upstream_auth_failed`, `upstream_access_denied`, `upstream_entitlement_failed`, or `upstream_model_access_denied`. Invalid provider credentials are terminal for that request and should not be fixed by changing caller TPM/RPM. Target-specific provider access failures can route around to the next eligible target without marking the failed attempt retryable; inspect fallback rows to see whether another target recovered the request. If every attempted target fails with provider access classes, callers receive `503 upstream-access-denied` with a sanitized request ID.

Ordinary upstream 4xx malformed-request errors remain non-retryable and stop fallback so the same incompatible caller payload is not replayed to another provider. The router classifies common non-access 4xx cases as `upstream_bad_request`, `upstream_request_too_large`, or generic `upstream_status`; provider 408, quota, credit, billing, rate-limit, timeout, network, and 5xx classes remain retryable when another eligible target exists. Caller responses for terminal upstream failures include safe diagnostics in `error.details.error_class`, `error.details.upstream_status`, `X-Router-Error-Class`, and `X-Upstream-Status`; use those fields to distinguish upstream 400s from router-side `429` policy or traffic-shaping responses. Redirect responses are not followed; investigate the configured provider base URL instead of expecting the router to chase `Location` headers.

If an image-bearing request fails before upstream with `image_url_forbidden`, inspect only the URL class, not the raw image content. The default policy blocks `http`/`https` image URLs that point to or resolve to loopback, link-local, RFC1918/private, multicast, or unspecified addresses, including public URLs that redirect to those destinations. Prefer data URLs or a reviewed public object-store URL; use `server.upstream.allow_private_image_urls: true` only for a private VLM deployment with reviewed egress controls.

### Bad Image Analysis

Separate transport success from task quality. A model can remain active for general VLM routing even if it is not good enough for OCR-specific routing. For OCR routes, require exact-answer image smokes.

### Tool-Calling Failure

Validate the exact client dialect:

- Warp and many OpenAI-compatible agents use `/v1/chat/completions` tool passthrough.
- Codex CLI uses `/v1/responses`.
- Claude Code uses Anthropic Messages, preferably through `/anthropic/v1/messages`; legacy `/v1/messages` remains a compatibility alias.

Codex/OpenAI Responses traffic is eligible only for targets whose resolved provider dialect is `openai-responses` and whose tool metadata includes `tool_support.openai_responses` when tools are present. A group can have many MiniMax, Fireworks, Kimi, or other OpenAI Chat targets and still route Codex to a smaller Responses subset. For MiniMax `MiniMax-M3`, use the dedicated `minimax_responses` provider skin; the `minimax` Chat skin and `minimax_anthropic` Messages skin are separate validation surfaces.

Run the appropriate real tool smoke and assert file contents, not only assistant text.

### One Upstream Takes All Traffic

When a weighted group appears to send all requests for one client to one upstream, first separate configured weight from effective eligibility. Use request rows or provider/model mix to identify the inbound endpoint and selected upstream dialect, then inspect Provider catalog status:

1. Check `groupSummary` counts for the requested group and client surface, such as `openaiResponsesToolTargets` for Codex or `anthropicMessagesToolTargets` for Claude Code.
2. Inspect each active target row's `activeEligibilitySkin`. Native Chat, Responses, and Anthropic skins are separate pools unless a bridge target is explicitly reported.
3. Treat `inactiveToolSupport` as metadata-only. It explains why a catalog row can mention a capability while the active target cannot serve that caller shape.
4. Run a negative smoke for the missing surface and expect `502 no-eligible-target` with zero upstream attempts, or add a restricted smoke group with a validated skin and confirm distribution changes.

Do not raise weights or quotas to fix a one-upstream symptom until the effective target count for the inbound dialect is understood.

### Request-Shape-Specific Upstream 400s

Use this flow when direct provider smokes pass but production agent traffic receives upstream `400` or another non-retryable provider rejection.

1. Confirm the failed request IDs, UTC window, provider, model, and dialect from `request_usage` and `request_attempts`.
2. Compare successful and failed attempts for the same provider/model/dialect using only safe shape fields:

```sql
SELECT
  u.status,
  a.status_code AS upstream_status,
  rs.request_shape_fingerprint,
  rs.tool_schema_fingerprint,
  rs.total_request_bytes_bucket,
  rs.tool_schema_bytes_bucket,
  rs.estimated_input_tokens_bucket,
  rs.requested_output_cap_field,
  rs.requested_output_cap_bucket,
  rs.tool_count,
  rs.tool_choice_mode,
  rs.structured_output_present,
  rs.reasoning_present,
  rs.image_count,
  ts.endpoint_path,
  ts.bridge_direction,
  ts.translated_output_cap_field,
  ts.translated_output_cap_bucket,
  ts.translated_reasoning_control,
  ts.fields_stripped_count,
  ts.fields_rewritten_count,
  ts.unsupported_fields_present,
  COUNT(*) AS requests
FROM request_usage u
JOIN request_attempts a ON a.request_id = u.request_id
LEFT JOIN request_shapes rs ON rs.request_id = u.request_id
LEFT JOIN request_translation_shapes ts
  ON ts.request_id = a.request_id AND ts.attempt_index = a.attempt_index
WHERE u.ts >= :from
  AND u.ts < :to
  AND a.provider = :provider
  AND a.model = :model
  AND a.dialect = :dialect
GROUP BY 1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22
ORDER BY requests DESC;
```

3. If `unsupported_fields_present` is true, inspect `request_translation_field_events` for safe field names and actions:

```sql
SELECT field_name, action, reason, COUNT(*) AS events
FROM request_translation_field_events e
JOIN request_attempts a
  ON a.request_id = e.request_id AND a.attempt_index = e.attempt_index
WHERE a.ts >= :from
  AND a.ts < :to
  AND a.provider = :provider
  AND a.model = :model
  AND a.dialect = :dialect
GROUP BY 1,2,3
ORDER BY events DESC;
```

4. Reproduce with the same safe shape, not the raw content: same dialect, stream flag, tool count, tool-choice mode, structured-output flag, reasoning controls, multimodal presence, output-cap field/bucket, and a similarly sized sanitized payload. Do not copy user prompts, repository contents, images, image URLs, tool schemas, tool outputs, bearer tokens, provider keys, token hashes, or full production config into the reproduction.

## Logs

Recent router logs:

```bash
rtk ssh -i ~/.ssh/chetan-jun-2026.pem ubuntu@100.30.225.66 'cd /opt/smart-llmrouter/compose && sudo docker compose logs --tail=200 router'
```

Prefer DB traces for request-level details because logs should remain sanitized and compact.
