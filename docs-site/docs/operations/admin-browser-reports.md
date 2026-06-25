---
title: Admin Browser Reports
---

# Admin Browser Reports

Admin browser reports are an authenticated operational surface for usage, performance, cost, cache, fallback, and diagnostic drilldown. They are disabled by default and served separately from public `/docs/`. The browser dashboard uses Metrum branding, local embedded assets, and a dark/light mode toggle for authorized administrators.

## Access Model

Browser identity can be HTTP Basic under `server.admin_auth.basic` or OIDC sessions under `server.admin_auth.oidc`. Authorization is Casbin-backed under `server.admin_auth.authorization`; every `/admin/reports/*` page, API, export, and drilldown route requires an allow decision for object `admin:reports`.

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
        - p, reports_admin, example/prod, admin:reports, read|export
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
```

Ordinary router caller tokens receive `403 reports-forbidden`. Missing or invalid Basic credentials or missing/invalid OIDC sessions receive `401`.

## What It Shows

The browser UI displays requests, errors, tokens, cost, latency, TTFB, upstream/downstream throughput, cache hit/miss/bypass, attempts, fallbacks, provider/model groups, public token IDs, caller metadata, status codes, and recent safe request rows. Request drilldown joins the relational usage, attempt, trace-event, and terminal-error rows by request ID.

Responses do not include raw router tokens, token hashes, provider keys, raw prompts, raw images, raw tool outputs, full config values, or unsanitized upstream bodies.

## Charts

Summary responses include a `charts` array with stable chart IDs, titles, X/Y axis labels, axis types, units, series names, semantic color keys, scalar points, generation timestamp, selected range, and active safe filters. The browser dashboard uses that contract to render axes, unit-aware tick labels, legends, and hover tooltips. Money is displayed as USD, token and request counts use compact notation where appropriate, latency uses milliseconds or seconds, throughput uses `tok/s`, and rates use percentages.

Every chart is backed by the same safe aggregate fields shown in tables and Markdown export. Chart payloads contain scalar aggregate points only; they do not include prompts, image payloads, tool schemas or outputs, tokens, token hashes, provider keys, full config, or raw upstream bodies.

## Savings

The Savings tab compares stored actual request cost against a selected hypothetical baseline. Actual cost is always summed from request-time stored cost fields; it is not recalculated from current provider configuration. Baseline cost is calculated from stored input and output token counts:

```text
baseline_cost_usd =
  input_tokens / 1_000_000 * baseline_input_price_per_million_usd +
  output_tokens / 1_000_000 * baseline_output_price_per_million_usd
```

Built-in baselines are configured under `server.admin_reports.baselines` with source URL, source date, input USD/M, output USD/M, and notes. The default built-ins were source-checked on June 25, 2026: GPT-5.5 from OpenAI API pricing at $5.00/M input and $30.00/M output, and Claude Opus 4.8 from Anthropic Claude pricing at $5.00/M input and $25.00/M output. Baseline prices are externally maintained by providers and should be revalidated when producing contractual or customer-facing savings claims.

Administrators can also enter a custom baseline for the current browser session. Custom values are validated as finite nonnegative USD-per-million-token rates and are not persisted by the router.

## Embedded Assets

The admin HTML, CSS, JavaScript, Metrum logo, fonts, and local chart bundle are embedded in the router binary. The UI does not depend on external CDNs or runtime access to the Metrum website. Charts are assistive; the same data is available in tables and Markdown export.

Admin pages and APIs send no-store cache headers. Static admin assets may use private cache headers and contain no report data.

The dark/light theme preference is stored in browser `localStorage`; it does not change server-side reporting data or authorization policy. First visits follow the browser's system color-scheme preference.

## Smoke Test

```bash
curl -i -u admin:replace-with-password \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Expected for an authorized subject: `200` JSON with `summary`, `series`, grouped tables, and recent request rows.

OIDC deployments should first complete `/admin/auth/login`, then call the same report URL with the browser session cookie. A valid OIDC session without Casbin policy receives `403 reports-forbidden`.

Expected for an ordinary router token:

```bash
curl -i -H "Authorization: Bearer $ROUTER_TOKEN" \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Response: `403 reports-forbidden`.
