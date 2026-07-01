# Usage Reporting Playbook

License enforcement metadata is reportable as safe scalar request fields when enabled: `license_status`, `license_reason`, `license_id`, `license_customer_id`, `license_sku`, `license_key_id`, `license_expiry`, and `license_grace_active`. Use these fields to separate license-denied requests from caller quota, auth, upstream, and routing failures. Reports and exports must not include full license payloads, detached signatures, public/private key bytes, or signing-service details.

Usage reports support cost governance, quota reviews, incident analysis, and savings analysis.

For router-versus-fixed-model quality decisions, pair usage reports with the [Evaluation Evidence Playbook](EVALUATION_EVIDENCE_PLAYBOOK.md). Reports explain which providers/models served the run, how much they cost, how fast they were, and whether retries or fallbacks occurred; the external evaluator explains whether the task outcome was acceptable.

## Standard Dimensions

Report by token ID, caller user/project/environment, model group, provider/model/dialect, client type, caller IP, hour/day, cache status, request status, input/output/image tokens, request-time USD cost, and performance fields.

For commercial evaluation packages, include both business and support dimensions: savings by developer/user/project/key, provider/model mix, model groups used per user/project, usage per key, chargeback, quota/rate-limit review, expensive request investigation, fallback/error triage, performance triage, and security access review. Use anonymized exports for customer-facing examples and never include private hostnames, raw prompts, raw images, raw tokens, token hashes, provider keys, raw tool outputs, or full config.

Performance sections are included for latency triage:

- Downstream user performance groups by user, project, environment, and client with average/max latency, TTFB, downstream duration, downstream token throughput, errors, streams, and fallbacks.
- Upstream endpoint performance groups by provider, model, and API dialect with average/max upstream duration, latency, TTFB, upstream token throughput, attempts, fallbacks, errors, and cost.

Traffic-shaping triage uses safe scalar fields on `request_usage`: `traffic_shape_applied`, `traffic_shape_decision`, `traffic_shape_scope`, `traffic_shape_bucket`, `traffic_shape_retry_after_ms`, `traffic_shape_queue_wait_ms`, `traffic_shape_estimated_input_tokens`, `traffic_shape_reserved_output_tokens`, and `traffic_shape_total_reserved_tokens`. Join `request_traffic_shape_events` by `request_id` for one row per evaluated bucket. Reports aggregate queued count, rejection count, average/p50/p95/max queue wait, retry-after, caller/project/client/model-group dimensions, upstream 429s, and route-around outcomes. These fields distinguish shaped bursts from hard `rpm`/`tpm`/`concurrent` rejections and upstream provider 429 attempts without storing prompts, images, token hashes, provider keys, or full config.

Request-shape and provider-translation triage uses normalized diagnostics tables that are written independently of optional decision telemetry:

- `request_shapes`: one row per routed request with inbound dialect, requested/resolved group, client, stream flag, input item/message/role counts, tool result/function output counts, tool count, tool-choice mode, field-presence booleans, image/audio/video presence, input-text/tool-schema/request-size buckets, estimated-input-token bucket, requested output-cap field/bucket, and HMAC request/tool-schema fingerprints.
- `request_translation_shapes`: one row per upstream attempt with provider/model/dialect/path, optional `bridge_direction`, translated stream/tool/tool-choice/output-cap/reasoning controls, translated request-size bucket, strip/rewrite/unsupported/warning counts, and the same fingerprints for joining.
- `request_translation_field_events`: bounded child rows keyed by request ID and attempt index for safe field actions. Field names are allowlisted; unsafe or unexpected names are stored as `other`.

These tables are for answering “what changed between successful and failed requests to the same provider/model/dialect?” They must never contain raw prompts, raw images, image URLs, tool schema text, tool outputs, bearer tokens, provider keys, token hashes, full upstream headers, or full config.

For Responses-to-Chat bridge analysis, group `request_usage` by `inbound_dialect`, `target_dialect`, provider, model, and `request_translation_shapes.bridge_direction`. Native Responses attempts have inbound and target dialect `openai-responses`; bridged attempts have inbound `openai-responses`, target `openai-chat`, and bridge direction `responses_to_chat`. When a bridged request is rejected before upstream, decision telemetry filter reasons use bounded labels beginning with `responses-to-chat-`.

