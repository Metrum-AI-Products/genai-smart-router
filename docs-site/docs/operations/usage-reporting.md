---
title: Usage Reporting
---

# Usage Reporting

GenAI Smart Router records durable usage data for cost management, auditability, and model-group evaluation.

`router-usage-report` is an Enterprise Edition administrative CLI. It is intended for platform administrators and is run from a secure server console, deployment host shell, or controlled admin workstation with access to the usage database. It is not exposed through the public browser documentation site as an interactive tool.

<div class="contactBanner">
  <p>For dashboards, reports, or evaluation design, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Generate A Markdown Report

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out usage-24h.md
```

## Report Dimensions

Reports include:

- Calls, errors, status codes, latency, and upstream attempts.
- Input tokens, output tokens, total tokens, and throughput.
- Request-time input/output token prices and calculated input/output/total USD cost.
- Image/VLM fields including image presence, image count, upstream image-token counts when reported, calculated image input cost, and upstream-reported billed cost when available.
- Usage by public router token ID, user, project, and environment.
- Usage by caller IP and hour.
- Usage by router model group.
- Usage by external provider and model.
- Cache hits, misses, bypasses, occupancy, and hit rate.
- Streaming and non-streaming request counts.
- Request IDs that can be joined to diagnostic attempt, trace-event, and terminal-error rows by administrators.

## Troubleshooting By Request ID

Every response includes `X-Request-Id`. Structured error responses also include `request_id` in the error details. Administrators can use that ID to inspect:

- `request_usage` for the terminal request status, selected target, token counts, and cost fields.
- `request_attempts` for each upstream provider/model attempt, status code, duration, timeout/cancel flags, retryability, and sanitized error class/message.
- `request_trace_events` for ordered router decisions such as cache handling, upstream attempts, fallback, timeout, or terminal failure.
- `request_errors` for the terminal sanitized error summary.

Diagnostic rows do not store raw prompts, image payloads, bearer tokens, provider keys, token hashes, full upstream headers, or unsanitized upstream response bodies.

## Filtering

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --caller-project harbor-algotune-pca \
  --resolved-group <model-group> \
  --client codex \
  --out usage-codex.md
```

Reports use public token IDs and aggregated usage fields. They do not expose raw router tokens or raw provider API keys.

Cost fields are captured when each request finishes. Reports do not look up current provider pricing, which means a June report keeps the June price even if an upstream vendor changes rates in July. Operators should update provider catalog metadata whenever prices, modality support, or tool-capability validation changes.

For image requests, `input_price_per_million_usd` remains the fallback input-token rate. If a VLM has separate image pricing, configure `image_input_price_per_million_tokens_usd` for upstream-reported image tokens or `image_input_price_per_image_usd` for fixed per-image chargeback. When an upstream returns billed cost, the router stores those values as upstream-reported cost fields in addition to router-calculated cost fields.

For a buyer-facing explanation of cost policy and chargeback, see [Cost Governance](/docs/evaluation/cost-governance).
