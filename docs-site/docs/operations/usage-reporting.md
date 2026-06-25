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

When `server.admin_reports.enabled: true`, administrators with an authorized Basic Auth subject or OIDC session subject can open `/admin/reports/` to inspect the same operational dimensions through a Metrum-branded browser dashboard. The router serves the HTML, CSS, JavaScript, Metrum logo, fonts, and local chart bundle from the binary; no CDN or external brand-asset host is required. Report pages and APIs use no-store cache headers, conservative CSP, bounded time ranges, and Casbin policy checks for every page, API, export, and drilldown route.

The dashboard includes a dark/light mode toggle. The preference is stored in browser `localStorage`, and first visits follow the browser's system color-scheme preference.

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
    baselines:
      - id: gpt-5.5
        name: GPT-5.5
        input_price_per_million_usd: 5.00
        output_price_per_million_usd: 30.00
        pricing_source: https://developers.openai.com/api/docs/pricing
        pricing_updated_at: "2026-06-25"
      - id: claude-opus-4.8
        name: Claude Opus 4.8
        input_price_per_million_usd: 5.00
        output_price_per_million_usd: 25.00
        pricing_source: https://docs.anthropic.com/en/docs/about-claude/pricing
        pricing_updated_at: "2026-06-25"
```

Common endpoints:

- `/admin/reports/` renders the browser shell.
- `/admin/reports/api/summary?since=24h` returns totals, grouped tables, bounded request rows, and a reusable `charts` contract with chart IDs, titles, axis labels/types/units, series names, semantic color keys, scalar points, generation timestamp, range, and active safe filters.
- `/admin/reports/api/savings?since=24h&baseline=gpt-5.5` returns actual cost, selected baseline cost, savings USD, savings percent, time buckets, model-group breakdowns, source-dated baseline metadata, and chart descriptors.
- `/admin/reports/api/<report-name>?since=24h` returns shared scalar report rows and chart descriptors for overview, savings by user/key/group, model groups by user, usage by key, provider/model mix, latency/throughput, errors/fallbacks, cache, quotas/budgets, routing decisions, expensive requests, client breakdown, project chargeback, capability usage, and anomaly signals.
- `/admin/reports/api/requests?since=24h&limit=100` returns recent safe request rows.
- `/admin/reports/api/request/<request_id>` joins safe usage, attempt, trace, and terminal error rows.
- `/admin/reports/export.md?since=24h` returns the Markdown report used by the CLI renderer.

The embedded browser renderer uses the chart contract for axes, legends, unit-aware tick labels, and hover tooltips. Chart points are scalar aggregate values only and are backed by the same safe report fields exposed in tables and exports.

Savings reports use stored request-time actual cost fields for actual spend. Only the hypothetical baseline cost is calculated at report time from stored input/output token counts and selected baseline prices. Built-in baseline prices are source-dated in `server.admin_reports.baselines`; revalidate provider pricing before using savings figures in contractual or customer-facing claims. Custom browser-session baselines can be supplied with `baseline=custom`, `baseline_input_price_per_million_usd`, and `baseline_output_price_per_million_usd`.

The browser shell adds shared usability controls across tabs: URL-backed selected tab and search state, visible-table search, sortable headers, bounded page-size selection, refresh, copy-link, copy-field buttons, request-ID drilldown, and CSV export of visible safe scalar columns. Server endpoints remain authenticated and bounded; the browser controls do not expose or persist bearer tokens.

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