```sql
SELECT
  u.inbound_dialect,
  u.target_dialect,
  u.target_provider,
  u.target_model,
  COALESCE(ts.bridge_direction, '') AS bridge_direction,
  COUNT(*) AS requests,
  SUM(u.input_tokens) AS input_tokens,
  SUM(u.output_tokens) AS output_tokens,
  AVG(u.latency_ms) AS avg_latency_ms
FROM request_usage u
LEFT JOIN request_translation_shapes ts
  ON ts.request_id = u.request_id
WHERE u.ts >= :from
  AND u.ts < :to
  AND u.inbound_dialect = 'openai-responses'
GROUP BY 1,2,3,4,5
ORDER BY requests DESC;
```

For Chat-to-Responses bridge requests, `request_usage.inbound_dialect` remains `openai-chat` while `target_dialect` and `request_attempts.dialect` are `openai-responses`. Join `request_translation_shapes` on request ID and attempt index to confirm the translated endpoint path, output-cap field, tool count, safe stripped/rewritten field counts, and request-size bucket without inspecting raw payloads.

For reasoning proof, use request IDs from the smoke output and require one selected attempt plus matching translation telemetry:

```sql
SELECT
  u.request_id,
  u.resolved_group,
  u.status_code,
  u.fallback_used,
  a.provider,
  a.model,
  a.dialect,
  ts.translated_reasoning_control
FROM request_usage u
JOIN request_attempts a
  ON a.request_id = u.request_id
 AND a.selected = TRUE
JOIN request_translation_shapes ts
  ON ts.request_id = u.request_id
 AND ts.attempt_index = a.attempt_index
WHERE u.request_id IN (:chat_request_id, :responses_request_id, :anthropic_request_id)
ORDER BY u.request_id;
```

Expected `translated_reasoning_control` values are `reasoning_effort` for OpenAI Chat, `reasoning` for OpenAI Responses, and `thinking` for Anthropic Messages. The selected provider/model/dialect should match the deployment's reasoning smoke target set, and `fallback_used` should be false unless the run is explicitly testing fallback behavior.

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

`--retention-run` follows `server.retention.dry_run`. With `dry_run: true`, it behaves like a status job. With `dry_run: false`, it deletes at most one configured batch per supported table for `usage_diagnostics` (`request_attempts`, `request_trace_events`, `request_traffic_shape_events`, `request_upstream_shape_events`, `request_shapes`, `request_translation_shapes`, `request_translation_field_events`, `request_upstream_error_details`, `request_errors`) and `usage_detail` (`request_usage`). Unsupported classes are counted and stored as `blocked_not_implemented` with zero deleted rows. `usage_detail` still requires continuous finalized daily rollup coverage for the candidate window before any batch can delete. This slice does not archive rows, schedule retention jobs, expose a full legal-hold admin API, or delete decision telemetry/security/content-capture rows through the generic retention runner.

Keep retention terms precise:

- raw operational rows are request-level and child-table facts used for incident response and detailed troubleshooting;
- finalized rollups are immutable chargeback / usage aggregates generated from stored request-time facts;
- archived exports are external artifacts controlled by the operator, not produced by the current retention foundation;
- legal holds block dry-run eligibility by data class and timestamp range;
- purge execution, archive/export automation, schedulers, and full hold administration are future slices.

Invoices and commercial savings reports must sum stored request-time cost fields or finalized rollups derived from those fields. Do not reprice historical actuals from current provider config.

## Browser Admin Reports

Deployments can enable authenticated browser reports under `/admin/reports/` for routine operator inspection. The browser surface is disabled by default, requires HTTP Basic admin identity plus Casbin authorization for `admin:reports`, and serves embedded Metrum-branded HTML/CSS/JavaScript/logo/font/chart assets from the router binary without CDN dependencies. The header stacks the Metrum brand/title row above a full-width global filter row. It shows a version chip populated from `/admin/reports/api/version`, which is protected by the same admin report auth and returns safe build fields (`version`, `commit`, `build_date`, `go_version`, `goos`, `goarch`, and `license_compile_mode`) without requiring usage-report storage.

