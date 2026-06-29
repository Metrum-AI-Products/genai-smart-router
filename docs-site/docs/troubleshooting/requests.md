---
title: Request Troubleshooting
doc_type: howto
---

# Request Troubleshooting

Every request returns an `X-Request-Id` header. Use that ID to join caller symptoms to logs, usage rows, upstream attempts, trace events, terminal errors, and browser report drilldown. The [Diagnostics Schema](../reference/diagnostics-schema) documents the safe columns available for request-level triage.

## 1. Capture The Caller View

Record these safe fields:

- UTC timestamp;
- request ID;
- HTTP status;
- router error code, if present;
- requested model group;
- API shape, such as Chat Completions, Responses, or Anthropic Messages;
- client name, such as Codex CLI, Claude Code, Cursor, or an internal service;
- whether the request used streaming, tools, images, large input context, or a large output cap.

Do not record raw prompts, image payloads, bearer tokens, provider keys, tool outputs, or full request bodies unless a governed content-capture process is explicitly enabled for the deployment.

## 2. Check Caller Access

```bash
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/v1/models"
```

Expected: the requested model group appears in the response. If it is absent, the caller token is not allowed to use that group or the group is not configured.

Common access outcomes:

| Status | Meaning | Operator action |
|---|---|---|
| `401` | Missing, malformed, expired, or invalid caller token. | Issue or rotate the caller token. |
| `403` | Caller is authenticated but not authorized for the surface or model group. | Review caller access, admin policy, or metrics/report role. |
| `403 metrics-forbidden` | Ordinary caller attempted `/metrics`. | Use a metrics-admin token only for metrics scraping. |
| `403 reports-forbidden` | Ordinary caller attempted admin reports. | Use an authorized admin report identity. |

## 3. Open Request Evidence

With an authorized admin report identity, open the safe evidence bundle:

```bash
curl -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/request-evidence?request_id=<request_id>"
```

The bundle shows what the router safely knew and recorded: caller/project/client labels, requested model group, resolved group, selected target, stored request-time token and cost fields, latency/throughput, quota/key/cache state, traffic-shaping state, target candidate/filter summaries, attempts, sanitized upstream errors, and trace rows when those sections exist.

Use `diagnosticCompleteness` and `evidenceSections` to interpret gaps. `missing` on a failed request means a diagnostic section expected for that phase was not recorded; `not_applicable` means the request path did not reach that phase or the feature was disabled.

Evidence bundles do not expose raw prompts, image URLs or payloads, tool schemas, tool outputs, provider API keys, router bearer tokens, token hashes, full upstream headers, unsanitized upstream bodies, cookies, OIDC tokens, or full config.

## 4. Check Quota And Token Admission

Router-side quota, traffic-shaping, and admission failures usually return `429`. Distinguish them from upstream provider `429` attempts:

- Router hard-limit failures such as `rpm-exceeded`, `tpm-exceeded`, `concurrency-exceeded`, and `quota-exhausted` appear as the terminal caller response before any upstream attempt.
- `traffic-shaped` appears as a terminal caller response with a safe bucket and `Retry-After` when a configured caller/server shaping bucket limits the burst.
- Upstream `429` attempts may be followed by fallback to another target.
- Large-context developer tools can exhaust TPM through in-flight reservations even when daily or monthly budget remains available.

Use usage reports or admin browser troubleshooting buckets for quota, TPM/RPM, concurrency, traffic-shaping bucket, input-token, and max-token signals. For exact field names and retention classes, see the [Diagnostics Schema](../reference/diagnostics-schema).

Use Traffic tuning advisor when the question is whether to increase burst, change queueing, slow a caller, tune provider capacity, or route around an incompatible target:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --traffic-tuning-advisor \
  --caller-user <owner-user>
```

Examples:

- User sees errors but all shaping buckets were admitted or absent: treat `route_around_incompatible_target` as a request-shape/provider compatibility issue. Inspect upstream failures and request-shape failures instead of increasing burst or queue depth.
- User is being queued and cancellations increased: treat `disable_queue_for_latency_sensitive_client` as a signal to lower queue wait or fail fast for that client.
- Provider 429s affect multiple users: treat `investigate_provider_429_capacity` as shared capacity or entitlement work. Tune provider/model shaping, adaptive backoff, route weights, or upstream account limits before increasing one caller's burst.
- Large Cursor, Codex, Claude Code, or opencode payloads fail on selected upstreams: compare request-shape failure buckets, output-cap buckets, tool/modality metadata, and provider/model/dialect rows, then route around targets that cannot handle that shape.

## 5. Check Upstream Attempts

For a slow or failed request, inspect:

- selected provider/model/dialect;
- upstream status code;
- upstream duration and TTFB;
- timeout or cancellation flags;
- retryability;
- fallback transitions;
- sanitized terminal error class.

If only one provider/model/dialect is failing, isolate that upstream before changing the broader model group. If all targets are failing, inspect shared config, network, license, database, or caller request shape.

## 6. Check Request Shape

Common request-shape causes:

- requested model group does not support the API skin used by the client;
- tool calls are sent to a target without validated tool support;
- image input is sent to a text-only target;
- estimated input plus output cap exceeds target context limits;
- request bytes or tool schema bytes exceed configured target request-shape limits;
- a forced tool-choice shape is unsupported by the selected upstream;
- streaming behavior differs from the caller expectation.

Model-group contracts and provider catalog metadata should describe validated modalities, tools, dialects, pricing, and max-token behavior.

For “small requests work but large Cursor/Codex/Claude Code requests fail,” inspect the request drilldown or usage DB rows for `request_token_estimates`, `request_target_candidates`, and `request_target_filter_reasons`. Safe fields to compare are estimated total input tokens, requested output cap, total reserved tokens, request bytes, target `context_tokens`, context headroom, `request_bytes_fit`, `tool_schema_fit`, and bounded reasons such as `request-shape-context-exceeded`, `request-shape-max-request-bytes`, or `request-shape-tool-schema-bytes`. These diagnostics intentionally do not contain raw prompts, raw tool schemas, images, bearer tokens, token hashes, provider keys, or full config.

If every target is skipped, callers receive `502 no-eligible-target` before upstream with a request ID. Recovery is usually a config change: add accurate `context_tokens` or `request_shape_support`, remove a too-small target from the affected group, or keep the target in a smoke group until a large-payload validation passes.

## 7. Verify Recovery

After a config, credential, quota, or upstream fix:

```bash
curl -fsS "$ROUTER_BASE_URL/readyz"

curl -fsS "$ROUTER_BASE_URL/v1/chat/completions" \
  -H "Authorization: Bearer $ROUTER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "replace-with-allowed-model-group",
    "messages": [{"role": "user", "content": "Reply OK only."}],
    "max_tokens": 16
  }'
```

Then confirm the request appears in usage reports with the expected provider/model, status, latency, cost, and fallback state.
