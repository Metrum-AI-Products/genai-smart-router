# Usage Reporting Playbook

License enforcement metadata is reportable as safe scalar request fields when enabled: `license_status`, `license_reason`, `license_id`, `license_customer_id`, `license_sku`, `license_key_id`, `license_expiry`, and `license_grace_active`. Use these fields to separate license-denied requests from caller quota, auth, upstream, and routing failures. Reports and exports must not include full license payloads, detached signatures, public/private key bytes, or signing-service details.

Usage reports support cost governance, quota reviews, incident analysis, and savings analysis.

## Standard Dimensions

Report by token ID, caller user/project/environment, model group, provider/model/dialect, client type, caller IP, hour/day, cache status, request status, input/output/image tokens, request-time USD cost, and performance fields.

For commercial evaluation packages, include both business and support dimensions: savings by developer/user/project/key, provider/model mix, model groups used per user/project, usage per key, chargeback, quota/rate-limit review, expensive request investigation, fallback/error triage, performance triage, and security access review. Use anonymized exports for customer-facing examples and never include private hostnames, raw prompts, raw images, raw tokens, token hashes, provider keys, raw tool outputs, or full config.

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

Commercial rollup foundation:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --from 2026-06-14 \
  --to 2026-06-15 \
  --rollup \
  --rollup-type daily
```

Use `--rollup-type hourly` for recent operational trend aggregates, `daily` for customer/project/key/provider chargeback, and `monthly` for **customer internal chargeback and optional enterprise contract true-up summaries**. The command writes scalar rows to `usage_rollup_runs`, the selected aggregate table (`usage_rollup_hourly`, `usage_rollup_daily`, or `usage_rollup_monthly_billing`), `usage_rollup_decision_buckets`, and `usage_rollup_audit_events` from stored rows for the selected UTC `[from,to)` window. Rollup runs store source row count, source min/max request timestamp, deterministic source checksum, aggregate row counts, router version/commit, and generation/finalization timestamps.

These rollups support **customer governance and optional commercial true-up reviews**. Metrum enterprise commercial billing is handled outside the router by Metrum finance (signed license + external contract); these tables are **not** a Metrum product billing ledger.

Rows retain reporting dimensions for caller, token, client, model group, upstream provider/model/dialect, status class, stream/cache, image input, PII filter, contract bucket, validation status, and optional baseline ID. Measures include request/success/error counts, input/output/total tokens, input image count, input image tokens, request-time calculated cost, upstream-reported cost, optional baseline input/output/total cost and savings, latency, throughput, cache, fallback, and attempt counts. To store a commercial savings baseline on rollup rows, pass `--baseline-id`, `--baseline-name`, `--baseline-version`, `--baseline-input-price-per-million-usd`, and `--baseline-output-price-per-million-usd`.

Draft reruns replace the same draft run for that exact type/window without duplicate active rows. Add `--rollup-finalize` only after review/reconciliation; finalized windows are immutable through the generator, and later rollup runs are rejected if they overlap an existing finalized window of the same type. Retention uses finalized daily rollup metadata to block or allow `usage_detail` delete eligibility and never mutates finalized rollup rows.

Retention status:

```bash
router-usage-report \
  --retention-status \
  --config /app/config/config.yaml
```

`server.retention` is disabled by default and defaults to `dry_run: true`. A status run forces dry-run behavior, initializes an active config-derived retention policy version/rules, writes `retention_jobs` plus `retention_job_table_results`, and does not delete rows. It counts candidates for `usage_diagnostics`, `decision_telemetry`, `security_access_events`, `content_capture`, and `usage_detail`, subtracts active `legal_holds` by data class, optional request ID, and timestamp range, and records blocked `usage_detail` counts unless a finalized daily rollup covers the candidate window. Decision telemetry child tables are counted through their parent `request_usage.ts`.

Retention batch run:

```bash
router-usage-report \
  --retention-run \
  --config /app/config/config.yaml
