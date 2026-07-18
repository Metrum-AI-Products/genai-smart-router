# 05 — Prepaid Double-Entry Ledger and Settlement (#523)

Plan version: **v1.0.0**  
Issue: [#523](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/523)  
Classification: **launch blocking**

## Design approach

Implement D2 behind a small `Ledger` interface. The recommended launch adapter is
PostgreSQL double-entry; TigerBeetle remains a replaceable adapter when measured
posting rate or finance requirements justify another stateful system. Business
logic must not depend on PostgreSQL-specific counter updates.

Represent money/credits as integer microcredits with a versioned currency/unit
definition. Never use float64 for postings. Core tables are `ledger_accounts`,
`journal_entries`, `postings`, `balance_holds`, `ledger_source_events`,
`reconciliation_runs`, `reconciliation_items`, and `price_book_versions` plus
scalar price rows. Enforce balanced journal entries and uniqueness of source type
plus source ID. Entries are immutable; corrections are linked reversals or
adjustments.

Account families include customer available liability, customer held liability,
Metrum cash/processor clearing, revenue, provider-cost expense/accrual, promotional
credit expense, refund/dispute clearing, and reconciliation variance. Finance
must approve the exact chart of accounts before migration is finalized.

## Truth model

Keep these amounts distinct and labeled:

* **Quoted/reserved:** maximum customer charge admitted for a request.
* **Settled customer price:** actual debit from the versioned price book.
* **Calculated upstream cost:** router request-time catalog calculation.
* **Upstream billed cost:** upstream-reported amount when present.
* **Reconciled:** later provider/processor truth with an explicit adjustment.

Historical reports sum stored values. They never reprice old usage from the
current catalog. Router diagnostic retention cannot delete required commercial
journals.

## Human interaction and configuration

Finance approves D2-D4, chart of accounts, microcredit scale, rounding order,
minimum charge, tax ownership, refund/dispute accounting, promotional credit
expiry, reconciliation thresholds, retention, and manual-adjustment authority.
Operators configure database/backup references, consumer batch size, hold expiry,
reconciliation schedule, alert thresholds, and finance export destination.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Split PostgreSQL
roles into migrator, API writer, settlement worker, reconciliation worker, finance
read-only, and backup/restore. If TigerBeetle is approved later, define separate
cluster/admin/client identities while preserving the adapter. Secrets are DSNs or
workload identities and optional finance-export credentials; ledger code needs no
provider key, raw router key, signing private key, OIDC secret, or card data.
Non-secrets include unit/currency scale, accounts/price-book versions, rounding/
minimum/markup, hold/lock/batch timeout, reconciliation window/threshold,
retention, export format, and adjustment role/approval policy.

The support/admin surface shows balance, held/available split, journal timeline,
source request/event IDs, truth label, reconciliation status, and adjustment
workflow. It never offers an editable balance field; adjustments require an
amount, reason, external evidence reference, approver, and compensating postings.
See [the operations mockup](previews/operations-dashboard.html).

## Settlement flow

1. Stripe paid fulfillment posts cash/processor clearing against customer
   available liability (or promotional expense against liability for trial).
2. Grant issuance moves/allocates only bounded spend authority; it is not itself
   revenue.
3. Router reservation event creates or updates a hold using a unique request ID.
4. Completion settles actual customer price and releases unused amount; failure
   releases the full hold; lost completion is found by reconciliation/timeout.
5. Usage reconciliation joins by immutable request ID and price-book version.
6. Refund/dispute creates linked reversing/clearing entries and may suspend future
   grant issuance according to policy.

## Test plan

* Property tests generate credits/debits/holds/releases/reversals and assert every
  journal balances, customer available never drops below policy, and history is
  append-only.
* Concurrency tests race hold creation, settlement, release, top-up, refund, and
  cancellation under serializable/explicit locking semantics.
* Replay 10,000 mixed settlement events twice and in reverse/out-of-order batches;
  assert one economic effect per source ID and deterministic balance.
* Test every rounding boundary and price-book transition using golden vectors.
* Reconcile fixture includes missing usage, missing settlement, amount mismatch,
  late upstream billed cost, stuck hold, duplicate Stripe event, and reversal.
* PostgreSQL backup/restore followed by outbox replay must reproduce invariant
  totals and correlation counts.
* Load test at 10x forecast event rate and record p95 posting latency, lock waits,
  consumer lag, and database resource use. Trigger D2 reconsideration if sustained
  target approaches the documented threshold.

## Evidence, monitoring, and rollback

Expose balanced-entry failures (expected zero), negative-available prevention,
posting latency, hold count/age/value, consumer lag, reconciliation variance and
age, manual adjustments, and backup freshness. Evidence includes test seeds,
invariant SQL, redacted ledger timeline screenshot, replay transcript, load graph,
and restore drill. Rollback stops consumers, preserves outboxes/journals, deploys
the compatible prior binary, then resumes idempotent processing.

## Definition of done

Finance approves accounts/invariants and can independently trace Stripe event ->
credit -> grant/hold -> request settlement -> reconciliation/refund. All property,
concurrency, replay, load, and recovery tests pass on production PostgreSQL.
