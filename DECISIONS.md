# Prosumer Launch Decisions Register

Status labels below are intentional gates, not implementation assumptions.

**Commercial product vision (2026-07-19):** For Metrum-hosted / managed model-mix
access, the customer pays a **base monthly subscription** (Stripe) that includes a
**model-mix-specific usage allowance** (input + output tokens, and any other
plan-defined billable units). Usage **beyond the included allowance** is charged
via **x402** per-request payment and settlement. Stripe never runs on the router
inference hot path. x402 verification/settlement is a bounded local admission
step before upstream work and must not call Stripe.

## D1 — Tenancy model — NEEDS-HUMAN

Question: shared multi-tenant router versus dedicated tenant instance/namespace.

* **Shared logical tenancy:** one hosted router fleet; tenant-scoped callers,
  model-group access, budgets, subscription entitlement, included-allowance
  grants, and x402 payment scope. Fastest and least expensive; directly reuses
  existing caller isolation. Requires rigorous authorization, noisy-neighbor
  controls, and scoped telemetry.
* **Dedicated tenant instance/namespace:** one namespace/pod/config for each
  customer. Stronger blast-radius and network isolation, but makes provisioning,
  upgrades, cost, observability, and teardown materially more complex.
* **Hybrid:** shared Prosumer tier and a later dedicated tier. Adds product and
  operational complexity now.

Recommendation: launch a shared logical-tenancy Prosumer tier with hard per-tenant
limits and reserve dedicated deployment for a paid managed tier. Blocks tenant
provisioning topology and the production launch configuration.

## D2 — Ledger technology — NEEDS-HUMAN

Question: TigerBeetle or Postgres double-entry ledger.

* **Postgres:** relational, transactional, familiar alongside the control plane;
  lowest launch operations cost. Require append-only journal/posting tables,
  idempotency keys, serialization tests, and a clean `Ledger` interface.
* **TigerBeetle:** purpose-built high-throughput double-entry correctness and
  auditability; appropriate if event rate/financial-control requirements justify
  a second stateful cluster. Adds provisioning, backup, monitoring, and developer
  expertise.
* **Unstructured wallet counters:** rejected; cannot prove debit/credit integrity.

Recommendation: Postgres double-entry at launch behind a narrow interface, with
TigerBeetle adapter acceptance tests specified but not deployed. Revisit before
launch if forecast exceeds 100 ledger postings/sec sustained or finance requires
TigerBeetle-grade controls. Blocks final ledger schema and infrastructure scope.

The ledger must track at least: subscription recognition, included-allowance
grant/consume/expire/reset, x402 settlement credits/debits, adjustments,
reversals, and reconciliation labels. It is **not** a prepaid-credit wallet as the
primary commercial model.

## D3 — Billing model — DECIDED (product vision 2026-07-19)

**Decision:** For Metrum-hosted / managed model-mix product:

1. **Base monthly subscription** via Stripe Checkout + Customer Portal
   (recurring Price; SAQ-A hosted card capture).
2. **Included usage allowance** per billing period, sized by the plan’s
   **model mix** (at minimum input tokens + output tokens; additional billable
   units only when the plan explicitly defines them).
3. **Usage beyond included allowance** is charged via **x402** per-request
   payment and settlement on the router admission path.
4. Stripe Billing Meters / prepaid credit packs are **not** the primary overage
   rail for this product vision. Prepaid top-up may remain a future/optional
   rail only if product explicitly re-opens it; do not implement it as launch
   default.
5. Enterprise self-hosted remains signed `license.json` (+ optional online
   lease) and is out of this hosted billing model.

Implications:

* #524 owns Stripe subscription acquisition, webhooks, and Portal — not
  prepaid credit packs as the primary product.
* #506 is **launch-blocking** for hosted overage, not a fast-follow preview.
* #522 admits from **included-allowance grant** first; when allowance cannot
  cover the quoted request, admission requires a valid x402 settlement (or
  fails closed with a documented payment-required / allowance error).
* #534 is the integrated beta of subscription + included allowance + x402,
  not “Stripe meters instead of x402.”
* #520 epic language must describe subscription + included + x402, not
  prepaid-only pay-per-use.

## D4 — Pricing and plan catalog — NEEDS-HUMAN (numbers) / DECIDED (shape)

**Shape (decided):**

* Versioned **plan SKUs** map to:
  * base monthly Stripe Price ID;
  * allowed deployment-defined model groups (the “model mix”);
  * included billable units per period (input tokens, output tokens, and any
    plan-defined extras such as image units);
  * x402 price bands / quote rules per model group and request-shape bucket;
  * hard RPM/TPM/concurrency and traffic-shape defaults;
  * feature flags (admin reports, external policy, staging groups, etc.).
* Customer-facing price book stores **quoted customer charge** separately from
  **upstream provider cost**. Deterministic integer minor-unit rounding.
* Included allowance is granted at subscription period start (and on plan
  change per finance rules) and does not roll over unless Finance approves
  rollover.
* x402 quotes must be fully determined before upstream for v1 (pre-quoteable
  bands). Do not claim exact post-hoc token charging for x402 v1 unless a
  later approved scheme supports authorized-maximum settlement.

