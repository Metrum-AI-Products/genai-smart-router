---
title: Admin Browser Reports
---

# Admin Browser Reports

Admin browser reports are an authenticated operational surface for usage, performance, cost, cache, fallback, and diagnostic drilldown. They are disabled by default and served separately from public `/docs/`. The browser dashboard uses Metrum branding, local embedded assets, a dark operational theme, and an admin-only build-version chip for authorized administrators.

For commercial evaluations, this surface is a proof point as well as an operations tool. It lets evaluators inspect whether the router actually reduced cost, preserved workload outcomes, isolated access, explained provider/model choices, and produced enough evidence for chargeback, quota tuning, support triage, and security review.

For health checks, metrics, log dimensions, and alerting, see [Observability](./observability). For incident triage by request ID, see [Request Troubleshooting](../troubleshooting/requests).

## Access Model

Browser identity can be HTTP Basic under `server.admin_auth.basic` or OIDC sessions under `server.admin_auth.oidc`. Authorization is Casbin-backed under `server.admin_auth.authorization`; every `/admin/reports/*` page, API, export, and drilldown route requires an allow decision for object `admin:reports`. Aggregate pages/APIs use action `read`, Markdown export uses `export`, and request detail uses `drilldown`. Security access report APIs additionally require `admin:security_reports` so access metadata can be restricted more tightly than cost and performance reports.

```yaml
server:
  admin_auth:
    basic:
      enabled: true
      users:
        - username: admin
          password_hash_env: SMART_ROUTER_ADMIN_PASSWORD_HASH
          subject: basic:admin
          domain: example/prod
    authorization:
      enabled: true
      source: static
      policy:
        - g, basic:admin, reports_admin, example/prod
        - g, user:alice@example.com, reports_admin, example/prod
        - p, reports_admin, example/prod, admin:reports, read|export|drilldown
        - p, reports_admin, example/prod, admin:security_reports, read|export
  admin_reports:
    enabled: true
    path_prefix: /admin/reports
    default_since: 24h
    max_range: 31d
    max_rows: 500
    export_markdown: true
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
    security:
      enabled: true
      retention_days: 90
  client_ip:
    trusted_proxy_cidrs:
      - 10.0.0.0/8
    header_order:
      - X-Forwarded-For
      - X-Real-IP
    store_ip: true
```

Ordinary router caller tokens receive `403 reports-forbidden`. Missing or invalid Basic credentials or missing/invalid OIDC sessions receive `401`.

## What It Shows

The browser UI displays requests, errors, input tokens, output tokens, total tokens, input cost, output cost, total cost, savings, latency, TTFB, upstream output/total throughput, downstream write output/total throughput, cache hit/miss/bypass, attempts, fallbacks, provider/model/dialect groups, requested model, model-group usage by user, public token IDs, caller ID/user/project/environment, caller IP when stored, quota/key states, traffic-shaping decisions, routing strategy summaries, dynamic-score signal/score/threshold buckets, max-token and input-token buckets, admission reasons, policy execution outcomes/errors, fallback transition reasons, contract buckets, target validation buckets, capability usage, troubleshooting buckets, anomaly signals, status codes, expensive requests, client breakdowns, project chargeback, provider catalog/validation status, retention/rollup status, and recent safe request rows. Request drilldown joins the relational usage, attempt, trace-event, terminal-error, traffic-shaping, and decision-telemetry rows by request ID.

Responses do not include raw router tokens, token hashes, provider keys, raw prompts, raw images, raw tool outputs, full config values, or unsanitized upstream bodies.

## Buyer And Operator Questions

| Question | Report evidence |
|---|---|
| Which teams are driving spend or savings? | Savings by user, project, key, group, and provider/model; project chargeback; client breakdown. |
| Which model groups are used by each cohort? | Model groups by user/project/key and usage by requested model group. |
| Which providers are actually serving traffic? | Provider/model mix, active target metadata, validation status, attempts, fallbacks, and errors. |
| Are quotas and rate limits sized correctly? | Quotas/budgets, troubleshooting buckets, traffic-shaping buckets, max-token and input-token buckets, TPM/RPM/concurrency signals. |
| Why was a request expensive or slow? | Expensive requests, request drilldown, downstream user performance, upstream endpoint performance, latency and throughput. |
| Is access governed? | Security access events, ordinary-caller `403 reports-forbidden`, metrics-admin isolation, public token IDs, key state, and caller/project dimensions. |

