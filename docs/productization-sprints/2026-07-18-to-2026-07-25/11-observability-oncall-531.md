# 11 — Observability, Reconciliation SLOs, and On-Call (#531)

Plan version: **v1.0.0**  
Issue: [#531](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/531)  
Classification: **launch blocking; instrumentation contracts start with #521**

## Design approach and UI mockup

Define telemetry at each component's design time, then close this issue after a
cross-system game day. Use OpenTelemetry-compatible traces with safe correlation
IDs, bounded structured logs, Prometheus metrics and customer-safe status
synthetics. Router `/metrics` remains global and `metrics_admin`-only; customer
console data comes from scoped aggregate APIs, never Prometheus.

The operator information architecture is shown in
[operations-dashboard.html](previews/operations-dashboard.html): launch health,
customer journey synthetics, exposure/balance grants, ledger/reconciliation,
Stripe/webhooks, provisioning, router/provider, security/abuse, queues/workers,
and incidents. Dashboards link to runbooks and safe evidence; they do not expose
prompts, tokens, card data, provider keys, full config, raw webhooks or tenant PII.

## SLO and indicator catalog

Initial objectives require owner approval and staged baseline:

* Console/control API availability and p95 latency by safe route class.
* Signup/email verification and Checkout-session creation success.
* Paid webhook receipt latency, fulfillment success/lag, auto-top-up success.
* Ledger posting p95, outbox/consumer age, stuck-hold age and reconciliation delta.
* Provisioning success and p50/p95 time to active; DNS/TLS/smoke health if dedicated.
* Grant refresh success/lead time, reservation denial/latency and exposure cap.
* Router existing availability, end-to-end latency/TTFB, provider attempts/errors,
  fallback, token throughput, usage persistence, licensing and quotas.
* Security/risk challenge/deny, cross-tenant denials, kill switches and secret
  rotation health.
* Full synthetic time from verified funded fixture to first settled request.

Define error budgets, paging vs ticket thresholds, burn-rate windows, minimum
traffic handling, planned maintenance, and customer status impact. Financial
integrity and cross-tenant leakage are zero-tolerance pages, not ordinary SLOs.

## Human interaction and configuration

SRE owns SLO math, on-call schedules, page routing, dashboard/runbook standards,
retention/sampling and game day. Finance owns reconciliation/negative exposure
pages. Security owns auth/isolation/key alerts. Support/Product own status-page and
customer communication templates. Every alert has severity, threshold/window,
owner, runbook, safe context, dedupe, silence policy and recovery condition.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Required systems
are metrics, logs, traces, synthetic runner, paging/on-call, public status and
support. Prefer workload identity/mTLS; any ingestion/pager/status API key lives in
Secrets Manager scoped to its writer. Telemetry collectors cannot read provider,
Stripe, OIDC, signing, raw router or DB credentials. Non-secrets: endpoints, safe
service/environment/region labels, sampling/retention, metric cardinality budgets,
SLO targets/windows, alert thresholds/routes, dashboard/runbook/status component
IDs, synthetic tenant/token secret references, maintenance and evidence retention.
Synthetic API tokens are stage/prod-monitoring-only callers with minimal model/
quota access, rotated and never printed.

## Incident response flows

1. Page includes safe component/environment/region, symptom, threshold, first/last
   time, runbook and correlation link—never customer content/secrets.
2. On-call identifies blast radius and financial/security risk, starts timeline,
   applies tenant/component/global containment using audited controls.
3. Preserve ledger/webhook/outbox/audit evidence; do not delete/requeue blindly.
4. Communicate customer-safe status if impact threshold is met.
5. Recover via documented retry/reconciliation/rollback, run synthetics, close
   status, and create follow-up with evidence and prevention.

Runbooks cover identity/console, Stripe/webhook, ledger/reconciliation, grants/
reservation backend, provisioning/DNS/TLS, router/provider, database/queue, abuse,
cross-tenant/security, backup/restore, secret rotation, and global kill switch.

## Test and game-day plan

* Unit/contract tests validate metric names/types/labels, bounded cardinality,
  trace propagation/redaction, log sanitization and alert expressions.
* Synthetic probes exercise signup fixture, Checkout test fixture, provision,
  `/readyz`, `/v1/models`, all API skins, settlement and console balance.
* Fault-inject OIDC/email, Stripe webhook, DB lock/failover, outbox/worker, grant
  signer/refresh/reservation, EKS/DNS/TLS, provider 400/429/5xx/timeout, usage write,
  and console CDN/API. Each must show the intended signal/page/runbook/recovery.
* Seed negative reconciliation, stuck hold, duplicate webhook, near-expired grant,
  provisioning orphan and one tenant noisy-neighbor; verify exact alert ownership.
* Authorization test proves normal caller receives 403 `metrics-forbidden`, scoped
  console sees only its aggregates, and metrics-admin sees global operational data.
* Scan telemetry/evidence for forbidden secrets/content and induce a synthetic
  canary secret to prove detection.
* Record dashboard/pager/status screenshots, terminal fault replay, trace IDs,
  incident timeline, MTTA/MTTR, recovery checks and action items.

## Rollback and definition of done

Telemetry changes use versioned dashboards/rules and can roll back without
disabling core safety alerts. If observability fails, default safety controls stay
enabled and launch pauses; lack of visibility is not healthy. Done when on-call,
Finance, Security, Support and Product accept the game day and every launch SLO has
an observed signal, tested alert, current runbook and named owner.
