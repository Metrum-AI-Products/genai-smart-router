---
title: Usage Reporting
---

# Usage Reporting

GenAI Smart Router records durable usage data for cost management, auditability, and model-group validation.

`router-usage-report` is an Enterprise Edition administrative CLI. It is intended for platform administrators and is run from a secure server console, deployment host shell, or controlled admin workstation with access to the usage database. Deployments may also enable the authenticated browser reporting surface at `/admin/reports/`; it is separate from public `/docs/` and requires browser-admin authentication plus Casbin authorization.

<div class="contactBanner">
  <p>For dashboards, reports, or validation design, contact <a href="mailto:contact@metrum.ai">contact@metrum.ai</a>.</p>
</div>

## Generate A Markdown Report

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out usage-24h.md
```

Generated reports are Markdown files with structured tables for usage, cost, latency, throughput, downstream caller performance, and upstream endpoint performance. The public docs include graphical Chart.js examples built from the same report dimensions.

## Browser Admin Reports

When `server.admin_reports.enabled: true`, administrators with an authorized Basic Auth subject or OIDC session subject can open `/admin/reports/` to inspect the same operational dimensions through embedded browser assets. The router serves the HTML, CSS, JavaScript, and local chart bundle from the binary; no CDN is required. Report pages and APIs use no-store cache headers, conservative CSP, bounded time ranges, and Casbin policy checks for every page, API, export, and drilldown route.

Example policy shape:

```yaml
server:
  admin_auth:
    authorization:
      enabled: true
      source: static
      policy:
        - g, basic:admin, reports_admin, example/prod
        - g, user:alice@example.com, reports_admin, example/prod
        - p, reports_admin, example/prod, admin:reports, read|export
  admin_reports:
    enabled: true
    default_since: 24h
    max_range: 31d
    max_rows: 500
```

Common endpoints:

- `/admin/reports/` renders the browser shell.
- `/admin/reports/api/summary?since=24h` returns totals, charts, grouped tables, and bounded request rows.
- `/admin/reports/api/requests?since=24h&limit=100` returns recent safe request rows.
- `/admin/reports/api/request/<request_id>` joins safe usage, attempt, trace, and terminal error rows.
- `/admin/reports/export.md?since=24h` returns the Markdown report used by the CLI renderer.

## Report Dimensions

Reports include:

- Calls, errors, status codes, latency, and upstream attempts.
- Input tokens, output tokens, total tokens, and throughput.
- Downstream user performance grouped by user, project, environment, and client, including average/max latency, TTFB, downstream duration, and downstream token throughput.
- Upstream endpoint performance grouped by provider, model, and API dialect, including average/max upstream duration, latency, TTFB, attempts, fallbacks, cost, and upstream token throughput.
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

Governed content capture is separate from diagnostics. It is disabled by default and, when enabled by the deployment operator, writes redacted request/response/upstream-error content to dedicated relational tables keyed by `request_id`. Maintenance operations require Casbin authorization for `content:capture`: `DELETE /v1/content-captures/<request_id>` uses action `delete`, and `POST /v1/content-captures/purge-expired` uses action `purge`. Existing `content_admin: true` caller entries remain compatible. Both operations write audit rows. Usage reports remain metadata-oriented and do not print captured content.

## Filtering

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --caller-user <owner-user> \
  --caller-project <project> \
  --resolved-group <model-group> \
  --client <client> \
  --out usage-filtered.md
```

Reports use public token IDs and aggregated usage fields. They do not expose raw router tokens or raw provider API keys.

## Performance Triage

Use the downstream user performance section to identify which users, projects, or clients are seeing slow responses. Use the upstream endpoint performance section to identify provider/model/dialect combinations with high upstream duration, low token throughput, elevated errors, or fallback pressure. The per-request throughput table remains available for request-level drilldown when a grouped row needs investigation.

Cost fields are captured when each request finishes. Reports do not look up current provider pricing, which means a June report keeps the June price even if an upstream vendor changes rates in July. Operators should update provider catalog metadata whenever prices, modality support, or tool-capability validation changes.

For image requests, `input_price_per_million_usd` remains the fallback input-token rate. If a VLM has separate image pricing, configure `image_input_price_per_million_tokens_usd` for upstream-reported image tokens or `image_input_price_per_image_usd` for fixed per-image chargeback. When an upstream returns billed cost, the router stores those values as upstream-reported cost fields in addition to router-calculated cost fields.

For a buyer-facing explanation of cost policy and chargeback, see [Cost Governance](/docs/evaluation/cost-governance).

For anonymized graphical examples generated from production-style data, see [Report Examples](/docs/operations/report-examples).