## Security Access Reports

When `server.admin_reports.security.enabled: true`, the router persists safe scalar access events in the usage database for authorized API calls, missing or invalid caller-token attempts, caller authorization failures, model access denials, metrics/report/content authorization failures, Basic admin auth checks, and admin report reads/exports. The Security tab shows event outcome, reason, surface, safe caller/admin identity, public token ID, client, trusted-proxy-derived IP metadata, and input/output/total token counts where a completed model request reported usage.

Security reports use the same browser shell and chart contract as usage reports, but their API routes require `admin:security_reports` `read` or `export`. CSV export is available at `/admin/reports/security/export.csv` and includes only safe scalar fields with spreadsheet formula-leading values neutralized.

Report APIs are scoped to the authenticated admin's Casbin domain by default. A subject authorized in `example/prod` sees usage rows, request lists, request detail, Markdown export, and security events for caller project `example` and environment `prod`; cross-domain request IDs return `404`. Deployment-wide report administrators require an explicit `*` policy domain.

Configure `server.client_ip.trusted_proxy_cidrs` before relying on IP-based security triage. The router ignores `X-Forwarded-For` and `X-Real-IP` unless the direct remote address is in a trusted proxy CIDR. If no trusted proxy matches, reports use the direct remote address. Set `store_ip: false` only when deployment policy forbids IP storage; the report will then omit IP addresses and keep source/classification metadata best-effort.

## Shared Usability

The browser report shell provides shared controls for every tab:

- grouped left-sidebar navigation on desktop, with a header drawer on narrow screens;
- collapsible navigation groups with preferences stored in browser `localStorage`;
- a two-row header with the Metrum brand, title, and safe version chips on top, followed by a full-width global filter bar with the `Since` time range, Markdown export, mobile `Sections` drawer, and an accessible `Filters` disclosure for global investigation filters;
- global filters for caller ID, caller user, public token ID, caller IP, caller project, caller environment, requested model, resolved group, provider, target model, dialect, and client; these remain active while switching tabs;
- per-tab filter panels for tab-local controls such as baseline, status, cache state, sort, direction, and traffic-shaping bucket/scope; each report shows only the controls that apply to that tab;
- a `Rows` select in the table toolbar for the URL-backed server row limit, alongside a separate client-side page-size select for visible rows;
- selected tab, active filters, and search stored in shareable URL query parameters;
- client-side search across visible safe scalar fields;
- sortable table headers with click-to-sort controls and active ▲/▼ indicators;
- bounded page-size selection;
- manual refresh with last-refresh state;
- copy buttons for identifiers such as public token IDs, groups, providers, and request IDs;
- request-ID drilldown from request rows;
- CSV export of the visible table data with spreadsheet formula-leading values neutralized;
- consistent chart, table, loading, empty, and error states.

These controls are presentation helpers over bounded authenticated APIs. They do not expose raw tokens, token hashes, provider keys, prompts, images, tool outputs, raw cookies, OIDC tokens, full config, raw spreadsheet formulas, or unsanitized upstream responses.

Tables use per-tab column schemas instead of first-row key discovery. Column order, labels, and units are stable for each tab, CSV export follows the same visible columns, Markdown export escapes raw HTML and active Markdown table-cell syntax, and duplicate compatibility aliases are suppressed when they carry the same value. For example, usage tabs show `Input tokens`, `Output tokens`, and `Total tokens`; they do not show both `tokens` and `totalTokens` when those fields are equivalent. Cost fields follow the same rule: input, image, output, total, baseline, savings, and upstream-billed values are labeled separately when present.

## API Pagination

