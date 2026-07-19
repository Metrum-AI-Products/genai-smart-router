# 06 — Signed Balance Grants and Router Admission (#522)

Plan version: **v1.0.0**  
Issue: [#522](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/522)  
Classification: **launch blocking / provider-cost safety boundary**

**Commercial model update (2026-07-19):** This plan is superseded in part by DECISIONS.md D3. The hosted product is base monthly Stripe subscription + model-mix included allowance + x402 overage. Prepaid credit packs / Stripe Billing Meters are not the primary overage rail. Read this document with that model in mind; the GitHub issue body is the current source of truth.

## Design approach

Extend the existing caller token-budget reservation mechanism with a commercial
price reservation, using the same request-shape information before target
selection/upstream work. The router consumes a signed, short-lived **balance
grant** cached locally or delivered through config/lease refresh. It never calls
Stripe, the ledger, or the commercial API from the inference handler.

Grant scalar fields: version, grant ID, tenant/caller scope, price-book version,
unit/currency, maximum spend authority, already-issued sequence/window, issued/
not-before/expires timestamps, signer key ID, allowed environment/region, and
optional dedicated deployment fingerprint. Sign canonical bytes with a separate
KMS-backed commercial-grant key. Do not reuse license private keys or expose them
to control-plane/Stripe administrators.

The router maintains a durable local reservation journal keyed by request ID so
restarts cannot forget committed/held spend. For horizontally scaled shared
routers, choose a strongly consistent regional reservation backend or partition
grants so two pods cannot spend the same authority. A process-local counter is
not sufficient. Keep the schema relational and safe; store grant/public IDs and
amounts, never raw caller credentials or payment state.

## Price reservation

Worst-case quote uses the request-time customer price-book version, estimated
input/image/tool-schema units, caller output cap, target eligibility, configured
minimum/rounding, and a finance-approved uncertainty buffer. It must not assume a
cheap target when a compatible fallback could cost more. If the router cannot
produce a bounded quote, it denies before upstream.

On success, settle actual priced units and release the remainder. On provider
failure/cancel before billable work, release according to provider-cost evidence.
Cache hits follow the approved customer pricing policy but do not fabricate
upstream cost. Settlement is emitted through a transactional/durable outbox and
is idempotent at #523.

## Human interaction and configuration

Finance/Product approve D3-D4/D7: price units, margin, minimum, rounding, buffer,
grant size/expiry, trial cap, cache-hit policy, and negative-balance rule. API
owner approves D11 status/code and retry semantics. SRE configures verifier keys,
refresh lead time, maximum clock skew, durable reservation backend, outbox limits,
and fail-closed thresholds.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). The issuer needs
a narrowly scoped KMS asymmetric signing role and ledger/control-plane identity;
private key material is non-exportable. Router workloads receive only the public
key set, reservation backend identity/DSN, outbox identity, price-book payload,
and grant artifact/reference. Non-secrets are key IDs/algorithms, canonical
version, unit/currency, grant max/expiry/refresh/skew/buffer, price-book version,
quote limits, backend mode, outbox capacity/retry, D11 error/status, and metric
bounds. Provider keys remain existing router secrets; Stripe/commercial DB secrets
are absent from data-plane identities. Fixtures use test-only key IDs.

Customers see available/held balance and a safe low/exhausted message with top-up
link and request ID. They never see internal upstream/provider cost or grant
payload. Support sees grant ID, expiry, issued/consumed/held amount, refresh state,
and correlation IDs only.

## Request flow

```text
authenticate caller -> existing model/quota/shape admission
 -> load valid scoped grant + price-book version
 -> calculate conservative maximum customer charge
 -> atomically reserve against durable grant authority
 -> select compatible target and call upstream
 -> persist request-time usage/cost
 -> settle actual customer price + release difference
 -> enqueue signed/idempotent settlement
```

Any invalid/expired/consumed grant, reservation-backend uncertainty, or quote
overflow fails before target/provider selection. A settlement outage can queue
within a finite outbox, but once its safe bound is reached admission stops.

## Test plan

* Golden signature/canonicalization/key-rotation/not-before/expiry/scope/region/
  fingerprint/tamper/replay tests.
* Quote vectors for Chat, Responses, Messages, tools, images, cache, streaming,
  bridge directions, max-token edge values, expensive fallback, and rounding.
* Race N pods/processes against one nearly exhausted grant; total successful
  maximum reservation must never exceed authority by one microcredit.
* Crash before reserve, after reserve, after upstream, after usage write, during
  settlement, and after outbox publish; recovery produces exactly one final debit
  or release per request.
* Control-plane/ledger/settlement-backend outage tests prove bounded continued
  traffic and deterministic fail-closed behavior at expiry/outbox cap.
* Network trace/egress fixture asserts the inference handler makes no Stripe or
  commercial-control-plane request.
* Production-derived synthetic fixture records denial/admission/reservation safe
  scalars and no raw prompt/tool/image/token/grant payload.
* Load at 2x launch peak; compare baseline vs grant-admission p50/p95/p99 latency.
  Target incremental admission overhead: <=2 ms p95 within the regional backend.

## Monitoring and rollback

Metrics: grant valid/expiring/refresh failures, reserve/deny/release/settle counts,
value held/consumed, outbox age/depth, orphan/stuck reservation, quote overflow,
and admission latency. Global metrics stay metrics-admin only. Rollback must not
disable spend safety: revert to the prior grant-aware binary/config or suspend
pooled-provider callers; never roll back to unbounded pooled access.

## Definition of done

Concurrency/fault/load evidence proves Metrum's maximum exposure is the signed
grant plus approved buffer, every customer charge is traceable, and commercial
dependencies are absent from the inference network trace.
