# Usage Reporting Playbook

Usage reports support cost governance, quota reviews, incident analysis, and savings analysis.

## Standard Dimensions

Report by token ID, caller user/project/environment, model group, provider/model/dialect, client type, caller IP, hour/day, cache status, request status, input/output/image tokens, request-time USD cost, and performance fields.

Performance sections are included for latency triage:

- Downstream user performance groups by user, project, environment, and client with average/max latency, TTFB, downstream duration, downstream token throughput, errors, streams, and fallbacks.
- Upstream endpoint performance groups by provider, model, and API dialect with average/max upstream duration, latency, TTFB, upstream token throughput, attempts, fallbacks, errors, and cost.

## Common Reports

Daily usage:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --out /app/logs/usage-24h.md
```

## Browser Admin Reports

Deployments can enable authenticated browser reports under `/admin/reports/` for routine operator inspection. The browser surface is disabled by default, requires HTTP Basic admin identity plus Casbin authorization for `admin:reports`, and serves embedded Metrum-branded HTML/CSS/JavaScript/logo/font/chart assets from the router binary without CDN dependencies.

Use it when an operator needs quick usage, cost, latency, cache, fallback, provider/model, and request-drilldown views without shell access. The page includes a browser-local dark/light mode toggle; the selected preference is stored only in that browser. Keep `router-usage-report` for automation, exports, incident response, and headless/server environments.

The browser summary API returns chart descriptors with stable IDs, axis labels, units, series names, semantic color keys, and scalar points. Operators should use the charts for quick trend reading and the matching tables or Markdown export for exact reviewable values. Chart responses must remain safe aggregates only and must not include prompts, image payloads, tool outputs, bearer tokens, token hashes, provider keys, full config, or raw upstream bodies.

The browser shell includes shared usability controls for report tabs: selected tab/search state in the URL, safe-field search, sortable table headers, bounded page size, manual refresh, copy-link, copy-field buttons, request-ID drilldown, and CSV export of visible table columns. Smoke these controls after deployment with an authorized browser-admin user, then verify ordinary caller tokens still receive `403 reports-forbidden`.

Expanded tabs use safe scalar usage rows for overview, savings by user/key/group, model groups by user, usage by key, provider/model mix, latency/throughput, errors/fallbacks, cache, quotas/budgets, routing decisions, expensive requests, client breakdown, project chargeback, capability usage, and deterministic anomaly signals.

Smoke an enabled deployment:

```bash
curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Verify the branded `/admin/reports/` shell loads for an authorized browser-admin user, the dark/light toggle persists after reload, ordinary router caller tokens receive `403 reports-forbidden`, and `/docs/` remains public product documentation with no report data.

Filtered benchmark or project report:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --caller-user <owner-user> \
  --caller-project <project> \
  --caller-environment <environment> \
  --resolved-group <model-group> \
  --client <client> \
  --out /app/logs/usage-filtered.md
```

## Savings Reports

Use stored request-time calculated costs for actual route cost. Compare to a documented baseline model and price, state the baseline price source and date, and include both input and output tokens. Do not recalculate historical actual costs using current provider prices.

Browser admin reports expose the same rule through `/admin/reports/api/savings`. Built-in baselines live under `server.admin_reports.baselines` and include source URL, source date, input USD/M, output USD/M, and notes. Defaults checked on 2026-06-25 are GPT-5.5 at $5.00/M input and $30.00/M output from OpenAI API pricing, and Claude Opus 4.8 at $5.00/M input and $25.00/M output from Anthropic Claude pricing. Revalidate provider pricing before using savings figures in contractual or customer-facing claims. Custom browser-session baselines may be supplied with finite nonnegative input/output USD-per-million-token rates.

## Image Cost Reporting

For VLM requests, normal input-token pricing is the fallback. `image_input_price_per_million_tokens_usd` applies when upstream reports image tokens. `image_input_price_per_image_usd` applies for per-image billing or internal chargeback. Upstream-reported billed cost should be stored separately when the provider returns it.

## Data Handling

Reports may include public token IDs, caller metadata, providers, models, counts, costs, latency, and statuses. Reports must not include raw tokens, provider keys, token hashes, raw prompts, raw image payloads, or unsanitized provider responses.