**Numbers (needs Finance):** base prices, included token amounts per SKU,
x402 band tables, minimum charge, tax, trial policy, refunds, and margin
targets. Blocks production price activation.

## D5 — AWS Marketplace motion — NEEDS-HUMAN

Options: SaaS listing with Marketplace Metering; AMI; container/EKS product;
parallel BYOC. Recommendation: do not block hosted launch on Marketplace; pursue
SaaS/private offers after the Stripe subscription + x402 path works, and retain
AMI or container packaging for enterprise BYOC. Stripe-hosted card capture keeps
PCI scope at SAQ-A. Blocks listing implementation and commercial launch claims.

Marketplace metering, if used later, is a control-plane integration and never an
inference hot-path dependency. It must not replace x402 for hosted overage
unless Product explicitly revises D3.

## D6 — Hosted data residency — NEEDS-HUMAN

Options: one launch region, region selectable at signup, or single-region shared
with future regional cells. Recommendation: one documented launch region and no
cross-region replication of telemetry; make customer region a control-plane field
and refuse unsupported-region signup. Blocks production topology and legal copy.

## D7 — Provider-cost exposure — NEEDS-HUMAN

Options: Metrum-pooled keys, BYOK, or hybrid. Recommendation: pooled keys only
with active subscription entitlement, included-allowance grants, x402 payment
for overage, and hard caps; BYOK can be a fast-follow because it changes
credential custody and pricing. Blocks initial plan catalog and admission
policy.

Pooled traffic must never run unbounded: no active subscription / no remaining
included allowance / no valid x402 settlement ⇒ deny before upstream.

## D8 — Identity and transactional email — NEEDS-HUMAN

Question: which managed OIDC/email services own signup, verified email, session
claims, MFA readiness, and customer communications.

Options: Amazon Cognito plus SES; a dedicated SaaS identity provider plus its
email/integration; or a self-hosted identity stack. Recommendation: use a managed
OIDC provider with authorization-code/PKCE and verified-email claims; on the AWS
launch stack, Cognito plus SES has the smallest new vendor surface. Self-hosting
identity is not recommended for this sprint. Blocks #521 and #525.

## D9 — Hosted DNS model — NEEDS-HUMAN

Options: one regional shared API hostname, one hostname per logical tenant, or a
dedicated hostname only for dedicated deployments. Recommendation: one regional
hostname for the shared Prosumer fleet; tenant identity comes from the caller
credential, not the hostname. Use per-tenant Route 53/TLS only for a paid dedicated
tier. Blocks #526 and public onboarding copy.

## D10 — Trial and free-credit policy — NEEDS-HUMAN

Options: no free trial; time-boxed trial subscription with reduced included
allowance after verified identity/risk; or card-verified trial that converts to
paid. Recommendation: small time-boxed trial **subscription entitlement** (not
withdrawable prepaid cash) after verified identity and abuse checks, with hard
per-tenant and provider-account caps. Finance/Legal/Security must approve term,
included allowance, eligibility, region, and appeal. Blocks #529 and public
pricing/trial copy.

Do **not** describe trial as “free prepaid credits” under the current vision.

## D11 — Customer-facing commercial error contract — NEEDS-HUMAN (HTTP status)

Required distinct, documented error types (names stable; HTTP status needs API
owner approval):

| Situation | Error type (stable) | Behavior |
| --- | --- | --- |
| No active subscription / entitlement | `entitlement-inactive` | Deny before upstream; link to subscribe/reactivate |
| Plan does not include model group | existing model-access denial | Unchanged |
| Included allowance insufficient and no x402 proof | `payment-required` (x402 challenge) | HTTP 402 with x402 PAYMENT-REQUIRED; no upstream |
| x402 proof invalid/expired/replayed | `payment-invalid` | Fresh challenge; no upstream |
| x402 infrastructure unavailable | `payment-unavailable` | Retryable 502/503; no upstream |
| Hard quota/RPM/TPM/abuse cap | existing quota/traffic errors | Distinct from payment/entitlement |
| Subscription past-due after grace | `entitlement-past-due` | Deny or restricted mode per finance policy |

Recommendation: never overload provider quota/billing errors for commercial
subscription or x402 failures. Include safe request ID and console remediation
URL/header. Blocks #506, #522, #525, #533, #534.

## D12 — x402 production enablement — NEEDS-HUMAN

Question: which facilitator, network/asset, treasury custody, KYT/AML, tax, and
refund policy are approved for production x402 overage.

Recommendation: implement protocol + test-network path as launch-blocking
engineering; gate **live production settlement** on Security/Finance/Legal
approval of facilitator, wallet custody, compliance, monitoring, and rollback.
Staging must prove the full challenge → pay → settle → one upstream path before
any customer-facing enablement. Blocks production x402 flag and public claims.

## Historical note (superseded)

Earlier drafts recommended prepaid credits with opt-in auto-top-up as the primary
hosted billing model and treated x402 as optional/fast-follow. That is
**superseded** by D3 (2026-07-19). Issue text, sprint plans, and agent prompts
must follow subscription + included allowance + x402 overage. Do not reintroduce
prepaid packs or Stripe Billing Meters as the primary overage rail without an
explicit new decision.
