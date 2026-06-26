---
title: Admin Browser Reports
---

# Admin Browser Reports

Admin browser reports are an authenticated operational surface for usage, performance, cost, cache, fallback, and diagnostic drilldown. They are disabled by default and served separately from public `/docs/`. The browser dashboard uses Metrum branding, local embedded assets, and a dark/light mode toggle for authorized administrators.

For commercial evaluations, this surface is a proof point as well as an operations tool. It lets evaluators inspect whether the router actually reduced cost, preserved workload outcomes, isolated access, explained provider/model choices, and produced enough evidence for chargeback, quota tuning, support triage, and security review.

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

The browser UI displays requests, errors, input tokens, output tokens, total tokens, input cost, output cost, total cost, savings, latency, TTFB, upstream output/total throughput, downstream write output/total throughput, cache hit/miss/bypass, attempts, fallbacks, provider/model/dialect groups, requested model, model-group usage by user, public token IDs, caller ID/user/project/environment, caller IP when stored, quota/key states, routing strategy summaries, dynamic-score signal/score/threshold buckets, max-token and input-token buckets, admission reasons, policy execution outcomes/errors, fallback transition reasons, contract buckets, target validation buckets, capability usage, troubleshooting buckets, anomaly signals, status codes, expensive requests, client breakdowns, project chargeback, provider catalog/validation status, retention/rollup status, and recent safe request rows. Request drilldown joins the relational usage, attempt, trace-event, terminal-error, and decision-telemetry rows by request ID.

Responses do not include raw router tokens, token hashes, provider keys, raw prompts, raw images, raw tool outputs, full config values, or unsanitized upstream bodies.

## Buyer And Operator Questions

| Question | Report evidence |
|---|---|
| Which teams are driving spend or savings? | Savings by user, project, key, group, and provider/model; project chargeback; client breakdown. |
| Which model groups are used by each cohort? | Model groups by user/project/key and usage by requested model group. |
| Which providers are actually serving traffic? | Provider/model mix, active target metadata, validation status, attempts, fallbacks, and errors. |
| Are quotas and rate limits sized correctly? | Quotas/budgets, troubleshooting buckets, max-token and input-token buckets, TPM/RPM/concurrency signals. |
| Why was a request expensive or slow? | Expensive requests, request drilldown, downstream user performance, upstream endpoint performance, latency and throughput. |
| Is access governed? | Security access events, ordinary-caller `403 reports-forbidden`, metrics-admin isolation, public token IDs, key state, and caller/project dimensions. |

## Security Access Reports

When `server.admin_reports.security.enabled: true`, the router persists safe scalar access events in the usage database for authorized API calls, missing or invalid caller-token attempts, caller authorization failures, model access denials, metrics/report/content authorization failures, Basic admin auth checks, and admin report reads/exports. The Security tab shows event outcome, reason, surface, safe caller/admin identity, public token ID, client, trusted-proxy-derived IP metadata, and input/output/total token counts where a completed model request reported usage.

Security reports use the same browser shell and chart contract as usage reports, but their API routes require `admin:security_reports` `read` or `export`. CSV export is available at `/admin/reports/security/export.csv` and includes only safe scalar fields.

Configure `server.client_ip.trusted_proxy_cidrs` before relying on IP-based security triage. The router ignores `X-Forwarded-For` and `X-Real-IP` unless the direct remote address is in a trusted proxy CIDR. If no trusted proxy matches, reports use the direct remote address. Set `store_ip: false` only when deployment policy forbids IP storage; the report will then omit IP addresses and keep source/classification metadata best-effort.

## Shared Usability

The browser report shell provides shared controls for every tab:

- time range filters from the top filter bar;
- filters for caller ID, caller IP, project, requested model, resolved group, provider, target model, dialect, HTTP status, cache state, and client;
- selected tab and search stored in shareable URL query parameters;
- client-side search across visible safe scalar fields;
- sortable table headers;
- bounded page-size selection;
- manual refresh with last-refresh state;
- copy buttons for identifiers such as public token IDs, groups, providers, and request IDs;
- request-ID drilldown from request rows;
- CSV export of the visible table data;
- consistent loading, empty, and error states.

These controls are presentation helpers over bounded authenticated APIs. They do not expose raw tokens, token hashes, provider keys, prompts, images, tool outputs, raw cookies, OIDC tokens, full config, or unsanitized upstream responses.

## Report Tabs

The current browser surface includes:

- Overview: high-level usage, cost, latency, cache, fallback, and provider trends.
- Savings: actual request-time cost compared with selected source-dated baseline prices.
- Savings by user, key, model group, project, and provider/model.
- Model groups by user.
- Usage by API key, caller ID, and requested model.
- Provider and model mix. This tab reports actual provider/model usage, input/output/total tokens, input/output/total cost, latency, and throughput. It does not include baseline or savings fields by default; use the Savings tabs when a hypothetical baseline comparison is needed.
- Latency and throughput.
- Errors and fallbacks.
- Cache.
- Quotas and budgets.
- Troubleshooting buckets for quota, TPM/RPM or rate-limit, concurrency, max-token/context, upstream quota/billing, key-state, cache, fallback, multi-attempt, and HTTP error classes inferred from safe stored request fields.
- Routing decisions.
- Dynamic-score enabled signals, score buckets, and threshold buckets.
- Max-token buckets, input-token buckets, and admission reasons.
- Provider catalog status from safe runtime configuration metadata. The tab separates `catalog` rows from `active_target` rows so per-group target overrides for modalities, tools, pricing, max-token behavior, and validation are visible without changing catalog metadata. This endpoint does not expose provider API keys, headers, full config, or private deployment files.
- Retention and rollup status from existing usage DB status tables, including the latest retention job, per-table candidate/held/eligible/blocked counts, and recent hourly/daily/monthly rollup runs. This is a read-only status view; retention execution and rollup generation remain operator-controlled workflows.
- Contract buckets. This tab groups contract-present/pass/fail and failure-reason buckets by model group so operators can see whether a group is satisfying its configured quality and capability contract.
- Contract workloads. This tab groups deployment-defined contract workload labels by model group so validation and production traffic can be compared without exposing request content.
- Target validation. This tab groups target-validation status buckets by provider/model so stale, missing, or failing validation metadata is visible before it becomes a routing incident.
- Expensive requests.
- Client breakdown.
- Project chargeback.
- Capability usage for image/VLM, streaming, PII filter, cacheable, and dialect signals available in usage rows.
- Anomalies from deterministic rule-based operational signals such as errors, fallbacks, multi-attempt requests, slow requests, expensive requests, non-ok quota states, and abnormal key states such as disabled, revoked, expired, or suspended. This tab is not machine-learning anomaly detection; normal active key state is not anomalous, and baseline/savings fields are reserved for savings reports.
- Security access events for authorized and unauthorized access paths when enabled.
- Recent requests and request-ID drilldown.

Recent request rows include visible columns for time, request ID, caller ID, caller IP, public token ID, caller user/project/environment, client, requested model, resolved model group, provider, model, dialect, status, cache state, attempts, fallback flag, latency, input/output/total tokens, and stored total cost.

## Charts

Summary responses include a `charts` array with stable chart IDs, titles, X/Y axis labels, axis types, units, series names, semantic color keys, scalar points, generation timestamp, selected range, and active safe filters. The browser dashboard uses that contract to render axes, unit-aware tick labels, legends, and hover tooltips. Money is displayed as USD, token and request counts use compact notation where appropriate, latency uses milliseconds or seconds, throughput uses `tok/s`, and rates use percentages. Tables label totals explicitly as Total Tokens and Total cost whenever input/output breakdowns are present.

Every chart is backed by the same safe aggregate fields shown in tables and Markdown export. Chart payloads contain scalar aggregate points only; they do not include prompts, image payloads, tool schemas or outputs, tokens, token hashes, provider keys, full config, or raw upstream bodies.

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

## Embedded Assets

The admin HTML, CSS, JavaScript, Metrum logo, fonts, and local chart bundle are embedded in the router binary. The UI does not depend on external CDNs or runtime access to the Metrum website. Charts are assistive; the same data is available in tables and Markdown export.

Admin pages and APIs send no-store cache headers. Static admin assets may use private cache headers and contain no report data.

The dark/light theme preference is stored in browser `localStorage`; it does not change server-side reporting data or authorization policy. First visits follow the browser's system color-scheme preference.

## Smoke Test

```bash
curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/requests?since=24h&caller_id=example-caller&provider=openrouter&status=200&cache=miss"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/provider-catalog-status"

curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/retention-status"
```

Expected for an authorized subject: `200` JSON with `summary`, `series`, grouped tables, and recent request rows.

OIDC deployments should first complete `/admin/auth/login`, then call the same report URL with the browser session cookie. A valid OIDC session without Casbin policy receives `403 reports-forbidden`.

Expected for an ordinary router token:

```bash
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Response: `403 reports-forbidden`.