Security access reports can be enabled with `server.admin_reports.security.enabled: true`. They persist safe scalar access events for authorized calls, unauthorized caller-token attempts, report/metrics/content authorization failures, and Basic admin auth checks. Grant `admin:security_reports` separately from `admin:reports`, configure `server.client_ip.trusted_proxy_cidrs` before trusting forwarded IP headers, and verify exports contain no bearer tokens, token hashes, provider keys, prompts, images, raw cookies, OIDC tokens, or raw spreadsheet formulas.

Use it when an operator needs quick usage, cost, latency, cache, fallback, provider/model, and request-drilldown views without shell access. The page uses a dark operational theme aligned with embedded Metrum assets and grouped left-sidebar navigation on desktop. On narrow screens, the same report groups open from the header `Sections` drawer. Collapsed sidebar groups persist in browser `localStorage`; this is local presentation state only and does not change report APIs, URL filters, Markdown/CSV export, or Casbin authorization. Keep `router-usage-report` for automation, exports, incident response, and headless/server environments.

For target-distribution incidents, pair historical request reports with the current provider catalog status report. Provider/model mix and request rows show what actually served a time window by inbound dialect, requested group, provider, model, and status. `/admin/reports/api/provider-catalog-status` explains the current effective pool: `groupSummary` counts eligible active targets by native skin, and each active target row separates `effectiveToolSupport` from `inactiveToolSupport`. If Codex/Responses traffic all lands on one upstream while Chat traffic is balanced, the usual first check is whether the group has more than one `openaiResponsesToolTargets` entry rather than whether catalog metadata mentions Responses somewhere.

Request evidence bundles are the request-level diagnostic contract for incident response. Use:

```bash
curl -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/request-evidence?request_id=<request_id>"
```

The response is assembled from scalar relational rows and mirrors the path-style drilldown `/admin/reports/api/request/<request_id>`. It includes safe request, cost, admission/shaping, target eligibility, attempts, sanitized upstream errors, trace, request-shape, translation-shape, and decision telemetry sections where present. `diagnosticCompleteness`, `diagnosticCompletenessScore`, and `evidenceSections` classify each expected section as `present`, `not_applicable`, or `missing`. Investigate `partial_missing` and `minimal` bundles as logging regressions or early-failure gaps before assuming the router had no evidence.

Evidence endpoints require `admin:reports` `drilldown`, are Casbin-domain scoped, and use `Cache-Control: no-store`. They must not return raw prompts, raw responses, raw image URLs or payloads, raw tool schemas, raw tool outputs, provider API keys, router bearer tokens, token hashes, full upstream headers, unsanitized upstream bodies, full config, or spreadsheet-active export values.

Browser report APIs return chart descriptors with stable IDs, axis labels, units, series names, semantic color keys, and scalar points. Every report tab must render at least one aggregate chart when the selected filters return data; otherwise it must show an explicit no-chart-data state before the table. Category charts use the shared short-label encoder in the browser: long bucket names are shortened on the axis, full labels stay in tooltips and the collapsible bucket legend, and API/table/CSV/Markdown labels remain unchanged. Time-series charts keep the full RFC3339Nano `x` value and also include numeric `x_unix_ms` values so the browser can use Chart.js time scaling with adaptive UTC tick labels; tooltips show the full timestamp with an explicit `UTC` suffix, and exports continue to use the full timestamp. Operators should use the charts for quick trend reading and the matching tables or Markdown export for exact reviewable values. Chart responses must remain safe aggregates only and must not include prompts, image payloads, tool outputs, bearer tokens, token hashes, provider keys, full config, or raw upstream bodies.

