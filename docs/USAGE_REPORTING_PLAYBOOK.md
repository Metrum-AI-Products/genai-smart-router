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

Daily rollup foundation:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --from 2026-06-14 \
  --to 2026-06-15 \
  --rollup
```

This writes scalar rows to `usage_rollup_runs` and dimensioned `usage_rollup_daily` from stored `request_usage` rows for the selected UTC `[from,to)` window. Daily rows retain reporting dimensions for caller, token, client, model group, upstream provider/model/dialect, status class, stream/cache, image input, PII filter, contract bucket, and validation status, so chargeback and provider-performance reports do not need raw request detail after a window is closed. Measures include input/output/total tokens, input image count, input image tokens, cost, latency, throughput, cache, fallback, and error counts. Draft reruns replace the same draft run for that exact window. Add `--rollup-finalize` only after review; finalized windows are immutable, and later rollup runs are rejected if they overlap an existing finalized daily window. This rollup slice does not delete raw usage rows; the retention foundation only uses finalized rollup metadata to block or allow future `usage_detail` delete eligibility.

Retention dry-run status:

```bash
router-usage-report \
  --retention-status \
  --config /app/config/config.yaml
```

`server.retention` is disabled by default and supports only `dry_run: true` in this foundation. A status run initializes an active config-derived retention policy version/rules and writes `retention_jobs` plus `retention_job_table_results`. It counts candidates for `usage_diagnostics`, `security_access_events`, and `content_capture`, subtracts active `legal_holds` by data class and timestamp range, and records blocked `usage_detail` counts unless a finalized daily rollup covers the candidate window. This slice does not execute deletes, archive rows, schedule retention jobs, or provide full legal-hold admin APIs.

## Browser Admin Reports

Deployments can enable authenticated browser reports under `/admin/reports/` for routine operator inspection. The browser surface is disabled by default, requires HTTP Basic admin identity plus Casbin authorization for `admin:reports`, and serves embedded Metrum-branded HTML/CSS/JavaScript/logo/font/chart assets from the router binary without CDN dependencies.

Security access reports can be enabled with `server.admin_reports.security.enabled: true`. They persist safe scalar access events for authorized calls, unauthorized caller-token attempts, report/metrics/content authorization failures, and Basic admin auth checks. Grant `admin:security_reports` separately from `admin:reports`, configure `server.client_ip.trusted_proxy_cidrs` before trusting forwarded IP headers, and verify exports contain no bearer tokens, token hashes, provider keys, prompts, images, raw cookies, or OIDC tokens.

Use it when an operator needs quick usage, cost, latency, cache, fallback, provider/model, and request-drilldown views without shell access. The page includes a browser-local dark/light mode toggle; the selected preference is stored only in that browser. Keep `router-usage-report` for automation, exports, incident response, and headless/server environments.

The browser summary API returns chart descriptors with stable IDs, axis labels, units, series names, semantic color keys, and scalar points. Operators should use the charts for quick trend reading and the matching tables or Markdown export for exact reviewable values. Chart responses must remain safe aggregates only and must not include prompts, image payloads, tool outputs, bearer tokens, token hashes, provider keys, full config, or raw upstream bodies.

The browser shell includes shared usability controls for report tabs: selected tab/search state in the URL, safe-field search, sortable table headers, bounded page size, manual refresh, copy-link, copy-field buttons, request-ID drilldown, and CSV export of visible table columns. Smoke these controls after deployment with an authorized browser-admin user, then verify ordinary caller tokens still receive `403 reports-forbidden`.

Expanded tabs use safe scalar usage rows for overview, savings by user/key/group, model groups by user, usage by key, provider/model mix, latency/throughput, errors/fallbacks, cache, quotas/budgets, routing decisions, contract buckets, contract workloads, target validation, expensive requests, client breakdown, project chargeback, capability usage, and deterministic rule-based anomaly signals. The contract tabs expose only safe bucket labels, model-group names, provider/model labels, counts, cost, token, latency, cache, fallback, and PII-filter aggregate fields; they do not expose raw prompts, images, tool outputs, tokens, token hashes, provider keys, or full config.

Anomaly reports are operational triage views, not machine-learning anomaly detection. They group requests by deterministic rules such as error responses, fallback use, multiple upstream attempts, slow requests, expensive requests, non-ok quota states, and abnormal key states such as disabled, revoked, expired, or suspended. Normal active key state is not anomalous. Baseline and savings fields belong only to savings reports and should not be interpreted from anomaly report rows.

Smoke an enabled deployment:

```bash
curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"
```

Verify the branded `/admin/reports/` shell loads for an authorized browser-admin user, the dark/light toggle persists after reload, ordinary router caller tokens receive `403 reports-forbidden`, and `/docs/` remains public product documentation with no report data.

For security reports, also smoke:

```bash
curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/security/events?since=24h&limit=50"
```

Expected: `200` JSON with safe access-event rows for a subject authorized for `admin:security_reports`. A Basic/OIDC subject that only has `admin:reports` must receive `403 reports-forbidden`.

## Decision Telemetry

Decision telemetry is disabled by default. Enable it only when operators need request-by-request explainability for target eligibility, selected routing decisions, or cache bypass analysis:

```yaml
server:
  decision_telemetry:
    enabled: true
    max_candidates: 64
    max_filter_reasons: 256
    record_candidates: true
    record_cache_reasons: true
```

When enabled, usage reports include a Decision Telemetry Summary with row counts and top strategy, filter-reason, and cache-reason buckets. The rows are normalized and joinable by `request_id`; they do not store prompts, image payloads, tool schemas, raw tool outputs, bearer tokens, provider keys, token hashes, or full config.

Smoke after enabling:

1. Send a normal text request and confirm `request_target_candidates` has bounded candidate rows and `request_routing_decisions` has the selected strategy/target.
2. Send a negative request to a group with no compatible target, for example a tool request to a target without `tool_support`, and confirm `502 no-eligible-target` plus a safe `request_target_filter_reasons.reason` such as `tool-support`.
3. Send a request with `Cache-Control: no-cache` and confirm `request_cache_reasons.reason = cache-request-no-cache`.
4. Generate `router-usage-report` and confirm the Decision Telemetry Summary appears without raw prompt text or secrets.

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