```

`--retention-run` follows `server.retention.dry_run`. With `dry_run: true`, it behaves like a status job. With `dry_run: false`, it deletes at most one configured batch per supported table for `usage_diagnostics` (`request_attempts`, `request_trace_events`, `request_errors`) and `usage_detail` (`request_usage`). Unsupported classes are counted and stored as `blocked_not_implemented` with zero deleted rows. `usage_detail` still requires continuous finalized daily rollup coverage for the candidate window before any batch can delete. This slice does not archive rows, schedule retention jobs, expose a full legal-hold admin API, or delete decision telemetry/security/content-capture rows through the generic retention runner.

Keep retention terms precise:

- raw operational rows are request-level and child-table facts used for incident response and detailed troubleshooting;
- finalized rollups are immutable chargeback / usage aggregates generated from stored request-time facts;
- archived exports are external artifacts controlled by the operator, not produced by the current retention foundation;
- legal holds block dry-run eligibility by data class and timestamp range;
- purge execution, archive/export automation, schedulers, and full hold administration are future slices.

Invoices and commercial savings reports must sum stored request-time cost fields or finalized rollups derived from those fields. Do not reprice historical actuals from current provider config.

## Browser Admin Reports

Deployments can enable authenticated browser reports under `/admin/reports/` for routine operator inspection. The browser surface is disabled by default, requires HTTP Basic admin identity plus Casbin authorization for `admin:reports`, and serves embedded Metrum-branded HTML/CSS/JavaScript/logo/font/chart assets from the router binary without CDN dependencies.

Security access reports can be enabled with `server.admin_reports.security.enabled: true`. They persist safe scalar access events for authorized calls, unauthorized caller-token attempts, report/metrics/content authorization failures, and Basic admin auth checks. Grant `admin:security_reports` separately from `admin:reports`, configure `server.client_ip.trusted_proxy_cidrs` before trusting forwarded IP headers, and verify exports contain no bearer tokens, token hashes, provider keys, prompts, images, raw cookies, or OIDC tokens.

Use it when an operator needs quick usage, cost, latency, cache, fallback, provider/model, and request-drilldown views without shell access. The page includes a browser-local dark/light mode toggle; the selected preference is stored only in that browser. Keep `router-usage-report` for automation, exports, incident response, and headless/server environments.

The browser summary API returns chart descriptors with stable IDs, axis labels, units, series names, semantic color keys, and scalar points. Operators should use the charts for quick trend reading and the matching tables or Markdown export for exact reviewable values. Chart responses must remain safe aggregates only and must not include prompts, image payloads, tool outputs, bearer tokens, token hashes, provider keys, full config, or raw upstream bodies.

The browser shell includes shared usability controls for report tabs: selected tab/search state in the URL, filters for caller ID, caller IP, project, requested model, resolved group, provider, target model, dialect, HTTP status, cache state, and client, safe-field search, sortable table headers, bounded page size, manual refresh, copy-link, copy-field buttons, request-ID drilldown, and CSV export of visible table columns. Tables use explicit per-tab column schemas with stable labels and units; CSV export follows the same visible columns. Do not expose duplicate compatibility aliases such as both `tokens` and `totalTokens` when they carry the same total-token value. Prefer `Input tokens`, `Output tokens`, `Image tokens`, and `Total tokens` labels, and label cost fields as input/image/output/total/baseline/savings/upstream-billed cost when those fields are present. Smoke these controls after deployment with an authorized browser-admin user, then verify ordinary caller tokens still receive `403 reports-forbidden`.

Expanded tabs use safe scalar usage rows for overview, savings by user/key/group/project/provider-model, model groups by user, usage by key/caller/requested-model, provider/model mix, latency/throughput, errors/fallbacks, cache, quotas/budgets, troubleshooting buckets, routing decisions, dynamic-score signal/score/threshold buckets, max-token buckets, input-token buckets, admission reasons, contract buckets, contract workloads, target validation, expensive requests, client breakdown, project chargeback, capability usage, and deterministic rule-based anomaly signals. The contract tabs expose only safe bucket labels, model-group names, provider/model labels, counts, cost, token, latency, cache, fallback, and PII-filter aggregate fields; they do not expose raw prompts, images, tool outputs, tokens, token hashes, provider keys, or full config. Recent request rows show caller ID, caller IP, public token ID, requested model, provider/model/dialect, status, cache, attempts, fallback, latency, tokens, and cost.

Provider catalog status is read from safe runtime config metadata and returns separate `catalog` and `active_target` rows. Catalog rows show provider/model/dialect catalog metadata. Active target rows show resolved per-group target metadata, active group, target index, validation status/workload/age, pricing source/date, modalities, and tool-support labels after target overrides are applied. It must not expose provider API keys, headers, full config, or private deployment paths. Retention and rollup status is a read-only view over existing usage DB status tables; it shows the latest retention job, per-table candidate/held/eligible/blocked/deleted counts, and recent hourly/daily/monthly rollup runs without executing retention or generating rollups.

Troubleshooting buckets are deterministic groupings for quota, TPM/RPM or rate-limit, concurrency, max-token/context, upstream quota/billing, key-state, cache, fallback, multi-attempt, and HTTP error classes inferred from safe stored request fields.

Anomaly reports are operational triage views, not machine-learning anomaly detection. They group requests by deterministic rules such as error responses, fallback use, multiple upstream attempts, slow requests, expensive requests, non-ok quota states, and abnormal key states such as disabled, revoked, expired, or suspended. Normal active key state is not anomalous. Baseline and savings fields belong only to savings reports and should not be interpreted from anomaly report rows.

Smoke an enabled deployment:

```bash
curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"

curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/provider-catalog-status"

curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/retention-status"
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

When enabled, usage reports include a Decision Telemetry Summary with row counts for request shape, candidates, filter reasons, routing decisions, routing signals, score/ranking terms, policy executions, fallback transitions, and cache reasons. The summary also includes strategy, policy outcome/error-class, fallback-reason, filter-reason, cache-reason, enabled-signal, score, threshold, max-token, input-token, and admission-reason buckets. Admin request drilldown includes the same child row sets under `decisionTelemetry`, and scalar admin APIs expose dynamic signal, dynamic score bucket, dynamic threshold, max-token bucket, input-token bucket, and admission-reason reports. The rows are normalized and joinable by `request_id`; they do not store prompts, image payloads, tool schemas, raw tool outputs, bearer tokens, provider keys, token hashes, policy request/response JSON, or full config.

For policy-failure triage, join `request_usage` to `request_policy_executions` by `request_id` and inspect `strategy`, `policy_kind`, `outcome`, `error_class`, `terminal_error_type`, duration, eligible/all target counts, selected candidate index, and fallback count. Fail-closed script/external errors before selection produce policy execution rows even when no `request_routing_decisions` row exists.

For fallback triage, join `request_usage`, `request_attempts`, and `request_fallback_transitions` by `request_id`. A fallback transition row links the failed attempt/candidate to the next fallback candidate with the safe error class, retryable flag, and success flag, so operators can reconstruct cases such as "target selected first, provider returned 429, fallback target succeeded" without reading raw traces.

Smoke after enabling:

1. Send a normal text request and confirm `request_target_candidates` has bounded candidate rows and `request_routing_decisions` has the selected strategy/target.
2. Send a negative request to a group with no compatible target, for example a tool request to a target without `tool_support`, and confirm `502 no-eligible-target` plus a safe `request_target_filter_reasons.reason` such as `tool-support`.
3. Send a request with `Cache-Control: no-cache` and confirm `request_cache_reasons.reason = cache-request-no-cache`.
4. Send an external or script policy failure and confirm a safe `request_policy_executions` row exists without a routing decision row.
5. Send an upstream failure followed by fallback success and confirm `request_fallback_transitions.fallback_succeeded = true`.
6. Generate `router-usage-report` and confirm the Decision Telemetry Summary appears without raw prompt text or secrets.

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