The browser shell includes shared usability controls for report tabs: selected tab, filters, server sort/direction, limit, cursor state, and safe-field search in the URL; a two-row header with always-visible `Since`, Markdown export, mobile `Sections`, and an accessible `Filters` disclosure for global investigation filters. Global filters cover caller ID, caller user, public token ID, caller IP, project, environment, requested model, resolved group, provider, target model, dialect, and client, and they persist while switching tabs. Baseline, status, cache state, sort, direction, and traffic-shaping bucket/scope are exposed through per-tab filter panels so each report shows only relevant controls. Row limit is controlled by a `Rows` select in the table toolbar and remains the URL/API `limit` parameter. `Reset filters` clears only the active tab's local filters and never clears globals. The shell also includes returned-page/top-N quick filtering, sortable table headers, cursor first/previous/next controls for request/security detail, manual refresh, copy-link, copy-field buttons, request-ID drilldown, and CSV export labeled as current page, top-N rows, or visible rows. Tables use explicit per-tab column schemas with stable labels and units; CSV export follows the same visible columns and neutralizes spreadsheet formula-leading values. Markdown export is a full current-filter report, omits page cursors, and escapes raw HTML plus active Markdown table cell syntax. Do not expose duplicate compatibility aliases such as both `tokens` and `totalTokens` when they carry the same total-token value. Prefer `Input tokens`, `Output tokens`, `Image tokens`, and `Total tokens` labels, and label cost fields as input/image/output/total/baseline/savings/upstream-billed cost when those fields are present. Smoke these controls after deployment with an authorized browser-admin user, then verify ordinary caller tokens still receive `403 reports-forbidden`.

Each tab must have in-product help metadata: short description, purpose, data semantics, caveats, common filters, key columns, related reports, docs anchor, and empty-state guidance. The report header renders this metadata in the expandable `How to use this report` panel, sidebar items expose concise descriptions, and mobile navigation shows descriptions inline. Keep this metadata customer-safe and aligned with `docs-site/docs/operations/admin-browser-reports.md` whenever report tabs, pagination semantics, columns, or exports change.

Admin report APIs include a `pagination` object. Raw request and security-event APIs use server-side cursor pagination with a stable deterministic order and filtered `total_count`:

```bash
curl -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/requests?since=24h&limit=50&sort=timeUtc&direction=desc"

curl -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/requests?since=24h&limit=50&cursor=<next_cursor>"
```

The request APIs support `limit`, `cursor`, `sort`, and `direction`; request sort keys are `timeUtc`, `costUsd`, `latencyMs`, `status`, and `requestId`. Security-event sort keys are `timeUtc`, `status`, `outcome`, `surface`, and `reason`. Cursors are opaque signed values bound to the endpoint, sort, and direction. Malformed, tampered, or mismatched cursors return `400 invalid-report-filter`. Cursors are process-local operational tokens and should be treated as short-lived page positions, not durable bookmarks. Domain scoping is applied before counting, ordering, and page selection, so the next page cannot cross into another Casbin domain.

Aggregate reports remain top-N summaries when full aggregate pagination would require expensive in-memory regrouping. Their response metadata uses `mode: "top_n"`, `total_count: null`, `has_more`, and a note that describes the ranking criterion. Treat quick filtering and table-header sorting in the browser as operating over the returned top-N rows for those aggregate tabs. Browser CSV export is explicit about scope: current cursor page for request/security detail, returned top-N rows for aggregate tabs, or visible rows for unpaged responses. Markdown export is labeled as a full current-filter report and does not include the cursor parameter. Use CLI reports or a future bounded full-export workflow for full historical exports.

Operator examples:

- Find all errors for user X in the last 8h: open `/admin/reports/?tab=requests&since=8h&caller_user=X&status=500&limit=50&sort=timeUtc&direction=desc`, then page with Next until `has_more` is false. If the user/status/time filters change, confirm the cursor disappears from the URL before reviewing results.
- Sort expensive requests by cost and export the current page: open `/admin/reports/?tab=expensive-requests&since=24h&limit=50&sort=costUsd&direction=desc`, review the visible request IDs, then use `CSV current page`.
- Understand Provider/model top-N: `/admin/reports/?tab=provider-model-mix&since=24h&limit=50` ranks the returned top 50 provider/model groups. It is not page 1 of every provider/model row; use those dimensions on the Requests tab for cursor-paged detail.

Report APIs are domain-scoped unless the subject has an explicit `*` Casbin policy domain. A domain-scoped admin sees usage rows, request lists, request details, Markdown export, and security events for its own project/environment only. Use a separate deployment-global admin subject for cross-project incident response.

For local frontend regression coverage, run:

```bash
make admin-e2e
```

The target rebuilds the admin bundle and runs Playwright against a local Vite preview with mocked safe report APIs. It verifies the version chip, all admin report tabs, grouped sidebar coverage, collapse persistence, mobile drawer behavior, chart/table rendering, URL filter state, search, and CSV download behavior without touching production or requiring router/provider credentials. Keep the deployment smoke below for real auth, CSP, no-store headers, and ordinary caller-token rejection.

