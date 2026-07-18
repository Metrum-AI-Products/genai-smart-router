# 09 — Stripe Prepaid Payments, Auto-Top-Up, and Reconciliation (#524)

Plan version: **v1.0.0**  
Issue: [#524](https://github.com/sysadmin-metrum-ai/genai-smart-router/issues/524)  
Classification: **launch blocking**

## Product and UI approach

Use Stripe-hosted Checkout for one-time prepaid credit packs and Stripe Customer
Portal for payment method, receipts/invoices, and any approved cancellation
actions. Metrum pages show product/credit/balance state but never render card
fields. See [checkout-provisioning.html](previews/checkout-provisioning.html) and
[prosumer-console.html](previews/prosumer-console.html).

The console presents approved credit packs, currency, taxes/fees disclosure,
balance-after-purchase estimate, terms/refund links, and an opt-in auto-top-up
control: threshold, pack, monthly maximum, and explicit confirmation. Creating a
Checkout Session uses a server-generated idempotency key and authoritative
tenant/price mapping; the browser cannot submit arbitrary amount or Stripe Price
ID. The return page says `Confirming payment` until server fulfillment is true.

Stripe documents that fulfillment requires webhooks and that delayed payment
methods have asynchronous success/failure events: [Fulfill orders](https://docs.stripe.com/checkout/fulfillment).
Portal sessions are short-lived server-created redirects: [Customer Portal](https://docs.stripe.com/customer-management).

## Webhook and fulfillment design

Expose a dedicated raw-body webhook endpoint. Verify signature/timestamp with the
official Stripe library before parsing/processing. Persist a bounded immutable
receipt row containing Stripe event ID/type/created timestamp, safe customer/
session/payment-intent IDs, livemode, API version, payload fingerprint, processing
state, and retry fields—never the raw event/card/payment method details. Stripe
does not guarantee event order, so retrieve authoritative objects when needed and
apply monotonic state/idempotent source-event logic.

Allowlist required events, including the exact paid/async-success/async-failure,
refund/chargeback/payment-method/customer/Portal lifecycle events selected during
implementation. Unknown valid events are acknowledged and safely counted. A
payment credits #523 only after confirmed paid state; success URL/session ID is a
convenience lookup, never proof. Webhook transaction enqueues fulfillment; workers
credit the ledger, evaluate risk, issue/refresh grant, trigger provisioning if
needed, and notify exactly once.

## Auto-top-up flow

1. Customer enters Stripe-hosted payment method flow and confirms threshold/pack/
   monthly cap in Metrum UI.
2. Control plane stores Stripe customer/payment-method safe references and policy,
   never card details.
3. Ledger available balance crossing the threshold creates one top-up intent with
   deterministic period/sequence idempotency.
4. Stripe payment succeeds -> webhook -> ledger credit -> refreshed grant -> email.
5. Failure pauses repeated attempts, applies dunning/backoff, notifies customer,
   and preserves remaining balance. Exhaustion still denies before provider spend.

## Human interaction and configuration

Finance/Product/Legal approve D3-D4, credit packs, currencies/payment methods,
tax collection, statement descriptor, receipt/invoice behavior, refund/chargeback,
auto-top-up defaults/caps, dunning, delayed payment methods, trial interaction and
live launch checklist. Finance owns Stripe Products/Prices and test/live promotion.
Support owns refund/dispute playbooks; Security validates SAQ-A boundary and roles.

## Account, API-key, secret, and configuration inventory

Use the [shared registry](00-shared-account-config-inventory.md). Create separate
Stripe test/live accounts or strictly separated modes, least-privilege human roles,
a server restricted secret API key, independent webhook endpoint signing secrets,
Products/one-time Prices, Portal configuration, branding, tax/settings, approved
payment methods, webhook endpoint/event allowlist, and notification templates.
Store `sk_*` and `whsec_*` only in environment-specific Secrets Manager paths;
never expose them to browser, router, CI build, issues, or evidence. Stripe
customer/session/payment-intent/event/Product/Price/Portal IDs are safe references
but tenant-scoped and environment-specific. Configure API version, success/cancel
origins, timeouts/retries, idempotency retention, webhook tolerance, reconciliation
schedule, and finance alert destinations.

## Test plan

* Unit-test authoritative tenant/price selection, idempotency, event allowlist,
  state monotonicity, auto-top-up threshold/cap, refund/dispute mapping, and safe
  serialization/logging.
* Stripe CLI/test-mode E2E: instant success, browser disconnect, success redirect
  before webhook, duplicate/replayed/out-of-order delivery, invalid/old signature,
  delayed success/failure, card failure, refund partial/full, dispute won/lost,
  Portal payment update, auto-top-up success/failure/monthly cap, cancellation.
* Crash after receipt, outbox, ledger credit, grant, provision enqueue, notification
  and response; retries create one economic/customer effect.
* Reconciliation joins Stripe balance/event export to source events and ledger;
  seed missing, duplicate and amount/currency mismatch and prove alert/resolution.
* Browser/DAST/CSP test proves no card fields or secret appear in Metrum DOM,
  network logs, analytics, URLs, screenshots, DB, traces, or support records.
* Load webhook bursts/retries and top-up races at 10x forecast; record p95, queue
  depth/lag, Stripe rate-limit handling, and no duplicate credit.

## Monitoring, rollback, and definition of done

Track Checkout creation/success/abandon, signed/invalid/duplicate events, webhook
age, fulfillment retries/dead letters, top-up success/failure/cap, refunds/
disputes, reconciliation variance, and customer notification. Stop new Checkout/
auto-top-up before rollback, continue webhook receipt, drain/replay idempotently,
and never remove credited balance without a linked reversal. Done after Finance
and Security approve a complete test-mode replay and live-account configuration
review; no live charge is required for pre-launch acceptance unless Finance
explicitly authorizes a minimal reversible transaction.