Admin report API responses include a `pagination` object. Raw, event-like reports use cursor pagination when the router can page directly from indexed relational rows:

- `/admin/reports/api/requests`
- `/admin/reports/api/expensive-requests`
- `/admin/reports/api/security/events`

Example first page:

```text
/admin/reports/api/requests?since=24h&limit=50&sort=timeUtc&direction=desc
```

Example next page:

```text
/admin/reports/api/requests?since=24h&limit=50&sort=timeUtc&direction=desc&cursor=<next_cursor>
```

The response shape is:

```json
{
  "pagination": {
    "limit": 50,
    "returned": 50,
    "total_count": 1234,
    "has_more": true,
    "next_cursor": "opaque",
    "sort": "timeUtc",
    "direction": "desc",
    "mode": "cursor"
  }
}
```

Request sort keys are `timeUtc`, `costUsd`, `latencyMs`, `status`, and `requestId`. Security-event sort keys are `timeUtc`, `status`, `outcome`, `surface`, and `reason`. `limit` must be positive and no larger than `server.admin_reports.max_rows`; `direction` must be `asc` or `desc`. Cursors are opaque, signed, and bound to the endpoint, sort, and direction. Malformed, tampered, stale, or mismatched cursors return `400 invalid-report-filter`. Cursor pagination is domain-scoped the same way as the first page, so a subject authorized for `example/prod` cannot page into another project/environment.

Aggregate tabs such as usage by key, provider/model mix, savings by user, traffic-shaping summaries, and routing-decision buckets remain ranked top-N summaries. Their metadata uses:

```json
{
  "pagination": {
    "limit": 50,
    "returned": 50,
    "total_count": null,
    "has_more": true,
    "sort": "requests",
    "direction": "desc",
    "mode": "top_n",
    "note": "Aggregate rows are top-N for the selected filters."
  }
}
```

For those aggregate reports, browser search and table sorting operate over the returned top-N rows. Use the aggregate tabs to identify a dimension, then drill into `/admin/reports/api/requests` or `/admin/reports/api/security/events` with matching filters when you need stable page-by-page review.

CSV export from the browser exports the currently visible table columns. Markdown export is bounded by the selected report filters and remains an operational report export, not an unbounded full-history job.

## Navigation

Desktop report users navigate with a fixed left sidebar labeled `Report sections`. The sidebar groups reports by operator intent: Overview, Usage, Savings, Performance, Traffic shaping, Routing decisions, Provider catalog, Security, Request drilldown, and System status. The report header stacks the brand row above the global filter row so filters use the same full content width as the reports below. Filters, Markdown export, active tab URL, CSV export, and request drilldown stay in the main content area.

Each group header is keyboard-focusable and exposes expanded/collapsed state to assistive technology. Collapsed groups are remembered in browser `localStorage` under a versioned UI key so an administrator's browser keeps the same sidebar density after reloads. The preference is local presentation state only; it is not sent to report APIs, stored in the router, or included in shareable URLs.

On narrow screens the sidebar is hidden by default and opens from the `Sections` button in the header. The drawer uses the same grouped navigation and preserves deep links such as `/admin/reports/?tab=requests&since=24h`. The URL `tab` parameter remains the source of truth for the active report, and global or tab-specific filters continue to use one combined URL such as `/admin/reports/?tab=savings-by-key&since=6d&caller_user=alice&baseline=gpt-5.5`, so bookmarked links, filters, exports, and Casbin authorization behavior are unchanged.

Per-tab panels are mounted above the metrics, charts, and table. Savings tabs expose `Baseline`, `Sort`, and `Direction`; cache/error/performance tabs expose `Status`, `Cache`, `Sort`, and `Direction`; traffic-shaping tabs expose `Shape bucket`, `Shape scope`, `Sort`, and `Direction`; and diagnostic/status tabs expose sorting controls where a natural table sort applies. `Reset filters` clears only the active tab's local filters and never clears global investigation filters such as caller user or provider.

## Report Tabs

The current browser surface includes these grouped reports:

- Overview: high-level usage, cost, latency, cache, fallback, and provider trends.
- Usage: model groups, providers, API keys, model groups by user, key usage, caller usage, requested models, provider/model mix, clients, projects, and capability usage. The Provider/model tab reports actual provider/model usage, input/output/total tokens, input/output/total cost, latency, and throughput. It does not include baseline or savings fields by default; use the Savings tabs when a hypothetical baseline comparison is needed.
- Savings: actual request-time cost compared with selected source-dated baseline prices, plus savings by user, key, model group, project, and provider/model.
- Performance: latency and throughput, errors and fallbacks, and cache.
- Traffic shaping: overview, by user, by key, by client, by model group, provider capacity shaping, and adaptive upstream backoff. These tabs distinguish caller/server shaping decisions such as `rejected` and `queued`, show limiting scope/bucket, retry-after, queue wait, estimated input tokens, reserved output tokens, and total reserved tokens. Provider shaping and backoff tabs show provider/model/target admission, skipped targets, cooldown starts, upstream 429/quota backoff reasons, and successful route-around counts.
- Routing decisions: routing decisions, dynamic-score enabled signals, score buckets, threshold buckets, max-token buckets, input-token buckets, admission reasons, and troubleshooting buckets for quota, TPM/RPM or rate-limit, concurrency, max-token/context, upstream quota/billing, key-state, cache, fallback, multi-attempt, and HTTP error classes inferred from safe stored request fields.

Example shaping URLs:

```text
/admin/reports/?tab=traffic-shaping-overview&since=24h&traffic_shape_scope=caller
/admin/reports/?tab=provider-capacity-shaping&since=24h&provider=openai
/admin/reports/?tab=adaptive-upstream-backoff&since=24h
```

Use caller shaping tabs when the caller received `429 traffic-shaped` or had queued requests. Use provider capacity shaping when the caller received `503 upstream-capacity-throttled` or when a target was skipped and another target succeeded. Use adaptive backoff when a prior upstream `429` or quota/billing response should temporarily protect that provider/model/target.
- Provider catalog: provider catalog status from safe runtime configuration metadata, target validation, contract buckets, and contract workloads. Catalog status separates `catalog` rows from `active_target` rows so per-group target overrides for modalities, tools, pricing, max-token behavior, and validation are visible without changing catalog metadata. This endpoint does not expose provider API keys, headers, full config, or private deployment files. Contract reports show contract-present/pass/fail, failure reasons, and deployment-defined workload labels without exposing request content.
- Security: security access events for authorized and unauthorized access paths when enabled.
- Request drilldown: expensive requests, recent requests and request-ID drilldown, and deterministic anomalies such as errors, fallbacks, multi-attempt requests, slow requests, expensive requests, non-ok quota states, and abnormal key states such as disabled, revoked, expired, or suspended. This is not machine-learning anomaly detection; normal active key state is not anomalous, and baseline/savings fields are reserved for savings reports.
- System status: quotas/budgets plus retention and rollup status from existing usage DB status tables, including the latest retention job, per-table candidate/held/eligible/blocked/deleted counts, and recent hourly/daily/monthly rollup runs. Retention status is read-only; retention execution and rollup generation remain operator-controlled workflows.

Recent request rows include visible columns for time, request ID, caller ID, caller IP, public token ID, caller user/project/environment, client, requested model, resolved model group, provider, model, dialect, status, cache state, attempts, fallback flag, latency, input/output/total tokens, and stored total cost.

## Charts

Report API responses include a `charts` array with stable chart IDs, titles, X/Y axis labels, axis types, units, series names, semantic color keys, scalar points, generation timestamp, selected range when applicable, and active safe filters when applicable. Every browser report tab renders at least one aggregate chart when matching data exists. If a selected range or filter has no aggregate points, the tab shows an explicit no-chart-data state before the table empty state. Money is displayed as USD, token and request counts use compact notation where appropriate, latency uses milliseconds or seconds, throughput uses `tok/s`, and rates use percentages. Tables label totals explicitly as Total tokens and Total cost whenever input/output breakdowns are present.