Expanded tabs use safe scalar usage rows for overview, savings by user/key/group/project/provider-model, model groups by user, usage by key/caller/requested-model, provider/model mix, latency/throughput, errors/fallbacks, upstream failures, request-shape failures, fallback health, user/client impact, cache, quotas/budgets, troubleshooting buckets, routing decisions, dynamic-score signal/score/threshold buckets, max-token buckets, input-token buckets, admission reasons, contract buckets, contract workloads, target validation, expensive requests, client breakdown, project chargeback, capability usage, and deterministic rule-based anomaly signals. The troubleshooting tabs use sanitized upstream error details, safe request/translation shape buckets, fallback transitions when present, and usage-row fallback metadata when transition rows are absent. The contract and troubleshooting tabs expose only safe bucket labels, model-group names, provider/model labels, counts, cost, token, latency, cache, fallback, and PII-filter aggregate fields; they do not expose raw prompts, images, tool outputs, tokens, token hashes, provider keys, or full config. Recent request rows show caller ID, caller IP, public token ID, requested model, provider/model/dialect, status, cache, attempts, fallback, latency, tokens, and cost.

Provider catalog status is read from safe runtime config metadata and returns separate `catalog` and `active_target` rows. Catalog rows show provider/model/dialect catalog metadata. Active target rows show resolved per-group target metadata, active group, target index, validation status/workload/age, pricing source/date, modalities, and tool-support labels after target overrides are applied. It must not expose provider API keys, headers, full config, or private deployment paths. Its charts summarize row source, validation status, and active-target provider counts. Retention and rollup status is a read-only view over existing usage DB status tables; it shows the latest retention job, per-table candidate/held/eligible/blocked/deleted counts, and recent hourly/daily/monthly rollup runs without executing retention or generating rollups. Its charts summarize retention eligibility/deletion counts and recent rollup status/type counts.

Troubleshooting buckets are deterministic groupings for quota, TPM/RPM or rate-limit, concurrency, max-token/context, upstream quota/billing, provider access, key-state, cache, fallback, multi-attempt, and HTTP error classes inferred from safe stored request fields.

Provider access failures are not caller quota problems. Use upstream-failure and request-evidence reports to separate `upstream_auth_failed`, `upstream_access_denied`, `upstream_entitlement_failed`, and `upstream_model_access_denied` from router-side `tpm-exceeded`, `rpm-exceeded`, and `quota-exhausted`. If fallback succeeded, the request row can be `200` while the failed access attempt remains visible in `request_attempts` and fallback-transition rows. If every attempted target failed with provider access classes, the terminal request error is `upstream-access-denied`.

Anomaly reports are operational triage views, not machine-learning anomaly detection. They group requests by deterministic rules such as error responses, fallback use, multiple upstream attempts, slow requests, expensive requests, non-ok quota states, and abnormal key states such as disabled, revoked, expired, or suspended. Normal active key state is not anomalous. Baseline and savings fields belong only to savings reports and should not be interpreted from anomaly report rows.

Smoke an enabled deployment:

```bash
curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/summary?since=24h"

curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/provider-catalog-status"

curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/retention-status"

curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/version"
```

Verify the branded `/admin/reports/` shell loads for an authorized browser-admin user, the header version chip matches the running router build metadata, every tab shows aggregate charts or the no-chart-data state, the curated table columns do not duplicate total-token aliases, ordinary router caller tokens receive `403 reports-forbidden`, and `/docs/` remains public product documentation with no report data.

For security reports, also smoke:

```bash
curl -i -u admin:<password> \
  "$ROUTER_BASE_URL/admin/reports/api/security/events?since=24h&limit=50"
```

Expected: `200` JSON with safe access-event rows for a subject authorized for `admin:security_reports`. A Basic/OIDC subject that only has `admin:reports` must receive `403 reports-forbidden`.

Pagination smoke: call `/admin/reports/api/requests?since=24h&limit=50`, confirm `pagination.mode` is `cursor`, `returned` is at most 50, and `next_cursor` appears when `has_more` is true. Call the same URL with `cursor=<next_cursor>` and confirm no duplicated request IDs. Repeat with a filter such as `caller_user`, `client`, or `resolved_group`. For aggregate tabs such as `/admin/reports/api/usage-by-key?since=24h&limit=50`, confirm `pagination.mode` is `top_n`.

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

For fallback triage, join `request_usage`, `request_attempts`, `request_fallback_transitions`, and, when enabled, `request_upstream_error_details` by `request_id`. A fallback transition row links the failed attempt/candidate to the next fallback candidate with the safe error class, retryable flag, and success flag, so operators can reconstruct cases such as "target selected first, provider returned 429, fallback target succeeded" without reading raw traces. Upstream error detail rows add bounded provider fields such as code, type, param, request ID, and sanitized message without raw upstream bodies.

For upstream shared-capacity triage, join `request_usage` to `request_upstream_shape_events` by `request_id`. Inspect `scope`, `provider`, `model`, `dialect`, `bucket`, `decision`, `retry_after_ms`, and `backoff_reason` to distinguish `provider-shape-throttled`, `model-shape-throttled`, `target-shape-throttled`, `adaptive-backoff-provider-429`, and `adaptive-backoff-provider-quota`. Use these rows with `request_attempts.error_class` to separate proactive local shaping from upstream-returned rate limits or provider quota failures.

Traffic-shaping incident reports:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --traffic-shaped-only \
  --caller-user <owner-user>

router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --provider <provider-name> \
  --traffic-shape-scope provider
```

Read the caller sections first for `429 traffic-shaped`: confirm whether the request was rejected or queued, which bucket limited it, and whether queue wait p50/p95/max explains latency. Read Provider Capacity Shaping and Adaptive Backoff for `503 upstream-capacity-throttled`: confirm whether all eligible targets were locally skipped, whether a prior upstream `429`/quota event started cooldown, and whether successful route-arounds indicate the model group still had enough target diversity.

Traffic tuning advisor:

```bash
router-usage-report \
  --driver postgres \
  --dsn "$ROUTER_USAGE_DB_DSN" \
  --since 24h \
  --traffic-tuning-advisor \
  --caller-user <owner-user>