Every chart is backed by the same safe aggregate fields shown in tables and Markdown export. The catalog-status tab charts source, validation, and active-target provider counts. The retention-status tab charts retention eligibility/deletion counts and recent rollup status/type counts. Chart payloads contain scalar aggregate points only; they do not include prompts, image payloads, tool schemas or outputs, tokens, token hashes, provider keys, full config, or raw upstream bodies.

Docusaurus product docs may show anonymized Chart.js examples built from safe report fixtures. The `router-usage-report` CLI remains focused on stable Markdown tables, relational rollups, and machine-reviewable metrics unless a deployment explicitly adds a chart export workflow outside the router binary.

## Savings

The Savings tab compares stored actual request cost against a selected hypothetical baseline. Actual cost is always summed from request-time stored cost fields; it is not recalculated from current provider configuration. Baseline cost is calculated from stored input and output token counts:

```text
baseline_cost_usd =
  input_tokens / 1_000_000 * baseline_input_price_per_million_usd +
  output_tokens / 1_000_000 * baseline_output_price_per_million_usd
```

Built-in baselines are configured under `server.admin_reports.baselines` with source URL, source date, input USD/M, output USD/M, and notes. The default built-ins were source-checked on June 25, 2026: GPT-5.5 from OpenAI API pricing at $5.00/M input and $30.00/M output, and Claude Opus 4.8 from Anthropic Claude pricing at $5.00/M input and $25.00/M output. Baseline prices are externally maintained by providers and should be revalidated when producing contractual or customer-facing savings claims.

Administrators can also enter a custom baseline for the current browser session. Custom values are validated as finite nonnegative USD-per-million-token rates and are not persisted by the router.

Actual router cost is summed from stored request-time input, output, image, calculated, and upstream-reported billed cost fields. Reports must not reprice historical actuals from current config. Older rows that predate a cost field can still be counted for usage, latency, or token volume, but savings and chargeback views should label the missing cost coverage instead of treating it as zero spend.

## Version Status

The browser shell calls `/admin/reports/api/version` after load and displays a compact version chip with the router version, build date, and license compile mode when available. The endpoint uses the same admin reports authentication, license, and Casbin `admin:reports` `read` authorization as aggregate report APIs. It does not require a usage database and does not expose raw router tokens, token hashes, provider keys, full config, prompts, images, tool outputs, cookies, or OIDC tokens.

Use it as a quick operator check that the loaded browser bundle is talking to the expected router binary. The public `/version` endpoint remains available for deployment health checks; `/admin/reports/api/version` exists so authenticated report users can see build metadata without leaving the admin report surface.

## Embedded Assets

The admin HTML, CSS, JavaScript, Metrum logo, fonts, and local chart bundle are embedded in the router binary. The UI does not depend on external CDNs or runtime access to the Metrum website. Charts are assistive; the same data is available in tables and Markdown export.

Admin pages and APIs send no-store cache headers. Static admin assets may use private cache headers and contain no report data.

The admin report shell uses a dark operational theme aligned with the embedded Metrum assets. Theme presentation does not change server-side reporting data or authorization policy.

## Smoke Test

```bash
curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/requests?since=24h&limit=50&caller_id=example-caller&provider=openrouter&status=200&cache=miss&sort=timeUtc&direction=desc"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/provider-catalog-status"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/retention-status"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/version"
```

Expected for an authorized subject: `200` JSON with `summary`, `series`, `charts`, grouped tables, request rows where applicable, and `pagination` metadata. Request and security-event APIs return `mode: "cursor"` and `next_cursor` when another page exists. Aggregate APIs return `mode: "top_n"`. The catalog-status and retention-status responses also include `charts` so those tabs are not table-only. The version endpoint returns safe build fields such as `version`, `build_date`, runtime platform, and `license_compile_mode`.

OIDC deployments should first complete `/admin/auth/login`, then call the same report URL with the browser session cookie. A valid OIDC session without Casbin policy receives `403 reports-forbidden`.

Expected for an ordinary router token:

```bash
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Response: `403 reports-forbidden`.