```

The advisor is deterministic and report-only. It reads existing `request_usage`, `request_traffic_shape_events`, `request_upstream_shape_events`, and `request_attempts` data, then emits recommendation classes such as `enable_queue`, `increase_queue_depth`, `increase_caller_burst`, `disable_queue_for_latency_sensitive_client`, `investigate_provider_429_capacity`, `route_around_incompatible_target`, and `no_shaping_change_indicated`. Each row includes safe evidence counts, p50/p95/max queue wait, retry-after, upstream 400/429/5xx/timeout counts, client cancellations, fallback rate, affected user/client counts, triggering threshold, and config fields to inspect. It never prints raw prompts, raw images, raw tool schemas, bearer tokens, token hashes, provider keys, or full config.

Use the advisor before changing production burst or queue values:

- If all shaping buckets were admitted or absent while upstream 400s dominate, follow `route_around_incompatible_target`: inspect request-shape failures, provider/model/dialect compatibility, tool/modality metadata, and `models.<group>.targets[]`. Increasing burst or queue depth will not fix upstream invalid-request failures.
- If router `429 traffic-shaped` rows or caller shaping rejections dominate while upstream errors are low, inspect the named `callers[].traffic_shape.*_burst` field and decide whether a small burst increase or client-side slowdown is safer.
- If queued requests later reject or p95 queue wait is high, inspect `queue.max_depth` and `queue.max_wait_ms`. Increase them only for workflows that can tolerate added latency.
- If client cancellations rise with high queue wait, reduce queue wait or disable queueing for that latency-sensitive caller.
- If provider 429/backoff appears across users, tune provider/model shared `traffic_shape` and `upstream_429_backoff`, or shift route weights, before increasing one user's burst.

Smoke after enabling:

1. Send a normal text request and confirm `request_target_candidates` has bounded candidate rows and `request_routing_decisions` has the selected strategy/target.
2. Send a negative request to a group with no compatible target, for example a tool request to a target without `tool_support`, and confirm `502 no-eligible-target` plus a safe `request_target_filter_reasons.reason` such as `tool-support`.
3. Send a request with `Cache-Control: no-cache` and confirm `request_cache_reasons.reason = cache-request-no-cache`.
4. Send an external or script policy failure and confirm a safe `request_policy_executions` row exists without a routing decision row.
5. Send an upstream failure followed by fallback success and confirm `request_fallback_transitions.fallback_succeeded = true`.
6. Enable a very low provider `traffic_shape` on a local smoke target, send two quick requests, and confirm one `request_upstream_shape_events.decision = 'skipped'` row without raw prompt text or secrets.
7. Generate `router-usage-report --traffic-shaped-only` and confirm Traffic Shaping Summary, Provider Capacity Shaping, and Adaptive Backoff sections appear without raw prompt text or secrets.
8. Open `/admin/reports/?tab=traffic-shaping-overview&since=24h`, `/admin/reports/?tab=provider-capacity-shaping&since=24h`, and `/admin/reports/?tab=adaptive-upstream-backoff&since=24h`; verify tables, charts, search, sort, and CSV export use safe scalar fields only.
9. Generate `router-usage-report` and confirm the Decision Telemetry Summary appears without raw prompt text or secrets.

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

## Evaluation Evidence Joins

When joining router reports with Harbor or another evaluation harness, use safe scalar fields only:

- UTC time window for the run;
- caller user, project, environment, public token ID, and client;
- requested model group, resolved group, provider, model, and dialect;
- request ID, run label, task ID, seed, attempt, or trace correlation ID when available;
- status, error type, attempts, fallback count, timeout, cancellation, latency, TTFB, duration, and throughput;
- input, output, image, cache, request-time calculated cost, and upstream-reported billed cost fields.

Do not add raw prompts, raw images, tool outputs, bearer tokens, token hashes, provider keys, private repository contents, private hostnames, or full production config to evaluation reports. If customer-owned raw cases are required for debugging, use a governed support path outside router usage reporting.

For fixed-model comparisons, report both actual routed cost and baseline cost from documented baseline prices or the fixed model's own usage data. State the price source/date and do not reprice historical routed actuals from current config.

## Savings Reports

Use stored request-time calculated costs for actual route cost. Compare to a documented baseline model and price, state the baseline price source and date, and include both input and output tokens. Do not recalculate historical actual costs using current provider prices.

Browser admin reports expose the same rule through `/admin/reports/api/savings`. Built-in baselines live under `server.admin_reports.baselines` and include source URL, source date, input USD/M, output USD/M, and notes. Defaults checked on 2026-06-25 are GPT-5.5 at $5.00/M input and $30.00/M output from OpenAI API pricing, and Claude Opus 4.8 at $5.00/M input and $25.00/M output from Anthropic Claude pricing. Revalidate provider pricing before using savings figures in contractual or customer-facing claims. Custom browser-session baselines may be supplied with finite nonnegative input/output USD-per-million-token rates.

Savings breakdown browser tabs should expose four charts when rows and a baseline are present: request count, actual vs baseline cost, savings USD, and savings rate. Check the chart descriptors with a safe admin API smoke such as `jq '.charts[] | {id: .chart_id, title, series: [.series[].name]}'`, then verify the browser chart shows compact category-axis labels plus a bucket legend for full user/key/group/project/provider-model labels. Negative savings is valid evidence that the selected baseline would have been cheaper for that bucket; use the table and related usage/latency reports to decide whether the model group, target weights, baseline, or workload quality gate needs review.

The visible savings breakdown table should stay savings-first: dimension, optional secondary dimension, requests, input/output/total tokens, actual cost, baseline cost, savings, savings rate, and average cost per request. CSV export follows this visible order. If the investigation needs latency, throughput, cache, fallback, or detailed provider mix, switch to the related usage, performance, errors/fallbacks, or request drilldown tab instead of widening the savings table back into a generic scalar report.

## Image Cost Reporting

For VLM requests, normal input-token pricing is the fallback. `image_input_price_per_million_tokens_usd` applies when upstream reports image tokens. `image_input_price_per_image_usd` applies for per-image billing or internal chargeback. Upstream-reported billed cost should be stored separately when the provider returns it.

## Data Handling

Reports may include public token IDs, caller metadata, providers, models, counts, costs, latency, and statuses. Reports must not include raw tokens, provider keys, token hashes, raw prompts, raw image payloads, or unsanitized provider responses.
